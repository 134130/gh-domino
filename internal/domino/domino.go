package domino

import (
	"context"
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/134130/gh-domino/git"
	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/color"
	"github.com/134130/gh-domino/internal/ui"

	"github.com/134130/gh-domino/internal/spinner"
	"github.com/134130/gh-domino/internal/stackedpr"
	"github.com/134130/gh-domino/internal/util"
)

var write = func(msg string, args ...interface{}) {}

func success(msg string) {
	write("%s %s\n", color.Green("✔"), msg)
}

func failure(msg string) {
	write("%s %s\n", color.Red("✘"), msg)
}

func Run(ctx context.Context, cfg Config) error {
	write = func(msg string, args ...interface{}) {
		_, _ = fmt.Fprintf(cfg.Writer, msg, args...)
	}

	var err error
	if cfg.DumpTo != "" {
		git.CommandRunner, err = git.NewLoggingRunner(cfg.DumpTo)
		if err != nil {
			return fmt.Errorf("failed to create logging runner: %w", err)
		}
	}

	ctx, cancel := context.WithCancel(ctx)

	m := ui.NewModel(ctx, cancel)
	p := tea.NewProgram(m, tea.WithOutput(cfg.Writer))

	go func() {
		if _, err := p.Run(); err != nil {
			cancel()
			failure(fmt.Sprintf("Failed to start UI: %v", err))
		}
	}()

	defer func() {
		p.Quit()
		p.Wait()
	}()

	lw := ui.NewLogWriter(m, p)
	_, _ = lw.WriteString("git fetch origin")
	if err := git.Fetch(ctx, "origin", git.WithStdout(lw)); err != nil {
		return fmt.Errorf("failed to fetch origin: %s", err)
	}

	_, _ = lw.WriteString("gh pr list --author @me")
	prs, err := git.ListPullRequests(ctx)
	if err != nil {
		return fmt.Errorf("failed to list pull requests: %s", err)
	}

	_, _ = lw.WriteString("gh pr list --author @me --state merged --search sort:updated")
	mergedPRs, err := git.ListMergedPullRequests(ctx)
	if err != nil {
		return fmt.Errorf("failed to list merged pull requests: %s", err)
	}

	prHeadShas := make(map[string]string)
	for _, pr := range prs {
		_, _ = fmt.Fprintf(lw, "git rev-parse origin/%s", pr.HeadRefName)
		sha, err := git.RevParse(ctx, "origin/"+pr.HeadRefName)
		if err != nil {
			write("Could not get SHA for %s: %v\n", pr.HeadRefName, err)
			return fmt.Errorf("could not get SHA for %s: %w", pr.HeadRefName, err)
		}
		prHeadShas[pr.HeadRefName] = sha
	}

	roots, err := stackedpr.BuildDependencyTree(ctx, prs, mergedPRs, prHeadShas)
	if err != nil {
		return err
	}

	p.Send(ui.DoneMsg{})
	p.Wait()
	success("Fetching pull requests...")

	write("\n")
	write(stackedpr.RenderDependencyTree(roots))
	write("\n\n")

	if cfg.DryRun {
		write("Dry run mode enabled. The following PRs would be rebased:\n")
	}

	prMap := make(map[string]gitobj.PullRequest)
	for _, pr := range prs {
		prMap[pr.HeadRefName] = pr
	}
	mergedPRsByHeadRef := make(map[string]gitobj.PullRequest)
	for _, pr := range mergedPRs {
		mergedPRsByHeadRef[pr.HeadRefName] = pr
	}

	processedPRs := make(map[int]bool)

	totalProcessed := 0
	for _, root := range roots {
		processed, err := processDependencyTree(ctx, root, prMap, mergedPRsByHeadRef, prHeadShas, cfg, mergedPRs, processedPRs)
		if err != nil {
			var errRebaseConflict *ErrRebaseConflict
			if errors.As(err, &errRebaseConflict) {
				failure(fmt.Sprintf(`Failed to handle broken PR %s due to rebase conflicts.
  Please resolve the conflicts manually and re-run the tool if needed.
  You can use the following command to rebase manually:
      %s`, errRebaseConflict.BrokenPR.PR.PRNumberString(), errRebaseConflict.Command()))
			} else {
				failure(err.Error())
			}
			return nil
		}
		totalProcessed += processed
	}

	if totalProcessed == 0 {
		success("No broken PRs found.")
	}

	return nil
}

// processDependencyTree recursively traverses the dependency tree and handles broken PRs.
func processDependencyTree(
	ctx context.Context,
	node *stackedpr.Node,
	prMap map[string]gitobj.PullRequest,
	mergedPRsByHeadRef map[string]gitobj.PullRequest,
	prHeadShas map[string]string,
	cfg Config,
	mergedPRs []gitobj.PullRequest,
	processedPRs map[int]bool,
) (int, error) {
	if node == nil {
		return 0, nil
	}
	pr := node.Value

	// Skip already processed PRs
	if processedPRs[pr.Number] {
		return 0, nil
	}
	processedPRs[pr.Number] = true

	totalProcessed := 0

	isBroken, newBase, upstream, err := determinePRState(ctx, pr, prMap, mergedPRsByHeadRef, prHeadShas, cfg, mergedPRs)
	if err != nil {
		write("Error determining state for PR %s: %v\n", pr.PRNumberString(), err)
		// Continue to children even if parent has an error
	} else if isBroken {
		if newBase == "" {
			newBase = pr.BaseRefName
		}
		brokenPR := stackedpr.RebaseInfo{PR: pr, NewBase: newBase, Upstream: upstream}
		if err := handleBrokenPR(ctx, brokenPR, cfg, prHeadShas); err != nil {
			return totalProcessed, err
		}
		totalProcessed++
	}

	for _, child := range node.Children {
		processed, err := processDependencyTree(ctx, child, prMap, mergedPRsByHeadRef, prHeadShas, cfg, mergedPRs, processedPRs)
		if err != nil {
			return totalProcessed, err
		}
		totalProcessed += processed
	}
	return totalProcessed, nil
}

// determinePRState checks if a pull request is "broken" and needs to be rebased.
// A PR is considered broken if:
// 1. Its base branch corresponds to a PR that has already been merged.
// 2. In a dry run, its parent PR has been marked for rebase.
// 3. It has diverged from the default branch, containing commits from another merged PR.
// It returns whether the PR is broken and what its new base branch ('onto') should be.
func determinePRState(
	ctx context.Context,
	pr gitobj.PullRequest,
	prMap map[string]gitobj.PullRequest,
	mergedPRsByHeadRef map[string]gitobj.PullRequest,
	prHeadShas map[string]string,
	cfg Config,
	mergedPRs []gitobj.PullRequest,
) (isBroken bool, newBase string, upstream string, err error) {
	// --- Check 1: Is the base a merged PR? ---
	// This applies only to root PRs in a stack.
	if _, isStackedPR := prMap[pr.BaseRefName]; !isStackedPR {
		if mergedBasePR, isMerged := mergedPRsByHeadRef[pr.BaseRefName]; isMerged {
			if mergedBasePR.MergeCommit.Sha != "" && len(mergedBasePR.Commits) > 0 {
				lastCommit := mergedBasePR.Commits[len(mergedBasePR.Commits)-1].Oid
				isAncestor, err := git.IsAncestor(ctx, lastCommit, mergedBasePR.MergeCommit.Sha)
				if err != nil {
					return false, "", "", fmt.Errorf("failed to check ancestry: %v", err)
				}
				if !isAncestor {
					// Squash merge detected
					return true, mergedBasePR.BaseRefName, lastCommit, nil
				}
			}
			return true, mergedBasePR.BaseRefName, "", nil
		}
	}

	// --- Check 2: Has the PR diverged from its base? ---
	// This can happen if the base branch itself was updated (e.g., parent PR rebased).
	isDiverged := false
	if _, ok := prMap[pr.BaseRefName]; ok { // Only check divergence for stacked PRs
		baseShaOnOrigin, err := git.RevParse(ctx, "origin/"+pr.BaseRefName)
		if err != nil {
			return false, "", "", fmt.Errorf("could not get SHA for base %s: %v", pr.BaseRefName, err)
		}
		headSha := prHeadShas[pr.HeadRefName]
		mergeBase, err := git.GetMergeBase(ctx, "origin/"+pr.BaseRefName, headSha)
		if err != nil {
			return false, "", "", fmt.Errorf("could not get merge base for %s and %s: %v", pr.BaseRefName, pr.HeadRefName, err)
		}
		if mergeBase != baseShaOnOrigin {
			isDiverged = true
		}
	}

	// --- Check 3: In a dry run, was the parent PR "rebased" earlier in this run? ---
	isParentRebasedInDryRun := false
	if cfg.DryRun {
		if parentPR, ok := prMap[pr.BaseRefName]; ok {
			baseShaInMap := prHeadShas[parentPR.HeadRefName]
			if baseShaInMap == "dummy-sha-after-rebase" {
				isParentRebasedInDryRun = true
			}
		}
	}

	if isDiverged || isParentRebasedInDryRun {
		return true, "", "", nil // 'newBase' will be set to pr.BaseRefName by the caller
	}

	// --- Check 4: Does this root PR contain commits from another merged PR? ---
	// This handles cases where a PR was based on another branch that got merged while this PR was open.
	defaultBranch, err := git.GetDefaultBranch(ctx)
	if err != nil {
		return false, "", "", fmt.Errorf("could not get default branch: %v", err)
	}
	if pr.BaseRefName == defaultBranch {
		headSha := prHeadShas[pr.HeadRefName]
		mergeBase, err := git.GetMergeBase(ctx, "origin/"+defaultBranch, headSha)
		if err != nil {
			return false, "", "", fmt.Errorf("could not get merge base for %s: %v", pr.HeadRefName, err)
		}

		// Find the PR that introduced the merge base commit.
		for _, mergedPR := range mergedPRs {
			for _, commit := range mergedPR.Commits {
				if mergeBase == commit.Oid {
					return true, mergedPR.BaseRefName, "", nil
				}
			}
		}
	}

	return false, "", "", nil
}

// handleBrokenPR performs the necessary actions on a broken PR, such as rebasing,
// pushing, and updating the base branch on the remote. It handles both dry-run and real modes.
func handleBrokenPR(
	ctx context.Context,
	brokenPR stackedpr.RebaseInfo,
	cfg Config,
	prHeadShas map[string]string,
) error {
	if cfg.DryRun {
		updateBaseBranchString := ""
		if brokenPR.PR.BaseRefName != brokenPR.NewBase {
			updateBaseBranchString = fmt.Sprintf(" (update base branch to %s)", color.Cyan(brokenPR.NewBase))
		}
		write("  %s%s\n", brokenPR.PR.String(), updateBaseBranchString)
		prHeadShas[brokenPR.PR.HeadRefName] = "dummy-sha-after-rebase"
		return nil
	}

	// --- Rebase ---
	if !cfg.Auto {
		write("PR %s needs to be rebased onto %s\n", brokenPR.PR.String(), color.Cyan(brokenPR.NewBase))
		var cmd string
		if brokenPR.Upstream != "" {
			cmd = fmt.Sprintf("git rebase --onto %s %s %s", "origin/"+brokenPR.NewBase, brokenPR.Upstream, brokenPR.PR.HeadRefName)
		} else {
			cmd = fmt.Sprintf("git rebase %s %s", "origin/"+brokenPR.NewBase, brokenPR.PR.HeadRefName)
		}
		write("  Suggested command: %s\n", color.Yellow(cmd))
		response, err := util.AskForConfirmation("Run this command?")
		if err != nil {
			return fmt.Errorf("error reading input: %s", err)
		} else if !response {
			write("Skipping.\n")
			return nil // Not an error, just skipping this PR.
		}
	}

	msg := fmt.Sprintf("Rebasing %s onto %s...", brokenPR.PR.String(), color.Cyan(brokenPR.NewBase))
	if err := spinner.New(msg, cfg.Writer).Run(func() error {
		return git.Rebase(ctx, fmt.Sprintf("origin/%s", brokenPR.NewBase), brokenPR.Upstream, brokenPR.PR.HeadRefName)
	}); err != nil {
		if errors.Is(err, git.ErrRebaseConflict) {
			// Attempt to abort the rebase if there was a conflict.
			if err := git.AbortRebase(ctx); err != nil {
				return fmt.Errorf("rebase conflict occurred and failed to abort rebase: %v", err)
			}
			return &ErrRebaseConflict{BrokenPR: brokenPR}
		}
		return fmt.Errorf("failed to rebase %s onto %s: %v", brokenPR.PR.HeadRefName, brokenPR.NewBase, err)
	}
	success(msg)

	// --- Push ---
	if !cfg.Auto {
		write("Rebase completed successfully.\n")
		response, err := util.AskForConfirmation("Continue to push the rebased branch and update the PR?")
		if err != nil {
			return fmt.Errorf("error reading input: %s", err)
		} else if !response {
			write("Skipping push and PR update.\n")
			return nil
		}
	}

	msg = fmt.Sprintf("Pushing %s...", brokenPR.PR.PRNumberString())
	if err := spinner.New(msg, cfg.Writer).Run(func() error {
		return git.Push(ctx, brokenPR.PR.HeadRefName)
	}); err != nil {
		return fmt.Errorf("failed to push branch %s: %v", brokenPR.PR.HeadRefName, err)
	}
	success(msg)

	newSha, err := git.RevParse(ctx, "origin/"+brokenPR.PR.HeadRefName)
	if err != nil {
		return fmt.Errorf("could not get new SHA for %s: %v", brokenPR.PR.HeadRefName, err)
	}
	prHeadShas[brokenPR.PR.HeadRefName] = newSha

	// --- Update Base Branch ---
	if brokenPR.PR.BaseRefName != brokenPR.NewBase {
		if !cfg.Auto {
			write("Branch %s needs to be updated to base branch %s\n", color.Cyan(brokenPR.PR.HeadRefName), color.Cyan(brokenPR.NewBase))
			cmd := fmt.Sprintf("gh pr edit %s --base %s", brokenPR.PR.PRNumberString(), brokenPR.NewBase)
			write("  Suggested command: %s\n", color.Yellow(cmd))
			response, err := util.AskForConfirmation("Run this command?")
			if err != nil {
				return fmt.Errorf("error reading input: %s", err)
			} else if !response {
				write("Skipping base branch update.\n")
				return nil // Not an error, just skipping this PR.
			}
		}

		msg = fmt.Sprintf("Updating base branch of %s to %s...", brokenPR.PR.PRNumberString(), color.Cyan(brokenPR.NewBase))

		if err = spinner.New(msg, cfg.Writer).Run(func() error {
			return git.UpdateBaseBranch(ctx, brokenPR.PR.Number, brokenPR.NewBase)
		}); err != nil {
			return fmt.Errorf("failed to update base branch for PR %s: %v", brokenPR.PR.PRNumberString(), err)
		}
		success(msg)
	}
	return nil
}
