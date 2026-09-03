package pages

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// validateNISDUUID accepts empty (auto-generated on submit) or a real UUID.
func validateNISDUUID(s string) error {
	if s == "" {
		return nil
	}
	if _, err := uuid.Parse(s); err != nil {
		return fmt.Errorf("must be a valid UUID, or empty to auto-generate")
	}
	return nil
}

// validatePeerPort rejects non-digit characters outright (paired with
// BlockInvalidChars) and, once there's a full value, checks it's a real
// port number.
func validatePeerPort(s string) error {
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

// validateSocketPath requires an absolute Unix path (no NUL bytes, which
// are the one byte no POSIX path can ever contain).
func validateSocketPath(s string) error {
	if s == "" {
		return nil
	}
	if !strings.HasPrefix(s, "/") {
		return fmt.Errorf("must be an absolute path (start with /)")
	}
	if strings.ContainsRune(s, 0) {
		return fmt.Errorf("contains an invalid character")
	}
	return nil
}

// SubmitNISDFormMsg emitted when NISD form is submitted. On create
// (IsEdit false) NISDUUID/PeerPort/SocketPath are always blank — the
// submit handler derives them. TargetNISDUUID/TargetHypervisorID carry
// the selected NISDTarget's fields through so it needn't re-walk the topology.
// Initial is the pre-edit record (IsEdit only) — the submit handler carries
// its TotalSize/AvailableSize/NetInfo through unchanged, since the edit form
// only ever collects NISD UUID/Peer Port/Socket Path and PutNisd overwrites
// the whole record rather than merging.
type SubmitNISDFormMsg struct {
	TargetID           string
	FailureDomain      []string
	TargetNISDUUID     string
	TargetHypervisorID string
	TargetSize         int64
	NISDUUID           string
	PeerPort           string
	SocketPath         string
	IsEdit             bool
	Initial            domain.Nisd
}

// NISDFormPage handles creation/initialization of NISD instances. Its
// target picker is backend-aware: a Partition dropdown on backends that
// have one (niova-mdsvc), a Device dropdown on backends that don't
// (mdsvc-tidb) — see domain.BuildNISDTargets.
type NISDFormPage struct {
	Form    components.FormModel
	Targets []domain.NISDTarget
	IsEdit  bool
	Initial domain.Nisd
}

func NewNISDFormPage(isEdit bool, initial domain.Nisd, targets []domain.NISDTarget, usePartitions bool) *NISDFormPage {
	var targetOptions []components.SelectOption
	for _, t := range targets {
		targetOptions = append(targetOptions, components.SelectOption{Label: t.Label, Value: t.TargetID})
	}
	targetLabel := "Target Device"
	noneLabel := "No Devices Available"
	if usePartitions {
		targetLabel = "Target Device Partition"
		noneLabel = "No Partitions Available"
	}
	if len(targetOptions) == 0 {
		targetOptions = append(targetOptions, components.SelectOption{Label: noneLabel, Value: ""})
	}

	// On edit, FailureDomain's last element is always the target's own ID
	// regardless of scheme (partition ID or device ID — see
	// domain.NISDTarget's doc), so this pre-selects the right option either way.
	initialTargetID := ""
	if len(initial.FailureDomain) > 0 {
		initialTargetID = initial.FailureDomain[len(initial.FailureDomain)-1]
	}

	peerPortStr := ""
	if initial.PeerPort > 0 {
		peerPortStr = fmt.Sprintf("%d", initial.PeerPort)
	}

	// On create, NISD UUID/Peer Port/Socket Path are never asked for — the
	// submit handler derives all three (pre-assigned or fresh UUID,
	// auto-allocated port, port-derived socket path). Editing an
	// already-registered NISD is the one case these stay editable.
	fields := []components.FieldDefinition{
		{Type: components.FieldTypeSelect, Label: targetLabel, Value: initialTargetID, Options: targetOptions},
	}
	if isEdit {
		fields = append(fields,
			components.FieldDefinition{Type: components.FieldTypeText, Label: "NISD UUID", Placeholder: "Auto-generated if empty", Value: initial.ID, CharLimit: 64,
				Validate: validateNISDUUID},
			components.FieldDefinition{Type: components.FieldTypeText, Label: "Peer Port", Placeholder: "e.g. 9090", Value: peerPortStr, CharLimit: 10,
				Validate: validatePeerPort, BlockInvalidChars: true},
			components.FieldDefinition{Type: components.FieldTypeText, Label: "Socket Path", Placeholder: "e.g. /run/niova/nisd.sock", Value: initial.SocketPath, CharLimit: 128,
				Validate: validateSocketPath},
		)
	}

	return &NISDFormPage{
		Form:    components.NewForm(fields),
		Targets: targets,
		IsEdit:  isEdit,
		Initial: initial,
	}
}

func (p *NISDFormPage) isFormPage() {}

func (p *NISDFormPage) Init() tea.Cmd { return nil }

func (p *NISDFormPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" && p.Form.FocusedIndex == len(p.Form.Inputs)-1 && p.Form.Valid() {
			targetID := p.Form.Value(0)
			var nisdUUID, peerPort, sockPath string
			if p.IsEdit {
				nisdUUID = p.Form.Value(1)
				peerPort = p.Form.Value(2)
				sockPath = p.Form.Value(3)
			}
			var fd []string
			var targetNISDUUID, targetHVID string
			var targetSize int64
			for _, t := range p.Targets {
				if t.TargetID == targetID {
					fd = t.FailureDomain
					targetNISDUUID = t.NISDUUID
					targetHVID = t.HypervisorID
					targetSize = t.Size
					break
				}
			}
			return p, func() tea.Msg {
				return SubmitNISDFormMsg{
					TargetID:           targetID,
					FailureDomain:      fd,
					TargetNISDUUID:     targetNISDUUID,
					TargetHypervisorID: targetHVID,
					TargetSize:         targetSize,
					NISDUUID:           nisdUUID,
					PeerPort:           peerPort,
					SocketPath:         sockPath,
					IsEdit:             p.IsEdit,
					Initial:            p.Initial,
				}
			}
		}
	}
	cmd := p.Form.Update(msg)
	return p, cmd
}

func (p *NISDFormPage) View() string {
	title := "Initialize NISD Instance"
	if p.IsEdit {
		title = "Edit NISD Instance"
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render(title),
		"",
		p.Form.View(),
	)
}

func (p *NISDFormPage) Title() string { return "Initialize NISD" }

func (p *NISDFormPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Tab/Down"), key.WithHelp("Tab/Down", "Next Field")),
		key.NewBinding(key.WithKeys("◄/►/Space"), key.WithHelp("◄/►/Space", "Select Target")),
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Submit NISD")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Cancel")),
	}
}

// SubmitNISDBatchMsg is emitted when the operator confirms one or more
// checked targets on NISDBatchCreatePage — one NISD gets created per
// selected target. Mirrors main.go's old multi-select "space to toggle,
// a/n select all/none, enter to confirm" Initialize NISD flow, restored
// here for both backends: domain.BuildNISDTargets produces the same
// []NISDTarget shape whether usePartitions is true (niova-mdsvc) or false
// (mdsvc-tidb), so one page covers both.
type SubmitNISDBatchMsg struct {
	Targets []domain.NISDTarget
}

// nisdTargetOption pairs a NISDTarget with whether it already has a NISD
// registered against it. Shown in the list either way, so the operator
// sees the whole target inventory at a glance — only AlreadyUsed == false
// rows are selectable. Mirrors devicediscovery.go's discoveredDevice/
// AlreadyAdded exactly.
type nisdTargetOption struct {
	domain.NISDTarget
	AlreadyUsed bool
}

// NISDBatchCreatePage is the create-NISD entry point: a checkbox
// multi-select over every available target (Partition or Device), batch
// -creating one NISD per selection in a single confirm. Editing an
// existing NISD still goes through NISDFormPage, which collects real
// per-field corrections a batch flow has no business making.
type NISDBatchCreatePage struct {
	targets       []nisdTargetOption
	usePartitions bool
	selected      map[int]bool
	targetTable   table.Model
}

// NewNISDBatchCreatePage builds the page, flagging targets in usedTargetIDs
// (see domain.UsedNISDTargetIDs) as already having a NISD — shown but not
// selectable, so a partition/device already in use can't silently get a
// second, conflicting NISD created against it.
func NewNISDBatchCreatePage(targets []domain.NISDTarget, usedTargetIDs, existingNISDIDs map[string]bool, usePartitions bool) *NISDBatchCreatePage {
	options := make([]nisdTargetOption, len(targets))
	for i, t := range targets {
		// Two signals, covering what neither alone can:
		//   - usedTargetIDs (existing NISDs' FailureDomain) catches
		//     device-level reuse reliably, but is blind to partition-level
		//     reuse on mdsvc-tidb — its schema has no partition-level FK
		//     column, so that detail never survives a round trip.
		//   - t.NISDUUID != "" alone isn't trustworthy: that field is also
		//     how an operator pre-assigns a future NISD's UUID via Edit
		//     Partition before any NISD exists (main.go's original
		//     workflow). Cross-checking it against existingNISDIDs (real
		//     NISD IDs, from domain.NISDIDSet) tells "really created" apart
		//     from "pre-planned but not yet created" — closing the
		//     mdsvc-tidb partition-level gap without reintroducing the
		//     false positive on a pre-assigned-but-unused UUID.
		options[i] = nisdTargetOption{
			NISDTarget:  t,
			AlreadyUsed: usedTargetIDs[t.TargetID] || (t.NISDUUID != "" && existingNISDIDs[t.NISDUUID]),
		}
	}
	p := &NISDBatchCreatePage{targets: options, usePartitions: usePartitions, selected: make(map[int]bool)}
	p.targetTable = buildNISDTargetTable(options, p.selected)
	return p
}

func buildNISDTargetTable(targets []nisdTargetOption, selected map[int]bool) table.Model {
	cols := []table.Column{
		{Title: "Sel", Width: 3},
		{Title: "Target", Width: 60},
	}
	return newSelectableTable(cols, nisdTargetRows(targets, selected))
}

// nisdTargetRows builds the checkbox+data rows for buildNISDTargetTable, and
// is also called via targetTable.SetRows to refresh just the checkbox
// state. Already-used targets render with a "—" mark and an "(already has
// NISD)" suffix instead of a real checkbox, since they can't be toggled.
func nisdTargetRows(targets []nisdTargetOption, selected map[int]bool) []table.Row {
	rows := make([]table.Row, len(targets))
	for i, t := range targets {
		mark := "[ ]"
		label := t.Label
		switch {
		case t.AlreadyUsed:
			mark = "—"
			label += " (already has NISD)"
		case selected[i]:
			mark = "[x]"
		}
		rows[i] = table.Row{mark, label}
	}
	return rows
}

func (p *NISDBatchCreatePage) isFormPage() {}

func (p *NISDBatchCreatePage) Init() tea.Cmd { return nil }

func (p *NISDBatchCreatePage) Update(msg tea.Msg) (Page, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return p, nil
	}
	if len(p.targets) == 0 {
		return p, nil
	}
	cursor := p.targetTable.Cursor()
	switch keyMsg.String() {
	case " ":
		if cursor >= 0 && cursor < len(p.targets) && !p.targets[cursor].AlreadyUsed {
			p.selected[cursor] = !p.selected[cursor]
			p.targetTable.SetRows(nisdTargetRows(p.targets, p.selected))
		}
		return p, nil
	case "a":
		for i, t := range p.targets {
			if !t.AlreadyUsed {
				p.selected[i] = true
			}
		}
		p.targetTable.SetRows(nisdTargetRows(p.targets, p.selected))
		return p, nil
	case "n":
		p.selected = make(map[int]bool)
		p.targetTable.SetRows(nisdTargetRows(p.targets, p.selected))
		return p, nil
	case "enter":
		var chosen []domain.NISDTarget
		for i, sel := range p.selected {
			if sel && i < len(p.targets) {
				chosen = append(chosen, p.targets[i].NISDTarget)
			}
		}
		// Nothing checked — Enter still confirms just the highlighted row,
		// so a single-target pick doesn't require Space first (mirrors
		// DeviceDiscoveryPage/PartitionDiskResultPage's same convenience).
		// Doesn't apply to an already-used row, which isn't confirmable at all.
		if len(chosen) == 0 && cursor >= 0 && cursor < len(p.targets) && !p.targets[cursor].AlreadyUsed {
			chosen = []domain.NISDTarget{p.targets[cursor].NISDTarget}
		}
		if len(chosen) == 0 {
			return p, nil
		}
		return p, func() tea.Msg { return SubmitNISDBatchMsg{Targets: chosen} }
	}
	var cmd tea.Cmd
	p.targetTable, cmd = p.targetTable.Update(msg)
	return p, cmd
}

func (p *NISDBatchCreatePage) View() string {
	targetLabel := "devices"
	if p.usePartitions {
		targetLabel = "partitions"
	}
	if len(p.targets) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left,
			styles.TextBold.Render("Initialize NISD Instance"),
			"",
			styles.TextMuted.Render(fmt.Sprintf("No %s available — create one first.", targetLabel)),
		)
	}
	selectable, alreadyUsed := 0, 0
	for _, t := range p.targets {
		if t.AlreadyUsed {
			alreadyUsed++
		} else {
			selectable++
		}
	}
	selCount := 0
	for _, sel := range p.selected {
		if sel {
			selCount++
		}
	}
	countSuffix := ""
	if alreadyUsed > 0 {
		countSuffix = fmt.Sprintf(" (%d already have a NISD)", alreadyUsed)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render("Initialize NISD Instance"),
		"",
		styles.TextMuted.Render(fmt.Sprintf("Select one or more %s to initialize as NISDs%s:", targetLabel, countSuffix)),
		"",
		styles.Card.Render(p.targetTable.View()),
		"",
		styles.TextMuted.Render(fmt.Sprintf("Selected: %d/%d — Space toggle, a select all, n select none, Enter to create NISD(s)", selCount, selectable)),
	)
}

func (p *NISDBatchCreatePage) Title() string { return "Initialize NISD" }

func (p *NISDBatchCreatePage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Down/Up"), key.WithHelp("Down/Up", "Move Cursor")),
		key.NewBinding(key.WithKeys("Space"), key.WithHelp("Space", "Toggle Target")),
		key.NewBinding(key.WithKeys("a/n"), key.WithHelp("a/n", "Select All/None")),
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Create NISD(s)")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Cancel")),
	}
}

// NewNISDViewPage builds an interactive table of NISD instances; enter/
// space/v opens a detail card for the selected one.
func NewNISDViewPage(nisds []domain.Nisd, width, height int, canWrite bool) *TablePage[domain.Nisd] {
	columns := []table.Column{
		{Title: "NISD UUID", Width: 38},
		{Title: "Peer Port", Width: 14},
		{Title: "Socket Path", Width: 32},
		{Title: "Total Size", Width: 18},
	}

	var rows []table.Row
	for _, n := range nisds {
		sizeGB := fmt.Sprintf("%.2f GB", float64(n.TotalSize)/(1024*1024*1024))
		rows = append(rows, table.Row{
			n.ID, fmt.Sprintf("%d", n.PeerPort), n.SocketPath, sizeGB,
		})
	}

	actions := []key.Binding{
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "Initialize NISD")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "Edit NISD")),
	}

	return NewTablePage(nisds, columns, rows, width, height,
		lipgloss.Color("#FFFFFF"), styles.ColorPrimary, canWrite,
		"View All NISDs", "CONFIGURED NISD INSTANCES TABLE", renderNISDDetail, actions)
}

func renderNISDDetail(nisd domain.Nisd) string {
	bullets := []string{
		fmt.Sprintf("  • NISD UUID:         %s", nisd.ID),
		fmt.Sprintf("  • Peer Port:         %d", nisd.PeerPort),
		fmt.Sprintf("  • Client Socket Path: %s", nisd.SocketPath),
		fmt.Sprintf("  • Total Capacity:    %.2f GB (%d bytes)", float64(nisd.TotalSize)/(1024*1024*1024), nisd.TotalSize),
		fmt.Sprintf("  • Available Capacity:%.2f GB (%d bytes)", float64(nisd.AvailableSize)/(1024*1024*1024), nisd.AvailableSize),
	}
	return detailLines("NISD Instance Inspection View", "NISD INSTANCE DETAILS", lipgloss.Color("#FFFFFF"), lipgloss.Color("#8B5CF6"),
		nisd.ID, bullets, "[ Press Esc or Enter to return to NISD Table ]")
}
