package app

import (
	"context"
	"fmt"

	"github.com/134130/gh-domino/git"
	"github.com/134130/gh-domino/gitobj"
)

type LegacyStore struct{}

func NewLegacyStore() LegacyStore {
	return LegacyStore{}
}

func (s LegacyStore) Fetch(ctx context.Context, remote string) error {
	return git.Fetch(ctx, remote)
}

func (s LegacyStore) OpenPullRequests(ctx context.Context, author string) ([]gitobj.PullRequest, error) {
	if author != "" && author != "@me" {
		return nil, fmt.Errorf("custom author filters are not supported yet: %s", author)
	}
	return git.ListPullRequests(ctx)
}

func (s LegacyStore) MergedPullRequests(ctx context.Context, author string, limit int) ([]gitobj.PullRequest, error) {
	if author != "" && author != "@me" {
		return nil, fmt.Errorf("custom author filters are not supported yet: %s", author)
	}
	if limit != 0 && limit != 30 {
		return nil, fmt.Errorf("custom merged PR limits are not supported yet: %d", limit)
	}
	return git.ListMergedPullRequests(ctx)
}

func (s LegacyStore) RefSHA(ctx context.Context, ref string) (string, error) {
	return git.RevParse(ctx, ref)
}

func (s LegacyStore) DefaultBranch(ctx context.Context) (string, error) {
	return git.GetDefaultBranch(ctx)
}

func (s LegacyStore) MergeBase(ctx context.Context, a, b string) (string, error) {
	return git.GetMergeBase(ctx, a, b)
}

func (s LegacyStore) IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	return git.IsAncestor(ctx, ancestor, descendant)
}
