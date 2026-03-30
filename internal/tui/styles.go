package tui

import "github.com/charmbracelet/lipgloss"

var (
	cursorRowStyle = lipgloss.NewStyle().Background(lipgloss.Color("237")).Bold(true)
	titleStyle     = lipgloss.NewStyle().Bold(true)
	statusBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	brokenStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // ANSI yellow
	okStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // ANSI green
)

const (
	checkboxSelected   = "■"
	checkboxUnselected = "□"
	brokenIndicator    = "!" // rendered with brokenStyle
	okIndicator        = "✓" // rendered with okStyle
)
