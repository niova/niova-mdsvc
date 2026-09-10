package styles

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Palette Colors
	ColorPrimary     = lipgloss.Color("#7D56F4")
	ColorSecondary   = lipgloss.Color("#01BE85")
	ColorAccent      = lipgloss.Color("#FF79C6")
	ColorBackground  = lipgloss.Color("#0D1117")
	ColorCardBg      = lipgloss.Color("#161B22")
	ColorText        = lipgloss.Color("#FAFAFA")
	ColorMuted       = lipgloss.Color("#666666")
	ColorPlaceholder = lipgloss.Color("#fcfcfcff")
	ColorBorder      = lipgloss.Color("#30363D")
	ColorFocusBorder = lipgloss.Color("#01BE85")
	ColorError       = lipgloss.Color("#FF5F87")
	ColorSuccess     = lipgloss.Color("#50FA7B")
	ColorWarning     = lipgloss.Color("#F1FA8C")

	// Base Layout Styles
	AppTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorText).
			Background(ColorPrimary).
			Padding(0, 2).
			MarginRight(1)

	Breadcrumb = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	BadgeConnected = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorSecondary).
			Padding(0, 1)

	BadgeDisconnected = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(ColorError).
				Padding(0, 1)

	BadgeAuthEnabled = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#000000")).
				Background(ColorWarning).
				Padding(0, 1)

	BadgeAuthDisabled = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorText).
				Background(ColorMuted).
				Padding(0, 1)

	Card = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(1, 2)

	CardFocused = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorFocusBorder).
			Padding(1, 2)

	// Input Styles
	InputFocused = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorFocusBorder).
			Padding(0, 1).
			Width(48)

	InputBlurred = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1).
			Width(48)

	// InputError marks a field whose FieldDefinition.Validate currently
	// fails — used instead of InputFocused/InputBlurred regardless of
	// focus, so the problem stays visible even after tabbing away.
	InputError = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorError).
			Padding(0, 1).
			Width(48)

	// Text Formatting
	TextBold = lipgloss.NewStyle().Bold(true)

	TextMuted = lipgloss.NewStyle().Foreground(ColorMuted)

	TextSuccess = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)

	TextError = lipgloss.NewStyle().Foreground(ColorError).Bold(true)

	TextWarning = lipgloss.NewStyle().Foreground(ColorWarning)

	SelectedItem = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	UnselectedItem = lipgloss.NewStyle().
			Foreground(ColorText)

	// Status Banners
	StatusSuccessBanner = lipgloss.NewStyle().
				Foreground(ColorSuccess).
				Background(lipgloss.Color("#1B2A1E")).
				Padding(0, 1).
				Bold(true)

	StatusErrorBanner = lipgloss.NewStyle().
				Foreground(ColorError).
				Background(lipgloss.Color("#2A1B1E")).
				Padding(0, 1).
				Bold(true)

	// Help / Keymap Footer
	KeyBadge = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	KeyDesc = lipgloss.NewStyle().
		Foreground(ColorMuted)
)
