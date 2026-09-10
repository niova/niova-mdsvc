package domain

import (
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"
)

// BackendKind identifies which control-plane implementation niova-ctl is
// currently talking to. UI features specific to one backend (e.g. tenants,
// RBAC/ABAC, chunk placement on mdsvc-tidb) are gated on this.
type BackendKind int

const (
	BackendMdsvcPumice BackendKind = iota
	BackendMdsvcTidb
)

// ControlPlaneClient is the surface niova-ctl's UI layer needs from a
// control-plane backend, implemented by both mdsvcpumiceclient and
// mdsvctidbclient. Only methods every backend can honor belong here (LSP) —
// backend-specific capabilities go in PartitionWriter/PFSEditor/VdevDeleter.
type ControlPlaneClient interface {
	SetToken(token string)
	// RefreshTopology fetches and assembles the full PDU/Rack/Hypervisor tree.
	RefreshTopology(token string) (pdus []PDU, racks []Rack, standaloneHVs []Hypervisor, err error)
	GetNisds() ([]Nisd, error)
	GetVdevConfigs() ([]VdevConfig, error)
	GetPFS() ([]PFS, error)
	PutPDU(pdu *PDU) error
	PutRack(rack *Rack) error
	PutHypervisor(hv *Hypervisor) error
	PutDevice(dev *Device) error
	PutNisd(nisd *Nisd) error
	CreateVdev(vdev *VdevConfig) error
	// CreatePFS creates a new PFS. Every backend supports this; only some
	// additionally support PFSEditor.EditPFS.
	CreatePFS(pfs *PFS) error
}

// PartitionWriter is implemented by backends that support writing device
// partitions directly (niova-mdsvc only — mdsvc-tidb has no partition
// endpoint). Callers type-assert for it.
type PartitionWriter interface {
	PutPartition(part *DevicePartition) error
}

// PFSEditor is implemented by control-plane backends that support editing an
// existing PFS (niova-mdsvc only — mdsvc-tidb has no PFS update endpoint).
// Callers must type-assert for PFSEditor before attempting an edit.
type PFSEditor interface {
	EditPFS(pfs *PFS) error
}

// VdevDeleter is implemented by control-plane backends that support deleting
// a vdev pool.
type VdevDeleter interface {
	DeleteVdev(vdevID string) error
}

// UserServiceClient is the surface niova-ctl's UI layer needs from a
// user/auth backend. CreateAdminUser is separate from CreateUser because
// niova-mdsvc can bootstrap the first admin user without a token.
type UserServiceClient interface {
	Login(username, secretKey string) (*userlib.LoginResp, error)
	LoginWithTenant(username, secretKey, tenantUUID string) (*userlib.LoginResp, error)
	ListUsers(token string, req userlib.GetReq) ([]userlib.UserResp, error)
	CreateUser(token string, req *userlib.UserReq) (*userlib.UserResp, error)
	CreateAdminUser(req *userlib.UserReq) (*userlib.UserResp, error)
}
