package ui

import (
	"context"
	"fmt"
	"os/exec"
	"path"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/134130/gh-domino/git"
	"github.com/134130/gh-domino/internal/color"
)

var logBoxStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("8"))

const maxWidth = 80

type Model struct {
	ctx             context.Context
	cancel          context.CancelFunc
	spinner         spinner.Model
	ctxMsgHistories []string
	currCtxMsg      string
	logs            []string
}

func NewModel(ctx context.Context, cancel context.CancelFunc) *Model {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	return &Model{
		ctx:     ctx,
		cancel:  cancel,
		spinner: s,
	}
}

var _ tea.Model = (*Model)(nil)

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancel()
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) View() tea.View {
	return tea.NewView(m.String())
}

func (m *Model) SetCurrentContext(ctxMsg string) {
	if m.currCtxMsg != "" {
		m.ctxMsgHistories = append(m.ctxMsgHistories, fmt.Sprintf("%s %s", color.Grey("•"), m.currCtxMsg))
	}
	m.currCtxMsg = ctxMsg
	m.logs = nil
}

func (m *Model) Success(msg string) {
	if m.currCtxMsg != "" {
		m.ctxMsgHistories = append(m.ctxMsgHistories, fmt.Sprintf("%s %s", color.Green("✔"), msg))
		m.currCtxMsg = ""
	}
	m.logs = nil
}

func (m *Model) Failure(msg string) {
	if m.currCtxMsg != "" {
		m.ctxMsgHistories = append(m.ctxMsgHistories, fmt.Sprintf("%s %s", color.Red("✘"), msg))
		m.currCtxMsg = ""
	}
	m.logs = nil
}

func (m *Model) Log(msg string) {
	if len(m.logs) >= 5 {
		m.logs = m.logs[1:]
	}
	msg = strings.TrimSpace(msg)
	if len(msg) > maxWidth {
		msg = msg[:maxWidth-1] + "…"
	}
	m.logs = append(m.logs, msg)
}

func (m *Model) LogBytes(p []byte) {
	m.Log(string(p))
}

func (m *Model) LogWriter() WriterFunc {
	return func(p []byte) (n int, err error) {
		m.Log(string(p))
		return len(p), nil
	}
}

type WriterFunc func(p []byte) (n int, err error)

func (f WriterFunc) Write(p []byte) (n int, err error) {
	return f(p)
}

func (m *Model) CommandModifier(stdout bool) git.CommandModifier {
	return func(c *exec.Cmd) {
		args := make([]string, len(c.Args))
		copy(args, c.Args)

		if len(args) > 0 {
			args[0] = path.Base(c.Path)
		}

		m.Log(strings.Join(args, " "))
		if stdout {
			git.WithStdout(m.LogWriter())(c)
		}
	}
}

func (m *Model) String() string {
	var strs []string
	if len(m.ctxMsgHistories) > 0 {
		strs = append(strs, strings.Join(m.ctxMsgHistories, "\n"))
	}
	if m.currCtxMsg != "" {
		strs = append(strs, fmt.Sprintf("%s %s", m.spinner.View(), m.currCtxMsg))
	}
	if len(m.logs) > 0 {
		strs = append(strs, logBoxStyle.Render(strings.Join(m.logs, "\n")))
	}
	strs = append(strs, "\n")
	return strings.Join(strs, "\n")
}
