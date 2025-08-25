package domino

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/134130/gh-domino/git"
	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/color"

	"github.com/134130/gh-domino/internal/spinner"
	"github.com/134130/gh-domino/internal/stackedpr"
	"github.com/134130/gh-domino/internal/util"
)

var stderr = func(msg string, args ...interface{}) {}

func success(msg string) {
	stderr("%s %s\n", color.Green("✔"), msg)
}

func failure(msg string) {
	stderr("%s %s\n", color.Red("✘"), msg)
}

func Run(ctx context.Context, cfg Config) error {
	sp := spinner.New("Fetching pull requests...", cfg.Writer)
	sp.Start()

	stderr = func(msg string, args ...interface{}) {
		_, _ = fmt.Fprintf(cfg.Writer, msg, args...)
	}

	if err := git.Fetch(ctx, "origin"); err != nil {
		sp.Stop()
		return fmt.Errorf("failed to fetch origin: %s", err)
	}

	prs, err := git.ListPullRequests(ctx)
	if err != nil {
		sp.Stop()
		return fmt.Errorf("failed to list pull requests: %s", err)
	}

	mergedPRs, err := git.ListMergedPullRequests(ctx)
	if err != nil {
		sp.Stop()
		return fmt.Errorf("failed to list merged pull requests: %s", err)
	}

	prHeadShas := make(map[string]string)
	for _, pr := range prs {
		sha, err := git.RevParse(ctx, "origin/"+pr.HeadRefName)
		if err != nil {
			stderr("Could not get SHA for %s: %v\n", pr.HeadRefName, err)
			os.Exit(1)
		}
		prHeadShas[pr.HeadRefName] = sha
	}

	roots, err := stackedpr.BuildDependencyTree(ctx, prs, mergedPRs, prHeadShas)
	if err != nil {
		sp.Stop()
		return err
	}
	sp.Stop()
	success("Fetching pull requests...")

	stderr(stackedpr.RenderDependencyTree(roots))
	stderr("\n\n")

	if *cfg.DryRun {
		stderr("Dry run mode enabled. The following PRs would be rebased:\n")
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
			if errors.Is(err, git.ErrRebaseConflict) {
				// TODO: Handle rebase conflict error message gracefully
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
		stderr("Error determining state for PR %s: %v\n", pr.PRNumberString(), err)
		// Continue to children even if parent has an error
	} else if isBroken {
		if newBase == "" {
			newBase = pr.BaseRefName
		}
		brokenPR := stackedpr.RebaseInfo{PR: pr, NewBase: newBase, Upstream: upstream}
		if err := handleBrokenPR(ctx, brokenPR, cfg, prHeadShas); err != nil {
			return totalProcessed, fmt.Errorf("failed to handle broken PR %s: %w", brokenPR.PR.PRNumberString(), err)
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
	if *cfg.DryRun {
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
	if *cfg.DryRun {
		updateBaseBranchString := ""
		if brokenPR.PR.BaseRefName != brokenPR.NewBase {
			updateBaseBranchString = fmt.Sprintf(" (update base branch to %s)", color.Cyan(brokenPR.NewBase))
		}
		stderr("  %s%s\n", brokenPR.PR.String(), updateBaseBranchString)
		prHeadShas[brokenPR.PR.HeadRefName] = "dummy-sha-after-rebase"
		return nil
	}

	// --- Rebase ---
	if !*cfg.Auto {
		stderr("PR %s needs to be rebased onto %s\n", brokenPR.PR.String(), color.Cyan(brokenPR.NewBase))
		var cmd string
		if brokenPR.Upstream != "" {
			cmd = fmt.Sprintf("git rebase --onto %s %s %s", "origin/"+brokenPR.NewBase, brokenPR.Upstream, brokenPR.PR.HeadRefName)
		} else {
			cmd = fmt.Sprintf("git rebase %s %s", "origin/"+brokenPR.NewBase, brokenPR.PR.HeadRefName)
		}
		stderr("  Suggested command: %s\n", color.Yellow(cmd))
		response, err := util.AskForConfirmation("Run this command?")
		if err != nil {
			return fmt.Errorf("error reading input: %s", err)
		} else if !response {
			stderr("Skipping.\n")
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
		}
		return fmt.Errorf("failed to rebase %s onto %s: %v", brokenPR.PR.HeadRefName, brokenPR.NewBase, err)
	}
	success(msg)

	// --- Push ---
	if !*cfg.Auto {
		stderr("Rebase completed successfully.\n")
		response, err := util.AskForConfirmation("Continue to push the rebased branch and update the PR?")
		if err != nil {
			return fmt.Errorf("error reading input: %s", err)
		} else if !response {
			stderr("Skipping push and PR update.\n")
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
		if !*cfg.Auto {
			stderr("Branch %s needs to be updated to base branch %s\n", color.Cyan(brokenPR.PR.HeadRefName), color.Cyan(brokenPR.NewBase))
			cmd := fmt.Sprintf("gh pr edit %s --base %s", brokenPR.PR.PRNumberString(), brokenPR.NewBase)
			stderr("  Suggested command: %s\n", color.Yellow(cmd))
			response, err := util.AskForConfirmation("Run this command?")
			if err != nil {
				return fmt.Errorf("error reading input: %s", err)
			} else if !response {
				stderr("Skipping base branch update.\n")
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
