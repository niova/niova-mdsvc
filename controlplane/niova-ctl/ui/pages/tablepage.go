package pages

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// TablePage is the shared implementation behind every "browse a table,
// enter/space/v to inspect one in a detail card, esc/enter to return" page.
// Instantiate per resource type T via NewTablePage, supplying only the
// table's columns/rows and a detail-card render function.
type TablePage[T any] struct {
	Table      table.Model
	Items      []T
	Selected   *T
	Inspecting bool

	// CanWrite gates the Add/Edit(/Delete) keybindings — false for a
	// non-admin session, or (Partitions only) a backend that doesn't
	// support writing this resource at all.
	CanWrite bool

	pageTitle   string // returned by Title()
	tableHeader string // shown above the table when not inspecting, e.g. "CONFIGURED PDUS TABLE"

	// renderInspect renders the full inspect-mode view for the selected item
	// (not a fixed badge+bullets+footer template, since some pages show
	// extra context above the card that others don't).
	renderInspect func(item T) string

	// writeActions are the Add/Edit(/Delete)-style keybindings shown when
	// not inspecting and CanWrite is true.
	writeActions []key.Binding

	// allRows is the full, unfiltered row set — rows[i] describes Items[i].
	// Table.Rows() only holds the visible subset once a filter narrows it.
	// Also doubles as the filter's search corpus (see applyFilter), so every
	// displayed column is searchable with no separate "which fields" list.
	allRows []table.Row

	search searchBox

	// visibleIdx maps each currently-visible table row back to its index in
	// Items/allRows, in the same order as the table's rows. Identity
	// ([0,1,2,...]) when unfiltered.
	visibleIdx []int

	// visibleWidth is how much of the Card's own width is actually
	// available for the table (see availableTableWidth) — tracked so
	// hScroll can be clamped and so View() knows how much of each row to
	// slice out. Updated on every resize by SetTableWidth.
	visibleWidth int
	// hScroll is how far right the table's natural (unclipped, autoSizeColumns
	// never shrinks it) content has been scrolled — see scrollClip. Reset
	// whenever the column set changes shape (e.g. a filter changes which
	// rows are visible doesn't change columns, so this only resets on
	// construction).
	hScroll int
}

// NewTablePage builds a TablePage. rows must be in the same order as items
// (row i must describe items[i]) so the cursor resolves back to its item and
// '/' can filter against those cells. columns' own Width fields are ignored
// (see autoSizeColumns); width sizes the scroll viewport, not the columns.
func NewTablePage[T any](
	items []T,
	columns []table.Column,
	rows []table.Row,
	width, height int,
	selectedFg, selectedBg lipgloss.Color,
	canWrite bool,
	pageTitle, tableHeader string,
	renderInspect func(item T) string,
	writeActions []key.Binding,
) *TablePage[T] {
	tHeight := height - 16
	if tHeight < 4 {
		tHeight = 4
	}

	columns = autoSizeColumns(columns, rows)

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(tHeight),
	)
	t.KeyMap.LineDown.SetKeys("down", "j")

	s := table.DefaultStyles()
	s.Header = s.Header.BorderStyle(lipgloss.NormalBorder()).BorderForeground(styles.ColorMuted).BorderBottom(true).Bold(true)
	s.Selected = s.Selected.Foreground(selectedFg).Background(selectedBg).Bold(true)
	t.SetStyles(s)

	return &TablePage[T]{
		Table:         t,
		Items:         items,
		CanWrite:      canWrite,
		pageTitle:     pageTitle,
		tableHeader:   tableHeader,
		renderInspect: renderInspect,
		writeActions:  writeActions,
		allRows:       rows,
		search:        newSearchBox(),
		visibleIdx:    identityIndices(len(items)),
		visibleWidth:  availableTableWidth(width),
	}
}

// identityIndices returns [0, 1, ..., n-1] — TablePage's visibleIdx when no
// filter is applied.
func identityIndices(n int) []int {
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	return idx
}

func (p *TablePage[T]) Init() tea.Cmd { return nil }

func (p *TablePage[T]) Update(msg tea.Msg) (Page, tea.Cmd) {
	if p.Inspecting {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "esc", "enter", "q", "space", "v", "V":
				p.Inspecting = false
				p.Selected = nil
				return p, nil
			}
		}
		return p, nil
	}

	// While the filter box has focus, every key goes to it except Esc
	// (cancel: clear the filter and close the box) and Enter (commit: keep
	// the narrowed rows, return keyboard focus to table navigation).
	if p.search.Filtering() {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "esc":
				p.clearFilter()
				return p, nil
			case "enter":
				p.search.Commit()
				return p, nil
			}
		}
		cmd := p.search.Type(msg)
		p.applyFilter()
		return p, cmd
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "/":
			return p, p.search.Open()
		case "esc":
			// Esc is otherwise unbound at this level (bubbles/table doesn't
			// use it either) — safe to repurpose as "clear an applied
			// filter" once the box itself isn't focused anymore.
			if p.search.Value() != "" {
				p.clearFilter()
				return p, nil
			}
		case "enter", "space", "v", "V":
			if item, ok := p.itemAt(p.Table.Cursor()); ok {
				p.Selected = &item
				p.Inspecting = true
				return p, nil
			}
		case "shift+left":
			// Plain Left/Right are already global tab-switch shortcuts
			// (navigation.go), so scrolling within a table needs its own
			// keys — see scrollClip's doc for why this exists at all.
			p.hScroll = max(p.hScroll-8, 0)
			return p, nil
		case "shift+right":
			p.hScroll = min(p.hScroll+8, maxHScroll(tableNaturalWidth(p.Table.Columns()), p.visibleWidth))
			return p, nil
		}
	}

	var cmd tea.Cmd
	p.Table, cmd = p.Table.Update(msg)
	return p, cmd
}

// applyFilter recomputes visibleIdx and the table's rows from the filter
// box's current text (see filterRows). Called after every keystroke while
// filtering.
func (p *TablePage[T]) applyFilter() {
	term := p.search.Value()
	idx, rows := filterRows(p.allRows, term)
	p.visibleIdx = idx
	p.Table.SetRows(rows)
	if term != "" {
		p.Table.SetCursor(0)
	}
}

// clearFilter resets the filter box and restores the full row set, keeping
// the cursor on whatever item it was over — row order is otherwise
// unchanged by filtering, but the item's row *index* generally isn't the
// same once hidden rows reappear above it.
func (p *TablePage[T]) clearFilter() {
	cursor := p.Table.Cursor()
	realIdx := 0
	if cursor >= 0 && cursor < len(p.visibleIdx) {
		realIdx = p.visibleIdx[cursor]
	}
	p.search.Clear()
	p.applyFilter()
	p.Table.SetCursor(realIdx)
}

// IsFiltering satisfies pages.Filtering — see its doc comment.
func (p *TablePage[T]) IsFiltering() bool { return p.search.Filtering() }

// FilterView satisfies pages.FilterViewer — see its doc comment. The app
// renders this in a fixed-height slot outside p.View(), so opening/closing
// the search box never changes this page's own rendered height.
func (p *TablePage[T]) FilterView() string { return p.search.View() }

func (p *TablePage[T]) View() string {
	if p.Inspecting && p.Selected != nil {
		return p.renderInspect(*p.Selected)
	}
	countStr := fmt.Sprintf("(%d items)", len(p.Items))
	if p.search.Value() != "" {
		countStr = fmt.Sprintf("(%d/%d matches)", len(p.visibleIdx), len(p.Items))
	}
	if scrollable := maxHScroll(tableNaturalWidth(p.Table.Columns()), p.visibleWidth) > 0; scrollable {
		countStr += fmt.Sprintf(" — Shift+←/→ to scroll (col %d)", p.hScroll)
	}
	headerStr := fmt.Sprintf("%s %s", styles.TextBold.Render(p.tableHeader), styles.TextMuted.Render(countStr))

	tableView := scrollClip(p.Table.View(), p.hScroll, p.visibleWidth)
	return lipgloss.JoinVertical(lipgloss.Left,
		headerStr,
		"",
		styles.Card.Render(tableView),
	)
}

func (p *TablePage[T]) Title() string { return p.pageTitle }

func (p *TablePage[T]) KeyBindings() []key.Binding {
	if p.Inspecting {
		return nil
	}
	if p.search.Filtering() {
		return []key.Binding{
			key.NewBinding(key.WithKeys("Enter"), key.WithHelp("Enter", "Apply Filter")),
			key.NewBinding(key.WithKeys("Esc"), key.WithHelp("Esc", "Clear Filter")),
		}
	}
	bindings := p.writeActions
	if !p.CanWrite {
		bindings = nil
	}
	if maxHScroll(tableNaturalWidth(p.Table.Columns()), p.visibleWidth) > 0 {
		bindings = append(bindings,
			key.NewBinding(key.WithKeys("shift+left", "shift+right"), key.WithHelp("Shift+←/→", "Scroll Table")),
		)
	}
	filterHint := key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "Search"))
	if p.search.Value() != "" {
		filterHint = key.NewBinding(key.WithKeys("/", "Esc"), key.WithHelp("/ Esc", "Edit/Clear Search"))
	}
	return append(bindings, filterHint)
}

func (p *TablePage[T]) IsInspecting() bool { return p.Inspecting }

func (p *TablePage[T]) SetTableHeight(h int) { p.Table.SetHeight(h) }

// SetTableWidth satisfies pages.TableWidthSetter — column widths themselves
// never change with terminal size (autoSizeColumns is content-only now),
// but how much of them is actually visible does, so this updates the
// scroll viewport (visibleWidth) and re-clamps hScroll on a live resize,
// the width equivalent of SetTableHeight.
func (p *TablePage[T]) SetTableWidth(w int) {
	p.visibleWidth = availableTableWidth(w)
	p.hScroll = min(p.hScroll, maxHScroll(tableNaturalWidth(p.Table.Columns()), p.visibleWidth))
}

func (p *TablePage[T]) SelectedForEdit() (any, bool) {
	return p.itemAt(p.Table.Cursor())
}

// SelectedItem returns the item under the table cursor, resolved through
// any active filter — the public counterpart of itemAt for call sites
// outside this package that need the typed T rather than SelectedForEdit's
// any (e.g. update.go's delete-vdev confirm dialog).
func (p *TablePage[T]) SelectedItem() (T, bool) {
	return p.itemAt(p.Table.Cursor())
}

// itemAt resolves a table row index back to its item through visibleIdx,
// so a cursor over filtered rows still points at the right item in Items.
func (p *TablePage[T]) itemAt(cursor int) (T, bool) {
	var zero T
	if cursor < 0 || cursor >= len(p.visibleIdx) {
		return zero, false
	}
	idx := p.visibleIdx[cursor]
	if idx < 0 || idx >= len(p.Items) {
		return zero, false
	}
	return p.Items[idx], true
}

// cardBody renders the content inside an inspect view's Card border: title
// badge, bullet lines, and the "press Esc/Enter" footer. Factored out of
// detailLines so pages needing extra context above the card can reuse it.
func cardBody(badgeText string, badgeFg, badgeBg lipgloss.Color, title string, bullets []string, returnHint string) string {
	badge := lipgloss.NewStyle().Foreground(badgeFg).Background(badgeBg).Bold(true).Padding(0, 1)

	lines := make([]string, 0, len(bullets)+4)
	lines = append(lines, fmt.Sprintf("%s %s", badge.Render(badgeText), styles.TextBold.Render(title)))
	lines = append(lines, "")
	lines = append(lines, bullets...)
	lines = append(lines, "")
	lines = append(lines, styles.TextMuted.Render(returnHint))

	return strings.Join(lines, "\n")
}

// detailLines wraps cardBody in the heading + Card chrome shared by every
// inspect view. Pages with extra context above the card (Hypervisor) call
// cardBody directly and build that chrome themselves instead.
func detailLines(heading, badgeText string, badgeFg, badgeBg lipgloss.Color, title string, bullets []string, returnHint string) string {
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.TextBold.Render(heading),
		"",
		styles.Card.Render(cardBody(badgeText, badgeFg, badgeBg, title, bullets, returnHint)),
	)
}
