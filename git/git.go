package git

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/134130/gh-domino/gitobj"
)

func ListPullRequests(ctx context.Context) ([]gitobj.PullRequest, error) {
	stdout := &bytes.Buffer{}
	fields := []string{
		"number", "title", "url", "author", "state", "isDraft",
		"mergeCommit", "baseRefName", "headRefName", "headRepository", "commits",
	}
	listArgs := []string{
		"pr", "list", "--author", "@me", "--json", strings.Join(fields, ","),
	}
	if err := NewCommand("gh", listArgs...).Run(ctx, WithStdout(stdout)); err != nil {
		return nil, err
	}

	var prs []gitobj.PullRequest
	if err := json.NewDecoder(stdout).Decode(&prs); err != nil {
		return nil, fmt.Errorf("failed to decode pr list: %w", err)
	}

	sort.Slice(prs, func(i, j int) bool {
		return prs[i].Number < prs[j].Number
	})

	return prs, nil
}

func GetDefaultBranch(ctx context.Context) (string, error) {
	stdout := &bytes.Buffer{}
	args := []string{"remote", "show", "origin"}
	if err := NewCommand("git", args...).Run(ctx, WithStdout(stdout)); err != nil {
		return "", err
	}

	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.Contains(line, "HEAD branch") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}

	return "", fmt.Errorf("could not determine default branch")
}

func GetBranchCommits(ctx context.Context, base, head string) ([]string, error) {
	stdout := &bytes.Buffer{}
	args := []string{"log", "--pretty=%H", fmt.Sprintf("%s..%s", base, head)}
	if err := NewCommand("git", args...).Run(ctx, WithStdout(stdout)); err != nil {
		return nil, err
	}

	var commits []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		if trimmedLine := strings.TrimSpace(line); trimmedLine != "" {
			commits = append(commits, trimmedLine)
		}
	}

	return commits, nil
}

func Rebase(ctx context.Context, onto, branch string) error {
	return NewCommand("git", "rebase", onto, branch).Run(ctx)
}

func Push(ctx context.Context, branch string) error {
	return NewCommand("git", "push", "--force-with-lease", "origin", branch).Run(ctx)
}

func Fetch(ctx context.Context, remote string) error {
	args := []string{"fetch", remote}
	return NewCommand("git", args...).Run(ctx)
}

func IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	args := []string{"merge-base", "--is-ancestor", ancestor, descendant}
	err := NewCommand("git", args...).Run(ctx)
	if err == nil {
		return true, nil
	}
	var gitErr *GitError
	if errors.As(err, &gitErr) && gitErr.ExitCode == 1 {
		return false, nil
	}
	return false, err
}

func ListMergedPullRequests(ctx context.Context) ([]gitobj.PullRequest, error) {
	stdout := &bytes.Buffer{}
	fields := []string{
		"number", "title", "url", "author", "state", "isDraft",
		"mergeCommit", "baseRefName", "headRefName", "headRepository", "commits",
	}
	listArgs := []string{
		"pr", "list", "--author", "@me", "--state", "merged", "--limit", "30",
		"--json", strings.Join(fields, ","),
	}
	if err := NewCommand("gh", listArgs...).Run(ctx, WithStdout(stdout)); err != nil {
		return nil, err
	}

	var prs []gitobj.PullRequest
	if err := json.NewDecoder(stdout).Decode(&prs); err != nil {
		return nil, fmt.Errorf("failed to decode merged pr list: %w", err)
	}

	sort.Slice(prs, func(i, j int) bool {
		return prs[i].Number < prs[j].Number
	})

	return prs, nil
}

func GetMergeBase(ctx context.Context, branch1, branch2 string) (string, error) {
	stdout := &bytes.Buffer{}
	args := []string{"merge-base", branch1, branch2}
	if err := NewCommand("git", args...).Run(ctx, WithStdout(stdout)); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func UpdateBaseBranch(ctx context.Context, prNumber int, newBase string) error {
	args := []string{"pr", "edit", fmt.Sprintf("%d", prNumber), "--base", newBase}
	return NewCommand("gh", args...).Run(ctx)
}

func RevParse(ctx context.Context, ref string) (string, error) {
	stdout := &bytes.Buffer{}
	args := []string{"rev-parse", ref}
	if err := NewCommand("git", args...).Run(ctx, WithStdout(stdout)); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}
