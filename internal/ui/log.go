package ui

import (
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

type LogWriter struct {
	m *Model
	p *tea.Program
}

func NewLogWriter(m *Model, p *tea.Program) *LogWriter {
	return &LogWriter{m: m, p: p}
}

var _ io.Writer = (*LogWriter)(nil)
var _ io.StringWriter = (*LogWriter)(nil)

func (w LogWriter) Write(p []byte) (n int, err error) {
	w.p.Send(LogMsg(p))
	return len(p), nil
}

func (w LogWriter) WriteString(s string) (n int, err error) {
	w.p.Send(LogMsg(s))
	return len(s), nil
}
