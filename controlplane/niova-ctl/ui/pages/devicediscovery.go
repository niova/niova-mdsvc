package pages

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// SubmitDiscoverDevicesMsg is emitted to (re)start an SSH device scan
// against a hypervisor (domain.DiscoverDevices) — on first load and again
// whenever the operator changes the Hypervisor selector.
type SubmitDiscoverDevicesMsg struct {
	Hypervisor domain.Hypervisor
}

// DevicesDiscoveredMsg reports the result of an SSH device scan.
type DevicesDiscoveredMsg struct {
	Hypervisor domain.Hypervisor
	Devices    []domain.Device
	Err        error
}

// DeviceDiscoverySelectedMsg is emitted when the operator picks "Enter
// Manually" to proceed to the blank Add Device form instead of confirming
// any discovered devices.
type DeviceDiscoverySelectedMsg struct {
	HypervisorID string
}

// SubmitDiscoveredDevicesMsg is emitted when the operator confirms one or
// more checked discovered devices (main.go's old multi-select "space to
// toggle, a/n select all/none, enter to confirm" flow) — these are written
// straight via PutDevice, same as main, with no per-device edit stop.
type SubmitDiscoveredDevicesMsg struct {
	HypervisorID string
	Devices      []domain.Device
}

// discoveryFocus tracks which of the page's two regions has keyboard focus.
type discoveryFocus int

const (
	focusHypervisor discoveryFocus = iota
	focusDevices
)

// DeviceDiscoveryPage is a single screen: a Hypervisor selector at the top
// (◄/►/Space to change) with the checkbox device list for whichever
// hypervisor is currently selected below it — changing the selector
// re-scans via SSH automatically. Tab switches focus between the two regions.
type DeviceDiscoveryPage struct {
	hvs     []domain.Hypervisor
	hvIndex int
	focus   discoveryFocus
	loading bool

	devices  []discoveredDevice // every SSH-discovered device, flagged
	devTable table.Model
	selected map[int]bool
	errMsg   string
}

// discoveredDevice pairs an SSH-discovered device with whether it's already
// initialized on this hypervisor. Shown in the list either way, so the
// operator sees the whole disk inventory at a glance — only AlreadyAdded ==
// false rows are selectable.
type discoveredDevice struct {
	domain.Device
	AlreadyAdded bool
}

// NewDeviceDiscoveryPage builds the page and, if there's at least one
// hypervisor, the tea.Cmd that kicks off SSH discovery against the first
// one. Callers must run that cmd immediately — pages.Page has no Init()
// hook the app's Update loop invokes.
func NewDeviceDiscoveryPage(hvs []domain.Hypervisor) (*DeviceDiscoveryPage, tea.Cmd) {
	p := &DeviceDiscoveryPage{hvs: hvs, devTable: buildDeviceTable(nil, nil)}
	return p, p.discoverCmd()
}

func (p *DeviceDiscoveryPage) isFormPage() {}

// currentHV returns the hypervisor the selector is on, or the zero value if
// none exist.
func (p *DeviceDiscoveryPage) currentHV() domain.Hypervisor {
	if p.hvIndex >= 0 && p.hvIndex < len(p.hvs) {
		return p.hvs[p.hvIndex]
	}
	return domain.Hypervisor{}
}

// discoverCmd (re)starts an SSH scan against the currently selected
// hypervisor. Returns nil if there's nothing to scan.
func (p *DeviceDiscoveryPage) discoverCmd() tea.Cmd {
	if len(p.hvs) == 0 {
		return nil
	}
	p.loading = true
	p.errMsg = ""
	p.devices = nil
	p.selected = make(map[int]bool)
	p.devTable = buildDeviceTable(nil, nil)
	hv := p.currentHV()
	return func() tea.Msg { return SubmitDiscoverDevicesMsg{Hypervisor: hv} }
}

// annotateAlreadyInitialized flags discovered devices already attached to
// this hypervisor (matched by ID, or DevicePath as a fallback), shown
// alongside the addable ones rather than hidden. Returns the annotated
// list plus how many were flagged.
func annotateAlreadyInitialized(discovered []domain.Device, existing []domain.Device) ([]discoveredDevice, int) {
	existingIDs := make(map[string]bool, len(existing))
	existingPaths := make(map[string]bool, len(existing))
	for _, e := range existing {
		if e.ID != "" {
			existingIDs[e.ID] = true
		}
		if e.DevicePath != "" {
			existingPaths[e.DevicePath] = true
		}
	}
	rows := make([]discoveredDevice, len(discovered))
	alreadyAdded := 0
	for i, d := range discovered {
		already := existingIDs[d.ID] || existingPaths[d.DevicePath]
		rows[i] = discoveredDevice{Device: d, AlreadyAdded: already}
		if already {
			alreadyAdded++
		}
	}
	return rows, alreadyAdded
}

func buildDeviceTable(devices []discoveredDevice, selected map[int]bool) table.Model {
	cols := []table.Column{
		{Title: "Sel", Width: 3},
		{Title: "Device Path", Width: 20},
		{Title: "Size", Width: 14},
		{Title: "Serial Number", Width: 24},
		{Title: "Partitions", Width: 36},
	}
	return newSelectableTable(cols, deviceRows(devices, selected))
}

// deviceRows builds the checkbox+data rows for buildDeviceTable, and is
// also called via devTable.SetRows to refresh just the checkbox state.
// Already-added devices render with a "—" mark and an "(already added)"
// suffix instead of a real checkbox, since they can't be toggled.
func deviceRows(devices []discoveredDevice, selected map[int]bool) []table.Row {
	var rows []table.Row
	for i, d := range devices {
		mark := "[ ]"
		path := d.DevicePath
		switch {
		case d.AlreadyAdded:
			mark = "—"
			path += " (already added)"
		case selected[i]:
			mark = "[x]"
		}
		sizeGB := fmt.Sprintf("%.2f GB", float64(d.Size)/(1024*1024*1024))
		rows = append(rows, table.Row{mark, path, sizeGB, d.SerialNumber, partitionSummary(d)})
	}
	rows = append(rows, table.Row{"", "— Enter Manually —", "", "", ""})
	return rows
}

// partitionSummary renders a device's OS-level partitions as their short
// suffixes (e.g. "p1,p2,p3,p4" rather than the full "/dev/nvme6n1p1" paths),
// matching what an operator would see from a plain `lsblk`, and truncates to
// fit the column instead of overflowing it when a disk has many partitions.
func partitionSummary(d discoveredDevice) string {
	if len(d.Partitions) == 0 {
		return "-"
	}
	suffixes := make([]string, len(d.Partitions))
	for i, p := range d.Partitions {
		s := strings.TrimPrefix(p.PartitionPath, d.DevicePath)
		if s == "" || s == p.PartitionPath {
			s = filepath.Base(p.PartitionPath)
		}
		suffixes[i] = s
	}

	const maxWidth = 34 // column Width (36) minus a little padding
	joined := strings.Join(suffixes, ",")
	if len(joined) <= maxWidth {
		return joined
	}
	shown, length := 0, 0
	for _, s := range suffixes {
		add := len(s) + 1
		if length+add > maxWidth {
			break
		}
		length += add
		shown++
	}
	if shown == 0 {
		shown = 1
	}
	return fmt.Sprintf("%s +%d", strings.Join(suffixes[:shown], ","), len(suffixes)-shown)
}

// newSelectableTable builds a table.Model with this page's shared styling
// and a height that hugs its row count instead of a fixed constant.
func newSelectableTable(cols []table.Column, rows []table.Row) table.Model {
	h := len(rows) + 2
	if h < 4 {
		h = 4
	}
	if h > 12 {
		h = 12
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true), table.WithHeight(h))
	t.KeyMap.LineDown.SetKeys("down", "j")
	s := table.DefaultStyles()
	s.Header = s.Header.BorderStyle(lipgloss.NormalBorder()).BorderForeground(styles.ColorMuted).BorderBottom(true).Bold(true)
	s.Selected = s.Selected.Foreground(lipgloss.Color("#FFFFFF")).Background(styles.ColorPrimary).Bold(true)
	t.SetStyles(s)
	return t
}

func (p *DeviceDiscoveryPage) Init() tea.Cmd { return nil }

func (p *DeviceDiscoveryPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if dm, ok := msg.(DevicesDiscoveredMsg); ok {
		// A scan for a hypervisor the operator has since navigated away
		// from (changed the selector again before this one returned) —
		// drop it instead of clobbering whatever's now on screen.
		if dm.Hypervisor.ID != p.currentHV().ID {
			return p, nil
		}
		p.loading = false
		annotated, _ := annotateAlreadyInitialized(dm.Devices, dm.Hypervisor.Dev)
		p.devices = annotated
		p.selected = make(map[int]bool)
		p.errMsg = ""
		switch {
		case dm.Err != nil:
			p.errMsg = "SSH discovery failed: " + dm.Err.Error() + " — enter manually below."
		case len(annotated) == 0:
			p.errMsg = "No devices discovered on this hypervisor — enter manually below."
		}
		p.devTable = buildDeviceTable(p.devices, p.selected)
		return p, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return p, nil
	}

	if p.loading {
		// Ignore input while the SSH scan is in flight (Esc still works via
		// the App-level cancelForm() fallback, since this key isn't matched
		// by any App-level case either).
		return p, nil
	}

	if keyMsg.String() == "tab" || keyMsg.String() == "shift+tab" {
		if p.focus == focusHypervisor {
			p.focus = focusDevices
		} else {
			p.focus = focusHypervisor
		}
		return p, nil
	}

	if p.focus == focusHypervisor {
		switch keyMsg.String() {
		case "left", "h":
			if len(p.hvs) > 1 {
				p.hvIndex = (p.hvIndex - 1 + len(p.hvs)) % len(p.hvs)
				return p, p.discoverCmd()
			}
		case "right", "l", "space":
			if len(p.hvs) > 1 {
				p.hvIndex = (p.hvIndex + 1) % len(p.hvs)
				return p, p.discoverCmd()
			}
		case "down":
			p.focus = focusDevices
		}
		return p, nil
	}

	// focus == focusDevices
	cursor := p.devTable.Cursor()
	hvID := p.currentHV().ID
	onManualRow := cursor >= len(p.devices)

	switch keyMsg.String() {
	case "up":
		if cursor == 0 {
			p.focus = focusHypervisor
			return p, nil
		}
	case " ":
		// Toggle the highlighted device — no-op on the trailing "Enter
		// Manually" row and on already-added devices, neither of which is
		// checkable.
		if !onManualRow && !p.devices[cursor].AlreadyAdded {
			p.selected[cursor] = !p.selected[cursor]
			p.devTable.SetRows(deviceRows(p.devices, p.selected))
		}
		return p, nil
	case "a":
		for i, d := range p.devices {
			if !d.AlreadyAdded {
				p.selected[i] = true
			}
		}
		p.devTable.SetRows(deviceRows(p.devices, p.selected))
		return p, nil
	case "n":
		p.selected = make(map[int]bool)
		p.devTable.SetRows(deviceRows(p.devices, p.selected))
		return p, nil
	case "enter":
		if onManualRow {
			return p, func() tea.Msg {
				return DeviceDiscoverySelectedMsg{HypervisorID: hvID}
			}
		}
		var chosen []domain.Device
		for i, sel := range p.selected {
			if sel && i < len(p.devices) {
				chosen = append(chosen, p.devices[i].Device)
			}
		}
		// Nothing checked — Enter on a device row still confirms just that
		// one, so a single-disk pick doesn't require space first. Doesn't
		// apply to an already-added row, which isn't confirmable at all.
		if len(chosen) == 0 && !p.devices[cursor].AlreadyAdded {
			chosen = []domain.Device{p.devices[cursor].Device}
		}
		if len(chosen) == 0 {
			return p, nil
		}
		return p, func() tea.Msg {
			return SubmitDiscoveredDevicesMsg{HypervisorID: hvID, Devices: chosen}
		}
	}
	var cmd tea.Cmd
	p.devTable, cmd = p.devTable.Update(msg)
	return p, cmd
}

// hvSelectorView renders the Hypervisor field in the same "◄ Label ►" style
// components.FormModel uses for FieldTypeSelect, so this page reads like
// the same kind of field the rest of niova-ctl's forms use.
func (p *DeviceDiscoveryPage) hvSelectorView() string {
	optLabel := "No Hypervisors Available"
	if hv := p.currentHV(); hv.ID != "" {
		optLabel = fmt.Sprintf("%s (%s)", hv.Name, domain.FormatIPAddresses(hv))
	}
	return renderDiscoverySelector(p.focus == focusHypervisor, "Hypervisor", optLabel, len(p.hvs))
}

func (p *DeviceDiscoveryPage) View() string {
	sections := []string{
		styles.TextBold.Render("INITIALIZE DISK"),
		"",
		styles.Card.Render(p.hvSelectorView()),
		"",
	}

	switch {
	case len(p.hvs) == 0:
		sections = append(sections, styles.TextMuted.Render("No hypervisors available — create one first."))
	case p.loading:
		sections = append(sections, fmt.Sprintf("Connecting to %s via SSH...", p.currentHV().Name))
	default:
		selectable, alreadyAdded := 0, 0
		for _, d := range p.devices {
			if d.AlreadyAdded {
				alreadyAdded++
			} else {
				selectable++
			}
		}
		countSuffix := fmt.Sprintf("(%d found)", len(p.devices))
		if alreadyAdded > 0 {
			countSuffix = fmt.Sprintf("(%d found — %d already added)", len(p.devices), alreadyAdded)
		}
		header := fmt.Sprintf("%s %s", styles.TextBold.Render("DISCOVERED DEVICES"), styles.TextMuted.Render(countSuffix))
		sections = append(sections, header, "")
		if p.errMsg != "" {
			sections = append(sections, styles.TextMuted.Render(p.errMsg), "")
		}
		sections = append(sections, styles.Card.Render(p.devTable.View()))
		if selectable > 0 {
			selCount := 0
			for _, sel := range p.selected {
				if sel {
					selCount++
				}
			}
			sections = append(sections, "", styles.TextMuted.Render(fmt.Sprintf("Selected: %d/%d devices — Space toggle, a select all, n select none", selCount, selectable)))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (p *DeviceDiscoveryPage) Title() string { return "Initialize Disk" }

func (p *DeviceDiscoveryPage) KeyBindings() []key.Binding {
	bindings := []key.Binding{
		key.NewBinding(key.WithKeys("Tab"), key.WithHelp("Tab", "Switch Hypervisor/Devices")),
	}
	if p.focus == focusHypervisor {
		bindings = append(bindings, key.NewBinding(key.WithKeys("◄/►"), key.WithHelp("◄/►", "Change Hypervisor")))
	} else {
		bindings = append(bindings,
			key.NewBinding(key.WithKeys("Down/Up"), key.WithHelp("Down/Up", "Move Cursor")),
			key.NewBinding(key.WithKeys("Space"), key.WithHelp("Space", "Toggle Device")),
			key.NewBinding(key.WithKeys("a/n"), key.WithHelp("a/n", "Select All/None")),
			key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Add Selected / Enter Manually")),
		)
	}
	bindings = append(bindings, key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Cancel")))
	return bindings
}

func (p *DeviceDiscoveryPage) SetTableHeight(h int) {
	p.devTable.SetHeight(h)
}
