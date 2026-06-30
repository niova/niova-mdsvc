package restapi

// This file defines the flat snake_case DTOs for the niova-specific write
// endpoints (single-entity infra writes plus the vdev lifecycle operations).
// These endpoints are NOT part of the TiDB REST contract; they expose the
// remaining /func write operations over REST. The PDU/Rack/Hypervisor/Device
// NISD shapes mirror the flat schemas already declared in openapi.yaml.

// WriteResponse is the generic result for writes that return only an id/status
// (the internal ResponseXML). Endpoint-specific responses are defined below.
type WriteResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ---- /pdu (PUT_PDU) ----

type PDU struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	Location      string `json:"location,omitempty"`
	Specification string `json:"specification,omitempty"`
	PowerCap      string `json:"power_cap,omitempty"`
}

// ---- /rack (PUT_RACK) ----

type Rack struct {
	ID            string `json:"id"`
	PDUID         string `json:"pdu_id,omitempty"`
	Name          string `json:"name,omitempty"`
	Location      string `json:"location,omitempty"`
	Specification string `json:"specification,omitempty"`
}

// ---- /hypervisor (PUT_HYPERVISOR) ----

type Hypervisor struct {
	ID          string   `json:"id"`
	RackID      string   `json:"rack_id,omitempty"`
	Name        string   `json:"name,omitempty"`
	PortRange   string   `json:"port_range,omitempty"`
	SSHPort     string   `json:"ssh_port,omitempty"`
	RDMAEnabled bool     `json:"rdma_enabled,omitempty"`
	IPAddrs     []string `json:"ip_addrs,omitempty"`
}

// ---- /device (PUT_DEVICE) ----

type Device struct {
	ID            string `json:"id"`
	HypervisorID  string `json:"hypervisor_id,omitempty"`
	Name          string `json:"name,omitempty"`
	DevicePath    string `json:"device_path,omitempty"`
	SerialNumber  string `json:"serial_number,omitempty"`
	State         int    `json:"state,omitempty"`
	Size          int64  `json:"size,omitempty"`
	FailureDomain string `json:"failure_domain,omitempty"`
}

// ---- /nisd (POST = PUT_NISD) ----

// NISD mirrors the internal Nisd write payload. failure_domain is the ordered
// list [pdu, rack, hv, device, partition] the control plane expects (a single
// flat array rather than openapi's fd_* fields, to round-trip without loss).
type NISD struct {
	ID            string        `json:"id"`
	PeerPort      int           `json:"peer_port,omitempty"`
	FailureDomain []string      `json:"failure_domain,omitempty"`
	TotalSize     int64         `json:"total_size,omitempty"`
	AvailableSize int64         `json:"available_size,omitempty"`
	SocketPath    string        `json:"socket_path,omitempty"`
	NetInfo       []NetworkInfo `json:"net_info,omitempty"`
}

// ---- /pfs (PUT_PFS) ----

type PFS struct {
	ID      string   `json:"id"`
	Name    string   `json:"name,omitempty"`
	Offset  int      `json:"offset,omitempty"`
	VdevIDs []string `json:"vdev_ids,omitempty"`
}

// ---- /partition (PUT_PARTITION) ----

type Partition struct {
	PartitionID   string `json:"partition_id"`
	PartitionPath string `json:"partition_path,omitempty"`
	NISDUUID      string `json:"nisd_uuid,omitempty"`
	DevID         string `json:"dev_id,omitempty"`
	Size          int64  `json:"size,omitempty"`
}

// ---- /nisd_args (PUT_NISD_ARGS) ----

// NisdArgs is the (singleton) NISD launch-arguments record.
type NisdArgs struct {
	Defrag               bool   `json:"defrag,omitempty"`
	AllowDefragMCIBCache bool   `json:"allow_defrag_mcib_cache,omitempty"`
	MBCCnt               int    `json:"mbc_cnt,omitempty"`
	MergeHCnt            int    `json:"merge_h_cnt,omitempty"`
	MCIBReadCache        int    `json:"mcib_read_cache,omitempty"`
	S3                   string `json:"s3,omitempty"`
	DSync                string `json:"dsync,omitempty"`
}

// ---- /snap (CREATE_SNAP) ----

type CreateSnapRequest struct {
	VdevID   string   `json:"vdev_id"`
	SnapName string   `json:"snap_name"`
	ChunkSeq []uint64 `json:"chunk_seq,omitempty"`
}

type CreateSnapResponse struct {
	Success  bool   `json:"success"`
	SnapName string `json:"snap_name,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ---- /delete_vdev (DELETE_VDEV) ----

type DeleteVdevRequest struct {
	VdevID string `json:"vdev_id"`
}

// ---- /mount_vdev (MOUNT_VDEV) ----

type MountVdevRequest struct {
	VdevID string `json:"vdev_id"`
}

type MountVdevResponse struct {
	Success        bool   `json:"success"`
	MountCounter   uint64 `json:"mount_counter"`
	LastUpdatedLTS string `json:"last_updated_lts,omitempty"`
	Error          string `json:"error,omitempty"`
}
