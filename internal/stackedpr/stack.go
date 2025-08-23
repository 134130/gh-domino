package stackedpr

import (
	"context"

	"github.com/134130/gh-domino/git"
	"github.com/134130/gh-domino/gitobj"
)

type Node struct {
	Value        gitobj.PullRequest
	Children     []*Node
	OriginalBase *gitobj.PullRequest
}

func BuildDependencyTree(ctx context.Context, prs []gitobj.PullRequest, mergedPRs []gitobj.PullRequest, defaultBranch string, prHeadShas map[string]string) ([]*Node, error) {
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

		// Heuristic: check if the PR's ancestor contains commits from a merged PR.
		for i, mergedPR := range mergedPRs {
			if len(mergedPR.Commits) == 0 {
				continue
			}
			if mergedPR.BaseRefName != node.Value.BaseRefName {
				continue
			}
			ancestorCommit := mergedPR.Commits[0].Oid

			isAncestor, err := git.IsAncestor(ctx, ancestorCommit, prHeadShas[node.Value.HeadRefName])
			if err == nil && isAncestor {
				node.OriginalBase = &mergedPRs[i]
				break
			}
		}
	}

	return roots, nil
}

type RebaseInfo struct {
	PR   gitobj.PullRequest
	Onto string
}
