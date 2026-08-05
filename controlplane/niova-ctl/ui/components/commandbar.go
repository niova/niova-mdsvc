package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

type SubmitCommandMsg struct {
	Command string
}

// CommandBarModel implements a `:` command mode navigation bar. Its single-
// line View() shares one fixed-height slot with a page's own search box
// (view.go) — a command and a page search can't realistically be open at once.
type CommandBarModel struct {
	Active bool
	Input  textinput.Model
}

func NewCommandBar() CommandBarModel {
	ti := textinput.New()
	ti.Prompt = ": "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(styles.ColorPrimary).Bold(true)
	ti.Placeholder = "pdu, rack, hv, dev, part, nisd, vdev, pfs, tree, login, users, tenant-login, tenants, authz, quit"
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(styles.ColorMuted)
	ti.CharLimit = 64
	ti.Width = 100
	ti.Focus()

	return CommandBarModel{Active: false, Input: ti}
}

func (m CommandBarModel) Update(msg tea.Msg) (CommandBarModel, tea.Cmd) {
	if !m.Active {
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.Active = false
			m.Input.SetValue("")
			return m, nil

		case "enter":
			cmdStr := strings.TrimSpace(strings.TrimPrefix(m.Input.Value(), ":"))
			m.Active = false
			m.Input.SetValue("")
			return m, func() tea.Msg {
				return SubmitCommandMsg{Command: cmdStr}
			}
		}
	}

	var cmd tea.Cmd
	m.Input, cmd = m.Input.Update(msg)
	return m, cmd
}

func (m CommandBarModel) View() string {
	if !m.Active {
		return ""
	}
	return m.Input.View()
}
