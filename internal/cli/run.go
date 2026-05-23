package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/output"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cfg, err := Parse(args)
	if err != nil {
		return err
	}

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
	plan, err = plan.SelectStacks(cfg.StackNumbers)
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
	return fmt.Errorf("merge execution is not implemented yet; use `gh domino plan` or `gh domino merge --dry-run`")
}

func buildPlan(ctx context.Context, cfg Config) (*app.Plan, error) {
	if cfg.Repo != "" {
		return nil, fmt.Errorf("--repo is not implemented yet")
	}
	return app.NewPlanner(app.NewLegacyStore()).BuildPlan(ctx, cfg.PlanOptions())
}
