package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/app/gitkitexec"
	"github.com/134130/gh-domino/internal/app/gitkitstore"
	"github.com/134130/gh-domino/internal/output"
	"github.com/134130/gh-domino/internal/tui"
	"github.com/134130/gitkit/gitcmd"
)

var (
	buildPlanFunc        = buildPlan
	runSelectorFunc      = tui.RunSelector
	executePlanFunc      = executePlan
	isTerminalReaderFunc = isTerminalReader
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return RunWithIO(ctx, args, os.Stdin, stdout, stderr)
}

func RunWithIO(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := newCommand(stdin, stdout, stderr, runConfig)
	cmd.SetArgs(args)
	return cmd.ExecuteContext(ctx)
}

func runConfig(ctx context.Context, cfg Config, stdin io.Reader, stdout, stderr io.Writer) error {
	switch cfg.Command {
	case CommandTUI:
		return runTUI(ctx, cfg, stdout, stderr)
	case CommandList:
		return runList(ctx, cfg, stdout, stderr)
	case CommandPlan:
		return runPlan(ctx, cfg, stdout, stderr)
	case CommandMerge:
		return runMerge(ctx, cfg, stdin, stdout, stderr)
	default:
		return fmt.Errorf("unknown command: %s", cfg.Command)
	}
}

func runTUI(ctx context.Context, cfg Config, stdout, _ io.Writer) error {
	loadPlan := func(ctx context.Context, includeClean bool, progress app.ProgressSink) (*app.Plan, error) {
		next := cfg
		next.IncludeClean = includeClean
		return buildPlanFunc(ctx, next, progress)
	}

	plan, err := tui.RunPlanLoader(ctx, tui.PlanLoadOptions{
		IncludeClean: cfg.IncludeClean,
		LoadPlan:     loadPlan,
		Output:       stdout,
		NoColor:      cfg.NoColor,
		Verbose:      cfg.Verbose,
	})
	if err != nil {
		return err
	}

	result, err := runSelectorFunc(ctx, plan, tui.Options{
		IncludeClean: cfg.IncludeClean,
		Parallel:     cfg.Parallel,
		LoadPlan:     loadPlan,
		Output:       stdout,
		NoColor:      cfg.NoColor,
		Verbose:      cfg.Verbose,
	})
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	runResult, err := tui.RunExecution(ctx, result.Plan, tui.ExecutionOptions{
		Parallel: result.Parallel,
		Output:   stdout,
		NoColor:  cfg.NoColor,
		Verbose:  cfg.Verbose,
		Execute: func(ctx context.Context, plan *app.Plan, parallel int, progress app.ProgressSink) (*app.RunResult, error) {
			return executePlanFunc(ctx, cfg, plan, parallel, progress)
		},
	})
	if runResult != nil {
		if renderErr := output.RenderRunResult(stdout, runResult, cfg.Format, cfg.NoColor); renderErr != nil {
			return renderErr
		}
	}
	return err
}

func runList(ctx context.Context, cfg Config, stdout, stderr io.Writer) error {
	progress := commandProgress(cfg, stderr)
	defer progress.Close()
	plan, err := buildPlanFunc(ctx, cfg, progress.sink())
	if err != nil {
		return err
	}
	return output.RenderList(stdout, plan, output.ListOptions{
		Format:  cfg.Format,
		State:   cfg.State,
		Flat:    cfg.Flat,
		NoColor: cfg.NoColor,
	})
}

func runPlan(ctx context.Context, cfg Config, stdout, stderr io.Writer) error {
	progress := commandProgress(cfg, stderr)
	defer progress.Close()
	plan, err := buildPlanFunc(ctx, cfg, progress.sink())
	if err != nil {
		return err
	}
	selected, err := plan.Select(cfg.Selection())
	if err != nil {
		return err
	}
	return output.RenderPlan(stdout, plan, selected, output.PlanOptions{
		Format:  cfg.Format,
		NoColor: cfg.NoColor,
	})
}

func runMerge(ctx context.Context, cfg Config, stdin io.Reader, stdout, stderr io.Writer) error {
	if cfg.DryRun {
		return runPlan(ctx, cfg, stdout, stderr)
	}
	if !cfg.Yes && cfg.Format == output.FormatJSON {
		return fmt.Errorf("merge requires --yes or --dry-run with --format json")
	}
	if !cfg.Yes && !isTerminalReaderFunc(stdin) {
		return fmt.Errorf("merge requires --yes when stdin is not a terminal")
	}

	progress := commandProgress(cfg, stderr)
	defer progress.Close()
	plan, err := buildPlanFunc(ctx, cfg, progress.sink())
	if err != nil {
		return err
	}
	selected, err := plan.Select(cfg.Selection())
	if err != nil {
		return err
	}

	if !cfg.Yes {
		if err := output.RenderPlan(stdout, plan, selected, output.PlanOptions{
			Format:  cfg.Format,
			NoColor: cfg.NoColor,
		}); err != nil {
			return err
		}
		if len(selected.Actions) == 0 {
			return nil
		}
		confirmed, err := confirm(stdin, stderr, "Run selected actions?")
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	result, err := executePlanFunc(ctx, cfg, selected, cfg.Parallel, progress.sink())
	if result != nil {
		if renderErr := output.RenderRunResult(stdout, result, cfg.Format, cfg.NoColor); renderErr != nil {
			return renderErr
		}
	}
	return err
}

func executePlan(ctx context.Context, cfg Config, plan *app.Plan, parallel int, progress app.ProgressSink) (*app.RunResult, error) {
	runner := app.NewProgressRunner(gitcmd.NewRunner(), progress)
	executor := gitkitexec.New(runner)
	return executor.Execute(ctx, plan, app.ExecuteOptions{
		Remote:   cfg.Remote,
		Parallel: parallel,
		Progress: progress,
	})
}

func buildPlan(ctx context.Context, cfg Config, progress app.ProgressSink) (*app.Plan, error) {
	if cfg.Repo != "" {
		return nil, fmt.Errorf("--repo is not implemented yet")
	}
	runner := app.NewProgressRunner(gitcmd.NewRunner(), progress)
	store := gitkitstore.New(runner)
	opts := cfg.PlanOptions()
	opts.Progress = progress
	return app.NewPlanner(store).BuildPlan(ctx, opts)
}

type commandProgressHandle struct {
	progress *terminalProgress
}

func commandProgress(cfg Config, stderr io.Writer) commandProgressHandle {
	if cfg.Format == output.FormatJSON {
		return commandProgressHandle{}
	}
	return commandProgressHandle{
		progress: newTerminalProgress(stderr, cfg.NoColor, cfg.Verbose),
	}
}

func (h commandProgressHandle) sink() app.ProgressSink {
	if h.progress == nil {
		return nil
	}
	return h.progress
}

func (h commandProgressHandle) Close() {
	if h.progress != nil {
		h.progress.Close()
	}
}
