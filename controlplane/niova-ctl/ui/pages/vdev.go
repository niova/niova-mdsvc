package pages

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/bubbles/key"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// validateDigitsMin rejects non-digit characters outright (paired with
// BlockInvalidChars) and, once there's a full value, requires it be >= min.
// Blank is only rejected when required — Parity Block Count is legitimately
// optional (0 for Replica), Pool Size and Data Block Count aren't.
func validateDigitsMin(min int, required bool) func(string) error {
	return func(s string) error {
		for _, r := range s {
			if r < '0' || r > '9' {
				return fmt.Errorf("must contain only digits")
			}
		}
		if s == "" {
			if required {
				return fmt.Errorf("required")
			}
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil || n < min {
			return fmt.Errorf("must be a number >= %d", min)
		}
		return nil
	}
}

// SubmitVdevFormMsg emitted when Vdev form is submitted
type SubmitVdevFormMsg struct {
	Name         string
	SizeGB       string
	Redundancy   string
	DataBlkCnt   string
	ParityBlkCnt string
	FilterType   string
	PFS          string
	IsEdit       bool
}

// VdevFormPage handles creation of Vdev pools with redundancy & placement pickers
type VdevFormPage struct {
	Form   components.FormModel
	IsEdit bool
}

// zeroAsBlank renders n as a form field's initial value, except it returns
// "" instead of "0" when creating fresh (isEdit false) — see NewVdevFormPage.
func zeroAsBlank(isEdit bool, n int64) string {
	if !isEdit && n == 0 {
		return ""
	}
	return fmt.Sprintf("%d", n)
}

func NewVdevFormPage(isEdit bool, initial domain.VdevConfig, pfsList []domain.PFS) *VdevFormPage {
	redundancyOpts := []components.SelectOption{
		{Label: "Replica (Mirroring)", Value: "replica"},
		{Label: "Erasure Coding (EC)", Value: "ec"},
	}

	filterOpts := []components.SelectOption{
		{Label: "Any / Auto-Placement", Value: "any"},
		{Label: "Filter by Rack", Value: "rack"},
		{Label: "Filter by PDU", Value: "pdu"},
		{Label: "Filter by Hypervisor", Value: "hv"},
	}

	pfsOpts := []components.SelectOption{
		{Label: "None / Unassigned", Value: ""},
	}
	for _, p := range pfsList {
		label := p.Name
		if label == "" {
			label = p.ID
		}
		val := p.Name
		if val == "" {
			val = p.ID
		}
		pfsOpts = append(pfsOpts, components.SelectOption{Label: label, Value: val})
	}

	redStr := "replica"
	if initial.Redundancy.IsEC() {
		redStr = "ec"
	}

	pfsVal := initial.PFSName
	if pfsVal == "" {
		pfsVal = initial.PFSID
	}

	// Blank rather than a literal "0" for a brand-new pool — the server
	// rejects DataBlkCnt/ParityBlkCnt=0, so pre-filling "0" just invites
	// submitting it unedited. Editing an existing pool still pre-fills.
	sizeStr := zeroAsBlank(isEdit, initial.Size/(1024*1024*1024))
	dataBlkStr := zeroAsBlank(isEdit, int64(initial.DataBlkCnt))
	parityBlkStr := zeroAsBlank(isEdit, int64(initial.ParityBlkCnt))

	fields := []components.FieldDefinition{
		{Type: components.FieldTypeText, Label: "Vdev Pool Name", Placeholder: "e.g. vdev-pool-01", Value: initial.Name, CharLimit: 64,
			Validate: validateResourceName, BlockInvalidChars: true},
		{Type: components.FieldTypeText, Label: "Pool Size (GB)", Placeholder: "e.g. 500", Value: sizeStr, CharLimit: 32,
			Validate: validateDigitsMin(1, true), BlockInvalidChars: true},
		{Type: components.FieldTypeSelect, Label: "Redundancy Mode", Value: redStr, Options: redundancyOpts},
		{Type: components.FieldTypeText, Label: "Data Block Count (K)", Placeholder: "e.g. 4 — required, must be >= 1", Value: dataBlkStr, CharLimit: 10,
			Validate: validateDigitsMin(1, true), BlockInvalidChars: true},
		{Type: components.FieldTypeText, Label: "Parity Block Count (M)", Placeholder: "e.g. 2 if EC, else leave blank", Value: parityBlkStr, CharLimit: 10,
			Validate: validateDigitsMin(0, false), BlockInvalidChars: true},
		{Type: components.FieldTypeSelect, Label: "Placement Fault Domain", Value: initial.FilterType, Options: filterOpts},
		{Type: components.FieldTypeSelect, Label: "Parallel File System (PFS)", Value: pfsVal, Options: pfsOpts},
	}

	return &VdevFormPage{
		Form:   components.NewForm(fields),
		IsEdit: isEdit,
	}
}

func (p *VdevFormPage) isFormPage() {}

func (p *VdevFormPage) Init() tea.Cmd { return nil }

func (p *VdevFormPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" && p.Form.FocusedIndex == len(p.Form.Inputs)-1 && p.Form.Valid() {
			name := p.Form.Value(0)
			sizeGB := p.Form.Value(1)
			redundancy := p.Form.Value(2)
			dataBlk := p.Form.Value(3)
			parityBlk := p.Form.Value(4)
			filterType := p.Form.Value(5)
			pfs := p.Form.Value(6)
			return p, func() tea.Msg {
				return SubmitVdevFormMsg{
					Name:         name,
					SizeGB:       sizeGB,
					Redundancy:   redundancy,
					DataBlkCnt:   dataBlk,
					ParityBlkCnt: parityBlk,
					FilterType:   filterType,
					PFS:          pfs,
					IsEdit:       p.IsEdit,
				}
			}
		}
	}
	cmd := p.Form.Update(msg)
	return p, cmd
}

func (p *VdevFormPage) View() string {
	title := "Create Virtual Device Pool"
	if p.IsEdit {
		title = "Edit Virtual Device Pool"
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render(title),
		"",
		p.Form.View(),
	)
}

func (p *VdevFormPage) Title() string { return "Create Vdev" }

func (p *VdevFormPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Tab/Down"), key.WithHelp("Tab/Down", "Next Field")),
		key.NewBinding(key.WithKeys("◄/►/Space"), key.WithHelp("◄/►/Space", "Select Option")),
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Submit Vdev")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Cancel")),
	}
}

// NewVdevViewPage builds an interactive table of Vdev pools. Unlike other
// resources, Vdev actions aren't gated by CanWrite — any logged-in user can
// manage them — so this always passes canWrite=true to TablePage.
func NewVdevViewPage(vdevs []domain.VdevConfig, width, height int) *TablePage[domain.VdevConfig] {
	columns := []table.Column{
		{Title: "Vdev Pool Name", Width: 22},
		{Title: "Redundancy", Width: 18},
		{Title: "Data/Parity", Width: 16},
		{Title: "Vdev UUID", Width: 38},
	}

	var rows []table.Row
	for _, v := range vdevs {
		redStr := "Replica"
		if v.Redundancy.IsEC() {
			redStr = "Erasure Coding"
		}
		dataParityStr := fmt.Sprintf("%dK / %dM", v.DataBlkCnt, v.ParityBlkCnt)
		rows = append(rows, table.Row{
			v.Name, redStr, dataParityStr, v.ID,
		})
	}

	actions := []key.Binding{
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "Create Vdev Pool")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "Edit Vdev Pool")),
		key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "Delete Vdev Pool")),
	}

	return NewTablePage(vdevs, columns, rows, width, height,
		lipgloss.Color("#FFFFFF"), styles.ColorPrimary, true,
		"View All Vdevs", "CONFIGURED VDEV POOLS TABLE", renderVdevDetail, actions)
}

func renderVdevDetail(vdev domain.VdevConfig) string {
	redStr := "Replica"
	if vdev.Redundancy.IsEC() {
		redStr = "Erasure Coding"
	}
	pfsStr := vdev.PFSName
	if pfsStr == "" {
		pfsStr = vdev.PFSID
	}
	if pfsStr == "" {
		pfsStr = "None"
	}
	bullets := []string{
		fmt.Sprintf("  • Vdev UUID:        %s", vdev.ID),
		fmt.Sprintf("  • Redundancy Mode:  %s", redStr),
		fmt.Sprintf("  • Data Blocks (K):  %d", vdev.DataBlkCnt),
		fmt.Sprintf("  • Parity Blocks (M):%d", vdev.ParityBlkCnt),
		fmt.Sprintf("  • Fault Domain:     %s", vdev.FilterType),
		fmt.Sprintf("  • PFS:              %s", pfsStr),
	}
	return detailLines("Vdev Pool Inspection View", "VDEV POOL DETAILS", lipgloss.Color("#FFFFFF"), lipgloss.Color("#EC4899"),
		vdev.Name, bullets, "[ Press Esc or Enter to return to Vdev Table ]")
}
