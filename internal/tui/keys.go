package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Quit       key.Binding
	Tab        key.Binding
	ViewEvents key.Binding
	ViewStats  key.Binding
	ViewAppr   key.Binding
	Up         key.Binding
	Down       key.Binding
	Enter      key.Binding
	Escape     key.Binding
	Search     key.Binding
	FilterDec  key.Binding
	PageUp     key.Binding
	PageDown   key.Binding
	Home       key.Binding
	End        key.Binding
}

var keys = keyMap{
	Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Tab:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "view")),
	ViewEvents: key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "events")),
	ViewStats:  key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "stats")),
	ViewAppr:   key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "approvals")),
	Up:         key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
	Down:       key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
	Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "detail")),
	Escape:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
	Search:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	FilterDec:  key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "filter")),
	PageUp:     key.NewBinding(key.WithKeys("pgup")),
	PageDown:   key.NewBinding(key.WithKeys("pgdown")),
	Home:       key.NewBinding(key.WithKeys("g")),
	End:        key.NewBinding(key.WithKeys("G")),
}

// footerHelp returns the help text for the footer bar.
func footerHelp() string {
	return "q quit  tab view  j/k nav  / search  enter detail  d filter"
}
