package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
	"github.com/134130/gh-domino/internal/app"
	"golang.org/x/term"
)

type terminalProgress struct {
	w       io.Writer
	enabled bool
	noColor bool
	verbose bool

	done chan struct{}
	once sync.Once

	mu        sync.Mutex
	frame     int
	current   string
	lastWidth int
}

func newTerminalProgress(w io.Writer, noColor, verbose bool) *terminalProgress {
	p := &terminalProgress{
		w:       w,
		enabled: isTerminalWriter(w),
		noColor: noColor,
		verbose: verbose,
		done:    make(chan struct{}),
	}
	if p.enabled {
		go p.run()
	}
	return p
}

func (p *terminalProgress) Progress(event app.ProgressEvent) {
	if !p.enabled {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	switch event.Kind {
	case app.ProgressStart:
		p.current = event.Message
		p.renderLocked()
	case app.ProgressSuccess:
		p.clearLocked()
		p.current = ""
		p.printlnLocked(successStyle(p.noColor).Render("✔") + " " + event.Message)
	case app.ProgressFailure:
		p.clearLocked()
		p.current = ""
		p.printlnLocked(failureStyle(p.noColor).Render("✘") + " " + event.Message)
	case app.ProgressSkipped:
		p.clearLocked()
		p.current = ""
		p.printlnLocked(warningStyleCLI(p.noColor).Render("!") + " " + event.Message)
	case app.ProgressLog:
		if !p.verbose {
			return
		}
		message := event.Command
		if message == "" {
			message = event.Message
		}
		if strings.TrimSpace(message) == "" {
			return
		}
		p.clearLocked()
		p.printlnLocked(dimStyle(p.noColor).Render("• " + message))
		p.renderLocked()
	}
}

func (p *terminalProgress) Close() {
	if p == nil || !p.enabled {
		return
	}
	p.once.Do(func() {
		close(p.done)
		p.mu.Lock()
		defer p.mu.Unlock()
		p.clearLocked()
	})
}

func (p *terminalProgress) run() {
	ticker := time.NewTicker(spinner.MiniDot.FPS)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			if p.current != "" {
				p.frame = (p.frame + 1) % len(spinner.MiniDot.Frames)
				p.renderLocked()
			}
			p.mu.Unlock()
		case <-p.done:
			return
		}
	}
}

func (p *terminalProgress) renderLocked() {
	if p.current == "" {
		return
	}
	frame := spinner.MiniDot.Frames[p.frame]
	line := frame + " " + p.current
	if !p.noColor {
		line = progressStyle.Render(frame) + " " + p.current
	}
	width := lipgloss.Width(line)
	padding := ""
	if p.lastWidth > width {
		padding = strings.Repeat(" ", p.lastWidth-width)
	}
	_, _ = fmt.Fprintf(p.w, "\r%s%s", line, padding)
	p.lastWidth = width
}

func (p *terminalProgress) clearLocked() {
	if p.lastWidth == 0 {
		return
	}
	_, _ = fmt.Fprintf(p.w, "\r%s\r", strings.Repeat(" ", p.lastWidth))
	p.lastWidth = 0
}

func (p *terminalProgress) printlnLocked(line string) {
	_, _ = fmt.Fprintln(p.w, line)
}

func isTerminalWriter(w io.Writer) bool {
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

var progressStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(6))

func successStyle(noColor bool) lipgloss.Style {
	if noColor {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2))
}

func failureStyle(noColor bool) lipgloss.Style {
	if noColor {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1))
}

func warningStyleCLI(noColor bool) lipgloss.Style {
	if noColor {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3))
}

func dimStyle(noColor bool) lipgloss.Style {
	if noColor {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Faint(true)
}
