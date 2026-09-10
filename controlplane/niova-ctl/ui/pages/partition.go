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

// SubmitPartitionFormMsg emitted when Partition form is submitted
type SubmitPartitionFormMsg struct {
	PartitionID   string
	DeviceID      string
	PartitionPath string
	SizeGB        string
	NISDUUID      string
	IsEdit        bool
}

// PartitionFormPage handles creation of Device Partitions with Device dropdown picker
type PartitionFormPage struct {
	Form    components.FormModel
	Devices []domain.Device
	IsEdit  bool
	// InitialID carries the edited Partition's ID through submit — see
	// HypervisorFormPage.InitialID for why this matters.
	InitialID string
}

func NewPartitionFormPage(isEdit bool, initial domain.DevicePartition, devs []domain.Device) *PartitionFormPage {
	var devOptions []components.SelectOption
	for _, d := range devs {
		devOptions = append(devOptions, components.SelectOption{
			Label: fmt.Sprintf("%s (%s - UUID: %s)", d.Name, d.DevicePath, d.ID),
			Value: d.ID,
		})
	}
	if len(devOptions) == 0 {
		devOptions = append(devOptions, components.SelectOption{Label: "No Storage Devices Available", Value: ""})
	}

	fields := []components.FieldDefinition{
		{Type: components.FieldTypeSelect, Label: "Parent Storage Device", Value: initial.DevID, Options: devOptions},
		{Type: components.FieldTypeText, Label: "Partition Path", Placeholder: "e.g. /dev/sda1", Value: initial.PartitionPath, CharLimit: 128},
		{Type: components.FieldTypeText, Label: "Partition Size (GB)", Placeholder: "e.g. 100", Value: fmt.Sprintf("%d", initial.Size/(1024*1024*1024)), CharLimit: 32},
		{Type: components.FieldTypeText, Label: "Assigned NISD UUID", Placeholder: "Optional NISD UUID", Value: initial.NISDUUID, CharLimit: 64},
	}

	return &PartitionFormPage{
		Form:      components.NewForm(fields),
		Devices:   devs,
		IsEdit:    isEdit,
		InitialID: initial.PartitionID,
	}
}

func (p *PartitionFormPage) isFormPage() {}

func (p *PartitionFormPage) Init() tea.Cmd { return nil }

func (p *PartitionFormPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" && p.Form.FocusedIndex == len(p.Form.Inputs)-1 {
			devID := p.Form.Value(0)
			partPath := p.Form.Value(1)
			sizeGB := p.Form.Value(2)
			nisdUUID := p.Form.Value(3)
			return p, func() tea.Msg {
				return SubmitPartitionFormMsg{
					PartitionID:   p.InitialID,
					DeviceID:      devID,
					PartitionPath: partPath,
					SizeGB:        sizeGB,
					NISDUUID:      nisdUUID,
					IsEdit:        p.IsEdit,
				}
			}
		}
	}
	cmd := p.Form.Update(msg)
	return p, cmd
}

func (p *PartitionFormPage) View() string {
	title := "Create Device Partition"
	if p.IsEdit {
		title = "Edit Device Partition"
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render(title),
		"",
		p.Form.View(),
	)
}

func (p *PartitionFormPage) Title() string { return "Create Partition" }

func (p *PartitionFormPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Tab/Down"), key.WithHelp("Tab/Down", "Next Field")),
		key.NewBinding(key.WithKeys("◄/►/Space"), key.WithHelp("◄/►/Space", "Select Parent Device")),
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Submit Partition")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Cancel")),
	}
}

// NewPartitionViewPage builds an interactive table of Device Partitions;
// enter/space/v opens a detail card for the selected one.
func NewPartitionViewPage(hvs []domain.Hypervisor, width, height int, canWrite bool) *TablePage[domain.DevicePartition] {
	var allParts []domain.DevicePartition
	columns := []table.Column{
		{Title: "Partition Path", Width: 24},
		{Title: "Parent Device", Width: 20},
		{Title: "Size (GB)", Width: 14},
		{Title: "Assigned NISD UUID", Width: 38},
		{Title: "Partition ID", Width: 38},
	}

	var rows []table.Row
	for _, hv := range hvs {
		for _, dev := range hv.Dev {
			for _, part := range dev.Partitions {
				allParts = append(allParts, part)
				sizeGB := fmt.Sprintf("%.2f GB", float64(part.Size)/(1024*1024*1024))
				rows = append(rows, table.Row{
					part.PartitionPath, dev.Name, sizeGB, part.NISDUUID, part.PartitionID,
				})
			}
		}
	}

	actions := []key.Binding{
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "Create Partition")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "Edit Partition")),
	}

	return NewTablePage(allParts, columns, rows, width, height,
		lipgloss.Color("#FFFFFF"), styles.ColorPrimary, canWrite,
		"View All Partitions", "CONFIGURED DEVICE PARTITIONS TABLE", renderPartitionDetail, actions)
}

func renderPartitionDetail(part domain.DevicePartition) string {
	bullets := []string{
		fmt.Sprintf("  • Partition ID:      %s", part.PartitionID),
		fmt.Sprintf("  • Partition Path:    %s", part.PartitionPath),
		fmt.Sprintf("  • Parent Device ID:  %s", part.DevID),
		fmt.Sprintf("  • Capacity Size:     %.2f GB (%d bytes)", float64(part.Size)/(1024*1024*1024), part.Size),
		fmt.Sprintf("  • Assigned NISD UUID: %s", part.NISDUUID),
	}
	return detailLines("Partition Inspection View", "PARTITION DETAILS", lipgloss.Color("#FFFFFF"), lipgloss.Color("#10B981"),
		part.PartitionPath, bullets, "[ Press Esc or Enter to return to Partitions Table ]")
}
