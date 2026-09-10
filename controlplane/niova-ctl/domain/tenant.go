package domain

// TenantInfo describes one tenant in mdsvc-tidb's tenant registry.
type TenantInfo struct {
	TenantUUID  string
	SchemaName  string
	DisplayName string
	Status      int8 // 1 = active, 2 = deleted (mdsvc-tidb's controlplane repo TenantStatus* constants)
	CreatedBy   string
	CreatedAt   string
}

// StatusString renders Status for display.
func (t TenantInfo) StatusString() string {
	switch t.Status {
	case 1:
		return "active"
	case 2:
		return "deleted"
	default:
		return "unknown"
	}
}

// TenantCreateResult is the outcome of creating a tenant: the tenant record
// plus the one-time admin credentials generated for it.
type TenantCreateResult struct {
	Tenant        TenantInfo
	AdminUsername string
	AdminPassword string
}

// TenantAdminAuthenticator is implemented by backends with a distinct
// tenant-admin identity/login, separate from the regular Admin/User login
// (mdsvc-tidb's tenant registry, seeded independently of any data tenant).
// niova-mdsvc has no tenant concept at all.
type TenantAdminAuthenticator interface {
	TenantAdminLogin(username, password string) (accessToken string, expiresIn int, err error)
}

// TenantManager is implemented by backends that expose a tenant lifecycle
// (mdsvc-tidb only) — list/create/delete tenants using a tenant-admin
// session token obtained via TenantAdminAuthenticator.TenantAdminLogin.
type TenantManager interface {
	ListTenants(tenantAdminToken string) ([]TenantInfo, error)
	CreateTenant(tenantAdminToken, displayName string) (*TenantCreateResult, error)
	DeleteTenant(tenantAdminToken, tenantUUID string) error
}
