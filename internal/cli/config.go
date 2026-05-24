package cli

import (
	"fmt"
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

	Yes      bool
	DryRun   bool
	Parallel int
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

func errParallelMustBePositive() error {
	return fmt.Errorf("--parallel must be greater than 0")
}

type formatValue output.Format

func (v *formatValue) String() string {
	if v == nil {
		return ""
	}
	return string(*v)
}

func (v *formatValue) Type() string {
	return "format"
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

func (v *intListValue) Type() string {
	return "int"
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
