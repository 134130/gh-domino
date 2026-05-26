package cli

import (
	"context"
	"io"
	"strings"

	"github.com/134130/gh-domino/internal/output"
	"github.com/spf13/cobra"
)

type commandHandler func(context.Context, Config, io.Reader, io.Writer, io.Writer) error

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
		Use:           "gh domino",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE:          run(CommandTUI, validateTUI),
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	addGlobalFlags(root, &cfg, &jsonFlag)
	addTUIFlags(root, &cfg)

	tuiCmd := &cobra.Command{
		Use:  "tui",
		Args: cobra.NoArgs,
		RunE: run(CommandTUI, validateTUI),
	}
	addTUIFlags(tuiCmd, &cfg)

	listState := "all"
	listCmd := &cobra.Command{
		Use:  "list",
		Args: cobra.NoArgs,
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
		Use:  "plan",
		Args: cobra.NoArgs,
		RunE: run(CommandPlan, nil),
	}
	planCmd.Flags().BoolVar(&cfg.IncludeClean, "include-clean", false, "Include update-branch actions for clean PRs")
	addSelectionFlags(planCmd, &cfg, "Plan")

	mergeCmd := &cobra.Command{
		Use:  "merge",
		Args: cobra.NoArgs,
		RunE: run(CommandMerge, func(cfg Config) error {
			if cfg.Parallel < 1 {
				return errParallelMustBePositive()
			}
			return nil
		}),
	}
	mergeCmd.Flags().BoolVar(&cfg.Yes, "yes", false, "Run without interactive confirmation")
	mergeCmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "Print action plan without mutating")
	mergeCmd.Flags().BoolVar(&cfg.IncludeClean, "include-clean", false, "Include update-branch actions for clean PRs")
	mergeCmd.Flags().IntVar(&cfg.Parallel, "parallel", 1, "Max independent stacks to process in parallel")
	addSelectionFlags(mergeCmd, &cfg, "Execute")

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
	flags.StringVar(&cfg.Repo, "repo", cfg.Repo, "GitHub repository override")
	flags.StringVar(&cfg.Remote, "remote", cfg.Remote, "Git remote to fetch and inspect")
	flags.StringVar(&cfg.Author, "author", cfg.Author, "PR author filter")
	flags.IntVar(&cfg.MergedLimit, "merged-limit", cfg.MergedLimit, "Recently merged PR lookup limit")
	flags.Var((*formatValue)(&cfg.Format), "format", "Output format: human, json")
	flags.BoolVar(jsonFlag, "json", false, "Output JSON")
	flags.BoolVar(&cfg.NoColor, "no-color", cfg.NoColor, "Disable ANSI color")
	flags.BoolVarP(&cfg.Verbose, "verbose", "v", cfg.Verbose, "Print command/progress details")
}

func addSelectionFlags(cmd *cobra.Command, cfg *Config, verb string) {
	cmd.Flags().Var((*intListValue)(&cfg.PRNumbers), "pr", verb+" actions targeting this PR; repeatable")
	cmd.Flags().Var((*intListValue)(&cfg.SubtreeNums), "subtree", verb+" actions for this PR and descendants; repeatable")
	cmd.Flags().Var((*intListValue)(&cfg.ChainNumbers), "chain", verb+" actions from the root PR to this PR; repeatable")
}

func addTUIFlags(cmd *cobra.Command, cfg *Config) {
	cmd.Flags().BoolVar(&cfg.IncludeClean, "include-clean", false, "Start with clean PR update actions included")
	cmd.Flags().IntVar(&cfg.Parallel, "parallel", cfg.Parallel, "Initial max independent stacks to process in parallel")
}
