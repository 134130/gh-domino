package termrender

import (
	"fmt"
	"image/color"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/stackedpr"
)

type Options struct {
	Width              int
	NoColor            bool
	IsDark             bool
	MaxPreviewActions  int
	MaxPreviewWarnings int
}

type RowState struct {
	Selected bool
	Cursor   bool
}

type RowMode int

const (
	RowModeStatus RowMode = iota
	RowModePlan
)

type FlatNode struct {
	Node       *stackedpr.Node
	Depth      int
	TreePrefix string
}

type Context struct {
	preview    *app.Plan
	mode       RowMode
	opts       Options
	styles     styles
	statusByPR map[int]app.PullStatus
	actionByPR map[int]app.Action
	actionByID map[string]app.Action
	prByHead   map[string]gitobj.PullRequest
}

func NewContext(plan, preview *app.Plan, mode RowMode, opts Options) Context {
	if plan == nil {
		plan = &app.Plan{}
	}
	if preview == nil {
		preview = &app.Plan{}
	}
	actionByID := ActionsByID(plan.Actions)
	for _, action := range preview.Actions {
		actionByID[action.ID] = action
	}
	prByHead := PRsByHead(plan, preview)
	return Context{
		preview:    preview,
		mode:       mode,
		opts:       opts,
		styles:     newStyles(opts),
		statusByPR: StatusesByPR(plan.Pulls),
		actionByPR: ActionsByPR(plan.Actions),
		actionByID: actionByID,
		prByHead:   prByHead,
	}
}

func RenderList(plan *app.Plan, state string, flat bool, opts Options) string {
	if plan == nil {
		plan = &app.Plan{}
	}
	ctx := NewContext(plan, nil, RowModeStatus, opts)
	lines := []string{"Pull Requests"}
	if flat {
		for _, status := range plan.Pulls {
			if !MatchesState(status, state) {
				continue
			}
			node := &stackedpr.Node{Value: status.PR, OriginalBase: status.OriginalBase}
			lines = append(lines, ctx.RenderRow(FlatNode{Node: node}, RowState{}))
		}
		return strings.Join(lines, "\n") + "\n"
	}

	for _, fn := range FilteredTree(plan.Roots, ctx.statusByPR, state) {
		lines = append(lines, ctx.RenderRow(fn, RowState{}))
	}
	return strings.Join(lines, "\n") + "\n"
}

func RenderPlanSnapshot(plan, preview *app.Plan, opts Options) string {
	if plan == nil {
		plan = &app.Plan{}
	}
	if preview == nil {
		preview = &app.Plan{}
	}
	ctx := NewContext(plan, preview, RowModePlan, opts)
	lines := []string{"Pull Requests"}
	flat := FlattenTree(plan.Roots)
	if len(flat) == 0 {
		lines = append(lines, "  No pull requests found.")
	} else {
		for _, fn := range flat {
			lines = append(lines, ctx.RenderRow(fn, RowState{}))
		}
	}
	lines = append(lines, "", ctx.RenderActions())
	return strings.Join(lines, "\n") + "\n"
}

func (c Context) RenderRow(fn FlatNode, state RowState) string {
	if fn.Node == nil {
		return ""
	}
	pr := fn.Node.Value
	status, ok := c.statusByPR[pr.Number]
	if !ok {
		status = app.PullStatus{
			PR:           pr,
			OriginalBase: fn.Node.OriginalBase,
			State:        app.PullStateClean,
		}
	}
	if status.OriginalBase == nil {
		status.OriginalBase = fn.Node.OriginalBase
	}

	warning, hasWarning := warningForPR(c.preview, pr.Number)
	currentAction, hasCurrentAction := c.actionByPR[pr.Number]
	previewAction, hasPreviewAction := previewActionForPR(c.preview, pr.Number)
	displayAction, hasDisplayAction := c.displayActionForRow(pr, currentAction, hasCurrentAction, previewAction, hasPreviewAction)
	symbol, _, style := c.rowStatus(status, hasWarning, displayAction, hasDisplayAction)
	cursorActive := state.Cursor
	bg := cursorBg(c.opts.IsDark)

	plainStyle := lipgloss.NewStyle()
	if cursorActive && !c.opts.NoColor {
		plainStyle = plainStyle.Background(bg)
	}
	plain := func(value string) string {
		if value == "" {
			return ""
		}
		return plainStyle.Render(value)
	}
	withCursorBg := func(style lipgloss.Style) lipgloss.Style {
		if c.opts.NoColor {
			return lipgloss.NewStyle()
		}
		if cursorActive {
			return style.Background(bg)
		}
		return style
	}

	prefix := "  "
	switch {
	case cursorActive && state.Selected:
		prefix = ">*"
	case cursorActive:
		prefix = "> "
	case state.Selected:
		prefix = "* "
	}

	var row strings.Builder
	row.WriteString(withCursorBg(c.styles.selectedMark).Render(prefix))
	row.WriteString(withCursorBg(style).Render(symbol))
	row.WriteString(plain(" " + fn.TreePrefix))
	row.WriteString(withCursorBg(c.styles.prNumber(pr)).Render(fmt.Sprintf("#%d", pr.Number)))
	row.WriteString(plain(" " + pr.Title + " ("))
	row.WriteString(withCursorBg(c.styles.baseBranch).Render(pr.BaseRefName))
	row.WriteString(plain(" ← "))
	row.WriteString(withCursorBg(c.styles.headBranch).Render(pr.HeadRefName))
	row.WriteString(plain(")"))
	row.WriteString(c.reasonSuffix(status, displayAction, hasDisplayAction, warning, hasWarning, plain, withCursorBg))
	line := row.String()

	width := c.opts.width()
	if width > 0 {
		line = lipgloss.NewStyle().Inline(true).MaxWidth(width).Render(line)
	}
	if !cursorActive {
		return line
	}

	if extra := width - lipgloss.Width(line); extra > 0 {
		line += plainStyle.Render(strings.Repeat(" ", extra))
	}
	return line
}

func (c Context) RenderPreview() string {
	return RenderPreview(c.preview, c.opts)
}

func (c Context) RenderActions() string {
	return RenderActions(c.preview, c.opts)
}

func RenderPreview(plan *app.Plan, opts Options) string {
	styles := newStyles(opts)
	if plan == nil {
		return styles.dim.Render("Preview: 0 actions")
	}

	actionCount := len(plan.Actions)
	warningCount := len(plan.Warnings)
	header := fmt.Sprintf("Preview: %d actions", actionCount)
	if warningCount > 0 {
		header += fmt.Sprintf(" · %d warnings", warningCount)
	}
	lines := []string{styles.meta.Render(header)}

	maxActions := opts.maxPreviewActions()
	for i, action := range plan.Actions {
		if i == maxActions {
			lines = append(lines, "  "+styles.dim.Render(fmt.Sprintf("… %d more actions", actionCount-maxActions)))
			break
		}
		lines = append(lines, "  "+ActionSummary(action, opts))
	}
	maxWarnings := opts.maxPreviewWarnings()
	for i, warning := range plan.Warnings {
		if i == maxWarnings {
			lines = append(lines, "  "+styles.dim.Render(fmt.Sprintf("… %d more warnings", warningCount-maxWarnings)))
			break
		}
		lines = append(lines, "  "+styles.warning.Render(WarningSummary(warning)))
	}
	return strings.Join(lines, "\n")
}

func RenderActions(plan *app.Plan, opts Options) string {
	styles := newStyles(opts)
	if plan == nil {
		plan = &app.Plan{}
	}

	lines := []string{"Actions"}
	if len(plan.Actions) == 0 {
		lines = append(lines, "  No actions.")
	}
	for _, action := range plan.Actions {
		lines = append(lines, "  "+ActionSummary(action, opts))
	}
	if len(plan.Warnings) > 0 {
		lines = append(lines, "", "Warnings")
		for _, warning := range plan.Warnings {
			lines = append(lines, "  "+styles.warning.Render(WarningSummary(warning)))
		}
	}
	return strings.Join(lines, "\n")
}

func ActionSummary(action app.Action, opts Options) string {
	styles := newStyles(opts)
	pr := action.PR
	prNumber := styles.prNumber(pr).Render(fmt.Sprintf("#%d", pr.Number))
	head := styles.headBranch.Render(pr.HeadRefName)
	switch action.Kind {
	case app.ActionRepairPR:
		target := action.NewBase
		if target == "" {
			target = pr.BaseRefName
		}
		return fmt.Sprintf("%s %s %s → %s",
			styles.repair.Render("repair"),
			prNumber,
			head,
			styles.baseBranch.Render(target),
		)
	case app.ActionUpdateBranch:
		return fmt.Sprintf("%s %s %s",
			styles.update.Render("update"),
			prNumber,
			head,
		)
	default:
		return fmt.Sprintf("%s %s %s", action.Kind, prNumber, head)
	}
}

func WarningSummary(warning app.SelectionWarning) string {
	if warning.DependencyPR != 0 {
		return fmt.Sprintf("warning: #%d depends on unselected #%d", warning.PRNumber, warning.DependencyPR)
	}
	return fmt.Sprintf("warning: #%d depends on unselected %s", warning.PRNumber, warning.DependencyID)
}

func FlattenTree(roots []*stackedpr.Node) []FlatNode {
	return flattenTree(roots, nil)
}

func FilteredTree(roots []*stackedpr.Node, statuses map[int]app.PullStatus, state string) []FlatNode {
	if state == "" || state == "all" {
		return FlattenTree(roots)
	}
	visible := visiblePRs(roots, statuses, state)
	return flattenTree(roots, visible)
}

func StatusesByPR(statuses []app.PullStatus) map[int]app.PullStatus {
	out := make(map[int]app.PullStatus, len(statuses))
	for _, status := range statuses {
		out[status.PR.Number] = status
	}
	return out
}

func ActionsByPR(actions []app.Action) map[int]app.Action {
	out := make(map[int]app.Action, len(actions))
	for _, action := range actions {
		out[action.PR.Number] = action
	}
	return out
}

func ActionsByID(actions []app.Action) map[string]app.Action {
	out := make(map[string]app.Action, len(actions))
	for _, action := range actions {
		out[action.ID] = action
	}
	return out
}

func PRsByHead(plans ...*app.Plan) map[string]gitobj.PullRequest {
	out := map[string]gitobj.PullRequest{}
	for _, plan := range plans {
		if plan == nil {
			continue
		}
		var walk func(*stackedpr.Node)
		walk = func(node *stackedpr.Node) {
			if node == nil {
				return
			}
			if node.Value.HeadRefName != "" {
				out[node.Value.HeadRefName] = node.Value
			}
			if node.OriginalBase != nil && node.OriginalBase.HeadRefName != "" {
				out[node.OriginalBase.HeadRefName] = *node.OriginalBase
			}
			for _, child := range node.Children {
				walk(child)
			}
		}
		for _, root := range plan.Roots {
			walk(root)
		}
		for _, status := range plan.Pulls {
			if status.PR.HeadRefName != "" {
				out[status.PR.HeadRefName] = status.PR
			}
			if status.OriginalBase != nil && status.OriginalBase.HeadRefName != "" {
				out[status.OriginalBase.HeadRefName] = *status.OriginalBase
			}
		}
		for _, action := range plan.Actions {
			if action.PR.HeadRefName != "" {
				out[action.PR.HeadRefName] = action.PR
			}
		}
	}
	return out
}

func ParentsByPR(roots []*stackedpr.Node) map[int]int {
	out := map[int]int{}
	var walk func(*stackedpr.Node, int)
	walk = func(node *stackedpr.Node, parent int) {
		if node == nil {
			return
		}
		if parent != 0 {
			out[node.Value.Number] = parent
		}
		for _, child := range node.Children {
			walk(child, node.Value.Number)
		}
	}
	for _, root := range roots {
		walk(root, 0)
	}
	return out
}

func MatchesState(status app.PullStatus, state string) bool {
	switch state {
	case "", "all":
		return true
	case "broken":
		return status.State == app.PullStateBroken
	case "clean":
		return status.State == app.PullStateClean
	case "updateable":
		return status.State == app.PullStateUpdateable
	default:
		return false
	}
}

func (c Context) rowStatus(status app.PullStatus, hasWarning bool, action app.Action, hasAction bool) (symbol, label string, style lipgloss.Style) {
	if hasWarning && c.mode == RowModePlan {
		return "!", "WARN", c.styles.warning
	}
	if c.mode == RowModeStatus {
		switch status.State {
		case app.PullStateBroken:
			return "✘", "BROKEN", c.styles.repair
		case app.PullStateUpdateable:
			return "↻", "STALE", c.styles.update
		default:
			return "✔︎", "CLEAN", c.styles.clean
		}
	}
	if hasAction {
		switch action.Kind {
		case app.ActionRepairPR:
			return "✘", "REPAIR", c.styles.repair
		case app.ActionUpdateBranch:
			return "↻", "UPDATE", c.styles.update
		}
	}
	if status.State == app.PullStateUpdateable {
		return "↻", "STALE", c.styles.update
	}
	return "✔︎", "CLEAN", c.styles.clean
}

func (c Context) displayActionForRow(
	pr gitobj.PullRequest,
	currentAction app.Action,
	hasCurrentAction bool,
	previewAction app.Action,
	hasPreviewAction bool,
) (app.Action, bool) {
	if c.mode == RowModeStatus {
		return app.Action{}, false
	}
	if hasCurrentAction {
		return currentAction, true
	}
	if hasPreviewAction {
		return previewAction, true
	}
	return projectedFollowUpActionForPR(c.preview, pr)
}

func (c Context) reasonSuffix(
	status app.PullStatus,
	action app.Action,
	hasAction bool,
	warning app.SelectionWarning,
	hasWarning bool,
	plain func(string) string,
	withCursorBg func(lipgloss.Style) lipgloss.Style,
) string {
	parts := make([]string, 0, 3)
	switch {
	case hasWarning && c.mode == RowModePlan:
		parts = append(parts, plain("warning"))
		if reason := warningReasonText(warning, hasWarning, plain, withCursorBg, c.styles.warning); reason != "" {
			parts = append(parts, reason)
		}
	case c.mode == RowModePlan && hasAction:
		parts = append(parts, c.planReasonParts(action, status, plain, withCursorBg)...)
	case c.mode == RowModePlan && status.State == app.PullStateUpdateable:
		parts = append(parts, c.statusReasonParts(status, plain, withCursorBg)...)
	case c.mode == RowModeStatus:
		parts = append(parts, c.statusReasonParts(status, plain, withCursorBg)...)
	}
	if len(parts) == 0 {
		return ""
	}
	var out strings.Builder
	for _, part := range parts {
		out.WriteString(plain(" · "))
		out.WriteString(part)
	}
	return out.String()
}

func (c Context) planReasonParts(
	action app.Action,
	status app.PullStatus,
	plain func(string) string,
	withCursorBg func(lipgloss.Style) lipgloss.Style,
) []string {
	switch action.Kind {
	case app.ActionRepairPR:
		target := action.NewBase
		if target == "" {
			target = status.NewBase
		}
		if target == "" {
			target = action.PR.BaseRefName
		}
		parts := []string{c.renderTarget("rebase onto ", target, plain, withCursorBg)}
		reason := action.Reason
		if reason == "" {
			reason = status.Reason
		}
		if detail := c.reasonDetail(reason, status.OriginalBase, action, plain, withCursorBg); detail != "" {
			parts = append(parts, detail)
		}
		return parts
	case app.ActionUpdateBranch:
		parts := []string{plain("update branch")}
		reason := action.Reason
		if reason == "" {
			reason = status.Reason
		}
		if detail := c.reasonDetail(reason, status.OriginalBase, action, plain, withCursorBg); detail != "" {
			parts = append(parts, detail)
		}
		return parts
	default:
		return []string{plain(string(action.Kind))}
	}
}

func (c Context) statusReasonParts(
	status app.PullStatus,
	plain func(string) string,
	withCursorBg func(lipgloss.Style) lipgloss.Style,
) []string {
	switch status.State {
	case app.PullStateBroken:
		label := "needs rebase"
		if status.NewBase != "" {
			label = "needs rebase onto "
		}
		var parts []string
		if status.NewBase != "" {
			parts = []string{c.renderTarget(label, status.NewBase, plain, withCursorBg)}
		} else {
			parts = []string{plain(label)}
		}
		if detail := c.reasonDetail(status.Reason, status.OriginalBase, app.Action{}, plain, withCursorBg); detail != "" {
			parts = append(parts, detail)
		}
		return parts
	case app.PullStateUpdateable:
		parts := []string{plain("stale branch")}
		if detail := c.reasonDetail(status.Reason, status.OriginalBase, app.Action{}, plain, withCursorBg); detail != "" {
			parts = append(parts, detail)
		}
		return parts
	default:
		return nil
	}
}

func (c Context) reasonDetail(
	reason app.Reason,
	originalBase *gitobj.PullRequest,
	action app.Action,
	plain func(string) string,
	withCursorBg func(lipgloss.Style) lipgloss.Style,
) string {
	switch reason {
	case app.ReasonMergedBase, app.ReasonMergedAncestor:
		if originalBase != nil {
			return fmt.Sprintf(
				"%s%s%s",
				plain("base "),
				withCursorBg(c.styles.prNumber(*originalBase)).Render(fmt.Sprintf("#%d", originalBase.Number)),
				plain(" was merged"),
			)
		}
		return plain("base was merged")
	case app.ReasonParentDiverged:
		return plain("parent changed")
	case app.ReasonParentWillChange:
		if dependency, ok := c.dependencyAction(action); ok {
			return fmt.Sprintf(
				"%s%s",
				plain("after "),
				withCursorBg(c.styles.prNumber(dependency.PR)).Render(fmt.Sprintf("#%d", dependency.PR.Number)),
			)
		}
		return plain("after parent repair")
	case app.ReasonBaseStale:
		return plain("base changed")
	case app.ReasonRebaseAll:
		return plain("clean update")
	default:
		return ""
	}
}

func (c Context) renderTarget(
	prefix string,
	ref string,
	plain func(string) string,
	withCursorBg func(lipgloss.Style) lipgloss.Style,
) string {
	if pr, ok := c.prByHead[ref]; ok {
		return plain(prefix) + withCursorBg(c.styles.prNumber(pr)).Render(fmt.Sprintf("#%d", pr.Number))
	}
	return plain(prefix + ref)
}

func (c Context) dependencyAction(action app.Action) (app.Action, bool) {
	for _, dependencyID := range action.DependsOn {
		if dependency, ok := c.actionByID[dependencyID]; ok {
			return dependency, true
		}
	}
	return app.Action{}, false
}

func warningReasonText(
	warning app.SelectionWarning,
	ok bool,
	plain func(string) string,
	withCursorBg func(lipgloss.Style) lipgloss.Style,
	warningStyle lipgloss.Style,
) string {
	if !ok {
		return ""
	}
	if warning.DependencyPR != 0 {
		return fmt.Sprintf("%s%s",
			plain("depends on "),
			withCursorBg(warningStyle).Render(fmt.Sprintf("#%d", warning.DependencyPR)),
		)
	}
	if warning.DependencyID != "" {
		return plain(fmt.Sprintf("depends on %s", warning.DependencyID))
	}
	return plain("missing dependency")
}

func previewActionForPR(plan *app.Plan, prNumber int) (app.Action, bool) {
	if plan == nil {
		return app.Action{}, false
	}
	for _, action := range plan.Actions {
		if action.PR.Number == prNumber {
			return action, true
		}
	}
	return app.Action{}, false
}

func projectedFollowUpActionForPR(plan *app.Plan, pr gitobj.PullRequest) (app.Action, bool) {
	if plan == nil {
		return app.Action{}, false
	}
	for _, action := range plan.Actions {
		if action.PR.Number == pr.Number {
			return app.Action{}, false
		}
	}
	for _, action := range plan.Actions {
		if action.PR.Number == pr.Number || action.PR.HeadRefName != pr.BaseRefName {
			continue
		}
		return app.Action{
			ID:        fmt.Sprintf("repair-pr-%d", pr.Number),
			Kind:      app.ActionRepairPR,
			PR:        pr,
			NewBase:   pr.BaseRefName,
			Reason:    app.ReasonParentWillChange,
			DependsOn: []string{action.ID},
		}, true
	}
	return app.Action{}, false
}

func warningForPR(plan *app.Plan, prNumber int) (app.SelectionWarning, bool) {
	if plan == nil {
		return app.SelectionWarning{}, false
	}
	index := slices.IndexFunc(plan.Warnings, func(warning app.SelectionWarning) bool {
		return warning.PRNumber == prNumber
	})
	if index < 0 {
		return app.SelectionWarning{}, false
	}
	return plan.Warnings[index], true
}

func visiblePRs(roots []*stackedpr.Node, statuses map[int]app.PullStatus, state string) map[int]bool {
	visible := map[int]bool{}
	var walk func(*stackedpr.Node) bool
	walk = func(node *stackedpr.Node) bool {
		if node == nil {
			return false
		}
		status, found := statuses[node.Value.Number]
		if !found {
			status = app.PullStatus{PR: node.Value, OriginalBase: node.OriginalBase, State: app.PullStateClean}
		}
		ok := MatchesState(status, state)
		for _, child := range node.Children {
			if walk(child) {
				ok = true
			}
		}
		if ok {
			visible[node.Value.Number] = true
		}
		return ok
	}
	for _, root := range roots {
		walk(root)
	}
	return visible
}

func flattenTree(roots []*stackedpr.Node, visible map[int]bool) []FlatNode {
	var result []FlatNode

	var walk func(node *stackedpr.Node, prefixParts []string, isLast bool, isRoot bool)
	walk = func(node *stackedpr.Node, prefixParts []string, isLast bool, isRoot bool) {
		if node == nil {
			return
		}
		if visible != nil && !visible[node.Value.Number] {
			return
		}
		var prefix string
		if isRoot {
			prefix = ""
		} else if isLast {
			prefix = strings.Join(prefixParts, "") + "└── "
		} else {
			prefix = strings.Join(prefixParts, "") + "├── "
		}

		result = append(result, FlatNode{
			Node:       node,
			Depth:      len(prefixParts),
			TreePrefix: prefix,
		})

		var childContinuation string
		if isRoot {
			childContinuation = ""
		} else if isLast {
			childContinuation = "    "
		} else {
			childContinuation = "│   "
		}

		children := visibleChildren(node.Children, visible)
		childParts := append(append([]string{}, prefixParts...), childContinuation)
		for i, child := range children {
			walk(child, childParts, i == len(children)-1, false)
		}
	}

	roots = visibleChildren(roots, visible)
	for i, root := range roots {
		walk(root, nil, i == len(roots)-1, true)
	}
	return result
}

func visibleChildren(nodes []*stackedpr.Node, visible map[int]bool) []*stackedpr.Node {
	if visible == nil {
		return nodes
	}
	children := make([]*stackedpr.Node, 0, len(nodes))
	for _, node := range nodes {
		if node != nil && visible[node.Value.Number] {
			children = append(children, node)
		}
	}
	return children
}

func (o Options) width() int {
	return o.Width
}

func (o Options) maxPreviewActions() int {
	if o.MaxPreviewActions <= 0 {
		return 3
	}
	return o.MaxPreviewActions
}

func (o Options) maxPreviewWarnings() int {
	if o.MaxPreviewWarnings <= 0 {
		return 2
	}
	return o.MaxPreviewWarnings
}

type styles struct {
	noColor      bool
	meta         lipgloss.Style
	dim          lipgloss.Style
	selectedMark lipgloss.Style
	repair       lipgloss.Style
	update       lipgloss.Style
	warning      lipgloss.Style
	clean        lipgloss.Style
	merged       lipgloss.Style
	baseBranch   lipgloss.Style
	headBranch   lipgloss.Style
}

func newStyles(opts Options) styles {
	if opts.NoColor {
		plain := lipgloss.NewStyle()
		return styles{
			noColor:      true,
			meta:         plain,
			dim:          plain,
			selectedMark: plain,
			repair:       plain,
			update:       plain,
			warning:      plain,
			clean:        plain,
			merged:       plain,
			baseBranch:   plain,
			headBranch:   plain,
		}
	}
	return styles{
		noColor:      false,
		meta:         lipgloss.NewStyle().Faint(true),
		dim:          lipgloss.NewStyle().Faint(true),
		selectedMark: lipgloss.NewStyle().Bold(true),
		repair:       lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)).Bold(true),
		update:       lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)).Bold(true),
		warning:      lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)).Bold(true),
		clean:        lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2)).Bold(true),
		merged:       lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(5)).Bold(true),
		baseBranch:   lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(6)),
		headBranch:   lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(4)),
	}
}

func (s styles) prNumber(pr gitobj.PullRequest) lipgloss.Style {
	base := lipgloss.NewStyle()
	if s.noColor {
		return base
	}
	if pr.IsDraft {
		return base.Bold(true).Foreground(lipgloss.ANSIColor(7))
	}
	switch pr.State {
	case gitobj.PullRequestStateOpen:
		return base.Bold(true).Foreground(lipgloss.ANSIColor(2))
	case gitobj.PullRequestStateClosed:
		return base.Bold(true).Foreground(lipgloss.ANSIColor(1))
	case gitobj.PullRequestStateMerged:
		return base.Bold(true).Foreground(lipgloss.ANSIColor(5))
	default:
		return base.Bold(true)
	}
}

func cursorBg(isDark bool) color.Color {
	if isDark {
		return lipgloss.Color("237")
	}
	return lipgloss.Color("254")
}
