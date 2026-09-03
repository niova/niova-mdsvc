package pages

import (
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// searchBox is the '/' live-filter box shared by every bubbles/table-backed
// page — TablePage[T] embeds one per table, and AuthzPolicyPage (RBAC +
// ABAC, two independent tables) embeds two. Filtering only ever needs a
// row's own rendered cells (see filterRows), never the type that populated
// them, so this has no per-page state beyond the input itself.
type searchBox struct {
	input     textinput.Model
	filtering bool // true only while the box has keyboard focus
}

func newSearchBox() searchBox {
	fi := textinput.New()
	fi.Prompt = "/ "
	fi.PromptStyle = lipgloss.NewStyle().Foreground(styles.ColorPrimary).Bold(true)
	fi.Placeholder = "search this table..."
	fi.PlaceholderStyle = lipgloss.NewStyle().Foreground(styles.ColorMuted)
	fi.CharLimit = 64
	fi.Width = 48
	return searchBox{input: fi}
}

func (s *searchBox) Value() string { return s.input.Value() }

func (s *searchBox) Filtering() bool { return s.filtering }

// Active reports whether there's anything to show for this box — either
// being edited right now, or holding an already-applied term.
func (s *searchBox) Active() bool { return s.filtering || s.input.Value() != "" }

// Open focuses the box on a fresh '/' press (or to resume editing an
// already-applied term) and returns its cursor-blink cmd.
func (s *searchBox) Open() tea.Cmd {
	s.filtering = true
	return s.input.Focus()
}

// Clear resets the box entirely (Esc while filtering, or Esc over an
// applied-but-unfocused filter). Caller still owns recomputing rows.
func (s *searchBox) Clear() {
	s.filtering = false
	s.input.SetValue("")
	s.input.Blur()
}

// Commit closes the box but keeps its text applied (Enter).
func (s *searchBox) Commit() {
	s.filtering = false
	s.input.Blur()
}

// Type feeds a keystroke to the box while it's open.
func (s *searchBox) Type(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	return cmd
}

// View renders the box, or "" when Active() is false — callers place this
// in a fixed-height slot outside their own content (see pages.FilterViewer)
// rather than inline, so opening/closing it never changes page height.
func (s *searchBox) View() string {
	if !s.Active() {
		return ""
	}
	return s.input.View()
}

// filterRows returns the indices into allRows (and the rows themselves)
// whose own cells contain term as a case-insensitive substring — every
// column actually shown in the table becomes searchable this way, with no
// per-page list of "which fields matter" to keep in sync as columns
// change. A blank term returns everything, identity-ordered.
func filterRows(allRows []table.Row, term string) ([]int, []table.Row) {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return identityIndices(len(allRows)), allRows
	}
	idx := make([]int, 0, len(allRows))
	rows := make([]table.Row, 0, len(allRows))
	for i, row := range allRows {
		if strings.Contains(strings.ToLower(strings.Join(row, " ")), term) {
			idx = append(idx, i)
			rows = append(rows, row)
		}
	}
	return idx, rows
}
