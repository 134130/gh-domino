package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/134130/gh-domino/internal/stackedpr"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PRStatus holds the pre-computed broken state for a single PR.
type PRStatus struct {
	Broken   bool
	NewBase  string
	Upstream string
}

// LoadResult is the data passed into the TUI after loading completes.
type LoadResult struct {
	Roots      []*stackedpr.Node
	Statuses   map[int]PRStatus
	PrHeadShas map[string]string
}

// LoadFunc is called async inside the TUI to fetch and compute PR data.
type LoadFunc func(ctx context.Context) (*LoadResult, error)

// SelectorResult is returned from RunSelector after the user confirms.
type SelectorResult struct {
	RebaseQueue      []stackedpr.RebaseInfo // broken PRs → local git rebase
	UpdateBranchNums []int                  // non-broken PRs → gh pr update-branch --rebase
	PrHeadShas       map[string]string
}

// FlatNode is one row in the DFS-flattened tree, used for cursor navigation.
type FlatNode struct {
	Node       *stackedpr.Node
	Depth      int
	TreePrefix string // pre-computed, e.g. "│   ├── "
}

type phase int

const (
	phaseLoading   phase = iota
	phaseSelecting
)

// Model is the bubbletea model for the PR selector TUI.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc
	loadFn LoadFunc

	phase   phase
	spinner spinner.Model
	keys    KeyMap
	loadErr error
	quit    bool

	// phaseSelecting state
	roots      []*stackedpr.Node
	statuses   map[int]PRStatus
	prHeadShas map[string]string
	flat       []FlatNode
	cursor     int
	offset     int
	selected   map[int]bool

	// set when Enter is pressed, returned to caller
	rebaseQueue      []stackedpr.RebaseInfo
	updateBranchNums []int

	width  int
	height int
}

// msgLoaded is sent by loadCmd when data is ready.
type msgLoaded struct {
	roots      []*stackedpr.Node
	statuses   map[int]PRStatus
	prHeadShas map[string]string
	flat       []FlatNode
	err        error
}

// NewModel creates an initial TUI model.
func NewModel(ctx context.Context, cancel context.CancelFunc, loadFn LoadFunc) Model {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	return Model{
		ctx:      ctx,
		cancel:   cancel,
		loadFn:   loadFn,
		phase:    phaseLoading,
		spinner:  s,
		keys:     DefaultKeyMap(),
		selected: make(map[int]bool),
	}
}

// RunSelector starts the interactive TUI and returns the user's selection.
// Returns nil if the user quit without selecting.
func RunSelector(ctx context.Context, cancel context.CancelFunc, loadFn LoadFunc) (*SelectorResult, error) {
	m := NewModel(ctx, cancel, loadFn)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	tuiModel, ok := finalModel.(Model)
	if !ok {
		return nil, fmt.Errorf("unexpected model type")
	}
	if tuiModel.quit {
		return nil, nil
	}
	if tuiModel.loadErr != nil {
		return nil, tuiModel.loadErr
	}
	return &SelectorResult{
		RebaseQueue:      tuiModel.rebaseQueue,
		UpdateBranchNums: tuiModel.updateBranchNums,
		PrHeadShas:       tuiModel.prHeadShas,
	}, nil
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.loadCmd())
}

func (m Model) loadCmd() tea.Cmd {
	return func() tea.Msg {
		result, err := m.loadFn(m.ctx)
		if err != nil {
			return msgLoaded{err: err}
		}
		flat := flattenTree(result.Roots)
		return msgLoaded{
			roots:      result.Roots,
			statuses:   result.Statuses,
			prHeadShas: result.PrHeadShas,
			flat:       flat,
		}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if key.Matches(msg, m.keys.Quit) {
			m.quit = true
			m.cancel()
			return m, tea.Quit
		}
		if m.phase == phaseSelecting {
			return m.updateSelecting(msg)
		}
		return m, nil

	case msgLoaded:
		if msg.err != nil {
			m.loadErr = msg.err
			return m, tea.Quit
		}
		m.roots = msg.roots
		m.statuses = msg.statuses
		m.prHeadShas = msg.prHeadShas
		m.flat = msg.flat
		m.phase = phaseSelecting
		return m, nil
	}

	return m, nil
}

func (m Model) updateSelecting(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		if len(m.flat) > 0 {
			pr := m.flat[m.cursor].Node.Value
			m.selected[pr.Number] = !m.selected[pr.Number]
		}

	case key.Matches(msg, m.keys.SelectAll):
		for _, fn := range m.flat {
			m.selected[fn.Node.Value.Number] = true
		}

	case key.Matches(msg, m.keys.DeselectAll):
		m.selected = make(map[int]bool)

	case key.Matches(msg, m.keys.SelectBroken):
		for _, fn := range m.flat {
			if m.statuses[fn.Node.Value.Number].Broken {
				m.selected[fn.Node.Value.Number] = true
			}
		}

	case key.Matches(msg, m.keys.Confirm):
		m.rebaseQueue, m.updateBranchNums = m.buildQueues()
		return m, tea.Quit
	}

	return m, nil
}

func (m Model) View() string {
	switch m.phase {
	case phaseLoading:
		return fmt.Sprintf("\n  %s Loading pull requests...\n", m.spinner.View())
	case phaseSelecting:
		return m.viewSelecting()
	}
	return ""
}

func (m Model) viewSelecting() string {
	if len(m.flat) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left,
			"",
			"  "+titleStyle.Render("Pull Requests — select PRs to rebase"),
			"",
			"  No stacked pull requests found.",
			"",
			statusBarStyle.Render("  q quit"),
		)
	}

	// Pre-render status bar to know its height before computing visible rows.
	// Fixed overhead: 1 (initial blank) + 2 (title + blank) + 1 (blank before bar) + 1 (trailing newline) = 5
	statusBar := m.renderStatusBar()
	statusBarLines := strings.Count(statusBar, "\n") + 1
	visibleRows := m.height - 5 - statusBarLines
	if visibleRows < 1 {
		visibleRows = 1
	}

	// Adjust scroll offset so cursor stays visible.
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+visibleRows {
		m.offset = m.cursor - visibleRows + 1
	}

	end := m.offset + visibleRows
	if end > len(m.flat) {
		end = len(m.flat)
	}

	rows := make([]string, end-m.offset)
	for i := m.offset; i < end; i++ {
		rows[i-m.offset] = m.renderRow(i)
	}

	sections := []string{"", "  " + titleStyle.Render("Pull Requests — select PRs to rebase"), ""}
	sections = append(sections, rows...)
	sections = append(sections, "", statusBar, "")
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderRow(i int) string {
	fn := m.flat[i]
	pr := fn.Node.Value
	status := m.statuses[pr.Number]

	checkbox := checkboxUnselected
	if m.selected[pr.Number] {
		checkbox = checkboxSelected
	}
	indic, indicStyle := okIndicator, okStyle
	if status.Broken {
		indic, indicStyle = brokenIndicator, brokenStyle
	}

	if i != m.cursor {
		return m.renderNormalRow(fn, checkbox, indic, indicStyle)
	}
	return m.renderCursorRow(fn, checkbox, indic, indicStyle)
}

func (m Model) renderNormalRow(fn FlatNode, checkbox, indic string, indicStyle lipgloss.Style) string {
	pr := fn.Node.Value
	prNum := prNumLipglossStyle(pr).Render(fmt.Sprintf("#%d", pr.Number))
	branchInfo := fmt.Sprintf("(%s ← %s)",
		baseBranchStyle.Render(pr.BaseRefName),
		headBranchStyle.Render(pr.HeadRefName),
	)
	origStr := ""
	if fn.Node.OriginalBase != nil {
		origNum := prNumLipglossStyle(*fn.Node.OriginalBase).Render(fmt.Sprintf("#%d", fn.Node.OriginalBase.Number))
		origStr = fmt.Sprintf(" [was on %s]", origNum)
	}
	return fmt.Sprintf("  %s %s  %s%s %s  %s%s",
		boldStyle.Render(checkbox),
		indicStyle.Render(indic),
		fn.TreePrefix,
		prNum,
		pr.Title,
		branchInfo,
		origStr,
	)
}

func (m Model) renderCursorRow(fn FlatNode, checkbox, indic string, indicStyle lipgloss.Style) string {
	pr := fn.Node.Value

	width := m.width
	if width == 0 {
		width = 80
	}

	// Each segment must carry cursorBg explicitly because inner ANSI resets (\x1b[0m)
	// from colored sub-strings clear the background mid-line.
	pl := lipgloss.NewStyle().Background(cursorBg).Bold(true)
	withBg := func(s lipgloss.Style) lipgloss.Style { return s.Background(cursorBg) }

	var b strings.Builder
	b.WriteString(pl.Render("> "))
	b.WriteString(withBg(boldStyle).Render(checkbox))
	b.WriteString(pl.Render(" "))
	b.WriteString(withBg(indicStyle).Render(indic))
	b.WriteString(pl.Render("  " + fn.TreePrefix))
	b.WriteString(withBg(prNumLipglossStyle(pr)).Render(fmt.Sprintf("#%d", pr.Number)))
	b.WriteString(pl.Render(" " + pr.Title + "  ("))
	b.WriteString(withBg(baseBranchStyle).Render(pr.BaseRefName))
	b.WriteString(pl.Render(" ← "))
	b.WriteString(withBg(headBranchStyle).Render(pr.HeadRefName))
	b.WriteString(pl.Render(")"))
	if fn.Node.OriginalBase != nil {
		b.WriteString(pl.Render(" [was on "))
		b.WriteString(withBg(prNumLipglossStyle(*fn.Node.OriginalBase)).Render(
			fmt.Sprintf("#%d", fn.Node.OriginalBase.Number),
		))
		b.WriteString(pl.Render("]"))
	}

	// Pad remaining width so the background fills the full terminal row.
	line := b.String()
	if extra := width - lipgloss.Width(line); extra > 0 {
		line += pl.Render(strings.Repeat(" ", extra))
	}
	return line
}

func (m Model) renderStatusBar() string {
	count := 0
	for _, fn := range m.flat {
		if m.selected[fn.Node.Value.Number] {
			count++
		}
	}

	dim := statusBarStyle
	key := statusBarKeyStyle

	maxWidth := m.width
	if maxWidth == 0 {
		maxWidth = 80
	}

	selPrefix := fmt.Sprintf("  %d selected  │  ", count)
	prefixWidth := lipgloss.Width(selPrefix)

	hints := []struct{ k, d string }{
		{"j/k", "move"},
		{"space", "toggle"},
		{"A", "all"},
		{"a", "none"},
		{"B", "broken"},
		{"enter", "rebase"},
		{"q", "quit"},
	}

	// Pack hints into wrapped lines, each starting at prefixWidth indent.
	type hintLine struct {
		b    strings.Builder
		used int
	}
	lines := []*hintLine{{used: prefixWidth}}

	for _, h := range hints {
		cur := lines[len(lines)-1]
		hasSep := cur.used > prefixWidth
		sepWidth := 0
		if hasSep {
			sepWidth = lipgloss.Width(" · ")
		}
		hintWidth := lipgloss.Width(h.k + " " + h.d)

		if hasSep && cur.used+sepWidth+hintWidth > maxWidth {
			// Wrap to a new line aligned with the hint area.
			lines = append(lines, &hintLine{used: prefixWidth})
			cur = lines[len(lines)-1]
			hasSep = false
		}

		if hasSep {
			cur.b.WriteString(dim.Render(" · "))
			cur.used += sepWidth
		}
		cur.b.WriteString(key.Render(h.k))
		cur.b.WriteString(dim.Render(" " + h.d))
		cur.used += hintWidth
	}

	// Render: first line prefixed with selection count, continuation lines indented.
	indent := strings.Repeat(" ", prefixWidth)
	renderedLines := make([]string, len(lines))
	for i, line := range lines {
		prefix := indent
		if i == 0 {
			prefix = dim.Render(selPrefix)
		}
		renderedLines[i] = prefix + line.b.String()
	}
	return strings.Join(renderedLines, "\n")
}

func (m Model) buildQueues() (rebase []stackedpr.RebaseInfo, update []int) {
	var walk func(node *stackedpr.Node)
	walk = func(node *stackedpr.Node) {
		pr := node.Value
		if m.selected[pr.Number] {
			status := m.statuses[pr.Number]
			if status.Broken {
				newBase := status.NewBase
				if newBase == "" {
					newBase = pr.BaseRefName
				}
				rebase = append(rebase, stackedpr.RebaseInfo{
					PR:       pr,
					NewBase:  newBase,
					Upstream: status.Upstream,
				})
			} else {
				update = append(update, pr.Number)
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	for _, root := range m.roots {
		walk(root)
	}
	return
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
