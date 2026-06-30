package main

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	pmdbClient "github.com/00pauln00/niova-pumicedb/go/pkg/pumiceclient"
	PumiceDBCommon "github.com/00pauln00/niova-pumicedb/go/pkg/pumicecommon"
	funclib "github.com/00pauln00/niova-pumicedb/go/pkg/pumicefunc/common"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"

	log "github.com/00pauln00/niova-lookout/pkg/xlog"
)

// restMux returns the TiDB-style REST router served under the /api/ prefix.
// Unlike restRoutes (exact-path, method-agnostic), this uses ServeMux
// method+pattern matching and path parameters (e.g. DELETE /api/vdev/{id}) so
// the routes mirror the TiDB control-plane contract. The httpserver delegates
// every /api/ request here (see HTTPServerHandler.RESTHandler).
func (handler *proxyHandler) restMux() *http.ServeMux {
	mux := http.NewServeMux()
	// vdev lifecycle + reads.
	mux.HandleFunc("POST /api/vdev", handler.handleCreateVdev)
	mux.HandleFunc("GET /api/vdev", handler.handleGetVdev)
	mux.HandleFunc("DELETE /api/vdev/{id}", handler.handleDeleteVdev)
	// reads.
	mux.HandleFunc("GET /api/nisd", handler.handleGetNisd)
	mux.HandleFunc("GET /api/chunk", handler.handleGetChunk)
	// Registered in subsequent milestones:
	//   "POST /api/infra", "GET /api/infra"
	//   "GET /api/chunks"        (paginated read)
	//   "GET|PUT /api/resource"  (entity reads/upserts)
	//   "POST /api/chunk/reassign", "POST|GET /api/pfs", "POST /api/reset"
	return mux
}

// restRoutes returns the legacy exact-path REST route table. These niova-only
// write endpoints have no TiDB equivalent yet; they keep their flat paths until
// they are folded into the /api/ contract (e.g. PUT /api/resource) in a later
// milestone. They run alongside restMux and the legacy /func endpoint.
func (handler *proxyHandler) restRoutes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		// nisd write (GET reads moved to GET /api/nisd; POST still served here).
		"/nisd": handler.handleNisd,
		// Single-entity infra writes.
		"/pdu":        handler.handlePutPDU,
		"/rack":       handler.handlePutRack,
		"/hypervisor": handler.handlePutHypervisor,
		"/device":     handler.handlePutDevice,
		"/pfs":        handler.handlePutPFS,
		"/partition":  handler.handlePutPartition,
		"/nisd_args":  handler.handlePutNisdArgs,
		// Vdev lifecycle writes.
		"/snap":       handler.handleCreateSnap,
		"/mount_vdev": handler.handleMountVdev,
	}
}

// tokenFromRequest extracts a bearer token from the Authorization header, if
// present. The REST contract defines no security scheme, so an absent token is
// not an error here; downstream auth (when enabled) decides.
func tokenFromRequest(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		return auth[len(prefix):]
	}
	return auth
}

// rncuiHeader carries the client-supplied write idempotency key (RNCUI).
const rncuiHeader = "X-RNCUI"

// rncuiFromRequest returns the RNCUI the client supplied via the X-RNCUI header.
// PumiceDB requires a stable RNCUI per write so that client retries dedup
// instead of double-applying; REST writes must therefore provide one. This is a
// niova/PumiceDB-specific extension to the (TiDB) REST contract: TiDB clients
// that never retry-with-dedup simply don't set it, and such writes are rejected.
func rncuiFromRequest(r *http.Request) (string, error) {
	rncui := strings.TrimSpace(r.Header.Get(rncuiHeader))
	if rncui == "" {
		return "", fmt.Errorf("missing required %s header", rncuiHeader)
	}
	return rncui, nil
}

// sendFuncReq sends a control-plane FuncReq to PMDB and returns the raw GOB
// reply bytes. It is the transport-agnostic core shared by the REST handlers
// (and is intended to back the legacy /func handlers as they migrate).
func (handler *proxyHandler) sendFuncReq(name string, cpReq cpLib.CPReq, isWrite bool, rncui string, wsn int64) ([]byte, error) {
	r := &funclib.FuncReq{Name: name, Args: cpReq}
	reply := new([]byte)
	reqArgs := &pmdbClient.PmdbReq{
		Request:  encode(r),
		ReqType:  PumiceDBCommon.FUNC_REQ,
		GetReply: 1,
		Reply:    reply,
	}

	var err error
	if isWrite {
		reqArgs.Rncui = rncui
		reqArgs.WriteSeqNum = wsn
		err = handler.pmdbClientObj.Put(reqArgs)
	} else {
		err = handler.pmdbClientObj.Get(reqArgs)
	}
	if err != nil {
		return nil, err
	}
	if reqArgs.Reply == nil {
		return nil, fmt.Errorf("nil reply received from pmdb for %s", name)
	}
	return *reqArgs.Reply, nil
}

// callFunc invokes a control-plane function with a typed payload and decodes the
// typed CPResp payload. respPayload must be a pointer to the expected response
// struct. The returned *cpLib.CPResp carries any structured CPError.
//
// For writes (isWrite), rncui must be the client-supplied RNCUI (see
// rncuiFromRequest); wsn is the write sequence number (0 matches the legacy
// /func default, where the sequence is encoded in the RNCUI). Reads pass "", 0.
func (handler *proxyHandler) callFunc(name string, cpReq cpLib.CPReq, isWrite bool, rncui string, wsn int64, respPayload any) (*cpLib.CPResp, error) {
	// For testing purpose!
	send := handler.sendFuncReq
	if handler.sendFuncReqFn != nil {
		send = handler.sendFuncReqFn
	}
	replyBytes, err := send(name, cpReq, isWrite, rncui, wsn)
	if err != nil {
		log.Errorf("callFunc: pmdb request failed | func=%s err=%v", name, err)
		return nil, err
	}
	if len(replyBytes) == 0 {
		return nil, fmt.Errorf("empty reply from pmdb for %s", name)
	}

	// Decode the CPResp envelope. The typed payload is left for gob to populate
	// into cpResp.Payload: gob allocates a fresh concrete value of the
	// wire-registered type and will NOT fill a caller-supplied pointer that was
	// pre-set on the interface field. We then copy that value into the caller's
	// respPayload pointer below.
	cpResp := &cpLib.CPResp{}
	if err := PumiceDBCommon.Decoder(PumiceDBCommon.GOB, replyBytes, cpResp); err != nil {
		log.Errorf("callFunc: failed to decode CPResp | func=%s size=%d err=%v", name, len(replyBytes), err)
		return nil, err
	}

	// On success the server returns the function payload as a concrete value in
	// cpResp.Payload; copy it into respPayload. Error responses carry a nil
	// payload (Error is set instead), so there is nothing to copy.
	if respPayload != nil && cpResp.Payload != nil {
		dst := reflect.ValueOf(respPayload)
		if dst.Kind() != reflect.Ptr || dst.IsNil() {
			return cpResp, fmt.Errorf("callFunc: respPayload must be a non-nil pointer, got %T", respPayload)
		}
		src := reflect.ValueOf(cpResp.Payload)
		if !src.Type().AssignableTo(dst.Elem().Type()) {
			return cpResp, fmt.Errorf("callFunc: %s returned payload of type %T, not assignable to %s",
				name, cpResp.Payload, dst.Elem().Type())
		}
		dst.Elem().Set(src)
	}
	return cpResp, nil
}
