package mdsvctidbclient

import (
	"net/url"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
)

// RBACPolicy and ABACPolicy are aliases for the domain types (rather than
// distinct mdsvctidbclient-local structs) so that domain.AuthzManager — which
// Client implements below — can be declared in domain without domain having
// to import this package.
type RBACPolicy = domain.RBACPolicy
type ABACPolicy = domain.ABACPolicy

func (c *Client) ListRBACPolicies() ([]RBACPolicy, error) {
	var resp struct {
		Policies []RBACPolicy `json:"policies"`
	}
	if err := c.get("/api/authz/rbac", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Policies, nil
}

func (c *Client) AddRBACPolicy(p RBACPolicy) error {
	return c.post("/api/authz/rbac", p, nil)
}

func (c *Client) DeleteRBACPolicy(p RBACPolicy) error {
	return c.delete("/api/authz/rbac/" + url.PathEscape(p.Resource) + "/" + url.PathEscape(p.Action) + "/" + url.PathEscape(p.Role))
}

func (c *Client) ListABACPolicies() ([]ABACPolicy, error) {
	var resp struct {
		Policies []ABACPolicy `json:"policies"`
	}
	if err := c.get("/api/authz/abac", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Policies, nil
}

func (c *Client) AddABACPolicy(p ABACPolicy) error {
	return c.post("/api/authz/abac", p, nil)
}

func (c *Client) DeleteABACPolicy(p ABACPolicy) error {
	return c.delete("/api/authz/abac/" + url.PathEscape(p.Resource) + "/" + url.PathEscape(p.Action))
}
