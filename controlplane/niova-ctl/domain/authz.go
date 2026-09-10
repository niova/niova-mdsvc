package domain

// RBACPolicy is a (resource, action, role) grant: the named role may perform
// the named action on the named resource type.
type RBACPolicy struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Role     string `json:"role"`
}

// ABACPolicy is a (resource, action) ownership check: only the resource's
// owner may perform the named action.
type ABACPolicy struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

// AuthzManager is implemented by backends with an RBAC/ABAC policy engine
// (mdsvc-tidb only). Kept out of ControlPlaneClient (ISP) — callers
// type-assert for it instead.
type AuthzManager interface {
	ListRBACPolicies() ([]RBACPolicy, error)
	AddRBACPolicy(p RBACPolicy) error
	DeleteRBACPolicy(p RBACPolicy) error
	ListABACPolicies() ([]ABACPolicy, error)
	AddABACPolicy(p ABACPolicy) error
	DeleteABACPolicy(p ABACPolicy) error
}
