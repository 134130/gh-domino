# gh-domino

A `gh` CLI extension to manage stacked pull requests like dominoes.

## What is it?

When working with a chain of dependent pull requests (stacked PRs), merging the first PR in the stack breaks the chain for the others. You then have to manually rebase each subsequent PR, which can be tedious.

`gh-domino` automates this process. It detects when a PR in a stack has been merged and automatically rebases the rest of the PRs in the chain for you.

## Installation

```bash
gh extension install 134130/gh-domino
```

## Usage

Navigate to your repository and run:

```bash
gh domino [--auto] [--dry-run]
```

### Options

- `--auto`: Automatically rebase the PRs without prompting for confirmation.
- `--dry-run`: Show what would happen without making any changes.

### Example

Imagine you have a stack of PRs: `#2` is based on `#1`, and `#3` is based on `#2`.

```
#1: feature-a (main ← feature-a)
└─ #2: feature-b (feature-a ← feature-b)
   └─ #3: feature-c (feature-b ← feature-c)
```

After you merge `#1`, the base of `#2` is now outdated. Running `gh domino` will detect this:

```
✔ Fetching pull requests... done
Pull Requests
└─  #2 feature-b (feature-a ← feature-b) [was on #1]
    └─  #3 feature-c (feature-b ← feature-c)
Dry run mode enabled. The following PRs would be rebased:
  #2 feature-b (feature-a ← feature-b) (update base branch to main)
  #3 feature-c (feature-b ← feature-c)
```

`gh-domino` correctly identifies that `#2` needs its base updated to `main` and that `#3` needs to be rebased onto the new `#2`.

The tool works with all of GitHub's merge strategies (Merge Commit, Rebase and Merge, and Squash and Merge) automatically.
