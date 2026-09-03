package pages

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

// renderDiscoverySelector renders a "◄ Label ►" cycling selector field, in
// the FieldTypeSelect style, shared by discovery pages whose top region
// behaves like a select field without being one. optionCount gates the
// "switch" hint.
func renderDiscoverySelector(focused bool, label, optLabel string, optionCount int) string {
	var lbl string
	if focused {
		lbl = styles.SelectedItem.Render("▶ " + label)
	} else {
		lbl = styles.UnselectedItem.Render("  " + label)
	}

	displayStr := "◄  " + optLabel + "  ►"
	var field string
	if focused {
		hint := " (◄/► or Space to switch)"
		if optionCount <= 1 {
			hint = ""
		}
		field = styles.InputFocused.Render(displayStr) + styles.TextMuted.Render(hint)
	} else {
		field = styles.InputBlurred.Render(displayStr)
	}
	return lipgloss.JoinVertical(lipgloss.Left, lbl, field)
}
