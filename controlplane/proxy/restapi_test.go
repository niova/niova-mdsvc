package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	restapi "github.com/00pauln00/niova-mdsvc/controlplane/restapi"
)

func init() { cpLib.RegisterGOBStructs() }

// errString is a trivial error used to drive server-error reply fakes.
type errString string

func (e errString) Error() string { return string(e) }

// gobReply encodes a successful CPResp payload exactly as the server's
// EncodeResponse does, for use as a fake transport reply.
func gobReply(t *testing.T, payload any) []byte {
	t.Helper()
	enc, err := cpLib.EncodeResponse(payload)
	if err != nil {
		t.Fatalf("EncodeResponse(%T): %v", payload, err)
	}
	return enc.([]byte)
}

// gobFuncErr encodes a CPResp carrying an ErrFunc (as ReadXxx handlers do on
// business-logic failures).
func gobFuncErr(t *testing.T, msg string) []byte {
	t.Helper()
	enc, err := cpLib.FuncError(errString(msg))
	if err != nil {
		t.Fatalf("FuncError: %v", err)
	}
	return enc.([]byte)
}

// gobAuthErr encodes a CPResp carrying an ErrAuth.
func gobAuthErr(t *testing.T, msg string) []byte {
	t.Helper()
	enc, err := cpLib.AuthError(errString(msg))
	if err != nil {
		t.Fatalf("AuthError: %v", err)
	}
	return enc.([]byte)
}

// capturedCall records the arguments the handler passed to the transport.
type capturedCall struct {
	name    string
	cpReq   cpLib.CPReq
	isWrite bool
	rncui   string
	wsn     int64
}

// newFakeHandler builds a proxyHandler whose transport returns (reply, sendErr)
// instead of hitting PMDB, recording the call into captured (if non-nil).
func newFakeHandler(reply []byte, sendErr error, captured *capturedCall) *proxyHandler {
	return &proxyHandler{
		sendFuncReqFn: func(name string, cpReq cpLib.CPReq, isWrite bool, rncui string, wsn int64) ([]byte, error) {
			if captured != nil {
				*captured = capturedCall{name: name, cpReq: cpReq, isWrite: isWrite, rncui: rncui, wsn: wsn}
			}
			return reply, sendErr
		},
	}
}

// decodeBody unmarshals a JSON response body into v.
func decodeBody(t *testing.T, rr *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rr.Body.Bytes(), v); err != nil {
		t.Fatalf("unmarshal response %q: %v", rr.Body.String(), err)
	}
}

// ---- GET /vdev ----

func TestHandleGetVdev_Success(t *testing.T) {
	reply := gobReply(t, cpLib.VdevCfg{ID: "vdev-1", Size: 16 * cpLib.CHUNK_SIZE, NumChunks: 2, NumReplica: 3})
	h := newFakeHandler(reply, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/vdev?id=vdev-1", nil)
	h.handleGetVdev(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var got restapi.GetVdevResponse
	decodeBody(t, rr, &got)
	if !got.Success || got.ID != "vdev-1" || got.Size != 16*cpLib.CHUNK_SIZE || got.NumChunks != 2 || got.NumReplicas != 3 {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestHandleGetVdev_MissingID(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/vdev", nil)
	h.handleGetVdev(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleGetVdev_NotFound_EmptyID(t *testing.T) {
	// Server returns a zero-value VdevCfg (no such vdev) -> 404.
	h := newFakeHandler(gobReply(t, cpLib.VdevCfg{}), nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/vdev?id=missing", nil)
	h.handleGetVdev(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleGetVdev_MethodNotAllowed(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/vdev?id=v", nil)
	h.handleGetVdev(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rr.Code)
	}
}

func TestHandleGetVdev_ServerFuncError_NotFound(t *testing.T) {
	h := newFakeHandler(gobFuncErr(t, "vdev not found"), nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/vdev?id=v", nil)
	h.handleGetVdev(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleGetVdev_ServerAuthError(t *testing.T) {
	h := newFakeHandler(gobAuthErr(t, "Invalid Token"), nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/vdev?id=v", nil)
	h.handleGetVdev(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rr.Code, rr.Body.String())
	}
}

// ---- GET /nisd ----

func TestHandleGetNisd_Success(t *testing.T) {
	nisd := cpLib.Nisd{
		ID:       "nisd-1",
		PeerPort: 6000,
		NetInfo:  cpLib.NetInfoList{{IPAddr: "10.0.0.1", Port: 7000}, {IPAddr: "10.0.0.2", Port: 7001}},
	}
	h := newFakeHandler(gobReply(t, nisd), nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nisd?id=nisd-1", nil)
	h.handleGetNisd(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var got restapi.GetNisdResponse
	decodeBody(t, rr, &got)
	if !got.Success || got.ID != "nisd-1" || got.PeerPort != 6000 || len(got.NetInfo) != 2 {
		t.Fatalf("unexpected response: %+v", got)
	}
	if got.NetInfo[0].IPAddr != "10.0.0.1" || got.NetInfo[0].Port != 7000 {
		t.Fatalf("net_info[0] = %+v", got.NetInfo[0])
	}
}

func TestHandleGetNisd_MissingID(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nisd", nil)
	h.handleGetNisd(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleGetNisd_NotFound(t *testing.T) {
	h := newFakeHandler(gobReply(t, cpLib.Nisd{}), nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nisd?id=missing", nil)
	h.handleGetNisd(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

// ---- GET /get_chunk ----

func TestHandleGetChunk_Success(t *testing.T) {
	cn := cpLib.ChunkNisd{NumReplicas: 2, NisdUUIDs: "nisd-a, nisd-b"}
	var cap capturedCall
	h := newFakeHandler(gobReply(t, cn), nil, &cap)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/get_chunk?vdev_id=v1&chunk_idx=3", nil)
	h.handleGetChunk(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var got restapi.GetChunkResponse
	decodeBody(t, rr, &got)
	if !got.Success || got.VdevID != "v1" || got.ChunkIdx != 3 {
		t.Fatalf("unexpected response: %+v", got)
	}
	if len(got.NisdIDs) != 2 || got.NisdIDs[0] != "nisd-a" || got.NisdIDs[1] != "nisd-b" {
		t.Fatalf("nisd_ids = %v", got.NisdIDs)
	}
	// The request ID forwarded to the server must be "vdevID/chunkIdx".
	gr, ok := cap.cpReq.Payload.(cpLib.GetReq)
	if !ok || gr.ID != "v1/3" {
		t.Fatalf("forwarded payload = %#v (want GetReq{ID:\"v1/3\"})", cap.cpReq.Payload)
	}
}

func TestHandleGetChunk_MissingParams(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/get_chunk?vdev_id=v1", nil)
	h.handleGetChunk(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleGetChunk_InvalidChunkIdx(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/get_chunk?vdev_id=v1&chunk_idx=-1", nil)
	h.handleGetChunk(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleGetChunk_NotFound_Empty(t *testing.T) {
	h := newFakeHandler(gobReply(t, cpLib.ChunkNisd{NisdUUIDs: ""}), nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/get_chunk?vdev_id=v1&chunk_idx=0", nil)
	h.handleGetChunk(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}

// ---- POST /create_vdev ----

func createVdevReq(t *testing.T, body string, rncui string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/vdev", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if rncui != "" {
		req.Header.Set(rncuiHeader, rncui)
	}
	return req
}

func TestHandleCreateVdev_Success(t *testing.T) {
	size := int64(3 * cpLib.CHUNK_SIZE)
	var cap capturedCall
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: "vdev-new", Success: true}), nil, &cap)
	rr := httptest.NewRecorder()
	req := createVdevReq(t, `{"size_bytes":25769803776,"num_replicas":3}`, "client-rncui-1")
	h.handleCreateVdev(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var got restapi.CreateVdevResponse
	decodeBody(t, rr, &got)
	wantChunks := int(cpLib.Count8GBChunks(size))
	if !got.Success || got.VdevID != "vdev-new" || got.NumChunks != wantChunks {
		t.Fatalf("unexpected response: %+v (wantChunks=%d)", got, wantChunks)
	}
	// Write contract: must be a write, carrying the client-supplied RNCUI.
	if !cap.isWrite {
		t.Errorf("expected isWrite=true")
	}
	if cap.rncui != "client-rncui-1" {
		t.Errorf("rncui = %q, want client-rncui-1", cap.rncui)
	}
	if cap.name != cpLib.CREATE_VDEV {
		t.Errorf("op = %q, want %q", cap.name, cpLib.CREATE_VDEV)
	}
}

func TestHandleCreateVdev_MissingRNCUI(t *testing.T) {
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: "x"}), nil, nil)
	rr := httptest.NewRecorder()
	req := createVdevReq(t, `{"size_bytes":25769803776,"num_replicas":3}`, "")
	h.handleCreateVdev(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreateVdev_BadSize(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := createVdevReq(t, `{"size_bytes":0,"num_replicas":3}`, "r1")
	h.handleCreateVdev(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCreateVdev_BadReplicas(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := createVdevReq(t, `{"size_bytes":25769803776,"num_replicas":0}`, "r1")
	h.handleCreateVdev(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCreateVdev_InvalidJSON(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := createVdevReq(t, `{not json`, "r1")
	h.handleCreateVdev(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleCreateVdev_NoIDReturned(t *testing.T) {
	// Server reports success-shaped reply but with no ID -> 500.
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: ""}), nil, nil)
	rr := httptest.NewRecorder()
	req := createVdevReq(t, `{"size_bytes":25769803776,"num_replicas":3}`, "r1")
	h.handleCreateVdev(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreateVdev_MethodNotAllowed(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/vdev", nil)
	h.handleCreateVdev(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rr.Code)
	}
}

// restRoutes must register exactly the legacy exact-path write endpoints.
func TestRestRoutesRegistered(t *testing.T) {
	h := &proxyHandler{}
	routes := h.restRoutes()
	for _, p := range []string{
		"/nisd",
		"/pdu", "/rack", "/hypervisor", "/device", "/pfs", "/partition", "/nisd_args",
		"/snap", "/mount_vdev",
	} {
		if routes[p] == nil {
			t.Errorf("route %s not registered", p)
		}
	}
}

// restMux must register the TiDB-style /api/ routes with method matching.
func TestRestMuxRegistered(t *testing.T) {
	h := &proxyHandler{}
	mux := h.restMux()
	for _, c := range []struct{ method, target string }{
		{http.MethodPost, "/api/vdev"},
		{http.MethodGet, "/api/vdev"},
		{http.MethodDelete, "/api/vdev/some-id"},
		{http.MethodGet, "/api/nisd"},
		{http.MethodGet, "/api/chunk"},
	} {
		req := httptest.NewRequest(c.method, c.target, nil)
		if _, pattern := mux.Handler(req); pattern == "" {
			t.Errorf("%s %s not registered", c.method, c.target)
		}
	}
}

// ---- single-entity + lifecycle writes ----

// writeReq builds a request with an optional JSON body and X-RNCUI header.
func writeReq(t *testing.T, method, target, body, rncui string) *http.Request {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if rncui != "" {
		r.Header.Set(rncuiHeader, rncui)
	}
	return r
}

func TestHandlePutPDU_Success(t *testing.T) {
	var cap capturedCall
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: "pdu-1", Success: true}), nil, &cap)
	rr := httptest.NewRecorder()
	h.handlePutPDU(rr, writeReq(t, http.MethodPost, "/pdu",
		`{"id":"pdu-1","name":"p","power_cap":"5kW"}`, "r1"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var got restapi.WriteResponse
	decodeBody(t, rr, &got)
	if !got.Success || got.ID != "pdu-1" {
		t.Fatalf("response = %+v", got)
	}
	if !cap.isWrite || cap.rncui != "r1" || cap.name != cpLib.PUT_PDU {
		t.Fatalf("captured = %+v", cap)
	}
	pdu, ok := cap.cpReq.Payload.(cpLib.PDU)
	if !ok || pdu.ID != "pdu-1" || pdu.PowerCapacity != "5kW" {
		t.Fatalf("payload = %#v", cap.cpReq.Payload)
	}
}

func TestHandlePutPDU_MissingRNCUI(t *testing.T) {
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: "x"}), nil, nil)
	rr := httptest.NewRecorder()
	h.handlePutPDU(rr, writeReq(t, http.MethodPost, "/pdu", `{"id":"x"}`, ""))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandlePutPDU_MissingID(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	h.handlePutPDU(rr, writeReq(t, http.MethodPost, "/pdu", `{"name":"p"}`, "r1"))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandlePutNisd_Success_MapsNetInfo(t *testing.T) {
	var cap capturedCall
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: "nisd-1", Success: true}), nil, &cap)
	rr := httptest.NewRecorder()
	h.handlePutNisd(rr, writeReq(t, http.MethodPost, "/nisd",
		`{"id":"nisd-1","peer_port":9000,"failure_domain":["pdu","rack"],"net_info":[{"ip_addr":"10.0.0.1","port":7000}]}`, "r1"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if cap.name != cpLib.PUT_NISD {
		t.Fatalf("op = %q", cap.name)
	}
	nisd, ok := cap.cpReq.Payload.(cpLib.Nisd)
	if !ok || nisd.ID != "nisd-1" || nisd.PeerPort != 9000 {
		t.Fatalf("payload = %#v", cap.cpReq.Payload)
	}
	if len(nisd.NetInfo) != 1 || nisd.NetInfo[0].Port != 7000 || nisd.NetInfoCnt != 1 {
		t.Fatalf("net_info mapping wrong: %+v cnt=%d", nisd.NetInfo, nisd.NetInfoCnt)
	}
	if len(nisd.FailureDomain) != 2 {
		t.Fatalf("failure_domain = %v", nisd.FailureDomain)
	}
}

func TestHandlePutNisdArgs_Success(t *testing.T) {
	var cap capturedCall
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{Success: true}), nil, &cap)
	rr := httptest.NewRecorder()
	h.handlePutNisdArgs(rr, writeReq(t, http.MethodPost, "/nisd_args",
		`{"defrag":true,"mbc_cnt":4,"s3":"bucket"}`, "r1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	args, ok := cap.cpReq.Payload.(cpLib.NisdArgs)
	if !ok || !args.Defrag || args.MBCCnt != 4 || args.S3 != "bucket" {
		t.Fatalf("payload = %#v", cap.cpReq.Payload)
	}
}

// /nisd dispatches GET->read, POST->write, others->405.
func TestHandleNisd_MethodDispatch(t *testing.T) {
	// GET -> read
	hGet := newFakeHandler(gobReply(t, cpLib.Nisd{ID: "n1", PeerPort: 1}), nil, nil)
	rr := httptest.NewRecorder()
	hGet.handleNisd(rr, httptest.NewRequest(http.MethodGet, "/nisd?id=n1", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	// POST -> write
	hPost := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: "n1", Success: true}), nil, nil)
	rr = httptest.NewRecorder()
	hPost.handleNisd(rr, writeReq(t, http.MethodPost, "/nisd", `{"id":"n1"}`, "r1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	// PUT -> 405
	rr = httptest.NewRecorder()
	newFakeHandler(nil, nil, nil).handleNisd(rr, httptest.NewRequest(http.MethodPut, "/nisd", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT status = %d, want 405", rr.Code)
	}
}

// deleteVdevReq builds a DELETE /api/vdev/{id} request with the {id} path value
// set (handlers read r.PathValue("id")) and an optional X-RNCUI header.
func deleteVdevReq(t *testing.T, id, rncui string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodDelete, "/api/vdev/"+id, nil)
	r.SetPathValue("id", id)
	if rncui != "" {
		r.Header.Set(rncuiHeader, rncui)
	}
	return r
}

func TestHandleDeleteVdev_Success(t *testing.T) {
	var cap capturedCall
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: "v1", Success: true}), nil, &cap)
	rr := httptest.NewRecorder()
	h.handleDeleteVdev(rr, deleteVdevReq(t, "v1", "r1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var got restapi.WriteResponse
	decodeBody(t, rr, &got)
	if !got.Success || got.ID != "v1" {
		t.Fatalf("response = %+v", got)
	}
	if cap.name != cpLib.DELETE_VDEV || !cap.isWrite || cap.rncui != "r1" {
		t.Fatalf("captured = %+v", cap)
	}
	dv, ok := cap.cpReq.Payload.(cpLib.DeleteVdevReq)
	if !ok || dv.ID != "v1" {
		t.Fatalf("payload = %#v", cap.cpReq.Payload)
	}
}

func TestHandleDeleteVdev_MissingID(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	h.handleDeleteVdev(rr, deleteVdevReq(t, "", "r1"))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleDeleteVdev_MissingRNCUI(t *testing.T) {
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{Success: true}), nil, nil)
	rr := httptest.NewRecorder()
	h.handleDeleteVdev(rr, deleteVdevReq(t, "v1", ""))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHandleMountVdev_Success(t *testing.T) {
	var cap capturedCall
	h := newFakeHandler(gobReply(t, cpLib.VdevMountInfo{MountCounter: 7}), nil, &cap)
	rr := httptest.NewRecorder()
	h.handleMountVdev(rr, writeReq(t, http.MethodPost, "/mount_vdev", `{"vdev_id":"v1"}`, "r1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var got restapi.MountVdevResponse
	decodeBody(t, rr, &got)
	if !got.Success || got.MountCounter != 7 {
		t.Fatalf("response = %+v", got)
	}
	if cap.name != cpLib.MOUNT_VDEV {
		t.Fatalf("op = %q", cap.name)
	}
	mv, ok := cap.cpReq.Payload.(cpLib.MountVdevRequest)
	if !ok || mv.VdevID != "v1" {
		t.Fatalf("payload = %#v", cap.cpReq.Payload)
	}
}

func TestHandleCreateSnap_Success(t *testing.T) {
	var cap capturedCall
	reply := gobReply(t, cpLib.SnapResponseXML{SnapName: cpLib.SnapName{Name: "snap-1", Success: true}})
	h := newFakeHandler(reply, nil, &cap)
	rr := httptest.NewRecorder()
	h.handleCreateSnap(rr, writeReq(t, http.MethodPost, "/snap",
		`{"vdev_id":"v1","snap_name":"snap-1","chunk_seq":[10,20,30]}`, "r1"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var got restapi.CreateSnapResponse
	decodeBody(t, rr, &got)
	if !got.Success || got.SnapName != "snap-1" {
		t.Fatalf("response = %+v", got)
	}
	snap, ok := cap.cpReq.Payload.(cpLib.SnapXML)
	if !ok || snap.Vdev != "v1" || snap.SnapName != "snap-1" || len(snap.Chunks) != 3 {
		t.Fatalf("payload = %#v", cap.cpReq.Payload)
	}
	// chunk_seq maps to ChunkXML{Idx: position, Seq: value}
	if snap.Chunks[1].Idx != 1 || snap.Chunks[1].Seq != 20 {
		t.Fatalf("chunk[1] = %+v", snap.Chunks[1])
	}
}

func TestHandleCreateSnap_NotCreated(t *testing.T) {
	reply := gobReply(t, cpLib.SnapResponseXML{SnapName: cpLib.SnapName{Name: "s", Success: false}})
	h := newFakeHandler(reply, nil, nil)
	rr := httptest.NewRecorder()
	h.handleCreateSnap(rr, writeReq(t, http.MethodPost, "/snap",
		`{"vdev_id":"v1","snap_name":"s"}`, "r1"))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleCreateSnap_MissingFields(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	h.handleCreateSnap(rr, writeReq(t, http.MethodPost, "/snap", `{"vdev_id":"v1"}`, "r1"))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}
