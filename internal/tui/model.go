package tui

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/stackedpr"
	"github.com/134130/gh-domino/internal/termrender"
)

// PlanLoader loads a plan while reporting progress.
type PlanLoader func(ctx context.Context, includeClean bool, progress app.ProgressSink) (*app.Plan, error)

// Options configures the selector model and program.
type Options struct {
	IncludeClean bool
	Parallel     int
	Output       io.Writer
	NoColor      bool
}

// SelectorResult is returned from RunSelector after the user confirms.
type SelectorResult struct {
	Plan     *app.Plan
	Parallel int
}

// Model is the bubbletea model for selecting actions from an app.Plan.
type Model struct {
	ctx context.Context

	help help.Model
	keys KeyMap

	plan       *app.Plan
	preview    *app.Plan
	previewErr error

	flat       []termrender.FlatNode
	repairByPR map[int]bool
	updateByPR map[int]bool
	parentByPR map[int]int

	mode         app.SelectionMode
	includeClean bool
	noColor      bool
	parallel     int
	selected     map[int]bool

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
	parallel := opts.Parallel
	if parallel < 1 {
		parallel = 1
	}

	m := Model{
		ctx:          ctx,
		help:         help.New(),
		keys:         DefaultKeyMap(),
		mode:         app.SelectNode,
		includeClean: opts.IncludeClean,
		noColor:      opts.NoColor,
		parallel:     parallel,
		selected:     make(map[int]bool),
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
	programOpts := []tea.ProgramOption{tea.WithContext(ctx)}
	if opts.Output != nil {
		programOpts = append(programOpts, tea.WithOutput(opts.Output))
	}
	p := tea.NewProgram(m, programOpts...)
	finalModel, err := p.Run()
	if err != nil {
		if interrupted := interruptedProgramErr(ctx, err); interrupted != nil {
			return nil, interrupted
		}
		return nil, err
	}
	tuiModel, ok := finalModel.(Model)
	if !ok {
		return nil, fmt.Errorf("unexpected model type")
	}
	if tuiModel.quit || !tuiModel.confirmed {
		return nil, nil
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
	return tea.RequestBackgroundColor
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

	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.Quit) {
			m.quit = true
			return m, tea.Quit
		}
		return m.updateSelecting(msg)
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
	v := tea.NewView(m.viewSelecting())
	v.AltScreen = true
	return v
}

// Selection returns the current app selection represented by the TUI state.
func (m Model) Selection() app.Selection {
	items := make([]app.SelectionItem, 0, len(m.selected))
	for _, fn := range m.flat {
		prNumber := fn.Node.Value.Number
		if !m.selected[prNumber] {
			continue
		}
		items = append(items, app.SelectionItem{
			PRNumber: prNumber,
			Mode:     app.SelectNode,
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
	m.flat = termrender.FlattenTree(plan.Roots)
	m.repairByPR = repairablePRs(plan)
	m.updateByPR = updateablePRs(plan)
	m.parentByPR = termrender.ParentsByPR(plan.Roots)
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

	targets := m.currentToggleTargets()
	allSelected := len(targets) > 0
	for _, prNumber := range targets {
		if !m.selected[prNumber] {
			allSelected = false
			break
		}
	}
	if allSelected {
		for _, prNumber := range targets {
			delete(m.selected, prNumber)
		}
		return
	}
	for _, prNumber := range targets {
		m.selected[prNumber] = true
	}
}

func (m Model) currentToggleTargets() []int {
	if len(m.flat) == 0 {
		return nil
	}
	node := m.flat[m.cursor].Node
	switch m.mode {
	case app.SelectSubtree:
		var targets []int
		collectSubtreeNumbers(node, &targets)
		return targets
	case app.SelectChain:
		var targets []int
		for current := node.Value.Number; current != 0; current = m.parentByPR[current] {
			targets = append(targets, current)
		}
		slices.Reverse(targets)
		return targets
	default:
		return []int{node.Value.Number}
	}
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
	repairs := m.repairTargets()
	updates := m.updateTargets()
	all := mergeTargets(repairs, updates)
	if len(all) == 0 {
		return
	}

	switch {
	case allSelected(m.selected, all):
		m.selected = make(map[int]bool)
	case len(updates) > 0 && len(repairs) > 0 && allSelected(m.selected, repairs):
		m.setSelectedTargets(all)
	case len(repairs) > 0:
		m.setSelectedTargets(repairs)
	default:
		m.setSelectedTargets(all)
	}
}

func (m Model) repairTargets() []int {
	return m.targetsMatching(func(prNumber int) bool {
		return m.repairByPR[prNumber]
	})
}

func (m Model) updateTargets() []int {
	return m.targetsMatching(func(prNumber int) bool {
		return m.updateByPR[prNumber] && !m.repairByPR[prNumber]
	})
}

func (m Model) targetsMatching(match func(int) bool) []int {
	seen := map[int]struct{}{}
	targets := make([]int, 0)
	for _, fn := range m.flat {
		if fn.Node == nil {
			continue
		}
		prNumber := fn.Node.Value.Number
		if !match(prNumber) {
			continue
		}
		targets = append(targets, prNumber)
		seen[prNumber] = struct{}{}
	}
	for prNumber := range m.repairByPR {
		if _, ok := seen[prNumber]; ok || !match(prNumber) {
			continue
		}
		targets = append(targets, prNumber)
		seen[prNumber] = struct{}{}
	}
	for prNumber := range m.updateByPR {
		if _, ok := seen[prNumber]; ok || !match(prNumber) {
			continue
		}
		targets = append(targets, prNumber)
		seen[prNumber] = struct{}{}
	}
	return targets
}

func (m *Model) setSelectedTargets(targets []int) {
	m.selected = make(map[int]bool, len(targets))
	for _, prNumber := range targets {
		m.selected[prNumber] = true
	}
}

func (m Model) toggleClean() (tea.Model, tea.Cmd) {
	includeClean := !m.includeClean
	m.includeClean = includeClean
	for _, prNumber := range m.updateTargets() {
		if includeClean {
			m.selected[prNumber] = true
			continue
		}
		delete(m.selected, prNumber)
	}
	m.refreshPreview()
	return m, nil
}

func allSelected(selected map[int]bool, targets []int) bool {
	if len(targets) == 0 {
		return false
	}
	for _, prNumber := range targets {
		if !selected[prNumber] {
			return false
		}
	}
	return true
}

func mergeTargets(groups ...[]int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0)
	for _, group := range groups {
		for _, prNumber := range group {
			if _, ok := seen[prNumber]; ok {
				continue
			}
			out = append(out, prNumber)
			seen[prNumber] = struct{}{}
		}
	}
	return out
}

func repairablePRs(plan *app.Plan) map[int]bool {
	out := map[int]bool{}
	if plan == nil {
		return out
	}
	for _, action := range plan.Actions {
		if action.Kind == app.ActionRepairPR {
			out[action.PR.Number] = true
		}
	}
	return out
}

func updateablePRs(plan *app.Plan) map[int]bool {
	out := map[int]bool{}
	if plan == nil {
		return out
	}
	for _, status := range plan.Pulls {
		if status.State == app.PullStateUpdateable {
			out[status.PR.Number] = true
		}
	}
	for _, action := range plan.Actions {
		if action.Kind == app.ActionUpdateBranch {
			out[action.PR.Number] = true
		}
	}
	return out
}

func (m Model) viewSelecting() string {
	header := m.renderHeader()
	helpView := m.renderHelp()
	preview := m.renderPreviewWithOptions(m.previewLimits(lineCount(helpView)))

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
	clean := "stale off"
	if m.includeClean {
		clean = "stale on"
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
	if m.noColor {
		return left + strings.Repeat(" ", gap) + right
	}
	return titleStyle.Render(left) + strings.Repeat(" ", gap) + metaStyle.Render(right)
}

func (m Model) renderRow(i int) string {
	fn := m.flat[i]
	pr := fn.Node.Value
	_, selected := m.selected[pr.Number]
	ctx := termrender.NewContext(m.plan, m.preview, termrender.RowModePlan, termrender.Options{
		Width:   m.width,
		NoColor: m.noColor,
		IsDark:  m.isDark,
	})
	return ctx.RenderRow(fn, termrender.RowState{
		Selected: selected,
		Cursor:   i == m.cursor,
	})
}

func (m Model) renderHelp() string {
	width := m.width
	if width == 0 {
		width = 80
	}

	separator := m.help.Styles.ShortSeparator.Inline(true).Render(m.help.ShortSeparator)
	if m.noColor {
		separator = m.help.ShortSeparator
	}
	separatorWidth := lipgloss.Width(separator)

	var lines []string
	var line strings.Builder
	used := 0
	for _, binding := range m.keys.ShortHelp() {
		if !binding.Enabled() {
			continue
		}

		help := binding.Help()
		item := help.Key + " " + help.Desc
		if !m.noColor {
			item = m.help.Styles.ShortKey.Inline(true).Render(help.Key) +
				" " +
				m.help.Styles.ShortDesc.Inline(true).Render(help.Desc)
		}
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

func (m Model) renderPreviewWithOptions(opts termrender.Options) string {
	if m.previewErr != nil {
		if m.noColor {
			return "Preview error: " + m.previewErr.Error()
		}
		return warningStyle.Render("Preview error: " + m.previewErr.Error())
	}
	opts.Width = m.width
	opts.NoColor = m.noColor
	opts.IsDark = m.isDark
	return termrender.RenderPreview(m.preview, opts)
}

func (m Model) previewLimits(helpLines int) termrender.Options {
	if m.height == 0 || m.preview == nil {
		return termrender.Options{}
	}

	actionCount := len(m.preview.Actions)
	warningCount := len(m.preview.Warnings)
	fullPreview := termrender.RenderPreview(m.preview, termrender.Options{
		Width:              m.width,
		NoColor:            m.noColor,
		IsDark:             m.isDark,
		MaxPreviewActions:  max(1, actionCount),
		MaxPreviewWarnings: max(1, warningCount),
	})
	availablePreviewLines := m.height - 2 - len(m.flat) - helpLines
	if availablePreviewLines >= lineCount(fullPreview) {
		return termrender.Options{
			MaxPreviewActions:  max(1, actionCount),
			MaxPreviewWarnings: max(1, warningCount),
		}
	}
	return termrender.Options{}
}

func collectSubtreeNumbers(node *stackedpr.Node, out *[]int) {
	if node == nil {
		return
	}
	*out = append(*out, node.Value.Number)
	for _, child := range node.Children {
		collectSubtreeNumbers(child, out)
	}
}

func lineCount(value string) int {
	if value == "" {
		return 0
	}
	return strings.Count(value, "\n") + 1
}
