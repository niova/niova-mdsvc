package pages

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"
)

// Messages for User operations
type UserLoggedInMsg struct {
	Resp       *userlib.LoginResp
	Username   string
	SecretKey  string
	TenantUUID string
	Err        error
}

type UserCreatedMsg struct {
	Resp *userlib.UserResp
	Err  error
}

type SubmitUserLoginMsg struct {
	Username   string
	SecretKey  string
	TenantUUID string
}

// SwitchToCreateAdminMsg is emitted from UserLoginPage's "create admin"
// shortcut — reachable pre-login, unlike TriggerAddForm's route to
// UserCreatePage, so a fresh deployment with no admin yet can bootstrap one.
type SwitchToCreateAdminMsg struct{}

type SubmitUserCreateMsg struct {
	Username  string
	SecretKey string
	IsAdmin   bool
}

// UserLoginPage handles user login form
type UserLoginPage struct {
	Form components.FormModel
}

// NewUserLoginPage builds the login form. showTenantField adds the optional
// Tenant UUID field — only meaningful against mdsvc-tidb, whose
// LoginWithTenant rejects a non-empty value against niova-mdsvc outright
// (see mdsvcpumiceclient's own doc), so niova-mdsvc omits the field entirely
// rather than offer an input that can only ever error.
func NewUserLoginPage(showTenantField bool) *UserLoginPage {
	fields := []components.FieldDefinition{
		{Label: "Username", Placeholder: "Enter username (e.g. admin)", CharLimit: 256},
		{Label: "Secret Key", Placeholder: "Enter secret key", CharLimit: 512},
	}
	if showTenantField {
		fields = append(fields, components.FieldDefinition{
			Label: "Tenant UUID (Optional)", Placeholder: "Enter Tenant UUID (leave blank for default tenant)", CharLimit: 256,
		})
	}
	return &UserLoginPage{
		Form: components.NewForm(fields),
	}
}

func (p *UserLoginPage) isFormPage() {}

func (p *UserLoginPage) Init() tea.Cmd {
	return nil
}

func (p *UserLoginPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "enter":
			username := p.Form.Value(0)
			secretKey := p.Form.Value(1)
			tenantUUID := p.Form.Value(2)
			if p.Form.FocusedIndex >= 1 || (username != "" && secretKey != "") {
				return p, func() tea.Msg {
					return SubmitUserLoginMsg{
						Username:   username,
						SecretKey:  secretKey,
						TenantUUID: tenantUUID,
					}
				}
			}
		case "ctrl+n":
			// Not a plain "n"/"c" — those need to stay typeable into the
			// focused Username/Secret Key field above.
			return p, func() tea.Msg { return SwitchToCreateAdminMsg{} }
		}
	}

	cmd := p.Form.Update(msg)
	return p, cmd
}

func (p *UserLoginPage) View() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render("User Login"),
		"",
		p.Form.View(),
		"",
		styles.TextMuted.Render("First time setup? Press Ctrl+N to create the initial admin user."),
	)
}

func (p *UserLoginPage) Title() string {
	return "User Authentication"
}

func (p *UserLoginPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Tab/Down"), key.WithHelp("Tab/Down", "Next Field")),
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Submit Login")),
		key.NewBinding(key.WithKeys("Ctrl+N"), key.WithHelp("Ctrl+N", "Create Admin User")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Back")),
	}
}

// UserCreatePage handles user creation form. Its field set depends on
// IsBootstrap — see NewUserCreatePage.
type UserCreatePage struct {
	Form        components.FormModel
	IsBootstrap bool
}

// NewUserCreatePage builds the Create User form. isBootstrap picks the
// field set: an authenticated admin adding a user gets Username + Secret
// Key + Role; pre-login bootstrap only offers Secret Key.
func NewUserCreatePage(isBootstrap bool) *UserCreatePage {
	var fields []components.FieldDefinition
	if isBootstrap {
		fields = []components.FieldDefinition{
			{Label: "Secret Key (Optional)", Placeholder: "Leave empty to auto-generate", CharLimit: 512},
		}
	} else {
		fields = []components.FieldDefinition{
			{Label: "Username", Placeholder: "Enter new username", CharLimit: 256, Validate: func(s string) error {
				if s == "" {
					return fmt.Errorf("required")
				}
				return nil
			}},
			{Label: "Secret Key (Optional)", Placeholder: "Leave empty to auto-generate", CharLimit: 512},
			// No token field here: the creator's own session token
			// authorizes the call.
			{Type: components.FieldTypeSelect, Label: "Role", Value: "user", Options: []components.SelectOption{
				{Label: "User", Value: "user"},
				{Label: "Admin", Value: "admin"},
			}},
		}
	}
	return &UserCreatePage{
		Form:        components.NewForm(fields),
		IsBootstrap: isBootstrap,
	}
}

func (p *UserCreatePage) isFormPage() {}

// EscAction satisfies pages.EscTarget: Esc reloads the Users list. Only
// reached while already logged in — update.go's pre-login gate handles the
// bootstrap (not-yet-logged-in) case before this is ever consulted.
func (p *UserCreatePage) EscAction() EscTargetKind { return EscTargetUsers }

func (p *UserCreatePage) Init() tea.Cmd {
	return nil
}

func (p *UserCreatePage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" && p.Form.FocusedIndex == len(p.Form.Inputs)-1 && p.Form.Valid() {
			if p.IsBootstrap {
				secretKey := p.Form.Value(0)
				return p, func() tea.Msg {
					return SubmitUserCreateMsg{
						Username:  userlib.AdminUsername,
						SecretKey: secretKey,
						IsAdmin:   true,
					}
				}
			}
			username := p.Form.Value(0)
			secretKey := p.Form.Value(1)
			role := p.Form.Value(2)
			return p, func() tea.Msg {
				return SubmitUserCreateMsg{
					Username:  username,
					SecretKey: secretKey,
					IsAdmin:   role == "admin",
				}
			}
		}
	}

	cmd := p.Form.Update(msg)
	return p, cmd
}

func (p *UserCreatePage) View() string {
	title := "Create New User"
	sections := []string{styles.TextBold.Render(title), ""}
	if p.IsBootstrap {
		title = "Bootstrap Admin User"
		sections = []string{
			styles.TextBold.Render(title),
			"",
			fmt.Sprintf("Username: %s", styles.SelectedItem.Render(userlib.AdminUsername)),
			styles.TextMuted.Render("(the server only allows bootstrapping the reserved \"admin\" account this way)"),
			"",
		}
	}
	sections = append(sections, p.Form.View())
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (p *UserCreatePage) Title() string {
	if p.IsBootstrap {
		return "Bootstrap Admin"
	}
	return "Create User"
}

func (p *UserCreatePage) KeyBindings() []key.Binding {
	bindings := []key.Binding{
		key.NewBinding(key.WithKeys("Tab/Down"), key.WithHelp("Tab/Down", "Next Field")),
	}
	if !p.IsBootstrap {
		bindings = append(bindings, key.NewBinding(key.WithKeys("◄/►/Space"), key.WithHelp("◄/►/Space", "Select Role")))
	}
	bindings = append(bindings,
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Create User")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Back")),
	)
	return bindings
}

type CopyOutputMsg struct {
	Err error
}

// ResultKind identifies which flow produced a UserResultPage — shared by
// user-creation and tenant-creation, so its EscAction (below) branches on
// this rather than update.go needing to tell them apart itself.
type ResultKind int

const (
	ResultKindUser ResultKind = iota
	ResultKindTenant
)

// UserResultPage displays user/tenant creation output details.
type UserResultPage struct {
	Kind     ResultKind
	TitleStr string
	Details  []string
}

func NewUserResultPage(kind ResultKind, title string, details []string) *UserResultPage {
	return &UserResultPage{
		Kind:     kind,
		TitleStr: title,
		Details:  details,
	}
}

// EscAction satisfies pages.EscTarget. Only reached while already logged
// in — update.go's pre-login gate handles the bootstrap-user case (Kind ==
// ResultKindUser, not yet logged in) before this is ever consulted.
func (p *UserResultPage) EscAction() EscTargetKind {
	if p.Kind == ResultKindTenant {
		return EscTargetTenants
	}
	return EscTargetUsers
}

func (p *UserResultPage) Init() tea.Cmd { return nil }

func (p *UserResultPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "c", "C", "y", "Y":
			text := strings.Join(p.Details, "\n")
			return p, func() tea.Msg {
				err := clipboard.WriteAll(text)
				return CopyOutputMsg{Err: err}
			}
		}
	}
	return p, nil
}

func (p *UserResultPage) View() string {
	var formatted []string
	for _, d := range p.Details {
		formatted = append(formatted, styles.UnselectedItem.Render(d))
	}
	content := lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render(p.TitleStr),
		"",
		styles.Card.Render(lipgloss.JoinVertical(lipgloss.Left, formatted...)),
	)
	return content
}

func (p *UserResultPage) Title() string { return p.TitleStr }

func (p *UserResultPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "Copy to Clipboard")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Back")),
	}
}

// NewUserListPage builds the Users table on the same shared TablePage every
// other resource browser uses — same '/' live search and inspect card. Only
// "a" Add User is wired (no Edit User form exists); loadUsersView already
// requires admin before this page is constructed, so CanWrite is always true.
func NewUserListPage(users []userlib.UserResp, width int) *TablePage[userlib.UserResp] {
	columns := []table.Column{
		{Title: "Username", Width: 24},
		{Title: "Role", Width: 12},
		{Title: "Status", Width: 12},
		{Title: "User ID", Width: 38},
	}
	var rows []table.Row
	for _, u := range users {
		rows = append(rows, table.Row{u.Username, u.UserRole, u.Status.String(), u.UserID})
	}
	actions := []key.Binding{
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "Add User")),
	}
	// height is a placeholder — loadUsersView calls a.ResizeActivePageTable()
	// immediately after construction, which corrects it via SetTableHeight.
	return NewTablePage(users, columns, rows, width, 30,
		lipgloss.Color("#FFFFFF"), styles.ColorPrimary, true,
		"User Accounts", "REGISTERED USERS", renderUserDetail, actions)
}

func renderUserDetail(u userlib.UserResp) string {
	bullets := []string{
		fmt.Sprintf("  • User ID:  %s", u.UserID),
		fmt.Sprintf("  • Role:     %s", u.UserRole),
		fmt.Sprintf("  • Status:   %s", u.Status.String()),
	}
	return detailLines("User Inspection View", "USER DETAILS", lipgloss.Color("#FFFFFF"), styles.ColorPrimary,
		u.Username, bullets, "[ Press Esc or Enter to return to Users Table ]")
}
