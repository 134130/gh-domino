package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/output"
)

type Command string

const (
	CommandLegacy Command = "legacy"
	CommandTUI    Command = "tui"
	CommandList   Command = "list"
	CommandPlan   Command = "plan"
	CommandMerge  Command = "merge"
)

type Config struct {
	Command Command

	Repo        string
	Remote      string
	Author      string
	MergedLimit int
	Format      output.Format
	NoColor     bool
	Verbose     bool
	DumpTo      string

	State        string
	Flat         bool
	IncludeClean bool
	PRNumber     int
	StackNumber  int

	Yes         bool
	DryRun      bool
	Headless    bool
	Parallel    int
	WorktreeDir string
	RebaseAll   bool
}

func Parse(args []string) (Config, error) {
	globalArgs, command, commandArgs := splitCommand(args)
	if command == "" {
		return parseLegacy(args)
	}

	cfg := Config{
		Command:     command,
		Remote:      "origin",
		Author:      "@me",
		MergedLimit: 30,
		Format:      output.FormatHuman,
		Parallel:    1,
	}
	if err := parseGlobal(globalArgs, &cfg); err != nil {
		return cfg, err
	}

	switch command {
	case CommandTUI:
		return parseTUI(commandArgs, cfg)
	case CommandList:
		return parseList(commandArgs, cfg)
	case CommandPlan:
		return parsePlan(commandArgs, cfg)
	case CommandMerge:
		return parseMerge(commandArgs, cfg)
	default:
		return cfg, fmt.Errorf("unknown command: %s", command)
	}
}

func (c Config) PlanOptions() app.PlanOptions {
	return app.PlanOptions{
		Remote:       c.Remote,
		Author:       c.Author,
		MergedLimit:  c.MergedLimit,
		IncludeClean: c.IncludeClean,
	}
}

func parseLegacy(args []string) (Config, error) {
	cfg := Config{
		Command:     CommandLegacy,
		Remote:      "origin",
		Author:      "@me",
		MergedLimit: 30,
		Format:      output.FormatHuman,
	}

	fs := flag.NewFlagSet("gh domino", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&cfg.Yes, "auto", false, "Enable auto mode")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "Don't rebase the changes")
	fs.StringVar(&cfg.DumpTo, "dump-to", "", "Dump git commands to a file")
	fs.BoolVar(&cfg.Headless, "headless", false, "Disable UI")
	fs.BoolVar(&cfg.RebaseAll, "rebase-all", false, "Rebase all open PRs")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if cfg.Yes && cfg.DryRun {
		return cfg, fmt.Errorf("cannot use --auto and --dry-run together")
	}
	return cfg, nil
}

func parseGlobal(args []string, cfg *Config) error {
	fs := flag.NewFlagSet("gh domino", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	addGlobalFlags(fs, cfg)
	jsonFlag := fs.Bool("json", false, "Output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *jsonFlag {
		cfg.Format = output.FormatJSON
	}
	return validateFormat(cfg.Format)
}

func parseTUI(args []string, cfg Config) (Config, error) {
	fs := flag.NewFlagSet("gh domino tui", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	addGlobalFlags(fs, &cfg)
	fs.BoolVar(&cfg.IncludeClean, "include-clean", false, "Allow update-branch actions for clean PRs")
	jsonFlag := fs.Bool("json", false, "Output JSON")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if *jsonFlag {
		cfg.Format = output.FormatJSON
	}
	return cfg, validateFormat(cfg.Format)
}

func parseList(args []string, cfg Config) (Config, error) {
	fs := flag.NewFlagSet("gh domino list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	addGlobalFlags(fs, &cfg)
	fs.StringVar(&cfg.State, "state", "all", "Filter state: all, broken, clean, updateable")
	fs.BoolVar(&cfg.Flat, "flat", false, "Print flat rows instead of a tree")
	fs.IntVar(&cfg.StackNumber, "stack", 0, "Show only the stack containing this PR")
	jsonFlag := fs.Bool("json", false, "Output JSON")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if *jsonFlag {
		cfg.Format = output.FormatJSON
	}
	if err := validateFormat(cfg.Format); err != nil {
		return cfg, err
	}
	return cfg, validateState(cfg.State)
}

func parsePlan(args []string, cfg Config) (Config, error) {
	fs := flag.NewFlagSet("gh domino plan", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	addGlobalFlags(fs, &cfg)
	fs.BoolVar(&cfg.IncludeClean, "include-clean", false, "Include update-branch actions for clean PRs")
	fs.IntVar(&cfg.PRNumber, "pr", 0, "Plan only actions for this PR")
	fs.IntVar(&cfg.StackNumber, "stack", 0, "Plan actions for the stack containing this PR")
	jsonFlag := fs.Bool("json", false, "Output JSON")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if *jsonFlag {
		cfg.Format = output.FormatJSON
	}
	return cfg, validateFormat(cfg.Format)
}

func parseMerge(args []string, cfg Config) (Config, error) {
	fs := flag.NewFlagSet("gh domino merge", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	addGlobalFlags(fs, &cfg)
	fs.BoolVar(&cfg.Yes, "yes", false, "Run without interactive confirmation")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "Print action plan without mutating")
	fs.BoolVar(&cfg.IncludeClean, "include-clean", false, "Include update-branch actions for clean PRs")
	fs.IntVar(&cfg.PRNumber, "pr", 0, "Execute only actions for this PR")
	fs.IntVar(&cfg.StackNumber, "stack", 0, "Execute actions for the stack containing this PR")
	fs.IntVar(&cfg.Parallel, "parallel", 1, "Max independent stacks to process in parallel")
	fs.StringVar(&cfg.WorktreeDir, "worktree-dir", "", "Temporary worktree base directory")
	jsonFlag := fs.Bool("json", false, "Output JSON")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if *jsonFlag {
		cfg.Format = output.FormatJSON
	}
	if err := validateFormat(cfg.Format); err != nil {
		return cfg, err
	}
	if cfg.Parallel < 1 {
		return cfg, fmt.Errorf("--parallel must be greater than 0")
	}
	return cfg, nil
}

func addGlobalFlags(fs *flag.FlagSet, cfg *Config) {
	fs.StringVar(&cfg.Repo, "repo", cfg.Repo, "GitHub repository override")
	fs.StringVar(&cfg.Remote, "remote", cfg.Remote, "Git remote to fetch and inspect")
	fs.StringVar(&cfg.Author, "author", cfg.Author, "PR author filter")
	fs.IntVar(&cfg.MergedLimit, "merged-limit", cfg.MergedLimit, "Recently merged PR lookup limit")
	fs.Var((*formatValue)(&cfg.Format), "format", "Output format: human, json")
	fs.BoolVar(&cfg.NoColor, "no-color", cfg.NoColor, "Disable ANSI color")
	fs.BoolVar(&cfg.Verbose, "verbose", cfg.Verbose, "Print command/progress details")
	fs.StringVar(&cfg.DumpTo, "dump-to", cfg.DumpTo, "Dump git commands to a file")
}

func splitCommand(args []string) ([]string, Command, []string) {
	for i, arg := range args {
		switch Command(arg) {
		case CommandTUI, CommandList, CommandPlan, CommandMerge:
			return args[:i], Command(arg), args[i+1:]
		}
	}
	return nil, "", args
}

func validateFormat(format output.Format) error {
	switch format {
	case output.FormatHuman, output.FormatJSON:
		return nil
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func validateState(state string) error {
	switch state {
	case "", "all", "broken", "clean", "updateable":
		return nil
	default:
		return fmt.Errorf("unsupported state: %s", state)
	}
}

type formatValue output.Format

func (v *formatValue) String() string {
	if v == nil {
		return ""
	}
	return string(*v)
}

func (v *formatValue) Set(value string) error {
	switch output.Format(value) {
	case output.FormatHuman, output.FormatJSON:
		*v = formatValue(value)
		return nil
	default:
		return fmt.Errorf("unsupported format: %s", value)
	}
}
