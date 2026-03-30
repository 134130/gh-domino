package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all keybindings for the TUI.
type KeyMap struct {
	Up           key.Binding
	Down         key.Binding
	Toggle       key.Binding
	SelectAll    key.Binding
	DeselectAll  key.Binding
	SelectBroken key.Binding
	Confirm      key.Binding
	Quit         key.Binding
}

// DefaultKeyMap returns the default keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:           key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:         key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Toggle:       key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "toggle")),
		SelectAll:    key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "select all")),
		DeselectAll:  key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "deselect all")),
		SelectBroken: key.NewBinding(key.WithKeys("B"), key.WithHelp("B", "select broken")),
		Confirm:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "rebase selected")),
		Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}
