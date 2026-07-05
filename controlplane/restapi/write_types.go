package restapi

import "time"

// This file defines the flat snake_case DTOs for the niova-specific write
// endpoints (single-entity infra writes plus the vdev lifecycle operations).
// These endpoints are NOT part of the TiDB REST contract; they expose the
// remaining /func write operations over REST. The PDU/Rack/Hypervisor/Device
// NISD shapes mirror the flat schemas already declared in openapi.yaml.

// WritePayload is the generic payload for writes that return only an id/status
// (the internal ResponseXML), carried in APIResponse[WritePayload].
// Endpoint-specific payloads are defined below.
type WritePayload struct {
	ID      string `json:"id,omitempty"`
	Message string `json:"message,omitempty"`
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

// CreateSnapPayload is the payload of the create-snap envelope.
type CreateSnapPayload struct {
	SnapName string `json:"snap_name,omitempty"`
}

// ---- DELETE /api/vdev/{id} (DELETE_VDEV) ----
// The vdev id travels in the URL path, so no request body type is needed.

// ---- /mount_vdev (MOUNT_VDEV) ----

type MountVdevRequest struct {
	VdevID string `json:"vdev_id"`
}

// MountVdevPayload is the payload of the mount-vdev envelope: the post-mount
// vdev state (updated mount info + the freshly minted access token). It is the
// flat snake_case projection of the internal cpLib.VdevCfg the server returns.
type MountVdevPayload struct {
	ID             string    `json:"id"`
	Name           string    `json:"name,omitempty"`
	Size           int64     `json:"size,omitempty"`
	NumChunks      int       `json:"num_chunks,omitempty"`
	NumReplica     int       `json:"num_replica,omitempty"`
	MountCounter   uint64    `json:"mount_counter"`
	LastUpdatedLTS time.Time `json:"last_updated_lts"`
	PFSID          string    `json:"pfs_id,omitempty"`
	AccessToken    string    `json:"access_token,omitempty"`
}
