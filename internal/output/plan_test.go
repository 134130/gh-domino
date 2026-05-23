package output

import (
	"strings"
	"testing"

	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/stackedpr"
)

func TestRenderPlanHuman(t *testing.T) {
	plan := &app.Plan{
		Actions: []app.Action{{
			ID:       "repair-pr-52",
			Kind:     app.ActionRepairPR,
			PR:       pull(52, "bar", "stack-1", "stack-2"),
			NewBase:  "main",
			Upstream: "abc123",
			Reason:   app.ReasonMergedBase,
		}},
	}

	var out strings.Builder
	if err := RenderPlan(&out, plan, FormatHuman); err != nil {
		t.Fatalf("RenderPlan returned error: %v", err)
	}

	want := "Actions\n  repair #52 stack-2 onto main (upstream abc123)\n"
	if got := out.String(); got != want {
		t.Fatalf("output mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestRenderPlanHumanWarnings(t *testing.T) {
	plan := &app.Plan{
		Actions: []app.Action{{
			ID:      "repair-pr-53",
			Kind:    app.ActionRepairPR,
			PR:      pull(53, "baz", "stack-1", "stack-2"),
			NewBase: "stack-1",
			Reason:  app.ReasonParentDiverged,
		}},
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
	if err := RenderPlan(&out, plan, FormatHuman); err != nil {
		t.Fatalf("RenderPlan returned error: %v", err)
	}

	want := "Actions\n" +
		"  repair #53 stack-2 onto stack-1\n" +
		"\n" +
		"Warnings\n" +
		"  #53 has an unselected related action: repair #52 stack-1 onto main. Use --subtree or --stack to include it.\n"
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
	if err := RenderPlan(&out, plan, FormatJSON); err != nil {
		t.Fatalf("RenderPlan returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`"warnings":[`,
		`"kind":"unselected_dependency"`,
		`"dependencyId":"repair-pr-52"`,
		`"dependencyPr":52`,
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
	if err := RenderList(&out, plan, ListOptions{Format: FormatHuman, State: "broken"}); err != nil {
		t.Fatalf("RenderList returned error: %v", err)
	}

	want := "Pull Requests\n└─ #52 bar (main <- stack-1)\n   └─ #53 baz (stack-1 <- stack-2) [broken: parent_diverged -> stack-1]\n"
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
