package app

import (
	"reflect"
	"testing"

	"github.com/134130/gh-domino/internal/stackedpr"
)

func TestPlanSelectEmptySelectionSelectsAllActions(t *testing.T) {
	plan := selectionTestPlan()

	selected, err := plan.Select(Selection{})
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}

	got := actionIDs(selected.Actions)
	want := []string{"repair-pr-52", "repair-pr-53", "update-branch-54"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selected actions mismatch\nwant: %#v\n got: %#v", want, got)
	}
	if len(selected.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", selected.Warnings)
	}
}

func TestPlanSelectNodeDoesNotIncludeDependencies(t *testing.T) {
	plan := selectionTestPlan()

	selected, err := plan.Select(Selection{Items: []SelectionItem{{
		PRNumber: 53,
		Mode:     SelectNode,
	}}})
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}

	got := actionIDs(selected.Actions)
	want := []string{"repair-pr-53"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selected actions mismatch\nwant: %#v\n got: %#v", want, got)
	}
	if len(selected.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %#v", selected.Warnings)
	}
	warning := selected.Warnings[0]
	if warning.Kind != SelectionWarningUnselectedDependency {
		t.Fatalf("warning kind mismatch: %s", warning.Kind)
	}
	if warning.PRNumber != 53 || warning.DependencyID != "repair-pr-52" || warning.DependencyPR != 52 {
		t.Fatalf("warning mismatch: %#v", warning)
	}
}

func TestPlanSelectSubtreeSelectsNodeAndDescendants(t *testing.T) {
	plan := selectionTestPlan()

	selected, err := plan.Select(Selection{Items: []SelectionItem{{
		PRNumber: 52,
		Mode:     SelectSubtree,
	}}})
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}

	got := actionIDs(selected.Actions)
	want := []string{"repair-pr-52", "repair-pr-53"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selected actions mismatch\nwant: %#v\n got: %#v", want, got)
	}
	if len(selected.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", selected.Warnings)
	}
}

func TestPlanSelectStackSelectsWholeRootStack(t *testing.T) {
	plan := selectionTestPlan()

	selected, err := plan.Select(Selection{Items: []SelectionItem{{
		PRNumber: 53,
		Mode:     SelectStack,
	}}})
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}

	got := actionIDs(selected.Actions)
	want := []string{"repair-pr-52", "repair-pr-53"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selected actions mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestPlanSelectDedupesRepeatedSelections(t *testing.T) {
	plan := selectionTestPlan()

	selected, err := plan.Select(Selection{Items: []SelectionItem{{
		PRNumber: 52,
		Mode:     SelectNode,
	}, {
		PRNumber: 52,
		Mode:     SelectSubtree,
	}}})
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}

	got := actionIDs(selected.Actions)
	want := []string{"repair-pr-52", "repair-pr-53"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selected actions mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestPlanSelectUnknownPRReturnsError(t *testing.T) {
	plan := selectionTestPlan()

	_, err := plan.Select(Selection{Items: []SelectionItem{{
		PRNumber: 999,
		Mode:     SelectNode,
	}}})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestPlanSelectStacksFiltersRootsPullsAndActions(t *testing.T) {
	plan := selectionTestPlan()

	selected, err := plan.SelectStacks([]int{53})
	if err != nil {
		t.Fatalf("SelectStacks returned error: %v", err)
	}

	gotActions := actionIDs(selected.Actions)
	wantActions := []string{"repair-pr-52", "repair-pr-53"}
	if !reflect.DeepEqual(gotActions, wantActions) {
		t.Fatalf("selected actions mismatch\nwant: %#v\n got: %#v", wantActions, gotActions)
	}

	gotRoots := rootPRNumbers(selected.Roots)
	wantRoots := []int{52}
	if !reflect.DeepEqual(gotRoots, wantRoots) {
		t.Fatalf("selected roots mismatch\nwant: %#v\n got: %#v", wantRoots, gotRoots)
	}

	gotPulls := pullStatusPRNumbers(selected.Pulls)
	wantPulls := []int{52, 53}
	if !reflect.DeepEqual(gotPulls, wantPulls) {
		t.Fatalf("selected pulls mismatch\nwant: %#v\n got: %#v", wantPulls, gotPulls)
	}
}

func TestPlanSelectStacksUnknownPRReturnsError(t *testing.T) {
	plan := selectionTestPlan()

	_, err := plan.SelectStacks([]int{999})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func actionIDs(actions []Action) []string {
	ids := make([]string, 0, len(actions))
	for _, action := range actions {
		ids = append(ids, action.ID)
	}
	return ids
}

func rootPRNumbers(roots []*stackedpr.Node) []int {
	numbers := make([]int, 0, len(roots))
	for _, root := range roots {
		numbers = append(numbers, root.Value.Number)
	}
	return numbers
}

func pullStatusPRNumbers(statuses []PullStatus) []int {
	numbers := make([]int, 0, len(statuses))
	for _, status := range statuses {
		numbers = append(numbers, status.PR.Number)
	}
	return numbers
}

func selectionTestPlan() *Plan {
	root := &stackedpr.Node{Value: pr(52, "bar", "main", "stack-1")}
	child := &stackedpr.Node{Value: pr(53, "baz", "stack-1", "stack-2")}
	root.Children = []*stackedpr.Node{child}
	other := &stackedpr.Node{Value: pr(54, "qux", "main", "feature")}

	return &Plan{
		Roots: []*stackedpr.Node{root, other},
		Pulls: []PullStatus{{
			PR:     root.Value,
			State:  PullStateBroken,
			Reason: ReasonMergedBase,
		}, {
			PR:     child.Value,
			State:  PullStateBroken,
			Reason: ReasonParentDiverged,
		}, {
			PR:    other.Value,
			State: PullStateUpdateable,
		}},
		Actions: []Action{{
			ID:      "repair-pr-52",
			Kind:    ActionRepairPR,
			PR:      root.Value,
			NewBase: "main",
			Reason:  ReasonMergedBase,
		}, {
			ID:        "repair-pr-53",
			Kind:      ActionRepairPR,
			PR:        child.Value,
			NewBase:   "stack-1",
			Reason:    ReasonParentDiverged,
			DependsOn: []string{"repair-pr-52"},
		}, {
			ID:     "update-branch-54",
			Kind:   ActionUpdateBranch,
			PR:     other.Value,
			Reason: ReasonRebaseAll,
		}},
	}
}
