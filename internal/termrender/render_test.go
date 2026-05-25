package termrender

import (
	"strings"
	"testing"

	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/stackedpr"
)

func TestRenderListStatusRows(t *testing.T) {
	root := &stackedpr.Node{Value: testPR(52, "bar", "main", "stack-1")}
	child := &stackedpr.Node{Value: testPR(53, "baz", "stack-1", "stack-2")}
	root.Children = []*stackedpr.Node{child}
	plan := &app.Plan{
		Roots: []*stackedpr.Node{root},
		Pulls: []app.PullStatus{{
			PR:    root.Value,
			State: app.PullStateClean,
		}, {
			PR:      child.Value,
			State:   app.PullStateBroken,
			Reason:  app.ReasonParentDiverged,
			NewBase: "stack-1",
		}},
	}

	got := RenderList(plan, "all", false, Options{NoColor: true})
	want := "Pull Requests\n" +
		"  ✔︎ #52 bar (main ← stack-1)\n" +
		"  ✘ └── #53 baz (stack-1 ← stack-2) · needs rebase onto stack-1 · parent changed\n"
	if got != want {
		t.Fatalf("output mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestRenderPlanSnapshotShowsTreeAndPreview(t *testing.T) {
	root := &stackedpr.Node{Value: testPR(52, "bar", "stack-1", "stack-2")}
	action := app.Action{
		ID:      "repair-pr-52",
		Kind:    app.ActionRepairPR,
		PR:      root.Value,
		NewBase: "main",
		Reason:  app.ReasonMergedBase,
	}
	plan := &app.Plan{
		Roots: []*stackedpr.Node{root},
		Pulls: []app.PullStatus{{
			PR:      root.Value,
			State:   app.PullStateBroken,
			Reason:  app.ReasonMergedBase,
			NewBase: "main",
		}},
		Actions: []app.Action{action},
	}
	preview := &app.Plan{Roots: plan.Roots, Pulls: plan.Pulls, Actions: []app.Action{action}}

	got := RenderPlanSnapshot(plan, preview, Options{NoColor: true})
	want := "Pull Requests\n" +
		"  ✘ #52 bar (stack-1 ← stack-2) · rebase onto main · base was merged\n" +
		"\n" +
		"Actions\n" +
		"  repair #52 stack-2 → main\n"
	if got != want {
		t.Fatalf("output mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestRenderPlanSnapshotUsesHumanMergedBaseReason(t *testing.T) {
	merged := testPR(51, "foo", "main", "stack-1")
	root := &stackedpr.Node{
		Value:        testPR(52, "bar", "stack-1", "stack-2"),
		OriginalBase: &merged,
	}
	action := app.Action{
		ID:      "repair-pr-52",
		Kind:    app.ActionRepairPR,
		PR:      root.Value,
		NewBase: "main",
		Reason:  app.ReasonMergedBase,
	}
	plan := &app.Plan{
		Roots: []*stackedpr.Node{root},
		Pulls: []app.PullStatus{{
			PR:           root.Value,
			OriginalBase: &merged,
			State:        app.PullStateBroken,
			Reason:       app.ReasonMergedBase,
			NewBase:      "main",
		}},
		Actions: []app.Action{action},
	}

	got := RenderPlanSnapshot(plan, plan, Options{NoColor: true})
	if !strings.Contains(got, "base #51 was merged") {
		t.Fatalf("expected human merged-base reason, got %q", got)
	}
	if strings.Contains(got, "merged_base") {
		t.Fatalf("did not expect internal reason code, got %q", got)
	}
}

func TestRenderPlanSnapshotDoesNotTruncateWhenWidthUnset(t *testing.T) {
	head := "domino-20260525145351-very-long-child-branch-name"
	root := &stackedpr.Node{Value: testPR(52, "bar", "main", head)}
	plan := &app.Plan{
		Roots: []*stackedpr.Node{root},
		Pulls: []app.PullStatus{{
			PR:    root.Value,
			State: app.PullStateClean,
		}},
	}

	got := RenderPlanSnapshot(plan, &app.Plan{}, Options{NoColor: true})
	if !strings.Contains(got, head+")") {
		t.Fatalf("expected full branch name to render without truncation, got %q", got)
	}
}

func TestRenderPlanSnapshotShowsAllActions(t *testing.T) {
	plan := &app.Plan{
		Actions: []app.Action{
			{ID: "a1", Kind: app.ActionRepairPR, PR: testPR(1, "one", "main", "one"), NewBase: "main"},
			{ID: "a2", Kind: app.ActionRepairPR, PR: testPR(2, "two", "main", "two"), NewBase: "main"},
			{ID: "a3", Kind: app.ActionRepairPR, PR: testPR(3, "three", "main", "three"), NewBase: "main"},
			{ID: "a4", Kind: app.ActionRepairPR, PR: testPR(4, "four", "main", "four"), NewBase: "main"},
		},
	}

	got := RenderPlanSnapshot(plan, plan, Options{NoColor: true})
	if strings.Contains(got, "more actions") {
		t.Fatalf("did not expect action truncation, got %q", got)
	}
	if !strings.Contains(got, "repair #4 four → main") {
		t.Fatalf("expected final action to render, got %q", got)
	}
}

func TestRenderRowProjectsFollowUpRepair(t *testing.T) {
	root := &stackedpr.Node{Value: testPR(52, "bar", "main", "stack-1")}
	child := &stackedpr.Node{Value: testPR(53, "baz", "stack-1", "stack-2")}
	root.Children = []*stackedpr.Node{child}
	parentAction := app.Action{
		ID:      "repair-pr-52",
		Kind:    app.ActionRepairPR,
		PR:      root.Value,
		NewBase: "main",
		Reason:  app.ReasonMergedBase,
	}
	plan := &app.Plan{
		Roots: []*stackedpr.Node{root},
		Pulls: []app.PullStatus{{
			PR:      root.Value,
			State:   app.PullStateBroken,
			Reason:  app.ReasonMergedBase,
			NewBase: "main",
		}, {
			PR:    child.Value,
			State: app.PullStateClean,
		}},
		Actions: []app.Action{parentAction},
	}
	preview := &app.Plan{Roots: plan.Roots, Pulls: plan.Pulls, Actions: []app.Action{parentAction}}
	ctx := NewContext(plan, preview, RowModePlan, Options{NoColor: true})

	got := ctx.RenderRow(FlattenTree(plan.Roots)[1], RowState{})
	if !strings.Contains(got, "✘") || !strings.Contains(got, "after #52") {
		t.Fatalf("expected projected repair row, got %q", got)
	}
}

func TestRenderPreviewLimitsActionsAndWarnings(t *testing.T) {
	plan := &app.Plan{
		Actions: []app.Action{
			{ID: "a1", Kind: app.ActionRepairPR, PR: testPR(1, "one", "main", "one"), NewBase: "main"},
			{ID: "a2", Kind: app.ActionRepairPR, PR: testPR(2, "two", "main", "two"), NewBase: "main"},
		},
		Warnings: []app.SelectionWarning{
			{Kind: app.SelectionWarningUnselectedDependency, PRNumber: 1, DependencyID: "a0"},
			{Kind: app.SelectionWarningUnselectedDependency, PRNumber: 2, DependencyID: "a1"},
		},
	}

	got := RenderPreview(plan, Options{NoColor: true, MaxPreviewActions: 1, MaxPreviewWarnings: 1})
	for _, want := range []string{"Preview: 2 actions · 2 warnings", "… 1 more actions", "… 1 more warnings"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in %q", want, got)
		}
	}
}

func TestNoColorSuppressesANSI(t *testing.T) {
	plan := &app.Plan{
		Actions: []app.Action{{
			ID:      "repair-pr-52",
			Kind:    app.ActionRepairPR,
			PR:      testPR(52, "bar", "stack-1", "stack-2"),
			NewBase: "main",
			Reason:  app.ReasonMergedBase,
		}},
	}

	got := RenderPreview(plan, Options{NoColor: true})
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("expected no ANSI escapes, got %q", got)
	}
}

func testPR(number int, title, base, head string) gitobj.PullRequest {
	return gitobj.PullRequest{
		Number:      number,
		Title:       title,
		BaseRefName: base,
		HeadRefName: head,
		State:       gitobj.PullRequestStateOpen,
	}
}
