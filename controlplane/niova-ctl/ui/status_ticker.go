package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// statusDisplayDuration is how long a status bar message (success, info, or
// error) stays visible before it self-clears.
const statusDisplayDuration = 6000 * time.Millisecond

// statusTickMsg drives the status bar's auto-dismiss. It fires on a fixed
// interval; the tick handler detects a change by diffing against its own
// last-seen copy, so StatusMsg call sites never need to know about expiry.
type statusTickMsg struct{}

func statusTickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return statusTickMsg{}
	})
}
