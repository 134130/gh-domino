package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/app/gitkitexec"
	"github.com/134130/gh-domino/internal/app/gitkitstore"
	"github.com/134130/gh-domino/internal/output"
	"github.com/134130/gh-domino/internal/tui"
	"github.com/134130/gitkit/gitcmd"
)

var (
	buildPlanFunc       = buildPlan
	runSelectorFunc     = tui.RunSelector
	executeSelectedFunc = executeSelectedPlan
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cmd := newCommand(stdout, stderr, runConfig)
	cmd.SetArgs(args)
	return cmd.ExecuteContext(ctx)
}

func runConfig(ctx context.Context, cfg Config, stdout, stderr io.Writer) error {
	switch cfg.Command {
	case CommandTUI:
		return runTUI(ctx, cfg, stdout, stderr)
	case CommandList:
		return runList(ctx, cfg, stdout)
	case CommandPlan:
		return runPlan(ctx, cfg, stdout)
	case CommandMerge:
		return runMerge(ctx, cfg, stdout)
	default:
		return fmt.Errorf("unknown command: %s", cfg.Command)
	}
}

func runTUI(ctx context.Context, cfg Config, stdout, _ io.Writer) error {
	plan, err := buildPlanFunc(ctx, cfg)
	if err != nil {
		return err
	}

	loadPlan := func(ctx context.Context, includeClean bool) (*app.Plan, error) {
		next := cfg
		next.IncludeClean = includeClean
		return buildPlanFunc(ctx, next)
	}

	result, err := runSelectorFunc(ctx, plan, tui.Options{
		IncludeClean: cfg.IncludeClean,
		Parallel:     cfg.Parallel,
		LoadPlan:     loadPlan,
		Output:       stdout,
		NoColor:      cfg.NoColor,
	})
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	return executeSelectedFunc(ctx, cfg, result.Plan, result.Parallel, stdout)
}

func runList(ctx context.Context, cfg Config, stdout io.Writer) error {
	plan, err := buildPlan(ctx, cfg)
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

func runPlan(ctx context.Context, cfg Config, stdout io.Writer) error {
	plan, err := buildPlan(ctx, cfg)
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

func runMerge(ctx context.Context, cfg Config, stdout io.Writer) error {
	if cfg.DryRun {
		return runPlan(ctx, cfg, stdout)
	}
	if !cfg.Yes {
		return fmt.Errorf("merge requires --yes until TUI confirmation is implemented")
	}

	plan, err := buildPlan(ctx, cfg)
	if err != nil {
		return err
	}
	plan, err = plan.Select(cfg.Selection())
	if err != nil {
		return err
	}

	return executeSelectedFunc(ctx, cfg, plan, cfg.Parallel, stdout)
}

func executeSelectedPlan(ctx context.Context, cfg Config, plan *app.Plan, parallel int, stdout io.Writer) error {
	runner := gitcmd.NewRunner()
	executor := gitkitexec.New(runner)
	result, err := executor.Execute(ctx, plan, app.ExecuteOptions{
		Remote:   cfg.Remote,
		Parallel: parallel,
	})
	if result != nil {
		if renderErr := output.RenderRunResult(stdout, result, cfg.Format); renderErr != nil {
			return renderErr
		}
	}
	return err
}

func buildPlan(ctx context.Context, cfg Config) (*app.Plan, error) {
	if cfg.Repo != "" {
		return nil, fmt.Errorf("--repo is not implemented yet")
	}
	store := gitkitstore.New(gitcmd.NewRunner())
	return app.NewPlanner(store).BuildPlan(ctx, cfg.PlanOptions())
}
