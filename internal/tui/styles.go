package tui

import (
	"github.com/134130/gh-domino/gitobj"
	"github.com/charmbracelet/lipgloss"
)

var (
	cursorBg        = lipgloss.Color("237")
	titleStyle      = lipgloss.NewStyle().Bold(true)
	statusBarStyle    = lipgloss.NewStyle()
	statusBarKeyStyle = lipgloss.NewStyle().Bold(true)
	brokenStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true) // ANSI yellow + bold
	okStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true) // ANSI green + bold
	boldStyle       = lipgloss.NewStyle().Bold(true)
	baseBranchStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(6)) // cyan (matches color.Cyan)
	headBranchStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(4)) // blue (matches color.Blue)
)

const (
	checkboxSelected   = "■"
	checkboxUnselected = "□"
	brokenIndicator    = "!" // rendered with brokenStyle
	okIndicator        = "✔" // rendered with okStyle
)

// prNumLipglossStyle returns the bold lipgloss style for a PR number based on its state.
// Callers can chain .Background(...) for cursor rows.
func prNumLipglossStyle(pr gitobj.PullRequest) lipgloss.Style {
	if pr.IsDraft {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(7)) // grey
	}
	switch pr.State {
	case gitobj.PullRequestStateOpen:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(2)) // green
	case gitobj.PullRequestStateClosed:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(1)) // red
	case gitobj.PullRequestStateMerged:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(5)) // purple
	default:
		return lipgloss.NewStyle().Bold(true)
	}
}
