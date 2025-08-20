# gh-domino

A `gh` CLI extension to manage stacked pull requests like dominoes.

## Description

Managing stacked pull requests (i.e., a series of PRs that depend on each other) can be cumbersome. When the base branch of a stack is updated, you often need to manually rebase each PR in the chain.

`gh-domino` simplifies this process by automating the synchronization of stacked PRs. It intelligently detects the chain of PRs, rebases them sequentially, and updates the corresponding pull requests on GitHub.

## Broken PR Detection Logic

`gh-domino` identifies a "Broken PR" that needs attention. A "Broken PR" is one whose base branch is either already merged or has diverged (e.g., due to a rebase/force-push), breaking the dependency chain.

The detection logic follows this flowchart:

```mermaid
TODO
```

## Installation

You can install this extension using the official `gh` CLI:

```bash
gh extension install 134130/gh-domino
```

## Usage

Navigate to your git repository where you have stacked pull requests. Then, run the following command:

```bash
gh domino
```

This command will:
1.  Identify the current PR stack based on your branch.
2.  Perform a `git rebase` for each branch in the stack.
3.  Force-push the updated branches.
4.  Update the corresponding pull requests on GitHub.
