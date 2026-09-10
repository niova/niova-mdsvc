package pages

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// SubmitPartitionDiskFormMsg is emitted when the partition-count form is
// submitted for a device.
type SubmitPartitionDiskFormMsg struct {
	DeviceID     string
	HypervisorID string
	DevicePath   string
	Count        string
}

// PartitionDiskFormPage collects how many equal partitions to carve out of
// a device via SSH (domain.CreateMultipleEqualPartitions). A device-level
// SSH utility, not a control-plane resource.
type PartitionDiskFormPage struct {
	Form   components.FormModel
	Device domain.Device
}

func NewPartitionDiskFormPage(dev domain.Device) *PartitionDiskFormPage {
	fields := []components.FieldDefinition{
		{Type: components.FieldTypeText, Label: "Number of Equal Partitions", Placeholder: "e.g. 4", CharLimit: 4},
	}
	return &PartitionDiskFormPage{Form: components.NewForm(fields), Device: dev}
}

func (p *PartitionDiskFormPage) isFormPage() {}

func (p *PartitionDiskFormPage) Init() tea.Cmd { return nil }

func (p *PartitionDiskFormPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" {
			count := p.Form.Value(0)
			return p, func() tea.Msg {
				return SubmitPartitionDiskFormMsg{
					DeviceID:     p.Device.ID,
					HypervisorID: p.Device.HypervisorID,
					DevicePath:   p.Device.DevicePath,
					Count:        count,
				}
			}
		}
	}
	cmd := p.Form.Update(msg)
	return p, cmd
}

func (p *PartitionDiskFormPage) View() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render("Partition Disk: "+p.Device.DevicePath),
		"",
		styles.TextMuted.Render("Creates N equal-sized partitions over SSH (parted). Only allowed on a"),
		styles.TextMuted.Render("device with no existing partitions — this (re)creates its partition table."),
		"",
		p.Form.View(),
	)
}

func (p *PartitionDiskFormPage) Title() string { return "Partition Disk" }

func (p *PartitionDiskFormPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Create Partitions")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Cancel")),
	}
}

// PartitionDiskResultPage shows the partition paths CreateMultipleEqualPartitions
// just laid down physically over SSH. It's a report, not a registration
// step — go to 5. Partitions → a (Discover Partitions) next to register any
// of these with the control plane, which is what makes them selectable as
// NISD targets.
type PartitionDiskResultPage struct {
	DevicePath string
	Partitions []domain.DevicePartitionInfo
}

func NewPartitionDiskResultPage(devicePath string, parts []domain.DevicePartitionInfo) *PartitionDiskResultPage {
	return &PartitionDiskResultPage{DevicePath: devicePath, Partitions: parts}
}

func (p *PartitionDiskResultPage) isFormPage() {}

func (p *PartitionDiskResultPage) Init() tea.Cmd { return nil }

func (p *PartitionDiskResultPage) Update(msg tea.Msg) (Page, tea.Cmd) { return p, nil }

func (p *PartitionDiskResultPage) View() string {
	lines := []string{
		styles.TextBold.Render("Partitions Created: " + p.DevicePath),
		"",
		fmt.Sprintf("%d partition(s) created on disk:", len(p.Partitions)),
		"",
	}
	for _, part := range p.Partitions {
		sizeGB := fmt.Sprintf("%.2f GB", float64(part.Size)/(1024*1024*1024))
		lines = append(lines, fmt.Sprintf("  • /dev/%s (%s)", part.Name, sizeGB))
	}
	lines = append(lines, "",
		styles.TextMuted.Render("Go to 5. Partitions → a to register these with the control plane,"),
		styles.TextMuted.Render("making them available as NISD targets. Press Esc to return to Disks."))
	return styles.Card.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (p *PartitionDiskResultPage) Title() string { return "Partition Result" }

func (p *PartitionDiskResultPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Back to Disks")),
	}
}
