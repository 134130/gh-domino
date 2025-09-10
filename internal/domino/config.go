package domino

import (
	"flag"
	"fmt"
	"io"
	"os"
)

type Config struct {
	Auto   bool
	DryRun bool
	DumpTo string
	Writer io.Writer
}

func ParseConfig() (Config, error) {
	c := Config{
		Writer: os.Stdout,
	}

	flag.BoolVar(&c.Auto, "auto", false, "Enable auto mode to rebase with confirmation")
	flag.BoolVar(&c.DryRun, "dry-run", false, "Don't rebase the changes")
	flag.StringVar(&c.DumpTo, "dump-to", "", "Dump git commands to a file for testing purposes")

	flag.Parse()

	if c.Auto && c.DryRun {
		return c, fmt.Errorf("cannot use --auto and --dry-run together")
	}

	return c, nil
}
