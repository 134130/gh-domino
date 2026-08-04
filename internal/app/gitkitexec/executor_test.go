package gitkitexec

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gitkit/gitcmd"
)

type fakeResponse struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
	delay    time.Duration
}

type fakeRunner struct {
	mu        sync.Mutex
	responses map[string][]fakeResponse
	commands  []gitcmd.Command
}

func newFakeRunner(responses map[string][]fakeResponse) *fakeRunner {
	return &fakeRunner{responses: responses}
}

func (r *fakeRunner) Run(_ context.Context, cmd gitcmd.Command) (gitcmd.Result, error) {
	r.mu.Lock()
	r.commands = append(r.commands, cmd)
	key := cmd.String()
	var response fakeResponse
	if queue := r.responses[key]; len(queue) > 0 {
		response = queue[0]
		r.responses[key] = queue[1:]
	}
	r.mu.Unlock()

	if response.delay > 0 {
		time.Sleep(response.delay)
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
	r.mu.Lock()
	defer r.mu.Unlock()
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

func TestExecutorIgnoresUnselectedDependency(t *testing.T) {
	runner := newFakeRunner(map[string][]fakeResponse{
		"git branch --show-current":                             {{stdout: "main\n"}},
		"git status --porcelain":                                {{stdout: ""}},
		"git rev-list --left-right --count origin/child...HEAD": {{stdout: "0\t0\n"}},
	})
	executor := New(runner)
	child := repairAction(53, "parent", "child", "", "")
	child.DependsOn = []string{"repair-pr-52"}
	plan := &app.Plan{Actions: []app.Action{child}}

	result, err := executor.Execute(context.Background(), plan, app.ExecuteOptions{Remote: "origin"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if got := result.Actions[0]; got.Status != app.ActionStatusSuccess {
		t.Fatalf("result mismatch: %#v", got)
	}
	want := []string{
		"git branch --show-current",
		"git status --porcelain",
		"git switch child",
		"git rev-list --left-right --count origin/child...HEAD",
		"git pull --rebase origin child",
		"git rebase origin/parent child",
		"git push --force-with-lease origin child",
		"git switch main",
	}
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
	if got, want := result.Actions[0].RetryCommand, "git rebase origin/main parent"; got != want {
		t.Fatalf("retry command mismatch: want %q, got %q", want, got)
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

func TestManualRebaseCommand(t *testing.T) {
	tests := []struct {
		name   string
		action app.Action
		want   string
	}{
		{
			name:   "base branch",
			action: repairAction(52, "main", "feature", "", ""),
			want:   "git rebase upstream/main feature",
		},
		{
			name:   "new base and upstream commit",
			action: repairAction(52, "stack-1", "feature", "main", "abc123"),
			want:   "git rebase --onto upstream/main abc123 feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := manualRebaseCommand(tt.action, "upstream"); got != tt.want {
				t.Fatalf("manualRebaseCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExecutorSettlesSelectedDependencyBeforeChild(t *testing.T) {
	runner := newFakeRunner(map[string][]fakeResponse{
		"git branch --show-current":                              {{stdout: "main\n"}},
		"git status --porcelain":                                 {{stdout: ""}},
		"git rev-list --left-right --count origin/parent...HEAD": {{stdout: "0\t0\n"}},
		"git rev-list --left-right --count origin/child...HEAD":  {{stdout: "0\t0\n"}},
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
	assertStatuses(t, result, []app.ActionStatus{app.ActionStatusSuccess, app.ActionStatusSuccess})

	want := []string{
		"git branch --show-current",
		"git status --porcelain",
		"git switch parent",
		"git rev-list --left-right --count origin/parent...HEAD",
		"git pull --rebase origin parent",
		"git rebase origin/main parent",
		"git push --force-with-lease origin parent",
		"git fetch origin +refs/heads/parent:refs/remotes/origin/parent",
		"git merge-base --is-ancestor origin/main origin/parent",
		"git switch child",
		"git rev-list --left-right --count origin/child...HEAD",
		"git pull --rebase origin child",
		"git rebase origin/parent child",
		"git push --force-with-lease origin child",
		"git switch main",
	}
	if got := commandStrings(runner.commands); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestExecutorRepairInDetachedWorktree(t *testing.T) {
	runner := newFakeRunner(nil)
	executor := New(runner)
	plan := &app.Plan{Actions: []app.Action{
		repairAction(52, "stack-1", "feature/branch", "main", "abc123"),
	}}

	result, err := executor.Execute(context.Background(), plan, app.ExecuteOptions{
		Remote:   "origin",
		Parallel: 2,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(result.Actions) != 1 || result.Actions[0].Status != app.ActionStatusSuccess {
		t.Fatalf("result mismatch: %#v", result.Actions)
	}
	got := commandStringsWithDir(runner.commands)
	path := worktreePathFromAddCommand(t, got[0])
	want := []string{
		"git worktree add --detach " + path + " origin/feature/branch",
		"git rebase --onto origin/main abc123 @" + path,
		"git push --force-with-lease origin HEAD:feature/branch @" + path,
		"gh pr edit 52 --base main @" + path,
		"git worktree remove --force " + path,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestExecutorKeepsPlanOrderForParallelResults(t *testing.T) {
	runner := newFakeRunner(map[string][]fakeResponse{
		"git rebase origin/main": {{delay: 20 * time.Millisecond}, {}},
	})
	executor := New(runner)
	plan := &app.Plan{Actions: []app.Action{
		repairAction(52, "main", "slow", "", ""),
		repairAction(53, "main", "fast", "", ""),
	}}

	result, err := executor.Execute(context.Background(), plan, app.ExecuteOptions{
		Remote:   "origin",
		Parallel: 2,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	got := []int{result.Actions[0].Action.PR.Number, result.Actions[1].Action.PR.Number}
	if !reflect.DeepEqual(got, []int{52, 53}) {
		t.Fatalf("result order mismatch: %#v", result.Actions)
	}
}

func TestExecutorRejectsInvalidParallel(t *testing.T) {
	executor := New(newFakeRunner(nil))

	_, err := executor.Execute(context.Background(), &app.Plan{}, app.ExecuteOptions{Parallel: -1})
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

func TestExecutorEmitsProgressForRepairAction(t *testing.T) {
	runner := newFakeRunner(map[string][]fakeResponse{
		"git branch --show-current":                               {{stdout: "main\n"}},
		"git status --porcelain":                                  {{stdout: ""}},
		"git rev-list --left-right --count origin/feature...HEAD": {{stdout: "0\t0\n"}},
	})
	progress := &recordProgress{}
	executor := New(runner)
	plan := &app.Plan{Actions: []app.Action{
		repairAction(52, "stack-1", "feature", "main", "abc123"),
	}}

	result, err := executor.Execute(context.Background(), plan, app.ExecuteOptions{
		Remote:   "origin",
		Progress: progress,
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Actions[0].Status != app.ActionStatusSuccess {
		t.Fatalf("result mismatch: %#v", result.Actions)
	}

	got := progress.messages()
	for _, want := range []string{
		"Preparing 1 actions",
		"Preparing current worktree",
		"Repairing #52 feature onto main",
		"Switching to feature",
		"Rebasing feature onto origin/main",
		"Pushing feature",
		"Updating base for #52 to main",
		"Repaired #52 feature",
		"Execution finished",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected progress to contain %q, got %q", want, got)
		}
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

func assertStatuses(t *testing.T, result *app.RunResult, want []app.ActionStatus) {
	t.Helper()
	got := make([]app.ActionStatus, 0, len(result.Actions))
	for _, action := range result.Actions {
		got = append(got, action.Status)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses mismatch\nwant: %#v\n got: %#v\nresult: %#v", want, got, result.Actions)
	}
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

func worktreePathFromAddCommand(t *testing.T, command string) string {
	t.Helper()
	fields := strings.Fields(command)
	if len(fields) < 2 {
		t.Fatalf("invalid worktree add command: %q", command)
	}
	return fields[len(fields)-2]
}

type recordProgress struct {
	mu     sync.Mutex
	events []app.ProgressEvent
}

func (r *recordProgress) Progress(event app.ProgressEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recordProgress) messages() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	values := make([]string, 0, len(r.events))
	for _, event := range r.events {
		values = append(values, event.Message)
	}
	return strings.Join(values, "\n")
}
