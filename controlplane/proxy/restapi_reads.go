package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	restapi "github.com/00pauln00/niova-mdsvc/controlplane/restapi"
)

// ---- shared request/error helpers (used by read and, later, write handlers) ----

// requireMethod writes a 405 and returns false if the request method differs
// from the one the endpoint supports.
func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		restapi.WriteError(w, restapi.StatusMethodNotAllowed,
			"method %s not allowed; use %s", r.Method, method)
		return false
	}
	return true
}

// cpErrOf returns the structured CPError from a response, guarding against nil.
func cpErrOf(resp *cpLib.CPResp) *cpLib.CPError {
	if resp == nil {
		return nil
	}
	return resp.Error
}

// statusForCPError maps a structured control-plane error to a Status code.
// CPError only carries a coarse 3-way code (AUTH_ERROR/FUNC_ERROR/INTERNAL)
// plus a free-text message, so FUNC_ERROR collapses to a generic failure
// rather than being classified further by message content.
func statusForCPError(e *cpLib.CPError) restapi.Status {
	switch e.Code {
	case cpLib.ErrAuth:
		return restapi.StatusUnauthorized
	case cpLib.ErrInternal:
		return restapi.StatusInternalError
	case cpLib.ErrFunc:
		return restapi.StatusFailure
	default:
		return restapi.StatusInternalError
	}
}

// writeCPError translates a failed control-plane call into a JSON HTTP error.
// err is the transport error from callFunc (may be nil); cpErr is the structured
// CPResp.Error (may be nil). At least one is expected to be non-nil. Used by the
// user/auth endpoints, which map errors to real HTTP status codes.
func writeCPError(w http.ResponseWriter, err error, cpErr *cpLib.CPError) {
	switch {
	case cpErr != nil:
		restapi.WriteError(w, statusForCPError(cpErr), "%s", cpErr.Message)
	case cpLib.IsKeyNotFoundError(err):
		restapi.WriteError(w, restapi.StatusNotFound, "%s", err.Error())
	case err != nil:
		restapi.WriteError(w, restapi.StatusInternalError, "%s", err.Error())
	default:
		restapi.WriteError(w, restapi.StatusInternalError, "unknown control-plane error")
	}
}

// writeMethodCPError reports a failed control-plane call for a non-user endpoint:
// HTTP 200 with the error in the APIResponse envelope ({status:!=0, error}),
// never an HTTP error code. This is the non-user counterpart to writeCPError.
func writeMethodCPError(w http.ResponseWriter, err error, cpErr *cpLib.CPError) {
	switch {
	case cpErr != nil:
		restapi.WriteMethodError(w, statusForCPError(cpErr), "%s", cpErr.Message)
	case cpLib.IsKeyNotFoundError(err):
		restapi.WriteMethodError(w, restapi.StatusNotFound, "%s", err.Error())
	case err != nil:
		restapi.WriteMethodError(w, restapi.StatusInternalError, "%s", err.Error())
	default:
		restapi.WriteMethodError(w, restapi.StatusInternalError, "unknown control-plane error")
	}
}

// splitCSV splits a comma-separated list into trimmed, non-empty elements.
// Returns nil (not a one-element slice) for an empty input.
func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---- GET /vdev?id= ----

// handleGetVdev returns vdev metadata. Maps to GET_VDEV_INFO (ReadVdevInfo).
func (handler *proxyHandler) handleGetVdev(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		restapi.WriteMethodError(w, restapi.StatusInvalidRequest, "missing required query parameter: id")
		return
	}

	cpReq := cpLib.CPReq{
		Token:   tokenFromRequest(r),
		Payload: cpLib.GetReq{ID: id},
	}
	var vdev cpLib.VdevConfig
	cpResp, err := handler.callFunc(cpLib.GET_VDEV_INFO, cpReq, false, "", 0, &vdev)
	if err != nil || cpErrOf(cpResp) != nil {
		writeMethodCPError(w, err, cpErrOf(cpResp))
		return
	}
	if vdev.ID == "" {
		restapi.WriteMethodError(w, restapi.StatusNotFound, "vdev %s not found", id)
		return
	}

	restapi.WriteData(w, restapi.GetVdevPayload{
		ID:            vdev.ID,
		Name:          vdev.Name,
		Size:          vdev.Size,
		NumChunks:     int(vdev.ChunkCnt),
		NumReplicas:   int(vdev.DataBlkCnt),
		FailureDomain: vdev.FilterType,
	})
}

// ---- GET /nisd?id= ----

// handleGetNisd returns NISD connection info. Maps to GET_NISD (ReadNisdConfig).
func (handler *proxyHandler) handleGetNisd(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		restapi.WriteMethodError(w, restapi.StatusInvalidRequest, "missing required query parameter: id")
		return
	}

	cpReq := cpLib.CPReq{
		Token:   tokenFromRequest(r),
		Payload: cpLib.GetReq{ID: id},
	}
	var nisd cpLib.Nisd
	cpResp, err := handler.callFunc(cpLib.GET_NISD, cpReq, false, "", 0, &nisd)
	if err != nil || cpErrOf(cpResp) != nil {
		writeMethodCPError(w, err, cpErrOf(cpResp))
		return
	}
	if nisd.ID == "" {
		restapi.WriteMethodError(w, restapi.StatusNotFound, "nisd %s not found", id)
		return
	}

	netInfo := make([]restapi.NetworkInfo, 0, len(nisd.NetInfo))
	for _, ni := range nisd.NetInfo {
		netInfo = append(netInfo, restapi.NetworkInfo{IPAddr: ni.IPAddr, Port: int(ni.Port)})
	}
	restapi.WriteData(w, restapi.GetNisdPayload{
		ID:       nisd.ID,
		PeerPort: int(nisd.PeerPort),
		NetInfo:  netInfo,
	})
}

// ---- GET /get_chunk?vdev_id=&chunk_idx= ----

// handleGetChunk returns the NISD placement for a single vdev chunk. Maps to
// GET_CHUNK_NISD (ReadChunkNisd), whose request ID is "vdevID/chunkIdx".
func (handler *proxyHandler) handleGetChunk(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	q := r.URL.Query()
	vdevID := strings.TrimSpace(q.Get("vdev_id"))
	chunkStr := strings.TrimSpace(q.Get("chunk_idx"))
	if vdevID == "" || chunkStr == "" {
		restapi.WriteMethodError(w, restapi.StatusInvalidRequest, "missing required query parameters: vdev_id and chunk_idx")
		return
	}
	chunkIdx, perr := strconv.Atoi(chunkStr)
	if perr != nil || chunkIdx < 0 {
		restapi.WriteMethodError(w, restapi.StatusInvalidRequest, "invalid chunk_idx %q: must be a non-negative integer", chunkStr)
		return
	}

	cpReq := cpLib.CPReq{
		Token:   tokenFromRequest(r),
		Payload: cpLib.GetReq{ID: fmt.Sprintf("%s/%d", vdevID, chunkIdx)},
	}
	var cn cpLib.ChunkNisd
	cpResp, err := handler.callFunc(cpLib.GET_CHUNK_NISD, cpReq, false, "", 0, &cn)
	if err != nil || cpErrOf(cpResp) != nil {
		writeMethodCPError(w, err, cpErrOf(cpResp))
		return
	}

	ids := splitCSV(cn.NisdIDs)
	if len(ids) == 0 {
		restapi.WriteMethodError(w, restapi.StatusNotFound, "chunk %d not found for vdev %s", chunkIdx, vdevID)
		return
	}

	restapi.WriteData(w, restapi.GetChunkPayload{
		VdevID:   vdevID,
		ChunkIdx: chunkIdx,
		NisdIDs:  ids,
	})
}

// ---- GET /api/recovery_assignment?vdev_id=&chunk_idx=&client_snapshot_seqno= ----

// handleGetRecoveryAssignment is a stub: the wire contract (route, request
// params, response envelope/payload shape) is defined ahead of the real
// recovery-assignment control-plane logic, which does not exist yet. Once a
// real implementation lands, this should look up chunk recovery placement
// the same way handleGetChunk does. For now it always reports StatusInternalError
// after validating the request shape, so the client's request/parse path is
// fully exercisable ahead of the real backing logic.
func (handler *proxyHandler) handleGetRecoveryAssignment(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	q := r.URL.Query()
	vdevID := strings.TrimSpace(q.Get("vdev_id"))
	chunkStr := strings.TrimSpace(q.Get("chunk_idx"))
	if vdevID == "" || chunkStr == "" {
		restapi.WriteMethodError(w, restapi.StatusInvalidRequest, "missing required query parameters: vdev_id and chunk_idx")
		return
	}
	if _, perr := strconv.Atoi(chunkStr); perr != nil {
		restapi.WriteMethodError(w, restapi.StatusInvalidRequest, "invalid chunk_idx %q: must be a non-negative integer", chunkStr)
		return
	}
	if strings.TrimSpace(q.Get("client_snapshot_seqno")) == "" {
		restapi.WriteMethodError(w, restapi.StatusInvalidRequest, "missing required query parameter: client_snapshot_seqno")
		return
	}

	restapi.WriteMethodError(w, restapi.StatusInternalError, "recovery assignment not yet implemented")
}
