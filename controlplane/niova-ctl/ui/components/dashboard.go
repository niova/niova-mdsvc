package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/ui/styles"
)

type SidebarItem struct {
	Key   string
	Label string
	Count int
}

type DashboardProps struct {
	ActiveTab       int
	Metrics         ClusterMetrics
	MainContentView string
	Width           int
	Height          int
	// IsAdmin hides the Users sidebar entry when false — admin-only
	// regardless of backend (mdsvc-tidb: GET/POST /users is
	// RequireRole(RoleAdmin) only). Authz is also admin-only but needs the
	// backend check HasAuthzAccess folds in too — see its own doc.
	IsAdmin bool
	// HasResourceAccess hides the PDUs..Topology sidebar entries when false.
	// Those routes (GET /infra, /nisd, /resource, /vdev, /pfs) require a
	// regular Admin/User login — a standalone tenant-admin session has none
	// of that, so there's nothing there for it to show.
	HasResourceAccess bool
	// HasTenantAdmin hides the Tenants sidebar entry when false. Tenant
	// management requires a wholly separate tenant-admin session (via
	// :tenant-login) — it's never tied to the regular user's role at all,
	// so this is independent of IsAdmin.
	HasTenantAdmin bool
	// HasAuthzAccess hides the Authz sidebar entry when false — RBAC/ABAC
	// policy management only exists against mdsvc-tidb (domain.AuthzManager);
	// niova-mdsvc has no policy engine to manage, so IsAdmin alone isn't
	// enough to show this one, unlike Users.
	HasAuthzAccess bool
}

// sidebarStyle and mainPanelStyle are package-level so MainPanelContentWidth
// can measure the exact same style objects RenderDashboard renders with,
// rather than a hand-counted budget kept in sync by hand elsewhere — that
// mismatch is exactly how a table once ended up sized wider than its room.
var (
	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.ColorMuted).
			Padding(0, 1).
			Width(24)

	mainPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.ColorPrimary).
			Padding(0, 1)

	// horizontalGap is the literal " " RenderDashboard joins the sidebar
	// and main panel with — kept alongside the styles above for the same
	// reason: one number, measured where it's true, not re-derived elsewhere.
	horizontalGap = 1
)

// MainPanelContentWidth returns how many columns of raw content the main
// panel's own content (DashboardProps.MainContentView) actually gets to
// use, for a given terminal width — i.e. termWidth minus the sidebar,
// the gap between them, and the main panel's own border/padding, all
// measured by actually rendering them rather than hand-counted.
func MainPanelContentWidth(termWidth int) int {
	sidebarWidth := lipgloss.Width(sidebarStyle.Render(""))
	mainPanelOverhead := lipgloss.Width(mainPanelStyle.Render(""))
	avail := termWidth - sidebarWidth - horizontalGap - mainPanelOverhead
	if avail < 10 {
		avail = 10
	}
	return avail
}

// RenderDashboard renders side-by-side split-panel dashboard layout
func RenderDashboard(props DashboardProps) string {
	var sidebarItems []SidebarItem
	if props.HasResourceAccess {
		sidebarItems = append(sidebarItems,
			SidebarItem{Key: "1", Label: "PDUs", Count: props.Metrics.PDUCount},
			SidebarItem{Key: "2", Label: "Racks", Count: props.Metrics.RackCount},
			SidebarItem{Key: "3", Label: "Hypervisors", Count: props.Metrics.HypervisorCount},
			SidebarItem{Key: "4", Label: "Disks", Count: props.Metrics.DeviceCount},
			SidebarItem{Key: "5", Label: "Partitions", Count: props.Metrics.PartitionCount},
			SidebarItem{Key: "6", Label: "NISDs", Count: props.Metrics.NISDCount},
			SidebarItem{Key: "7", Label: "Vdevs", Count: props.Metrics.VdevCount},
			SidebarItem{Key: "8", Label: "PFS", Count: props.Metrics.PFSCount},
			SidebarItem{Key: "9", Label: "Topology", Count: props.Metrics.PDUCount},
		)
	}
	if props.IsAdmin {
		sidebarItems = append(sidebarItems, SidebarItem{Key: "0", Label: "Users", Count: -1})
	}
	if props.HasTenantAdmin {
		sidebarItems = append(sidebarItems, SidebarItem{Key: "t", Label: "Tenants", Count: -1})
	}
	if props.HasAuthzAccess {
		sidebarItems = append(sidebarItems, SidebarItem{Key: "z", Label: "Authz", Count: -1})
	}

	var sbLines []string
	sbLines = append(sbLines, styles.TextBold.Render("Navigation"))
	sbLines = append(sbLines, styles.TextMuted.Render("────────────"))

	activeKey := fmt.Sprintf("%d", props.ActiveTab)
	if props.ActiveTab == 10 {
		activeKey = "t"
	} else if props.ActiveTab == 11 {
		activeKey = "z"
	}

	for _, item := range sidebarItems {
		var line string
		if item.Count < 0 {
			line = fmt.Sprintf("%s. %-11s", item.Key, item.Label)
		} else {
			line = fmt.Sprintf("%s. %-11s (%d)", item.Key, item.Label, item.Count)
		}
		if item.Key == activeKey {
			sbLines = append(sbLines, styles.SelectedItem.Render("▶ "+line))
		} else {
			sbLines = append(sbLines, styles.UnselectedItem.Render("  "+line))
		}
	}

	sidebarView := sidebarStyle.Render(strings.Join(sbLines, "\n"))
	mainBox := mainPanelStyle.Render(props.MainContentView)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		sidebarView,
		strings.Repeat(" ", horizontalGap),
		mainBox,
	)
}
