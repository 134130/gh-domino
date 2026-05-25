package git

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

type defaultBranchResponse struct {
	stdout string
	err    error
}

type defaultBranchRunner struct {
	responses map[string]defaultBranchResponse
	commands  []string
}

func (r *defaultBranchRunner) Run(_ context.Context, cmd string, args []string, mods ...CommandModifier) error {
	command := cmd + " " + strings.Join(args, " ")
	r.commands = append(r.commands, command)

	response, ok := r.responses[command]
	if !ok {
		return fmt.Errorf("unexpected command: %s", command)
	}

	execCmd := &exec.Cmd{}
	for _, mod := range mods {
		mod(execCmd)
	}
	if execCmd.Stdout != nil {
		_, _ = io.WriteString(execCmd.Stdout, response.stdout)
	}
	return response.err
}

func installDefaultBranchRunner(t *testing.T, runner *defaultBranchRunner) {
	t.Helper()
	previousRunner := CommandRunner
	previousCache := defaultBranchCache
	CommandRunner = runner
	defaultBranchCache = ""
	t.Cleanup(func() {
		CommandRunner = previousRunner
		defaultBranchCache = previousCache
	})
}

func TestGetDefaultBranchUsesLocalRemoteHead(t *testing.T) {
	runner := &defaultBranchRunner{responses: map[string]defaultBranchResponse{
		"git symbolic-ref --quiet --short refs/remotes/origin/HEAD": {stdout: "origin/main\n"},
	}}
	installDefaultBranchRunner(t, runner)

	branch, err := GetDefaultBranch(context.Background())
	if err != nil {
		t.Fatalf("GetDefaultBranch returned error: %v", err)
	}
	if branch != "main" {
		t.Fatalf("branch mismatch: want %q, got %q", "main", branch)
	}

	wantCommands := []string{"git symbolic-ref --quiet --short refs/remotes/origin/HEAD"}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("commands mismatch\nwant: %#v\n got: %#v", wantCommands, runner.commands)
	}
}

func TestGetDefaultBranchFallsBackToRemoteShow(t *testing.T) {
	runner := &defaultBranchRunner{responses: map[string]defaultBranchResponse{
		"git symbolic-ref --quiet --short refs/remotes/origin/HEAD": {
			err: &GitError{ExitCode: 1, Stderr: "not a symbolic ref"},
		},
		"git remote show origin": {stdout: `
* remote origin
  Fetch URL: git@github.com:owner/repo.git
  Push  URL: git@github.com:owner/repo.git
  HEAD branch: main
`},
	}}
	installDefaultBranchRunner(t, runner)

	branch, err := GetDefaultBranch(context.Background())
	if err != nil {
		t.Fatalf("GetDefaultBranch returned error: %v", err)
	}
	if branch != "main" {
		t.Fatalf("branch mismatch: want %q, got %q", "main", branch)
	}

	wantCommands := []string{
		"git symbolic-ref --quiet --short refs/remotes/origin/HEAD",
		"git remote show origin",
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("commands mismatch\nwant: %#v\n got: %#v", wantCommands, runner.commands)
	}
}

func TestGetDefaultBranchCachesResult(t *testing.T) {
	runner := &defaultBranchRunner{responses: map[string]defaultBranchResponse{
		"git symbolic-ref --quiet --short refs/remotes/origin/HEAD": {stdout: "origin/main\n"},
	}}
	installDefaultBranchRunner(t, runner)

	for range 2 {
		branch, err := GetDefaultBranch(context.Background())
		if err != nil {
			t.Fatalf("GetDefaultBranch returned error: %v", err)
		}
		if branch != "main" {
			t.Fatalf("branch mismatch: want %q, got %q", "main", branch)
		}
	}

	if len(runner.commands) != 1 {
		t.Fatalf("expected cached second lookup, got commands: %#v", runner.commands)
	}
}
