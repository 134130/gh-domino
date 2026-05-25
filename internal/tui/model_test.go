package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/stackedpr"
)

func TestModelToggleCurrentModeTogglesRanges(t *testing.T) {
	m := NewModel(context.Background(), tuiTestPlan(false), Options{})

	press(t, &m, " ")
	assertSelection(t, m.Selection(), []app.SelectionItem{{
		PRNumber: 52,
		Mode:     app.SelectNode,
	}})

	press(t, &m, "m")
	press(t, &m, " ")
	assertSelection(t, m.Selection(), []app.SelectionItem{
		{PRNumber: 52, Mode: app.SelectNode},
		{PRNumber: 53, Mode: app.SelectNode},
	})

	m = NewModel(context.Background(), tuiTestPlan(false), Options{})
	press(t, &m, "m")
	press(t, &m, "m")
	press(t, &m, "j")
	press(t, &m, " ")
	assertSelection(t, m.Selection(), []app.SelectionItem{
		{PRNumber: 52, Mode: app.SelectNode},
		{PRNumber: 53, Mode: app.SelectNode},
	})
}

func TestModelSubtreeAndChainPreviewActions(t *testing.T) {
	m := NewModel(context.Background(), tuiChainPlan(), Options{})

	press(t, &m, "m")
	press(t, &m, " ")
	assertActionIDs(t, m.Preview(), []string{"repair-pr-52", "repair-pr-53", "repair-pr-56"})

	m = NewModel(context.Background(), tuiChainPlan(), Options{})
	press(t, &m, "m")
	press(t, &m, "m")
	press(t, &m, "j")
	press(t, &m, "j")
	press(t, &m, " ")
	assertActionIDs(t, m.Preview(), []string{"repair-pr-52", "repair-pr-53", "repair-pr-56"})
}

func TestModelCleanToggleReloadsPlanAndUpdatesPreview(t *testing.T) {
	noClean := tuiTestPlan(false)
	withClean := tuiTestPlan(true)
	var requested []bool
	m := NewModel(context.Background(), noClean, Options{
		LoadPlan: func(_ context.Context, includeClean bool) (*app.Plan, error) {
			requested = append(requested, includeClean)
			if includeClean {
				return withClean, nil
			}
			return noClean, nil
		},
	})

	press(t, &m, "a")
	assertActionIDs(t, m.Preview(), []string{"repair-pr-52"})

	cmd := press(t, &m, "c")
	runCmd(t, &m, cmd)
	if !m.IncludeClean() {
		t.Fatalf("expected clean updates to be included")
	}
	if !reflect.DeepEqual(requested, []bool{true}) {
		t.Fatalf("requested include-clean mismatch: %#v", requested)
	}

	press(t, &m, "a")
	assertActionIDs(t, m.Preview(), []string{"repair-pr-52", "update-branch-54"})

	cmd = press(t, &m, "c")
	runCmd(t, &m, cmd)
	if m.IncludeClean() {
		t.Fatalf("expected clean updates to be excluded")
	}
	assertActionIDs(t, m.Preview(), []string{"repair-pr-52"})
}

func TestModelCleanNodeSelectionCreatesNoActionOrWarning(t *testing.T) {
	m := NewModel(context.Background(), tuiTestPlan(false), Options{})
	m.width = 140

	press(t, &m, "j")
	press(t, &m, " ")

	if got := len(m.Preview().Actions); got != 0 {
		t.Fatalf("expected no actions, got %d", got)
	}
	if got := len(m.Preview().Warnings); got != 0 {
		t.Fatalf("expected no warnings, got %d", got)
	}
	row := m.renderRow(1)
	if strings.Contains(row, "depends on") {
		t.Fatalf("did not expect warning row dependency reason, got %q", row)
	}
	if strings.Contains(row, "after #52") || strings.Contains(row, "after parent repair") {
		t.Fatalf("did not expect follow-up reason for clean-only selection, got %q", row)
	}
}

func TestModelSelectedParentProjectsChildFollowUpRow(t *testing.T) {
	m := NewModel(context.Background(), tuiTestPlan(false), Options{NoColor: true})
	m.width = 140

	press(t, &m, " ")

	assertActionIDs(t, m.Preview(), []string{"repair-pr-52"})
	row := m.renderRow(1)
	if !strings.Contains(row, "✘") {
		t.Fatalf("expected projected follow-up repair to render as cross, got %q", row)
	}
	if !strings.Contains(row, "after #52") {
		t.Fatalf("expected projected follow-up reason, got %q", row)
	}
}

func TestModelSelectedSubtreeShowsFollowUpAsRepairCandidate(t *testing.T) {
	m := NewModel(context.Background(), tuiTestPlan(false), Options{NoColor: true})
	m.width = 140

	press(t, &m, "m")
	press(t, &m, " ")

	assertActionIDs(t, m.Preview(), []string{"repair-pr-52", "repair-pr-53"})
	row := m.renderRow(1)
	if !strings.Contains(row, "✘") {
		t.Fatalf("expected follow-up repair to render as cross, got %q", row)
	}
	if !strings.Contains(row, "after #52") {
		t.Fatalf("expected follow-up reason, got %q", row)
	}
}

func TestModelHelpWrapsInsteadOfTruncating(t *testing.T) {
	m := NewModel(context.Background(), tuiTestPlan(false), Options{})
	m.width = 34
	m.help.SetWidth(34)

	got := m.renderHelp()
	if !strings.Contains(got, "\n") {
		t.Fatalf("expected help to wrap, got %q", got)
	}
	if strings.Contains(got, "…") {
		t.Fatalf("expected help not to truncate, got %q", got)
	}
}

func TestModelPreviewUsesAvailableVerticalSpace(t *testing.T) {
	m := NewModel(context.Background(), tuiManyActionsPlan(), Options{NoColor: true})
	m.width = 160
	m.height = 30

	press(t, &m, "a")

	got := m.viewSelecting()
	if strings.Contains(got, "more actions") {
		t.Fatalf("did not expect preview truncation when space is available, got %q", got)
	}
	if !strings.Contains(got, "repair #55") {
		t.Fatalf("expected final action to render, got %q", got)
	}
}

func TestModelQuitAndConfirmResult(t *testing.T) {
	m := NewModel(context.Background(), tuiTestPlan(false), Options{})

	press(t, &m, "q")
	if !m.Quit() || m.Confirmed() {
		t.Fatalf("expected quit without confirmation")
	}

	m = NewModel(context.Background(), tuiTestPlan(false), Options{Parallel: 2})
	press(t, &m, "a")
	press(t, &m, "p")
	press(t, &m, "enter")
	if !m.Confirmed() || m.Quit() {
		t.Fatalf("expected confirmed model")
	}
	if m.Parallel() != 3 {
		t.Fatalf("parallel mismatch: %d", m.Parallel())
	}
	assertActionIDs(t, m.Preview(), []string{"repair-pr-52"})
}

func press(t *testing.T, m *Model, key string) tea.Cmd {
	t.Helper()
	model, cmd := m.Update(keyMsg(key))
	next, ok := model.(Model)
	if !ok {
		t.Fatalf("unexpected model type %T", model)
	}
	*m = next
	return cmd
}

func runCmd(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatalf("expected command")
	}
	model, _ := m.Update(cmd())
	next, ok := model.(Model)
	if !ok {
		t.Fatalf("unexpected model type %T", model)
	}
	*m = next
}

func keyMsg(value string) tea.KeyPressMsg {
	switch value {
	case "enter":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	case "esc":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc})
	case "j", "k", "m", "a", "c", "p", "P", "q":
		r := []rune(value)[0]
		return tea.KeyPressMsg(tea.Key{Text: value, Code: r})
	case " ":
		return tea.KeyPressMsg(tea.Key{Text: " ", Code: tea.KeySpace})
	default:
		r := []rune(value)[0]
		return tea.KeyPressMsg(tea.Key{Text: value, Code: r})
	}
}

func assertSelection(t *testing.T, selection app.Selection, want []app.SelectionItem) {
	t.Helper()
	if selection.None {
		t.Fatalf("expected selection items, got none")
	}
	if !reflect.DeepEqual(selection.Items, want) {
		t.Fatalf("selection mismatch\nwant: %#v\n got: %#v", want, selection.Items)
	}
}

func assertActionIDs(t *testing.T, plan *app.Plan, want []string) {
	t.Helper()
	if plan == nil {
		t.Fatalf("plan is nil")
	}
	got := make([]string, 0, len(plan.Actions))
	for _, action := range plan.Actions {
		got = append(got, action.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("action IDs mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func tuiTestPlan(includeClean bool) *app.Plan {
	root := &stackedpr.Node{Value: tuiPR(52, "bar updates API contract", "main", "stack-1")}
	child := &stackedpr.Node{Value: tuiPR(53, "baz handles migration fallback", "stack-1", "stack-2")}
	root.Children = []*stackedpr.Node{child}
	other := &stackedpr.Node{Value: tuiPR(54, "aaa refactors client setup", "main", "feature-a")}

	plan := &app.Plan{
		Roots: []*stackedpr.Node{root, other},
		Pulls: []app.PullStatus{{
			PR:      root.Value,
			State:   app.PullStateBroken,
			Reason:  app.ReasonMergedBase,
			NewBase: "main",
		}, {
			PR:    child.Value,
			State: app.PullStateClean,
		}, {
			PR:    other.Value,
			State: app.PullStateClean,
		}},
		Actions: []app.Action{{
			ID:      "repair-pr-52",
			Kind:    app.ActionRepairPR,
			PR:      root.Value,
			NewBase: "main",
			Reason:  app.ReasonMergedBase,
		}},
	}
	if includeClean {
		plan.Pulls[2].State = app.PullStateUpdateable
		plan.Pulls[2].Reason = app.ReasonRebaseAll
		plan.Actions = append(plan.Actions, app.Action{
			ID:     "update-branch-54",
			Kind:   app.ActionUpdateBranch,
			PR:     other.Value,
			Reason: app.ReasonRebaseAll,
		})
	}
	return plan
}

func tuiChainPlan() *app.Plan {
	root := &stackedpr.Node{Value: tuiPR(52, "root", "main", "stack-1")}
	child := &stackedpr.Node{Value: tuiPR(53, "child", "stack-1", "stack-2")}
	grandchild := &stackedpr.Node{Value: tuiPR(56, "grandchild", "stack-2", "stack-3")}
	root.Children = []*stackedpr.Node{child}
	child.Children = []*stackedpr.Node{grandchild}

	return &app.Plan{
		Roots: []*stackedpr.Node{root},
		Actions: []app.Action{{
			ID:      "repair-pr-52",
			Kind:    app.ActionRepairPR,
			PR:      root.Value,
			NewBase: "main",
		}, {
			ID:        "repair-pr-53",
			Kind:      app.ActionRepairPR,
			PR:        child.Value,
			NewBase:   "stack-1",
			DependsOn: []string{"repair-pr-52"},
		}, {
			ID:        "repair-pr-56",
			Kind:      app.ActionRepairPR,
			PR:        grandchild.Value,
			NewBase:   "stack-2",
			DependsOn: []string{"repair-pr-53"},
		}},
	}
}

func tuiManyActionsPlan() *app.Plan {
	root := &stackedpr.Node{Value: tuiPR(52, "root", "main", "stack-1")}
	child := &stackedpr.Node{Value: tuiPR(53, "child", "stack-1", "stack-2")}
	grandchild := &stackedpr.Node{Value: tuiPR(54, "grandchild", "stack-2", "stack-3")}
	greatGrandchild := &stackedpr.Node{Value: tuiPR(55, "great grandchild", "stack-3", "stack-4")}
	root.Children = []*stackedpr.Node{child}
	child.Children = []*stackedpr.Node{grandchild}
	grandchild.Children = []*stackedpr.Node{greatGrandchild}

	return &app.Plan{
		Roots: []*stackedpr.Node{root},
		Actions: []app.Action{{
			ID:      "repair-pr-52",
			Kind:    app.ActionRepairPR,
			PR:      root.Value,
			NewBase: "main",
		}, {
			ID:      "repair-pr-53",
			Kind:    app.ActionRepairPR,
			PR:      child.Value,
			NewBase: "stack-1",
		}, {
			ID:      "repair-pr-54",
			Kind:    app.ActionRepairPR,
			PR:      grandchild.Value,
			NewBase: "stack-2",
		}, {
			ID:      "repair-pr-55",
			Kind:    app.ActionRepairPR,
			PR:      greatGrandchild.Value,
			NewBase: "stack-3",
		}},
	}
}

func tuiPR(number int, title, base, head string) gitobj.PullRequest {
	return gitobj.PullRequest{
		Number:      number,
		Title:       title,
		BaseRefName: base,
		HeadRefName: head,
		State:       gitobj.PullRequestStateOpen,
	}
}
