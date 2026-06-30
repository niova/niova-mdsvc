package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	restapi "github.com/00pauln00/niova-mdsvc/controlplane/restapi"
)

// maxVdevReplicas bounds num_replicas to what the domain (uint8) can hold.
const maxVdevReplicas = 255

// ---- POST /create_vdev ----

// handleCreateVdev creates a vdev and allocates its chunks. Maps to CREATE_VDEV
// (WPCreateVdev/APCreateVdev). It is a write, so it requires a client-supplied
// RNCUI via the X-RNCUI header (see rncuiFromRequest).
func (handler *proxyHandler) handleCreateVdev(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req restapi.CreateVdevRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.WriteError(w, http.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	if req.SizeBytes < 1 {
		restapi.WriteError(w, http.StatusBadRequest, "size_bytes must be >= 1")
		return
	}
	if req.NumReplicas < 1 || req.NumReplicas > maxVdevReplicas {
		restapi.WriteError(w, http.StatusBadRequest,
			"num_replicas must be between 1 and %d", maxVdevReplicas)
		return
	}

	rncui, err := rncuiFromRequest(r)
	if err != nil {
		restapi.WriteError(w, http.StatusBadRequest, "%s", err.Error())
		return
	}

	// No failure-domain scoping or PFS in the REST contract: empty Filter means
	// allocate across any NISDs. The server generates the vdev ID and chunk map.
	cpReq := cpLib.CPReq{
		Token: tokenFromRequest(r),
		Payload: cpLib.VdevReq{
			Vdev: &cpLib.VdevCfg{
				Size:       req.SizeBytes,
				NumReplica: uint8(req.NumReplicas),
			},
			Filter: cpLib.Filter{},
		},
	}

	var resp cpLib.ResponseXML
	cpResp, err := handler.callFunc(cpLib.CREATE_VDEV, cpReq, true, rncui, 0, &resp)
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}
	if resp.ID == "" {
		restapi.WriteError(w, http.StatusInternalServerError, "vdev creation returned no id")
		return
	}

	restapi.WriteJSON(w, http.StatusOK, restapi.CreateVdevResponse{
		Success:   true,
		VdevID:    resp.ID,
		NumChunks: int(cpLib.Count8GBChunks(req.SizeBytes)),
	})
}

// ---- shared write helpers ----

// decodeJSONBody decodes the JSON request body into dst, writing a 400 and
// returning false on failure.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		restapi.WriteError(w, http.StatusBadRequest, "invalid request body: %v", err)
		return false
	}
	return true
}

// runEntityWrite executes a write op whose server reply is a ResponseXML and
// emits a generic WriteResponse. payload is the (value) CPReq payload; fallbackID
// is echoed back when the server does not return an id. Writes require X-RNCUI.
func (handler *proxyHandler) runEntityWrite(w http.ResponseWriter, r *http.Request, op string, payload any, fallbackID string) {
	rncui, err := rncuiFromRequest(r)
	if err != nil {
		restapi.WriteError(w, http.StatusBadRequest, "%s", err.Error())
		return
	}
	cpReq := cpLib.CPReq{Token: tokenFromRequest(r), Payload: payload}

	var resp cpLib.ResponseXML
	cpResp, err := handler.callFunc(op, cpReq, true, rncui, 0, &resp)
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}

	id := resp.ID
	if id == "" {
		id = fallbackID
	}
	restapi.WriteJSON(w, http.StatusOK, restapi.WriteResponse{Success: true, ID: id})
}

// ---- single-entity infra writes ----

// POST /pdu (PUT_PDU)
func (handler *proxyHandler) handlePutPDU(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.PDU
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.ID == "" {
		restapi.WriteError(w, http.StatusBadRequest, "id is required")
		return
	}
	payload := cpLib.PDU{
		ID:            req.ID,
		Name:          req.Name,
		Location:      req.Location,
		Specification: req.Specification,
		PowerCapacity: req.PowerCap,
	}
	handler.runEntityWrite(w, r, cpLib.PUT_PDU, payload, req.ID)
}

// POST /rack (PUT_RACK)
func (handler *proxyHandler) handlePutRack(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.Rack
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.ID == "" {
		restapi.WriteError(w, http.StatusBadRequest, "id is required")
		return
	}
	payload := cpLib.Rack{
		ID:            req.ID,
		PDUID:         req.PDUID,
		Name:          req.Name,
		Location:      req.Location,
		Specification: req.Specification,
	}
	handler.runEntityWrite(w, r, cpLib.PUT_RACK, payload, req.ID)
}

// POST /hypervisor (PUT_HYPERVISOR)
func (handler *proxyHandler) handlePutHypervisor(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.Hypervisor
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.ID == "" {
		restapi.WriteError(w, http.StatusBadRequest, "id is required")
		return
	}
	payload := cpLib.Hypervisor{
		ID:          req.ID,
		RackID:      req.RackID,
		Name:        req.Name,
		PortRange:   req.PortRange,
		SSHPort:     req.SSHPort,
		RDMAEnabled: req.RDMAEnabled,
		IPAddrs:     req.IPAddrs,
	}
	handler.runEntityWrite(w, r, cpLib.PUT_HYPERVISOR, payload, req.ID)
}

// POST /device (PUT_DEVICE)
func (handler *proxyHandler) handlePutDevice(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.Device
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.ID == "" {
		restapi.WriteError(w, http.StatusBadRequest, "id is required")
		return
	}
	payload := cpLib.Device{
		ID:            req.ID,
		HypervisorID:  req.HypervisorID,
		Name:          req.Name,
		DevicePath:    req.DevicePath,
		SerialNumber:  req.SerialNumber,
		State:         uint16(req.State),
		Size:          req.Size,
		FailureDomain: req.FailureDomain,
	}
	handler.runEntityWrite(w, r, cpLib.PUT_DEVICE, payload, req.ID)
}

// handlePutNisd backs POST /nisd (PUT_NISD).
func (handler *proxyHandler) handlePutNisd(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.NISD
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.ID == "" {
		restapi.WriteError(w, http.StatusBadRequest, "id is required")
		return
	}
	nisd := cpLib.Nisd{
		ID:            req.ID,
		PeerPort:      uint16(req.PeerPort),
		FailureDomain: req.FailureDomain,
		TotalSize:     req.TotalSize,
		AvailableSize: req.AvailableSize,
		SocketPath:    req.SocketPath,
	}
	for _, ni := range req.NetInfo {
		nisd.NetInfo = append(nisd.NetInfo, cpLib.NetworkInfo{IPAddr: ni.IPAddr, Port: uint16(ni.Port)})
	}
	nisd.NetInfoCnt = len(nisd.NetInfo)
	handler.runEntityWrite(w, r, cpLib.PUT_NISD, nisd, req.ID)
}

// POST /pfs (PUT_PFS)
func (handler *proxyHandler) handlePutPFS(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.PFS
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.ID == "" {
		restapi.WriteError(w, http.StatusBadRequest, "id is required")
		return
	}
	payload := cpLib.PFS{
		ID:      req.ID,
		Name:    req.Name,
		Offset:  req.Offset,
		VdevIDs: req.VdevIDs,
	}
	handler.runEntityWrite(w, r, cpLib.PUT_PFS, payload, req.ID)
}

// POST /partition (PUT_PARTITION)
func (handler *proxyHandler) handlePutPartition(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.Partition
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.PartitionID == "" {
		restapi.WriteError(w, http.StatusBadRequest, "partition_id is required")
		return
	}
	payload := cpLib.DevicePartition{
		PartitionID:   req.PartitionID,
		PartitionPath: req.PartitionPath,
		NISDUUID:      req.NISDUUID,
		DevID:         req.DevID,
		Size:          req.Size,
	}
	handler.runEntityWrite(w, r, cpLib.PUT_PARTITION, payload, req.PartitionID)
}

// POST /nisd_args (PUT_NISD_ARGS) — singleton record, no id.
func (handler *proxyHandler) handlePutNisdArgs(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.NisdArgs
	if !decodeJSONBody(w, r, &req) {
		return
	}
	payload := cpLib.NisdArgs{
		Defrag:               req.Defrag,
		AllowDefragMCIBCache: req.AllowDefragMCIBCache,
		MBCCnt:               req.MBCCnt,
		MergeHCnt:            req.MergeHCnt,
		MCIBReadCache:        req.MCIBReadCache,
		S3:                   req.S3,
		DSync:                req.DSync,
	}
	handler.runEntityWrite(w, r, cpLib.PUT_NISD_ARGS, payload, "")
}

// ---- vdev lifecycle writes ----

// POST /snap (CREATE_SNAP)
func (handler *proxyHandler) handleCreateSnap(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.CreateSnapRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.VdevID == "" || req.SnapName == "" {
		restapi.WriteError(w, http.StatusBadRequest, "vdev_id and snap_name are required")
		return
	}
	rncui, err := rncuiFromRequest(r)
	if err != nil {
		restapi.WriteError(w, http.StatusBadRequest, "%s", err.Error())
		return
	}

	chunks := make([]cpLib.ChunkXML, 0, len(req.ChunkSeq))
	for idx, seq := range req.ChunkSeq {
		chunks = append(chunks, cpLib.ChunkXML{Idx: uint32(idx), Seq: seq})
	}
	payload := cpLib.SnapXML{SnapName: req.SnapName, Vdev: req.VdevID, Chunks: chunks}

	cpReq := cpLib.CPReq{Token: tokenFromRequest(r), Payload: payload}
	var resp cpLib.SnapResponseXML
	cpResp, err := handler.callFunc(cpLib.CREATE_SNAP, cpReq, true, rncui, 0, &resp)
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}
	if !resp.SnapName.Success {
		restapi.WriteError(w, http.StatusInternalServerError, "snapshot %q was not created", req.SnapName)
		return
	}
	restapi.WriteJSON(w, http.StatusOK, restapi.CreateSnapResponse{
		Success:  true,
		SnapName: resp.SnapName.Name,
	})
}

// handleDeleteVdev backs DELETE /api/vdev/{id} (DELETE_VDEV), matching the TiDB
// REST contract. The vdev id is taken from the path; the write requires the
// X-RNCUI header like every other write.
func (handler *proxyHandler) handleDeleteVdev(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		restapi.WriteError(w, http.StatusBadRequest, "vdev id is required")
		return
	}
	handler.runEntityWrite(w, r, cpLib.DELETE_VDEV, cpLib.DeleteVdevReq{ID: id}, id)
}

// POST /mount_vdev (MOUNT_VDEV)
func (handler *proxyHandler) handleMountVdev(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.MountVdevRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.VdevID == "" {
		restapi.WriteError(w, http.StatusBadRequest, "vdev_id is required")
		return
	}
	rncui, err := rncuiFromRequest(r)
	if err != nil {
		restapi.WriteError(w, http.StatusBadRequest, "%s", err.Error())
		return
	}

	cpReq := cpLib.CPReq{Token: tokenFromRequest(r), Payload: cpLib.MountVdevRequest{VdevID: req.VdevID}}
	var resp cpLib.VdevMountInfo
	cpResp, err := handler.callFunc(cpLib.MOUNT_VDEV, cpReq, true, rncui, 0, &resp)
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}
	out := restapi.MountVdevResponse{Success: true, MountCounter: resp.MountCounter}
	if !resp.LastUpdatedLTS.IsZero() {
		out.LastUpdatedLTS = resp.LastUpdatedLTS.Format(time.RFC3339)
	}
	restapi.WriteJSON(w, http.StatusOK, out)
}

// ---- method dispatch for paths that serve both reads and writes ----

// handleNisd routes /nisd by method: GET reads, POST writes.
func (handler *proxyHandler) handleNisd(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handler.handleGetNisd(w, r)
	case http.MethodPost:
		handler.handlePutNisd(w, r)
	default:
		restapi.WriteError(w, http.StatusMethodNotAllowed,
			"method %s not allowed; use GET or POST", r.Method)
	}
}
