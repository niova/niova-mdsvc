package pages

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// SubmitImportInfraFormMsg is emitted once the operator has entered a file
// path and it has been read and parsed successfully — Request is already
// decoded so the confirm dialog can show node counts without re-reading the
// file, and the App layer never has to touch a *os.File directly.
type SubmitImportInfraFormMsg struct {
	FilePath string
	Request  *domain.ImportInfraRequest
}

// ImportInfraFormPage takes a path to a JSON file (see domain.ImportInfraRequest
// for the expected shape — the same nested PDU->Rack->Hypervisor->Device->NISD
// document mdsvc-tidb's POST /api/infra accepts) and, on submit, reads and
// parses it locally so a bad path or malformed JSON is reported right here
// instead of surfacing later as an opaque "import failed".
type ImportInfraFormPage struct {
	Form components.FormModel
	Err  string
}

func NewImportInfraFormPage() *ImportInfraFormPage {
	fields := []components.FieldDefinition{
		{Type: components.FieldTypeText, Label: "Infra JSON file path", Placeholder: "e.g. /home/user/infra.json", CharLimit: 256},
	}
	return &ImportInfraFormPage{Form: components.NewForm(fields)}
}

func (p *ImportInfraFormPage) isFormPage() {}

func (p *ImportInfraFormPage) Init() tea.Cmd { return nil }

func (p *ImportInfraFormPage) Update(msg tea.Msg) (Page, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "enter" {
			p.Err = ""
			path := p.Form.Value(0)
			data, err := os.ReadFile(path)
			if err != nil {
				p.Err = "Cannot read file: " + err.Error()
				return p, nil
			}
			req, err := domain.ParseImportInfra(data)
			if err != nil {
				p.Err = err.Error()
				return p, nil
			}
			return p, func() tea.Msg {
				return SubmitImportInfraFormMsg{FilePath: path, Request: req}
			}
		}
	}
	cmd := p.Form.Update(msg)
	return p, cmd
}

func (p *ImportInfraFormPage) View() string {
	lines := []string{
		styles.TextBold.Render("Import Infrastructure from JSON"),
		"",
		styles.TextMuted.Render("Loads a full PDU -> Rack -> Hypervisor -> Device -> NISD hierarchy from a single"),
		styles.TextMuted.Render("JSON file in one pass. IDs may be omitted (generated on import). Existing entries"),
		styles.TextMuted.Render("are left untouched; this only creates."),
		"",
		p.Form.View(),
	}
	if p.Err != "" {
		lines = append(lines, "", styles.TextError.Render(p.Err))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (p *ImportInfraFormPage) Title() string { return "Import Infra" }

func (p *ImportInfraFormPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Load & Review")),
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Cancel")),
	}
}

// ImportInfraResultPage reports what domain.ImportInfra actually did — every
// node it attempted, success or failure — since a bulk import has no
// rollback (see ImportInfra's doc comment) and the operator needs to know
// exactly what's now sitting in the control plane, not just a pass/fail.
type ImportInfraResultPage struct {
	FilePath string
	Result   *domain.ImportResult
}

func NewImportInfraResultPage(filePath string, result *domain.ImportResult) *ImportInfraResultPage {
	return &ImportInfraResultPage{FilePath: filePath, Result: result}
}

func (p *ImportInfraResultPage) isFormPage() {}

func (p *ImportInfraResultPage) Init() tea.Cmd { return nil }

func (p *ImportInfraResultPage) Update(msg tea.Msg) (Page, tea.Cmd) { return p, nil }

func (p *ImportInfraResultPage) View() string {
	created, failed := p.Result.Counts()
	lines := []string{
		styles.TextBold.Render("Import Result: " + p.FilePath),
		"",
		fmt.Sprintf("%d created, %d failed", created, failed),
		"",
	}
	for _, o := range p.Result.Outcomes {
		label := o.Name
		if label == "" {
			label = o.ID
		}
		if o.Err != nil {
			lines = append(lines, styles.TextError.Render(fmt.Sprintf("  ✗ %s %s: %s", o.Kind, label, o.Err.Error())))
		} else {
			lines = append(lines, fmt.Sprintf("  ✓ %s %s", o.Kind, label))
		}
	}
	lines = append(lines, "", styles.TextMuted.Render("Press Esc to return to Topology."))
	return styles.Card.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (p *ImportInfraResultPage) Title() string { return "Import Infra Result" }

func (p *ImportInfraResultPage) KeyBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Back to Topology")),
	}
}
