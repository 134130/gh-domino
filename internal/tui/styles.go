package tui

import "charm.land/lipgloss/v2"

var (
	titleStyle = lipgloss.NewStyle().Bold(true)
	metaStyle  = lipgloss.NewStyle().Faint(true)

	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)).Bold(true)
)
