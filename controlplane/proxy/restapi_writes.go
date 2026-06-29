package main

import (
	"encoding/json"
	"net/http"

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
