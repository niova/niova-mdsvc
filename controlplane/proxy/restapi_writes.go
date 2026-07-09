package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

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
		restapi.WriteMethodError(w, restapi.StatusBadRequest, "invalid request body: %v", err)
		return
	}
	if req.SizeBytes < 1 {
		restapi.WriteMethodError(w, restapi.StatusBadRequest, "size_bytes must be >= 1")
		return
	}
	if req.NumReplicas < 1 || req.NumReplicas > maxVdevReplicas {
		restapi.WriteMethodError(w, restapi.StatusBadRequest, "num_replicas must be between 1 and %d", maxVdevReplicas)
		return
	}

	rncui, err := rncuiFromRequest(r)
	if err != nil {
		restapi.WriteMethodError(w, restapi.StatusBadRequest, "%s", err.Error())
		return
	}

	// Optional failure-domain scoping. An empty failure_domain means FD_ANY
	// (allocate across any NISDs). niova's allocator scopes to a single entity,
	// so only the first entity_id is applied.
	fdType, err := cpLib.ParseFD(req.FailureDomain)
	if err != nil {
		restapi.WriteMethodError(w, restapi.StatusBadRequest, "%s", err.Error())
		return
	}
	filter := cpLib.Filter{Type: fdType}
	if len(req.EntityIDs) > 0 {
		filter.ID = req.EntityIDs[0]
	}

	// The server generates the vdev ID and chunk map. Name is a niova-only
	// extension; the server validates it (alphanumeric) and indexes it for
	// name-based lookup.
	vdevCfg := &cpLib.VdevCfg{
		Size:       req.SizeBytes,
		NumReplica: uint8(req.NumReplicas),
		Name:       strings.TrimSpace(req.Name),
	}
	// Optional PFS link, supplied as either the PFS id (UUID) or its name. The
	// server records membership and applies the PFS placement offset; PFSName is
	// resolved to PFSID server-side (APCreateVdev).
	if pfs := strings.TrimSpace(req.PFS); pfs != "" {
		if uuid.Validate(pfs) == nil {
			vdevCfg.PFSID = pfs
		} else {
			vdevCfg.PFSName = pfs
		}
	}
	cpReq := cpLib.CPReq{
		Token:   tokenFromRequest(r),
		Payload: cpLib.VdevReq{Vdev: vdevCfg, Filter: filter},
	}

	var resp cpLib.ResponseXML
	cpResp, err := handler.callFunc(cpLib.CREATE_VDEV, cpReq, true, rncui, 0, &resp)
	if err != nil || cpErrOf(cpResp) != nil {
		writeMethodCPError(w, err, cpErrOf(cpResp))
		return
	}
	if resp.ID == "" {
		restapi.WriteMethodError(w, restapi.StatusInternal, "vdev creation returned no id")
		return
	}

	// Echo the applied request (TiDB parity). failure_domain is the normalized
	// level; pfs_id is populated only when the PFS was supplied as a UUID (a
	// name is resolved server-side and not returned here).
	restapi.WriteData(w, restapi.CreateVdevPayload{
		VdevID:        resp.ID,
		Name:          vdevCfg.Name,
		NumChunks:     int(cpLib.Count8GBChunks(req.SizeBytes)),
		FailureDomain: cpLib.FDName(fdType),
		PFSID:         vdevCfg.PFSID,
	})
}

// ---- shared write helpers ----

// decodeJSONBody decodes the JSON request body into dst, writing a 400 and
// returning false on failure. Used by the user/auth endpoints (real HTTP codes).
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		restapi.WriteError(w, restapi.StatusBadRequest, "invalid request body: %v", err)
		return false
	}
	return true
}

// decodeJSONBodyMethod is the non-user counterpart to decodeJSONBody: on a
// malformed body it writes the error in the envelope with HTTP 200 (never an
// HTTP error code) and returns false.
func decodeJSONBodyMethod(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		restapi.WriteMethodError(w, restapi.StatusBadRequest, "invalid request body: %v", err)
		return false
	}
	return true
}

// runEntityWrite executes a write op whose server reply is a ResponseXML and
// emits a WritePayload envelope. payload is the (value) CPReq payload; fallbackID
// is echoed back when the server does not return an id. Writes require X-RNCUI.
// Non-user endpoint: errors are reported in the envelope with HTTP 200.
func (handler *proxyHandler) runEntityWrite(w http.ResponseWriter, r *http.Request, op string, payload any, fallbackID string) {
	rncui, err := rncuiFromRequest(r)
	if err != nil {
		restapi.WriteMethodError(w, restapi.StatusBadRequest, "%s", err.Error())
		return
	}
	cpReq := cpLib.CPReq{Token: tokenFromRequest(r), Payload: payload}

	var resp cpLib.ResponseXML
	cpResp, err := handler.callFunc(op, cpReq, true, rncui, 0, &resp)
	if err != nil || cpErrOf(cpResp) != nil {
		writeMethodCPError(w, err, cpErrOf(cpResp))
		return
	}

	id := resp.ID
	if id == "" {
		id = fallbackID
	}
	restapi.WriteData(w, restapi.WritePayload{ID: id})
}

// ---- single-entity infra writes ----
//
// The per-entity infra upserts (pdu/rack/hypervisor/device/nisd/partition) are
// served by handlePutResource (PUT /api/resource?type=) in restapi_resource.go.
// Only the writes without a TiDB resource-type equivalent live here: PFS and the
// singleton NISD-args record.

// POST /api/pfs (PUT_PFS)
func (handler *proxyHandler) handlePutPFS(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.PFS
	if !decodeJSONBodyMethod(w, r, &req) {
		return
	}
	// Unlike the id-keyed infra upserts, PFS is a create: WPPFSCfg mints a UUID
	// when none is supplied and returns it. So an empty id is valid here (create);
	// a supplied id upserts a specific PFS. runEntityWrite echoes back whichever
	// id the server returns.
	payload := cpLib.PFS{
		ID:      req.ID,
		Name:    req.Name,
		Offset:  req.Offset,
		VdevIDs: req.VdevIDs,
	}
	handler.runEntityWrite(w, r, cpLib.PUT_PFS, payload, req.ID)
}

// POST /api/nisd_args (PUT_NISD_ARGS) — singleton record, no id.
func (handler *proxyHandler) handlePutNisdArgs(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.NisdArgs
	if !decodeJSONBodyMethod(w, r, &req) {
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

// POST /api/snap (CREATE_SNAP)
func (handler *proxyHandler) handleCreateSnap(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.CreateSnapRequest
	if !decodeJSONBodyMethod(w, r, &req) {
		return
	}
	if req.VdevID == "" || req.SnapName == "" {
		restapi.WriteMethodError(w, restapi.StatusBadRequest, "vdev_id and snap_name are required")
		return
	}
	rncui, err := rncuiFromRequest(r)
	if err != nil {
		restapi.WriteMethodError(w, restapi.StatusBadRequest, "%s", err.Error())
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
		writeMethodCPError(w, err, cpErrOf(cpResp))
		return
	}
	if !resp.SnapName.Success {
		restapi.WriteMethodError(w, restapi.StatusInternal, "snapshot %q was not created", req.SnapName)
		return
	}
	restapi.WriteData(w, restapi.CreateSnapPayload{SnapName: resp.SnapName.Name})
}

// handleDeleteVdev backs DELETE /api/vdev/{id} (DELETE_VDEV), matching the TiDB
// REST contract. The vdev id is taken from the path; the write requires the
// X-RNCUI header like every other write.
func (handler *proxyHandler) handleDeleteVdev(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		restapi.WriteMethodError(w, restapi.StatusBadRequest, "vdev id is required")
		return
	}
	handler.runEntityWrite(w, r, cpLib.DELETE_VDEV, cpLib.DeleteVdevReq{ID: id}, id)
}

// POST /api/mount_vdev (MOUNT_VDEV)
func (handler *proxyHandler) handleMountVdev(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req restapi.MountVdevRequest
	if !decodeJSONBodyMethod(w, r, &req) {
		return
	}
	if req.VdevID == "" {
		restapi.WriteMethodError(w, restapi.StatusBadRequest, "vdev_id is required")
		return
	}
	rncui, err := rncuiFromRequest(r)
	if err != nil {
		restapi.WriteMethodError(w, restapi.StatusBadRequest, "%s", err.Error())
		return
	}

	cpReq := cpLib.CPReq{Token: tokenFromRequest(r), Payload: cpLib.MountVdevRequest{VdevID: req.VdevID}}
	// MOUNT_VDEV (APMountVdev) returns the full VdevCfg: it carries the updated
	// mount info (counter, last-updated) plus the freshly minted AccessToken the
	// data path needs to open the vdev. Project it into the flat snake_case
	// MountVdevPayload so the whole REST surface stays snake_case.
	var resp cpLib.VdevCfg
	cpResp, err := handler.callFunc(cpLib.MOUNT_VDEV, cpReq, true, rncui, 0, &resp)
	if err != nil || cpErrOf(cpResp) != nil {
		writeMethodCPError(w, err, cpErrOf(cpResp))
		return
	}
	restapi.WriteData(w, restapi.MountVdevPayload{
		ID:             resp.ID,
		Name:           resp.Name,
		Size:           resp.Size,
		NumChunks:      int(resp.NumChunks),
		NumReplica:     int(resp.NumReplica),
		MountCounter:   resp.VdevMountInfo.MountCounter,
		LastUpdatedLTS: resp.VdevMountInfo.LastUpdatedLTS,
		PFSID:          resp.PFSID,
		AccessToken:    resp.AccessToken,
		DataBlkCnt:     resp.NumDataBlk,
		ParityBlkCnt:   resp.NumParityBlk,
	})
}
