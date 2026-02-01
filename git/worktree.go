package git

import "context"

func CreateWorktree(ctx context.Context, repoDir, path, branch string) error {
	cmd := NewCommand("git", "worktree", "add", "-B", branch, path, branch)
	return cmd.Run(ctx, WithWorkingDir(repoDir))
}

func RemoveWorktree(ctx context.Context, repoDir, path string) error {
	cmd := NewCommand("git", "worktree", "remove", path)
	return cmd.Run(ctx, WithWorkingDir(repoDir))
}
