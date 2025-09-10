package git

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/goccy/go-yaml"
)

type commandLog struct {
	Command  string
	Stdout   string
	Stderr   string
	ExitCode int `yaml:"exitCode"`
}

type LoggingRunner struct {
	r   Runner
	out string
}

func NewLoggingRunner(out string) (*LoggingRunner, error) {
	if out == "" {
		return nil, fmt.Errorf("log file path must be provided")
	}

	return &LoggingRunner{
		r:   &DefaultRunner{},
		out: out,
	}, nil
}

var _ Runner = (*LoggingRunner)(nil)

func (r *LoggingRunner) Run(ctx context.Context, cmd string, args []string, mods ...CommandModifier) error {
	f, err := os.OpenFile(r.out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	w := yaml.NewEncoder(f)
	defer w.Close() //nolint:errcheck

	log := &commandLog{
		Command: cmd + " " + strings.Join(args, " "),
	}

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)

	mod := func(c *exec.Cmd) {
		if c.Stdout != nil {
			c.Stdout = io.MultiWriter(c.Stdout, stdout)
		} else {
			c.Stdout = stdout
		}
		if c.Stderr != nil {
			c.Stderr = io.MultiWriter(c.Stderr, stderr)
		} else {
			c.Stderr = stderr
		}
	}

	err = r.r.Run(ctx, cmd, args, append(mods, mod)...)

	log.Stdout = stdout.String()
	log.Stderr = stderr.String()

	var exitCode int
	if err != nil {
		switch gErr := err.(type) {
		case *GitError:
			exitCode = gErr.ExitCode
		case *GHError:
			exitCode = gErr.ExitCode
		default:
			exitCode = 1
		}
	} else {
		exitCode = 0
	}

	if exitCode != 0 {
		log.ExitCode = exitCode
	} else {
		if stdout.Len() == 0 && stderr.Len() == 0 {
			log.ExitCode = exitCode
		}
	}

	return err
}
