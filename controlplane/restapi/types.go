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

// CreateInfraResponse is returned for /create_infra.
type CreateInfraResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ---- /create_vdev ----

type CreateVdevRequest struct {
	SizeBytes   int64 `json:"size_bytes"`
	NumReplicas int   `json:"num_replicas"`
}

type CreateVdevResponse struct {
	Success   bool   `json:"success"`
	VdevID    string `json:"vdev_id,omitempty"`
	NumChunks int    `json:"num_chunks,omitempty"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ---- /vdev ----

type GetVdevResponse struct {
	Success     bool   `json:"success"`
	ID          string `json:"id,omitempty"`
	Size        int64  `json:"size,omitempty"`
	NumChunks   int    `json:"num_chunks,omitempty"`
	NumReplicas int    `json:"num_replicas,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ---- /nisd ----

type GetNisdResponse struct {
	Success  bool          `json:"success"`
	ID       string        `json:"id,omitempty"`
	PeerPort int           `json:"peer_port,omitempty"`
	NetInfo  []NetworkInfo `json:"net_info,omitempty"`
	Error    string        `json:"error,omitempty"`
}

// ---- /get_chunk ----

type GetChunkResponse struct {
	Success  bool     `json:"success"`
	VdevID   string   `json:"vdev_id,omitempty"`
	ChunkIdx int      `json:"chunk_idx"`
	NisdIDs  []string `json:"nisd_ids,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// ---- /get_chunks (paginated) ----

type ChunkPlacement struct {
	ChunkIdx int      `json:"chunk_idx"`
	NisdIDs  []string `json:"nisd_ids"`
}

type GetChunksResponse struct {
	Success           bool             `json:"success"`
	VdevID            string           `json:"vdev_id,omitempty"`
	StartChunkIdx     int              `json:"start_chunk_idx"`
	Limit             int              `json:"limit,omitempty"`
	NextStartChunkIdx int              `json:"next_start_chunk_idx"`
	HasMore           bool             `json:"has_more"`
	TotalChunks       int              `json:"total_chunks"`
	Chunks            []ChunkPlacement `json:"chunks,omitempty"`
	Error             string           `json:"error,omitempty"`
}
