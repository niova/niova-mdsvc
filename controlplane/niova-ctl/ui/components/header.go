package components

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

type ClusterMetrics struct {
	PDUCount        int
	RackCount       int
	HypervisorCount int
	DeviceCount     int
	PartitionCount  int
	NISDCount       int
	VdevCount       int
	PFSCount        int
}

// HeaderProps contains data for header rendering
type HeaderProps struct {
	Title       string
	CPConnected bool
	AuthEnabled bool
	// LoggedInUser is the current identity's username, or "" if none is
	// established yet — the row is omitted entirely rather than shown as a
	// placeholder. Populated from the regular Admin/User login, or (when
	// there is no regular login) from a standalone tenant-admin session.
	LoggedInUser string
	// Role is the current identity's role (e.g. "admin", "user", or
	// "tenant-admin" for a standalone tenant-admin session), or "" if none
	// is established yet — the row is omitted entirely in that case.
	Role string
	// Backend identifies which control-plane backend this session is
	// talking to and where (e.g. "mdsvc-tidb (http://localhost:8081)" or
	// "niova-mdsvc (raft-uuid)") — computed by the caller, which is the only
	// place that holds the backend kind and connection target.
	Backend string
	// Tenant is the tenant this session is scoped to (a UUID or "default"),
	// or "" if none is established yet — the row is omitted entirely rather
	// than shown as a placeholder, since it's meaningless before login.
	Tenant              string
	Breadcrumbs         string
	Metrics             ClusterMetrics
	ShowHotkeys         bool
	HideResourceActions bool
	HideGlobalShortcuts bool
	KeyBindings         []key.Binding
}

// buildActionItems mirrors the keymap-grid contents RenderHeader lays out in
// its middle column — factored out so HeaderHeight can size against the same
// list without duplicating (and risking drift from) the filtering rules.
func buildActionItems(keyBindings []key.Binding, hideResourceActions, hideGlobalShortcuts bool) [][2]string {
	var actionItems [][2]string

	if !hideResourceActions {
		for _, kb := range keyBindings {
			h := kb.Help()
			keyStr := h.Key
			if !strings.HasPrefix(keyStr, "<") {
				keyStr = strings.ReplaceAll(keyStr, "+", "-")
				keyStr = "<" + strings.ToLower(keyStr) + ">"
			}
			actionItems = append(actionItems, [2]string{keyStr, h.Desc})
		}
	}

	// Insert <ctrl+r> Refresh Data at position 1 (Row 0, Col 2) alongside the first action item
	if !hideGlobalShortcuts {
		refreshItem := [2]string{"<ctrl+r>", "Refresh Data"}
		if len(actionItems) > 0 {
			actionItems = append(actionItems[:1], append([][2]string{refreshItem}, actionItems[1:]...)...)
		} else {
			actionItems = append(actionItems, refreshItem)
		}
	}

	return actionItems
}

// HeaderHeight returns the terminal rows RenderHeader will occupy. Grows by
// one row per two keybinding entries, so callers sizing content below it
// must use this rather than a hardcoded constant.
func HeaderHeight(keyBindings []key.Binding, hideResourceActions, hideGlobalShortcuts bool) int {
	actionItems := buildActionItems(keyBindings, hideResourceActions, hideGlobalShortcuts)

	gridRows := (len(actionItems) + 1) / 2
	midHeight := gridRows
	// Only meaningfully invoked once logged in (see call site), where all
	// five rows are always populated — Backend/Tenant/User/Role/Status.
	leftHeight := 5
	logoHeight := 4

	h := leftHeight
	if midHeight > h {
		h = midHeight
	}
	if logoHeight > h {
		h = logoHeight
	}
	return h
}

// RenderHeader builds the 3-column top header banner
func RenderHeader(props HeaderProps) string {
	// Column 1: Context & Cluster Metadata
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Bold(true)
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))

	status := "Offline"
	if props.CPConnected {
		status = "Connected"
	}
	backend := props.Backend
	if backend == "" {
		backend = "-"
	}

	// Tenant/User/Role are only meaningful once some session (regular or
	// tenant-admin) is established — an empty value means the row is
	// omitted entirely rather than shown as a "-" placeholder.
	leftLines := []string{
		fmt.Sprintf("%s %s", labelStyle.Render("Backend: "), valStyle.Render(backend)),
	}
	if props.Tenant != "" {
		leftLines = append(leftLines, fmt.Sprintf("%s %s", labelStyle.Render("Tenant:  "), valStyle.Render(props.Tenant)))
	}
	if props.LoggedInUser != "" {
		leftLines = append(leftLines, fmt.Sprintf("%s %s", labelStyle.Render("User:    "), valStyle.Render(props.LoggedInUser)))
	}
	if props.Role != "" {
		leftLines = append(leftLines, fmt.Sprintf("%s %s", labelStyle.Render("Role:    "), valStyle.Render(props.Role)))
	}
	leftLines = append(leftLines, fmt.Sprintf("%s %s", labelStyle.Render("Status:  "), valStyle.Render(status)))
	leftCol := strings.Join(leftLines, "\n")

	// Column 2: Keymap Grid for Active View
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#3B82F6")).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted)

	// Build keymaps grid for current active window. Sourced
	// entirely from the active page's own KeyBindings() — every Page
	// implementation provides one, so there is no per-tab fallback list
	// here to fall out of sync with what a page actually supports.
	actionItems := buildActionItems(props.KeyBindings, props.HideResourceActions, props.HideGlobalShortcuts)

	col1Width := 28

	var gridRows []string
	for i := 0; i < len(actionItems); i += 2 {
		item1 := actionItems[i]
		k1Str := keyStyle.Render(item1[0]) + " " + descStyle.Render(item1[1])
		visibleLen1 := utf8.RuneCountInString(item1[0]) + 1 + utf8.RuneCountInString(item1[1])
		pad1 := col1Width - visibleLen1
		if pad1 < 1 {
			pad1 = 1
		}
		col1Str := k1Str + strings.Repeat(" ", pad1)

		col2Str := ""
		if i+1 < len(actionItems) {
			item2 := actionItems[i+1]
			k2Str := keyStyle.Render(item2[0]) + " " + descStyle.Render(item2[1])
			col2Str = k2Str
		}
		gridRows = append(gridRows, col1Str+col2Str)
	}

	midCol := strings.Join(gridRows, "\n")

	// Column 3: ASCII Art Banner
	logoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Bold(true)
	logoAscii := logoStyle.Render(
		"  _   _  _____  ____   _   _   ___  \n" +
			" | \\ | ||_   _|/ __ \\ | | | | / _ \\ \n" +
			" |  \\| |  | | | |  | || | | |/ /_\\ \\\n" +
			" |_|\\__| _| |_ \\____/  \\___//_/   \\_\\",
	)

	// Combine 3 columns horizontally
	headerBox := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftCol,
		strings.Repeat(" ", 4),
		midCol,
		strings.Repeat(" ", 6),
		logoAscii,
	)

	return headerBox
}
