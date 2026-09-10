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

// Messages for authorization policy administration (mdsvc-tidb backend only)
type SubmitAuthzPolicyMsg struct {
	IsRBAC   bool
	Resource string
	Action   string
	Role     string // ignored for ABAC
}

type AuthzPolicySavedMsg struct{ Err error }

// AuthzDeleteTarget identifies exactly one policy to delete, carried as the
// ConfirmDialog's Data.
type AuthzDeleteTarget struct {
	IsRBAC bool
	RBAC   domain.RBACPolicy
	ABAC   domain.ABACPolicy
}

type AuthzPolicyDeletedMsg struct{ Err error }

// AuthzFormPage handles adding one RBAC or ABAC policy.
type AuthzFormPage struct {
	Form   components.FormModel
	IsRBAC bool
}

func NewAuthzFormPage(isRBAC bool) *AuthzFormPage {
	fields := []components.FieldDefinition{
		{Type: components.FieldTypeText, Label: "Resource", Placeholder: "e.g. vdev, pdu, nisd", CharLimit: 64},
		{Type: components.FieldTypeText, Label: "Action", Placeholder: "e.g. read, write", CharLimit: 64},
	}
	if isRBAC {
		fields = append(fields, components.FieldDefinition{Type: components.FieldTypeText, Label: "Role", Placeholder: "e.g. admin, user", CharLimit: 64})
	}
	return &AuthzFormPage{Form: components.NewForm(fields), IsRBAC: isRBAC}
}

func (p *AuthzFormPage) isFormPage() {}

// EscAction satisfies pages.EscTarget: Esc reloads the Authz policy tables.
func (p *AuthzFormPage) EscAction() EscTargetKind { return EscTargetAuthz }

func (p *AuthzFormPage) Init() tea.Cmd { return nil }

func (p *AuthzFormPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" && p.Form.FocusedIndex == len(p.Form.Inputs)-1 {
			resource := p.Form.Value(0)
			action := p.Form.Value(1)
			role := ""
			if p.IsRBAC {
				role = p.Form.Value(2)
			}
			return p, func() tea.Msg {
				return SubmitAuthzPolicyMsg{IsRBAC: p.IsRBAC, Resource: resource, Action: action, Role: role}
			}
		}
	}
	cmd := p.Form.Update(msg)
	return p, cmd
}

func (p *AuthzFormPage) View() string {
	title := "Add ABAC Policy"
	if p.IsRBAC {
		title = "Add RBAC Policy"
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render(title),
		"",
		p.Form.View(),
	)
}

func (p *AuthzFormPage) Title() string {
	if p.IsRBAC {
		return "Add RBAC Policy"
	}
	return "Add ABAC Policy"
}

func (p *AuthzFormPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Tab/Down"), key.WithHelp("Tab/Down", "Next Field")),
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Save Policy")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Cancel")),
	}
}

// AuthzPolicyPage shows RBAC and ABAC policies side by side; Tab switches
// which table has focus (and therefore which one 'a'/'d'/'/' act on — '/'
// opens a live search box (searchBox) scoped to whichever table currently
// has focus, same matching rule as TablePage: a case-insensitive substring
// against that row's own rendered cells).
type AuthzPolicyPage struct {
	RBACTable table.Model
	ABACTable table.Model
	RBAC      []domain.RBACPolicy
	ABAC      []domain.ABACPolicy
	FocusRBAC bool

	rbacAllRows []table.Row
	abacAllRows []table.Row
	rbacVisible []int
	abacVisible []int
	rbacSearch  searchBox
	abacSearch  searchBox

	// visibleWidth is shared (both tables scroll against the same budget,
	// see availableTableWidth); rbacHScroll/abacHScroll are independent, so
	// switching focus keeps whatever scroll position each was left at.
	visibleWidth int
	rbacHScroll  int
	abacHScroll  int
}

// activeSearch returns whichever table's search box is relevant to the
// currently focused table.
func (p *AuthzPolicyPage) activeSearch() *searchBox {
	if p.FocusRBAC {
		return &p.rbacSearch
	}
	return &p.abacSearch
}

// activeHScroll returns a pointer to the focused table's scroll offset.
func (p *AuthzPolicyPage) activeHScroll() *int {
	if p.FocusRBAC {
		return &p.rbacHScroll
	}
	return &p.abacHScroll
}

// focusedTable returns whichever of RBACTable/ABACTable currently has focus
// — a pointer, since scroll clamping only ever reads its Columns().
func (p *AuthzPolicyPage) focusedTable() *table.Model {
	if p.FocusRBAC {
		return &p.RBACTable
	}
	return &p.ABACTable
}

func tableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.BorderStyle(lipgloss.NormalBorder()).BorderForeground(styles.ColorMuted).BorderBottom(true).Bold(true)
	s.Selected = s.Selected.Foreground(lipgloss.Color("#FFFFFF")).Background(styles.ColorPrimary).Bold(true)
	return s
}

func NewAuthzPolicyPage(rbac []domain.RBACPolicy, abac []domain.ABACPolicy, width int) *AuthzPolicyPage {
	rbacCols := []table.Column{
		{Title: "Resource", Width: 20}, {Title: "Action", Width: 16}, {Title: "Role", Width: 16},
	}
	var rbacRows []table.Row
	for _, p := range rbac {
		rbacRows = append(rbacRows, table.Row{p.Resource, p.Action, p.Role})
	}
	rbacCols = autoSizeColumns(rbacCols, rbacRows)
	rt := table.New(table.WithColumns(rbacCols), table.WithRows(rbacRows), table.WithFocused(true), table.WithHeight(8))
	rt.KeyMap.LineDown.SetKeys("down", "j")
	rt.SetStyles(tableStyles())

	abacCols := []table.Column{
		{Title: "Resource", Width: 20}, {Title: "Action", Width: 16},
	}
	var abacRows []table.Row
	for _, p := range abac {
		abacRows = append(abacRows, table.Row{p.Resource, p.Action})
	}
	abacCols = autoSizeColumns(abacCols, abacRows)
	at := table.New(table.WithColumns(abacCols), table.WithRows(abacRows), table.WithFocused(false), table.WithHeight(8))
	at.KeyMap.LineDown.SetKeys("down", "j")
	at.SetStyles(tableStyles())

	return &AuthzPolicyPage{
		RBACTable:    rt,
		ABACTable:    at,
		RBAC:         rbac,
		ABAC:         abac,
		FocusRBAC:    true,
		rbacAllRows:  rbacRows,
		abacAllRows:  abacRows,
		rbacVisible:  identityIndices(len(rbac)),
		abacVisible:  identityIndices(len(abac)),
		rbacSearch:   newSearchBox(),
		abacSearch:   newSearchBox(),
		visibleWidth: availableTableWidth(width),
	}
}

func (p *AuthzPolicyPage) Init() tea.Cmd { return nil }

// SetFocusRBAC switches which table has focus, e.g. to carry the previously
// focused table across a reload (see App.loadAuthzView).
func (p *AuthzPolicyPage) SetFocusRBAC(rbac bool) {
	p.FocusRBAC = rbac
	if rbac {
		p.RBACTable.Focus()
		p.ABACTable.Blur()
	} else {
		p.ABACTable.Focus()
		p.RBACTable.Blur()
	}
}

func (p *AuthzPolicyPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	active := p.activeSearch()

	// Same priority order as TablePage: while the focused table's search
	// box has keyboard focus, every key goes to it except Esc/Enter.
	if active.Filtering() {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "esc":
				p.clearFilter()
				return p, nil
			case "enter":
				active.Commit()
				return p, nil
			}
		}
		cmd := active.Type(msg)
		p.applyFilters()
		return p, cmd
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "tab":
			p.SetFocusRBAC(!p.FocusRBAC)
			return p, nil
		case "/":
			return p, active.Open()
		case "esc":
			if active.Value() != "" {
				p.clearFilter()
				return p, nil
			}
		case "shift+left":
			hs := p.activeHScroll()
			*hs = max(*hs-8, 0)
			return p, nil
		case "shift+right":
			hs := p.activeHScroll()
			*hs = min(*hs+8, maxHScroll(tableNaturalWidth(p.focusedTable().Columns()), p.visibleWidth))
			return p, nil
		}
	}

	var cmd tea.Cmd
	if p.FocusRBAC {
		p.RBACTable, cmd = p.RBACTable.Update(msg)
	} else {
		p.ABACTable, cmd = p.ABACTable.Update(msg)
	}
	return p, cmd
}

// applyFilters recomputes both tables' visible rows from their own search
// box's current text — always both, not just the focused one, so a filter
// left applied on the table you tabbed away from stays in effect.
func (p *AuthzPolicyPage) applyFilters() {
	idx, rows := filterRows(p.rbacAllRows, p.rbacSearch.Value())
	p.rbacVisible = idx
	p.RBACTable.SetRows(rows)
	if p.rbacSearch.Value() != "" {
		p.RBACTable.SetCursor(0)
	}

	idx, rows = filterRows(p.abacAllRows, p.abacSearch.Value())
	p.abacVisible = idx
	p.ABACTable.SetRows(rows)
	if p.abacSearch.Value() != "" {
		p.ABACTable.SetCursor(0)
	}
}

// clearFilter resets the focused table's search box, keeping its cursor on
// whatever item it was over — same reasoning as TablePage.clearFilter.
func (p *AuthzPolicyPage) clearFilter() {
	if p.FocusRBAC {
		cursor := p.RBACTable.Cursor()
		realIdx := 0
		if cursor >= 0 && cursor < len(p.rbacVisible) {
			realIdx = p.rbacVisible[cursor]
		}
		p.rbacSearch.Clear()
		p.applyFilters()
		p.RBACTable.SetCursor(realIdx)
		return
	}
	cursor := p.ABACTable.Cursor()
	realIdx := 0
	if cursor >= 0 && cursor < len(p.abacVisible) {
		realIdx = p.abacVisible[cursor]
	}
	p.abacSearch.Clear()
	p.applyFilters()
	p.ABACTable.SetCursor(realIdx)
}

// IsFiltering satisfies pages.Filtering — see its doc comment.
func (p *AuthzPolicyPage) IsFiltering() bool { return p.activeSearch().Filtering() }

// FilterView satisfies pages.FilterViewer — see its doc comment.
func (p *AuthzPolicyPage) FilterView() string { return p.activeSearch().View() }

func (p *AuthzPolicyPage) View() string {
	rbacTitle := "RBAC Policies"
	abacTitle := "ABAC Policies"
	if p.FocusRBAC {
		rbacTitle = "▶ " + rbacTitle
	} else {
		abacTitle = "▶ " + abacTitle
	}

	rbacCount := fmt.Sprintf("(%d items)", len(p.RBAC))
	if p.rbacSearch.Value() != "" {
		rbacCount = fmt.Sprintf("(%d/%d matches)", len(p.rbacVisible), len(p.RBAC))
	}
	if maxHScroll(tableNaturalWidth(p.RBACTable.Columns()), p.visibleWidth) > 0 {
		rbacCount += fmt.Sprintf(" — Shift+←/→ to scroll (col %d)", p.rbacHScroll)
	}
	abacCount := fmt.Sprintf("(%d items)", len(p.ABAC))
	if p.abacSearch.Value() != "" {
		abacCount = fmt.Sprintf("(%d/%d matches)", len(p.abacVisible), len(p.ABAC))
	}
	if maxHScroll(tableNaturalWidth(p.ABACTable.Columns()), p.visibleWidth) > 0 {
		abacCount += fmt.Sprintf(" — Shift+←/→ to scroll (col %d)", p.abacHScroll)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		fmt.Sprintf("%s %s", styles.TextBold.Render(rbacTitle), styles.TextMuted.Render(rbacCount)),
		styles.Card.Render(scrollClip(p.RBACTable.View(), p.rbacHScroll, p.visibleWidth)),
		"",
		fmt.Sprintf("%s %s", styles.TextBold.Render(abacTitle), styles.TextMuted.Render(abacCount)),
		styles.Card.Render(scrollClip(p.ABACTable.View(), p.abacHScroll, p.visibleWidth)),
	)
	return content
}

func (p *AuthzPolicyPage) Title() string { return "Authorization Policies" }

func (p *AuthzPolicyPage) KeyBindings() []key.Binding {
	if p.activeSearch().Filtering() {
		return []key.Binding{
			key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Apply Filter")),
			key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Clear Filter")),
		}
	}
	bindings := []key.Binding{
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "Add Policy")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "Delete Selected Policy")),
	}
	if maxHScroll(tableNaturalWidth(p.focusedTable().Columns()), p.visibleWidth) > 0 {
		bindings = append(bindings,
			key.NewBinding(key.WithKeys("shift+left", "shift+right"), key.WithHelp("Shift+←/→", "Scroll Table")),
		)
	}
	filterHint := key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "Search"))
	if p.activeSearch().Value() != "" {
		filterHint = key.NewBinding(key.WithKeys("/", "Esc"), key.WithHelp("/ Esc", "Edit/Clear Search"))
	}
	return append(bindings, filterHint)
}

// SelectedRBAC returns the currently highlighted RBAC row, if the RBAC table
// has focus and a row is selected — resolved through rbacVisible, so a
// cursor over filtered rows still points at the right policy.
func (p *AuthzPolicyPage) SelectedRBAC() (domain.RBACPolicy, bool) {
	if !p.FocusRBAC {
		return domain.RBACPolicy{}, false
	}
	cursor := p.RBACTable.Cursor()
	if cursor < 0 || cursor >= len(p.rbacVisible) {
		return domain.RBACPolicy{}, false
	}
	idx := p.rbacVisible[cursor]
	if idx < 0 || idx >= len(p.RBAC) {
		return domain.RBACPolicy{}, false
	}
	return p.RBAC[idx], true
}

// SelectedABAC returns the currently highlighted ABAC row, if the ABAC table
// has focus and a row is selected — resolved through abacVisible, so a
// cursor over filtered rows still points at the right policy.
func (p *AuthzPolicyPage) SelectedABAC() (domain.ABACPolicy, bool) {
	if p.FocusRBAC {
		return domain.ABACPolicy{}, false
	}
	cursor := p.ABACTable.Cursor()
	if cursor < 0 || cursor >= len(p.abacVisible) {
		return domain.ABACPolicy{}, false
	}
	idx := p.abacVisible[cursor]
	if idx < 0 || idx >= len(p.ABAC) {
		return domain.ABACPolicy{}, false
	}
	return p.ABAC[idx], true
}

func (p *AuthzPolicyPage) SetTableHeight(h int) {
	// Account for dual-table internal overhead (2 titles, 2 table borders, 1 separator)
	hAdj := h - 4
	if hAdj < 4 {
		hAdj = 4
	}
	h2 := hAdj / 2
	if h2 < 2 {
		h2 = 2
	}
	p.RBACTable.SetHeight(h2)
	p.ABACTable.SetHeight(h2)
}

// SetTableWidth satisfies pages.TableWidthSetter — see TablePage's own
// SetTableWidth: column widths don't change with terminal size, only how
// much of them (shared by both tables) is visible does.
func (p *AuthzPolicyPage) SetTableWidth(w int) {
	p.visibleWidth = availableTableWidth(w)
	p.rbacHScroll = min(p.rbacHScroll, maxHScroll(tableNaturalWidth(p.RBACTable.Columns()), p.visibleWidth))
	p.abacHScroll = min(p.abacHScroll, maxHScroll(tableNaturalWidth(p.ABACTable.Columns()), p.visibleWidth))
}
