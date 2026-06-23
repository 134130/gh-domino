package cli

import (
	"context"
	"io"
	"strings"

	"github.com/134130/gh-domino/internal/output"
	"github.com/spf13/cobra"
)

type commandHandler func(context.Context, Config, io.Reader, io.Writer, io.Writer) error

const (
	rootShort = "Repair stacked GitHub pull requests after a parent PR is merged or rebased"
	rootLong  = `gh-domino repairs existing stacked pull requests after a parent PR is merged or rebased.

It reads GitHub PR metadata and remote branch refs, builds the dependency tree, and plans the rebase/base-update actions needed to keep the remaining stack mergeable.

Use gh domino plan to preview actions, gh domino merge to execute them from the command line, or gh domino for the interactive TUI. Execution preserves dependency order: parent PRs are repaired before child PRs. Parallel execution is used only when independent stacks can safely run at the same time.`
	rootExample = `  gh domino
  gh domino list --state broken
  gh domino plan --chain 52
  gh domino merge --yes --chain 52 --parallel 4`

	tuiLong = `Inspect stacked PRs, select repair actions, and execute the selected plan from an interactive terminal UI.

The TUI uses the same planner and executor as the plan and merge commands. Execution preserves dependency order, and parallel execution is used only when independent stacks can safely run at the same time.`

	listLong = `Show stacked PRs and their repair status.

The list command builds the dependency tree from GitHub PR metadata and remote branch refs, then reports whether each PR is clean, broken, or updateable.`

	planLong = `Preview the repair actions gh-domino would run without changing branches.

Actions are ordered by stack dependency. With --chain, gh-domino selects the path from the upper/root PR down to the selected lower/child PR, preserving parent-before-child order.`

	mergeLong = `Execute the selected repair plan.

gh-domino preserves dependency order: parent actions run before child actions. --parallel runs independent stacks concurrently when possible, but dependent PRs in the same chain are still serialized.

Repair actions may rebase branches, push with --force-with-lease, and update PR base branches on GitHub. Update actions may run gh pr update-branch --rebase.`
)

func Parse(args []string) (Config, error) {
	var parsed Config
	cmd := newCommand(strings.NewReader(""), io.Discard, io.Discard, func(_ context.Context, cfg Config, _ io.Reader, _, _ io.Writer) error {
		parsed = cfg
		return nil
	})
	cmd.SetArgs(args)
	return parsed, cmd.ExecuteContext(context.Background())
}

func newCommand(stdin io.Reader, stdout, stderr io.Writer, handler commandHandler) *cobra.Command {
	cfg := defaultConfig()
	jsonFlag := false

	run := func(command Command, validate func(Config) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, _ []string) error {
			cfg.Command = command
			if jsonFlag {
				cfg.Format = output.FormatJSON
			}
			if err := validateFormat(cfg.Format); err != nil {
				return err
			}
			if validate != nil {
				if err := validate(cfg); err != nil {
					return err
				}
			}
			return handler(cmd.Context(), cfg, stdin, stdout, stderr)
		}
	}

	validateTUI := func(cfg Config) error {
		if cfg.Parallel < 1 {
			return errParallelMustBePositive()
		}
		return nil
	}

	root := &cobra.Command{
		Use:           "domino",
		Short:         rootShort,
		Long:          rootLong,
		Example:       rootExample,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          run(CommandTUI, validateTUI),
		Annotations: map[string]string{
			cobra.CommandDisplayNameAnnotation: "gh domino",
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	addGlobalFlags(root, &cfg, &jsonFlag)
	addTUIFlags(root, &cfg)

	tuiCmd := &cobra.Command{
		Use:     "tui",
		Short:   "Inspect and repair stacks in an interactive terminal UI",
		Long:    tuiLong,
		Example: "  gh domino tui\n  gh domino tui --parallel 2",
		Args:    cobra.NoArgs,
		RunE:    run(CommandTUI, validateTUI),
	}
	addTUIFlags(tuiCmd, &cfg)

	listState := "all"
	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "Show stacked PRs and their repair status",
		Long:    listLong,
		Example: "  gh domino list\n  gh domino list --state broken\n  gh domino list --state updateable --flat",
		Args:    cobra.NoArgs,
		RunE: run(CommandList, func(cfg Config) error {
			return validateState(cfg.State)
		}),
	}
	listCmd.PreRun = func(*cobra.Command, []string) {
		cfg.State = listState
	}
	listCmd.Flags().StringVar(&listState, "state", "all", "Filter state: all, broken, clean, updateable")
	listCmd.Flags().BoolVar(&cfg.Flat, "flat", false, "Print flat rows instead of a tree")

	planCmd := &cobra.Command{
		Use:     "plan",
		Short:   "Preview repair actions without changing branches",
		Long:    planLong,
		Example: "  gh domino plan\n  gh domino plan --chain 52\n  gh domino plan --subtree 52 --json",
		Args:    cobra.NoArgs,
		RunE:    run(CommandPlan, nil),
	}
	planCmd.Flags().BoolVar(&cfg.IncludeClean, "include-clean", false, "Include update-branch actions for updateable PRs")
	addSelectionFlags(planCmd, &cfg)

	mergeCmd := &cobra.Command{
		Use:     "merge",
		Short:   "Execute selected repair actions",
		Long:    mergeLong,
		Example: "  gh domino merge\n  gh domino merge --dry-run --chain 52\n  gh domino merge --yes --chain 52 --parallel 4",
		Args:    cobra.NoArgs,
		RunE: run(CommandMerge, func(cfg Config) error {
			if cfg.Parallel < 1 {
				return errParallelMustBePositive()
			}
			return nil
		}),
	}
	mergeCmd.Flags().BoolVar(&cfg.Yes, "yes", false, "Run without interactive confirmation")
	mergeCmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "Print selected action plan without mutating")
	mergeCmd.Flags().BoolVar(&cfg.IncludeClean, "include-clean", false, "Include update-branch actions for updateable PRs")
	mergeCmd.Flags().IntVar(&cfg.Parallel, "parallel", 1, "Max independent stacks to process in parallel when dependencies allow")
	addSelectionFlags(mergeCmd, &cfg)

	root.AddCommand(tuiCmd, listCmd, planCmd, mergeCmd)
	return root
}

func defaultConfig() Config {
	return Config{
		Command:     CommandTUI,
		Remote:      "origin",
		Author:      "@me",
		MergedLimit: 30,
		Format:      output.FormatHuman,
		Parallel:    1,
	}
}

func addGlobalFlags(cmd *cobra.Command, cfg *Config, jsonFlag *bool) {
	flags := cmd.PersistentFlags()
	flags.StringVar(&cfg.Remote, "remote", cfg.Remote, "Git remote to fetch and inspect")
	flags.StringVar(&cfg.Author, "author", cfg.Author, "PR author filter")
	flags.IntVar(&cfg.MergedLimit, "merged-limit", cfg.MergedLimit, "Number of recently merged PRs to inspect")
	flags.Var((*formatValue)(&cfg.Format), "format", "Output format: human, json")
	flags.BoolVar(jsonFlag, "json", false, "Output JSON")
	flags.BoolVar(&cfg.NoColor, "no-color", cfg.NoColor, "Disable ANSI color")
	flags.BoolVarP(&cfg.Verbose, "verbose", "v", cfg.Verbose, "Print command/progress details")
}

func addSelectionFlags(cmd *cobra.Command, cfg *Config) {
	cmd.Flags().Var((*intListValue)(&cfg.PRNumbers), "pr", "Select only actions targeting this PR; repeatable")
	cmd.Flags().Var((*intListValue)(&cfg.SubtreeNums), "subtree", "Select actions for this PR and descendant PRs; repeatable")
	cmd.Flags().Var((*intListValue)(&cfg.ChainNumbers), "chain", "Select the dependency chain ending at this PR, ordered parent before child; repeatable")
}

func addTUIFlags(cmd *cobra.Command, cfg *Config) {
	cmd.Flags().BoolVar(&cfg.IncludeClean, "include-clean", false, "Start with update-branch actions included for updateable PRs")
	cmd.Flags().IntVar(&cfg.Parallel, "parallel", cfg.Parallel, "Initial max independent stacks to process in parallel when dependencies allow")
}
