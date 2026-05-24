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

func TestPlanSelectNoneSelectsNoActions(t *testing.T) {
	plan := selectionTestPlan()

	selected, err := plan.Select(Selection{None: true})
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}

	if len(selected.Actions) != 0 {
		t.Fatalf("expected no actions, got %#v", selected.Actions)
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

func TestPlanSelectChainSelectsAncestorsOnly(t *testing.T) {
	plan := chainSelectionTestPlan()

	selected, err := plan.Select(Selection{Items: []SelectionItem{{
		PRNumber: 56,
		Mode:     SelectChain,
	}}})
	if err != nil {
		t.Fatalf("Select returned error: %v", err)
	}

	got := actionIDs(selected.Actions)
	want := []string{"repair-pr-52", "repair-pr-53", "repair-pr-56"}
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

func actionIDs(actions []Action) []string {
	ids := make([]string, 0, len(actions))
	for _, action := range actions {
		ids = append(ids, action.ID)
	}
	return ids
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

func chainSelectionTestPlan() *Plan {
	root := &stackedpr.Node{Value: pr(52, "bar", "main", "stack-1")}
	child := &stackedpr.Node{Value: pr(53, "baz", "stack-1", "stack-2")}
	sibling := &stackedpr.Node{Value: pr(55, "sibling", "stack-1", "sibling")}
	grandchild := &stackedpr.Node{Value: pr(56, "quux", "stack-2", "stack-3")}
	root.Children = []*stackedpr.Node{child, sibling}
	child.Children = []*stackedpr.Node{grandchild}

	return &Plan{
		Roots: []*stackedpr.Node{root},
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
			ID:        "repair-pr-55",
			Kind:      ActionRepairPR,
			PR:        sibling.Value,
			NewBase:   "stack-1",
			Reason:    ReasonParentDiverged,
			DependsOn: []string{"repair-pr-52"},
		}, {
			ID:        "repair-pr-56",
			Kind:      ActionRepairPR,
			PR:        grandchild.Value,
			NewBase:   "stack-2",
			Reason:    ReasonParentDiverged,
			DependsOn: []string{"repair-pr-53"},
		}},
	}
}
