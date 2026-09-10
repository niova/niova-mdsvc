package mdsvctidbclient

import (
	"net/http"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
)

var (
	_ domain.ControlPlaneClient = (*Client)(nil)
	_ domain.UserServiceClient  = (*Client)(nil)

	// Capabilities this backend additionally supports — asserted here so
	// drift fails the build instead of a silent runtime capability-check miss.
	_ domain.VdevDeleter              = (*Client)(nil)
	_ domain.TenantAdminAuthenticator = (*Client)(nil)
	_ domain.TenantManager            = (*Client)(nil)
	_ domain.AuthzManager             = (*Client)(nil)

	// Deliberately NOT implemented by this backend:
	// domain.PFSEditor (see pfs.go) and domain.PartitionWriter — mdsvc-tidb
	// has no device_partition resource type at all (neither GET nor PUT
	// /api/resource accepts it; see mdsvc-tidb's
	// internal/constants/resources.go APIResourceTypeIndexes /
	// APIMutableResourceTypeIndexes, which both omit DevicePartition).
)

// ProbeAuth reports whether the server has authentication enabled.
// mdsvc-tidb always responds HTTP 200, even on rejection, so the decision
// comes from the envelope's Status field, not the HTTP status code.
func ProbeAuth(baseURL string) (authEnabled bool, err error) {
	probe := New(baseURL)
	env, err := probe.doRaw(http.MethodGet, "/api/infra", nil, nil)
	if err != nil {
		return false, err
	}
	return env.Status != StatusOK, nil
}
