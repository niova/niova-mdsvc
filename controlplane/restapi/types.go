// Package restapi defines the REST wire contract for the mdsvc control plane
// HTTP API. The DTOs here mirror the schemas in openapi.yaml exactly (flat,
// snake_case JSON) so that consumers can talk to either the niova-mdsvc or the
// TiDB control plane interchangeably.
//
// These types are intentionally decoupled from the internal CPReq/CPResp
// envelope and domain structs (controlplane/ctlplanefuncs/lib): the proxy REST
// handlers translate between the two.
package restapi

// NetworkInfo is a single ip/port endpoint for a NISD.
type NetworkInfo struct {
	IPAddr string `json:"ip_addr"`
	Port   int    `json:"port"`
}

// ---- /create_infra ----

// CreateInfraRequest is the nested PDU -> Rack -> Hypervisor -> Device -> NISD
// hierarchy. IDs may be omitted and are generated server-side.
type CreateInfraRequest struct {
	PDUs []PDUNode `json:"pdus"`
}

type PDUNode struct {
	ID            string     `json:"id,omitempty"`
	Name          string     `json:"name,omitempty"`
	Location      string     `json:"location,omitempty"`
	Specification string     `json:"specification,omitempty"`
	PowerCap      string     `json:"power_cap,omitempty"`
	Racks         []RackNode `json:"racks,omitempty"`
}

type RackNode struct {
	ID            string           `json:"id,omitempty"`
	Name          string           `json:"name,omitempty"`
	Location      string           `json:"location,omitempty"`
	Specification string           `json:"specification,omitempty"`
	Hypervisors   []HypervisorNode `json:"hypervisors,omitempty"`
}

type HypervisorNode struct {
	ID          string       `json:"id,omitempty"`
	Name        string       `json:"name,omitempty"`
	PortRange   string       `json:"port_range,omitempty"`
	SSHPort     string       `json:"ssh_port,omitempty"` // defaults to "22" when omitted
	RDMAEnabled bool         `json:"rdma_enabled,omitempty"`
	IPAddrs     []string     `json:"ip_addrs,omitempty"`
	Devices     []DeviceNode `json:"devices,omitempty"`
}

type DeviceNode struct {
	ID            string     `json:"id,omitempty"`
	Name          string     `json:"name,omitempty"`
	DevicePath    string     `json:"device_path,omitempty"`
	SerialNumber  string     `json:"serial_number,omitempty"`
	State         int        `json:"state,omitempty"` // defaults to 2 when omitted/0
	Size          int64      `json:"size,omitempty"`
	FailureDomain string     `json:"failure_domain,omitempty"` // defaults to "fd-<device_id>"
	NISDs         []NISDNode `json:"nisds,omitempty"`
}

type NISDNode struct {
	ID            string        `json:"id,omitempty"`
	PeerPort      int           `json:"peer_port,omitempty"`
	TotalSize     int64         `json:"total_size,omitempty"`
	AvailableSize int64         `json:"available_size,omitempty"` // defaults to total_size when omitted/0
	SocketPath    string        `json:"socket_path,omitempty"`
	NetInfo       []NetworkInfo `json:"net_info,omitempty"`
}

// ---- /create_vdev ----

type CreateVdevRequest struct {
	SizeBytes    int64 `json:"size_bytes"`
	DataBlkCnt   int   `json:"data_blk_cnt"`             // data blocks per chunk (replica count for replication)
	ParityBlkCnt int   `json:"parity_blk_cnt,omitempty"` // parity blocks per chunk (0 for replication)
	// Redundancy selects the chunk redundancy scheme: 0=REPLICATION, 1=EC_32K,
	// 2=EC_64K, 3=EC_128K. See models.Redundancy for the full mapping.
	Redundancy int `json:"redundancy,omitempty"`
	// FailureDomain scopes NISD allocation to a failure-domain level
	// ("pdu"|"rack"|"hv"|"device"|"partition"); empty/"any" auto-picks.
	FailureDomain string `json:"failure_domain,omitempty"`
	// EntityIDs restricts allocation to specific failure-domain entities. niova's
	// allocator scopes to a single entity, so only the first id is applied.
	EntityIDs []string `json:"entity_ids,omitempty"`
	// PFS optionally links the vdev to a parallel file system, given as either the
	// PFS id (UUID) or its name. The control plane records the membership and
	// applies the PFS placement offset.
	PFS string `json:"pfs,omitempty"`
	// Name is a niova-only extension (not part of the TiDB contract): a
	// human-readable vdev name the control plane indexes for name-based lookup.
	// Must be alphanumeric; the server rejects invalid names.
	Name string `json:"name,omitempty"`
}

// CreateVdevPayload is the payload of the create-vdev envelope
// (APIResponse[CreateVdevPayload]). name/failure_domain/pfs_id echo the applied
// request for TiDB parity (failure_domain is the normalized level; pfs_id is set
// only when the PFS was supplied as a UUID).
type CreateVdevPayload struct {
	VdevID        string `json:"vdev_id,omitempty"`
	Name          string `json:"name,omitempty"`
	NumChunks     int    `json:"num_chunks,omitempty"`
	FailureDomain string `json:"failure_domain,omitempty"`
	PFSID         string `json:"pfs_id,omitempty"`
}

// ---- /vdev ----

// GetVdevPayload is the payload of the get-vdev envelope. name/failure_domain are
// included for TiDB parity (failure_domain is the level scoped at creation).
type GetVdevPayload struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name,omitempty"`
	Size          int64  `json:"size,omitempty"`
	NumChunks     int    `json:"num_chunks,omitempty"`
	NumReplicas   int    `json:"num_replicas,omitempty"`
	FailureDomain string `json:"failure_domain,omitempty"`
}

// ---- /nisd ----

// GetNisdPayload is the payload of the get-nisd envelope.
type GetNisdPayload struct {
	ID       string        `json:"id,omitempty"`
	PeerPort int           `json:"peer_port,omitempty"`
	NetInfo  []NetworkInfo `json:"net_info,omitempty"`
}

// ---- /get_chunk ----

// GetChunkPayload is the payload of the get-chunk envelope.
type GetChunkPayload struct {
	VdevID   string   `json:"vdev_id,omitempty"`
	ChunkIdx int      `json:"chunk_idx"`
	NisdIDs  []string `json:"nisd_ids,omitempty"`
}

// ---- /api/recovery_assignment ----

// GetRecoveryAssignmentPayload is the payload of the recovery-assignment
// envelope. Not yet backed by real control-plane logic (see
// handleGetRecoveryAssignment, which always reports StatusInternalError) - the
// shape is defined now so the client's REST contract is stable ahead of the
// real implementation.
type GetRecoveryAssignmentPayload struct {
	VdevID        string   `json:"vdev_id,omitempty"`
	ChunkIdx      int      `json:"chunk_idx"`
	SnapshotSeqno int64    `json:"snapshot_seqno,omitempty"`
	TgtNisd       string   `json:"tgt_nisd,omitempty"`
	SrcNisdList   []string `json:"src_nisd_list,omitempty"`
}
