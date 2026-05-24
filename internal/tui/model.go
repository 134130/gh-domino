package tui

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/stackedpr"
)

// PlanLoader rebuilds a plan when the user toggles clean update actions.
type PlanLoader func(ctx context.Context, includeClean bool) (*app.Plan, error)

// Options configures the selector model and program.
type Options struct {
	IncludeClean bool
	Parallel     int
	LoadPlan     PlanLoader
	Output       io.Writer
}

// SelectorResult is returned from RunSelector after the user confirms.
type SelectorResult struct {
	Plan     *app.Plan
	Parallel int
}

// FlatNode is one row in the DFS-flattened tree, used for cursor navigation.
type FlatNode struct {
	Node       *stackedpr.Node
	Depth      int
	TreePrefix string
}

type phase int

const (
	phaseSelecting phase = iota
	phaseLoading
)

// Model is the bubbletea model for selecting actions from an app.Plan.
type Model struct {
	ctx context.Context

	phase   phase
	spinner spinner.Model
	help    help.Model
	keys    KeyMap

	plan       *app.Plan
	preview    *app.Plan
	loadPlan   PlanLoader
	loadErr    error
	previewErr error

	flat       []FlatNode
	statusByPR map[int]app.PullStatus
	actionByPR map[int]app.Action

	mode         app.SelectionMode
	includeClean bool
	parallel     int
	selected     map[int]app.SelectionMode

	cursor int
	offset int

	confirmed bool
	quit      bool

	width  int
	height int
	isDark bool
}

type msgPlanLoaded struct {
	plan         *app.Plan
	includeClean bool
	err          error
}

// NewModel creates a TUI model from an already built plan.
func NewModel(ctx context.Context, plan *app.Plan, opts Options) Model {
	s := spinner.New()
	s.Spinner = spinner.MiniDot

	parallel := opts.Parallel
	if parallel < 1 {
		parallel = 1
	}

	m := Model{
		ctx:          ctx,
		phase:        phaseSelecting,
		spinner:      s,
		help:         help.New(),
		keys:         DefaultKeyMap(),
		loadPlan:     opts.LoadPlan,
		mode:         app.SelectNode,
		includeClean: opts.IncludeClean,
		parallel:     parallel,
		selected:     make(map[int]app.SelectionMode),
	}
	m.setPlan(plan)
	m.refreshPreview()
	return m
}

// RunSelector starts the interactive TUI and returns the selected plan.
// It returns nil when the user quits without confirmation.
func RunSelector(ctx context.Context, plan *app.Plan, opts Options) (*SelectorResult, error) {
	if plan == nil {
		return nil, fmt.Errorf("plan is nil")
	}

	m := NewModel(ctx, plan, opts)
	programOpts := []tea.ProgramOption{}
	if opts.Output != nil {
		programOpts = append(programOpts, tea.WithOutput(opts.Output))
	}
	p := tea.NewProgram(m, programOpts...)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	tuiModel, ok := finalModel.(Model)
	if !ok {
		return nil, fmt.Errorf("unexpected model type")
	}
	if tuiModel.quit || !tuiModel.confirmed {
		return nil, nil
	}
	if tuiModel.loadErr != nil {
		return nil, tuiModel.loadErr
	}
	if tuiModel.previewErr != nil {
		return nil, tuiModel.previewErr
	}
	return &SelectorResult{
		Plan:     tuiModel.preview,
		Parallel: tuiModel.parallel,
	}, nil
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tea.RequestBackgroundColor)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.help.Styles = help.DefaultStyles(m.isDark)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetWidth(msg.Width)
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.Quit) {
			m.quit = true
			return m, tea.Quit
		}
		if m.phase == phaseLoading {
			return m, nil
		}
		return m.updateSelecting(msg)

	case msgPlanLoaded:
		if msg.err != nil {
			m.loadErr = msg.err
			return m, tea.Quit
		}
		m.includeClean = msg.includeClean
		m.phase = phaseSelecting
		m.setPlan(msg.plan)
		m.refreshPreview()
		return m, nil
	}

	return m, nil
}

func (m Model) updateSelecting(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}

	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.flat)-1 {
			m.cursor++
		}

	case key.Matches(msg, m.keys.Toggle):
		m.toggleCurrent()

	case key.Matches(msg, m.keys.CycleMode):
		m.cycleMode()

	case key.Matches(msg, m.keys.ToggleAllActionable):
		m.toggleAllActionable()

	case key.Matches(msg, m.keys.ToggleClean):
		return m.toggleClean()

	case key.Matches(msg, m.keys.ParallelUp):
		m.parallel++

	case key.Matches(msg, m.keys.ParallelDown):
		if m.parallel > 1 {
			m.parallel--
		}

	case key.Matches(msg, m.keys.Confirm):
		if m.previewErr != nil {
			return m, nil
		}
		m.confirmed = true
		return m, tea.Quit
	}

	m.refreshPreview()
	return m, nil
}

func (m Model) View() tea.View {
	var content string
	switch m.phase {
	case phaseLoading:
		content = fmt.Sprintf("%s Rebuilding plan...\n", m.spinner.View())
	default:
		content = m.viewSelecting()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// Selection returns the current app selection represented by the TUI state.
func (m Model) Selection() app.Selection {
	items := make([]app.SelectionItem, 0, len(m.selected))
	for _, fn := range m.flat {
		prNumber := fn.Node.Value.Number
		mode, ok := m.selected[prNumber]
		if !ok {
			continue
		}
		items = append(items, app.SelectionItem{
			PRNumber: prNumber,
			Mode:     mode,
		})
	}
	if len(items) == 0 {
		return app.Selection{None: true}
	}
	return app.Selection{Items: items}
}

// Preview returns the currently selected plan preview.
func (m Model) Preview() *app.Plan {
	return m.preview
}

// Confirmed reports whether the user confirmed execution.
func (m Model) Confirmed() bool {
	return m.confirmed
}

// Quit reports whether the user quit without confirmation.
func (m Model) Quit() bool {
	return m.quit
}

// Parallel returns the current execution parallelism.
func (m Model) Parallel() int {
	return m.parallel
}

// IncludeClean reports whether clean update actions are currently included.
func (m Model) IncludeClean() bool {
	return m.includeClean
}

func (m *Model) setPlan(plan *app.Plan) {
	if plan == nil {
		plan = &app.Plan{}
	}
	m.plan = plan
	m.flat = flattenTree(plan.Roots)
	m.statusByPR = statusesByPR(plan.Pulls)
	m.actionByPR = actionsByPR(plan.Actions)
	if m.cursor >= len(m.flat) {
		m.cursor = max(0, len(m.flat)-1)
	}
}

func (m *Model) refreshPreview() {
	if m.plan == nil {
		m.preview = nil
		m.previewErr = fmt.Errorf("plan is nil")
		return
	}
	preview, err := m.plan.Select(m.Selection())
	m.preview = preview
	m.previewErr = err
}

func (m *Model) toggleCurrent() {
	if len(m.flat) == 0 {
		return
	}
	prNumber := m.flat[m.cursor].Node.Value.Number
	if selectedMode, ok := m.selected[prNumber]; ok && selectedMode == m.mode {
		delete(m.selected, prNumber)
		return
	}
	m.selected[prNumber] = m.mode
}

func (m *Model) cycleMode() {
	switch m.mode {
	case app.SelectNode:
		m.mode = app.SelectSubtree
	case app.SelectSubtree:
		m.mode = app.SelectChain
	default:
		m.mode = app.SelectNode
	}
}

func (m *Model) toggleAllActionable() {
	if len(m.actionByPR) == 0 {
		return
	}
	allSelected := true
	for prNumber := range m.actionByPR {
		if _, ok := m.selected[prNumber]; !ok {
			allSelected = false
			break
		}
	}
	if allSelected {
		for prNumber := range m.actionByPR {
			delete(m.selected, prNumber)
		}
		return
	}
	for prNumber := range m.actionByPR {
		m.selected[prNumber] = app.SelectNode
	}
}

func (m Model) toggleClean() (tea.Model, tea.Cmd) {
	includeClean := !m.includeClean
	if m.loadPlan == nil {
		m.includeClean = includeClean
		m.refreshPreview()
		return m, nil
	}
	m.phase = phaseLoading
	return m, m.loadPlanCmd(includeClean)
}

func (m Model) loadPlanCmd(includeClean bool) tea.Cmd {
	return func() tea.Msg {
		plan, err := m.loadPlan(m.ctx, includeClean)
		return msgPlanLoaded{
			plan:         plan,
			includeClean: includeClean,
			err:          err,
		}
	}
}

func (m Model) viewSelecting() string {
	header := m.renderHeader()
	preview := m.renderPreview()
	helpView := m.renderHelp()

	if len(m.flat) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left,
			header,
			"No pull requests found.",
			"",
			preview,
			helpView,
		)
	}

	previewLines := lineCount(preview)
	helpLines := lineCount(helpView)
	visibleRows := m.height - 2 - previewLines - helpLines
	if visibleRows < 1 {
		if m.height == 0 {
			visibleRows = len(m.flat)
		} else {
			visibleRows = 1
		}
	}

	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+visibleRows {
		m.offset = m.cursor - visibleRows + 1
	}

	end := min(m.offset+visibleRows, len(m.flat))
	rows := make([]string, 0, end-m.offset)
	for i := m.offset; i < end; i++ {
		rows = append(rows, m.renderRow(i))
	}

	sections := []string{header}
	sections = append(sections, rows...)
	sections = append(sections, "", preview, helpView)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderHeader() string {
	left := "Pull Requests"
	clean := "clean off"
	if m.includeClean {
		clean = "clean on"
	}
	right := fmt.Sprintf("mode %s · parallel %d · %s", m.mode, m.parallel, clean)

	width := m.width
	if width == 0 {
		width = 80
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		gap = 2
	}
	return titleStyle.Render(left) + strings.Repeat(" ", gap) + metaStyle.Render(right)
}

func (m Model) renderRow(i int) string {
	fn := m.flat[i]
	pr := fn.Node.Value
	status := m.statusByPR[pr.Number]
	warning, hasWarning := warningForPR(m.preview, pr.Number)
	symbol, label, style := m.rowStatus(pr.Number, hasWarning)
	cursorActive := i == m.cursor
	bg := cursorBg(m.isDark)

	plainStyle := lipgloss.NewStyle()
	if cursorActive {
		plainStyle = plainStyle.Background(bg)
	}
	plain := func(value string) string {
		if value == "" {
			return ""
		}
		return plainStyle.Render(value)
	}
	withCursorBg := func(style lipgloss.Style) lipgloss.Style {
		if cursorActive {
			return style.Background(bg)
		}
		return style
	}

	_, selected := m.selected[pr.Number]
	prefix := "  "
	switch {
	case cursorActive && selected:
		prefix = ">*"
	case cursorActive:
		prefix = "> "
	case selected:
		prefix = "* "
	}

	var row strings.Builder
	row.WriteString(withCursorBg(selectedMark).Render(prefix))
	row.WriteString(withCursorBg(style).Render(symbol))
	row.WriteString(plain(" " + fn.TreePrefix))
	row.WriteString(withCursorBg(prNumberStyle(pr)).Render(fmt.Sprintf("#%d", pr.Number)))
	row.WriteString(plain(" " + pr.Title + " ("))
	row.WriteString(withCursorBg(baseBranchStyle).Render(pr.BaseRefName))
	row.WriteString(plain(" ← "))
	row.WriteString(withCursorBg(headBranchStyle).Render(pr.HeadRefName))
	row.WriteString(plain(")"))
	row.WriteString(reasonSuffix(label, status, fn.Node.OriginalBase, warning, hasWarning, plain, withCursorBg))
	line := row.String()

	width := m.width
	if width == 0 {
		width = 80
	}
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

func (m Model) rowStatus(prNumber int, hasWarning bool) (symbol, label string, style lipgloss.Style) {
	if hasWarning {
		return "!", "WARN", warningStyle
	}
	if action, ok := m.actionByPR[prNumber]; ok {
		switch action.Kind {
		case app.ActionRepairPR:
			if action.Reason == app.ReasonParentWillChange {
				return "•", "DEPENDENT", dependentStyle
			}
			return "✘", "REPAIR", repairStyle
		case app.ActionUpdateBranch:
			return "✔︎", "UPDATE", updateStyle
		}
	}
	return "✔︎", "CLEAN", cleanStyle
}

func (m Model) renderHelp() string {
	width := m.width
	if width == 0 {
		width = 80
	}

	separator := m.help.Styles.ShortSeparator.Inline(true).Render(m.help.ShortSeparator)
	separatorWidth := lipgloss.Width(separator)

	var lines []string
	var line strings.Builder
	used := 0
	for _, binding := range m.keys.ShortHelp() {
		if !binding.Enabled() {
			continue
		}

		help := binding.Help()
		item := m.help.Styles.ShortKey.Inline(true).Render(help.Key) +
			" " +
			m.help.Styles.ShortDesc.Inline(true).Render(help.Desc)
		itemWidth := lipgloss.Width(item)

		if used > 0 && width > 0 && used+separatorWidth+itemWidth > width {
			lines = append(lines, line.String())
			line.Reset()
			used = 0
		}
		if used > 0 {
			line.WriteString(separator)
			used += separatorWidth
		}
		line.WriteString(item)
		used += itemWidth
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderPreview() string {
	if m.previewErr != nil {
		return warningStyle.Render("Preview error: " + m.previewErr.Error())
	}
	if m.preview == nil {
		return dimStyle.Render("Preview: 0 actions")
	}

	actionCount := len(m.preview.Actions)
	warningCount := len(m.preview.Warnings)
	header := fmt.Sprintf("Preview: %d actions", actionCount)
	if warningCount > 0 {
		header += fmt.Sprintf(" · %d warnings", warningCount)
	}
	lines := []string{metaStyle.Render(header)}

	const maxActions = 3
	for i, action := range m.preview.Actions {
		if i == maxActions {
			lines = append(lines, "  "+dimStyle.Render(fmt.Sprintf("… %d more actions", actionCount-maxActions)))
			break
		}
		lines = append(lines, "  "+actionSummary(action))
	}
	for i, warning := range m.preview.Warnings {
		if i == 2 {
			lines = append(lines, "  "+dimStyle.Render(fmt.Sprintf("… %d more warnings", warningCount-2)))
			break
		}
		lines = append(lines, "  "+warningStyle.Render(warningSummary(warning)))
	}
	return strings.Join(lines, "\n")
}

func actionSummary(action app.Action) string {
	pr := action.PR
	prNumber := prNumberStyle(pr).Render(fmt.Sprintf("#%d", pr.Number))
	head := headBranchStyle.Render(pr.HeadRefName)
	switch action.Kind {
	case app.ActionRepairPR:
		target := action.NewBase
		if target == "" {
			target = pr.BaseRefName
		}
		return fmt.Sprintf("%s %s %s → %s",
			repairStyle.Render("repair"),
			prNumber,
			head,
			baseBranchStyle.Render(target),
		)
	case app.ActionUpdateBranch:
		return fmt.Sprintf("%s %s %s",
			updateStyle.Render("update"),
			prNumber,
			head,
		)
	default:
		return fmt.Sprintf("%s %s %s", action.Kind, prNumber, head)
	}
}

func warningSummary(warning app.SelectionWarning) string {
	if warning.DependencyPR != 0 {
		return fmt.Sprintf("warning: #%d depends on unselected #%d", warning.PRNumber, warning.DependencyPR)
	}
	return fmt.Sprintf("warning: #%d depends on unselected %s", warning.PRNumber, warning.DependencyID)
}

func reasonSuffix(
	label string,
	status app.PullStatus,
	originalBase *gitobj.PullRequest,
	warning app.SelectionWarning,
	hasWarning bool,
	plain func(string) string,
	withCursorBg func(lipgloss.Style) lipgloss.Style,
) string {
	parts := make([]string, 0, 2)
	switch label {
	case "WARN":
		parts = append(parts, plain("warning"))
		if reason := warningReasonText(warning, hasWarning, plain, withCursorBg); reason != "" {
			parts = append(parts, reason)
		}
	case "DEPENDENT", "REPAIR", "UPDATE":
		if reason := rowReasonText(status, originalBase); reason != "" {
			parts = append(parts, plain(reason))
		}
	}
	if originalBase != nil {
		prNumber := withCursorBg(mergedStyle).Render(fmt.Sprintf("#%d", originalBase.Number))
		if status.Reason == app.ReasonMergedBase || status.Reason == app.ReasonMergedAncestor {
			parts = append(parts, fmt.Sprintf("%s%s", prNumber, plain(" was merged")))
		} else {
			parts = append(parts, fmt.Sprintf("%s%s", plain("was "), prNumber))
		}
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

func warningReasonText(
	warning app.SelectionWarning,
	ok bool,
	plain func(string) string,
	withCursorBg func(lipgloss.Style) lipgloss.Style,
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

func rowReasonText(status app.PullStatus, originalBase *gitobj.PullRequest) string {
	if originalBase != nil && (status.Reason == app.ReasonMergedBase || status.Reason == app.ReasonMergedAncestor) {
		return ""
	}
	return reasonText(status.Reason)
}

func reasonText(reason app.Reason) string {
	switch reason {
	case app.ReasonMergedBase:
		return "merged base"
	case app.ReasonParentDiverged:
		return "parent diverged"
	case app.ReasonParentWillChange:
		return "after parent repair"
	case app.ReasonMergedAncestor:
		return "merged ancestor"
	case app.ReasonRebaseAll:
		return "clean update"
	default:
		return ""
	}
}

func statusesByPR(statuses []app.PullStatus) map[int]app.PullStatus {
	out := make(map[int]app.PullStatus, len(statuses))
	for _, status := range statuses {
		out[status.PR.Number] = status
	}
	return out
}

func actionsByPR(actions []app.Action) map[int]app.Action {
	out := make(map[int]app.Action, len(actions))
	for _, action := range actions {
		out[action.PR.Number] = action
	}
	return out
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

func lineCount(value string) int {
	if value == "" {
		return 0
	}
	return strings.Count(value, "\n") + 1
}

// flattenTree converts the dependency tree into a DFS-ordered flat list
// with pre-computed tree line prefixes for rendering.
func flattenTree(roots []*stackedpr.Node) []FlatNode {
	var result []FlatNode

	var walk func(node *stackedpr.Node, prefixParts []string, isLast bool, isRoot bool)
	walk = func(node *stackedpr.Node, prefixParts []string, isLast bool, isRoot bool) {
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

		childParts := append(append([]string{}, prefixParts...), childContinuation)
		for i, child := range node.Children {
			walk(child, childParts, i == len(node.Children)-1, false)
		}
	}

	for _, root := range roots {
		walk(root, nil, false, true)
	}
	return result
}
