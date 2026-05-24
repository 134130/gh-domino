package gitkitexec

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gitkit/gitcmd"
)

type fakeResponse struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

type fakeRunner struct {
	responses map[string][]fakeResponse
	commands  []gitcmd.Command
}

func newFakeRunner(responses map[string][]fakeResponse) *fakeRunner {
	return &fakeRunner{responses: responses}
}

func (r *fakeRunner) Run(_ context.Context, cmd gitcmd.Command) (gitcmd.Result, error) {
	r.commands = append(r.commands, cmd)
	key := cmd.String()
	var response fakeResponse
	if queue := r.responses[key]; len(queue) > 0 {
		response = queue[0]
		r.responses[key] = queue[1:]
	}

	result := gitcmd.Result{
		Command:  cmd,
		Stdout:   []byte(response.stdout),
		Stderr:   []byte(response.stderr),
		ExitCode: response.exitCode,
	}
	if response.err != nil {
		return result, response.err
	}
	if response.exitCode != 0 {
		return result, &gitcmd.ExitError{
			Result: result,
			Err:    fmt.Errorf("exit status %d", response.exitCode),
		}
	}
	return result, nil
}

func (r *fakeRunner) Start(_ context.Context, cmd gitcmd.Command) (gitcmd.Process, error) {
	r.commands = append(r.commands, cmd)
	return nil, nil
}

func TestExecutorRepairInCurrentWorktree(t *testing.T) {
	runner := newFakeRunner(map[string][]fakeResponse{
		"git branch --show-current":                               {{stdout: "main\n"}},
		"git status --porcelain":                                  {{stdout: ""}},
		"git rev-list --left-right --count origin/feature...HEAD": {{stdout: "0\t0\n"}},
	})
	executor := New(runner)
	plan := &app.Plan{Actions: []app.Action{
		repairAction(52, "stack-1", "feature", "main", "abc123"),
	}}

	result, err := executor.Execute(context.Background(), plan, app.ExecuteOptions{Remote: "origin"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(result.Actions) != 1 || result.Actions[0].Status != app.ActionStatusSuccess {
		t.Fatalf("result mismatch: %#v", result.Actions)
	}
	want := []string{
		"git branch --show-current",
		"git status --porcelain",
		"git switch feature",
		"git rev-list --left-right --count origin/feature...HEAD",
		"git pull --rebase origin feature",
		"git rebase --onto origin/main abc123 feature",
		"git push --force-with-lease origin feature",
		"gh pr edit 52 --base main",
		"git switch main",
	}
	if got := commandStrings(runner.commands); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestExecutorStashesAndRestoresCurrentWorktree(t *testing.T) {
	runner := newFakeRunner(map[string][]fakeResponse{
		"git branch --show-current":                               {{stdout: "main\n"}},
		"git status --porcelain":                                  {{stdout: " M file.go\n"}},
		"git stash push -u -m gh-domino: preserve working tree":   {{stdout: "Saved working directory and index state\n"}},
		"git stash list --format=%gd -n 1":                        {{stdout: "stash@{0}\n"}},
		"git rev-list --left-right --count origin/feature...HEAD": {{stdout: "0\t0\n"}},
	})
	executor := New(runner)
	plan := &app.Plan{Actions: []app.Action{
		repairAction(52, "main", "feature", "", ""),
	}}

	result, err := executor.Execute(context.Background(), plan, app.ExecuteOptions{Remote: "origin"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(result.Actions) != 1 || result.Actions[0].Status != app.ActionStatusSuccess {
		t.Fatalf("result mismatch: %#v", result.Actions)
	}
	want := []string{
		"git branch --show-current",
		"git status --porcelain",
		"git stash push -u -m gh-domino: preserve working tree",
		"git stash list --format=%gd -n 1",
		"git switch feature",
		"git rev-list --left-right --count origin/feature...HEAD",
		"git pull --rebase origin feature",
		"git rebase origin/main feature",
		"git push --force-with-lease origin feature",
		"git switch main",
		"git stash apply stash@{0}",
		"git stash drop stash@{0}",
	}
	if got := commandStrings(runner.commands); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestExecutorUpdateBranchDoesNotPrepareCurrentWorktree(t *testing.T) {
	runner := newFakeRunner(nil)
	executor := New(runner)
	plan := &app.Plan{Actions: []app.Action{{
		ID:   "update-branch-52",
		Kind: app.ActionUpdateBranch,
		PR:   pull(52, "main", "feature"),
	}}}

	result, err := executor.Execute(context.Background(), plan, app.ExecuteOptions{Remote: "origin"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(result.Actions) != 1 || result.Actions[0].Status != app.ActionStatusSuccess {
		t.Fatalf("result mismatch: %#v", result.Actions)
	}
	want := []string{"gh pr update-branch --rebase 52"}
	if got := commandStrings(runner.commands); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestExecutorFailsRepairWithLocalUnpushedCommits(t *testing.T) {
	runner := newFakeRunner(map[string][]fakeResponse{
		"git branch --show-current":                               {{stdout: "main\n"}},
		"git status --porcelain":                                  {{stdout: ""}},
		"git rev-list --left-right --count origin/feature...HEAD": {{stdout: "0\t1\n"}},
	})
	executor := New(runner)
	plan := &app.Plan{Actions: []app.Action{
		repairAction(52, "main", "feature", "", ""),
	}}

	result, err := executor.Execute(context.Background(), plan, app.ExecuteOptions{Remote: "origin"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if got := result.Actions[0]; got.Status != app.ActionStatusFailed || !strings.Contains(got.Error, "unpushed commit") {
		t.Fatalf("result mismatch: %#v", got)
	}
	for _, command := range commandStrings(runner.commands) {
		if strings.HasPrefix(command, "git pull") || strings.HasPrefix(command, "git rebase") || strings.HasPrefix(command, "git push") {
			t.Fatalf("unexpected command after local unpushed check: %s", command)
		}
	}
}

func TestExecutorSkipsSelectedDependentActionWhenDependencyFails(t *testing.T) {
	runner := newFakeRunner(map[string][]fakeResponse{
		"git branch --show-current":                              {{stdout: "main\n"}},
		"git status --porcelain":                                 {{stdout: ""}},
		"git rev-list --left-right --count origin/parent...HEAD": {{stdout: "0\t0\n"}},
		"git rebase origin/main parent":                          {{exitCode: 1, stderr: "conflict\n"}},
	})
	executor := New(runner)
	parent := repairAction(52, "main", "parent", "", "")
	child := repairAction(53, "parent", "child", "", "")
	child.DependsOn = []string{parent.ID}
	plan := &app.Plan{Actions: []app.Action{parent, child}}

	result, err := executor.Execute(context.Background(), plan, app.ExecuteOptions{Remote: "origin"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(result.Actions) != 2 {
		t.Fatalf("action count mismatch: %#v", result.Actions)
	}
	if result.Actions[0].Status != app.ActionStatusFailed || result.Actions[0].Error != "rebase conflict" {
		t.Fatalf("parent result mismatch: %#v", result.Actions[0])
	}
	if result.Actions[1].Status != app.ActionStatusSkipped {
		t.Fatalf("child result mismatch: %#v", result.Actions[1])
	}
	want := []string{
		"git branch --show-current",
		"git status --porcelain",
		"git switch parent",
		"git rev-list --left-right --count origin/parent...HEAD",
		"git pull --rebase origin parent",
		"git rebase origin/main parent",
		"git rebase --abort",
		"git switch main",
	}
	if got := commandStrings(runner.commands); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestExecutorRepairInDetachedWorktree(t *testing.T) {
	worktreeDir := t.TempDir()
	path := worktreeDir + "/gh-domino-52-feature-branch"
	runner := newFakeRunner(nil)
	executor := New(runner)
	plan := &app.Plan{Actions: []app.Action{
		repairAction(52, "stack-1", "feature/branch", "main", "abc123"),
	}}

	result, err := executor.Execute(context.Background(), plan, app.ExecuteOptions{
		Remote:      "origin",
		WorktreeDir: worktreeDir,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(result.Actions) != 1 || result.Actions[0].Status != app.ActionStatusSuccess {
		t.Fatalf("result mismatch: %#v", result.Actions)
	}
	want := []string{
		"git worktree add --detach " + path + " origin/feature/branch",
		"git rebase --onto origin/main abc123 @" + path,
		"git push --force-with-lease origin HEAD:feature/branch @" + path,
		"gh pr edit 52 --base main @" + path,
		"git worktree remove --force " + path,
	}
	if got := commandStringsWithDir(runner.commands); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestExecutorRejectsParallelExecution(t *testing.T) {
	executor := New(newFakeRunner(nil))

	_, err := executor.Execute(context.Background(), &app.Plan{}, app.ExecuteOptions{Parallel: 2})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestExecutorReturnsSetupErrorWhenCurrentBranchCannotBeRead(t *testing.T) {
	runner := newFakeRunner(map[string][]fakeResponse{
		"git branch --show-current": {{err: errors.New("detached")}},
	})
	executor := New(runner)
	plan := &app.Plan{Actions: []app.Action{
		repairAction(52, "main", "feature", "", ""),
	}}

	_, err := executor.Execute(context.Background(), plan, app.ExecuteOptions{})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func repairAction(number int, base, head, newBase, upstream string) app.Action {
	return app.Action{
		ID:       fmt.Sprintf("repair-pr-%d", number),
		Kind:     app.ActionRepairPR,
		PR:       pull(number, base, head),
		NewBase:  newBase,
		Upstream: upstream,
	}
}

func pull(number int, base, head string) gitobj.PullRequest {
	return gitobj.PullRequest{
		Number:      number,
		Title:       head,
		BaseRefName: base,
		HeadRefName: head,
	}
}

func commandStrings(commands []gitcmd.Command) []string {
	out := make([]string, 0, len(commands))
	for _, command := range commands {
		out = append(out, command.String())
	}
	return out
}

func commandStringsWithDir(commands []gitcmd.Command) []string {
	out := make([]string, 0, len(commands))
	for _, command := range commands {
		value := command.String()
		if command.Dir != "" {
			value += " @" + command.Dir
		}
		out = append(out, value)
	}
	return out
}
