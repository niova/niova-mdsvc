package pages

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

func configViewViewportHeight(termHeight int) int {
	headerHeight := components.HeaderHeight(nil, false, false)
	h := termHeight - headerHeight - 5
	if h < 4 {
		h = 4
	}
	return h
}

var (
	badgePDU = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(styles.ColorPrimary).
			Bold(true).
			Padding(0, 1)

	badgeRack = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(styles.ColorSecondary).
			Bold(true).
			Padding(0, 1)

	badgeHV = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#3B82F6")).
		Bold(true).
		Padding(0, 1)

	badgeDev = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#F59E0B")).
			Bold(true).
			Padding(0, 1)

	badgePart = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#10B981")).
			Bold(true).
			Padding(0, 1)
)

// ConfigViewPage handles tree representation of full Control Plane hierarchy
type ConfigViewPage struct {
	Viewport       viewport.Model
	PDUs           []domain.PDU
	StandaloneHVs  []domain.Hypervisor
	ExpandedPDUs   map[int]bool
	ExpandedRacks  map[string]bool
	ExpandedHypers map[string]bool
	ExpandedDevs   map[string]bool
	TermWidth      int
	TermHeight     int
	Ready          bool
}

func NewConfigViewPage(pdus []domain.PDU, standaloneHVs []domain.Hypervisor, width, height int) *ConfigViewPage {
	vp := viewport.New(width, configViewViewportHeight(height))
	p := &ConfigViewPage{
		Viewport:       vp,
		PDUs:           pdus,
		StandaloneHVs:  standaloneHVs,
		ExpandedPDUs:   make(map[int]bool),
		ExpandedRacks:  make(map[string]bool),
		ExpandedHypers: make(map[string]bool),
		ExpandedDevs:   make(map[string]bool),
		TermWidth:      width,
		TermHeight:     height,
		Ready:          true,
	}
	p.RefreshContent()
	return p
}

func (p *ConfigViewPage) Init() tea.Cmd { return nil }

func (p *ConfigViewPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.TermWidth = msg.Width
		p.TermHeight = msg.Height
		p.Viewport.Width = msg.Width
		p.Viewport.Height = configViewViewportHeight(msg.Height)
	}
	p.Viewport, cmd = p.Viewport.Update(msg)
	return p, cmd
}

func (p *ConfigViewPage) RefreshContent() {
	var sb strings.Builder
	sb.WriteString(styles.TextBold.Render("CONTROL PLANE FABRIC TOPOLOGY") + "\n\n")

	if len(p.PDUs) == 0 && len(p.StandaloneHVs) == 0 {
		sb.WriteString(styles.TextMuted.Render("No infrastructure entities configured in Control Plane."))
	} else {
		for _, pdu := range p.PDUs {
			var cardLines []string

			// PDU Header line with Badge
			pduTitle := fmt.Sprintf("%s %s", badgePDU.Render("PDU"), styles.SelectedItem.Render(pdu.Name))
			cardLines = append(cardLines, pduTitle)

			// PDU Details
			var pduMeta []string
			pduMeta = append(pduMeta, fmt.Sprintf("UUID: %s", pdu.ID))
			if strings.TrimSpace(pdu.Location) != "" {
				pduMeta = append(pduMeta, fmt.Sprintf("Location: %s", pdu.Location))
			}
			if strings.TrimSpace(pdu.PowerCapacity) != "" {
				pduMeta = append(pduMeta, fmt.Sprintf("Capacity: %s", pdu.PowerCapacity))
			}
			if strings.TrimSpace(pdu.Specification) != "" {
				pduMeta = append(pduMeta, fmt.Sprintf("Spec: %s", pdu.Specification))
			}
			cardLines = append(cardLines, styles.TextMuted.Render("  "+strings.Join(pduMeta, "  •  ")))

			// Racks under PDU
			if len(pdu.Racks) == 0 {
				cardLines = append(cardLines, styles.TextMuted.Render("  (No Racks attached)"))
			} else {
				for rIdx, rack := range pdu.Racks {
					rackPrefix := "  ├── "
					if rIdx == len(pdu.Racks)-1 {
						rackPrefix = "  └── "
					}
					rackHeader := fmt.Sprintf("%s%s %s (UUID: %s)",
						rackPrefix, badgeRack.Render("RACK"), styles.TextBold.Render(rack.Name), rack.ID)
					cardLines = append(cardLines, rackHeader)

					var rackMeta []string
					if strings.TrimSpace(rack.Location) != "" {
						rackMeta = append(rackMeta, fmt.Sprintf("Location: %s", rack.Location))
					}
					if strings.TrimSpace(rack.Specification) != "" {
						rackMeta = append(rackMeta, fmt.Sprintf("Spec: %s", rack.Specification))
					}
					if len(rackMeta) > 0 {
						cardLines = append(cardLines, styles.TextMuted.Render("  │   "+strings.Join(rackMeta, "  •  ")))
					}

					// Hypervisors under Rack
					for hIdx, hv := range rack.Hypervisors {
						hvPrefix := "  │   ├── "
						if hIdx == len(rack.Hypervisors)-1 {
							hvPrefix = "  │   └── "
						}
						ipStr := domain.FormatIPAddresses(hv)
						hvHeader := fmt.Sprintf("%s%s %s (IP: %s, Ports: %s)",
							hvPrefix, badgeHV.Render("HV"), styles.TextBold.Render(hv.Name), ipStr, hv.PortRange)
						cardLines = append(cardLines, hvHeader)

						// Devices under Hypervisor
						for dIdx, dev := range hv.Dev {
							devPrefix := "  │   │   ├── "
							if dIdx == len(hv.Dev)-1 {
								devPrefix = "  │   │   └── "
							}
							sizeFormatted := fmt.Sprintf("%.2f GB", float64(dev.Size)/(1024*1024*1024))
							devHeader := fmt.Sprintf("%s%s %s (%s, Path: %s)",
								devPrefix, badgeDev.Render("DEV"), dev.Name, sizeFormatted, dev.DevicePath)
							cardLines = append(cardLines, devHeader)

							// Partitions under Device
							for pIdx, part := range dev.Partitions {
								partPrefix := "  │   │   │   ├── "
								if pIdx == len(dev.Partitions)-1 {
									partPrefix = "  │   │   │   └── "
								}
								partSize := fmt.Sprintf("%.2f GB", float64(part.Size)/(1024*1024*1024))
								partHeader := fmt.Sprintf("%s%s Path: %s (Size: %s, NISD: %s)",
									partPrefix, badgePart.Render("PART"), part.PartitionPath, partSize, part.NISDUUID)
								cardLines = append(cardLines, partHeader)
							}
						}
					}
				}
			}

			// Render formatted PDU Card
			cardContent := strings.Join(cardLines, "\n")
			sb.WriteString(styles.Card.Render(cardContent) + "\n\n")
		}

		if len(p.StandaloneHVs) > 0 {
			var standaloneLines []string
			standaloneLines = append(standaloneLines, styles.TextBold.Render("Standalone Hypervisors"))
			for _, hv := range p.StandaloneHVs {
				ipStr := domain.FormatIPAddresses(hv)
				hvHeader := fmt.Sprintf("  ├─ %s %s (IP: %s, Ports: %s)",
					badgeHV.Render("HV"), styles.TextBold.Render(hv.Name), ipStr, hv.PortRange)
				standaloneLines = append(standaloneLines, hvHeader)
			}
			sb.WriteString(styles.Card.Render(strings.Join(standaloneLines, "\n")) + "\n")
		}
	}

	p.Viewport.SetContent(sb.String())
}

func (p *ConfigViewPage) View() string {
	p.RefreshContent()
	return lipgloss.JoinVertical(lipgloss.Left,
		p.Viewport.View(),
	)
}

func (p *ConfigViewPage) Title() string { return "View Configuration Hierarchy" }

func (p *ConfigViewPage) KeyBindings() []key.Binding {
	return nil
}

func (p *ConfigViewPage) SetViewportHeight(h int) { p.Viewport.Height = h }
