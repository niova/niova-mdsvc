package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// commandModeShortcuts documents ":"-prefixed commands, typed into the
// CommandBar rather than pressed directly — they aren't key.Bindings, so
// they can't be folded into GlobalKeys, but the list here should stay in
// sync with the switch in App.handleCommandSubmit.
var commandModeShortcuts = [][2]string{
	{"pdu", "Jump to Power Distribution Units"},
	{"rack", "Jump to Server Racks"},
	{"hv / hypervisor", "Jump to Hypervisors"},
	{"dev / disk", "Jump to Physical Storage Devices"},
	{"part", "Jump to Device Partitions"},
	{"nisd", "Jump to NISD Service Daemons"},
	{"vdev", "Jump to Virtual Device Pools"},
	{"pfs", "Jump to Parallel File Systems"},
	{"tree / topo", "Jump to Hierarchical Topology Tree"},
	{"login", "Login (or Switch to a Different User)"},
	{"users", "List Registered Users (Admin Only)"},
	{"tenant-login", "Login as Tenant-Admin (mdsvc-tidb Only)"},
	{"tenants", "List Tenants (mdsvc-tidb Only)"},
	{"authz", "RBAC/ABAC Policies (mdsvc-tidb Only)"},
	{"quit / q / exit", "Quit Application"},
}

// HelpModalModel implements a ? hotkey cheat sheet modal overlay. It renders
// from GlobalKeys plus whichever page is currently active, so the cheat
// sheet can never drift from what a key actually does — there is no second,
// hand-maintained copy of this list to fall out of sync.
type HelpModalModel struct {
	Active bool
	help   help.Model
}

func NewHelpModal() HelpModalModel {
	h := help.New()
	h.Styles.FullKey = lipgloss.NewStyle().Bold(true).Foreground(styles.ColorPrimary)
	h.Styles.FullDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	h.Styles.FullSeparator = lipgloss.NewStyle().Foreground(styles.ColorMuted)
	return HelpModalModel{Active: false, help: h}
}

func (m HelpModalModel) Update(msg tea.Msg) (HelpModalModel, tea.Cmd) {
	if !m.Active {
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc", "q", "?", "enter", "space":
			m.Active = false
			return m, nil
		}
	}
	return m, nil
}

// helpKeyMap adapts (global bindings, current-page bindings) into the
// column groups bubbles/help expects: each entry in FullHelp() renders as
// one column.
type helpKeyMap struct {
	global []key.Binding
	page   []key.Binding
}

func (k helpKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.global, k.page} }

// View renders the cheat sheet for whichever page is currently active.
// pageTitle and pageBindings come straight from App.ActivePage — the same
// values already feeding the footer and header keymap hints.
func (m HelpModalModel) View(pageTitle string, pageBindings []key.Binding) string {
	if !m.Active {
		return ""
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorSecondary).
		Padding(1, 2).
		Background(styles.ColorCardBg)

	title := styles.TextBold.Render("💡 Niova CTL — Keyboard Shortcuts")
	subtitle := styles.TextMuted.Render("Global shortcuts, plus shortcuts for the current view: " + pageTitle)

	km := helpKeyMap{global: GlobalKeys, page: pageBindings}
	body := m.help.FullHelpView(km.FullHelp())

	cmdCellKey := lipgloss.NewStyle().Bold(true).Foreground(styles.ColorPrimary).Width(20)
	cmdCellDesc := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	var cmdRows []string
	for _, c := range commandModeShortcuts {
		cmdRows = append(cmdRows, lipgloss.JoinHorizontal(lipgloss.Left, cmdCellKey.Render(c[0]), cmdCellDesc.Render(c[1])))
	}
	cmdSection := lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render("Command Mode (:)"),
		strings.Join(cmdRows, "\n"),
	)

	footer := styles.TextMuted.Render("[ Press Esc, ?, or Enter to close this help guide ]")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		body,
		"",
		cmdSection,
		"",
		footer,
	)

	return boxStyle.Render(content)
}
