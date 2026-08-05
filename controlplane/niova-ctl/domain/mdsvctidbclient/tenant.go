package mdsvctidbclient

import (
	"net/url"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
)

// TenantInfo and TenantCreateResult are aliases for the domain types (rather
// than distinct mdsvctidbclient-local structs) so that domain.TenantManager /
// domain.TenantAdminAuthenticator — which Client implements below — can be
// declared in domain without domain having to import this package.
type TenantInfo = domain.TenantInfo
type TenantCreateResult = domain.TenantCreateResult

type wireTenantData struct {
	TenantUUID  string `json:"tenant_uuid"`
	SchemaName  string `json:"schema_name"`
	DisplayName string `json:"display_name,omitempty"`
	Status      int8   `json:"status"`
	CreatedBy   string `json:"created_by,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

func tenantFromWire(w wireTenantData) TenantInfo {
	return TenantInfo{
		TenantUUID:  w.TenantUUID,
		SchemaName:  w.SchemaName,
		DisplayName: w.DisplayName,
		Status:      w.Status,
		CreatedBy:   w.CreatedBy,
		CreatedAt:   w.CreatedAt,
	}
}

// TenantAdminLogin authenticates against the tenant-admin account (a
// separate identity from regular users — seeded at server startup, see
// TENANT_ADMIN_USERNAME/PASSWORD in mdsvc-tidb's .env). The returned token
// is scoped to the /cp/tenants lifecycle endpoints only.
func (c *Client) TenantAdminLogin(username, password string) (accessToken string, expiresIn int, err error) {
	var resp loginResp
	if err := c.post("/tenant-admin/login", loginRequest{Username: username, Password: password}, &resp); err != nil {
		return "", 0, err
	}
	return resp.AccessToken, resp.ExpiresIn, nil
}

func (c *Client) ListTenants(tenantAdminToken string) ([]TenantInfo, error) {
	var resp struct {
		Tenants []wireTenantData `json:"tenants"`
	}
	if err := c.withToken(tenantAdminToken).get("/cp/tenants", nil, &resp); err != nil {
		return nil, err
	}
	out := make([]TenantInfo, 0, len(resp.Tenants))
	for _, w := range resp.Tenants {
		t := tenantFromWire(w)
		if t.Status == 1 {
			out = append(out, t)
		}
	}
	return out, nil
}

type createTenantRequest struct {
	DisplayName string `json:"display_name,omitempty"`
}

type createTenantResp struct {
	Tenant        wireTenantData `json:"tenant"`
	AdminUsername string         `json:"admin_username"`
	AdminPassword string         `json:"admin_password"`
}

func (c *Client) CreateTenant(tenantAdminToken, displayName string) (*TenantCreateResult, error) {
	var resp createTenantResp
	if err := c.withToken(tenantAdminToken).post("/cp/tenants", createTenantRequest{DisplayName: displayName}, &resp); err != nil {
		return nil, err
	}
	return &TenantCreateResult{
		Tenant:        tenantFromWire(resp.Tenant),
		AdminUsername: resp.AdminUsername,
		AdminPassword: resp.AdminPassword,
	}, nil
}

func (c *Client) DeleteTenant(tenantAdminToken, tenantUUID string) error {
	return c.withToken(tenantAdminToken).delete("/cp/tenants/" + url.PathEscape(tenantUUID))
}
