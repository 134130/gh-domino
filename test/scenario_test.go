package test

import (
	"testing"

	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/test/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHarnessRepairsMergeCommitMergedParent(t *testing.T) {
	h := harness.New(t)
	h.Branch("stack-1", "origin/main")
	h.Commit("stack-1", "parent.txt", "parent\n")
	h.Push("stack-1")
	h.OpenPR(1, "parent", "main", "stack-1")

	h.Branch("stack-2", "origin/stack-1")
	h.Commit("stack-2", "child.txt", "child\n")
	h.Push("stack-2")
	h.OpenPR(2, "child", "stack-1", "stack-2")

	h.MergeCommit(1)
	plan := h.Plan(app.PlanOptions{})
	action := actionForPR(t, plan, 2)
	assert.Equal(t, "main", action.NewBase)
	assert.Empty(t, action.Upstream)
	assert.Equal(t, app.ReasonMergedBase, action.Reason)

	result := h.Execute(plan, app.ExecuteOptions{})
	assertActionStatuses(t, result, []app.ActionStatus{app.ActionStatusSuccess})
	assert.Equal(t, "main", h.PRBase(2))
	assert.Equal(t, h.Ref("origin/main"), h.MergeBase("origin/main", "origin/stack-2"))
}

func TestHarnessRepairsSquashMergedParentWithUpstreamBoundary(t *testing.T) {
	h := harness.New(t)
	h.Branch("stack-1", "origin/main")
	parentCommit := h.Commit("stack-1", "parent.txt", "parent\n")
	h.Push("stack-1")
	h.OpenPR(1, "parent", "main", "stack-1")

	h.Branch("stack-2", "origin/stack-1")
	h.Commit("stack-2", "child.txt", "child\n")
	h.Push("stack-2")
	h.OpenPR(2, "child", "stack-1", "stack-2")

	h.SquashMerge(1)
	plan := h.Plan(app.PlanOptions{})
	action := actionForPR(t, plan, 2)
	assert.Equal(t, "main", action.NewBase)
	assert.Equal(t, parentCommit, action.Upstream)
	assert.Equal(t, app.ReasonMergedBase, action.Reason)

	result := h.Execute(plan, app.ExecuteOptions{})
	assertActionStatuses(t, result, []app.ActionStatus{app.ActionStatusSuccess})
	assert.Equal(t, "main", h.PRBase(2))
	commits := h.RevList("origin/main..origin/stack-2")
	assert.Len(t, commits, 1)
	assert.NotContains(t, commits, parentCommit)
}

func TestHarnessRepairsSquashMergedParentAdvancedAfterChildBranch(t *testing.T) {
	h := harness.New(t)
	h.Branch("stack-1", "origin/main")
	h.Commit("stack-1", "parent-a.txt", "parent a\n")
	sharedBoundary := h.Commit("stack-1", "parent-b.txt", "parent b\n")
	h.Push("stack-1")

	h.Branch("stack-2", "origin/stack-1")
	h.Commit("stack-2", "child-a.txt", "child a\n")
	h.Commit("stack-2", "child-b.txt", "child b\n")
	h.Push("stack-2")

	h.Commit("stack-1", "parent-late.txt", "parent late\n")
	h.Push("stack-1")
	h.OpenPR(1, "parent", "main", "stack-1")
	h.OpenPR(2, "child", "main", "stack-2")

	h.SquashMerge(1)
	plan := h.Plan(app.PlanOptions{})
	action := actionForPR(t, plan, 2)
	assert.Equal(t, "main", action.NewBase)
	assert.Equal(t, sharedBoundary, action.Upstream)
	assert.Equal(t, app.ReasonMergedAncestor, action.Reason)

	result := h.Execute(plan, app.ExecuteOptions{})
	assertActionStatuses(t, result, []app.ActionStatus{app.ActionStatusSuccess})
	assert.Contains(t, h.Runner().CommandStrings(), "git rebase --onto origin/main "+sharedBoundary+" stack-2")
	assert.Equal(t, "main", h.PRBase(2))
	assert.Equal(t, h.Ref("origin/main"), h.MergeBase("origin/main", "origin/stack-2"))
	assert.Len(t, h.RevList("origin/main..origin/stack-2"), 2)
}

func TestHarnessRepairsRebaseMergedParentWithUpstreamBoundary(t *testing.T) {
	h := harness.New(t)
	h.Branch("stack-1", "origin/main")
	parentCommit := h.Commit("stack-1", "parent.txt", "parent\n")
	h.Push("stack-1")
	h.OpenPR(1, "parent", "main", "stack-1")

	h.Branch("stack-2", "origin/stack-1")
	h.Commit("stack-2", "child.txt", "child\n")
	h.Push("stack-2")
	h.OpenPR(2, "child", "stack-1", "stack-2")

	h.Commit("main", "main.txt", "main advanced\n")
	h.Push("main")
	h.RebaseMerge(1)
	plan := h.Plan(app.PlanOptions{})
	action := actionForPR(t, plan, 2)
	assert.Equal(t, "main", action.NewBase)
	assert.Equal(t, parentCommit, action.Upstream)
	assert.Equal(t, app.ReasonMergedBase, action.Reason)

	result := h.Execute(plan, app.ExecuteOptions{})
	assertActionStatuses(t, result, []app.ActionStatus{app.ActionStatusSuccess})
	assert.Equal(t, "main", h.PRBase(2))
}

func TestHarnessRepairsRemoteOnlyHeadBranch(t *testing.T) {
	h := harness.New(t)
	h.Branch("stack-1", "origin/main")
	h.Commit("stack-1", "parent.txt", "parent\n")
	h.Push("stack-1")
	h.OpenPR(1, "parent", "main", "stack-1")

	h.Branch("stack-2", "origin/stack-1")
	h.Commit("stack-2", "child.txt", "child\n")
	h.Push("stack-2")
	h.OpenPR(2, "child", "stack-1", "stack-2")

	h.SquashMerge(1)
	h.DeleteLocalBranch("stack-2")
	result := h.Execute(h.Plan(app.PlanOptions{}), app.ExecuteOptions{})
	assertActionStatuses(t, result, []app.ActionStatus{app.ActionStatusSuccess})
	assert.Equal(t, "main", h.PRBase(2))
}

func TestHarnessCleanDescendantSelectionDoesNotCreateParentFollowUp(t *testing.T) {
	h := harness.New(t)
	h.Branch("stack-1", "origin/main")
	h.Commit("stack-1", "parent.txt", "parent\n")
	h.Push("stack-1")
	h.OpenPR(1, "parent", "main", "stack-1")

	h.Branch("stack-2", "origin/stack-1")
	h.Commit("stack-2", "child.txt", "child\n")
	h.Push("stack-2")
	h.OpenPR(2, "child", "stack-1", "stack-2")

	h.Branch("stack-3", "origin/stack-2")
	before := h.Commit("stack-3", "grandchild.txt", "grandchild\n")
	h.Push("stack-3")
	h.OpenPR(3, "grandchild", "stack-2", "stack-3")

	h.SquashMerge(1)
	plan := h.Plan(app.PlanOptions{})
	selected, err := plan.Select(app.Selection{Items: []app.SelectionItem{{
		PRNumber: 3,
		Mode:     app.SelectNode,
	}}})
	require.NoError(t, err)
	assert.Empty(t, actionIDs(selected.Actions))
	assert.Empty(t, selected.Warnings)

	result := h.Execute(selected, app.ExecuteOptions{})
	assertActionStatuses(t, result, []app.ActionStatus{})
	assert.Equal(t, before, h.Ref("origin/stack-3"))
	assert.Equal(t, "stack-2", h.PRBase(3))
}

func TestHarnessTreeSelectorsUseChainAndSubtreeScopes(t *testing.T) {
	h := harness.New(t)
	h.Branch("stack-1", "origin/main")
	h.Commit("stack-1", "root.txt", "root\n")
	h.Push("stack-1")
	h.OpenPR(1, "root", "main", "stack-1")

	h.Branch("stack-2", "origin/stack-1")
	h.Commit("stack-2", "node.txt", "node\n")
	h.Push("stack-2")
	h.OpenPR(2, "node", "stack-1", "stack-2")

	h.Branch("stack-2-1", "origin/stack-2")
	h.Commit("stack-2-1", "sibling-a.txt", "a\n")
	h.Push("stack-2-1")
	h.OpenPR(21, "sibling-a", "stack-2", "stack-2-1")

	h.Branch("stack-2-2", "origin/stack-2")
	h.Commit("stack-2-2", "sibling-b.txt", "b\n")
	h.Push("stack-2-2")
	h.OpenPR(22, "sibling-b", "stack-2", "stack-2-2")

	h.Branch("stack-3", "origin/stack-2-2")
	h.Commit("stack-3", "leaf.txt", "leaf\n")
	h.Push("stack-3")
	h.OpenPR(3, "leaf", "stack-2-2", "stack-3")

	h.Commit("stack-1", "root-fix.txt", "fix\n")
	h.Push("stack-1")
	plan := h.Plan(app.PlanOptions{})

	chain, err := plan.Select(app.Selection{Items: []app.SelectionItem{{
		PRNumber: 3,
		Mode:     app.SelectChain,
	}}})
	require.NoError(t, err)
	assert.Equal(t, []string{"repair-pr-2", "repair-pr-22", "repair-pr-3"}, actionIDs(chain.Actions))

	subtree, err := plan.Select(app.Selection{Items: []app.SelectionItem{{
		PRNumber: 2,
		Mode:     app.SelectSubtree,
	}}})
	require.NoError(t, err)
	assert.Equal(t, []string{"repair-pr-2", "repair-pr-21", "repair-pr-22", "repair-pr-3"}, actionIDs(subtree.Actions))
}

func TestHarnessRepairsIndependentSiblingsInParallel(t *testing.T) {
	h := harness.New(t)
	h.Branch("stack-1", "origin/main")
	h.Commit("stack-1", "root.txt", "root\n")
	h.Push("stack-1")
	h.OpenPR(1, "root", "main", "stack-1")

	h.Branch("stack-2-1", "origin/stack-1")
	h.Commit("stack-2-1", "sibling-a.txt", "a\n")
	h.Push("stack-2-1")
	h.OpenPR(21, "sibling-a", "stack-1", "stack-2-1")

	h.Branch("stack-2-2", "origin/stack-1")
	h.Commit("stack-2-2", "sibling-b.txt", "b\n")
	h.Push("stack-2-2")
	h.OpenPR(22, "sibling-b", "stack-1", "stack-2-2")

	h.SquashMerge(1)
	plan := h.Plan(app.PlanOptions{})
	assert.Equal(t, []string{"repair-pr-21", "repair-pr-22"}, actionIDs(plan.Actions))

	result := h.Execute(plan, app.ExecuteOptions{Parallel: 2})
	assertActionStatuses(t, result, []app.ActionStatus{app.ActionStatusSuccess, app.ActionStatusSuccess})
	assert.Equal(t, "main", h.PRBase(21))
	assert.Equal(t, "main", h.PRBase(22))
	assert.Equal(t, h.Ref("origin/main"), h.MergeBase("origin/main", "origin/stack-2-1"))
	assert.Equal(t, h.Ref("origin/main"), h.MergeBase("origin/main", "origin/stack-2-2"))
}

func TestHarnessConflictAbortsWithoutPushOrBaseUpdate(t *testing.T) {
	h := harness.New(t)
	h.Commit("main", "conflict.txt", "base\n")
	h.Push("main")

	h.Branch("stack-1", "origin/main")
	h.Commit("stack-1", "conflict.txt", "parent\n")
	h.Push("stack-1")
	h.OpenPR(1, "parent", "main", "stack-1")

	h.Branch("stack-2", "origin/stack-1")
	h.Commit("stack-2", "conflict.txt", "child\n")
	h.Push("stack-2")
	h.OpenPR(2, "child", "stack-1", "stack-2")

	h.SquashMerge(1)
	before := h.Ref("origin/stack-2")
	h.Commit("main", "conflict.txt", "main\n")
	h.Push("main")

	plan := h.Plan(app.PlanOptions{})
	result := h.Execute(plan, app.ExecuteOptions{})
	assertActionStatuses(t, result, []app.ActionStatus{app.ActionStatusFailed})
	assert.Contains(t, result.Actions[0].Error, "rebase conflict")
	assert.Equal(t, before, h.Ref("origin/stack-2"))
	assert.Equal(t, "stack-1", h.PRBase(2))
	assert.Empty(t, h.StatusPorcelain())
}

func actionForPR(t *testing.T, plan *app.Plan, number int) app.Action {
	t.Helper()
	for _, action := range plan.Actions {
		if action.PR.Number == number {
			return action
		}
	}
	require.FailNow(t, "missing action for PR", "PR #%d in %#v", number, plan.Actions)
	return app.Action{}
}

func actionIDs(actions []app.Action) []string {
	ids := make([]string, 0, len(actions))
	for _, action := range actions {
		ids = append(ids, action.ID)
	}
	return ids
}

func assertActionStatuses(t *testing.T, result *app.RunResult, want []app.ActionStatus) {
	t.Helper()
	got := make([]app.ActionStatus, 0, len(result.Actions))
	for _, action := range result.Actions {
		got = append(got, action.Status)
	}
	assert.Equal(t, want, got, "result: %#v", result.Actions)
}
