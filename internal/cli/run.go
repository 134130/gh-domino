package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/app/gitkitexec"
	"github.com/134130/gh-domino/internal/app/gitkitstore"
	"github.com/134130/gh-domino/internal/output"
	"github.com/134130/gitkit/gitcmd"
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

func runTUI(context.Context, Config, io.Writer, io.Writer) error {
	return fmt.Errorf("tui is not implemented yet; use `gh domino list` or `gh domino plan`")
}

func runList(ctx context.Context, cfg Config, stdout io.Writer) error {
	plan, err := buildPlan(ctx, cfg)
	if err != nil {
		return err
	}
	return output.RenderList(stdout, plan, output.ListOptions{
		Format: cfg.Format,
		State:  cfg.State,
		Flat:   cfg.Flat,
	})
}

func runPlan(ctx context.Context, cfg Config, stdout io.Writer) error {
	plan, err := buildPlan(ctx, cfg)
	if err != nil {
		return err
	}
	plan, err = plan.Select(cfg.Selection())
	if err != nil {
		return err
	}
	return output.RenderPlan(stdout, plan, cfg.Format)
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

	runner := gitcmd.NewRunner()
	executor := gitkitexec.New(runner)
	result, err := executor.Execute(ctx, plan, app.ExecuteOptions{
		Remote:   cfg.Remote,
		Parallel: cfg.Parallel,
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
