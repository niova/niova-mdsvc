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

// decodePayloadCode asserts a success envelope ({success:true, payload:{...}})
// with the given HTTP status code and returns the decoded payload.
func decodePayloadCode[T any](t *testing.T, rr *httptest.ResponseRecorder, wantCode int) T {
	t.Helper()
	if rr.Code != wantCode {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, wantCode, rr.Body.String())
	}
	var resp restapi.APIResponse[T]
	decodeBody(t, rr, &resp)
	if !resp.Success {
		t.Fatalf("want success:true; got %+v (body=%s)", resp, rr.Body.String())
	}
	if resp.Payload == nil {
		t.Fatalf("want non-nil payload; body=%s", rr.Body.String())
	}
	return *resp.Payload
}

// decodePayload asserts an HTTP 200 success envelope and returns the payload.
func decodePayload[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	return decodePayloadCode[T](t, rr, http.StatusOK)
}

// expectMethodError asserts a non-user endpoint reported a method error in the
// envelope: HTTP 200 with {success:false, error:non-empty}. Under the unified
// contract these endpoints never emit an HTTP error status (routing-level 405 is
// the only exception; see requireMethod).
func expectMethodError(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (method error in envelope); body=%s", rr.Code, rr.Body.String())
	}
	var got restapi.ErrorResponse
	decodeBody(t, rr, &got)
	if got.Success || got.Error == "" {
		t.Fatalf("want {success:false, error:...}; got %+v", got)
	}
}

// ---- GET /vdev ----

func TestHandleGetVdev_Success(t *testing.T) {
	reply := gobReply(t, cpLib.VdevCfg{ID: "vdev-1", Size: 16 * cpLib.CHUNK_SIZE, NumChunks: 2, NumReplica: 3})
	h := newFakeHandler(reply, nil, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/vdev?id=vdev-1", nil)
	h.handleGetVdev(rr, req)

	got := decodePayload[restapi.GetVdevPayload](t, rr)
	if got.ID != "vdev-1" || got.Size != 16*cpLib.CHUNK_SIZE || got.NumChunks != 2 || got.NumReplicas != 3 {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestHandleGetVdev_MissingID(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/vdev", nil)
	h.handleGetVdev(rr, req)
	expectMethodError(t, rr)
}

func TestHandleGetVdev_NotFound_EmptyID(t *testing.T) {
	// Server returns a zero-value VdevCfg (no such vdev) -> method error in body.
	h := newFakeHandler(gobReply(t, cpLib.VdevCfg{}), nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/vdev?id=missing", nil)
	h.handleGetVdev(rr, req)
	expectMethodError(t, rr)
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
	// A control-plane function error is reported in the envelope with HTTP 200.
	h := newFakeHandler(gobFuncErr(t, "vdev not found"), nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/vdev?id=v", nil)
	h.handleGetVdev(rr, req)
	expectMethodError(t, rr)
}

func TestHandleGetVdev_ServerAuthError(t *testing.T) {
	// Even an auth error on a non-user (data) endpoint travels in the envelope
	// with HTTP 200 — only the user/auth endpoints map errors to HTTP codes.
	h := newFakeHandler(gobAuthErr(t, "Invalid Token"), nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/vdev?id=v", nil)
	h.handleGetVdev(rr, req)
	expectMethodError(t, rr)
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

	got := decodePayload[restapi.GetNisdPayload](t, rr)
	if got.ID != "nisd-1" || got.PeerPort != 6000 || len(got.NetInfo) != 2 {
		t.Fatalf("unexpected payload: %+v", got)
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
	expectMethodError(t, rr)
}

func TestHandleGetNisd_NotFound(t *testing.T) {
	h := newFakeHandler(gobReply(t, cpLib.Nisd{}), nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nisd?id=missing", nil)
	h.handleGetNisd(rr, req)
	expectMethodError(t, rr)
}

// ---- GET /get_chunk ----

func TestHandleGetChunk_Success(t *testing.T) {
	cn := cpLib.ChunkNisd{NumReplicas: 2, NisdUUIDs: "nisd-a, nisd-b"}
	var cap capturedCall
	h := newFakeHandler(gobReply(t, cn), nil, &cap)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/get_chunk?vdev_id=v1&chunk_idx=3", nil)
	h.handleGetChunk(rr, req)

	got := decodePayload[restapi.GetChunkPayload](t, rr)
	if got.VdevID != "v1" || got.ChunkIdx != 3 {
		t.Fatalf("unexpected payload: %+v", got)
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
	expectMethodError(t, rr)
}

func TestHandleGetChunk_InvalidChunkIdx(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/get_chunk?vdev_id=v1&chunk_idx=-1", nil)
	h.handleGetChunk(rr, req)
	expectMethodError(t, rr)
}

func TestHandleGetChunk_NotFound_Empty(t *testing.T) {
	h := newFakeHandler(gobReply(t, cpLib.ChunkNisd{NisdUUIDs: ""}), nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/get_chunk?vdev_id=v1&chunk_idx=0", nil)
	h.handleGetChunk(rr, req)
	expectMethodError(t, rr)
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

	got := decodePayload[restapi.CreateVdevPayload](t, rr)
	wantChunks := int(cpLib.Count8GBChunks(size))
	if got.VdevID != "vdev-new" || got.NumChunks != wantChunks {
		t.Fatalf("unexpected payload: %+v (wantChunks=%d)", got, wantChunks)
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

func TestHandleCreateVdev_Filter(t *testing.T) {
	var cap capturedCall
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: "vdev-new", Success: true}), nil, &cap)
	rr := httptest.NewRecorder()
	req := createVdevReq(t,
		`{"size_bytes":25769803776,"num_replicas":3,"failure_domain":"hv","entity_ids":["hv-1","hv-2"]}`,
		"r1")
	h.handleCreateVdev(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	vr, ok := cap.cpReq.Payload.(cpLib.VdevReq)
	if !ok {
		t.Fatalf("payload = %#v", cap.cpReq.Payload)
	}
	// failure_domain -> Filter.Type; only the first entity_id is applied.
	if vr.Filter.Type != cpLib.FD_HV || vr.Filter.ID != "hv-1" {
		t.Fatalf("filter = %+v, want {Type:FD_HV ID:hv-1}", vr.Filter)
	}
}

func TestHandleCreateVdev_PFSByID(t *testing.T) {
	var cap capturedCall
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: "vdev-new", Success: true}), nil, &cap)
	rr := httptest.NewRecorder()
	const pfsID = "550e8400-e29b-41d4-a716-446655440000"
	req := createVdevReq(t,
		`{"size_bytes":25769803776,"num_replicas":3,"pfs":"`+pfsID+`"}`, "r1")
	h.handleCreateVdev(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	vr := cap.cpReq.Payload.(cpLib.VdevReq)
	// A UUID goes to PFSID; PFSName stays empty.
	if vr.Vdev.PFSID != pfsID || vr.Vdev.PFSName != "" {
		t.Fatalf("pfs = {ID:%q Name:%q}, want ID=%q", vr.Vdev.PFSID, vr.Vdev.PFSName, pfsID)
	}
}

func TestHandleCreateVdev_PFSByName(t *testing.T) {
	var cap capturedCall
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: "vdev-new", Success: true}), nil, &cap)
	rr := httptest.NewRecorder()
	req := createVdevReq(t,
		`{"size_bytes":25769803776,"num_replicas":3,"pfs":"my-pfs"}`, "r1")
	h.handleCreateVdev(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	vr := cap.cpReq.Payload.(cpLib.VdevReq)
	// A non-UUID goes to PFSName; PFSID stays empty.
	if vr.Vdev.PFSName != "my-pfs" || vr.Vdev.PFSID != "" {
		t.Fatalf("pfs = {ID:%q Name:%q}, want Name=my-pfs", vr.Vdev.PFSID, vr.Vdev.PFSName)
	}
}

func TestHandleCreateVdev_Name(t *testing.T) {
	var cap capturedCall
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: "vdev-new", Success: true}), nil, &cap)
	rr := httptest.NewRecorder()
	req := createVdevReq(t,
		`{"size_bytes":25769803776,"num_replicas":3,"name":"myvdev1"}`, "r1")
	h.handleCreateVdev(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	vr := cap.cpReq.Payload.(cpLib.VdevReq)
	if vr.Vdev.Name != "myvdev1" {
		t.Fatalf("name = %q, want myvdev1", vr.Vdev.Name)
	}
}

func TestHandleCreateVdev_BadFailureDomain(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := createVdevReq(t, `{"size_bytes":25769803776,"num_replicas":3,"failure_domain":"widget"}`, "r1")
	h.handleCreateVdev(rr, req)
	expectMethodError(t, rr)
}

func TestHandleCreateVdev_MissingRNCUI(t *testing.T) {
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: "x"}), nil, nil)
	rr := httptest.NewRecorder()
	req := createVdevReq(t, `{"size_bytes":25769803776,"num_replicas":3}`, "")
	h.handleCreateVdev(rr, req)
	expectMethodError(t, rr)
}

func TestHandleCreateVdev_BadSize(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := createVdevReq(t, `{"size_bytes":0,"num_replicas":3}`, "r1")
	h.handleCreateVdev(rr, req)
	expectMethodError(t, rr)
}

func TestHandleCreateVdev_BadReplicas(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := createVdevReq(t, `{"size_bytes":25769803776,"num_replicas":0}`, "r1")
	h.handleCreateVdev(rr, req)
	expectMethodError(t, rr)
}

func TestHandleCreateVdev_InvalidJSON(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	req := createVdevReq(t, `{not json`, "r1")
	h.handleCreateVdev(rr, req)
	expectMethodError(t, rr)
}

func TestHandleCreateVdev_NoIDReturned(t *testing.T) {
	// Server reports a success-shaped reply but with no ID -> method error in body.
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: ""}), nil, nil)
	rr := httptest.NewRecorder()
	req := createVdevReq(t, `{"size_bytes":25769803776,"num_replicas":3}`, "r1")
	h.handleCreateVdev(rr, req)
	expectMethodError(t, rr)
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

// restMux must register the TiDB-style /api/ routes with method matching.
func TestRestMuxRegistered(t *testing.T) {
	h := &proxyHandler{}
	mux := h.restMux()
	for _, c := range []struct{ method, target string }{
		{http.MethodPost, "/api/vdev"},
		{http.MethodGet, "/api/vdev"},
		{http.MethodDelete, "/api/vdev/some-id"},
		{http.MethodPost, "/api/snap"},
		{http.MethodPost, "/api/mount_vdev"},
		{http.MethodGet, "/api/nisd"},
		{http.MethodGet, "/api/chunk"},
		{http.MethodGet, "/api/resource"},
		{http.MethodPut, "/api/resource"},
		{http.MethodGet, "/api/infra"},
		{http.MethodPost, "/api/pfs"},
		{http.MethodGet, "/api/pfs"},
		{http.MethodPost, "/api/nisd_args"},
		{http.MethodGet, "/api/nisd_args"},
		{http.MethodPost, "/users/login"},
		{http.MethodPost, "/users/admin"},
		{http.MethodPost, "/api/users"},
		{http.MethodGet, "/api/users"},
		{http.MethodGet, "/api/users/some-id"},
		{http.MethodPut, "/api/users/some-id"},
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

func TestHandlePutResource_PDU(t *testing.T) {
	var cap capturedCall
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: "pdu-1", Success: true}), nil, &cap)
	rr := httptest.NewRecorder()
	h.handlePutResource(rr, writeReq(t, http.MethodPut, "/api/resource?type=pdu",
		`{"id":"pdu-1","name":"p","power_cap":"5kW"}`, "r1"))

	got := decodePayload[restapi.WritePayload](t, rr)
	if got.ID != "pdu-1" {
		t.Fatalf("payload = %+v", got)
	}
	if !cap.isWrite || cap.rncui != "r1" || cap.name != cpLib.PUT_PDU {
		t.Fatalf("captured = %+v", cap)
	}
	pdu, ok := cap.cpReq.Payload.(cpLib.PDU)
	if !ok || pdu.ID != "pdu-1" || pdu.PowerCapacity != "5kW" {
		t.Fatalf("payload = %#v", cap.cpReq.Payload)
	}
}

func TestHandlePutResource_Nisd_MapsNetInfo(t *testing.T) {
	var cap capturedCall
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: "nisd-1", Success: true}), nil, &cap)
	rr := httptest.NewRecorder()
	h.handlePutResource(rr, writeReq(t, http.MethodPut, "/api/resource?type=nisd",
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

func TestHandlePutResource_MissingType(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	h.handlePutResource(rr, writeReq(t, http.MethodPut, "/api/resource", `{"id":"x"}`, "r1"))
	expectMethodError(t, rr)
}

func TestHandlePutResource_UnsupportedType(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	h.handlePutResource(rr, writeReq(t, http.MethodPut, "/api/resource?type=widget", `{"id":"x"}`, "r1"))
	expectMethodError(t, rr)
}

func TestHandlePutResource_MissingID(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	h.handlePutResource(rr, writeReq(t, http.MethodPut, "/api/resource?type=pdu", `{"name":"p"}`, "r1"))
	expectMethodError(t, rr)
}

func TestHandlePutResource_MissingRNCUI(t *testing.T) {
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{ID: "x"}), nil, nil)
	rr := httptest.NewRecorder()
	h.handlePutResource(rr, writeReq(t, http.MethodPut, "/api/resource?type=pdu", `{"id":"x"}`, ""))
	expectMethodError(t, rr)
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

	got := decodePayload[restapi.WritePayload](t, rr)
	if got.ID != "v1" {
		t.Fatalf("payload = %+v", got)
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
	expectMethodError(t, rr)
}

func TestHandleDeleteVdev_MissingRNCUI(t *testing.T) {
	h := newFakeHandler(gobReply(t, cpLib.ResponseXML{Success: true}), nil, nil)
	rr := httptest.NewRecorder()
	h.handleDeleteVdev(rr, deleteVdevReq(t, "v1", ""))
	expectMethodError(t, rr)
}

func TestHandleMountVdev_Success(t *testing.T) {
	var cap capturedCall
	reply := gobReply(t, cpLib.VdevCfg{
		ID:            "v1",
		AccessToken:   "tok-1",
		VdevMountInfo: cpLib.VdevMountInfo{MountCounter: 7},
	})
	h := newFakeHandler(reply, nil, &cap)
	rr := httptest.NewRecorder()
	h.handleMountVdev(rr, writeReq(t, http.MethodPost, "/mount_vdev", `{"vdev_id":"v1"}`, "r1"))

	got := decodePayload[restapi.MountVdevPayload](t, rr)
	if got.ID != "v1" || got.AccessToken != "tok-1" || got.MountCounter != 7 {
		t.Fatalf("payload = %+v", got)
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

	got := decodePayload[restapi.CreateSnapPayload](t, rr)
	if got.SnapName != "snap-1" {
		t.Fatalf("payload = %+v", got)
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
	expectMethodError(t, rr)
}

func TestHandleCreateSnap_MissingFields(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	h.handleCreateSnap(rr, writeReq(t, http.MethodPost, "/snap", `{"vdev_id":"v1"}`, "r1"))
	expectMethodError(t, rr)
}

// ---- GET /api/resource ----

func TestHandleGetResource_PDU(t *testing.T) {
	reply := gobReply(t, cpLib.ResourceListResp{
		ResourceType: cpLib.ResourcePDU,
		PDUs:         []cpLib.PDU{{ID: "pdu-1", Name: "p", PowerCapacity: "5kW"}},
	})
	h := newFakeHandler(reply, nil, nil)
	rr := httptest.NewRecorder()
	h.handleGetResource(rr, httptest.NewRequest(http.MethodGet, "/api/resource?type=pdu", nil))

	got := decodePayload[restapi.GetResourcePayload](t, rr)
	if got.Type != "pdu" || len(got.Resources) != 1 {
		t.Fatalf("payload = %+v", got)
	}
	var pdu restapi.PDU
	if err := json.Unmarshal(got.Resources[0], &pdu); err != nil {
		t.Fatalf("unmarshal resource: %v", err)
	}
	if pdu.ID != "pdu-1" || pdu.PowerCap != "5kW" {
		t.Fatalf("pdu = %+v", pdu)
	}
}

func TestHandleGetResource_SingleFetch(t *testing.T) {
	var cap capturedCall
	reply := gobReply(t, cpLib.ResourceListResp{
		ResourceType: cpLib.ResourceNisd,
		Nisds:        []cpLib.Nisd{{ID: "nisd-1"}},
	})
	h := newFakeHandler(reply, nil, &cap)
	rr := httptest.NewRecorder()
	h.handleGetResource(rr, httptest.NewRequest(http.MethodGet, "/api/resource?type=nisd&id=nisd-1", nil))

	got := decodePayload[restapi.GetResourcePayload](t, rr)
	if len(got.Resources) != 1 {
		t.Fatalf("payload = %+v", got)
	}
	var nisd restapi.NISD
	if err := json.Unmarshal(got.Resources[0], &nisd); err != nil {
		t.Fatalf("unmarshal resource: %v", err)
	}
	if nisd.ID != "nisd-1" {
		t.Fatalf("nisd = %+v", nisd)
	}
	// A single-entity fetch must forward the id and disable GetAll.
	gr, ok := cap.cpReq.Payload.(cpLib.GetResourceReq)
	if !ok || gr.ID != "nisd-1" || gr.GetAll {
		t.Fatalf("forwarded req = %#v", cap.cpReq.Payload)
	}
}

func TestHandleGetResource_MissingType(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	h.handleGetResource(rr, httptest.NewRequest(http.MethodGet, "/api/resource", nil))
	expectMethodError(t, rr)
}

func TestHandleGetResource_UnsupportedType(t *testing.T) {
	h := newFakeHandler(nil, nil, nil)
	rr := httptest.NewRecorder()
	h.handleGetResource(rr, httptest.NewRequest(http.MethodGet, "/api/resource?type=widget", nil))
	expectMethodError(t, rr)
}

// ---- GET /api/pfs ----

func TestHandleGetPFS_Success(t *testing.T) {
	reply := gobReply(t, []cpLib.PFS{{ID: "pfs-1", Name: "p", VdevIDs: []string{"v1"}}})
	h := newFakeHandler(reply, nil, nil)
	rr := httptest.NewRecorder()
	h.handleGetPFS(rr, httptest.NewRequest(http.MethodGet, "/api/pfs?id=pfs-1", nil))

	got := decodePayload[restapi.GetPFSPayload](t, rr)
	if len(got.PFS) != 1 || got.PFS[0].ID != "pfs-1" || len(got.PFS[0].VdevIDs) != 1 {
		t.Fatalf("payload = %+v", got)
	}
}

// ---- GET /api/nisd_args ----

func TestHandleGetNisdArgs_Success(t *testing.T) {
	reply := gobReply(t, cpLib.NisdArgs{Defrag: true, MBCCnt: 4, S3: "bucket"})
	h := newFakeHandler(reply, nil, nil)
	rr := httptest.NewRecorder()
	h.handleGetNisdArgs(rr, httptest.NewRequest(http.MethodGet, "/api/nisd_args", nil))

	got := decodePayload[restapi.GetNisdArgsPayload](t, rr)
	if !got.NisdArgs.Defrag || got.NisdArgs.MBCCnt != 4 || got.NisdArgs.S3 != "bucket" {
		t.Fatalf("payload = %+v", got)
	}
}

// ---- GET /api/infra ----

// TestHandleGetInfra_AssemblesTree drives the per-type GET_ALL_RESOURCES calls
// with a fake that returns a distinct ResourceListResp per resource type, and
// asserts the proxy nests them PDU -> Rack -> Hypervisor -> Device -> NISD.
func TestHandleGetInfra_AssemblesTree(t *testing.T) {
	h := &proxyHandler{
		sendFuncReqFn: func(name string, cpReq cpLib.CPReq, isWrite bool, rncui string, wsn int64) ([]byte, error) {
			rt := cpReq.Payload.(cpLib.GetResourceReq).ResourceType
			var rl cpLib.ResourceListResp
			switch rt {
			case cpLib.ResourcePDU:
				rl = cpLib.ResourceListResp{PDUs: []cpLib.PDU{{ID: "pdu-1"}}}
			case cpLib.ResourceRack:
				rl = cpLib.ResourceListResp{Racks: []cpLib.Rack{{ID: "rack-1", PDUID: "pdu-1"}}}
			case cpLib.ResourceHypervisor:
				rl = cpLib.ResourceListResp{Hypervisors: []cpLib.Hypervisor{{ID: "hv-1", RackID: "rack-1"}}}
			case cpLib.ResourceDevice:
				rl = cpLib.ResourceListResp{Devices: []cpLib.Device{{ID: "dev-1", HypervisorID: "hv-1"}}}
			case cpLib.ResourceNisd:
				rl = cpLib.ResourceListResp{Nisds: []cpLib.Nisd{{ID: "nisd-1", FailureDomain: []string{"pdu-1", "rack-1", "hv-1", "dev-1"}}}}
			}
			enc, err := cpLib.EncodeResponse(rl)
			if err != nil {
				return nil, err
			}
			return enc.([]byte), nil
		},
	}
	rr := httptest.NewRecorder()
	h.handleGetInfra(rr, httptest.NewRequest(http.MethodGet, "/api/infra", nil))

	got := decodePayload[restapi.GetInfraPayload](t, rr)
	if len(got.Infra.PDUs) != 1 || got.Infra.PDUs[0].ID != "pdu-1" {
		t.Fatalf("pdus = %+v", got.Infra.PDUs)
	}
	racks := got.Infra.PDUs[0].Racks
	if len(racks) != 1 || racks[0].ID != "rack-1" {
		t.Fatalf("racks = %+v", racks)
	}
	hvs := racks[0].Hypervisors
	if len(hvs) != 1 || hvs[0].ID != "hv-1" {
		t.Fatalf("hypervisors = %+v", hvs)
	}
	devs := hvs[0].Devices
	if len(devs) != 1 || devs[0].ID != "dev-1" {
		t.Fatalf("devices = %+v", devs)
	}
	nisds := devs[0].NISDs
	if len(nisds) != 1 || nisds[0].ID != "nisd-1" {
		t.Fatalf("nisds = %+v", nisds)
	}
}
