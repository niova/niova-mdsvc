package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestMainPanelContentWidthNeverOverflows guards against a hand-counted
// width budget drifting out of sync with what RenderDashboard actually
// renders — builds content exactly as wide as MainPanelContentWidth
// promises, renders it, and checks the result never exceeds termWidth.
func TestMainPanelContentWidthNeverOverflows(t *testing.T) {
	for _, termWidth := range []int{60, 100, 140, 200, 320} {
		n := MainPanelContentWidth(termWidth)
		content := strings.Repeat("x", n)

		full := RenderDashboard(DashboardProps{
			MainContentView:   content,
			HasResourceAccess: true,
			Metrics:           ClusterMetrics{PDUCount: 1, NISDCount: 346, VdevCount: 219}, // realistic multi-digit counts
		})

		if got := lipgloss.Width(full); got > termWidth {
			t.Errorf("termWidth=%d: MainPanelContentWidth=%d produced a render %d columns wide (overflow of %d)",
				termWidth, n, got, got-termWidth)
		}
	}
}
