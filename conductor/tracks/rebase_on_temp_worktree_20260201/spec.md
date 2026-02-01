# Specification: Rebase on Temporary Worktree

## 1. Overview
This feature will modify the rebase process of `gh-domino` to use a temporary Git worktree. This will prevent the user's current workspace from being polluted with intermediate rebase operations, branch checkouts, and potential conflicts. By isolating the rebase process, we can ensure that the user's work-in-progress is not disturbed.

## 2. Functional Requirements
- The rebase operation must be performed in a separate, temporary Git worktree. This worktree will be automatically created if a specific path is not provided.
- The temporary worktree should be created at the beginning of the rebase process and removed at the end.
- All Git operations related to the rebase (fetching, checking out branches, rebasing, pushing) must be executed within the temporary worktree.
- The user's original worktree must remain unchanged throughout the rebase process.
- In case of errors or interruptions, the temporary worktree should be cleaned up and removed.
- The feature should be integrated into the existing `gh domino` command.

## 3. Non-Functional Requirements
- The performance impact of using a temporary worktree should be minimal.
- The implementation should be robust and handle edge cases, such as the temporary worktree already existing or failing to be created/removed.
- The code should be well-documented to explain the worktree-based rebase process.
