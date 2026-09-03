package pages

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// LoginMode identifies which pre-login flow the operator picked from
// LoginChoicePage.
type LoginMode int

const (
	LoginModeUser LoginMode = iota
	LoginModeTenantAdmin
	LoginModeCreateAdmin
)

// SelectLoginModeMsg is emitted when the user picks a login mode from the
// landing chooser.
type SelectLoginModeMsg struct {
	Mode LoginMode
}

// LoginChoicePage is the landing screen before any session exists. Its two
// options depend on backend: mdsvc-tidb offers Admin/User login vs Tenant
// Administrator login; niova-mdsvc has no default admin, so it offers
// Login vs Create Admin User instead.
type LoginChoicePage struct {
	Backend domain.BackendKind
	Cursor  int
}

func NewLoginChoicePage(backend domain.BackendKind) *LoginChoicePage {
	return &LoginChoicePage{Backend: backend}
}

func (p *LoginChoicePage) isFormPage() {}

func (p *LoginChoicePage) Init() tea.Cmd { return nil }

// modes returns this backend's two options in display order — index must
// stay in sync with optionLabels/helpLines below.
func (p *LoginChoicePage) modes() [2]LoginMode {
	if p.Backend == domain.BackendMdsvcTidb {
		return [2]LoginMode{LoginModeUser, LoginModeTenantAdmin}
	}
	return [2]LoginMode{LoginModeUser, LoginModeCreateAdmin}
}

func (p *LoginChoicePage) Update(msg tea.Msg) (Page, tea.Cmd) {
	modes := p.modes()
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if p.Cursor > 0 {
				p.Cursor--
			}
		case "down", "j":
			if p.Cursor < 1 {
				p.Cursor++
			}
		case "1":
			return p, func() tea.Msg { return SelectLoginModeMsg{Mode: modes[0]} }
		case "2":
			return p, func() tea.Msg { return SelectLoginModeMsg{Mode: modes[1]} }
		case "enter":
			return p, func() tea.Msg { return SelectLoginModeMsg{Mode: modes[p.Cursor]} }
		}
	}
	return p, nil
}

func (p *LoginChoicePage) optionLabels() [2]string {
	if p.Backend == domain.BackendMdsvcTidb {
		return [2]string{"1. Login as Admin / User", "2. Login as Tenant Administrator"}
	}
	return [2]string{"1. Login", "2. Create Admin User (first-time setup)"}
}

func (p *LoginChoicePage) helpLines() []string {
	if p.Backend == domain.BackendMdsvcTidb {
		return []string{
			"Admin / User accounts manage PDUs, Racks, Hypervisors, Disks,",
			"NISDs, Vdevs and PFS within a tenant.",
			"",
			"Tenant Administrators manage tenants themselves — creating and",
			"deleting them — and have no access to infrastructure resources.",
		}
	}
	return []string{
		"Login if an admin account already exists on this control plane.",
		"",
		`Create Admin User bootstraps the reserved "admin" account on a`,
		"fresh niova-mdsvc deployment — unlike mdsvc-tidb's built-in",
		"admin/admin, niova-mdsvc ships with no default admin, so this is",
		"how you get in the very first time.",
	}
}

func (p *LoginChoicePage) View() string {
	options := p.optionLabels()

	var lines []string
	lines = append(lines, styles.TextBold.Render("Select Login Mode"), "")
	for i, opt := range options {
		if i == p.Cursor {
			lines = append(lines, styles.SelectedItem.Render("▶ "+opt))
		} else {
			lines = append(lines, styles.UnselectedItem.Render("  "+opt))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.JoinVertical(lipgloss.Left,
		content,
		"",
		styles.Card.Render(lipgloss.JoinVertical(lipgloss.Left, p.helpLines()...)),
	)
}

func (p *LoginChoicePage) Title() string { return "Select Login Mode" }

func (p *LoginChoicePage) KeyBindings() []key.Binding {
	if p.Backend == domain.BackendMdsvcTidb {
		return []key.Binding{
			key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "Admin / User Login")),
			key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "Tenant Administrator Login")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "Login")),
		key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "Create Admin User")),
	}
}
