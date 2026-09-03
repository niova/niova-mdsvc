package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

type ConfirmDialogType int

const (
	ConfirmTypeInfo ConfirmDialogType = iota
	ConfirmTypeWarning
	ConfirmTypeDanger
)

type ConfirmResultMsg struct {
	Confirmed bool
	ActionID  string
	Data      interface{}
}

// ConfirmDialogModel displays an interactive confirmation overlay modal
type ConfirmDialogModel struct {
	Title     string
	Message   string
	Details   []string
	Type      ConfirmDialogType
	ActionID  string
	Data      interface{}
	Selection bool // true = Confirm, false = Cancel
	Active    bool
}

func NewConfirmDialog(title, message string, details []string, dialogType ConfirmDialogType, actionID string, data interface{}) ConfirmDialogModel {
	return ConfirmDialogModel{
		Title:     title,
		Message:   message,
		Details:   details,
		Type:      dialogType,
		ActionID:  actionID,
		Data:      data,
		Selection: true, // Default to Confirm
		Active:    true,
	}
}

func (d *ConfirmDialogModel) Update(msg tea.Msg) (ConfirmDialogModel, tea.Cmd) {
	if !d.Active {
		return *d, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "left", "right", "tab", "h", "l":
			d.Selection = !d.Selection
			return *d, nil

		case "y", "Y":
			d.Active = false
			return *d, func() tea.Msg {
				return ConfirmResultMsg{Confirmed: true, ActionID: d.ActionID, Data: d.Data}
			}

		case "n", "N", "esc":
			d.Active = false
			return *d, func() tea.Msg {
				return ConfirmResultMsg{Confirmed: false, ActionID: d.ActionID, Data: d.Data}
			}

		case "enter":
			d.Active = false
			confirmed := d.Selection
			return *d, func() tea.Msg {
				return ConfirmResultMsg{Confirmed: confirmed, ActionID: d.ActionID, Data: d.Data}
			}
		}
	}

	return *d, nil
}

func (d ConfirmDialogModel) View() string {
	if !d.Active {
		return ""
	}

	var titleStyle lipgloss.Style
	var cardStyle lipgloss.Style

	switch d.Type {
	case ConfirmTypeDanger:
		titleStyle = lipgloss.NewStyle().Foreground(styles.ColorError).Bold(true)
		cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.ColorError).
			Padding(1, 2).
			Width(65)
	case ConfirmTypeWarning:
		titleStyle = lipgloss.NewStyle().Foreground(styles.ColorWarning).Bold(true)
		cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.ColorWarning).
			Padding(1, 2).
			Width(65)
	default:
		titleStyle = styles.TextBold
		cardStyle = styles.Card.Width(65)
	}

	var lines []string
	lines = append(lines, titleStyle.Render("! "+d.Title))
	lines = append(lines, "")
	lines = append(lines, d.Message)

	if len(d.Details) > 0 {
		lines = append(lines, "")
		for _, detail := range d.Details {
			lines = append(lines, styles.TextMuted.Render("  • "+detail))
		}
	}

	lines = append(lines, "")

	confirmBtn := "[ Yes / Confirm (Enter) ]"
	cancelBtn := "[ No / Cancel (Esc) ]"

	if d.Selection {
		confirmBtn = styles.SelectedItem.Render("▶ " + confirmBtn)
		cancelBtn = styles.UnselectedItem.Render("  " + cancelBtn)
	} else {
		confirmBtn = styles.UnselectedItem.Render("  " + confirmBtn)
		cancelBtn = styles.SelectedItem.Render("▶ " + cancelBtn)
	}

	btnRow := fmt.Sprintf("%s    %s", confirmBtn, cancelBtn)
	lines = append(lines, btnRow)

	content := strings.Join(lines, "\n")
	return cardStyle.Render(content)
}
