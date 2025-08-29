package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/cli/safeexec"
)

// Runner defines an interface for running commands, allowing for mocking in tests.
type Runner interface {
	Run(ctx context.Context, cmd string, args []string, mods ...CommandModifier) error
}

// DefaultRunner is the default implementation of Runner that executes actual commands.
type DefaultRunner struct{}

// CommandRunner is a global instance of a Runner, which can be replaced by a mock in tests.
var CommandRunner Runner = &DefaultRunner{}

func (r *DefaultRunner) Run(ctx context.Context, cmdName string, args []string, mods ...CommandModifier) error {
	exe, err := path(cmdName)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return &NotInstalledError{
				message: fmt.Sprintf("unable to find %s executable in PATH; please install %s before retrying", cmdName, cmdName),
				err:     err,
			}
		}
		return err
	}

	cmd := exec.CommandContext(ctx, exe, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	for _, mod := range mods {
		mod(cmd)
	}

	err = cmd.Run()
	if err != nil {
		switch cmdName {
		case "git":
			ge := GitError{err: err}
			var exitError *exec.ExitError
			if errors.As(err, &exitError) {
				ge.Stderr = stderr.String()
				ge.ExitCode = exitError.ExitCode()
			}
			return &ge

		case "gh":
			ge := GHError{err: err}
			var exitError *exec.ExitError
			if errors.As(err, &exitError) {
				ge.Stderr = stderr.String()
				ge.ExitCode = exitError.ExitCode()
			}
			return &ge
		default:
			panic(fmt.Sprintf("unsupported command: %s", cmdName))
		}
	}

	return nil
}

type Command struct {
	cmd  string
	args []string
}

func NewCommand(cmd string, args ...string) *Command {
	return &Command{
		cmd:  cmd,
		args: args,
	}
}

func (c *Command) Run(ctx context.Context, mods ...CommandModifier) error {
	return CommandRunner.Run(ctx, c.cmd, c.args, mods...)
}

type CommandModifier func(c *exec.Cmd)

func WithStdout(stdout io.Writer) CommandModifier {
	return func(c *exec.Cmd) {
		if stdout == nil {
			return
		}
		if c.Stdout == nil {
			c.Stdout = stdout
		} else {
			c.Stdout = io.MultiWriter(c.Stdout, stdout)
		}
	}
}

func WithStderr(stderr io.Writer) CommandModifier {
	return func(c *exec.Cmd) {
		if stderr == nil {
			return
		}
		if c.Stderr == nil {
			c.Stderr = stderr
		} else {
			c.Stderr = io.MultiWriter(c.Stderr, stderr)
		}
	}
}

func WithCombinedOutput(output io.Writer) CommandModifier {
	return func(c *exec.Cmd) {
		if output == nil {
			return
		}
		WithStdout(output)(c)
		WithStderr(output)(c)
	}
}

func WithStdin(stdin io.Reader) CommandModifier {
	return func(c *exec.Cmd) {
		c.Stdin = stdin
	}
}

func path(cmd string) (string, error) {
	switch cmd {
	case "git":
		return safeexec.LookPath("git")
	case "gh":
		if ghExe := os.Getenv("GH_PATH"); ghExe != "" {
			return ghExe, nil
		}
		return safeexec.LookPath("gh")
	}

	return "", fmt.Errorf("unsupported command: %s", cmd)
}
