package app

import (
	"context"
	"fmt"
	"reflect"
	"strings"
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
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("actions mismatch\nwant: %#v\n got: %#v", want, got)
	}
	if got, want := plan.Pulls[1].State, PullStateClean; got != want {
		t.Fatalf("child state mismatch: want %q, got %q", want, got)
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

func TestPlannerIgnoresMergedAncestorWhenDefaultBranchAlreadyInHead(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	store.ancestors[pair("origin/main", "sha-feature")] = true
	store.ancestors[pair("p1c1", "sha-feature")] = true
	store.mergeBases[pair("origin/main", "sha-feature")] = "sha-main"

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

	if len(plan.Actions) != 0 {
		t.Fatalf("expected no actions, got %#v", plan.Actions)
	}
	if got, want := plan.Pulls[0].State, PullStateClean; got != want {
		t.Fatalf("state mismatch: want %q, got %q", want, got)
	}
	if plan.Pulls[0].OriginalBase != nil {
		t.Fatalf("expected no original base, got #%d", plan.Pulls[0].OriginalBase.Number)
	}
}

func TestPlannerIgnoresMergedPRCommitsMissingLocally(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	store.mergeBases[pair("origin/main", "sha-feature")] = "sha-main"
	store.ancestorErrors[pair("missing-commit", "sha-feature")] = fmt.Errorf("failed to run git: fatal: Not a valid commit name missing-commit")

	plan, err := NewPlanner(store).Build(ctx, Snapshot{
		OpenPullRequests: []gitobj.PullRequest{
			pr(52, "bar", "main", "feature"),
		},
		MergedPullRequests: []gitobj.PullRequest{
			withCommits(pr(51, "old merged", "main", "old-stack"), "missing-commit"),
		},
		HeadSHAs: map[string]string{
			"feature": "sha-feature",
		},
	}, PlanOptions{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(plan.Actions) != 0 {
		t.Fatalf("expected no actions, got %#v", plan.Actions)
	}
}

func TestPlannerBuildPlanEmitsProgress(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	progress := &recordProgress{}

	_, err := NewPlanner(store).BuildPlan(ctx, PlanOptions{Progress: progress})
	if err != nil {
		t.Fatalf("BuildPlan returned error: %v", err)
	}

	got := progress.messages()
	for _, want := range []string{
		"Fetching origin",
		"Fetched origin",
		"Loading open pull requests",
		"Loaded 0 open pull requests",
		"Loading merged pull requests",
		"Loaded 0 merged pull requests",
		"Building dependency tree",
		"Built dependency tree",
		"Classifying 0 pull requests",
		"Classified 0 pull requests",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected progress to contain %q, got %q", want, got)
		}
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
	defaultBranch  string
	refSHAs        map[string]string
	mergeBases     map[[2]string]string
	ancestors      map[[2]string]bool
	ancestorErrors map[[2]string]error
}

type recordProgress struct {
	events []ProgressEvent
}

func (r *recordProgress) Progress(event ProgressEvent) {
	r.events = append(r.events, event)
}

func (r *recordProgress) messages() string {
	values := make([]string, 0, len(r.events))
	for _, event := range r.events {
		values = append(values, event.Message)
	}
	return strings.Join(values, "\n")
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		defaultBranch:  "main",
		refSHAs:        map[string]string{},
		mergeBases:     map[[2]string]string{},
		ancestors:      map[[2]string]bool{},
		ancestorErrors: map[[2]string]error{},
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

func (s *fakeStore) DefaultBranch(context.Context, string) (string, error) {
	return s.defaultBranch, nil
}

func (s *fakeStore) MergeBase(_ context.Context, a, b string) (string, error) {
	if sha, ok := s.mergeBases[pair(a, b)]; ok {
		return sha, nil
	}
	return "", fmt.Errorf("missing merge base for %s and %s", a, b)
}

func (s *fakeStore) IsAncestor(_ context.Context, ancestor, descendant string) (bool, error) {
	if err, ok := s.ancestorErrors[pair(ancestor, descendant)]; ok {
		return false, err
	}
	return s.ancestors[pair(ancestor, descendant)], nil
}
