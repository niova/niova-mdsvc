package components

import (
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// StatusType enum
type StatusType int

const (
	StatusInfo StatusType = iota
	StatusSuccess
	StatusError
)

// RenderStatusBar renders an alert message banner
func RenderStatusBar(msg string, statusType StatusType) string {
	if msg == "" {
		return ""
	}
	switch statusType {
	case StatusSuccess:
		return styles.StatusSuccessBanner.Render("✔ " + msg)
	case StatusError:
		return styles.StatusErrorBanner.Render("✖ " + msg)
	default:
		return styles.TextMuted.Render("ℹ " + msg)
	}
}
