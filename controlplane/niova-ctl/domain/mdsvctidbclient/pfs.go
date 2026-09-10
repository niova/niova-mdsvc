package mdsvctidbclient

import (
	"fmt"
	"net/url"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

type createPFSRequest struct {
	Name string `json:"name"`
}

type createPFSResp struct {
	PFSID string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
}

type wirePFS struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Offset  int      `json:"offset"`
	VdevIDs []string `json:"vdev_ids"`
}

// CreatePFS creates a new PFS. mdsvc-tidb has no PFS update endpoint, so
// Client has no EditPFS method (domain.PFSEditor isn't implemented). The ID
// check below is a caller-bug guard: a pre-existing ID here means something
// upstream mixed up create and edit.
func (c *Client) CreatePFS(pfs *ctlplfl.PFS) error {
	if pfs.ID != "" {
		return fmt.Errorf("mdsvctidbclient: CreatePFS called with a pre-existing ID %q — this backend has no PFS update endpoint (domain.PFSEditor isn't implemented)", pfs.ID)
	}
	var resp createPFSResp
	if err := c.post("/api/pfs", createPFSRequest{Name: pfs.Name}, &resp); err != nil {
		return err
	}
	pfs.ID = resp.PFSID
	return nil
}

func (c *Client) getPFSByID(id string) (*ctlplfl.PFS, error) {
	var w wirePFS
	if err := c.get("/api/pfs", url.Values{"id": {id}}, &w); err != nil {
		return nil, err
	}
	return &ctlplfl.PFS{ID: w.ID, Name: w.Name, Offset: w.Offset, VdevIDs: w.VdevIDs}, nil
}

// GetPFS derives a PFS listing since mdsvc-tidb has no bulk-list endpoint
// for PFS: enumerate every vdev's pfs_id and fetch each distinct one. This
// necessarily misses any PFS with zero vdevs — a known limitation.
func (c *Client) GetPFS() ([]ctlplfl.PFS, error) {
	vdevs, err := c.GetVdevConfigs()
	if err != nil {
		return nil, fmt.Errorf("mdsvctidbclient: deriving PFS list from vdevs: %w", err)
	}

	seen := make(map[string]bool)
	var out []ctlplfl.PFS
	for _, v := range vdevs {
		if v.PFSID == "" || seen[v.PFSID] {
			continue
		}
		seen[v.PFSID] = true
		pfs, err := c.getPFSByID(v.PFSID)
		if err != nil {
			continue // best-effort: skip PFS entries that fail to resolve
		}
		out = append(out, *pfs)
	}
	return out, nil
}
