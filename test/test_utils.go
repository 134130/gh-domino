package test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/goccy/go-yaml"

	"github.com/134130/gh-domino/git"
)

func ptr[T any](v T) *T {
	return &v
}

type YAMLCommand struct {
	Command  string
	Stdout   string
	Stderr   string
	ExitCode int `yaml:"exitCode"`
}

type YAMLRunner struct {
	Commands []YAMLCommand
	used     map[int]struct{}
}

func NewYAMLRunner(ctx context.Context, filename string) (*YAMLRunner, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	file, err := os.Open(filepath.Join(dir, "../", filename))
	if err != nil {
		return nil, fmt.Errorf("failed to open test data file: %w", err)
	}

	var runners []YAMLCommand
	if err := yaml.NewDecoder(file).DecodeContext(ctx, &runners); err != nil {
		return nil, fmt.Errorf("failed to decode YAML commands: %w", err)
	}

	return &YAMLRunner{
		Commands: runners,
		used:     make(map[int]struct{}),
	}, nil
}

var _ git.Runner = (*YAMLRunner)(nil)

func (r *YAMLRunner) Run(ctx context.Context, cmd string, args []string, mods ...git.CommandModifier) error {
	execCmd := &exec.Cmd{}
	for _, mod := range mods {
		mod(execCmd)
	}

	var found *YAMLCommand
	for i, c := range r.Commands {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, ok := r.used[i]; ok {
			continue
		}
		if c.Command == fmt.Sprintf("%s %s", cmd, strings.Join(args, " ")) {
			r.used[i] = struct{}{}
			found = &c
			break
		}
	}

	if found == nil {
		panic(fmt.Sprintf("could not find command '%s %s'", cmd, strings.Join(args, " ")))
	}

	if execCmd.Stdout != nil {
		_, err := io.Copy(execCmd.Stdout, strings.NewReader(found.Stdout))
		if err != nil {
			panic(fmt.Sprintf("failed to write stdout: %v", err))
		}
	}
	if execCmd.Stderr != nil {
		_, err := io.Copy(execCmd.Stderr, strings.NewReader(found.Stderr))
		if err != nil {
			panic(fmt.Sprintf("failed to write stderr: %v", err))
		}
	}

	if found.ExitCode != 0 {
		switch cmd {
		case "git":
			return &git.GitError{
				ExitCode: found.ExitCode,
				Stderr:   "",
			}
		case "gh":
			return &git.GHError{
				ExitCode: found.ExitCode,
				Stderr:   "",
			}
		}
	}

	return nil
}

type LoggingRunner struct {
	r git.Runner
}

func NewLoggingRunner() *LoggingRunner {
	return &LoggingRunner{r: &git.DefaultRunner{}}
}

var _ git.Runner = (*LoggingRunner)(nil)

func (r *LoggingRunner) Run(ctx context.Context, cmd string, args []string, mods ...git.CommandModifier) error {
	f, err := os.OpenFile("/Users/cooper/development/gh-domino/test_runner.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close() //nolint:errcheck

	_, _ = fmt.Fprintf(f, "- command: %s %s\n", cmd, strings.Join(args, " "))

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

	style := lipgloss.NewStyle().PaddingLeft(6)
	writeYamlStr := func(label, content string) {
		if len(content) == 0 {
			return
		}
		content = strings.TrimSuffix(content, "\n")
		if strings.ContainsRune(content, '\n') {
			_, _ = fmt.Fprintf(f, "  %s: |\n", label)
			_, _ = fmt.Fprint(f, style.Render(content))
			_, _ = fmt.Fprint(f, "\n")
		} else {
			_, _ = fmt.Fprintf(f, "  %s: %s\n", label, strings.TrimSpace(content))
		}
	}

	writeYamlStr("stdout", stdout.String())
	writeYamlStr("stderr", stderr.String())

	var exitCode int
	if err != nil {
		switch gErr := err.(type) {
		case *git.GitError:
			exitCode = gErr.ExitCode
		case *git.GHError:
			exitCode = gErr.ExitCode
		default:
			exitCode = 1
		}
	} else {
		exitCode = 0
	}

	if exitCode != 0 {
		_, _ = fmt.Fprintf(f, "  exitCode: %d\n", exitCode)
	} else {
		if stdout.Len() == 0 && stderr.Len() == 0 {
			_, _ = fmt.Fprintf(f, "  exitCode: %d\n", exitCode)
		}
	}

	return err
}
