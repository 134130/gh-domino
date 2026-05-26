package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/tui"
)

func TestRunMergeRejectsNonInteractiveConfirmationBeforeBuildingPlan(t *testing.T) {
	restore := stubRunHooks(t)
	defer restore()

	var built bool
	buildPlanFunc = func(context.Context, Config, app.ProgressSink) (*app.Plan, error) {
		built = true
		return &app.Plan{}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := RunWithIO(context.Background(), []string{"merge"}, strings.NewReader("y\n"), &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "stdin is not a terminal") {
		t.Fatalf("unexpected error: %v", err)
	}
	if built {
		t.Fatalf("did not expect plan to be built")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected no output, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunMergeRejectsJSONConfirmationBeforeBuildingPlan(t *testing.T) {
	restore := stubRunHooks(t)
	defer restore()

	var built bool
	buildPlanFunc = func(context.Context, Config, app.ProgressSink) (*app.Plan, error) {
		built = true
		return &app.Plan{}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := RunWithIO(context.Background(), []string{"merge", "--format", "json"}, strings.NewReader("y\n"), &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "requires --yes or --dry-run") {
		t.Fatalf("unexpected error: %v", err)
	}
	if built {
		t.Fatalf("did not expect plan to be built")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected no output, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunMergeConfirmsThenExecutesSelectedPlan(t *testing.T) {
	restore := stubRunHooks(t)
	defer restore()
	restoreTerminal := stubTerminalInput(t, true)
	defer restoreTerminal()

	plan := cliMergePlan()
	buildPlanFunc = func(context.Context, Config, app.ProgressSink) (*app.Plan, error) {
		return plan, nil
	}

	var executedPlan *app.Plan
	executePlanFunc = func(_ context.Context, _ Config, selected *app.Plan, parallel int, _ app.ProgressSink) (*app.RunResult, error) {
		executedPlan = selected
		if parallel != 1 {
			t.Fatalf("parallel mismatch: %d", parallel)
		}
		return &app.RunResult{Actions: []app.ActionResult{{
			Action: selected.Actions[0],
			Status: app.ActionStatusSuccess,
		}}}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := RunWithIO(context.Background(), []string{"merge", "--no-color"}, strings.NewReader("yes\n"), &stdout, &stderr)
	if err != nil {
		t.Fatalf("RunWithIO returned error: %v", err)
	}
	if executedPlan == nil || len(executedPlan.Actions) != 1 || executedPlan.Actions[0].ID != "repair-pr-52" {
		t.Fatalf("executed unexpected plan: %#v", executedPlan)
	}
	if !strings.Contains(stdout.String(), "Actions") || !strings.Contains(stdout.String(), "repair #52 feature") {
		t.Fatalf("expected plan in stdout, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Results") {
		t.Fatalf("expected run result in stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Run selected actions? (y/N): ") {
		t.Fatalf("expected prompt on stderr, got %q", stderr.String())
	}
}

func TestRunMergeCancelDoesNotExecute(t *testing.T) {
	restore := stubRunHooks(t)
	defer restore()
	restoreTerminal := stubTerminalInput(t, true)
	defer restoreTerminal()

	buildPlanFunc = func(context.Context, Config, app.ProgressSink) (*app.Plan, error) {
		return cliMergePlan(), nil
	}

	var executed bool
	executePlanFunc = func(context.Context, Config, *app.Plan, int, app.ProgressSink) (*app.RunResult, error) {
		executed = true
		return &app.RunResult{}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := RunWithIO(context.Background(), []string{"merge", "--no-color"}, strings.NewReader("\n"), &stdout, &stderr)
	if err != nil {
		t.Fatalf("RunWithIO returned error: %v", err)
	}
	if executed {
		t.Fatalf("did not expect execution")
	}
	if !strings.Contains(stdout.String(), "repair #52 feature") {
		t.Fatalf("expected plan in stdout, got %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "Results") {
		t.Fatalf("did not expect result output, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Run selected actions? (y/N): ") {
		t.Fatalf("expected prompt on stderr, got %q", stderr.String())
	}
}

func TestRunMergeWithNoActionsDoesNotPromptOrExecute(t *testing.T) {
	restore := stubRunHooks(t)
	defer restore()
	restoreTerminal := stubTerminalInput(t, true)
	defer restoreTerminal()

	buildPlanFunc = func(context.Context, Config, app.ProgressSink) (*app.Plan, error) {
		return &app.Plan{}, nil
	}

	var executed bool
	executePlanFunc = func(context.Context, Config, *app.Plan, int, app.ProgressSink) (*app.RunResult, error) {
		executed = true
		return &app.RunResult{}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := RunWithIO(context.Background(), []string{"merge", "--no-color"}, strings.NewReader("yes\n"), &stdout, &stderr)
	if err != nil {
		t.Fatalf("RunWithIO returned error: %v", err)
	}
	if executed {
		t.Fatalf("did not expect execution")
	}
	if !strings.Contains(stdout.String(), "No actions.") {
		t.Fatalf("expected no-action plan in stdout, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("did not expect prompt, got %q", stderr.String())
	}
}

func TestRunMergeYesExecutesWithoutInteractiveInput(t *testing.T) {
	restore := stubRunHooks(t)
	defer restore()

	buildPlanFunc = func(context.Context, Config, app.ProgressSink) (*app.Plan, error) {
		return cliMergePlan(), nil
	}

	var executed bool
	executePlanFunc = func(context.Context, Config, *app.Plan, int, app.ProgressSink) (*app.RunResult, error) {
		executed = true
		return &app.RunResult{}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := RunWithIO(context.Background(), []string{"merge", "--yes"}, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("RunWithIO returned error: %v", err)
	}
	if !executed {
		t.Fatalf("expected execution")
	}
	if strings.Contains(stderr.String(), "Run selected actions?") {
		t.Fatalf("did not expect prompt, got %q", stderr.String())
	}
}

func TestRunDefaultUsesTUIPathAndQuitSkipsExecute(t *testing.T) {
	restore := stubRunHooks(t)
	defer restore()

	plan := &app.Plan{}
	var built bool
	var selected bool
	var executed bool
	buildPlanFunc = func(_ context.Context, cfg Config, _ app.ProgressSink) (*app.Plan, error) {
		built = true
		if cfg.Command != CommandTUI {
			t.Fatalf("expected TUI command, got %s", cfg.Command)
		}
		return plan, nil
	}
	runSelectorFunc = func(_ context.Context, got *app.Plan, opts tui.Options) (*tui.SelectorResult, error) {
		selected = true
		if got != plan {
			t.Fatalf("selector got unexpected plan")
		}
		if opts.LoadPlan == nil {
			t.Fatalf("expected clean-toggle plan loader")
		}
		return nil, nil
	}
	executePlanFunc = func(context.Context, Config, *app.Plan, int, app.ProgressSink) (*app.RunResult, error) {
		executed = true
		return nil, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Run(context.Background(), []string{}, &stdout, &stderr); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !built || !selected {
		t.Fatalf("expected build and selector to run, built=%v selected=%v", built, selected)
	}
	if executed {
		t.Fatalf("did not expect execution after selector quit")
	}
}

func TestRunTUIConfirmExecutesSelectedPlan(t *testing.T) {
	restore := stubRunHooks(t)
	defer restore()

	initial := &app.Plan{}
	selectedPlan := &app.Plan{Actions: []app.Action{{ID: "repair-pr-52"}}}
	buildPlanFunc = func(_ context.Context, _ Config, _ app.ProgressSink) (*app.Plan, error) {
		return initial, nil
	}
	runSelectorFunc = func(_ context.Context, got *app.Plan, opts tui.Options) (*tui.SelectorResult, error) {
		if got != initial {
			t.Fatalf("selector got unexpected plan")
		}
		if opts.Parallel != 2 {
			t.Fatalf("initial parallel mismatch: %d", opts.Parallel)
		}
		return &tui.SelectorResult{Plan: selectedPlan, Parallel: 4}, nil
	}

	var executedPlan *app.Plan
	var executedParallel int
	executePlanFunc = func(_ context.Context, _ Config, plan *app.Plan, parallel int, _ app.ProgressSink) (*app.RunResult, error) {
		executedPlan = plan
		executedParallel = parallel
		return &app.RunResult{}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Run(context.Background(), []string{"tui", "--parallel", "2"}, &stdout, &stderr); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if executedPlan != selectedPlan {
		t.Fatalf("executed unexpected plan")
	}
	if executedParallel != 4 {
		t.Fatalf("executed parallel mismatch: %d", executedParallel)
	}
}

func stubRunHooks(t *testing.T) func() {
	t.Helper()
	oldBuildPlan := buildPlanFunc
	oldRunSelector := runSelectorFunc
	oldExecute := executePlanFunc
	return func() {
		buildPlanFunc = oldBuildPlan
		runSelectorFunc = oldRunSelector
		executePlanFunc = oldExecute
	}
}

func stubTerminalInput(t *testing.T, terminal bool) func() {
	t.Helper()
	old := isTerminalReaderFunc
	isTerminalReaderFunc = func(io.Reader) bool {
		return terminal
	}
	return func() {
		isTerminalReaderFunc = old
	}
}

func cliMergePlan() *app.Plan {
	pr := gitobj.PullRequest{
		Number:      52,
		Title:       "feature",
		BaseRefName: "stack-1",
		HeadRefName: "feature",
	}
	action := app.Action{
		ID:      "repair-pr-52",
		Kind:    app.ActionRepairPR,
		PR:      pr,
		NewBase: "main",
		Reason:  app.ReasonMergedBase,
	}
	return &app.Plan{
		Pulls: []app.PullStatus{{
			PR:      pr,
			State:   app.PullStateBroken,
			Reason:  app.ReasonMergedBase,
			NewBase: "main",
		}},
		Actions: []app.Action{action},
	}
}
