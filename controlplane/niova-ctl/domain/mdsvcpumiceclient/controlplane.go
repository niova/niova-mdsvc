// Package mdsvcpumiceclient adapts niova-mdsvc's raft/gossip clients to
// domain's backend-neutral interfaces — the niova-mdsvc counterpart to
// mdsvctidbclient. Both are peer packages depending inward on domain, so
// domain stays free of either backend's concrete dependencies.
package mdsvcpumiceclient

import (
	"strings"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	ctlplcl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/client"
	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"

	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
)

// Capabilities controlPlaneClient supports, asserted here so drift fails the
// build instead of surfacing as a silent capability-check failure at
// runtime in app.go (mirrors mdsvctidbclient/probe.go's equivalent block).
var (
	_ domain.ControlPlaneClient = (*controlPlaneClient)(nil)
	_ domain.PartitionWriter    = (*controlPlaneClient)(nil)
	_ domain.PFSEditor          = (*controlPlaneClient)(nil)
	_ domain.VdevDeleter        = (*controlPlaneClient)(nil)
)

// controlPlaneClient adapts *ctlplcl.CliCFuncs to domain.ControlPlaneClient.
type controlPlaneClient struct {
	cp *ctlplcl.CliCFuncs
}

// NewControlPlaneClient wraps an existing raft/gossip control-plane client
// so it satisfies domain.ControlPlaneClient.
func NewControlPlaneClient(cp *ctlplcl.CliCFuncs) domain.ControlPlaneClient {
	if cp == nil {
		return nil
	}
	return &controlPlaneClient{cp: cp}
}

func (l *controlPlaneClient) SetToken(token string) { l.cp.SetToken(token) }

func (l *controlPlaneClient) RefreshTopology(token string) ([]domain.PDU, []domain.Rack, []domain.Hypervisor, error) {
	pdus, racks, hvs := RefreshCPData(l.cp, token)
	return pdus, racks, hvs, nil
}

func (l *controlPlaneClient) GetNisds() ([]domain.Nisd, error) {
	return l.cp.GetNisds(ctlplfl.GetReq{GetAll: true})
}

func (l *controlPlaneClient) GetVdevConfigs() ([]domain.VdevConfig, error) {
	return l.cp.GetVdevConfigs(&ctlplfl.GetReq{GetAll: true})
}

func (l *controlPlaneClient) GetPFS() ([]domain.PFS, error) {
	return l.cp.GetPFS(&ctlplfl.GetReq{GetAll: true})
}

func (l *controlPlaneClient) PutPDU(pdu *domain.PDU) error {
	_, err := l.cp.PutPDU(pdu)
	return err
}

func (l *controlPlaneClient) PutRack(rack *domain.Rack) error {
	_, err := l.cp.PutRack(rack)
	return err
}

func (l *controlPlaneClient) PutHypervisor(hv *domain.Hypervisor) error {
	_, err := l.cp.PutHypervisor(hv)
	return err
}

func (l *controlPlaneClient) PutDevice(dev *domain.Device) error {
	_, err := l.cp.PutDevice(dev)
	return err
}

// PutPartition satisfies domain.PartitionWriter — niova-mdsvc supports
// writing partitions directly, unlike mdsvc-tidb.
func (l *controlPlaneClient) PutPartition(part *domain.DevicePartition) error {
	_, err := l.cp.PutPartition(part)
	return err
}

func (l *controlPlaneClient) PutNisd(nisd *domain.Nisd) error {
	_, err := l.cp.PutNisd(nisd)
	return err
}

func (l *controlPlaneClient) CreateVdev(vdev *domain.VdevConfig) error {
	_, err := l.cp.CreateVdev(&ctlplfl.VdevReq{Vdev: vdev})
	return err
}

func (l *controlPlaneClient) CreatePFS(pfs *domain.PFS) error {
	_, err := l.cp.PutPFS(pfs)
	return err
}

// EditPFS satisfies domain.PFSEditor — niova-mdsvc's PutPFS is an upsert
// keyed by ID (matching every other Put* method here), so editing is the
// same call as creating; only mdsvc-tidb needs to distinguish the two.
func (l *controlPlaneClient) EditPFS(pfs *domain.PFS) error {
	_, err := l.cp.PutPFS(pfs)
	return err
}

// DeleteVdev satisfies domain.VdevDeleter.
func (l *controlPlaneClient) DeleteVdev(vdevID string) error {
	_, err := l.cp.DeleteVdev(&ctlplfl.DeleteVdevReq{ID: vdevID})
	return err
}

// InitControlPlane initializes the raft/gossip control plane client.
func InitControlPlane(raftUUID, gossipPath, logFile string) *ctlplcl.CliCFuncs {
	log.Info("InitControlPlane")
	if raftUUID == "" || gossipPath == "" {
		log.Warn("Control plane enabled but missing raft UUID or gossip path")
		return nil
	}

	log.Info("Initializing control plane client, raft uuid: ", raftUUID)
	appUUID := uuid.New().String()

	cpClient := ctlplcl.InitCliCFuncs(appUUID, raftUUID, gossipPath, logFile)
	log.Info("Control plane client initialized with UUID: ", appUUID)
	return cpClient
}

// ProbeServerAuth tests whether the raft/gossip server requires authentication.
func ProbeServerAuth(cpClient *ctlplcl.CliCFuncs) bool {
	if cpClient == nil {
		return false
	}
	_, err := cpClient.GetNisds(ctlplfl.GetReq{ID: uuid.NewString()})
	if err == nil {
		log.Info("Auth probe: server has authentication disabled")
		return false
	}
	if strings.Contains(err.Error(), "Failed to lookup for key") {
		log.Info("Auth probe: server authentication disabled (no key lookup)")
		return false
	}
	log.Info("Auth probe: server authentication enabled (", err, ")")
	return true
}

// RefreshCPData fetches flat lists from the control plane and stitches
// hierarchical trees via domain.StitchTopology — the same pure assembly
// logic mdsvc-tidb's own topology fetch (mdsvctidbclient/infra.go) reuses.
func RefreshCPData(cpClient *ctlplcl.CliCFuncs, token string) ([]domain.PDU, []domain.Rack, []domain.Hypervisor) {
	if cpClient == nil {
		return nil, nil, nil
	}
	cpClient.SetToken(token)

	pdus, err := cpClient.GetPDUs(&ctlplfl.GetReq{GetAll: true})
	if err != nil {
		log.Error("RefreshCPData: GetPDUs failed: ", err)
		pdus = []domain.PDU{}
	}

	racks, err := cpClient.GetRacks(&ctlplfl.GetReq{GetAll: true})
	if err != nil {
		log.Error("RefreshCPData: GetRacks failed: ", err)
		racks = []domain.Rack{}
	}

	hvs, err := cpClient.GetHypervisor(&ctlplfl.GetReq{GetAll: true})
	if err != nil {
		log.Error("RefreshCPData: GetHypervisor failed: ", err)
		hvs = []domain.Hypervisor{}
	}

	devices, err := cpClient.GetDevices(ctlplfl.GetReq{GetAll: true})
	if err != nil {
		log.Error("RefreshCPData: GetDevices failed: ", err)
		devices = []domain.Device{}
	}

	partitions, err := cpClient.GetPartition(ctlplfl.GetReq{GetAll: true})
	if err != nil {
		log.Error("RefreshCPData: GetPartition failed: ", err)
		partitions = []domain.DevicePartition{}
	}

	return domain.StitchTopology(pdus, racks, hvs, devices, partitions)
}
