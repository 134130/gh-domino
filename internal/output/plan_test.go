package output

import (
	"strings"
	"testing"

	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/stackedpr"
)

func TestRenderPlanHuman(t *testing.T) {
	node := &stackedpr.Node{Value: pull(52, "bar", "stack-1", "stack-2")}
	action := app.Action{
		ID:       "repair-pr-52",
		Kind:     app.ActionRepairPR,
		PR:       node.Value,
		NewBase:  "main",
		Upstream: "abc123",
		Reason:   app.ReasonMergedBase,
	}
	plan := &app.Plan{
		Roots: []*stackedpr.Node{node},
		Pulls: []app.PullStatus{{
			PR:      node.Value,
			State:   app.PullStateBroken,
			Reason:  app.ReasonMergedBase,
			NewBase: "main",
		}},
		Actions: []app.Action{action},
	}
	selected := &app.Plan{
		Roots:   plan.Roots,
		Pulls:   plan.Pulls,
		Actions: []app.Action{action},
	}

	var out strings.Builder
	if err := RenderPlan(&out, plan, selected, PlanOptions{Format: FormatHuman, NoColor: true}); err != nil {
		t.Fatalf("RenderPlan returned error: %v", err)
	}

	want := "Pull Requests\n" +
		"  ✘ #52 bar (stack-1 ← stack-2) · rebase onto main · base was merged\n" +
		"\n" +
		"Actions\n" +
		"  repair #52 stack-2 → main\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestRenderPlanHumanWarnings(t *testing.T) {
	parent := &stackedpr.Node{Value: pull(52, "qux", "main", "stack-1")}
	child := &stackedpr.Node{Value: pull(53, "baz", "stack-1", "stack-2")}
	parent.Children = []*stackedpr.Node{child}
	parentAction := app.Action{
		ID:      "repair-pr-52",
		Kind:    app.ActionRepairPR,
		PR:      parent.Value,
		NewBase: "main",
		Reason:  app.ReasonMergedBase,
	}
	childAction := app.Action{
		ID:        "repair-pr-53",
		Kind:      app.ActionRepairPR,
		PR:        child.Value,
		NewBase:   "stack-1",
		Reason:    app.ReasonParentDiverged,
		DependsOn: []string{"repair-pr-52"},
	}
	plan := &app.Plan{
		Roots: []*stackedpr.Node{parent},
		Pulls: []app.PullStatus{{
			PR:      parent.Value,
			State:   app.PullStateBroken,
			Reason:  app.ReasonMergedBase,
			NewBase: "main",
		}, {
			PR:      child.Value,
			State:   app.PullStateBroken,
			Reason:  app.ReasonParentDiverged,
			NewBase: "stack-1",
		}},
		Actions: []app.Action{parentAction, childAction},
	}
	selected := &app.Plan{
		Roots:   plan.Roots,
		Pulls:   plan.Pulls,
		Actions: []app.Action{childAction},
		Warnings: []app.SelectionWarning{{
			Kind:             app.SelectionWarningUnselectedDependency,
			PRNumber:         53,
			ActionID:         "repair-pr-53",
			DependencyID:     "repair-pr-52",
			DependencyPR:     52,
			DependencyKind:   app.ActionRepairPR,
			DependencyHead:   "stack-1",
			DependencyBase:   "main",
			DependencyReason: app.ReasonMergedBase,
		}},
	}

	var out strings.Builder
	if err := RenderPlan(&out, plan, selected, PlanOptions{Format: FormatHuman, NoColor: true}); err != nil {
		t.Fatalf("RenderPlan returned error: %v", err)
	}

	want := "Pull Requests\n" +
		"  ✘ #52 qux (main ← stack-1) · rebase onto main · base was merged\n" +
		"  ! └── #53 baz (stack-1 ← stack-2) · warning · depends on #52\n" +
		"\n" +
		"Actions\n" +
		"  repair #53 stack-2 → stack-1\n" +
		"\n" +
		"Warnings\n" +
		"  warning: #53 depends on unselected #52\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestRenderPlanJSONWarnings(t *testing.T) {
	plan := &app.Plan{
		Actions: []app.Action{{
			ID:      "repair-pr-53",
			Kind:    app.ActionRepairPR,
			PR:      pull(53, "baz", "stack-1", "stack-2"),
			NewBase: "stack-1",
			Reason:  app.ReasonParentDiverged,
		}},
		Warnings: []app.SelectionWarning{{
			Kind:         app.SelectionWarningUnselectedDependency,
			PRNumber:     53,
			ActionID:     "repair-pr-53",
			DependencyID: "repair-pr-52",
			DependencyPR: 52,
		}},
	}

	var out strings.Builder
	if err := RenderPlan(&out, plan, plan, PlanOptions{Format: FormatJSON}); err != nil {
		t.Fatalf("RenderPlan returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`"warnings":[`,
		`"reason":"parent_diverged"`,
		`"kind":"unselected_dependency"`,
		`"dependencyId":"repair-pr-52"`,
		`"dependencyPr":52`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got %q", want, got)
		}
	}
}

func TestRenderRunResultHuman(t *testing.T) {
	result := &app.RunResult{
		Actions: []app.ActionResult{{
			Action: app.Action{
				ID:      "repair-pr-52",
				Kind:    app.ActionRepairPR,
				PR:      pull(52, "bar", "stack-1", "stack-2"),
				NewBase: "main",
			},
			Status: app.ActionStatusSuccess,
		}, {
			Action: app.Action{
				ID:   "update-branch-53",
				Kind: app.ActionUpdateBranch,
				PR:   pull(53, "baz", "main", "topic"),
			},
			Status: app.ActionStatusFailed,
			Error:  "update failed",
		}},
	}

	var out strings.Builder
	if err := RenderRunResult(&out, result, FormatHuman); err != nil {
		t.Fatalf("RenderRunResult returned error: %v", err)
	}

	want := "Results\n" +
		"  success repair #52 stack-2 onto main\n" +
		"  failed update-branch #53 topic: update failed\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestRenderRunResultJSON(t *testing.T) {
	result := &app.RunResult{
		Actions: []app.ActionResult{{
			Action: app.Action{
				ID:      "repair-pr-52",
				Kind:    app.ActionRepairPR,
				PR:      pull(52, "bar", "stack-1", "stack-2"),
				NewBase: "main",
			},
			Status: app.ActionStatusFailed,
			Error:  "rebase conflict",
		}},
	}

	var out strings.Builder
	if err := RenderRunResult(&out, result, FormatJSON); err != nil {
		t.Fatalf("RenderRunResult returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`"id":"repair-pr-52"`,
		`"status":"failed"`,
		`"error":"rebase conflict"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got %q", want, got)
		}
	}
}

func TestRenderListFiltersBrokenWithTreeContext(t *testing.T) {
	parent := &stackedpr.Node{Value: pull(52, "bar", "main", "stack-1")}
	child := &stackedpr.Node{Value: pull(53, "baz", "stack-1", "stack-2")}
	parent.Children = []*stackedpr.Node{child}

	plan := &app.Plan{
		Roots: []*stackedpr.Node{parent},
		Pulls: []app.PullStatus{{
			PR:    parent.Value,
			State: app.PullStateClean,
		}, {
			PR:      child.Value,
			State:   app.PullStateBroken,
			Reason:  app.ReasonParentDiverged,
			NewBase: "stack-1",
		}},
	}

	var out strings.Builder
	if err := RenderList(&out, plan, ListOptions{Format: FormatHuman, State: "broken", NoColor: true}); err != nil {
		t.Fatalf("RenderList returned error: %v", err)
	}

	want := "Pull Requests\n" +
		"  ✔︎ #52 bar (main ← stack-1)\n" +
		"  ✘ └── #53 baz (stack-1 ← stack-2) · needs rebase onto #52 · parent changed\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch\nwant: %q\n got: %q", want, got)
	}
}

func pull(number int, title, base, head string) gitobj.PullRequest {
	return gitobj.PullRequest{
		Number:      number,
		Title:       title,
		BaseRefName: base,
		HeadRefName: head,
	}
}
