# Product Definition: gh-domino

## Initial Concept
gh-domino is a GitHub CLI extension designed to streamline the management of stacked pull requests. It automates the often tedious process of rebasing dependent pull requests (PRs) once a preceding PR in the stack has been merged.

## Core Purpose
The core purpose of `gh-domino` is to automate the rebase of stacked pull requests, ensuring a smooth and efficient workflow for developers working with interdependent code changes.

## Target Users
The primary target users for `gh-domino` are developers who actively work with stacked pull requests. This tool addresses a specific pain point for these users by eliminating the manual effort required to keep their PR stacks synchronized.

## Key Features
- **Automated Rebase:** Automatically rebases dependent PRs when a base PR is merged, maintaining the integrity of the pull request stack.
- **Zero Configuration:** Requires no setup or configuration, allowing developers to integrate it seamlessly into their existing workflows.
- **No State Management:** Operates without special branch naming conventions or local state files, working directly with existing branches and PRs.
- **GitHub Merge Strategy Compatibility:** Works flawlessly with all of GitHub's merge strategies, including Merge Commit, Squash and Merge, and Rebase and Merge.
