package mdsvctidbclient

import (
	"fmt"
	"net/url"
)

// ChunkInfo is one chunk's replica placement, as returned by GetChunk.
type ChunkInfo struct {
	VdevID   string
	ChunkIdx int
	NisdIDs  []string
}

func (c *Client) GetChunk(vdevID string, chunkIdx int) (*ChunkInfo, error) {
	var resp struct {
		VdevID   string   `json:"vdev_id"`
		ChunkIdx int      `json:"chunk_idx"`
		NisdIDs  []string `json:"NisdIDs"` // capitalized in the wire response, confirmed against the live server
	}
	q := url.Values{"vdev_id": {vdevID}, "chunk_idx": {fmt.Sprint(chunkIdx)}}
	if err := c.get("/api/chunk", q, &resp); err != nil {
		return nil, err
	}
	return &ChunkInfo{VdevID: resp.VdevID, ChunkIdx: resp.ChunkIdx, NisdIDs: resp.NisdIDs}, nil
}

// ChunkPlacement is one chunk's placement + geometry, as returned within a
// GetChunks page.
type ChunkPlacement struct {
	ChunkIdx     int
	NisdIDs      []string
	DataBlkCnt   int
	ParityBlkCnt int
	Redundancy   int
}

// ChunksPage is one page of a vdev's chunk placements.
type ChunksPage struct {
	VdevID            string
	StartChunkIdx     int
	NextStartChunkIdx int
	HasMore           bool
	ChunkCnt          int
	Chunks            []ChunkPlacement
}

// GetChunks fetches one page of chunk placements starting at startChunkIdx
// (0 for the first page). Use page.NextStartChunkIdx/page.HasMore to fetch
// the next page.
func (c *Client) GetChunks(vdevID string, startChunkIdx, limit int) (*ChunksPage, error) {
	var resp struct {
		VdevID            string `json:"vdev_id"`
		StartChunkIdx     int    `json:"start_chunk_idx"`
		NextStartChunkIdx int    `json:"next_start_chunk_idx"`
		HasMore           bool   `json:"has_more"`
		ChunkCnt          int    `json:"chunk_cnt,omitempty"`
		Chunks            []struct {
			ChunkIdx     int      `json:"chunk_idx"`
			NisdIDs      []string `json:"nisd_ids"`
			DataBlkCnt   int      `json:"data_blk_cnt"`
			ParityBlkCnt int      `json:"parity_blk_cnt"`
			Redundancy   int      `json:"redundancy"`
		} `json:"chunks"`
	}
	q := url.Values{"vdev_id": {vdevID}, "start_chunk_idx": {fmt.Sprint(startChunkIdx)}}
	if limit > 0 {
		q.Set("limit", fmt.Sprint(limit))
	}
	if err := c.get("/api/chunks", q, &resp); err != nil {
		return nil, err
	}
	page := &ChunksPage{
		VdevID:            resp.VdevID,
		StartChunkIdx:     resp.StartChunkIdx,
		NextStartChunkIdx: resp.NextStartChunkIdx,
		HasMore:           resp.HasMore,
		ChunkCnt:          resp.ChunkCnt,
	}
	for _, ch := range resp.Chunks {
		page.Chunks = append(page.Chunks, ChunkPlacement{
			ChunkIdx: ch.ChunkIdx, NisdIDs: ch.NisdIDs,
			DataBlkCnt: ch.DataBlkCnt, ParityBlkCnt: ch.ParityBlkCnt, Redundancy: ch.Redundancy,
		})
	}
	return page, nil
}

type reassignRequest struct {
	VdevID           string `json:"vdev_id"`
	ChunkIdx         int    `json:"chunk_idx"`
	FailedReplicaIdx int    `json:"failed_replica_idx"`
	VdevConfigNum    int64  `json:"vdev_config_num"`
	SnapshotSeqno    int64  `json:"snapshot_seqno"`
}

// ReassignResult is the outcome of reassigning a failed chunk replica.
type ReassignResult struct {
	NisdID        string
	VdevConfigNum int64
	SnapshotSeqno int64
	Applied       bool
}

func (c *Client) Reassign(vdevID string, chunkIdx, failedReplicaIdx int, vdevConfigNum, snapshotSeqno int64) (*ReassignResult, error) {
	var resp struct {
		NisdID        string `json:"nisd_id"`
		VdevConfigNum int64  `json:"vdev_config_num"`
		SnapshotSeqno int64  `json:"snapshot_seqno"`
		Applied       bool   `json:"applied"`
	}
	req := reassignRequest{VdevID: vdevID, ChunkIdx: chunkIdx, FailedReplicaIdx: failedReplicaIdx, VdevConfigNum: vdevConfigNum, SnapshotSeqno: snapshotSeqno}
	if err := c.post("/api/chunk/reassign", req, &resp); err != nil {
		return nil, err
	}
	return &ReassignResult{NisdID: resp.NisdID, VdevConfigNum: resp.VdevConfigNum, SnapshotSeqno: resp.SnapshotSeqno, Applied: resp.Applied}, nil
}

type mountVdevRequest struct {
	VdevID string `json:"vdev_id"`
}

// MountResult is the outcome of mounting a vdev.
type MountResult struct {
	ID            string
	MountCounter  uint64
	Size          int64
	AccessToken   string
	DataBlkCnt    int
	ParityBlkCnt  int
	Redundancy    int
	LastMountedAt string
}

func (c *Client) MountVdev(vdevID string) (*MountResult, error) {
	var resp struct {
		ID            string `json:"id"`
		MountCounter  uint64 `json:"mount_counter"`
		Size          int64  `json:"size"`
		AccessToken   string `json:"access_token,omitempty"`
		DataBlkCnt    int    `json:"data_blk_cnt"`
		ParityBlkCnt  int    `json:"parity_blk_cnt,omitempty"`
		Redundancy    int    `json:"redundancy"`
		LastMountedAt string `json:"last_mounted_at,omitempty"`
	}
	if err := c.post("/api/vdev/mount", mountVdevRequest{VdevID: vdevID}, &resp); err != nil {
		return nil, err
	}
	return &MountResult{
		ID: resp.ID, MountCounter: resp.MountCounter, Size: resp.Size, AccessToken: resp.AccessToken,
		DataBlkCnt: resp.DataBlkCnt, ParityBlkCnt: resp.ParityBlkCnt, Redundancy: resp.Redundancy,
		LastMountedAt: resp.LastMountedAt,
	}, nil
}

func (c *Client) DeleteVdev(vdevID string) error {
	return c.delete("/api/vdev/" + url.PathEscape(vdevID))
}
