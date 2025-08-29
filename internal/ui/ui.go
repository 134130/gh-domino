package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var logBoxStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("8"))

type Model struct {
	ctx     context.Context
	cancel  context.CancelFunc
	spinner spinner.Model
	logs    []string
	done    bool
}

func NewModel(ctx context.Context, cancel context.CancelFunc) *Model {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	return &Model{
		ctx:     ctx,
		cancel:  cancel,
		spinner: s,
		logs:    make([]string, 0),
	}
}

var _ tea.Model = (*Model)(nil)

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancel()
			m.done = true
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case LogMsg:
		m.log(string(msg))
		return m, nil
	case DoneMsg:
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) View() string {
	if m.done {
		return ""
	}
	return lipgloss.JoinVertical(
		lipgloss.Top,
		fmt.Sprintf("%s Fetching pull requests...", m.spinner.View()),
		logBoxStyle.Render(strings.Join(m.logs, "\n")),
	)
}

func (m *Model) log(msg string) {
	if len(m.logs) >= 5 {
		m.logs = m.logs[1:]
	}
	m.logs = append(m.logs, msg)
}
