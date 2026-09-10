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

// SubmitPFSFormMsg emitted when PFS form is submitted
type SubmitPFSFormMsg struct {
	// ID is the existing PFS's ID when IsEdit is true, "" for a new PFS —
	// PFS.ID is always server-assigned, never client-supplied.
	ID     string
	Name   string
	IsEdit bool
}

// PFSFormPage handles creation of PFS file systems
type PFSFormPage struct {
	Form   components.FormModel
	ID     string
	IsEdit bool
}

func NewPFSFormPage(isEdit bool, initial domain.PFS) *PFSFormPage {
	fields := []components.FieldDefinition{
		{Type: components.FieldTypeText, Label: "File System Name", Placeholder: "e.g. pfs-cluster-01", Value: initial.Name, CharLimit: 64},
	}
	return &PFSFormPage{
		Form:   components.NewForm(fields),
		ID:     initial.ID,
		IsEdit: isEdit,
	}
}

func (p *PFSFormPage) isFormPage() {}

func (p *PFSFormPage) Init() tea.Cmd { return nil }

func (p *PFSFormPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" {
			name := p.Form.Value(0)
			return p, func() tea.Msg {
				return SubmitPFSFormMsg{
					ID:     p.ID,
					Name:   name,
					IsEdit: p.IsEdit,
				}
			}
		}
	}
	cmd := p.Form.Update(msg)
	return p, cmd
}

func (p *PFSFormPage) View() string {
	title := "Create Parallel File System (PFS)"
	if p.IsEdit {
		title = "Edit Parallel File System"
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render(title),
		"",
		p.Form.View(),
	)
}

func (p *PFSFormPage) Title() string { return "Create PFS" }

func (p *PFSFormPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Tab/Down"), key.WithHelp("Tab/Down", "Next Field")),
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Submit PFS")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Cancel")),
	}
}

// NewPFSViewPage builds an interactive table of PFS file systems; enter/
// space/v opens a detail card for the selected one.
func NewPFSViewPage(pfss []domain.PFS, width, height int, canWrite bool) *TablePage[domain.PFS] {
	columns := []table.Column{
		{Title: "File System Name", Width: 24},
		{Title: "Component Vdevs", Width: 18},
		{Title: "PFS UUID", Width: 38},
	}

	var rows []table.Row
	for _, p := range pfss {
		rows = append(rows, table.Row{
			p.Name, fmt.Sprintf("%d vdevs", len(p.VdevIDs)), p.ID,
		})
	}

	actions := []key.Binding{
		key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "Create File System")),
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "Edit File System")),
	}

	return NewTablePage(pfss, columns, rows, width, height,
		lipgloss.Color("#FFFFFF"), styles.ColorPrimary, canWrite,
		"View All PFS", "CONFIGURED PARALLEL FILE SYSTEMS TABLE", renderPFSDetail, actions)
}

func renderPFSDetail(pfs domain.PFS) string {
	bullets := []string{
		fmt.Sprintf("  • PFS UUID:         %s", pfs.ID),
		fmt.Sprintf("  • Cluster Name:     %s", pfs.Name),
		fmt.Sprintf("  • Attached Vdevs:   %d", len(pfs.VdevIDs)),
	}
	if len(pfs.VdevIDs) > 0 {
		bullets = append(bullets, "", styles.TextBold.Render("Vdev Identifiers:"))
		for _, vID := range pfs.VdevIDs {
			bullets = append(bullets, fmt.Sprintf("    ├─ %s", vID))
		}
	}
	return detailLines("PFS Inspection View", "PFS CLUSTER DETAILS", lipgloss.Color("#FFFFFF"), styles.ColorPrimary,
		pfs.Name, bullets, "[ Press Esc or Enter to return to PFS Table ]")
}
