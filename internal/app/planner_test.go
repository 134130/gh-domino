package app

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/134130/gh-domino/gitobj"
)

func TestPlannerBuildsRepairActionsForMergedBaseStack(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	store.refSHAs["origin/stack-2"] = "sha-stack-2"
	store.mergeBases[pair("origin/stack-2", "sha-stack-3")] = "sha-stack-2"
	store.ancestors[pair("p1c1", "merge-sha")] = true

	plan, err := NewPlanner(store).Build(ctx, Snapshot{
		OpenPullRequests: []gitobj.PullRequest{
			pr(52, "bar", "stack-1", "stack-2"),
			pr(53, "baz", "stack-2", "stack-3"),
		},
		MergedPullRequests: []gitobj.PullRequest{
			withMerge(withCommits(pr(51, "foo", "main", "stack-1"), "p1c1"), "merge-sha"),
		},
		HeadSHAs: map[string]string{
			"stack-2": "sha-stack-2",
			"stack-3": "sha-stack-3",
		},
	}, PlanOptions{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	got := actionSummaries(plan.Actions)
	want := []string{
		"repair-pr-52 repair_pr #52 base=main upstream= depends=[] reason=merged_base",
		"repair-pr-53 repair_pr #53 base=stack-2 upstream= depends=[repair-pr-52] reason=parent_diverged",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("actions mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestPlannerUsesUpstreamForSquashMergedBase(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()

	plan, err := NewPlanner(store).Build(ctx, Snapshot{
		OpenPullRequests: []gitobj.PullRequest{
			pr(52, "bar", "stack-1", "stack-2"),
		},
		MergedPullRequests: []gitobj.PullRequest{
			withMerge(withCommits(pr(51, "foo", "main", "stack-1"), "p1c1", "p1c2"), "squash-sha"),
		},
		HeadSHAs: map[string]string{
			"stack-2": "sha-stack-2",
		},
	}, PlanOptions{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(plan.Actions))
	}
	if got, want := plan.Actions[0].Upstream, "p1c2"; got != want {
		t.Fatalf("upstream mismatch: want %q, got %q", want, got)
	}
}

func TestPlannerIncludesCleanPRsWhenRequested(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	store.mergeBases[pair("origin/main", "sha-feature")] = "sha-main"

	plan, err := NewPlanner(store).Build(ctx, Snapshot{
		OpenPullRequests: []gitobj.PullRequest{
			pr(10, "feature", "main", "feature"),
		},
		HeadSHAs: map[string]string{
			"feature": "sha-feature",
		},
	}, PlanOptions{IncludeClean: true})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	got := actionSummaries(plan.Actions)
	want := []string{
		"update-branch-10 update_branch #10 base= upstream= depends=[] reason=rebase_all",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("actions mismatch\nwant: %#v\n got: %#v", want, got)
	}

	if len(plan.Pulls) != 1 {
		t.Fatalf("expected 1 pull status, got %d", len(plan.Pulls))
	}
	if got, want := plan.Pulls[0].State, PullStateUpdateable; got != want {
		t.Fatalf("state mismatch: want %q, got %q", want, got)
	}
}

func TestPlannerMarksMergedAncestorReason(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	store.ancestors[pair("p1c1", "sha-feature")] = true

	plan, err := NewPlanner(store).Build(ctx, Snapshot{
		OpenPullRequests: []gitobj.PullRequest{
			pr(52, "bar", "main", "feature"),
		},
		MergedPullRequests: []gitobj.PullRequest{
			withCommits(pr(51, "foo", "main", "stack-1"), "p1c1"),
		},
		HeadSHAs: map[string]string{
			"feature": "sha-feature",
		},
	}, PlanOptions{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(plan.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(plan.Actions))
	}
	if got, want := plan.Actions[0].Reason, ReasonMergedAncestor; got != want {
		t.Fatalf("reason mismatch: want %q, got %q", want, got)
	}
}

func actionSummaries(actions []Action) []string {
	summaries := make([]string, 0, len(actions))
	for _, action := range actions {
		summaries = append(summaries, fmt.Sprintf(
			"%s %s #%d base=%s upstream=%s depends=%v reason=%s",
			action.ID,
			action.Kind,
			action.PR.Number,
			action.NewBase,
			action.Upstream,
			action.DependsOn,
			action.Reason,
		))
	}
	return summaries
}

func pr(number int, title, base, head string) gitobj.PullRequest {
	return gitobj.PullRequest{
		Number:      number,
		Title:       title,
		State:       gitobj.PullRequestStateOpen,
		BaseRefName: base,
		HeadRefName: head,
	}
}

func withCommits(pr gitobj.PullRequest, commits ...string) gitobj.PullRequest {
	for _, commit := range commits {
		pr.Commits = append(pr.Commits, struct {
			Oid string `json:"oid"`
		}{Oid: commit})
	}
	return pr
}

func withMerge(pr gitobj.PullRequest, sha string) gitobj.PullRequest {
	pr.State = gitobj.PullRequestStateMerged
	pr.MergeCommit.Sha = sha
	return pr
}

type fakeStore struct {
	defaultBranch string
	refSHAs       map[string]string
	mergeBases    map[[2]string]string
	ancestors     map[[2]string]bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		defaultBranch: "main",
		refSHAs:       map[string]string{},
		mergeBases:    map[[2]string]string{},
		ancestors:     map[[2]string]bool{},
	}
}

func pair(a, b string) [2]string {
	return [2]string{a, b}
}

func (s *fakeStore) Fetch(context.Context, string) error {
	return nil
}

func (s *fakeStore) OpenPullRequests(context.Context, string) ([]gitobj.PullRequest, error) {
	return nil, nil
}

func (s *fakeStore) MergedPullRequests(context.Context, string, int) ([]gitobj.PullRequest, error) {
	return nil, nil
}

func (s *fakeStore) RefSHA(_ context.Context, ref string) (string, error) {
	if sha, ok := s.refSHAs[ref]; ok {
		return sha, nil
	}
	return "", fmt.Errorf("missing ref SHA for %s", ref)
}

func (s *fakeStore) DefaultBranch(context.Context) (string, error) {
	return s.defaultBranch, nil
}

func (s *fakeStore) MergeBase(_ context.Context, a, b string) (string, error) {
	if sha, ok := s.mergeBases[pair(a, b)]; ok {
		return sha, nil
	}
	return "", fmt.Errorf("missing merge base for %s and %s", a, b)
}

func (s *fakeStore) IsAncestor(_ context.Context, ancestor, descendant string) (bool, error) {
	return s.ancestors[pair(ancestor, descendant)], nil
}
