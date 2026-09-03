package pages

import (
	"fmt"
	"regexp"

	"github.com/charmbracelet/bubbles/key"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// validateResourceName mirrors main.go's isAlphanumericStr convention
// (letters, digits, hyphen, underscore — extended from letters/digits-only
// in #120 "Allow hyphens in vdev name validation"): no spaces, nothing that
// would need escaping downstream.
func validateResourceName(s string) error {
	if s == "" {
		return fmt.Errorf("required")
	}
	for _, r := range s {
		alnum := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !alnum && r != '-' && r != '_' {
			return fmt.Errorf("only letters, digits, hyphen (-) and underscore (_) — no spaces")
		}
	}
	return nil
}

var powerCapacityRe = regexp.MustCompile(`(?i)^\d+(\.\d+)?\s?kw$`)

// validatePowerCapacity accepts empty (not required) but requires a real
// "<number>kW" value otherwise, e.g. "30kW" or "12.5 kW".
func validatePowerCapacity(s string) error {
	if s == "" {
		return nil
	}
	if !powerCapacityRe.MatchString(s) {
		return fmt.Errorf("must be a number followed by kW, e.g. 30kW")
	}
	return nil
}

// isPowerCapacityChar rejects a keystroke immediately for any character
// that could never appear in a valid value (paired with AllowedChars) —
// unlike validatePowerCapacity itself, which requires the full "kW" suffix
// and so can't gate individual keystrokes without blocking every
// in-progress value on the way to one.
func isPowerCapacityChar(r rune) bool {
	return (r >= '0' && r <= '9') || r == '.' || r == ' ' || r == 'k' || r == 'K' || r == 'w' || r == 'W'
}

// commonDatacenterCities seeds the Location field's autocomplete — a
// starting set, not an exhaustive list; any free-text value still works,
// this only offers a shortcut for the common case.
var commonDatacenterCities = []string{
	"Pune", "Mumbai", "Bengaluru", "Hyderabad", "Chennai", "Delhi", "Noida", "Gurugram", "Kolkata", "Ahmedabad",
	"Singapore", "Tokyo", "Hong Kong", "Sydney", "London", "Frankfurt", "Amsterdam", "Dublin", "Paris", "Zurich",
	"New York", "San Jose", "Ashburn", "Dallas", "Chicago", "Seattle", "Toronto", "São Paulo",
}

// SubmitPDUFormMsg emitted when PDU form is submitted
type SubmitPDUFormMsg struct {
	PDUID         string
	Name          string
	Specification string
	Location      string
	PowerCapacity string
	IsEdit        bool
}

// PDUFormPage handles creation and editing of PDUs
type PDUFormPage struct {
	Form      components.FormModel
	IsEdit    bool
	InitialID string
}

func NewPDUFormPage(isEdit bool, initial domain.PDU) *PDUFormPage {
	fields := []components.FieldDefinition{
		{Type: components.FieldTypeText, Label: "PDU Name", Placeholder: "e.g. PDU-RackA-01", Value: initial.Name, CharLimit: 64,
			Validate: validateResourceName, BlockInvalidChars: true},
		{Type: components.FieldTypeText, Label: "Specification / Description", Placeholder: "Enter description/spec", Value: initial.Specification, CharLimit: 256},
		{Type: components.FieldTypeText, Label: "Location", Placeholder: "e.g. Pune", Value: initial.Location, CharLimit: 128,
			Suggestions: commonDatacenterCities},
		{Type: components.FieldTypeText, Label: "Power Capacity", Placeholder: "e.g. 30kW", Value: initial.PowerCapacity, CharLimit: 64,
			Validate: validatePowerCapacity, AllowedChars: isPowerCapacityChar},
	}
	return &PDUFormPage{
		Form:      components.NewForm(fields),
		IsEdit:    isEdit,
		InitialID: initial.ID,
	}
}

func (p *PDUFormPage) isFormPage() {}

func (p *PDUFormPage) Init() tea.Cmd {
	return nil
}

func (p *PDUFormPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" && p.Form.FocusedIndex == len(p.Form.Inputs)-1 && p.Form.Valid() {
			name := p.Form.Value(0)
			spec := p.Form.Value(1)
			loc := p.Form.Value(2)
			cap := p.Form.Value(3)
			pduID := p.InitialID
			return p, func() tea.Msg {
				return SubmitPDUFormMsg{
					PDUID:         pduID,
					Name:          name,
					Specification: spec,
					Location:      loc,
					PowerCapacity: cap,
					IsEdit:        p.IsEdit,
				}
			}
		}
	}

	cmd := p.Form.Update(msg)
	return p, cmd
}

func (p *PDUFormPage) View() string {
	title := "Create New Power Distribution Unit (PDU)"
	if p.IsEdit {
		title = "Edit PDU Configuration"
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render(title),
		"",
		p.Form.View(),
	)
}

func (p *PDUFormPage) Title() string {
	if p.IsEdit {
		return "Edit PDU"
	}
	return "Add PDU"
}

func (p *PDUFormPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Tab/Down"), key.WithHelp("Tab/Down", "Next Field")),
		key.NewBinding(key.WithKeys("→"), key.WithHelp("→", "Accept City Suggestion")),
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Submit PDU")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Cancel")),
	}
}

// NewPDUViewPage builds an interactive table of PDUs; enter/space/v opens a
// detail card for the selected one.
func NewPDUViewPage(pdus []domain.PDU, width, height int, canWrite bool) *TablePage[domain.PDU] {
	columns := []table.Column{
		{Title: "PDU Name", Width: 20},
		{Title: "Location", Width: 22},
		{Title: "Power Capacity", Width: 18},
		{Title: "Attached Racks", Width: 16},
		{Title: "PDU UUID", Width: 38},
	}

	var rows []table.Row
	for _, p := range pdus {
		rackCnt := fmt.Sprintf("%d Racks", len(p.Racks))
		rows = append(rows, table.Row{
			p.Name, p.Location, p.PowerCapacity, rackCnt, p.ID,
		})
	}

	actions := []key.Binding{
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "Add PDU")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "Edit PDU")),
	}

	return NewTablePage(pdus, columns, rows, width, height,
		lipgloss.Color("#FFFFFF"), styles.ColorPrimary, canWrite,
		"View All PDUs", "CONFIGURED PDUS TABLE", renderPDUDetail, actions)
}

func renderPDUDetail(pdu domain.PDU) string {
	bullets := []string{
		fmt.Sprintf("  • PDU UUID:         %s", pdu.ID),
		fmt.Sprintf("  • Location:         %s", pdu.Location),
		fmt.Sprintf("  • Power Capacity:   %s", pdu.PowerCapacity),
		fmt.Sprintf("  • Specification:    %s", pdu.Specification),
		fmt.Sprintf("  • Attached Racks:   %d", len(pdu.Racks)),
	}
	return detailLines("PDU Inspection View", "PDU DETAILS", lipgloss.Color("#FFFFFF"), styles.ColorPrimary,
		pdu.Name, bullets, "[ Press Esc or Enter to return to PDU Table ]")
}
