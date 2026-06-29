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
	req := httptest.NewRequest(http.MethodPost, "/create_vdev", strings.NewReader(body))
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
	req := httptest.NewRequest(http.MethodGet, "/create_vdev", nil)
	h.handleCreateVdev(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rr.Code)
	}
}

// restRoutes must register exactly the implemented endpoints.
func TestRestRoutesRegistered(t *testing.T) {
	h := &proxyHandler{}
	routes := h.restRoutes()
	for _, p := range []string{"/vdev", "/nisd", "/get_chunk", "/create_vdev"} {
		if routes[p] == nil {
			t.Errorf("route %s not registered", p)
		}
	}
}
