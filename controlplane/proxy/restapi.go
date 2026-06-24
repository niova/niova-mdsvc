package main

import (
	"fmt"
	"net/http"
	"strings"

	pmdbClient "github.com/00pauln00/niova-pumicedb/go/pkg/pumiceclient"
	PumiceDBCommon "github.com/00pauln00/niova-pumicedb/go/pkg/pumicecommon"
	funclib "github.com/00pauln00/niova-pumicedb/go/pkg/pumicefunc/common"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"

	log "github.com/00pauln00/niova-lookout/pkg/xlog"
)

// restRoutes returns the REST API route table registered on the HTTP server.
// Routes are added here as each endpoint is implemented; they run alongside the
// legacy /func endpoint until the migration to REST is complete.
func (handler *proxyHandler) restRoutes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/vdev":      handler.handleGetVdev,
		"/nisd":      handler.handleGetNisd,
		"/get_chunk": handler.handleGetChunk,
		// Registered in subsequent milestones:
		//   "/create_vdev", "/create_infra" (writes)
		//   "/get_chunks"                   (paginated read)
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
	replyBytes, err := handler.sendFuncReq(name, cpReq, isWrite, rncui, wsn)
	if err != nil {
		log.Errorf("callFunc: pmdb request failed | func=%s err=%v", name, err)
		return nil, err
	}
	if len(replyBytes) == 0 {
		return nil, fmt.Errorf("empty reply from pmdb for %s", name)
	}

	cpResp := &cpLib.CPResp{Payload: respPayload}
	if err := PumiceDBCommon.Decoder(PumiceDBCommon.GOB, replyBytes, cpResp); err != nil {
		log.Errorf("callFunc: failed to decode CPResp | func=%s size=%d err=%v", name, len(replyBytes), err)
		return nil, err
	}
	return cpResp, nil
}
