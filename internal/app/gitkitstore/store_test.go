package gitkitstore

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/134130/gitkit/gitcmd"
)

type fakeRunner struct {
	results  map[string]gitcmd.Result
	commands []gitcmd.Command
}

func (r *fakeRunner) Run(_ context.Context, cmd gitcmd.Command) (gitcmd.Result, error) {
	r.commands = append(r.commands, cmd)
	result, ok := r.results[cmd.String()]
	if !ok {
		return gitcmd.Result{Command: cmd}, fmt.Errorf("unexpected command: %s", cmd.String())
	}
	result.Command = cmd
	return result, nil
}

func (r *fakeRunner) Start(_ context.Context, cmd gitcmd.Command) (gitcmd.Process, error) {
	return nil, fmt.Errorf("unexpected start: %s", cmd.String())
}

func TestOpenPullRequestsUsesAuthorAndSorts(t *testing.T) {
	runner := &fakeRunner{results: map[string]gitcmd.Result{
		"gh pr list --author cooper --json " + pullRequestFields: {
			Stdout: []byte(`[
				{"number":53,"title":"baz","state":"OPEN","baseRefName":"stack-1","headRefName":"stack-2"},
				{"number":52,"title":"bar","state":"OPEN","baseRefName":"main","headRefName":"stack-1"}
			]`),
		},
	}}

	prs, err := New(runner).OpenPullRequests(context.Background(), "cooper")
	if err != nil {
		t.Fatalf("OpenPullRequests returned error: %v", err)
	}

	got := []int{prs[0].Number, prs[1].Number}
	want := []int{52, 53}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PR order mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestMergedPullRequestsUsesAuthorStateAndLimit(t *testing.T) {
	command := "gh pr list --author @me --state merged --limit 10 --json " + pullRequestFields
	runner := &fakeRunner{results: map[string]gitcmd.Result{
		command: {Stdout: []byte(`[]`)},
	}}

	prs, err := New(runner).MergedPullRequests(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("MergedPullRequests returned error: %v", err)
	}
	if len(prs) != 0 {
		t.Fatalf("expected no PRs, got %#v", prs)
	}

	if len(runner.commands) != 1 || runner.commands[0].String() != command {
		t.Fatalf("command mismatch: %#v", runner.commands)
	}
}

func TestGitOperationsUseGitClient(t *testing.T) {
	runner := &fakeRunner{results: map[string]gitcmd.Result{
		"git fetch upstream":                  {Stdout: []byte{}},
		"git rev-parse upstream/feature":      {Stdout: []byte("abc123\n")},
		"git merge-base upstream/main abc123": {Stdout: []byte("base123\n")},
	}}

	store := New(runner)
	if err := store.Fetch(context.Background(), "upstream"); err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	sha, err := store.RefSHA(context.Background(), "upstream/feature")
	if err != nil {
		t.Fatalf("RefSHA returned error: %v", err)
	}
	if sha != "abc123" {
		t.Fatalf("sha mismatch: %q", sha)
	}

	base, err := store.MergeBase(context.Background(), "upstream/main", "abc123")
	if err != nil {
		t.Fatalf("MergeBase returned error: %v", err)
	}
	if base != "base123" {
		t.Fatalf("merge base mismatch: %q", base)
	}
}

func TestDefaultBranchCachesByRemote(t *testing.T) {
	command := "git symbolic-ref --quiet --short refs/remotes/origin/HEAD"
	runner := &fakeRunner{results: map[string]gitcmd.Result{
		command: {Stdout: []byte("origin/main\n")},
	}}

	store := New(runner)
	for range 2 {
		branch, err := store.DefaultBranch(context.Background(), "origin")
		if err != nil {
			t.Fatalf("DefaultBranch returned error: %v", err)
		}
		if branch != "main" {
			t.Fatalf("branch mismatch: want %q, got %q", "main", branch)
		}
	}

	if len(runner.commands) != 1 || runner.commands[0].String() != command {
		t.Fatalf("expected cached default branch lookup, got %#v", runner.commands)
	}
}
