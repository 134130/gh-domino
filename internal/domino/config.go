package domino

import (
	"flag"
	"fmt"
	"io"
	"os"
)

type Config struct {
	Auto   *bool
	DryRun *bool
	Writer io.Writer
}

func ParseConfig() (Config, error) {
	c := Config{
		Auto:   flag.Bool("auto", false, "Enable auto mode to rebase without confirmation"),
		DryRun: flag.Bool("dry-run", false, "Don't rebase the changes"),
		Writer: os.Stdout,
	}
	flag.Parse()

	if *c.Auto && *c.DryRun {
		return c, fmt.Errorf("cannot use --auto and --dry-run together")
	}

	return c, nil
}
