package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/134130/gh-domino/gitobj"
)

func cursorBg(isDark bool) color.Color {
	if isDark {
		return lipgloss.Color("237")
	}
	return lipgloss.Color("254")
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true)
	metaStyle    = lipgloss.NewStyle().Faint(true)
	dimStyle     = lipgloss.NewStyle().Faint(true)
	selectedMark = lipgloss.NewStyle().Bold(true)

	repairStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)).Bold(true)
	updateStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)).Bold(true)
	warningStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)).Bold(true)
	cleanStyle   = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2)).Bold(true)
	mergedStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(5)).Bold(true)

	baseBranchStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(6))
	headBranchStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(4))
)

func prNumberStyle(pr gitobj.PullRequest) lipgloss.Style {
	if pr.IsDraft {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(7))
	}
	switch pr.State {
	case gitobj.PullRequestStateOpen:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(2))
	case gitobj.PullRequestStateClosed:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(1))
	case gitobj.PullRequestStateMerged:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(5))
	default:
		return lipgloss.NewStyle().Bold(true)
	}
}
