package pages

import (
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/components"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// minVisibleTableWidth is the floor availableTableWidth ever returns, so an
// absurdly narrow terminal still gets a usable (if cramped) scroll viewport
// instead of a negative/zero one.
const minVisibleTableWidth = 20

// scrollSafetyMargin is subtracted on top of the measured chrome so an
// off-by-one estimate errs toward unused space at the edge, not toward a
// scroll offset view.go's MaxWidth clamp would silently cut back below what
// scrollClip believes is visible.
const scrollSafetyMargin = 2

// availableTableWidth converts a terminal width into how much of it a
// table's own columns actually get to render into — measured against the
// real chrome (components.MainPanelContentWidth, styles.Card) rather than a
// hand-counted constant that can drift out of sync with either.
func availableTableWidth(termWidth int) int {
	cardOverhead := lipgloss.Width(styles.Card.Render(""))
	avail := components.MainPanelContentWidth(termWidth) - cardOverhead - scrollSafetyMargin
	if avail < minVisibleTableWidth {
		return minVisibleTableWidth
	}
	return avail
}

// autoSizeColumns replaces every column's Width with one sized to its own
// content (header title plus every row's cell, plus a little padding),
// instead of the caller's fixed value. Deliberately never shrinks a column
// even if the total ends up wider than the terminal — whatever doesn't fit
// is reached by scrolling horizontally instead (see scrollClip).
func autoSizeColumns(cols []table.Column, rows []table.Row) []table.Column {
	n := len(cols)
	if n == 0 {
		return cols
	}
	natural := make([]int, n)
	for i, c := range cols {
		natural[i] = len(c.Title)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < n && len(cell) > natural[i] {
				natural[i] = len(cell)
			}
		}
	}

	const pad = 1
	out := make([]table.Column, n)
	for i, c := range cols {
		out[i] = table.Column{Title: c.Title, Width: natural[i] + pad}
	}
	return out
}

// cellStylePadding is bubbles/table.DefaultStyles()'s Cell/Header Padding(0,
// 1), applied on top of col.Width by renderRow — every rendered cell is
// actually col.Width+2 wide. TablePage/AuthzPolicyPage never override
// Styles.Header/Cell, so this is a safe constant rather than a per-table value.
const cellStylePadding = 2

// tableNaturalWidth sums a table's current column widths, plus each one's
// own bubbles/table cell padding (see cellStylePadding) — its full,
// unclipped rendered width, which scrollClip's caller scrolls a viewport
// across.
func tableNaturalWidth(cols []table.Column) int {
	total := 0
	for _, c := range cols {
		total += c.Width + cellStylePadding
	}
	return total
}

// maxHScroll is the furthest right a horizontal scroll offset may go before
// the table's last column would start leaving blank space on the left —
// natural is the table's full unclipped width, visible how much of it
// actually fits in the Card.
func maxHScroll(natural, visible int) int {
	if d := natural - visible; d > 0 {
		return d
	}
	return 0
}

// scrollClip returns the horizontal slice [offset, offset+visible) of every
// line in tableView — ANSI-aware (github.com/charmbracelet/x/ansi), so
// header styling and the selected-row highlight survive the cut intact
// instead of being corrupted by slicing raw bytes mid-escape-sequence.
func scrollClip(tableView string, offset, visible int) string {
	if visible <= 0 {
		return tableView
	}
	lines := strings.Split(tableView, "\n")
	for i, line := range lines {
		lines[i] = ansi.Cut(line, offset, offset+visible)
	}
	return strings.Join(lines, "\n")
}
