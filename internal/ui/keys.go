package ui

import "charm.land/bubbles/v2/key"

// KeyMap holds every dashboard binding. Handlers match against it with
// key.Matches and the help bubble renders the footer and the Help overlay from
// it, so a binding can never be advertised without being handled.
type KeyMap struct {
	Refresh  key.Binding
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding
	Help     key.Binding
	Quit     key.Binding
	Escape   key.Binding
}

// NewKeyMap builds the default dashboard bindings.
func NewKeyMap() KeyMap {
	return KeyMap{
		Refresh:  key.NewBinding(key.WithKeys("r"), key.WithHelp("[r]", "refresh")),
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("[↑/k]", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("[↑↓/j/k]", "scroll")),
		PageUp:   key.NewBinding(key.WithKeys("pgup"), key.WithHelp("[pgup]", "page up")),
		PageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("[pgup/pgdn]", "page")),
		Home:     key.NewBinding(key.WithKeys("home"), key.WithHelp("[home]", "top")),
		End:      key.NewBinding(key.WithKeys("end"), key.WithHelp("[end]", "bottom")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("[?]", "help")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("[q]", "quit")),
		Escape:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("[esc]", "quit")),
	}
}

// ShortHelp is the footer, in render order.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Refresh, k.Down, k.PageDown, k.Help, k.Quit}
}

// FullHelp is the grouped list shown inside the Help overlay.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Refresh, k.Help},
		{k.Down, k.PageDown},
		{k.Home, k.End},
		{k.Quit, k.Escape},
	}
}

// HelpKeyMap describes the keys available while the Help overlay is open.
type HelpKeyMap struct {
	Scroll key.Binding
	Page   key.Binding
	Close  key.Binding
	Quit   key.Binding
}

// NewHelpKeyMap builds the contextual footer shown beneath the Help overlay.
func NewHelpKeyMap() HelpKeyMap {
	return HelpKeyMap{
		Scroll: key.NewBinding(key.WithKeys("up", "k", "down", "j"), key.WithHelp("[↑↓/j/k]", "scroll")),
		Page:   key.NewBinding(key.WithKeys("pgup", "pgdown", "home", "end"), key.WithHelp("[pgup/pgdn]", "page")),
		Close:  key.NewBinding(key.WithKeys("esc", "?", "q"), key.WithHelp("[esc/?/q]", "close")),
		Quit:   key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("[ctrl+c]", "quit")),
	}
}

// ShortHelp is the one-line overlay footer.
func (k HelpKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Scroll, k.Page, k.Close, k.Quit}
}

// FullHelp satisfies help.KeyMap.
func (k HelpKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }
