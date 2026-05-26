package stackedpr

import "github.com/134130/gh-domino/gitobj"

type Node struct {
	Value        gitobj.PullRequest
	Children     []*Node
	OriginalBase *gitobj.PullRequest
}
