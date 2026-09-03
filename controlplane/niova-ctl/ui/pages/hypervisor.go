package pages

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// validateHVPrimaryIP requires a real, well-formed IP — a blank Primary IP
// left a hypervisor unreachable over SSH in earlier testing. Required,
// unlike the other fields here.
func validateHVPrimaryIP(s string) error {
	if s == "" {
		return fmt.Errorf("required — SSH discovery/partitioning needs a real address")
	}
	if !domain.IsValidIP(s) {
		return fmt.Errorf("must be a valid IP address")
	}
	return nil
}

// validateHVAdditionalIPs allows empty (no additional IPs) but requires
// every comma-separated entry to parse as a real IP.
func validateHVAdditionalIPs(s string) error {
	if s == "" {
		return nil
	}
	for _, ip := range strings.Split(s, ",") {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		if !domain.IsValidIP(ip) {
			return fmt.Errorf("%q is not a valid IP address", ip)
		}
	}
	return nil
}

// validateHVSSHPort rejects non-digit characters outright (paired with
// BlockInvalidChars) and, once there's a full value, checks it's a real
// port number. Empty is fine — ssh.go falls back to :22.
func validateHVSSHPort(s string) error {
	for _, r := range s {
		if r < '0' || r > '9' {
			return fmt.Errorf("must contain only digits")
		}
	}
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("must be a port number from 1-65535")
	}
	return nil
}

// validateHVPortRange defers to domain.ParsePortRange — the exact same
// parser AllocatePortPair/NISD creation would use, so a range that fails
// here would fail there too.
func validateHVPortRange(s string) error {
	if s == "" {
		return nil
	}
	if _, _, err := domain.ParsePortRange(s); err != nil {
		return fmt.Errorf(`must be "start-end" with start < end (e.g. 8000-8100)`)
	}
	return nil
}

// isPortRangeChar rejects a keystroke immediately for any character that
// could never appear in a valid "start-end" value (paired with
// AllowedChars) — unlike validateHVPortRange itself, which needs the full
// value and so can't gate individual keystrokes without blocking every
// in-progress range on the way to one (e.g. "900" before "9000-9100").
func isPortRangeChar(r rune) bool {
	return (r >= '0' && r <= '9') || r == '-'
}

// SubmitHypervisorFormMsg emitted when Hypervisor form is submitted
type SubmitHypervisorFormMsg struct {
	HypervisorID  string
	Name          string
	RackID        string
	PrimaryIP     string
	AdditionalIPs string
	SSHPort       string
	PortRange     string
	IsEdit        bool
}

// HypervisorFormPage handles creation and editing of Hypervisors with Rack dropdown
type HypervisorFormPage struct {
	Form   components.FormModel
	Racks  []domain.Rack
	IsEdit bool
	// InitialID carries the edited Hypervisor's ID through submit — not a
	// visible form field. Without it, submit can't tell edit from create
	// and always mints a fresh UUID, duplicating the hypervisor.
	InitialID string
}

func NewHypervisorFormPage(isEdit bool, initial domain.Hypervisor, racks []domain.Rack) *HypervisorFormPage {
	primaryIP := ""
	if len(initial.IPAddrs) > 0 {
		primaryIP = initial.IPAddrs[0]
	}
	additionalIPs := ""
	if len(initial.IPAddrs) > 1 {
		additionalIPs = strings.Join(initial.IPAddrs[1:], ",")
	}

	var rackOptions []components.SelectOption
	rackOptions = append(rackOptions, components.SelectOption{Label: "None (Standalone Hypervisor)", Value: ""})
	for _, r := range racks {
		rackOptions = append(rackOptions, components.SelectOption{
			Label: fmt.Sprintf("%s (UUID: %s)", r.Name, r.ID),
			Value: r.ID,
		})
	}

	fields := []components.FieldDefinition{
		{Type: components.FieldTypeText, Label: "Hypervisor Name", Placeholder: "e.g. hv-node-01", Value: initial.Name, CharLimit: 64},
		{Type: components.FieldTypeSelect, Label: "Parent Rack", Value: initial.RackID, Options: rackOptions},
		{Type: components.FieldTypeText, Label: "Primary IP", Placeholder: "192.168.1.100", Value: primaryIP, CharLimit: 64,
			Validate: validateHVPrimaryIP},
		{Type: components.FieldTypeText, Label: "Additional IPs (comma-separated)", Placeholder: "192.168.1.101,192.168.2.100", Value: additionalIPs, CharLimit: 512,
			Validate: validateHVAdditionalIPs},
		{Type: components.FieldTypeText, Label: "SSH Port", Placeholder: "22", Value: initial.SSHPort, CharLimit: 10,
			Validate: validateHVSSHPort, BlockInvalidChars: true},
		{Type: components.FieldTypeText, Label: "Port Range", Placeholder: "8000-8100", Value: initial.PortRange, CharLimit: 32,
			Validate: validateHVPortRange, AllowedChars: isPortRangeChar},
	}
	return &HypervisorFormPage{
		Form:      components.NewForm(fields),
		Racks:     racks,
		IsEdit:    isEdit,
		InitialID: initial.ID,
	}
}

func (p *HypervisorFormPage) isFormPage() {}

func (p *HypervisorFormPage) Init() tea.Cmd { return nil }

func (p *HypervisorFormPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" && p.Form.FocusedIndex == len(p.Form.Inputs)-1 && p.Form.Valid() {
			name := p.Form.Value(0)
			rackID := p.Form.Value(1)
			ip := p.Form.Value(2)
			addIPs := p.Form.Value(3)
			sshPort := p.Form.Value(4)
			pRange := p.Form.Value(5)
			return p, func() tea.Msg {
				return SubmitHypervisorFormMsg{
					HypervisorID:  p.InitialID,
					Name:          name,
					RackID:        rackID,
					PrimaryIP:     ip,
					AdditionalIPs: addIPs,
					SSHPort:       sshPort,
					PortRange:     pRange,
					IsEdit:        p.IsEdit,
				}
			}
		}
	}

	cmd := p.Form.Update(msg)
	return p, cmd
}

func (p *HypervisorFormPage) View() string {
	title := "Create New Hypervisor Node"
	if p.IsEdit {
		title = "Edit Hypervisor Node"
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render(title),
		"",
		p.Form.View(),
	)
}

func (p *HypervisorFormPage) Title() string {
	if p.IsEdit {
		return "Edit Hypervisor"
	}
	return "Add Hypervisor"
}

func (p *HypervisorFormPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Tab/Down"), key.WithHelp("Tab/Down", "Next Field")),
		key.NewBinding(key.WithKeys("◄/►/Space"), key.WithHelp("◄/►/Space", "Select Rack Option")),
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Submit Hypervisor")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Cancel")),
	}
}

// NewHypervisorViewPage builds an interactive table of Hypervisors. The
// HV-ID -> parent-label map is resolved once here and captured by the
// detail-card renderer's closure, since TablePage has no notion of
// cross-resource lookups.
func NewHypervisorViewPage(pdus []domain.PDU, standaloneHVs []domain.Hypervisor, width, height int, canWrite bool) *TablePage[domain.Hypervisor] {
	var allHVs []domain.Hypervisor
	parentMap := make(map[string]string)

	columns := []table.Column{
		{Title: "Hypervisor", Width: 18},
		{Title: "Parent Rack / PDU", Width: 26},
		{Title: "IP Address", Width: 18},
		{Title: "SSH Port", Width: 10},
		{Title: "Port Range", Width: 14},
		{Title: "Devices", Width: 10},
		{Title: "UUID", Width: 38},
	}

	var rows []table.Row

	for _, pdu := range pdus {
		for _, rack := range pdu.Racks {
			for _, hv := range rack.Hypervisors {
				allHVs = append(allHVs, hv)
				parentStr := fmt.Sprintf("%s ➔ %s", rack.Name, pdu.Name)
				parentMap[hv.ID] = parentStr
				ipStr := domain.FormatIPAddresses(hv)
				rows = append(rows, table.Row{
					hv.Name, parentStr, ipStr, hv.SSHPort, hv.PortRange, fmt.Sprintf("%d", len(hv.Dev)), hv.ID,
				})
			}
		}
	}

	for _, hv := range standaloneHVs {
		allHVs = append(allHVs, hv)
		parentMap[hv.ID] = "Standalone (No Rack/PDU)"
		ipStr := domain.FormatIPAddresses(hv)
		rows = append(rows, table.Row{
			hv.Name, "Standalone", ipStr, hv.SSHPort, hv.PortRange, fmt.Sprintf("%d", len(hv.Dev)), hv.ID,
		})
	}

	actions := []key.Binding{
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "Add Hypervisor")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "Edit Hypervisor")),
	}

	// renderInspect doesn't use detailLines directly (unlike every other
	// resource) because Hypervisor shows an extra "Top Topology" badge
	// above the card that the others don't — it calls cardBody for the
	// card's own content and assembles the surrounding chrome itself.
	renderInspect := func(hv domain.Hypervisor) string {
		parentInfo := parentMap[hv.ID]
		badgePDU := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(styles.ColorPrimary).Bold(true).Padding(0, 1)

		bullets := []string{
			fmt.Sprintf("  • Node UUID:         %s", hv.ID),
			fmt.Sprintf("  • Parent Topology:   %s", parentInfo),
			fmt.Sprintf("  • IP Addresses:      %s", domain.FormatIPAddresses(hv)),
			fmt.Sprintf("  • SSH Port:          %s", hv.SSHPort),
			fmt.Sprintf("  • Port Range:        %s", hv.PortRange),
			fmt.Sprintf("  • RDMA Enabled:      %t", hv.RDMAEnabled),
			fmt.Sprintf("  • Attached Disks:    %d", len(hv.Dev)),
		}
		if len(hv.Dev) > 0 {
			bullets = append(bullets, "", styles.TextBold.Render("Storage Disks:"))
			for _, d := range hv.Dev {
				sizeGB := fmt.Sprintf("%.2f GB", float64(d.Size)/(1024*1024*1024))
				bullets = append(bullets, fmt.Sprintf("    ├─ %s (Path: %s, Size: %s, Partitions: %d)", d.Name, d.DevicePath, sizeGB, len(d.Partitions)))
			}
		}

		body := cardBody("HYPERVISOR DETAILS", lipgloss.Color("#FFFFFF"), lipgloss.Color("#3B82F6"),
			hv.Name, bullets, "[ Press Esc or Enter to return to Hypervisors Table ]")

		return lipgloss.JoinVertical(lipgloss.Left,
			styles.TextBold.Render("Hypervisor Inspection View"),
			"",
			badgePDU.Render("Top Topology: "+parentInfo),
			"",
			styles.Card.Render(body),
		)
	}

	return NewTablePage(allHVs, columns, rows, width, height,
		lipgloss.Color("#FFFFFF"), styles.ColorPrimary, canWrite,
		"View All Hypervisors", "CONFIGURED HYPERVISORS TABLE", renderInspect, actions)
}
