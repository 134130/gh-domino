package domino

import (
	"context"
	"fmt"
	"os"

	"github.com/134130/gh-domino/git"
	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/color"
	"github.com/134130/gh-domino/internal/spinner"
	"github.com/134130/gh-domino/internal/stackedpr"
	"github.com/134130/gh-domino/internal/util"
)

func Run(ctx context.Context, cfg Config) error {
	sp := spinner.New("Fetching pull requests...", cfg.Writer)
	sp.Start()
	defer sp.Stop()

	stderr := func(msg string, args ...interface{}) {
		_, _ = fmt.Fprintf(cfg.Writer, msg, args...)
	}

	if err := git.Fetch(ctx, "origin"); err != nil {
		return fmt.Errorf("failed to fetch origin: %s", err)
	}

	prs, err := git.ListPullRequests(ctx)
	if err != nil {
		return fmt.Errorf("failed to list pull requests: %s", err)
	}

	mergedPRs, err := git.ListMergedPullRequests(ctx)
	if err != nil {
		return fmt.Errorf("failed to list merged pull requests: %s", err)
	}

	defaultBranch, err := git.GetDefaultBranch(ctx)
	if err != nil {
		return fmt.Errorf("failed to get default branch: %s", err)
	}

	roots, err := stackedpr.BuildDependencyTree(ctx, prs, mergedPRs, defaultBranch)
	if err != nil {
		return err
	}
	sp.Stop()
	stderr("%s Fetching pull requests... done\n", color.Green("✔"))

	stderr(stackedpr.RenderDependencyTree(roots) + "\n")

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
	prHeadShas := make(map[string]string)
	for _, pr := range prs {
		sha, err := git.RevParse(ctx, "origin/"+pr.HeadRefName)
		if err != nil {
			stderr("Could not get SHA for %s: %v\n", pr.HeadRefName, err)
			os.Exit(1)
		}
		prHeadShas[pr.HeadRefName] = sha
	}

	queue := make([]*stackedpr.Node, 0, len(roots))
	queue = append(queue, roots...)

	processedPRs := make(map[int]bool)
	foundBrokenPR := false

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		pr := node.Value

		if processedPRs[pr.Number] {
			continue
		}
		processedPRs[pr.Number] = true

		var onto string
		isBroken := false

		// Determine the correct base SHA for this PR.
		var baseSha string
		isParentRebasedInDryRun := false
		if parentPR, ok := prMap[pr.BaseRefName]; ok {
			// The base is another PR in the stack. Use its (potentially updated) SHA.
			baseSha = prHeadShas[parentPR.HeadRefName]
			if *cfg.DryRun && baseSha == "dummy-sha-after-rebase" {
				isParentRebasedInDryRun = true
			}
		} else {
			// The base is a regular branch. Get its SHA.
			var err error
			baseSha, err = git.RevParse(ctx, "origin/"+pr.BaseRefName)
			if err != nil {
				stderr("Could not get SHA for base %s: %v\n", pr.BaseRefName, err)
				continue
			}
		}

		// Check if the PR's base branch corresponds to a merged PR.
		// If so, this PR is considered broken and needs its base updated.
		if _, ok := prMap[pr.BaseRefName]; !ok { // It's a root PR
			if mergedBasePR, ok := mergedPRsByHeadRef[pr.BaseRefName]; ok {
				isBroken = true
				onto = mergedBasePR.BaseRefName
			}
		}

		// Check if the PR is broken due to divergence from its base.
		if !isBroken {
			if isParentRebasedInDryRun {
				isBroken = true
			} else {
				headSha := prHeadShas[pr.HeadRefName]
				isAncestor, err := git.IsAncestor(ctx, baseSha, headSha)
				if err != nil {
					stderr("Could not check ancestor for %s: %v\n", pr.HeadRefName, err)
					continue
				}
				if !isAncestor {
					isBroken = true
				}
			}
		}

		if isBroken {
			foundBrokenPR = true
			// If 'onto' is not set yet, it means the PR is broken due to divergence,
			// so it should be rebased onto its own base branch.
			if onto == "" {
				onto = pr.BaseRefName
			}
			brokenPR := stackedpr.RebaseInfo{PR: pr, Onto: onto}

			if *cfg.DryRun {
				updateBaseBranchString := ""
				if brokenPR.PR.BaseRefName != brokenPR.Onto {
					updateBaseBranchString = fmt.Sprintf(" (update base branch to %s)", color.Cyan(brokenPR.Onto))
				}
				stderr("  %s%s\n", brokenPR.String(), updateBaseBranchString)
				prHeadShas[pr.HeadRefName] = "dummy-sha-after-rebase"
			} else {
				// Real rebase logic...
				if !*cfg.Auto {
					stderr("PR %s needs to be rebased onto %s\n", brokenPR.String(), color.Cyan(brokenPR.Onto))
					cmd := fmt.Sprintf("git rebase %s %s", "origin/"+brokenPR.Onto, brokenPR.PR.HeadRefName)
					stderr("  Suggested command: %s\n", color.Yellow(cmd))
					response, err := util.AskForConfirmation("Run this command?")
					if err != nil {
						stderr("Error reading input: %s\n", err.Error())
						os.Exit(1)
					}
					if !response {
						stderr("Skipping.\n")
						continue
					}
				}

				msg := fmt.Sprintf("Rebasing the PR %s onto %s...", brokenPR.String(), color.Cyan(brokenPR.Onto))
				sp = spinner.New(msg, cfg.Writer)
				sp.Start()

				if err := git.Rebase(ctx, fmt.Sprintf("origin/%s", brokenPR.Onto), brokenPR.PR.HeadRefName); err != nil {
					sp.Stop()
					stderr("Error rebasing: %v\n", err)
					os.Exit(1)
				}
				sp.Stop()
				stderr("%s %s\n", color.Green("✔"), msg)

				msg = fmt.Sprintf("Pushing the PR %s...", brokenPR.PR.PRNumberString())
				sp = spinner.New(msg, cfg.Writer)
				sp.Start()

				if err := git.Push(ctx, brokenPR.PR.HeadRefName); err != nil {
					sp.Stop()
					stderr("Error pushing: %v\n", err)
					os.Exit(1)
				}
				sp.Stop()
				stderr("%s %s\n", color.Green("✔"), msg)

				newSha, err := git.RevParse(ctx, "origin/"+pr.HeadRefName)
				if err != nil {
					stderr("Could not get new SHA for %s: %v\n", pr.HeadRefName, err)
					os.Exit(1)
				}
				prHeadShas[pr.HeadRefName] = newSha

				if brokenPR.PR.BaseRefName != brokenPR.Onto {
					msg = fmt.Sprintf("Updating the PR %s's base branch to %s...", brokenPR.PR.PRNumberString(), color.Cyan(brokenPR.Onto))
					sp = spinner.New(msg, cfg.Writer)
					sp.Start()
					if err := git.UpdateBaseBranch(ctx, brokenPR.PR.Number, brokenPR.Onto); err != nil {
						sp.Stop()
						stderr("Error updating base branch for PR %s: %v\n", brokenPR.PR.PRNumberString(), err)
						os.Exit(1)
					}
					sp.Stop()
					stderr("%s %s\n", color.Green("✔"), msg)
				}
			}
		}

		for _, child := range node.Children {
			queue = append(queue, child)
		}
	}

	if !foundBrokenPR {
		stderr("No broken PRs found.\n")
	}

	return nil
}
