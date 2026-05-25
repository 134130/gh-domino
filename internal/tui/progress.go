package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/termrender"
	"golang.org/x/term"
)

const maxProgressLines = 5

type progressMsg struct {
	event app.ProgressEvent
	ch    <-chan app.ProgressEvent
}

type progressDoneMsg struct{}

type channelProgressSink struct {
	ch chan<- app.ProgressEvent
}

func (s channelProgressSink) Progress(event app.ProgressEvent) {
	s.ch <- event
}

func waitProgressCmd(ch <-chan app.ProgressEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return progressDoneMsg{}
		}
		return progressMsg{event: event, ch: ch}
	}
}

type progressState struct {
	title     string
	current   string
	completed []app.ProgressEvent
	activity  []string
	noColor   bool
	verbose   bool
}

func newProgressState(title string, noColor, verbose bool) progressState {
	return progressState{title: title, noColor: noColor, verbose: verbose}
}

func (s *progressState) apply(event app.ProgressEvent) {
	switch event.Kind {
	case app.ProgressStart:
		s.current = event.Message
	case app.ProgressSuccess, app.ProgressFailure, app.ProgressSkipped:
		s.current = ""
		s.completed = append(s.completed, event)
		if len(s.completed) > maxProgressLines {
			s.completed = s.completed[len(s.completed)-maxProgressLines:]
		}
	case app.ProgressLog:
		if event.Command != "" && !s.verbose {
			return
		}
		message := event.Command
		if message == "" {
			message = event.Message
		}
		if strings.TrimSpace(message) == "" {
			return
		}
		s.activity = append(s.activity, message)
		if len(s.activity) > maxProgressLines {
			s.activity = s.activity[len(s.activity)-maxProgressLines:]
		}
	}
}

func (s progressState) view(spinnerView string) string {
	var lines []string
	title := s.title
	if !s.noColor {
		title = titleStyle.Render(title)
	}
	lines = append(lines, title)
	for _, event := range s.completed {
		lines = append(lines, s.statusLine(event))
	}
	if s.current != "" {
		lines = append(lines, fmt.Sprintf("%s %s", spinnerView, s.current))
	}
	if len(s.activity) > 0 {
		for _, line := range s.activity {
			if s.noColor {
				lines = append(lines, "  "+line)
			} else {
				lines = append(lines, "  "+metaStyle.Render(line))
			}
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func (s progressState) statusLine(event app.ProgressEvent) string {
	symbol := "✔"
	style := successStyleTUI(s.noColor)
	switch event.Kind {
	case app.ProgressFailure:
		symbol = "✘"
		style = failureStyleTUI(s.noColor)
	case app.ProgressSkipped:
		symbol = "!"
		style = warningStyleTUI(s.noColor)
	}
	return fmt.Sprintf("%s %s", style.Render(symbol), event.Message)
}

type PlanLoadOptions struct {
	IncludeClean bool
	LoadPlan     PlanLoader
	Output       io.Writer
	NoColor      bool
	Verbose      bool
}

type planLoadModel struct {
	ctx          context.Context
	cancel       context.CancelFunc
	includeClean bool
	loadPlan     PlanLoader
	spinner      spinner.Model
	progress     progressState
	plan         *app.Plan
	err          error
}

func RunPlanLoader(ctx context.Context, opts PlanLoadOptions) (*app.Plan, error) {
	if opts.LoadPlan == nil {
		return nil, fmt.Errorf("plan loader is nil")
	}
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	loadCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := planLoadModel{
		ctx:          loadCtx,
		cancel:       cancel,
		includeClean: opts.IncludeClean,
		loadPlan:     opts.LoadPlan,
		spinner:      s,
		progress:     newProgressState("Loading pull requests", opts.NoColor, opts.Verbose),
	}
	programOpts := []tea.ProgramOption{}
	if opts.Output != nil {
		programOpts = append(programOpts, tea.WithOutput(opts.Output))
		if !isTerminalWriter(opts.Output) {
			programOpts = append(programOpts, tea.WithInput(nil))
		}
	}
	p := tea.NewProgram(m, programOpts...)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	loaded, ok := finalModel.(planLoadModel)
	if !ok {
		return nil, fmt.Errorf("unexpected loader model type")
	}
	return loaded.plan, loaded.err
}

func (m planLoadModel) Init() tea.Cmd {
	ch := make(chan app.ProgressEvent, 64)
	return tea.Batch(m.spinner.Tick, waitProgressCmd(ch), m.loadPlanCmd(channelProgressSink{ch: ch}, ch))
}

func (m planLoadModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.cancel()
		}
		return m, nil
	case progressMsg:
		m.progress.apply(msg.event)
		return m, waitProgressCmd(msg.ch)
	case progressDoneMsg:
		return m, nil
	case msgPlanLoaded:
		m.plan = msg.plan
		m.err = msg.err
		return m, tea.Quit
	}
	return m, nil
}

func (m planLoadModel) View() tea.View {
	v := tea.NewView(m.progress.view(m.spinner.View()))
	v.AltScreen = true
	return v
}

func (m planLoadModel) loadPlanCmd(sink app.ProgressSink, ch chan app.ProgressEvent) tea.Cmd {
	return func() tea.Msg {
		defer close(ch)
		plan, err := m.loadPlan(m.ctx, m.includeClean, sink)
		return msgPlanLoaded{
			plan:         plan,
			includeClean: m.includeClean,
			err:          err,
		}
	}
}

type ExecuteFunc func(ctx context.Context, plan *app.Plan, parallel int, progress app.ProgressSink) (*app.RunResult, error)

type ExecutionOptions struct {
	Parallel int
	Execute  ExecuteFunc
	Output   io.Writer
	NoColor  bool
	Verbose  bool
}

type actionViewState struct {
	status  string
	message string
}

type executionModel struct {
	ctx      context.Context
	cancel   context.CancelFunc
	plan     *app.Plan
	parallel int
	execute  ExecuteFunc
	spinner  spinner.Model
	progress progressState
	actions  map[string]actionViewState
	result   *app.RunResult
	err      error
	noColor  bool
	isDark   bool
}

func RunExecution(ctx context.Context, plan *app.Plan, opts ExecutionOptions) (*app.RunResult, error) {
	if plan == nil {
		return nil, fmt.Errorf("plan is nil")
	}
	if opts.Execute == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	parallel := opts.Parallel
	if parallel < 1 {
		parallel = 1
	}
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := executionModel{
		ctx:      execCtx,
		cancel:   cancel,
		plan:     plan,
		parallel: parallel,
		execute:  opts.Execute,
		spinner:  s,
		progress: newProgressState(fmt.Sprintf("Executing %d actions · parallel %d", len(plan.Actions), parallel), opts.NoColor, opts.Verbose),
		actions:  initialActionStates(plan),
		noColor:  opts.NoColor,
	}
	programOpts := []tea.ProgramOption{}
	if opts.Output != nil {
		programOpts = append(programOpts, tea.WithOutput(opts.Output))
		if !isTerminalWriter(opts.Output) {
			programOpts = append(programOpts, tea.WithInput(nil))
		}
	}
	p := tea.NewProgram(m, programOpts...)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	executed, ok := finalModel.(executionModel)
	if !ok {
		return nil, fmt.Errorf("unexpected execution model type")
	}
	return executed.result, executed.err
}

func (m executionModel) Init() tea.Cmd {
	ch := make(chan app.ProgressEvent, 128)
	return tea.Batch(tea.RequestBackgroundColor, m.spinner.Tick, waitProgressCmd(ch), m.executeCmd(channelProgressSink{ch: ch}, ch))
}

func (m executionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.cancel()
		}
		return m, nil
	case progressMsg:
		m.applyProgress(msg.event)
		return m, waitProgressCmd(msg.ch)
	case progressDoneMsg:
		return m, nil
	case msgExecutionDone:
		m.result = msg.result
		m.err = msg.err
		return m, tea.Quit
	}
	return m, nil
}

func isTerminalWriter(w io.Writer) bool {
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func (m executionModel) View() tea.View {
	var lines []string
	title := m.progress.title
	if !m.noColor {
		title = titleStyle.Render(title)
	}
	lines = append(lines, title)
	for _, action := range m.plan.Actions {
		lines = append(lines, m.actionLine(action))
	}
	if len(m.progress.activity) > 0 {
		lines = append(lines, "")
		for _, line := range m.progress.activity {
			if m.noColor {
				lines = append(lines, "  "+line)
			} else {
				lines = append(lines, "  "+metaStyle.Render(line))
			}
		}
	}
	v := tea.NewView(strings.Join(lines, "\n") + "\n")
	v.AltScreen = true
	return v
}

type msgExecutionDone struct {
	result *app.RunResult
	err    error
}

func (m executionModel) executeCmd(sink app.ProgressSink, ch chan app.ProgressEvent) tea.Cmd {
	return func() tea.Msg {
		defer close(ch)
		result, err := m.execute(m.ctx, m.plan, m.parallel, sink)
		return msgExecutionDone{result: result, err: err}
	}
}

func (m *executionModel) applyProgress(event app.ProgressEvent) {
	m.progress.apply(event)
	if event.ActionID == "" {
		return
	}
	state := m.actions[event.ActionID]
	switch event.Kind {
	case app.ProgressStart:
		state.status = "running"
		state.message = event.Message
	case app.ProgressLog:
		if event.Command != "" && !m.progress.verbose {
			return
		}
		state.message = event.Message
	case app.ProgressSuccess:
		state.status = string(app.ActionStatusSuccess)
		state.message = event.Message
	case app.ProgressFailure:
		state.status = string(app.ActionStatusFailed)
		state.message = event.Message
	case app.ProgressSkipped:
		state.status = string(app.ActionStatusSkipped)
		state.message = event.Message
	}
	m.actions[event.ActionID] = state
}

func (m executionModel) actionLine(action app.Action) string {
	state := m.actions[action.ID]
	symbol := "·"
	switch state.status {
	case "running":
		symbol = m.spinner.View()
	case string(app.ActionStatusSuccess):
		symbol = successStyleTUI(m.noColor).Render("✔")
	case string(app.ActionStatusFailed):
		symbol = failureStyleTUI(m.noColor).Render("✘")
	case string(app.ActionStatusSkipped):
		symbol = warningStyleTUI(m.noColor).Render("!")
	}
	summary := termrender.ActionSummary(action, termrender.Options{NoColor: m.noColor, IsDark: m.isDark})
	if state.message == "" {
		return fmt.Sprintf("%s %s", symbol, summary)
	}
	message := state.message
	if !m.noColor {
		message = metaStyle.Render(message)
	}
	return fmt.Sprintf("%s %s · %s", symbol, summary, message)
}

func initialActionStates(plan *app.Plan) map[string]actionViewState {
	states := make(map[string]actionViewState, len(plan.Actions))
	for _, action := range plan.Actions {
		states[action.ID] = actionViewState{status: "waiting"}
	}
	return states
}

func successStyleTUI(noColor bool) lipgloss.Style {
	if noColor {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2))
}

func failureStyleTUI(noColor bool) lipgloss.Style {
	if noColor {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1))
}

func warningStyleTUI(noColor bool) lipgloss.Style {
	if noColor {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3))
}
