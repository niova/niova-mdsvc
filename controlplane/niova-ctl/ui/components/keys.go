package components

import "github.com/charmbracelet/bubbles/key"

// GlobalKeys are the shortcuts that work regardless of active page (see the
// tea.KeyMsg switch in ui/App.Update) — the single source of truth for help
// rendering, rather than a hand-duplicated cheat sheet that could drift.
var GlobalKeys = []key.Binding{
	key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"), key.WithHelp("1-9", "jump to view")),
	key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "previous tab")),
	key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next tab")),
	key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "jump to PDUs tab")),
	key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "jump to Racks tab")),
	key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "jump to NISDs tab")),
	key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "jump to Vdevs tab")),
	key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "jump to PFS tab")),
	key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "jump to Users view")),
	key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "jump to Tenants view (mdsvc-tidb)")),
	key.NewBinding(key.WithKeys("z"), key.WithHelp("z", "jump to Authz Policies view (mdsvc-tidb)")),
	key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add/create in current view")),
	key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit selected item")),
	key.NewBinding(key.WithKeys("ctrl+r", "f5"), key.WithHelp("ctrl+r", "refresh topology & metrics")),
	key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "open command mode")),
	key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "toggle this help")),
	key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back / close")),
	key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
}
