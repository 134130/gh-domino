package app

import (
	"context"

	"github.com/134130/gh-domino/gitobj"
)

type RepositoryStore interface {
	Fetch(ctx context.Context, remote string) error
	OpenPullRequests(ctx context.Context, author string) ([]gitobj.PullRequest, error)
	MergedPullRequests(ctx context.Context, author string, limit int) ([]gitobj.PullRequest, error)
	RefSHA(ctx context.Context, ref string) (string, error)
	DefaultBranch(ctx context.Context) (string, error)
	MergeBase(ctx context.Context, a, b string) (string, error)
	IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error)
}
