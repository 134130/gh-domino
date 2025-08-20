package stackedpr

import (
	"context"
	"fmt"

	"github.com/134130/gh-domino/git"
	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/color"
)

type Node struct {
	Value        gitobj.PullRequest
	Children     []*Node
	OriginalBase *gitobj.PullRequest
}

func BuildDependencyTree(ctx context.Context, prs []gitobj.PullRequest, mergedPRs []gitobj.PullRequest, defaultBranch string) ([]*Node, error) {
	prMap := make(map[string]*Node)                           // HeadRefName -> Node
	isChild := make(map[string]bool)                          // HeadRefName -> isChild
	mergedPRsByHeadRef := make(map[string]gitobj.PullRequest) // HeadRefName -> Merged PR
	for _, pr := range mergedPRs {
		mergedPRsByHeadRef[pr.HeadRefName] = pr
	}

	// Create node for each PR
	for _, pr := range prs {
		prMap[pr.HeadRefName] = &Node{Value: pr}
	}

	// Build parent/child relationships
	for _, pr := range prs {
		// avoid self-parenting
		if pr.BaseRefName == pr.HeadRefName {
			continue
		}

		if parent, ok := prMap[pr.BaseRefName]; ok {
			node := prMap[pr.HeadRefName]
			parent.Children = append(parent.Children, node)
			isChild[pr.HeadRefName] = true
		}
	}

	// Determine roots
	var roots []*Node
	for _, pr := range prs {
		if !isChild[pr.HeadRefName] {
			roots = append(roots, prMap[pr.HeadRefName])
		}
	}

	// Find original base
	for _, node := range prMap {
		// If the base is another PR in the stack, skip (still part of the same stack).
		if _, ok := prMap[node.Value.BaseRefName]; ok {
			continue
		}

		// Base is a branch of a PR that was merged (branch not deleted yet).
		if mergedPR, ok := mergedPRsByHeadRef[node.Value.BaseRefName]; ok {
			node.OriginalBase = &mergedPR
			continue
		}

		if node.Value.BaseRefName != defaultBranch {
			// If the base is not the default branch, skip (not a root).
			continue
		}

		// Base points to default branch
		mergeBase, err := git.GetMergeBase(ctx, "origin/"+defaultBranch, "origin/"+node.Value.HeadRefName)
		if err != nil {
			// If we cannot find a merge base, skip this node
			continue
		}

		// Heuristic: match merge-commit sha
		newOnMainCommits, err := git.GetBranchCommits(ctx, mergeBase, "origin/"+defaultBranch)
		if err != nil || len(newOnMainCommits) == 0 {
			continue
		}

		newOnMainCommitsSet := make(map[string]struct{}, len(newOnMainCommits))
		for _, sha := range newOnMainCommits {
			newOnMainCommitsSet[sha] = struct{}{}
		}

		found := false
		for i, mergedPR := range mergedPRs {
			if mergedPR.MergeCommit.Sha == "" {
				continue
			}

			if _, ok := newOnMainCommitsSet[mergedPR.MergeCommit.Sha]; ok {
				node.OriginalBase = &mergedPRs[i]
				found = true
				break
			}
		}
		if found {
			continue
		}
	}

	return roots, nil
}

type RebaseInfo struct {
	PR   gitobj.PullRequest
	Onto string
}

func (ri *RebaseInfo) String() string {
	return fmt.Sprintf("%s %s (%s ← %s)",
		ri.PR.PRNumberString(),
		ri.PR.Title,
		color.Cyan(ri.PR.BaseRefName),
		color.Blue(ri.PR.HeadRefName),
	)
}
