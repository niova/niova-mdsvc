package pages

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// Messages for tenant lifecycle operations (mdsvc-tidb backend only)
type SubmitTenantAdminLoginMsg struct {
	Username string
	Password string
}

type TenantAdminLoggedInMsg struct {
	AccessToken string
	ExpiresIn   int
	Username    string
	Err         error
}

type SubmitTenantCreateMsg struct {
	DisplayName string
}

type TenantCreatedMsg struct {
	Result *domain.TenantCreateResult
	Err    error
}

type TenantDeletedMsg struct {
	Err error
}

// TenantAdminLoginPage handles the separate tenant-admin login (distinct
// identity/session from a regular user login).
type TenantAdminLoginPage struct {
	Form components.FormModel
}

func NewTenantAdminLoginPage() *TenantAdminLoginPage {
	fields := []components.FieldDefinition{
		{Label: "Username", Placeholder: "tenant-admin", Value: "tenant-admin", CharLimit: 256},
		{Label: "Password", Placeholder: "Enter tenant-admin password", CharLimit: 512},
	}
	return &TenantAdminLoginPage{Form: components.NewForm(fields)}
}

func (p *TenantAdminLoginPage) isFormPage() {}

func (p *TenantAdminLoginPage) Init() tea.Cmd { return nil }

func (p *TenantAdminLoginPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" {
			username := p.Form.Value(0)
			password := p.Form.Value(1)
			if p.Form.FocusedIndex == len(p.Form.Inputs)-1 || (username != "" && password != "") {
				return p, func() tea.Msg {
					return SubmitTenantAdminLoginMsg{Username: username, Password: password}
				}
			}
		}
	}
	cmd := p.Form.Update(msg)
	return p, cmd
}

func (p *TenantAdminLoginPage) View() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render("Tenant Administrator Login"),
		"",
		p.Form.View(),
	)
}

func (p *TenantAdminLoginPage) Title() string { return "Tenant Administrator Login" }

func (p *TenantAdminLoginPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Tab/Down"), key.WithHelp("Tab/Down", "Next Field")),
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Submit Login")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Back")),
	}
}

// TenantFormPage handles tenant creation (display name only — the server
// generates the tenant UUID, schema, and a fresh admin account for it).
type TenantFormPage struct {
	Form components.FormModel
	// Submitting guards against duplicate tenant creation while the async
	// CreateTenant call is in flight. Reset to false on failure so retry works.
	Submitting bool
}

func NewTenantFormPage() *TenantFormPage {
	fields := []components.FieldDefinition{
		{Type: components.FieldTypeText, Label: "Display Name", Placeholder: "e.g. acme-corp", CharLimit: 255},
	}
	return &TenantFormPage{Form: components.NewForm(fields)}
}

func (p *TenantFormPage) isFormPage() {}

// EscAction satisfies pages.EscTarget: Esc reloads the Tenants list.
func (p *TenantFormPage) EscAction() EscTargetKind { return EscTargetTenants }

func (p *TenantFormPage) Init() tea.Cmd { return nil }

func (p *TenantFormPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" && !p.Submitting {
			p.Submitting = true
			name := p.Form.Value(0)
			return p, func() tea.Msg {
				return SubmitTenantCreateMsg{DisplayName: name}
			}
		}
	}
	cmd := p.Form.Update(msg)
	return p, cmd
}

func (p *TenantFormPage) View() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render("Create New Tenant"),
		"",
		p.Form.View(),
	)
}

func (p *TenantFormPage) Title() string { return "Create Tenant" }

func (p *TenantFormPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Create Tenant")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Cancel")),
	}
}

// NewTenantListPage builds the Tenants table on the same shared TablePage
// every other resource browser uses — same '/' live search and inspect card.
// KeyBindings lists both Add and Delete here (unlike numbered resource tabs)
// since Tenants has no ActiveTab slot of its own, reached via :tenants only.
func NewTenantListPage(tenants []domain.TenantInfo, width int) *TablePage[domain.TenantInfo] {
	columns := []table.Column{
		{Title: "Display Name", Width: 24},
		{Title: "Status", Width: 10},
		{Title: "Schema", Width: 20},
		{Title: "Created By", Width: 16},
		{Title: "Tenant UUID", Width: 38},
	}
	var rows []table.Row
	for _, t := range tenants {
		rows = append(rows, table.Row{t.DisplayName, t.StatusString(), t.SchemaName, t.CreatedBy, t.TenantUUID})
	}
	actions := []key.Binding{
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "Create Tenant")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "Delete Selected Tenant")),
	}
	// height is a placeholder — loadTenantsView calls a.ResizeActivePageTable()
	// immediately after construction, which corrects it via SetTableHeight.
	return NewTablePage(tenants, columns, rows, width, 30,
		lipgloss.Color("#FFFFFF"), styles.ColorPrimary, true,
		"Tenants", "REGISTERED TENANTS", renderTenantDetail, actions)
}

func renderTenantDetail(t domain.TenantInfo) string {
	bullets := []string{
		fmt.Sprintf("  • Tenant UUID:  %s", t.TenantUUID),
		fmt.Sprintf("  • Status:       %s", t.StatusString()),
		fmt.Sprintf("  • Schema:       %s", t.SchemaName),
		fmt.Sprintf("  • Created By:   %s", t.CreatedBy),
		fmt.Sprintf("  • Created At:   %s", t.CreatedAt),
	}
	return detailLines("Tenant Inspection View", "TENANT DETAILS", lipgloss.Color("#FFFFFF"), styles.ColorPrimary,
		t.DisplayName, bullets, "[ Press Esc or Enter to return to Tenants Table ]")
}
