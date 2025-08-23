package test

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
