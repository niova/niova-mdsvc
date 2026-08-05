package ui

import (
	"fmt"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
)

// backendLabel identifies which control-plane backend and endpoint this
// session is talking to, for display in the header.
func (a *App) backendLabel() string {
	if a.Backend == domain.BackendMdsvcTidb {
		if a.MdsvcURL != "" {
			return fmt.Sprintf("mdsvc-tidb (%s)", a.MdsvcURL)
		}
		return "mdsvc-tidb"
	}
	if a.RaftUUID != "" {
		return fmt.Sprintf("niova-mdsvc (%s)", a.RaftUUID)
	}
	return "niova-mdsvc"
}

// tenantLabel reports which tenant this session is scoped to, or "" if none
// is established yet, or if this backend has no tenant concept at all
// (niova-mdsvc — see hasTenantAdminAccess). Distinguished from "" via
// IsLoggedIn() since LoginTenantUUID is also empty for the default tenant.
func (a *App) tenantLabel() string {
	if !a.IsLoggedIn() || !a.hasTenantAdminAccess() {
		return ""
	}
	if a.Auth.TenantUUID != "" {
		return a.Auth.TenantUUID
	}
	return "default"
}

// hasTenantAdminAccess reports whether this backend has a tenant registry at
// all — domain.TenantAdminAuthenticator (mdsvc-tidb only); niova-mdsvc has no
// tenant concept, so :tenant-login and the header's Tenant row both stay
// hidden against it, mirroring hasAuthzAccess's own capability check below.
func (a *App) hasTenantAdminAccess() bool {
	_, ok := a.CPClient.(domain.TenantAdminAuthenticator)
	return ok
}

// isAdmin reports whether the current session can perform admin-gated
// actions. When auth is disabled server-side, treat everyone as admin
// (mirrors mdsvc-tidb's RequireRole). Exact for mdsvc-tidb; an
// approximation for niova-mdsvc's dynamic RBAC/ABAC engine.
func (a *App) isAdmin() bool {
	if !a.AuthEnabled {
		return true
	}
	return a.Auth.User != nil && a.Auth.User.IsAdmin
}

// hasResourceAccess reports whether the session can view infrastructure
// resources at all. Those endpoints require a regular Admin/User login — a
// standalone tenant-admin session has none, so resource tabs stay hidden.
func (a *App) hasResourceAccess() bool {
	if !a.AuthEnabled {
		return true
	}
	return a.IsLoggedIn()
}

// hasAuthzAccess reports whether the session can reach RBAC/ABAC policy
// management — an admin session against a backend that implements
// domain.AuthzManager (mdsvc-tidb only). Mirrors loadAuthzView's checks.
func (a *App) hasAuthzAccess() bool {
	if !a.isAdmin() {
		return false
	}
	_, ok := a.CPClient.(domain.AuthzManager)
	return ok
}

// partitionWriter returns the backend's domain.PartitionWriter capability
// and whether it has one (niova-mdsvc only). Centralizes the type
// assertion so every caller probes the same capability the same way.
func (a *App) partitionWriter() (domain.PartitionWriter, bool) {
	pw, ok := a.CPClient.(domain.PartitionWriter)
	return pw, ok
}

// supportsPartitionWrite reports whether partitionWriter() has a capability,
// for the call sites that only need the yes/no (gating, usePartitions)
// rather than the writer itself.
func (a *App) supportsPartitionWrite() bool {
	_, ok := a.partitionWriter()
	return ok
}

// canRefreshCPData reports whether it's safe to call CPClient for cached
// topology/NISD/Vdev/PFS data. False pre-login and for a tenant-admin-only
// session (no token authorizing /api/ calls) when auth is required; true
// unconditionally for a no-auth deployment.
func (a *App) canRefreshCPData() bool {
	return a.CPClient != nil && (!a.AuthEnabled || a.IsLoggedIn())
}

// tabRequiresAdmin reports whether adding/editing a resource on this tab is
// gated to admins only, per mdsvc-tidb's cmd/server/main.go route table:
// PUT /resource (PDU/Rack/Hypervisor/Device/NISD) and POST /pfs are
// Admin-only. Vdev (tab 7) isn't listed — it's Admin+User for all actions.
func tabRequiresAdmin(tab int) bool {
	switch tab {
	case 1, 2, 3, 4, 6, 8: // PDU, Rack, Hypervisor, Device, NISD, PFS
		return true
	case 9: // Topology's "a" imports PDU/Rack/Hypervisor/Device/NISD — same writes as above
		return true
	default:
		return false
	}
}
