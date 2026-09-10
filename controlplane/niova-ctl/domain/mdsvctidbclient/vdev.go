package mdsvctidbclient

import (
	"strings"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

type createVdevRequest struct {
	Name          string   `json:"name,omitempty"`
	SizeBytes     int64    `json:"size_bytes"`
	Redundancy    int      `json:"redundancy,omitempty"`
	DataBlkCnt    int      `json:"data_blk_cnt"`
	ParityBlkCnt  int      `json:"parity_blk_cnt,omitempty"`
	FailureDomain string   `json:"failure_domain,omitempty"`
	EntityIDs     []string `json:"entity_ids,omitempty"`
	PFS           string   `json:"pfs,omitempty"`
}

type createVdevResp struct {
	VdevID        string `json:"vdev_id,omitempty"`
	Name          string `json:"name,omitempty"`
	ChunkCnt      int    `json:"num_chunks,omitempty"`
	FailureDomain string `json:"failure_domain,omitempty"`
	PFSID         string `json:"pfs_id,omitempty"`
}

// wireVdev mirrors what GET /api/resource?type=vdev echoes back (confirmed
// against the live server) — richer than createVdevResp.
type wireVdev struct {
	ID            string  `json:"id"`
	Name          string  `json:"name,omitempty"`
	Size          int64   `json:"size"`
	ChunkCnt      int     `json:"chunk_cnt"`
	Redundancy    int     `json:"redundancy"`
	DataBlkCnt    int     `json:"data_blk_cnt"`
	ParityBlkCnt  int     `json:"parity_blk_cnt"`
	PFSID         *string `json:"pfs_id"`
	FailureDomain string  `json:"failure_domain,omitempty"`
}

// CreateVdev creates a vdev. mdsvc-tidb assigns the ID server-side (unlike
// niova-mdsvc's client-supplied ID) — on success the server's
// id/chunk_cnt/pfs_id are written back into vdev.
func (c *Client) CreateVdev(vdev *ctlplfl.VdevConfig) error {
	pfs := vdev.PFSName
	if pfs == "" {
		pfs = vdev.PFSID
	}
	req := createVdevRequest{
		Name:         vdev.Name,
		SizeBytes:    vdev.Size,
		Redundancy:   int(vdev.Redundancy), // enum values are identical between backends
		DataBlkCnt:   int(vdev.DataBlkCnt),
		ParityBlkCnt: int(vdev.ParityBlkCnt),
		PFS:          pfs,
	}
	if vdev.FilterType != "" {
		req.FailureDomain = strings.ToUpper(vdev.FilterType)
	}
	if vdev.FilterID != "" {
		req.EntityIDs = []string{vdev.FilterID}
	}

	var resp createVdevResp
	if err := c.post("/api/vdev", req, &resp); err != nil {
		return err
	}
	vdev.ID = resp.VdevID
	vdev.ChunkCnt = uint32(resp.ChunkCnt)
	vdev.PFSID = resp.PFSID
	return nil
}

func (c *Client) GetVdevConfigs() ([]ctlplfl.VdevConfig, error) {
	var wire []wireVdev
	if err := c.getResourceList("vdev", &wire); err != nil {
		return nil, err
	}
	out := make([]ctlplfl.VdevConfig, len(wire))
	for i, w := range wire {
		cfg := ctlplfl.VdevConfig{
			ID:           w.ID,
			Name:         w.Name,
			Size:         w.Size,
			Redundancy:   ctlplfl.RedundancyMode(w.Redundancy),
			ChunkCnt:     uint32(w.ChunkCnt),
			DataBlkCnt:   uint8(w.DataBlkCnt),
			ParityBlkCnt: uint8(w.ParityBlkCnt),
			FilterType:   w.FailureDomain,
		}
		if w.PFSID != nil {
			cfg.PFSID = *w.PFSID
		}
		out[i] = cfg
	}
	return out, nil
}
