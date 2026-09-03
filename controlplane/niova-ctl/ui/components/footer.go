package components

import (
	"github.com/charmbracelet/lipgloss"
)

// RenderFooter builds the bottom navigation bar as a single breadcrumb pill
func RenderFooter(activeTab int) string {
	pillStyleActive := lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#06B6D4")).Bold(true).Padding(0, 1)

	tabNames := []string{"users", "pdus", "racks", "hypervisors", "disks", "partitions", "nisds", "vdevs", "pfs", "topology", "tenants", "authz"}

	if activeTab >= 0 && activeTab < len(tabNames) {
		return pillStyleActive.Render("<" + tabNames[activeTab] + ">")
	}
	return ""
}
