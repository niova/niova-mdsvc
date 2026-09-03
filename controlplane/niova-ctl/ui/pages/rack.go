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

// SubmitRackFormMsg emitted when Rack form is submitted
type SubmitRackFormMsg struct {
	RackID        string
	Name          string
	PDUID         string
	Specification string
	Location      string
	IsEdit        bool
}

// RackFormPage handles creation and editing of Racks with PDU association dropdown
type RackFormPage struct {
	Form   components.FormModel
	PDUs   []domain.PDU
	IsEdit bool
	// InitialID carries the edited Rack's ID through submit — see
	// HypervisorFormPage.InitialID for why this matters.
	InitialID string
}

func NewRackFormPage(isEdit bool, initial domain.Rack, pdus []domain.PDU) *RackFormPage {
	var pduOptions []components.SelectOption
	pduOptions = append(pduOptions, components.SelectOption{Label: "None (Standalone Rack)", Value: ""})
	for _, p := range pdus {
		pduOptions = append(pduOptions, components.SelectOption{
			Label: fmt.Sprintf("%s (UUID: %s)", p.Name, p.ID),
			Value: p.ID,
		})
	}

	fields := []components.FieldDefinition{
		{Type: components.FieldTypeText, Label: "Rack Name", Placeholder: "e.g. Rack-01", Value: initial.Name, CharLimit: 64},
		{Type: components.FieldTypeSelect, Label: "Parent PDU", Value: initial.PDUID, Options: pduOptions},
		{Type: components.FieldTypeText, Label: "Specification / Description", Placeholder: "Enter description/spec", Value: initial.Specification, CharLimit: 256},
		{Type: components.FieldTypeText, Label: "Location", Placeholder: "e.g. Row A - Position 04", Value: initial.Location, CharLimit: 128},
	}
	return &RackFormPage{
		Form:      components.NewForm(fields),
		PDUs:      pdus,
		IsEdit:    isEdit,
		InitialID: initial.ID,
	}
}

func (p *RackFormPage) isFormPage() {}

func (p *RackFormPage) Init() tea.Cmd { return nil }

func (p *RackFormPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" && p.Form.FocusedIndex == len(p.Form.Inputs)-1 {
			name := p.Form.Value(0)
			pduID := p.Form.Value(1)
			spec := p.Form.Value(2)
			loc := p.Form.Value(3)
			return p, func() tea.Msg {
				return SubmitRackFormMsg{
					RackID:        p.InitialID,
					Name:          name,
					PDUID:         pduID,
					Specification: spec,
					Location:      loc,
					IsEdit:        p.IsEdit,
				}
			}
		}
	}

	cmd := p.Form.Update(msg)
	return p, cmd
}

func (p *RackFormPage) View() string {
	title := "Create New Server Rack"
	if p.IsEdit {
		title = "Edit Server Rack"
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render(title),
		"",
		p.Form.View(),
	)
}

func (p *RackFormPage) Title() string {
	if p.IsEdit {
		return "Edit Rack"
	}
	return "Add Rack"
}

func (p *RackFormPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Tab/Down"), key.WithHelp("Tab/Down", "Next Field")),
		key.NewBinding(key.WithKeys("◄/►/Space"), key.WithHelp("◄/►/Space", "Select PDU Option")),
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Submit Rack")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Cancel")),
	}
}

// NewRackViewPage builds an interactive table of Racks. The Rack-ID ->
// parent-PDU-name map is resolved once here and captured by the detail-card
// renderer's closure, since TablePage has no cross-resource lookup notion.
func NewRackViewPage(racks []domain.Rack, pdus []domain.PDU, width, height int, canWrite bool) *TablePage[domain.Rack] {
	var allRacks []domain.Rack
	parentMap := make(map[string]string)
	pduMap := make(map[string]domain.PDU)

	for _, p := range pdus {
		pduMap[p.ID] = p
	}

	columns := []table.Column{
		{Title: "Rack Name", Width: 20},
		{Title: "Parent PDU", Width: 24},
		{Title: "Location", Width: 22},
		{Title: "Hypervisors", Width: 14},
		{Title: "UUID", Width: 38},
	}

	var rows []table.Row

	for _, r := range racks {
		allRacks = append(allRacks, r)
		pduName := "Standalone"
		if parentPDU, found := pduMap[r.PDUID]; found {
			pduName = parentPDU.Name
		} else if r.PDUID != "" {
			pduName = r.PDUID
		}
		parentMap[r.ID] = pduName

		rows = append(rows, table.Row{
			r.Name, pduName, r.Location, fmt.Sprintf("%d nodes", len(r.Hypervisors)), r.ID,
		})
	}

	actions := []key.Binding{
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "Add Rack")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "Edit Rack")),
	}

	renderInspect := func(rack domain.Rack) string {
		bullets := []string{
			fmt.Sprintf("  • Rack UUID:         %s", rack.ID),
			fmt.Sprintf("  • Parent PDU:        %s", parentMap[rack.ID]),
			fmt.Sprintf("  • Location:          %s", rack.Location),
			fmt.Sprintf("  • Specification:     %s", rack.Specification),
			fmt.Sprintf("  • Hypervisors Count: %d", len(rack.Hypervisors)),
		}
		if len(rack.Hypervisors) > 0 {
			bullets = append(bullets, "", styles.TextBold.Render("Hypervisor Nodes:"))
			for _, hv := range rack.Hypervisors {
				ipStr := domain.FormatIPAddresses(hv)
				bullets = append(bullets, fmt.Sprintf("    ├─ %s (IPs: %s, Ports: %s, Devices: %d)", hv.Name, ipStr, hv.PortRange, len(hv.Dev)))
			}
		}
		return detailLines("Rack Inspection View", "SERVER RACK DETAILS", lipgloss.Color("#000000"), styles.ColorSecondary,
			rack.Name, bullets, "[ Press Esc or Enter to return to Server Racks Table ]")
	}

	return NewTablePage(allRacks, columns, rows, width, height,
		lipgloss.Color("#000000"), styles.ColorSecondary, canWrite,
		"View All Server Racks", "CONFIGURED SERVER RACKS TABLE", renderInspect, actions)
}
