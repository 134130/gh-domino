# Scenario-Based Test Plan

This document captures behavior scenarios for `test/harness`. The harness uses
a real local Git remote/worktree for Git behavior and a narrow fake GitHub pull
request state for PR metadata and base updates.

## Core Model

Use this stack shape unless a scenario says otherwise:

```text
PR1: main    <- stack-1
PR2: stack-1 <- stack-2
PR3: stack-2 <- stack-3
```

In this model:

- `main` is the repository default branch.
- `origin/*` refs are the source of truth for planning.
- `PR2`'s direct parent is `stack-1`.
- `PR3`'s direct parent is `stack-2`.
- A repair action rebases the PR head branch, pushes it with lease, and updates
  the GitHub PR base branch only when the direct base branch changes.

## Planning Principles

These principles are part of the expected behavior and should be tested
directly.

- A stack is a branch graph. GitHub PR metadata discovers base/head branches and
  merged state, but repair behavior follows ordinary Git branch semantics.
- A PR's parent is its direct base branch. The planner must not infer hidden
  ancestors beyond that branch relationship.
- Selecting a PR must not implicitly select its parent PRs.
- `--subtree <pr>` selects the PR and its descendants, not its ancestors.
- `--chain <pr>` selects the root-to-PR ancestor chain, including the selected
  PR. It does not select descendants or sibling branches.
- If the user wants a whole tree, they should select the root PR with
  `--subtree <root-pr>`.
- A child action may depend on a direct parent action when both are in the same
  selected plan. That is a dependency on the direct parent branch changing, not
  recursive ancestor inference.
- If a selected action depends on an unselected direct parent action, execution
  must not auto-include the parent. The plan may report an unselected dependency
  warning; mutating execution should repair the selected PR against its current
  direct parent branch.
- Dry-run and real execution must use the same action plan. Dry-run must not
  mutate local Git state or GitHub PR state.
- Local branches are not required. The planner should operate from fetched
  remote refs.

## Expected Action Vocabulary

Use these high-level actions in assertions:

- `repair_pr`: rebase the PR head branch, push with lease, and update the PR base
  branch if the direct base changed.
- `update_branch`: ask GitHub to update a clean PR branch when clean PR updates
  are explicitly requested.

Use these reasons in plan assertions:

- `merged_base`: the PR's direct base branch belongs to a merged PR.
- `parent_diverged`: the PR's direct parent branch changed or is projected to
  change in the same selected plan.
- `merged_ancestor`: the PR is based on commits from a recently merged PR even
  though its current base is the default branch.
- `rebase_all`: the PR is clean but was included because clean branch updates
  were requested.

## Merge Strategy Scenarios

| ID | Scenario | Expected behavior |
| --- | --- | --- |
| MS-01 | `PR1` is merged with a merge commit. `PR2` and `PR3` remain open. | Plan `repair_pr` for `PR2` with reason `merged_base`, new base `main`, and no upstream boundary. Plan `repair_pr` for `PR3` with reason `parent_diverged`, depending on the `PR2` action. |
| MS-02 | `PR1` is squash-merged. `PR2` still contains `PR1`'s original commits followed by its own commits. | Plan `repair_pr` for `PR2` with new base `main` and upstream set to the last original commit from `PR1`. Execution uses `git rebase --onto origin/main <last-pr1-commit> stack-2` so `PR1` commits are not replayed into `PR2`. |
| MS-03 | `PR1` is rebase-merged and the original `PR1` commit SHAs are not ancestors of GitHub's merge commit SHA. | Treat it like a history-rewriting merge for boundary purposes. Plan `repair_pr` for `PR2` with new base `main` and upstream set to the last original commit from `PR1`. |
| MS-04 | `PR1` is merged and the `stack-1` branch is deleted on the remote. | Use the recently merged PR metadata to identify `stack-1` as `PR2`'s merged direct base. Plan `PR2` onto `main`. |
| MS-05 | `PR1` is merged but the `stack-1` branch still exists on the remote. | Merged PR state wins over branch presence. Plan `PR2` onto `main`; do not keep it based on stale `stack-1`. |
| MS-06 | `PR1` is merged but is outside the merged PR lookup limit. | Do not guess. If the planner cannot prove that `stack-1` belongs to a merged PR, it should avoid a destructive repair plan and surface a lookup-limit or missing-context warning/error when possible. |

## Parent Update Scenarios

| ID | Scenario | Expected behavior |
| --- | --- | --- |
| PU-01 | `PR1` is still open. Review feedback adds a fix commit to `stack-1`. `PR2` is still based on the old `stack-1` tip. | Plan `repair_pr` for `PR2` with reason `parent_diverged`. Rebase `stack-2` onto `origin/stack-1`. Do not update `PR2`'s base branch. |
| PU-02 | `PR1` is still open and `stack-1` is force-pushed/rebased. | Plan `repair_pr` for `PR2` with reason `parent_diverged`. Rebase onto the new `origin/stack-1`. |
| PU-03 | `PR1` is updated, and `PR2` is already based on the new `origin/stack-1` tip. | `PR2` is clean. No action unless clean updates are explicitly requested. |
| PU-04 | Clean updates are requested for a clean PR. | Plan `update_branch` with reason `rebase_all`. This must be opt-in and separate from broken PR repair. |

## Multi-Level Stack Scenarios

| ID | Scenario | Expected behavior |
| --- | --- | --- |
| ML-01 | `PR1` is merged. `PR2` and `PR3` remain open. The whole tree is selected by using `--subtree <PR2>` or no selector. | Plan actions in dependency order: repair `PR2`, then repair `PR3`. `PR3` depends only on the direct parent action for `PR2`. |
| ML-02 | Same state as ML-01, but only `PR3` is selected with exact PR selection. | Do not implicitly select `PR2`. Execute only `PR3` against its current direct parent `origin/stack-2`. Report an unselected direct parent dependency warning if the plan includes the `PR3` action. |
| ML-03 | `PR2` has already been repaired and pushed onto `main`. `PR3` is still based on the old `stack-2` tip. | Plan `repair_pr` for `PR3` with reason `parent_diverged`. Rebase onto `origin/stack-2`. No ancestor lookup beyond `PR3`'s direct parent is needed. |
| ML-04 | `PR1` and `PR2` are both merged, and `PR3` is still open. `PR2` was merged with base `main`. | Treat `stack-2` as `PR3`'s merged direct base. Plan `PR3` onto `main`. |
| ML-05 | `PR1` and `PR2` are both merged, but `PR2` was merged into `stack-1` rather than `main`. | Treat `stack-2` as `PR3`'s merged direct base and plan `PR3` onto `stack-1`. Do not recursively collapse `stack-1` to `main` in the same action. |
| ML-06 | Two independent stacks exist, and only one stack is broken. | Generate actions only for the broken stack. Independent stacks must not create dependencies on each other. |

## Branching Stack Scenarios

Use this branch shape for fan-out scenarios:

```text
PR1:   main    <- stack-1
PR2.1: stack-1 <- stack-2-1
PR2.2: stack-1 <- stack-2-2
PR2.3: stack-1 <- stack-2-3
```

Here `PR2.1`, `PR2.2`, and `PR2.3` are siblings. They share `PR1` as their
direct parent, but they do not depend on each other.

| ID | Scenario | Expected behavior |
| --- | --- | --- |
| BS-01 | `PR1` is merged. `PR2.1`, `PR2.2`, and `PR2.3` remain open. | Plan one independent `repair_pr` action for each sibling with reason `merged_base` and new base `main`. No sibling action depends on another sibling action. |
| BS-02 | `PR1` is still open and `stack-1` receives a new fix commit. All sibling PRs are still based on the old `stack-1` tip. | Plan one independent `repair_pr` action for each sibling with reason `parent_diverged`. Each sibling rebases onto `origin/stack-1`; no PR base branch is updated. |
| BS-03 | Same state as BS-01, but only `PR2.2` is selected with exact PR selection. | Repair only `PR2.2`. Do not implicitly select `PR2.1`, `PR2.3`, or `PR1`. |
| BS-04 | `PR1` is still open, and `--subtree <PR1>` is selected. | Include `PR1`'s descendants: `PR2.1`, `PR2.2`, and `PR2.3`. Exact action output still includes only PRs that are actionable. |
| BS-05 | `PR1` is still open, and `--chain <PR2.2>` is selected. | Include the root-to-PR chain for `PR2.2`, such as `PR1` and `PR2.2`. Do not include sibling branches such as `PR2.1` or `PR2.3`, and do not include descendants of `PR2.2`. |
| BS-06 | `PR1` is already merged, and `--chain <PR2.2>` is selected. | Do not group siblings through a hidden merged parent node. Select only the chain that exists in the open PR graph for `PR2.2`, unless a future explicit selector is added for shared merged bases. |
| BS-07 | One sibling conflicts during rebase, while the other siblings can rebase cleanly. | The failed sibling aborts and reports a manual command. Because siblings are independent, the executor may continue with other siblings if the execution policy supports continue-on-error; otherwise it should stop deterministically without implying sibling dependency. |
| BS-08 | `PR2.1` is repaired and pushed. `PR2.2` and `PR2.3` are not repaired yet. | `PR2.2` and `PR2.3` remain independently repairable against their direct parent `origin/stack-1`. `PR2.1` changing must not create dependencies for sibling branches. |
| BS-09 | The tree branches as `PR1 -> PR2.1 -> PR3` and `PR1 -> PR2.2`. `--chain <PR3>` is selected. | Include only the selected linear path: `PR1`, `PR2.1`, and `PR3`, limited to actionable PRs. Do not include sibling branch `PR2.2`. |
| BS-10 | The tree branches as `PR1 -> PR2.1 -> PR3` and `PR1 -> PR2.2`. `--chain <PR2.1>` is selected. | Include `PR1` and `PR2.1`, limited to actionable PRs. Do not include descendant `PR3` or sibling branch `PR2.2`. Use `--subtree <PR2.1>` when descendants are intended. |
| BS-11 | The tree is `PR1 -> PR2`, with `PR2 -> PR2.1` and `PR2 -> PR2.2 -> PR3`. | `--pr <PR3>` selects only `PR3`; `--chain <PR3>` selects `PR1`, `PR2`, `PR2.2`, and `PR3`; `--subtree <PR2.2>` selects `PR2.2` and `PR3`; `--subtree <PR2>` selects `PR2`, `PR2.1`, `PR2.2`, and `PR3`; `--subtree <PR1>` selects the whole tree. |

## Conflict and Failure Scenarios

| ID | Scenario | Expected behavior |
| --- | --- | --- |
| CF-01 | `PR1` is merged and `PR2` conflicts while rebasing onto `main`. | Abort the rebase. Do not push `stack-2`. Do not update `PR2`'s base branch. Report a manual rebase command. Do not continue to `PR3` because its direct parent action failed. |
| CF-02 | `PR2` repairs successfully, then `PR3` conflicts while rebasing onto the updated `stack-2`. | Keep the successful `PR2` result. Abort `PR3`'s rebase, do not push `stack-3`, and report a manual rebase command for `PR3`. |
| CF-03 | `git push --force-with-lease` fails because the remote branch changed after planning. | Stop before PR base edit. Report that the remote branch moved and the user should refetch/re-run. |
| CF-04 | Rebase and push succeed, but `gh pr edit --base` fails. | Report partial success. A later run should be able to retry the base update without requiring another rebase if the branch is already correct. |
| CF-05 | The working tree is dirty or a rebase is already in progress. | Mutating execution should stop before modifying branches. If worktree-based execution is used, the original working tree should remain untouched. |

## Remote and Branch Availability Scenarios

| ID | Scenario | Expected behavior |
| --- | --- | --- |
| RB-01 | `stack-2` and `stack-3` do not exist as local branches. Only `origin/stack-2` and `origin/stack-3` exist. | Planning succeeds from remote refs. Execution creates/checks out whatever local state it needs without requiring the user to create branches manually. |
| RB-02 | Local `stack-2` exists but is stale compared to `origin/stack-2`. | Planning uses `origin/stack-2`, not the stale local branch. Execution must avoid rebasing the wrong commit range. |
| RB-03 | Branch names contain slashes, such as `feature/trunk-b`. | Stack detection, worktree paths, rebase commands, push commands, and PR base edits handle the branch name safely. |
| RB-04 | The default branch is `trunk`, not `main`. | New base calculations use the repository default branch. No behavior should be hardcoded to `main`. |
| RB-05 | A PR head branch is in a fork rather than the base repository. | If fork PR repair is unsupported, fail explicitly before mutation. If supported later, fetch and push using the correct fork remote/ref instead of assuming `origin/<head>`. |

## Output and Selection Scenarios

| ID | Scenario | Expected behavior |
| --- | --- | --- |
| OS-01 | `gh domino plan` is run with no selector. | Show all actionable repairs and their dependencies. Do not mutate repository or GitHub state. |
| OS-02 | `gh domino plan --pr <PR3>` is run when `PR3` has an unselected direct parent action. | Do not auto-add the parent. Include an unselected dependency warning if the selected plan still shows `PR3`'s action. |
| OS-03 | `gh domino plan --subtree <PR2>` is run. | Include `PR2` and descendants such as `PR3`. Do not include ancestors such as `PR1`. |
| OS-04 | `gh domino plan --chain <PR3>` is run. | Include the root-to-`PR3` ancestor chain. In a branching tree, exclude descendants and sibling branches. This is explicit chain selection, not implicit dependency inclusion. |
| OS-05 | Human and JSON output are both requested in separate tests. | Human output should be stable enough for snapshots. JSON output should expose action IDs, kinds, PR numbers, heads, new bases, upstream boundaries, reasons, dependencies, and warnings. |
