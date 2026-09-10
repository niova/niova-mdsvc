package pages

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// SubmitDiscoverPartitionsMsg (re)starts an SSH scan for existing
// partitions on a device (domain.GetDevicePartitionInfo).
type SubmitDiscoverPartitionsMsg struct {
	Device     domain.Device
	Hypervisor domain.Hypervisor
}

// PartitionsDiscoveredMsg reports the result of that scan.
type PartitionsDiscoveredMsg struct {
	Device     domain.Device
	Partitions []domain.DevicePartitionInfo
	Err        error
}

// SubmitDiscoveredPartitionsMsg is emitted when the operator confirms one
// or more checked, already-existing partitions found on the device — each
// gets registered via PutPartition with a fresh NISD UUID, exactly what
// main.go's own Create Partition flow did.
type SubmitDiscoveredPartitionsMsg struct {
	DeviceID   string
	Partitions []domain.DevicePartitionInfo
}

// SubmitWholeDevicePartitionMsg is emitted when the operator confirms using
// the entire device as a single partition, because SSH discovery found
// nothing already on it.
type SubmitWholeDevicePartitionMsg struct {
	DeviceID string
}

type partitionDiscoveryFocus int

const (
	pdFocusDevice partitionDiscoveryFocus = iota
	pdFocusPartitions
)

// partitionOption pairs an SSH-discovered physical partition with whether
// it's already registered with the control plane. Shown in the list either
// way, so the operator sees the whole disk's partition inventory at a
// glance — only AlreadyRegistered == false rows are selectable. Mirrors
// devicediscovery.go's discoveredDevice/AlreadyAdded exactly.
type partitionOption struct {
	domain.DevicePartitionInfo
	AlreadyRegistered bool
}

// PartitionDiscoveryPage is a single screen: a Device selector at the top
// (◄/►/Space, see domain.BuildPartitionableDevices) and, below it,
// whatever SSH discovery finds on that device's disk. Deliberately no
// "type a path by hand" fallback — only registers what's actually there.
type PartitionDiscoveryPage struct {
	devices []domain.PartitionableDevice
	devIdx  int
	focus   partitionDiscoveryFocus
	loading bool

	partitions []partitionOption
	partTable  table.Model
	selected   map[int]bool
	errMsg     string
}

// NewPartitionDiscoveryPage builds the page and, if there's at least one
// eligible device, the tea.Cmd that kicks off SSH discovery against the
// first one — same reasoning as NewDeviceDiscoveryPage.
func NewPartitionDiscoveryPage(devices []domain.PartitionableDevice) (*PartitionDiscoveryPage, tea.Cmd) {
	p := &PartitionDiscoveryPage{devices: devices, partTable: buildPartitionTable(nil, nil)}
	return p, p.discoverCmd()
}

func (p *PartitionDiscoveryPage) isFormPage() {}

// currentDevice returns the device/hypervisor pair the selector is on, or
// zero values if there's nothing to select.
func (p *PartitionDiscoveryPage) currentDevice() (domain.Device, domain.Hypervisor, bool) {
	if p.devIdx >= 0 && p.devIdx < len(p.devices) {
		d := p.devices[p.devIdx]
		return d.Device, d.Hypervisor, true
	}
	return domain.Device{}, domain.Hypervisor{}, false
}

// discoverCmd (re)starts an SSH scan against the currently selected
// device. Returns nil if there's nothing to scan.
func (p *PartitionDiscoveryPage) discoverCmd() tea.Cmd {
	dev, hv, ok := p.currentDevice()
	if !ok {
		return nil
	}
	p.loading = true
	p.errMsg = ""
	p.partitions = nil
	p.selected = make(map[int]bool)
	p.partTable = buildPartitionTable(nil, nil)
	return func() tea.Msg { return SubmitDiscoverPartitionsMsg{Device: dev, Hypervisor: hv} }
}

func buildPartitionTable(parts []partitionOption, selected map[int]bool) table.Model {
	cols := []table.Column{
		{Title: "Sel", Width: 3},
		{Title: "Partition Path", Width: 20},
		{Title: "Size", Width: 14},
		{Title: "Partition ID (by-id)", Width: 40},
	}
	return newSelectableTable(cols, partitionRows(parts, selected))
}

// partitionRows builds the checkbox+data rows for buildPartitionTable, and
// is also called via partTable.SetRows to refresh just the checkbox state.
// Leads with Path (e.g. "/dev/nvme3n1p1") rather than the longer by-id name.
// Already-registered partitions render with a "—" mark and an "(already
// registered)" suffix instead of a real checkbox, since they can't be
// toggled — mirrors devicediscovery.go's deviceRows exactly.
func partitionRows(parts []partitionOption, selected map[int]bool) []table.Row {
	var rows []table.Row
	for i, part := range parts {
		mark := "[ ]"
		path := part.Path
		switch {
		case part.AlreadyRegistered:
			mark = "—"
			path += " (already registered)"
		case selected[i]:
			mark = "[x]"
		}
		sizeGB := fmt.Sprintf("%.2f GB", float64(part.Size)/(1024*1024*1024))
		rows = append(rows, table.Row{mark, path, sizeGB, part.Name})
	}
	return rows
}

// annotateAlreadyRegistered flags SSH-discovered physical partitions that
// are already registered with the control plane (matched by by-id name,
// which is what registration stores as PartitionID), shown alongside the
// still-registerable ones rather than hidden. Mirrors devicediscovery.go's
// annotateAlreadyInitialized exactly.
func annotateAlreadyRegistered(discovered []domain.DevicePartitionInfo, registered []domain.DevicePartition) []partitionOption {
	registeredIDs := make(map[string]bool, len(registered))
	for _, r := range registered {
		if r.PartitionID != "" {
			registeredIDs[r.PartitionID] = true
		}
	}
	rows := make([]partitionOption, len(discovered))
	for i, d := range discovered {
		rows[i] = partitionOption{DevicePartitionInfo: d, AlreadyRegistered: registeredIDs[d.Name]}
	}
	return rows
}

func (p *PartitionDiscoveryPage) Init() tea.Cmd { return nil }

func (p *PartitionDiscoveryPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if dm, ok := msg.(PartitionsDiscoveredMsg); ok {
		curDev, _, curOK := p.currentDevice()
		if !curOK || dm.Device.ID != curDev.ID {
			// A scan for a device the operator has since navigated away
			// from — drop it instead of clobbering what's now on screen.
			return p, nil
		}
		p.loading = false
		p.partitions = annotateAlreadyRegistered(dm.Partitions, curDev.Partitions)
		p.selected = make(map[int]bool)
		p.errMsg = ""
		if dm.Err != nil {
			p.errMsg = "SSH discovery failed: " + dm.Err.Error()
		}
		p.partTable = buildPartitionTable(p.partitions, p.selected)
		return p, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return p, nil
	}

	if p.loading {
		// Esc still works via the App-level cancelForm() fallback, since
		// this key isn't matched by any App-level case either.
		return p, nil
	}

	if keyMsg.String() == "tab" || keyMsg.String() == "shift+tab" {
		if p.focus == pdFocusDevice {
			p.focus = pdFocusPartitions
		} else {
			p.focus = pdFocusDevice
		}
		return p, nil
	}

	if p.focus == pdFocusDevice {
		switch keyMsg.String() {
		case "left", "h":
			if len(p.devices) > 1 {
				p.devIdx = (p.devIdx - 1 + len(p.devices)) % len(p.devices)
				return p, p.discoverCmd()
			}
		case "right", "l", "space":
			if len(p.devices) > 1 {
				p.devIdx = (p.devIdx + 1) % len(p.devices)
				return p, p.discoverCmd()
			}
		case "down":
			p.focus = pdFocusPartitions
		}
		return p, nil
	}

	// focus == pdFocusPartitions
	dev, _, devOK := p.currentDevice()
	if !devOK {
		return p, nil
	}
	cursor := p.partTable.Cursor()

	switch keyMsg.String() {
	case "up":
		if cursor == 0 {
			p.focus = pdFocusDevice
			return p, nil
		}
	case " ":
		// No-op on an already-registered row, same as devicediscovery.go.
		if cursor < len(p.partitions) && !p.partitions[cursor].AlreadyRegistered {
			p.selected[cursor] = !p.selected[cursor]
			p.partTable.SetRows(partitionRows(p.partitions, p.selected))
		}
		return p, nil
	case "a":
		for i, part := range p.partitions {
			if !part.AlreadyRegistered {
				p.selected[i] = true
			}
		}
		p.partTable.SetRows(partitionRows(p.partitions, p.selected))
		return p, nil
	case "n":
		p.selected = make(map[int]bool)
		p.partTable.SetRows(partitionRows(p.partitions, p.selected))
		return p, nil
	case "enter":
		if len(p.partitions) == 0 {
			// Nothing found on disk — the whole-device fallback, same as
			// main.go's "use whole device as one partition?" prompt.
			return p, func() tea.Msg { return SubmitWholeDevicePartitionMsg{DeviceID: dev.ID} }
		}
		var chosen []domain.DevicePartitionInfo
		for i, sel := range p.selected {
			if sel && i < len(p.partitions) {
				chosen = append(chosen, p.partitions[i].DevicePartitionInfo)
			}
		}
		// Nothing checked — Enter on a partition row still confirms just
		// that one, so a single-partition pick doesn't require space first.
		// Doesn't apply to an already-registered row, which isn't
		// confirmable at all.
		if len(chosen) == 0 && cursor < len(p.partitions) && !p.partitions[cursor].AlreadyRegistered {
			chosen = []domain.DevicePartitionInfo{p.partitions[cursor].DevicePartitionInfo}
		}
		if len(chosen) == 0 {
			return p, nil
		}
		return p, func() tea.Msg {
			return SubmitDiscoveredPartitionsMsg{DeviceID: dev.ID, Partitions: chosen}
		}
	}
	var cmd tea.Cmd
	p.partTable, cmd = p.partTable.Update(msg)
	return p, cmd
}

// deviceSelectorView renders the Parent Storage Device field in the same
// "◄ Label ►" style components.FormModel uses for FieldTypeSelect.
func (p *PartitionDiscoveryPage) deviceSelectorView() string {
	optLabel := "No Devices Available"
	if dev, hv, ok := p.currentDevice(); ok {
		optLabel = fmt.Sprintf("%s (%s on %s)", dev.Name, dev.DevicePath, hv.Name)
	}
	return renderDiscoverySelector(p.focus == pdFocusDevice, "Parent Storage Device", optLabel, len(p.devices))
}

func (p *PartitionDiscoveryPage) View() string {
	sections := []string{
		styles.TextBold.Render("CREATE DEVICE PARTITION"),
		"",
		styles.Card.Render(p.deviceSelectorView()),
		"",
	}

	switch {
	case len(p.devices) == 0:
		sections = append(sections, styles.TextMuted.Render("No devices available — initialize a disk first."))
	case p.loading:
		dev, hv, _ := p.currentDevice()
		sections = append(sections, fmt.Sprintf("Connecting to %s on %s via SSH...", dev.DevicePath, hv.Name))
	case len(p.partitions) == 0:
		sections = append(sections, styles.TextBold.Render("NO EXISTING PARTITIONS FOUND"), "")
		if p.errMsg != "" {
			sections = append(sections, styles.TextMuted.Render(p.errMsg), "")
		}
		sections = append(sections, styles.Card.Render(lipgloss.JoinVertical(lipgloss.Left,
			"This device has no partitions on disk yet.",
			"",
			"Press Enter to register the entire device as a single partition —",
			`the same "use whole device?" fallback main.go's Create Partition`,
			"flow offered when it found nothing to select from.",
		)))
	default:
		selectable, alreadyRegistered := 0, 0
		for _, part := range p.partitions {
			if part.AlreadyRegistered {
				alreadyRegistered++
			} else {
				selectable++
			}
		}
		countSuffix := fmt.Sprintf("(%d found)", len(p.partitions))
		if alreadyRegistered > 0 {
			countSuffix = fmt.Sprintf("(%d found — %d already registered)", len(p.partitions), alreadyRegistered)
		}
		header := fmt.Sprintf("%s %s", styles.TextBold.Render("PARTITIONS FOUND ON DISK"), styles.TextMuted.Render(countSuffix))
		sections = append(sections, header, "")
		if p.errMsg != "" {
			sections = append(sections, styles.TextMuted.Render(p.errMsg), "")
		}
		sections = append(sections, styles.Card.Render(p.partTable.View()))
		if selectable > 0 {
			selCount := 0
			for _, sel := range p.selected {
				if sel {
					selCount++
				}
			}
			sections = append(sections, "", styles.TextMuted.Render(fmt.Sprintf("Selected: %d/%d partitions — Space toggle, a select all, n select none, Enter to register", selCount, selectable)))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (p *PartitionDiscoveryPage) Title() string { return "Create Partition" }

func (p *PartitionDiscoveryPage) KeyBindings() []key.Binding {
	bindings := []key.Binding{
		key.NewBinding(key.WithKeys("Tab"), key.WithHelp("Tab", "Switch Device/Partitions")),
	}
	switch {
	case p.focus == pdFocusDevice:
		bindings = append(bindings, key.NewBinding(key.WithKeys("◄/►"), key.WithHelp("◄/►", "Change Device")))
	case len(p.partitions) > 0:
		bindings = append(bindings,
			key.NewBinding(key.WithKeys("Down/Up"), key.WithHelp("Down/Up", "Move Cursor")),
			key.NewBinding(key.WithKeys("Space"), key.WithHelp("Space", "Toggle Partition")),
			key.NewBinding(key.WithKeys("a/n"), key.WithHelp("a/n", "Select All/None")),
			key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Register Selected")),
		)
	default:
		bindings = append(bindings, key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Use Whole Device")))
	}
	bindings = append(bindings, key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Cancel")))
	return bindings
}

func (p *PartitionDiscoveryPage) SetTableHeight(h int) {
	p.partTable.SetHeight(h)
}
