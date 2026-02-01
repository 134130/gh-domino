package domino

import (
	"flag"
	"fmt"
	"io"
	"os"
)

type Config struct {
	Auto         bool
	DryRun       bool
	Headless     bool
	DumpTo       string
	Writer       io.Writer
	WorktreePath string
}

func ParseConfig() (Config, error) {
	c := Config{
		Writer: os.Stdout,
	}

	flag.BoolVar(&c.Auto, "auto", false, "Enable auto mode to rebase with confirmation")
	flag.BoolVar(&c.DryRun, "dry-run", false, "Don't rebase the changes")
	flag.StringVar(&c.DumpTo, "dump-to", "", "Dump git commands to a file for testing purposes")
	flag.BoolVar(&c.Headless, "headless", false, "Disable UI")
	flag.StringVar(&c.WorktreePath, "worktree", "", "Path to the temporary worktree")
	flag.StringVar(&c.WorktreePath, "w", "", "Path to the temporary worktree (shorthand)")

	flag.Parse()

	if c.Auto && c.DryRun {
		return c, fmt.Errorf("cannot use --auto and --dry-run together")
	}

	return c, nil
}
