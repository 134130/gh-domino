package color

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	bold          = lipgloss.NewStyle().Bold(true)
	strikethrough = lipgloss.NewStyle().Strikethrough(true)
	red           = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1))
	green         = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2))
	yellow        = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3))
	blue          = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(4))
	purple        = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(5))
	cyan          = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(6))
	grey          = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(7))
)

func Bold(a ...interface{}) string {
	return bold.Render(fmt.Sprint(a...))
}

func Strikethrough(a ...interface{}) string {
	return strikethrough.Render(fmt.Sprint(a...))
}

func Blue(a ...interface{}) string {
	return blue.Render(fmt.Sprint(a...))
}

func Cyan(a ...interface{}) string {
	return cyan.Render(fmt.Sprint(a...))
}

func Green(a ...interface{}) string {
	return green.Render(fmt.Sprint(a...))
}

func Red(a ...interface{}) string {
	return red.Render(fmt.Sprint(a...))
}

func Yellow(a ...interface{}) string {
	return yellow.Render(fmt.Sprint(a...))
}

func Purple(a ...interface{}) string {
	return purple.Render(fmt.Sprint(a...))
}

func Grey(a ...interface{}) string {
	return grey.Render(fmt.Sprint(a...))
}
