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

// SubmitDeviceFormMsg emitted when Device form is submitted
type SubmitDeviceFormMsg struct {
	DeviceID     string
	Name         string
	HypervisorID string
	DevicePath   string
	SerialNumber string
	SizeGB       string
	IsEdit       bool
}

// DeviceFormPage handles creation of Storage Devices with Hypervisor dropdown
type DeviceFormPage struct {
	Form        components.FormModel
	Hypervisors []domain.Hypervisor
	IsEdit      bool
	// InitialID carries the edited Device's ID through submit — see
	// HypervisorFormPage.InitialID for why this matters.
	InitialID string
}

func NewDeviceFormPage(isEdit bool, initial domain.Device, hvs []domain.Hypervisor) *DeviceFormPage {
	var hvOptions []components.SelectOption
	for _, hv := range hvs {
		hvOptions = append(hvOptions, components.SelectOption{
			Label: fmt.Sprintf("%s (UUID: %s)", hv.Name, hv.ID),
			Value: hv.ID,
		})
	}
	if len(hvOptions) == 0 {
		hvOptions = append(hvOptions, components.SelectOption{Label: "No Hypervisors Available", Value: ""})
	}

	fields := []components.FieldDefinition{
		{Type: components.FieldTypeText, Label: "Device Name", Placeholder: "e.g. sda", Value: initial.Name, CharLimit: 64},
		{Type: components.FieldTypeSelect, Label: "Host Hypervisor", Value: initial.HypervisorID, Options: hvOptions},
		{Type: components.FieldTypeText, Label: "Device Path", Placeholder: "e.g. /dev/sda", Value: initial.DevicePath, CharLimit: 128},
		{Type: components.FieldTypeText, Label: "Serial Number", Placeholder: "e.g. SN-12345678", Value: initial.SerialNumber, CharLimit: 128},
		{Type: components.FieldTypeText, Label: "Size (in GB)", Placeholder: "e.g. 500", Value: fmt.Sprintf("%d", initial.Size/(1024*1024*1024)), CharLimit: 32},
	}

	return &DeviceFormPage{
		Form:        components.NewForm(fields),
		Hypervisors: hvs,
		IsEdit:      isEdit,
		InitialID:   initial.ID,
	}
}

func (p *DeviceFormPage) isFormPage() {}

func (p *DeviceFormPage) Init() tea.Cmd { return nil }

func (p *DeviceFormPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" && p.Form.FocusedIndex == len(p.Form.Inputs)-1 {
			name := p.Form.Value(0)
			hvID := p.Form.Value(1)
			devPath := p.Form.Value(2)
			sn := p.Form.Value(3)
			sizeGB := p.Form.Value(4)
			return p, func() tea.Msg {
				return SubmitDeviceFormMsg{
					DeviceID:     p.InitialID,
					Name:         name,
					HypervisorID: hvID,
					DevicePath:   devPath,
					SerialNumber: sn,
					SizeGB:       sizeGB,
					IsEdit:       p.IsEdit,
				}
			}
		}
	}
	cmd := p.Form.Update(msg)
	return p, cmd
}

func (p *DeviceFormPage) View() string {
	title := "Initialize New Storage Device"
	if p.IsEdit {
		title = "Edit Storage Device"
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render(title),
		"",
		p.Form.View(),
	)
}

func (p *DeviceFormPage) Title() string { return "Initialize Device" }

func (p *DeviceFormPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Tab/Down"), key.WithHelp("Tab/Down", "Next Field")),
		key.NewBinding(key.WithKeys("◄/►/Space"), key.WithHelp("◄/►/Space", "Select Hypervisor")),
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Submit Device")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Cancel")),
	}
}

// NewDeviceViewPage builds an interactive table of Storage Devices; enter/
// space/v opens a detail card for the selected one.
func NewDeviceViewPage(hvs []domain.Hypervisor, width, height int, canWrite bool) *TablePage[domain.Device] {
	var allDevs []domain.Device
	columns := []table.Column{
		{Title: "Device Name", Width: 18},
		{Title: "Device Path", Width: 22},
		{Title: "Size (GB)", Width: 14},
		{Title: "Serial Number", Width: 22},
		{Title: "Partitions", Width: 12},
		{Title: "Device UUID", Width: 38},
	}

	var rows []table.Row
	for _, hv := range hvs {
		for _, dev := range hv.Dev {
			allDevs = append(allDevs, dev)
			sizeGB := fmt.Sprintf("%.2f GB", float64(dev.Size)/(1024*1024*1024))
			rows = append(rows, table.Row{
				dev.Name, dev.DevicePath, sizeGB, dev.SerialNumber, fmt.Sprintf("%d", len(dev.Partitions)), dev.ID,
			})
		}
	}

	actions := []key.Binding{
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "Initialize Disk")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "Edit Disk")),
		key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "Split Disk into Partitions")),
	}

	return NewTablePage(allDevs, columns, rows, width, height,
		lipgloss.Color("#000000"), styles.ColorSecondary, canWrite,
		"View All Devices", "INITIALIZED STORAGE DEVICES TABLE", renderDeviceDetail, actions)
}

func renderDeviceDetail(dev domain.Device) string {
	bullets := []string{
		fmt.Sprintf("  • Device UUID:       %s", dev.ID),
		fmt.Sprintf("  • Device Path:       %s", dev.DevicePath),
		fmt.Sprintf("  • Serial Number:     %s", dev.SerialNumber),
		fmt.Sprintf("  • Capacity Size:     %.2f GB (%d bytes)", float64(dev.Size)/(1024*1024*1024), dev.Size),
		fmt.Sprintf("  • Parent Hypervisor: %s", dev.HypervisorID),
		fmt.Sprintf("  • Partitions Count:  %d", len(dev.Partitions)),
	}
	if len(dev.Partitions) > 0 {
		bullets = append(bullets, "", styles.TextBold.Render("Partitions:"))
		for _, part := range dev.Partitions {
			pSizeGB := fmt.Sprintf("%.2f GB", float64(part.Size)/(1024*1024*1024))
			bullets = append(bullets, fmt.Sprintf("    ├─ %s (Size: %s, NISD UUID: %s)", part.PartitionPath, pSizeGB, part.NISDUUID))
		}
	}
	return detailLines("Device Inspection View", "STORAGE DEVICE DETAILS", lipgloss.Color("#000000"), lipgloss.Color("#F59E0B"),
		dev.Name, bullets, "[ Press Esc or Enter to return to Devices Table ]")
}
