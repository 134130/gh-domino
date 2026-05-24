package tui

import "charm.land/bubbles/v2/key"

// KeyMap defines all keybindings for the TUI.
type KeyMap struct {
	Up                  key.Binding
	Down                key.Binding
	Toggle              key.Binding
	CycleMode           key.Binding
	ToggleAllActionable key.Binding
	ToggleClean         key.Binding
	ParallelUp          key.Binding
	ParallelDown        key.Binding
	Confirm             key.Binding
	Quit                key.Binding
}

// DefaultKeyMap returns the default keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/up", "move"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/down", "move"),
		),
		Toggle: key.NewBinding(
			key.WithKeys(" ", "space"),
			key.WithHelp("space", "toggle"),
		),
		CycleMode: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "mode"),
		),
		ToggleAllActionable: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "all"),
		),
		ToggleClean: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "clean"),
		),
		ParallelUp: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "parallel+"),
		),
		ParallelDown: key.NewBinding(
			key.WithKeys("P"),
			key.WithHelp("P", "parallel-"),
		),
		Confirm: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "run"),
		),
		Quit: key.NewBinding(
			key.WithKeys("esc", "q", "ctrl+c"),
			key.WithHelp("q/esc", "quit"),
		),
	}
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Up,
		k.Down,
		k.Toggle,
		k.CycleMode,
		k.ToggleAllActionable,
		k.ToggleClean,
		k.ParallelUp,
		k.ParallelDown,
		k.Confirm,
		k.Quit,
	}
}

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Toggle, k.CycleMode},
		{k.ToggleAllActionable, k.ToggleClean, k.ParallelUp, k.ParallelDown},
		{k.Confirm, k.Quit},
	}
}
