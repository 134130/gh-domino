package git

import (
	"fmt"
)

type NotInstalledError struct {
	message string
	err     error
}

func (e *NotInstalledError) Error() string {
	return e.message
}

func (e *NotInstalledError) Unwrap() error {
	return e.err
}

type GitError struct {
	ExitCode int
	Stderr   string
	err      error
}

func (ge *GitError) Error() string {
	if ge.Stderr == "" {
		return fmt.Sprintf("failed to run git: %v", ge.err)
	}
	return fmt.Sprintf("failed to run git: %v", ge.Stderr)
}

func (ge *GitError) Unwrap() error {
	return ge.err
}

type GHError struct {
	ExitCode int
	Stderr   string
	err      error
}

func (ge *GHError) Error() string {
	if ge.Stderr == "" {
		return fmt.Sprintf("failed to run gh: %v", ge.err)
	}
	return fmt.Sprintf("failed to run gh: %v", ge.Stderr)
}

func (ge *GHError) Unwrap() error {
	return ge.err
}
