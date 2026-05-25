package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/tui"
)

func TestRunMergeRequiresYesBeforeBuildingPlan(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Run(context.Background(), []string{"merge"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected no output, got stdout=%q stderr=%q", stdout.String(), stderr.String())
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
