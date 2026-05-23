package gitkitstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gitkit/ghcli"
	"github.com/134130/gitkit/gitcmd"
	"github.com/134130/gitkit/gitrepo"
)

const pullRequestFields = "number,title,url,author,state,isDraft,mergeCommit,baseRefName,headRefName,headRepository,commits"

type Store struct {
	git gitrepo.Client
	gh  ghcli.Client
}

type Option func(*Store)

func New(r gitcmd.Runner, opts ...Option) Store {
	store := Store{
		git: gitrepo.New(r),
		gh:  ghcli.New(r),
	}
	for _, opt := range opts {
		opt(&store)
	}
	return store
}

func WithDir(dir string) Option {
	return func(s *Store) {
		s.git = s.git.InDir(dir)
		s.gh = s.gh.InDir(dir)
	}
}

func WithStreams(stdout, stderr io.Writer) Option {
	return func(s *Store) {
		s.git = s.git.Stream(stdout, stderr)
		s.gh = s.gh.Stream(stdout, stderr)
	}
}

func (s Store) Fetch(ctx context.Context, remote string) error {
	return s.git.Fetch(ctx, remote)
}

func (s Store) OpenPullRequests(ctx context.Context, author string) ([]gitobj.PullRequest, error) {
	return s.listPullRequests(ctx, []string{
		"pr", "list",
		"--author", normalizedAuthor(author),
		"--json", pullRequestFields,
	})
}

func (s Store) MergedPullRequests(ctx context.Context, author string, limit int) ([]gitobj.PullRequest, error) {
	if limit < 1 {
		limit = 30
	}
	return s.listPullRequests(ctx, []string{
		"pr", "list",
		"--author", normalizedAuthor(author),
		"--state", "merged",
		"--limit", strconv.Itoa(limit),
		"--json", pullRequestFields,
	})
}

func (s Store) RefSHA(ctx context.Context, ref string) (string, error) {
	return s.git.RevParse(ctx, ref)
}

func (s Store) DefaultBranch(ctx context.Context, remote string) (string, error) {
	if strings.TrimSpace(remote) == "" {
		remote = "origin"
	}
	return s.git.DefaultBranch(ctx, remote)
}

func (s Store) MergeBase(ctx context.Context, a, b string) (string, error) {
	return s.git.MergeBase(ctx, a, b)
}

func (s Store) IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	return s.git.IsAncestor(ctx, ancestor, descendant)
}

func (s Store) listPullRequests(ctx context.Context, args []string) ([]gitobj.PullRequest, error) {
	out, err := s.gh.OutputBytes(ctx, args...)
	if err != nil {
		return nil, err
	}

	var prs []gitobj.PullRequest
	if err := json.NewDecoder(bytes.NewReader(out)).Decode(&prs); err != nil {
		return nil, fmt.Errorf("decode pull requests: %w", err)
	}

	sort.Slice(prs, func(i, j int) bool {
		return prs[i].Number < prs[j].Number
	})
	return prs, nil
}

func normalizedAuthor(author string) string {
	if author == "" {
		return "@me"
	}
	return author
}

var _ app.RepositoryStore = Store{}
