package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/134130/gh-domino/git"
	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/domino"
	"github.com/134130/gh-domino/internal/output"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cfg, err := Parse(args)
	if err != nil {
		return err
	}

	if cfg.DumpTo != "" {
		runner, err := git.NewLoggingRunner(cfg.DumpTo)
		if err != nil {
			return fmt.Errorf("failed to create logging runner: %w", err)
		}
		git.CommandRunner = runner
	}

	switch cfg.Command {
	case CommandLegacy:
		return runLegacy(ctx, cfg, stdout)
	case CommandTUI:
		return runTUI(ctx, cfg, stdout)
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

func runLegacy(ctx context.Context, cfg Config, stdout io.Writer) error {
	return domino.Run(ctx, domino.Config{
		Auto:      cfg.Yes,
		DryRun:    cfg.DryRun,
		DumpTo:    "",
		Headless:  cfg.Headless,
		RebaseAll: cfg.RebaseAll,
		Writer:    stdout,
	})
}

func runTUI(ctx context.Context, cfg Config, stdout io.Writer) error {
	return domino.Run(ctx, domino.Config{
		DumpTo:    "",
		RebaseAll: cfg.IncludeClean,
		Writer:    stdout,
	})
}

func runList(ctx context.Context, cfg Config, stdout io.Writer) error {
	if cfg.StackNumber != 0 {
		return fmt.Errorf("--stack is not implemented yet")
	}
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
	if cfg.PRNumber != 0 {
		return fmt.Errorf("--pr is not implemented yet")
	}
	if cfg.StackNumber != 0 {
		return fmt.Errorf("--stack is not implemented yet")
	}
	plan, err := buildPlan(ctx, cfg)
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
		return fmt.Errorf("merge requires --yes for now; use `gh domino plan` or `gh domino merge --dry-run` to preview")
	}
	if cfg.PRNumber != 0 {
		return fmt.Errorf("--pr is not implemented yet")
	}
	if cfg.StackNumber != 0 {
		return fmt.Errorf("--stack is not implemented yet")
	}
	if cfg.WorktreeDir != "" {
		return fmt.Errorf("--worktree-dir is not implemented yet")
	}
	if cfg.Parallel != 1 {
		return fmt.Errorf("--parallel is not implemented yet")
	}
	return domino.Run(ctx, domino.Config{
		Auto:      true,
		Headless:  true,
		RebaseAll: cfg.IncludeClean,
		Writer:    stdout,
	})
}

func buildPlan(ctx context.Context, cfg Config) (*app.Plan, error) {
	if cfg.Repo != "" {
		return nil, fmt.Errorf("--repo is not implemented yet")
	}
	return app.NewPlanner(app.NewLegacyStore()).BuildPlan(ctx, cfg.PlanOptions())
}
