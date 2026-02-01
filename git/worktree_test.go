package git_test

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/134130/gh-domino/git"
)

func TestCreateAndRemoveWorktree(t *testing.T) {
	// Create a temporary directory for the test repository
	repoDir, err := ioutil.TempDir("", "test-repo")
	assert.NoError(t, err)
	defer os.RemoveAll(repoDir)

	ctx := context.Background()

	// Initialize a new Git repository
	initCmd := git.NewCommand("git", "init")
	err = initCmd.Run(ctx, git.WithWorkingDir(repoDir))
	assert.NoError(t, err)

	// Create a dummy file and commit it
	dummyFilePath := filepath.Join(repoDir, "dummy.txt")
	err = ioutil.WriteFile(dummyFilePath, []byte("hello"), 0644)
	assert.NoError(t, err)
	addCmd := git.NewCommand("git", "add", ".")
	err = addCmd.Run(ctx, git.WithWorkingDir(repoDir))
	assert.NoError(t, err)
	commitCmd := git.NewCommand("git", "commit", "-m", "Initial commit")
	err = commitCmd.Run(ctx, git.WithWorkingDir(repoDir))
	assert.NoError(t, err)

	// Create a new branch
	branchCmd := git.NewCommand("git", "branch", "test-branch")
	err = branchCmd.Run(ctx, git.WithWorkingDir(repoDir))
	assert.NoError(t, err)

	// Create a temporary directory for the worktree
	worktreeDir, err := ioutil.TempDir("", "test-worktree")
	assert.NoError(t, err)
	defer os.RemoveAll(worktreeDir)

	// Test creating a worktree
	err = git.CreateWorktree(ctx, repoDir, worktreeDir, "test-branch")
	assert.NoError(t, err)
	assert.FileExists(t, filepath.Join(worktreeDir, "dummy.txt"))

	// Test removing the worktree
	err = git.RemoveWorktree(ctx, repoDir, worktreeDir)
	assert.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(worktreeDir, ".git"))
}
