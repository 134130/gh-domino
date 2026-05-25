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

func GetGitURL(ctx context.Context, mods ...CommandModifier) (string, error) {
	stdout := &bytes.Buffer{}
	args := []string{"remote", "get-url", "origin"}
	mods = append(mods, WithStdout(stdout))
	if err := NewCommand("git", args...).Run(ctx, mods...); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func Clone(ctx context.Context, repoURL, targetDir string, mods ...CommandModifier) error {
	args := []string{"clone", repoURL, targetDir}
	return NewCommand("git", args...).Run(ctx, mods...)
}

func ListPullRequests(ctx context.Context, mods ...CommandModifier) ([]gitobj.PullRequest, error) {
	stdout := &bytes.Buffer{}
	fields := []string{
		"number", "title", "url", "author", "state", "isDraft",
		"mergeCommit", "baseRefName", "headRefName", "headRepository", "commits",
	}
	listArgs := []string{
		"pr", "list", "--author", "@me", "--json", strings.Join(fields, ","),
	}
	mods = append(mods, WithStdout(stdout))
	if err := NewCommand("gh", listArgs...).Run(ctx, mods...); err != nil {
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

var defaultBranchCache string

func GetDefaultBranch(ctx context.Context, mods ...CommandModifier) (string, error) {
	if defaultBranchCache != "" {
		return defaultBranchCache, nil
	}

	branch, err := getDefaultBranchFromRemoteHead(ctx, "origin", mods...)
	if err == nil {
		defaultBranchCache = branch
		return defaultBranchCache, nil
	}

	branch, err = getDefaultBranchFromRemoteShow(ctx, "origin", mods...)
	if err != nil {
		return "", err
	}
	defaultBranchCache = branch
	return defaultBranchCache, nil
}

func getDefaultBranchFromRemoteHead(ctx context.Context, remote string, mods ...CommandModifier) (string, error) {
	stdout := &bytes.Buffer{}
	args := []string{"symbolic-ref", "--quiet", "--short", "refs/remotes/" + remote + "/HEAD"}
	mods = append(append([]CommandModifier{}, mods...), WithStdout(stdout))
	if err := NewCommand("git", args...).Run(ctx, mods...); err != nil {
		return "", err
	}

	if branch, ok := defaultBranchFromSymbolicRef(remote, stdout.String()); ok {
		return branch, nil
	}
	return "", fmt.Errorf("could not determine default branch from refs/remotes/%s/HEAD", remote)
}

func getDefaultBranchFromRemoteShow(ctx context.Context, remote string, mods ...CommandModifier) (string, error) {
	stdout := &bytes.Buffer{}
	args := []string{"remote", "show", remote}
	mods = append(append([]CommandModifier{}, mods...), WithStdout(stdout))
	if err := NewCommand("git", args...).Run(ctx, mods...); err != nil {
		return "", err
	}

	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.Contains(line, "HEAD branch") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				branch := strings.TrimSpace(parts[1])
				if branch != "" {
					return branch, nil
				}
			}
		}
	}

	return "", fmt.Errorf("could not determine default branch")
}

func defaultBranchFromSymbolicRef(remote, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	branch, ok := strings.CutPrefix(ref, remote+"/")
	return branch, ok && branch != ""
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

func Rebase(ctx context.Context, newBase, upstream, branch string, mods ...CommandModifier) error {
	var gitErr *GitError

	args := []string{"rebase"}
	if upstream != "" {
		args = append(args, "--onto", newBase, upstream, branch)
	} else {
		args = append(args, newBase, branch)
	}

	if err := NewCommand("git", args...).Run(ctx, mods...); err != nil && errors.As(err, &gitErr) {
		if gitErr.ExitCode == 1 {
			return ErrRebaseConflict
		}
	} else {
		return err
	}
	return nil
}

func AbortRebase(ctx context.Context, mods ...CommandModifier) error {
	return NewCommand("git", "rebase", "--abort").Run(ctx, mods...)
}

func Push(ctx context.Context, branch string, mods ...CommandModifier) error {
	return NewCommand("git", "push", "--force-with-lease", "origin", branch).Run(ctx, mods...)
}

func WorktreeAdd(ctx context.Context, path, branch string, mods ...CommandModifier) error {
	return NewCommand("git", "worktree", "add", "--force", path, "-B", branch, "origin/"+branch).Run(ctx, mods...)
}

func WorktreeRemove(ctx context.Context, path string, mods ...CommandModifier) error {
	return NewCommand("git", "worktree", "remove", "--force", path).Run(ctx, mods...)
}

func Fetch(ctx context.Context, remote string, mods ...CommandModifier) error {
	args := []string{"fetch", remote}
	return NewCommand("git", args...).Run(ctx, mods...)
}

func IsAncestor(ctx context.Context, ancestor, descendant string, mods ...CommandModifier) (bool, error) {
	args := []string{"merge-base", "--is-ancestor", ancestor, descendant}
	err := NewCommand("git", args...).Run(ctx, mods...)
	if err == nil {
		return true, nil
	}
	var gitErr *GitError
	if errors.As(err, &gitErr) && gitErr.ExitCode == 1 {
		return false, nil
	}
	return false, err
}

func ListMergedPullRequests(ctx context.Context, mods ...CommandModifier) ([]gitobj.PullRequest, error) {
	stdout := &bytes.Buffer{}
	fields := []string{
		"number", "title", "url", "author", "state", "isDraft",
		"mergeCommit", "baseRefName", "headRefName", "headRepository", "commits",
	}
	listArgs := []string{
		"pr", "list", "--author", "@me", "--state", "merged", "--limit", "30",
		"--json", strings.Join(fields, ","),
	}
	mods = append(mods, WithStdout(stdout))
	if err := NewCommand("gh", listArgs...).Run(ctx, mods...); err != nil {
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

func GetMergeBase(ctx context.Context, branch1, branch2 string, mods ...CommandModifier) (string, error) {
	stdout := &bytes.Buffer{}
	args := []string{"merge-base", branch1, branch2}
	mods = append(mods, WithStdout(stdout))
	if err := NewCommand("git", args...).Run(ctx, mods...); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func UpdateBaseBranch(ctx context.Context, prNumber int, newBase string, mods ...CommandModifier) error {
	args := []string{"pr", "edit", fmt.Sprint(prNumber), "--base", newBase}
	return NewCommand("gh", args...).Run(ctx, mods...)
}

func RevParse(ctx context.Context, ref string, mods ...CommandModifier) (string, error) {
	stdout := &bytes.Buffer{}
	args := []string{"rev-parse", ref}
	mods = append(mods, WithStdout(stdout))
	if err := NewCommand("git", args...).Run(ctx, mods...); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func ShortSha(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// UpdateBranch rebases the PR branch onto the latest base branch via GitHub API.
// Equivalent to clicking "Update branch" with rebase on the GitHub PR page.
func UpdateBranch(ctx context.Context, prNumber int, mods ...CommandModifier) error {
	args := []string{"pr", "update-branch", "--rebase", fmt.Sprint(prNumber)}
	return NewCommand("gh", args...).Run(ctx, mods...)
}
