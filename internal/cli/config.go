package cli

import (
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/output"
)

type Command string

const (
	CommandTUI   Command = "tui"
	CommandList  Command = "list"
	CommandPlan  Command = "plan"
	CommandMerge Command = "merge"
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

	State        string
	Flat         bool
	IncludeClean bool
	PRNumbers    []int
	SubtreeNums  []int
	ChainNumbers []int

	Yes         bool
	DryRun      bool
	Parallel    int
	WorktreeDir string
}

func Parse(args []string) (Config, error) {
	globalArgs, command, commandArgs := splitCommand(args)
	if command == "" {
		command = CommandTUI
		globalArgs = args
		commandArgs = nil
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

func (c Config) Selection() app.Selection {
	items := make([]app.SelectionItem, 0, len(c.PRNumbers)+len(c.SubtreeNums)+len(c.ChainNumbers))
	for _, number := range c.PRNumbers {
		items = append(items, app.SelectionItem{PRNumber: number, Mode: app.SelectNode})
	}
	for _, number := range c.SubtreeNums {
		items = append(items, app.SelectionItem{PRNumber: number, Mode: app.SelectSubtree})
	}
	for _, number := range c.ChainNumbers {
		items = append(items, app.SelectionItem{PRNumber: number, Mode: app.SelectChain})
	}
	return app.Selection{Items: items}
}

func (c Config) HasSelection() bool {
	return len(c.PRNumbers) > 0 || len(c.SubtreeNums) > 0 || len(c.ChainNumbers) > 0
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
	fs.Var((*intListValue)(&cfg.PRNumbers), "pr", "Plan actions targeting this PR; repeatable")
	fs.Var((*intListValue)(&cfg.SubtreeNums), "subtree", "Plan actions for this PR and descendants; repeatable")
	fs.Var((*intListValue)(&cfg.ChainNumbers), "chain", "Plan actions from the root PR to this PR; repeatable")
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
	fs.Var((*intListValue)(&cfg.PRNumbers), "pr", "Execute actions targeting this PR; repeatable")
	fs.Var((*intListValue)(&cfg.SubtreeNums), "subtree", "Execute actions for this PR and descendants; repeatable")
	fs.Var((*intListValue)(&cfg.ChainNumbers), "chain", "Execute actions from the root PR to this PR; repeatable")
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

type intListValue []int

func (v *intListValue) String() string {
	if v == nil {
		return ""
	}
	return fmt.Sprint([]int(*v))
}

func (v *intListValue) Set(value string) error {
	number, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("invalid number %q: %w", value, err)
	}
	if number < 1 {
		return fmt.Errorf("number must be greater than 0: %d", number)
	}
	*v = append(*v, number)
	return nil
}
