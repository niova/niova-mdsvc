package srvctlplanefuncs

import (
	"C"
	"fmt"
	"math/rand"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/btree"

	log "github.com/00pauln00/niova-lookout/pkg/xlog"

	auth "github.com/00pauln00/niova-mdsvc/controlplane/auth/jwt"
	authz "github.com/00pauln00/niova-mdsvc/controlplane/authorizer"
	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"

	pmCmn "github.com/00pauln00/niova-pumicedb/go/pkg/pumicecommon"
	funclib "github.com/00pauln00/niova-pumicedb/go/pkg/pumicefunc/common"
	PumiceDBServer "github.com/00pauln00/niova-pumicedb/go/pkg/pumiceserver"
	storageiface "github.com/00pauln00/niova-pumicedb/go/pkg/utils/storage/interface"
)

var colmfamily string

var (
	authorizer  *authz.Authorizer
	authEnabled = true
)

// InitAuthorizer initializes the global authorizer instance from hardcoded policies.
func InitAuthorizer() {
	authorizer = authz.NewAuthorizer()
}

// SetAuthEnabled enables or disables authentication and authorization.
// This is called at startup with the value resolved from the AUTH_ENABLED
// env var (default true). When disabled, ValidateToken bypasses JWT verification.
func SetAuthEnabled(enabled bool) {
	authEnabled = enabled
}

// IsAuthEnabled reports whether authentication and authorization are enabled.
func IsAuthEnabled() bool {
	return authEnabled
}

// GetAuthorizer returns the global authorizer instance
func GetAuthorizer() *authz.Authorizer {
	return authorizer
}

const (
	DEVICE_ID        = "d"
	NISD_ID          = "n"
	SERIAL_NUM       = "sn"
	STATE            = "s"
	FAILURE_DOMAIN   = "fd"
	CLIENT_PORT      = "cp"
	PEER_PORT        = "pp"
	IP_ADDR          = "ip"
	TOTAL_SPACE      = "ts"
	AVAIL_SPACE      = "as"
	SIZE             = "sz"
	NUM_CHUNKS       = "nc"
	NUM_REPLICAS     = "nr"
	ERASURE_CODE     = "e"
	LOCATION         = "l"
	POWER_CAP        = "pw"
	SPEC             = "sp"
	NAME             = "nm"
	PORT_RANGE       = "pr"
	SSH_PORT         = "ssh"
	PORT             = "prt"
	DEVICE_PATH      = "dp"
	PARTITION_PATH   = "ptp"
	NETWORK_INFO     = "ni"
	NETWORK_INFO_CNT = "nic"
	SOCKET_PATH      = "sck"
	ENABLE_RDMA      = "rdma"
	ARCHIVE          = "arch"

	DEFRAG                  = "dfg"
	MBCCnt                  = "mbc"
	MergeHCnt               = "mhc"
	MCIReadCache            = "mrc"
	S3                      = "s3"
	DSYNC                   = "ds"
	ALLOW_DEFRAG_MCIB_CACHE = "admc"

	FD_TYPE_KEY = "fdt"
	FD_ID_KEY   = "fdi"

	cfgkey       = "cfg"
	NisdCfgKey   = "n_cfg"
	deviceCfgKey = "d_cfg"
	sdeviceKey   = "sd"
	parentInfo   = "pi"
	pduKey       = "p"
	rackKey      = "r"
	nisdKey      = "n"
	vdevKey      = "v"
	vnameKey     = "vname"
	chunkKey     = "c"
	hvKey        = "hv"
	ptKey        = "pt"
	argsKey      = "na"
	pfsKey       = "pfs"
)

// TokenClaims holds user identity extracted from a verified JWT.
type TokenClaims struct {
	UserID  string
	Role    string
	IsAdmin bool
}

// ValidateToken verifies the JWT token using CP_SECRET and extracts user claims.
// Returns an error if the token is empty, invalid, or missing required fields.
// When authentication is disabled (SetAuthEnabled(false)), the token is not
// verified and an anonymous identity is returned instead.
func ValidateToken(token string) (*TokenClaims, error) {
	if !authEnabled {
		return &TokenClaims{
			UserID: "anonymous",
			Role:   "anonymous",
		}, nil
	}
	if token == "" {
		return nil, fmt.Errorf("user token is required")
	}
	tc := &auth.Token{
		Secret: []byte(ctlplfl.CP_SECRET),
	}
	claims, err := tc.VerifyToken(token)
	if err != nil {
		log.Errorf("token verification failed: %v", err)
		return nil, fmt.Errorf("authentication failed: %v", err)
	}
	userID, ok := claims["userID"].(string)
	if !ok || userID == "" {
		log.Error("userID not found in token")
		return nil, fmt.Errorf("invalid token: missing userID")
	}
	userRole, ok := claims["role"].(string)
	if !ok || userRole == "" {
		log.Error("role not found in token")
		return nil, fmt.Errorf("invalid token: missing role")
	}
	isAdmin, _ := claims["isAdmin"].(bool)
	tokenClaim := &TokenClaims{
		UserID:  userID,
		Role:    userRole,
		IsAdmin: isAdmin,
	}
	return tokenClaim, nil
}

// validateAndAuthorizeRBAC verifies the token and checks RBAC-only permissions.
// For operations that also require ABAC (ownership checks)
func validateAndAuthorizeRBAC(token string, fn authz.FunctionName) (*TokenClaims, error) {
	tc, err := ValidateToken(token)
	if err != nil {
		return nil, err
	}
	if authorizer != nil && authEnabled {
		if !authorizer.CheckRBAC(fn, []string{tc.Role}) {
			log.Errorf("user %s with role %s not authorized for %s", tc.UserID, tc.Role, fn)
			return nil, fmt.Errorf("authorization failed: insufficient permissions")
		}
	}
	log.Infof("user %s authorized for %s", tc.UserID, fn)
	return tc, nil
}

func SetClmFamily(cf string) {
	colmfamily = cf
}

func getConfKey(cfgType, id string) string {
	return fmt.Sprintf("%s/%s", cfgType, id)
}

func getVdevChunkKey(vdevID string) string {
	return fmt.Sprintf("%s/%s/%s", vdevKey, vdevID, chunkKey)
}

func isUUID(s string) bool {
	_, err := uuid.Parse(s)

	if err == nil {
		return true
	}

	return false
}

func ReadSnapByName(args ...interface{}) (interface{}, error) {

	cbargs := args[0].(*PumiceDBServer.PmdbCbArgs)
	Snap := args[1].(ctlplfl.SnapXML)

	//FIX: Arbitrary read size
	key := fmt.Sprintf("snap/%s", Snap.SnapName)
	log.Info("Key to be read : ", key)
	readResult, err := cbargs.Store.Read(key, colmfamily)
	if err != nil {
		log.Error("Range read failure ", err)
		return ctlplfl.FuncError(err)
	}

	log.Debugf("ReadSnapByName: returning snap data for key %s", key)
	return ctlplfl.EncodeResponse(readResult)
}

func ReadSnapForVdev(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)

	Snap := args[1].(ctlplfl.SnapXML)

	key := fmt.Sprintf("%s/snap", Snap.Vdev)
	rrargs := storageiface.RangeReadArgs{
		Selector:   colmfamily,
		Key:        key,
		BufSize:    cbArgs.ReplySize,
		Consistent: false,
		Prefix:     key,
	}
	readResult, err := cbArgs.Store.RangeRead(rrargs)
	if err != nil {
		log.Error("Range read failure ", err)
		return ctlplfl.FuncError(err)
	}

	log.Tracef("ReadSnapForVdev: range read returned %d results for vdev %s", len(readResult.ResultMap), Snap.Vdev)
	for key := range readResult.ResultMap {
		c := strings.Split(key, "/")

		idx, err := strconv.ParseUint(c[len(c)-2], 10, 32)
		if err != nil {
			log.Errorf("ReadSnapForVdev: failed to parse chunk index from key %q: %v", key, err)
			return ctlplfl.FuncError(err)
		}
		seq, err := strconv.ParseUint(c[len(c)-1], 10, 64)
		if err != nil {
			log.Errorf("ReadSnapForVdev: failed to parse chunk seq from key %q: %v", key, err)
			return ctlplfl.FuncError(err)
		}

		Snap.Chunks = append(Snap.Chunks, ctlplfl.ChunkXML{
			Idx: uint32(idx),
			Seq: seq,
		})
	}

	log.Debugf("ReadSnapForVdev: returning snap info for vdev %s with %d chunks", Snap.Vdev, len(Snap.Chunks))
	return ctlplfl.EncodeResponse(Snap)
}

func WritePrepCreateSnap(args ...interface{}) (interface{}, error) {

	Snap := args[0].(ctlplfl.SnapXML)

	commitChgs := make([]funclib.CommitChg, 0)
	for _, chunk := range Snap.Chunks {
		// Schema: {vdev}/snap/{chunk}/{Seq} : {Ref count}
		// TODO: Change the dummy ref count
		chg := funclib.CommitChg{
			Key:   []byte(fmt.Sprintf("%s/snap/%d/%d", Snap.Vdev, chunk.Idx, chunk.Seq)),
			Value: []byte{uint8(1)},
		}
		commitChgs = append(commitChgs, chg)
	}

	// Schema: snap/{name}:{blob}
	chg := funclib.CommitChg{
		Key:   []byte(fmt.Sprintf("snap/%s", Snap.SnapName)),
		Value: args[0].([]byte),
	}

	commitChgs = append(commitChgs, chg)

	//Fill the response structure
	snapResponse := ctlplfl.SnapResponseXML{
		SnapName: ctlplfl.SnapName{
			Name:    Snap.SnapName,
			Success: true,
		},
	}

	//Fill in FuncIntrm structure
	funcIntrm := funclib.FuncIntrm{
		Changes:  commitChgs,
		Response: snapResponse,
	}
	return pmCmn.Encoder(pmCmn.GOB, funcIntrm)
}

func applyKV(chgs []funclib.CommitChg, ds storageiface.DataStore) error {
	for _, chg := range chgs {
		var err error
		switch chg.Op {
		case funclib.OpApply:
			log.Debug("Applying change: ", string(chg.Key), " -> ", string(chg.Value))
			err = ds.Write(string(chg.Key), string(chg.Value), colmfamily)
		case funclib.OpDelete:
			log.Debug("Deleting key: ", string(chg.Key))
			err = ds.Delete(string(chg.Key), colmfamily)
		default:
			log.Error("Unknown operation type: ", chg.Op, " for key: ", string(chg.Key))
			return fmt.Errorf("unknown operation type %d for key %s", chg.Op, string(chg.Key))
		}
		if err != nil {
			log.Error("Failed to apply changes for key: ", string(chg.Key), " err: ", err)
			return fmt.Errorf("failed to apply changes for key: %s", string(chg.Key))
		}
	}
	return nil
}

func deleteKV(chgs []funclib.CommitChg, ds storageiface.DataStore) error {
	for _, chg := range chgs {
		log.Debug("Deleting key: ", string(chg.Key))
		err := ds.Delete(string(chg.Key), colmfamily)
		if err != nil {
			log.Fatal("Failed to apply changes for key: ", string(chg.Key))
			return fmt.Errorf("failed to delete key: %s, err: %v", string(chg.Key), err)
		}
	}
	return nil
}

func ApplyFunc(args ...interface{}) (interface{}, error) {
	cbargs := args[0].(*PumiceDBServer.PmdbCbArgs)

	var intrm funclib.FuncIntrm
	buf := C.GoBytes(cbargs.AppData, C.int(cbargs.AppDataSize))
	err := pmCmn.Decoder(pmCmn.GOB, buf, &intrm)
	if err != nil {
		log.Error("Failed to decode the apply changes: ", err)
		return ctlplfl.FuncError(fmt.Errorf("failed to decode apply changes: %v", err))
	}

	// Write-prep stored a CPResp directly (e.g. auth failure); forward it as-is.
	if cpResp, ok := intrm.Response.(ctlplfl.CPResp); ok && cpResp.Error != nil {
		log.Debugf("ApplyFunc: forwarding write-prep error response: %s", cpResp.Error.Message)
		return pmCmn.Encoder(pmCmn.GOB, cpResp)
	}

	err = applyKV(intrm.Changes, cbargs.Store)
	if err != nil {
		log.Error("applyKV(): ", err)
		return ctlplfl.FuncError(err)
	}

	resp, err := ctlplfl.EncodeResponse(intrm.Response)
	if err != nil {
		log.Error("Failed to encode response: ", err)
		return ctlplfl.FuncError(fmt.Errorf("failed to encode response: %v", err))
	}
	return resp, nil
}

func ApplyNisd(args ...interface{}) (interface{}, error) {
	// Extract callback arguments
	cbargs, ok := args[1].(*PumiceDBServer.PmdbCbArgs)
	if !ok {
		err := fmt.Errorf("invalid argument: expecting type PmdbCbArgs")
		log.Errorf("ApplyNisd: %v", err)
		return ctlplfl.FuncError(err)
	}
	cpReq, ok := args[0].(ctlplfl.CPReq)
	if !ok {
		err := fmt.Errorf("invalid argument: expecting type CPReq")
		log.Errorf("ApplyNisd: %v", err)
		return ctlplfl.FuncError(err)
	}

	nisd, ok := cpReq.Payload.(ctlplfl.Nisd)
	if !ok {
		err := fmt.Errorf("invalid payload: expecting type Nisd")
		log.Errorf("ApplyNisd: %v", err)
		return ctlplfl.FuncError(err)
	}
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.ApplyNisd); err != nil {
		log.Error("ApplyNisd: RBAC authorization failed for NISD:", nisd.ID, " error:", err)
		return ctlplfl.AuthError(err)
	}

	log.Debugf("ApplyNisd: RBAC authorization successful for NISD=%s", nisd.ID)

	// Decode intermediate changes received from pmdb
	var intrm funclib.FuncIntrm
	buf := C.GoBytes(cbargs.AppData, C.int(cbargs.AppDataSize))

	err := pmCmn.Decoder(pmCmn.GOB, buf, &intrm)
	if err != nil {
		log.Error("Failed to decode the apply changes: ", err)
		return ctlplfl.FuncError(fmt.Errorf("failed to decode apply changes: %v", err))
	}

	log.Debugf("ApplyNisd: decoded intermediate changes | numChanges=%d", len(intrm.Changes))

	// Apply KV changes to the store
	err = applyKV(intrm.Changes, cbargs.Store)
	if err != nil {
		log.Error("applyKV(): ", err)
		return ctlplfl.FuncError(err)
	}

	log.Debugf("ApplyNisd: KV changes successfully applied for NISD=%s", nisd.ID)

	// Update in-memory handler/registry
	err = HR.AddNisd(&nisd)
	if err != nil {
		log.Error("ApplyNisd: AddNisd failed:", err)
		return nil, err
	}

	resp, err := ctlplfl.EncodeResponse(intrm.Response)
	if err != nil {
		log.Error("Failed to encode response: ", err)
		return ctlplfl.FuncError(fmt.Errorf("failed to encode response: %v", err))
	}
	return resp, nil
}

// TODO: This method needs to be tested
func ReadAllNisdConfigs(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.ReadAllNisdConfigs); err != nil {
		log.Errorf("ReadAllNisdConfigs: auth failure: %v", err)
		return ctlplfl.AuthError(err)
	}

	var fields []string
	if req, ok := cpReq.Payload.(ctlplfl.GetReq); ok {
		fields = req.Fields
	}

	log.Trace("fetching nisd details for key : ", NisdCfgKey)
	readResult, err := cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector:   colmfamily,
		Key:        NisdCfgKey,
		BufSize:    cbArgs.ReplySize,
		Consistent: false,
		Prefix:     NisdCfgKey,
	})
	if err != nil {
		log.Error("Range read failure ", err)
		return ctlplfl.FuncError(err)
	}
	nisdList := ParseEntities[ctlplfl.Nisd](readResult.ResultMap, NisdParser{})

	if len(fields) > 0 {
		log.Debugf("ReadAllNisdConfigs: returning %d nisd configs (projected fields)", len(nisdList))
		return ctlplfl.EncodeResponse(nisdToAvailSize(nisdList))
	}
	log.Debugf("ReadAllNisdConfigs: returning %d nisd configs", len(nisdList))
	return ctlplfl.EncodeResponse(nisdList)
}

func nisdToAvailSize(nisds []ctlplfl.Nisd) []ctlplfl.NisdListAvailSize {
	result := make([]ctlplfl.NisdListAvailSize, 0, len(nisds))
	for _, n := range nisds {
		entry := ctlplfl.NisdListAvailSize{
			ID:            n.ID,
			TotalSize:     n.TotalSize,
			AvailableSize: n.AvailableSize,
		}
		if len(n.FailureDomain) > ctlplfl.HV_IDX {
			entry.PDU = n.FailureDomain[ctlplfl.PDU_IDX]
			entry.Rack = n.FailureDomain[ctlplfl.RACK_IDX]
			entry.HV = n.FailureDomain[ctlplfl.HV_IDX]
		}
		result = append(result, entry)
	}
	return result
}

func ReadNisdConfig(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	req := cpReq.Payload.(ctlplfl.GetReq)
	if err := req.ValidateRequest(); err != nil {
		log.Errorf("ReadNisdConfig: invalid request: %v", err)
		return ctlplfl.AuthError(err)
	}

	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.ReadNisdConfig); err != nil {
		log.Errorf("ReadNisdConfig: auth failure: %v", err)
		return ctlplfl.AuthError(err)
	}
	key := getConfKey(NisdCfgKey, req.ID)
	log.Trace("fetching nisd details for key : ", key)
	rrargs := storageiface.RangeReadArgs{
		Selector:   colmfamily,
		Key:        key,
		BufSize:    cbArgs.ReplySize,
		Consistent: false,
		Prefix:     key,
	}
	readResult, err := cbArgs.Store.RangeRead(rrargs)
	if err != nil {
		log.Error("Range read failure ", err)
		return ctlplfl.FuncError(err)
	}
	nisdList := ParseEntities[ctlplfl.Nisd](readResult.ResultMap, NisdParser{})
	log.Debugf("ReadNisdConfig: returning nisd config for key %s", key)
	return ctlplfl.EncodeResponse(nisdList[0])
}

func getNisdList(cbArgs *PumiceDBServer.PmdbCbArgs) ([]ctlplfl.Nisd, error) {
	readResult, err := cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector:   colmfamily,
		Key:        NisdCfgKey,
		BufSize:    cbArgs.ReplySize,
		Consistent: false,
		Prefix:     NisdCfgKey,
	})
	if err != nil {
		log.Error("Range read failure ", err)
		return nil, err
	}
	nisdList := ParseEntities[ctlplfl.Nisd](readResult.ResultMap, NisdParser{})
	return nisdList, nil
}

func WPNisdCfg(args ...interface{}) (interface{}, error) {
	cpReq := args[0].(ctlplfl.CPReq)
	nisd := cpReq.Payload.(ctlplfl.Nisd)
	if err := nisd.Validate(); err != nil {
		log.Error("WPNisdCfg validation failed for NISD:", nisd.ID, " error:", err)
		return ctlplfl.WPFuncError(err)
	}

	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.WPNisdCfg); err != nil {
		log.Error("WPNisdCfg RBAC authorization failed for NISD:", nisd.ID, " error:", err)
		return ctlplfl.WPAuthError(err)
	}

	log.Debug("WPNisdCfg authorization successful for NISD:", nisd.ID)

	// Populate entities and generate the list of changes that need to be committed
	commitChgs := PopulateEntities[*ctlplfl.Nisd](&nisd, nisdPopulator{}, NisdCfgKey)
	log.Debug("WPNisdCfg populated entities for NISD:", nisd.ID)

	// Prepare success response
	nisdResponse := ctlplfl.ResponseXML{
		Name:    nisd.ID,
		Success: true,
	}

	funcIntrm := funclib.FuncIntrm{
		Changes:  commitChgs,
		Response: nisdResponse,
	}

	log.Debug("WPNisdCfg successfully prepared response for NISD:", nisd.ID)

	// Encode and return the final function response
	return pmCmn.Encoder(pmCmn.GOB, funcIntrm)
}

func RdDeviceInfo(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	req := cpReq.Payload.(ctlplfl.GetReq)
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.RdDeviceInfo); err != nil {
		log.Errorf("RdDeviceInfo: auth failure: %v", err)
		return ctlplfl.AuthError(err)
	}
	key := getConfKey(deviceCfgKey, req.ID)
	readResult, err := cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector:   colmfamily,
		Key:        key,
		BufSize:    cbArgs.ReplySize,
		Consistent: false,
		Prefix:     key,
	})
	if err != nil {
		log.Error("Range read failure ", err)
		return ctlplfl.FuncError(err)
	}
	deviceList := ParseEntities[ctlplfl.Device](readResult.ResultMap, deviceWithPartitionParser{})
	log.Debugf("RdDeviceInfo: returning device info for key %s", key)
	return ctlplfl.EncodeResponse(deviceList)
}

func WPDeviceInfo(args ...interface{}) (interface{}, error) {
	cpReq := args[0].(ctlplfl.CPReq)
	dev := cpReq.Payload.(ctlplfl.Device)
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.WPDeviceInfo); err != nil {
		log.Errorf("WPDeviceInfo: auth failure: %v", err)
		return ctlplfl.WPAuthError(err)
	}
	nisdResponse := ctlplfl.ResponseXML{
		Name:    dev.ID,
		Success: true,
	}

	commitChgs := PopulateEntities[*ctlplfl.Device](&dev, devicePopulator{}, deviceCfgKey)
	for _, pt := range dev.Partitions {
		ptCommits := PopulateEntities[*ctlplfl.DevicePartition](&pt, partitionPopulator{}, fmt.Sprintf("%s/%s/%s", deviceCfgKey, dev.ID, ptKey))
		commitChgs = append(commitChgs, ptCommits...)
	}

	//Fill in FuncIntrm structure
	funcIntrm := funclib.FuncIntrm{
		Changes:  commitChgs,
		Response: nisdResponse,
	}
	return pmCmn.Encoder(pmCmn.GOB, funcIntrm)
}

// Generates all the Keys and Values that needs to be inserted into VDEV key space on vdev generation
func genAllocationKV(ID, chunk string, nisd *ctlplfl.NisdVdevAlloc, i int, commitChgs *[]funclib.CommitChg) {
	vcKey := getVdevChunkKey(ID)
	nKey := fmt.Sprintf("%s/%s/%s", nisdKey, nisd.Ptr.ID, ID)

	// TODO: handle EC blocks
	*commitChgs = append(*commitChgs, funclib.CommitChg{
		Key:   []byte(fmt.Sprintf("%s/%s/R.%d", vcKey, chunk, i)),
		Value: []byte(nisd.Ptr.ID),
	})
	*commitChgs = append(*commitChgs, funclib.CommitChg{
		Key:   []byte(nKey),
		Value: []byte(fmt.Sprintf("R.%d.%s", i, chunk)),
	})
	log.Debugf("generated kv updates for chunk %s/R.%d", chunk, i)
}

// isAlphanumeric returns true if s contains only ASCII letters and digits.
func isAlphanumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// Initialize the VDEV during the write preparation stage.
// Since the VDEV ID is derived from a randomly generated UUID,
// It needs to be generated within the Write Prep Phase.
func WPCreateVdev(args ...interface{}) (interface{}, error) {
	commitChgs := make([]funclib.CommitChg, 0)
	// Decode the input buffer into structure format
	cpReq, ok := args[0].(ctlplfl.CPReq)
	if !ok {
		err := fmt.Errorf("invalid argument: expecting type CPReq")
		log.Errorf("WPCreateVdev: %v", err)
		return ctlplfl.WPFuncError(err)
	}
	req, ok := cpReq.Payload.(ctlplfl.VdevReq)
	if !ok {
		err := fmt.Errorf("invalid payload: expecting type VdevReq")
		log.Errorf("WPCreateVdev: %v", err)
		return ctlplfl.WPFuncError(err)
	}
	tc, err := validateAndAuthorizeRBAC(cpReq.Token, authz.WPCreateVdev)
	if err != nil {
		log.Errorf("WPCreateVdev: auth failure: %v", err)
		return ctlplfl.WPAuthError(err)
	}
	if len(req.Vdev.Name) > 0 && !isAlphanumeric(req.Vdev.Name) {
		return ctlplfl.WPFuncError(fmt.Errorf("vdev name %q is invalid: must be non-empty and contain only letters and digits", req.Vdev.Name))
	}

	err = req.Vdev.Init()
	if err != nil {
		log.Errorf("failed to initialize vdev: %v", err)
		return ctlplfl.WPFuncError(err)
	}
	log.Infof("initializing vdev: %+v for user: %s", req.Vdev, tc.UserID)
	key := getConfKey(vdevKey, req.Vdev.ID)
	for _, field := range []string{SIZE, NUM_CHUNKS, NUM_REPLICAS, NAME} {
		var value string
		switch field {
		case SIZE:
			value = strconv.Itoa(int(req.Vdev.Size))
		case NUM_CHUNKS:
			value = strconv.Itoa(int(req.Vdev.NumChunks))
		case NUM_REPLICAS:
			value = strconv.Itoa(int(req.Vdev.NumReplica))
		case NAME:
			value = req.Vdev.Name
		default:
			continue
		}
		commitChgs = append(commitChgs, funclib.CommitChg{
			Key:   []byte(fmt.Sprintf("%s/%s/%s", key, cfgkey, field)),
			Value: []byte(value),
		})
	}

	// Add reverse-index key so duplicate name lookups are O(1)
	reverseNameKey := fmt.Sprintf("%s/%s", vnameKey, req.Vdev.Name)
	commitChgs = append(commitChgs, funclib.CommitChg{
		Key:   []byte(reverseNameKey),
		Value: []byte(req.Vdev.ID),
	})

	// Persist the failure domain filter so it can be surfaced in metrics/dashboards.
	fdTypeName := ctlplfl.FDName(req.Filter.Type)
	commitChgs = append(commitChgs, funclib.CommitChg{
		Key:   fmt.Appendf(nil, "%s/%s/%s", key, cfgkey, FD_TYPE_KEY),
		Value: []byte(fdTypeName),
	})
	if req.Filter.ID != "" {
		commitChgs = append(commitChgs, funclib.CommitChg{
			Key:   fmt.Appendf(nil, "%s/%s/%s", key, cfgkey, FD_ID_KEY),
			Value: []byte(req.Filter.ID),
		})
	}

	// Add ownership key to mark user as owner of this vdev
	ownershipKey := fmt.Sprintf("/u/%s/v/%s", tc.UserID, req.Vdev.ID)
	commitChgs = append(commitChgs, funclib.CommitChg{
		Key:   []byte(ownershipKey),
		Value: []byte("1"),
	})
	log.Infof("added ownership key: %s for vdev: %s", ownershipKey, req.Vdev.ID)

	if req.Vdev.PFSID != "" {
		commitChgs = append(commitChgs, funclib.CommitChg{
			Key:   []byte(fmt.Sprintf("%s/%s/pfs", key, cfgkey)),
			Value: []byte(req.Vdev.PFSID),
		})
		commitChgs = append(commitChgs, funclib.CommitChg{
			Key: []byte(fmt.Sprintf("%s/%s/v/%s", pfsKey, req.Vdev.PFSID, req.Vdev.ID)),
		})
	}
	if req.Vdev.Name != "" {
		// Add ownership key to mark user as owner of this vdev name
		ownershipKeybyName := fmt.Sprintf("/u/%s/vname/%s", tc.UserID, req.Vdev.Name)
		commitChgs = append(commitChgs, funclib.CommitChg{
			Key:   []byte(ownershipKeybyName),
			Value: []byte("1"),
		})
	}

	//Fill in FuncIntrm structure
	funcIntrm := funclib.FuncIntrm{
		Changes: commitChgs,
		Response: ctlplfl.ResponseXML{
			ID: req.Vdev.ID,
		},
	}
	return pmCmn.Encoder(pmCmn.GOB, funcIntrm)
}

func allocateNisdPerChunk(req *ctlplfl.VdevReq, fd int, chunkIdx int,
	commitChgs *[]funclib.CommitChg,
	nisdMap *btree.Map[string, *ctlplfl.NisdVdevAlloc],
	deviceUsage map[string]*DeviceUsageInfo,
	offset int) error {

	if fd < 0 || fd >= ctlplfl.FD_MAX {
		return fmt.Errorf("invalid failure domain: %d", fd)
	}
	chunk := strconv.Itoa(chunkIdx)
	pickedNISD := make(map[string]struct{})
	pickedDevices := make(map[string]struct{})
	pickedEntity := make(map[int]struct{})

	tree := HR.FD[fd].Tree
	treeLen := tree.Len()
	if treeLen == 0 {
		return fmt.Errorf("no entities available in failure domain %d", fd)
	}

	// Filtered allocation path
	if req.Filter.ID != "" {
		en, err := GetEntityByID(req.Filter)
		if err != nil {
			return err
		}

		for r := 0; r < int(req.Vdev.NumReplica); r++ {
			devOffset := chunkIdx + offset + r
			dev, err := HR.PickDevice(en, pickedDevices, deviceUsage, devOffset)
			if err != nil {
				log.Error("PickDevice(): failed to pick device: ", err)
				return err
			}
			nisd, err := HR.PickNISDFromDevice(dev, pickedNISD, nisdMap)
			if err != nil {
				log.Error("PickNISDFromDevice(): failed to pick NISD: ", err)
				return err
			}
			genAllocationKV(req.Vdev.ID, chunk, nisd, r, commitChgs)
			pickedDevices[dev.ID] = struct{}{}
			info := deviceUsage[dev.ID]
			if info == nil {
				info = &DeviceUsageInfo{AvailableSize: dev.AvailableSize}
				deviceUsage[dev.ID] = info
			}
			info.Usage++
			info.AvailableSize -= ctlplfl.CHUNK_SIZE
		}
		return nil
	}

	// Deterministic entity start index (provides cross-vdev diversity)
	hash := ctlplfl.NisdAllocHash([]byte(req.Vdev.ID + chunk))
	startIdx, err := GetIdxForNisdAlloc(hash, treeLen)
	if err != nil {
		return err
	}

	entityIdx := startIdx

	for r := 0; r < int(req.Vdev.NumReplica); r++ {
		var (
			nisd    *ctlplfl.NisdVdevAlloc
			picked        = -1
			lastErr error = fmt.Errorf("no entity attempted")
			tried         = 0
		)

		for tried < treeLen {
			curIdx := entityIdx
			entityIdx = (entityIdx + 1) % treeLen

			// do not count skipped entities as attempts
			if _, used := pickedEntity[curIdx]; used {
				continue
			}

			tried++

			ent, ok := tree.GetAt(curIdx)
			if !ok {
				lastErr = fmt.Errorf("failed to fetch entity idx=%d fd=%d", curIdx, fd)
				continue
			}

			// Phase 1: Pick optimal device within this entity
			devOffset := chunkIdx + offset + r
			dev, devErr := HR.PickDevice(ent, pickedDevices, deviceUsage, devOffset)
			if devErr != nil {
				lastErr = devErr
				continue
			}

			// Phase 2: Pick optimal NISD within the chosen device
			nisd, lastErr = HR.PickNISDFromDevice(dev, pickedNISD, nisdMap)
			if lastErr == nil {
				picked = curIdx
				pickedDevices[dev.ID] = struct{}{}
				info := deviceUsage[dev.ID]
				if info == nil {
					info = &DeviceUsageInfo{AvailableSize: dev.AvailableSize}
					deviceUsage[dev.ID] = info
				}
				info.Usage++
				info.AvailableSize -= ctlplfl.CHUNK_SIZE
				break
			}
		}

		if picked == -1 {
			return fmt.Errorf(
				"failed to allocate replica %d for chunk=%s fd=%d after %d entities (startIdx=%d): lastErr=%v",
				r, chunk, fd, treeLen, startIdx, lastErr,
			)
		}

		genAllocationKV(req.Vdev.ID, chunk, nisd, r, commitChgs)
		pickedEntity[picked] = struct{}{}
	}

	return nil
}

func allocateNisdPerVdev(req *ctlplfl.VdevReq, fd int, nisdMap *btree.Map[string, *ctlplfl.NisdVdevAlloc], offset int) ([]funclib.CommitChg, error) {
	commitCh := make([]funclib.CommitChg, 0)
	// Track device usage across all chunks in this vdev
	deviceUsage := make(map[string]*DeviceUsageInfo)

	for i := 0; i < int(req.Vdev.NumChunks); i++ {
		log.Debugf("allocating nisd for chunk: %d, from fd: %d ", i, fd)
		err := allocateNisdPerChunk(req, fd, i, &commitCh, nisdMap, deviceUsage, offset)
		if err != nil {
			err = fmt.Errorf("failed to allocate nisd from fd: %d, %v", fd, err)
			log.Error(err)
			return nil, err
		}
	}
	return commitCh, nil
}

func AllocNISDs(req *ctlplfl.VdevReq, allocMap *btree.Map[string, *ctlplfl.NisdVdevAlloc], txn *funclib.FuncIntrm, offset int) error {

	if req.Filter.Type != ctlplfl.FD_ANY {
		return allocateNisdsAtFailureDomain(req, ctlplfl.GetFDIdx(req.Filter.Type), allocMap, txn, offset)
	}

	startFD, err := HR.GetFDLevel(int(req.Vdev.NumReplica))
	if err != nil {
		return fmt.Errorf("failed to resolve failure domain: %w", err)
	}

	for fd := startFD; fd < ctlplfl.FD_MAX; fd++ {
		if err := allocateNisdsAtFailureDomain(req, fd, allocMap, txn, offset); err != nil {
			log.Error("allocation failed, retrying next failure domain: ", err)
			allocMap.Clear()
			continue
		}
		return nil
	}

	return fmt.Errorf("unable to allocate NISDs for vdev %s", req.Vdev.ID)
}

func allocateNisdsAtFailureDomain(req *ctlplfl.VdevReq, fd int,
	allocMap *btree.Map[string, *ctlplfl.NisdVdevAlloc], txn *funclib.FuncIntrm, offset int) error {

	log.Debugf("selected fd %d for vdev ID: %s", fd, req.Vdev.ID)

	changes, err := allocateNisdPerVdev(req, fd, allocMap, offset)
	if err != nil {
		return err
	}

	txn.Changes = append(txn.Changes, changes...)
	return nil
}

func commitAllocChgs(txn *funclib.FuncIntrm,
	allocMap *btree.Map[string, *ctlplfl.NisdVdevAlloc],
	cbArgs *PumiceDBServer.PmdbCbArgs) error {

	allocMap.Ascend("", func(_ string, alloc *ctlplfl.NisdVdevAlloc) bool {
		txn.Changes = append(txn.Changes, funclib.CommitChg{
			Key:   []byte(fmt.Sprintf("%s/%s", getConfKey(NisdCfgKey, alloc.Ptr.ID), AVAIL_SPACE)),
			Value: []byte(strconv.Itoa(int(alloc.AvailableSize))),
		})
		return true
	})

	return applyKV(txn.Changes, cbArgs.Store)
}

func applyNISDAlloc(allocMap *btree.Map[string, *ctlplfl.NisdVdevAlloc]) {
	allocMap.Ascend("", func(_ string, alloc *ctlplfl.NisdVdevAlloc) bool {
		// Compute the delta: how much AvailableSize changed for this NISD
		delta := alloc.AvailableSize - alloc.Ptr.AvailableSize
		alloc.Ptr.AvailableSize = alloc.AvailableSize
		if alloc.Ptr.AvailableSize < ctlplfl.CHUNK_SIZE {
			log.Debugf(
				"removing nisd %s, available space %f GB below chunk size",
				alloc.Ptr.ID,
				BytesToGB(alloc.AvailableSize),
			)
			HR.DeleteNisd(alloc.Ptr)
		} else {
			// Incrementally update device available sizes
			devID := alloc.Ptr.FailureDomain[ctlplfl.DEVICE_IDX]
			for i := range HR.FD {
				for _, fdID := range alloc.Ptr.FailureDomain {
					ent, ok := HR.FD[i].Tree.Get(&Entities{ID: fdID})
					if !ok {
						continue
					}
					dn, ok := ent.Devices.Get(&ctlplfl.DeviceAlloc{ID: devID})
					if !ok {
						continue
					}
					dn.AvailableSize += delta
					break
				}
			}
		}
		return true
	})
}

// Creates a VDEV, allocates the NISD and updates the PMDB with new data
func APCreateVdev(args ...interface{}) (interface{}, error) {
	log.Info("APCreateVdev called")
	cpreq, ok1 := args[0].(ctlplfl.CPReq)
	req, ok2 := cpreq.Payload.(ctlplfl.VdevReq)
	cbArgs, ok3 := args[1].(*PumiceDBServer.PmdbCbArgs)
	if !ok1 || !ok2 || !ok3 {
		err := fmt.Errorf("invalid argument to the APCreateVdev srvctl function")
		log.Errorf("APCreateVdev: %v", err)
		return ctlplfl.InternalError(err)
	}
	// Check for duplicate vdev name before allocating any resources.
	if req.Vdev.Name != "" {
		reverseNameKey := fmt.Sprintf("%s/%s", vnameKey, req.Vdev.Name)
		existing, readErr := cbArgs.Store.Read(reverseNameKey, colmfamily)
		if readErr == nil && len(existing) > 0 {
			return ctlplfl.FuncError(fmt.Errorf("vdev with name %q already exists (ID: %s)", req.Vdev.Name, string(existing)))
		}
	}

	allocMap := btree.NewMap[string, *ctlplfl.NisdVdevAlloc](32)
	defer allocMap.Clear()
	HR.Dump()

	var intrm funclib.FuncIntrm
	buf := C.GoBytes(cbArgs.AppData, C.int(cbArgs.AppDataSize))
	err := pmCmn.Decoder(pmCmn.GOB, buf, &intrm)
	if err != nil {
		log.Error("Failed to decode the apply changes: ", err)
		return ctlplfl.FuncError(fmt.Errorf("failed to decode apply changes: %v", err))
	}
	// Write-prep stored a CPResp directly (e.g. auth failure); forward it as-is.
	if cpResp, ok := intrm.Response.(ctlplfl.CPResp); ok && cpResp.Error != nil {
		log.Debugf("APCreateVdev: forwarding write-prep error response: %s", cpResp.Error.Message)
		return pmCmn.Encoder(pmCmn.GOB, cpResp)
	}
	resp, ok4 := intrm.Response.(ctlplfl.ResponseXML)
	if !ok4 && resp.Error != "" {
		log.Errorf("APCreateVdev: invalid response type in decoded intermediate data")
		return ctlplfl.FuncError(fmt.Errorf("invalid response type"))
	}
	req.Vdev.ID = resp.ID
	req.Vdev.NumChunks = uint32(ctlplfl.Count8GBChunks(req.Vdev.Size))
	offset := 0
	if req.Vdev.PFSID != "" {
		offsetKey := fmt.Sprintf("%s/%s/offset", pfsKey, req.Vdev.PFSID)
		res, err := cbArgs.Store.Read(offsetKey, colmfamily)
		if err != nil {
			log.Errorf("APCreateVdev: failed to read offset for vdev %s: %v", req.Vdev.ID, err)
			return ctlplfl.FuncError(err)
		}
		parsed, err := strconv.Atoi(string(res))
		if err != nil {
			log.Errorf("APCreateVdev: failed to parse offset for vdev %s: %v", req.Vdev.ID, err)
			return ctlplfl.FuncError(err)
		}
		offset = parsed
	}

	if err := AllocNISDs(&req, allocMap, &intrm, offset); err != nil {
		log.Errorf("APCreateVdev: NISD allocation failed for vdev %s: %v", req.Vdev.ID, err)
		return ctlplfl.FuncError(err)
	}

	if req.Vdev.PFSID != "" {
		offset++
		intrm.Changes = append(intrm.Changes, funclib.CommitChg{
			Key:   []byte(fmt.Sprintf("%s/%s/offset", pfsKey, req.Vdev.PFSID)),
			Value: []byte(strconv.Itoa(offset)),
		})
	}

	resp.Success = true

	if err := commitAllocChgs(&intrm, allocMap, cbArgs); err != nil {
		log.Errorf("APCreateVdev: failed to commit allocation changes for vdev %s: %v", req.Vdev.ID, err)
		return ctlplfl.FuncError(err)
	}

	applyNISDAlloc(allocMap)

	log.Infof("vdev %s, request successfully processed", req.Vdev.ID)
	HR.Dump()

	return ctlplfl.EncodeResponse(resp)
}

func WPCreatePartition(args ...interface{}) (interface{}, error) {
	cpReq := args[0].(ctlplfl.CPReq)
	pt := cpReq.Payload.(ctlplfl.DevicePartition)
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.WPCreatePartition); err != nil {
		log.Errorf("WPCreatePartition: auth failure: %v", err)
		return ctlplfl.WPAuthError(err)
	}
	resp := &ctlplfl.ResponseXML{
		Name:    pt.PartitionID,
		Success: true,
	}
	commitChgs := PopulateEntities[*ctlplfl.DevicePartition](&pt, partitionPopulator{}, ptKey)
	devPTCommits := PopulateEntities[*ctlplfl.DevicePartition](&pt, partitionPopulator{}, fmt.Sprintf("%s/%s/%s", deviceCfgKey, pt.DevID, ptKey))
	commitChgs = append(commitChgs, devPTCommits...)
	funcIntrm := funclib.FuncIntrm{
		Changes:  commitChgs,
		Response: resp,
	}
	return pmCmn.Encoder(pmCmn.GOB, funcIntrm)
}

func ReadPartition(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	req := cpReq.Payload.(ctlplfl.GetReq)
	key := ptKey
	if !req.GetAll {
		key = getConfKey(ptKey, req.ID)
	}
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.ReadPartition); err != nil {
		log.Errorf("ReadPartition: auth failure: %v", err)
		return ctlplfl.AuthError(err)
	}
	readResult, err := cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      key,
		BufSize:  cbArgs.ReplySize,
		Prefix:   key,
	})
	if err != nil {
		log.Error("Range read failure ", err)
		return ctlplfl.FuncError(err)
	}
	pt := ParseEntities[ctlplfl.DevicePartition](readResult.ResultMap, ptParser{})
	log.Debugf("ReadPartition: returning %d partition(s) for key %s", len(pt), key)
	return ctlplfl.EncodeResponse(pt)
}

func WPPDUCfg(args ...interface{}) (interface{}, error) {
	cpReq := args[0].(ctlplfl.CPReq)
	pdu := cpReq.Payload.(ctlplfl.PDU)
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.WPPDUCfg); err != nil {
		log.Errorf("WPPDUCfg: auth failure: %v", err)
		return ctlplfl.WPAuthError(err)
	}
	resp := &ctlplfl.ResponseXML{
		Name:    pdu.ID,
		Success: true,
	}
	commitChgs := PopulateEntities[*ctlplfl.PDU](&pdu, pduPopulator{}, pduKey)
	funcIntrm := funclib.FuncIntrm{
		Changes:  commitChgs,
		Response: resp,
	}

	return pmCmn.Encoder(pmCmn.GOB, funcIntrm)
}

func ReadPDUCfg(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	req := cpReq.Payload.(ctlplfl.GetReq)
	key := pduKey
	if !req.GetAll {
		key = getConfKey(pduKey, req.ID)
	}
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.ReadPDUCfg); err != nil {
		log.Errorf("ReadPDUCfg: auth failure: %v", err)
		return ctlplfl.AuthError(err)
	}
	readResult, err := cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      key,
		BufSize:  cbArgs.ReplySize,
		Prefix:   key,
	})
	if err != nil {
		log.Error("Range read failure: ", err)
		return ctlplfl.FuncError(err)
	}
	pduList := ParseEntities[ctlplfl.PDU](readResult.ResultMap, pduParser{})

	log.Debugf("ReadPDUCfg: returning %d PDU config(s) for key %s", len(pduList), key)
	return ctlplfl.EncodeResponse(pduList)

}

func WPRackCfg(args ...interface{}) (interface{}, error) {
	cpReq := args[0].(ctlplfl.CPReq)
	rack := cpReq.Payload.(ctlplfl.Rack)
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.WPRackCfg); err != nil {
		log.Errorf("WPRackCfg: auth failure: %v", err)
		return ctlplfl.WPAuthError(err)
	}
	resp := &ctlplfl.ResponseXML{
		Name:    rack.ID,
		Success: true,
	}
	commitChgs := PopulateEntities[*ctlplfl.Rack](&rack, rackPopulator{}, rackKey)
	funcIntrm := funclib.FuncIntrm{
		Changes:  commitChgs,
		Response: resp,
	}

	return pmCmn.Encoder(pmCmn.GOB, funcIntrm)
}

func ReadRackCfg(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	req := cpReq.Payload.(ctlplfl.GetReq)
	key := rackKey
	if !req.GetAll {
		key = getConfKey(rackKey, req.ID)
	}
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.ReadRackCfg); err != nil {
		log.Errorf("ReadRackCfg: auth failure: %v", err)
		return ctlplfl.AuthError(err)
	}
	readResult, err := cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      key,
		BufSize:  cbArgs.ReplySize,
		Prefix:   key,
	})
	if err != nil {
		log.Error("Range read failure ", err)
		return ctlplfl.FuncError(err)
	}
	rackList := ParseEntities[ctlplfl.Rack](readResult.ResultMap, rackParser{})
	log.Debugf("ReadRackCfg: returning %d rack config(s) for key %s", len(rackList), key)
	return ctlplfl.EncodeResponse(rackList)
}

func WPHyperVisorCfg(args ...interface{}) (interface{}, error) {
	cpReq := args[0].(ctlplfl.CPReq)
	hv := cpReq.Payload.(ctlplfl.Hypervisor)
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.WPHyperVisorCfg); err != nil {
		log.Errorf("WPHyperVisorCfg: auth failure: %v", err)
		return ctlplfl.WPAuthError(err)
	}
	resp := &ctlplfl.ResponseXML{
		Name:    hv.ID,
		Success: true,
	}
	commitChgs := PopulateEntities[*ctlplfl.Hypervisor](&hv, hvPopulator{}, hvKey)
	funcIntrm := funclib.FuncIntrm{
		Changes:  commitChgs,
		Response: resp,
	}

	return pmCmn.Encoder(pmCmn.GOB, funcIntrm)

}

func ReadHyperVisorCfg(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	req := cpReq.Payload.(ctlplfl.GetReq)

	key := hvKey
	if !req.GetAll {
		key = getConfKey(hvKey, req.ID)
	}
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.ReadHyperVisorCfg); err != nil {
		log.Errorf("ReadHyperVisorCfg: auth failure: %v", err)
		return ctlplfl.AuthError(err)
	}
	readResult, err := cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      key,
		BufSize:  cbArgs.ReplySize,
		Prefix:   key,
	})
	if err != nil {
		log.Error("Range read failure ", err)
		return ctlplfl.FuncError(err)
	}

	hvList := ParseEntities[ctlplfl.Hypervisor](readResult.ResultMap, hvParser{})

	log.Debugf("ReadHyperVisorCfg: returning %d hypervisor config(s) for key %s", len(hvList), key)
	return ctlplfl.EncodeResponse(hvList)
}

func ReadAllResources(args ...any) (any, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	req := cpReq.Payload.(ctlplfl.GetResourceReq)

	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.ReadAllResources); err != nil {
		log.Errorf("ReadAllResources: auth failure: %v", err)
		return ctlplfl.AuthError(err)
	}

	resp := ctlplfl.ResourceListResp{ResourceType: req.ResourceType}

	rangeRead := func(rootKey string) (map[string][]byte, error) {
		key := rootKey
		if !req.GetAll {
			key = getConfKey(rootKey, req.ID)
		}
		result, err := cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
			Selector: colmfamily,
			Key:      key,
			BufSize:  cbArgs.ReplySize,
			Prefix:   key,
		})
		if err != nil {
			if strings.Contains(err.Error(), "Failed to lookup for key") {
				return map[string][]byte{}, nil
			}
			return nil, err
		}
		return result.ResultMap, nil
	}

	switch req.ResourceType {
	case ctlplfl.ResourceNisd:
		rm, err := rangeRead(NisdCfgKey)
		if err != nil {
			log.Errorf("ReadAllResources: range read failed for nisd: %v", err)
			return ctlplfl.FuncError(err)
		}
		resp.Nisds = ParseEntities[ctlplfl.Nisd](rm, NisdParser{})

	case ctlplfl.ResourceRack:
		rm, err := rangeRead(rackKey)
		if err != nil {
			log.Errorf("ReadAllResources: range read failed for rack: %v", err)
			return ctlplfl.FuncError(err)
		}
		resp.Racks = ParseEntities[ctlplfl.Rack](rm, rackParser{})

	case ctlplfl.ResourcePDU:
		rm, err := rangeRead(pduKey)
		if err != nil {
			log.Errorf("ReadAllResources: range read failed for pdu: %v", err)
			return ctlplfl.FuncError(err)
		}
		resp.PDUs = ParseEntities[ctlplfl.PDU](rm, pduParser{})

	case ctlplfl.ResourceHypervisor:
		rm, err := rangeRead(hvKey)
		if err != nil {
			log.Errorf("ReadAllResources: range read failed for hypervisor: %v", err)
			return ctlplfl.FuncError(err)
		}
		resp.Hypervisors = ParseEntities[ctlplfl.Hypervisor](rm, hvParser{})

	case ctlplfl.ResourceDevice:
		rm, err := rangeRead(deviceCfgKey)
		if err != nil {
			log.Errorf("ReadAllResources: range read failed for device: %v", err)
			return ctlplfl.FuncError(err)
		}
		resp.Devices = ParseEntities[ctlplfl.Device](rm, deviceWithPartitionParser{})

	case ctlplfl.ResourcePartition:
		rm, err := rangeRead(ptKey)
		if err != nil {
			log.Errorf("ReadAllResources: range read failed for partition: %v", err)
			return ctlplfl.FuncError(err)
		}
		resp.Partitions = ParseEntities[ctlplfl.DevicePartition](rm, ptParser{})

	default:
		return ctlplfl.FuncError(fmt.Errorf("unknown resource type: %q", req.ResourceType))
	}

	log.Debugf("ReadAllResources: returning result for resource type %q", req.ResourceType)
	return ctlplfl.EncodeResponse(resp)
}

func ReadVdevsInfoWithChunkMapping(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	req := cpReq.Payload.(ctlplfl.GetReq)
	tc, err := ValidateToken(cpReq.Token)
	if err != nil {
		log.Errorf("ReadVdevsInfoWithChunkMapping: token validation failed: %v", err)
		return ctlplfl.FuncError(err)
	}

	vdevuuid := req.ID
	if !isUUID(req.ID) && !req.GetAll {
		vnKey := getConfKey(vnameKey, req.ID)
		var rqResult *storageiface.RangeReadResult
		rqResult, err = cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
			Selector: colmfamily,
			Key:      vnKey,
			BufSize:  cbArgs.ReplySize,
			Prefix:   vnKey,
		})
		if err != nil {
			log.Error("RangeReadKV failure: ", err)
			return ctlplfl.FuncError(err)
		}
		for k, v := range rqResult.ResultMap {
			parts := strings.Split(strings.Trim(k, "/"), "/")
			if len(parts) == ELEMENT_KEY {
				vdevuuid = string(v)
			}
		}
	}
	if authorizer != nil {
		if req.GetAll {
			// Listing all vdevs with chunk info: admin only, same restriction as ReadAllVdevInfo
			if !authorizer.Authorize(authz.ReadAllVdevInfo, tc.UserID, []string{tc.Role}, map[string]string{}, nil, "") {
				log.Errorf("user %s with role %s not authorized to list all vdevs with chunk info", tc.UserID, tc.Role)
				return ctlplfl.AuthError(fmt.Errorf("User is not authorized"))
			}
		} else {
			// Specific vdev: verify RBAC + ABAC ownership
			attributes := map[string]string{"vdev": vdevuuid}
			if !authorizer.Authorize(authz.ReadVdevsInfoWithChunkMapping, tc.UserID, []string{tc.Role}, attributes, cbArgs.Store, colmfamily) {
				log.Errorf("user %s with role %s not authorized to read vdev %s with chunk info", tc.UserID, tc.Role, vdevuuid)
				return ctlplfl.AuthError(fmt.Errorf("User is not authorized"))
			}
		}
	}

	key := vdevKey
	if !req.GetAll {
		key = getConfKey(vdevKey, vdevuuid)
	}

	nisdResult, err := cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      NisdCfgKey,
		BufSize:  cbArgs.ReplySize,
		Prefix:   NisdCfgKey,
	})
	if err != nil {
		log.Error("Range read failure: ", err)
		return ctlplfl.FuncError(err)
	}
	// ParseEntitiesMap now returns map[string]Entity
	nisdEntityMap := ParseEntitiesMap(nisdResult.ResultMap, NisdParser{})

	readResult, err := cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      key,
		BufSize:  cbArgs.ReplySize,
		Prefix:   key,
	})
	if err != nil {
		log.Error("Range read failure: ", err)
		return ctlplfl.FuncError(err)
	}

	vdevMap := make(map[string]*ctlplfl.Vdev)
	// top-level map: vdevID -> (nisdID -> *NisdChunk)
	vdevNisdChunkMap := make(map[string]map[string]*ctlplfl.NisdChunk)

	for k, value := range readResult.ResultMap {
		parts := strings.Split(strings.Trim(k, "/"), "/")
		if len(parts) <= VDEV_CFG_C_KEY {
			continue
		}
		vdevID := parts[BASE_UUID_PREFIX]
		// expect something like: /<root>/<vdevID>/c/<chunkIndex> -> <nisdID>
		if _, ok := vdevMap[vdevID]; !ok {
			vdevMap[vdevID] = &ctlplfl.Vdev{Cfg: ctlplfl.VdevCfg{
				ID: vdevID}}
		}
		vdev := vdevMap[vdevID]
		if parts[VDEV_CFG_C_KEY] == cfgkey {
			switch parts[VDEV_ELEMENT_KEY] {
			case SIZE:
				if sz, err := strconv.ParseInt(string(value), 10, 64); err == nil {
					vdev.Cfg.Size = sz
				}
			case NUM_CHUNKS:
				if nc, err := strconv.ParseUint(string(value), 10, 32); err == nil {
					vdev.Cfg.NumChunks = uint32(nc)
				}
			case NUM_REPLICAS:
				if nr, err := strconv.ParseUint(string(value), 10, 8); err == nil {
					vdev.Cfg.NumReplica = uint8(nr)
				}
			case pfsKey:
				vdev.Cfg.PFSID = string(value)
			case NAME:
				vdev.Cfg.Name = string(value)
			}
		} else if parts[VDEV_CFG_C_KEY] == ctlplfl.MntKey {
			switch parts[VDEV_ELEMENT_KEY] {
			case ctlplfl.MOUNT_COUNTER:
				if mc, err := strconv.ParseUint(string(value), 10, 64); err == nil {
					vdev.Cfg.VdevMountInfo.MountCounter = mc
				}
			case ctlplfl.LAST_UPDATED_LTS:
				if lu, err := time.Parse(time.RFC3339, string(value)); err == nil {
					vdev.Cfg.VdevMountInfo.LastUpdatedLTS = lu
				}
			}
		} else if parts[VDEV_CFG_C_KEY] == chunkKey {

			nisdID := string(value)

			// ensure per-vdev map exists
			if _, ok := vdevNisdChunkMap[vdevID]; !ok {
				vdevNisdChunkMap[vdevID] = make(map[string]*ctlplfl.NisdChunk)
			}
			perVdevMap := vdevNisdChunkMap[vdevID]

			// if chunk index is stored in parts[3] (original code used parts[3])
			chunkIdx := -1
			if idx, err := strconv.Atoi(parts[3]); err == nil {
				chunkIdx = idx
			}

			// create nisd chunk entry for this vdev if not present
			if _, ok := perVdevMap[nisdID]; !ok {
				// lookup nisd entity from parsed nisd map
				if ent, ok := nisdEntityMap[nisdID]; ok {
					// ent is Entity (interface) created by nisdParser (pointer to ctlplfl.Nisd)
					if nisdPtr, ok := ent.(*ctlplfl.Nisd); ok {
						perVdevMap[nisdID] = &ctlplfl.NisdChunk{
							Nisd:  *nisdPtr,
							Chunk: make([]int, 0),
						}
					} else {
						// fallback: create minimal Nisd with ID only
						perVdevMap[nisdID] = &ctlplfl.NisdChunk{
							Nisd:  ctlplfl.Nisd{ID: nisdID},
							Chunk: make([]int, 0),
						}
					}
				} else {
					// fallback: create minimal Nisd with ID only
					perVdevMap[nisdID] = &ctlplfl.NisdChunk{
						Nisd:  ctlplfl.Nisd{ID: nisdID},
						Chunk: make([]int, 0),
					}
				}
			}
			if chunkIdx >= 0 {
				perVdevMap[nisdID].Chunk = append(perVdevMap[nisdID].Chunk, chunkIdx)
			}
		}
	}

	vdevList := make([]ctlplfl.Vdev, 0, len(vdevMap))
	// attach only the nisd chunks that belong to each vdev
	for vid, v := range vdevMap {
		if perVdevMap, ok := vdevNisdChunkMap[vid]; ok {
			for _, nc := range perVdevMap {
				v.NisdToChkMap = append(v.NisdToChkMap, *nc)
			}
		}
		vdevList = append(vdevList, *v)
	}

	log.Debugf("ReadVdevsInfoWithChunkMapping: returning %d vdev(s) with chunk info", len(vdevList))
	return ctlplfl.EncodeResponse(vdevList)
}

func ReadVdevInfo(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	req := cpReq.Payload.(ctlplfl.GetReq)
	err := req.ValidateRequest()
	if err != nil {
		log.Errorf("invalid request: %v", err)
		return ctlplfl.FuncError(fmt.Errorf("Invalid Request"))
	}
	tc, err := ValidateToken(cpReq.Token)
	if err != nil {
		log.Errorf("token validation failed: %v", err)
		return ctlplfl.AuthError(fmt.Errorf("Invalid Token"))
	}
	vdevID := req.ID
	if !isUUID(req.ID) {
		vnKey := getConfKey(vnameKey, req.ID)
		var rqResult *storageiface.RangeReadResult
		rqResult, err = cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
			Selector: colmfamily,
			Key:      vnKey,
			BufSize:  cbArgs.ReplySize,
			Prefix:   vnKey,
		})
		if err != nil {
			log.Error("RangeReadKV failure: ", err)
			return ctlplfl.FuncError(err)
		}
		for k, v := range rqResult.ResultMap {
			parts := strings.Split(strings.Trim(k, "/"), "/")
			if len(parts) == ELEMENT_KEY {
				vdevID = string(v)
			}
		}
	}
	if authorizer != nil {
		attributes := map[string]string{"vdev": vdevID}
		log.Infof("user attributes: %s, userid: %s, role: %s, cbargsstore: %s, colmfamily: %s", attributes, tc.UserID, tc.Role, cbArgs.Store, colmfamily)
		if !authorizer.Authorize(authz.ReadVdevInfo, tc.UserID, []string{tc.Role}, attributes, cbArgs.Store, colmfamily) {
			log.Errorf("user %s with role %s not authorized to read vdev %s", tc.UserID, tc.Role, vdevID)
			return ctlplfl.AuthError(fmt.Errorf("User is not authorized"))
		}
	}
	log.Infof("user %s authorized to read vdev %s", tc.UserID, vdevID)
	vKey := getConfKey(vdevKey, vdevID)
	var rqResult *storageiface.RangeReadResult
	rqResult, err = cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      vKey,
		BufSize:  cbArgs.ReplySize,
		Prefix:   vKey,
	})
	if err != nil {
		log.Error("RangeReadKV failure: ", err)
		return ctlplfl.FuncError(err)
	}

	vdevList := ParseEntities[ctlplfl.VdevCfg](rqResult.ResultMap, vdevParser{})
	if len(vdevList) == 0 {
		log.Errorf("vdev %s not found", req.ID)
		return ctlplfl.FuncError(fmt.Errorf("vdev not found"))
	}

	vdevInfo := vdevList[0]

	log.Debugf("returning vdev config for %s", req.ID)
	return ctlplfl.EncodeResponse(vdevInfo)
}

func WPMountVdev(args ...interface{}) (interface{}, error) {
	cpReq, ok := args[0].(ctlplfl.CPReq)
	if !ok {
		return nil, fmt.Errorf("invalid request type")
	}
	req, _ := cpReq.Payload.(ctlplfl.MountVdevRequest)
	if !ok {
		return nil, fmt.Errorf("invalid payload type")
	}

	tc, err := ValidateToken(cpReq.Token)
	if err != nil {
		log.Errorf("token validation failed: %v", err)
		return ctlplfl.WPAuthError(err)
	}

	if authorizer != nil {
		if !authorizer.CheckRBAC(authz.WPMountVdev, []string{tc.Role}) {
			log.Errorf("user %s with role %s not authorized", tc.UserID, tc.Role)
			return ctlplfl.WPAuthError(fmt.Errorf("User is not authorized"))
		}
	}

	now := time.Now()
	mountInfo := ctlplfl.VdevMountInfo{
		LastUpdatedLTS: now,
	}
	resp, err := pmCmn.Encoder(pmCmn.GOB, mountInfo)
	if err != nil {
		return ctlplfl.FuncError(fmt.Errorf("failed to encode mount info: %v", err))
	}
	funcIntrm := funclib.FuncIntrm{
		Response: resp,
	}
	log.Debugf("mount vdev request proceeding to apply phase for vdevID %v", req.VdevID)
	return pmCmn.Encoder(pmCmn.GOB, funcIntrm)
}

func APMountVdev(args ...interface{}) (interface{}, error) {
	cpReq, ok1 := args[0].(ctlplfl.CPReq)
	cbArgs, ok2 := args[1].(*PumiceDBServer.PmdbCbArgs)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("invalid arguments to APMountVdev")
	}
	req, ok := cpReq.Payload.(ctlplfl.MountVdevRequest)
	if !ok {
		return nil, fmt.Errorf("invalid payload type")
	}
	// Do we need separate token validation for APMountVdev
	tc, err := ValidateToken(cpReq.Token)
	if err != nil {
		log.Errorf("token validation failed: %v", err)
		return ctlplfl.AuthError(err)
	}
	vdevID := req.VdevID
	if !isUUID(req.VdevID) {
		vnKey := getConfKey(vnameKey, req.VdevID)
		var rqResult *storageiface.RangeReadResult
		rqResult, err = cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
			Selector: colmfamily,
			Key:      vnKey,
			BufSize:  cbArgs.ReplySize,
			Prefix:   vnKey,
		})
		if err != nil {
			log.Error("RangeReadKV failure: ", err)
			return ctlplfl.FuncError(err)
		}
		for k, v := range rqResult.ResultMap {
			parts := strings.Split(strings.Trim(k, "/"), "/")
			if len(parts) == ELEMENT_KEY {
				vdevID = string(v)
			}
		}
	}
	if authorizer != nil {
		attributes := map[string]string{"vdev": vdevID}
		if !authorizer.Authorize(authz.APMountVdev, tc.UserID, []string{tc.Role}, attributes, cbArgs.Store, colmfamily) {
			log.Errorf("user %s with role %s not authorized for vdev %s", tc.UserID, tc.Role, vdevID)
			return ctlplfl.AuthError(fmt.Errorf("User is not authorized"))
		}
	}

	var intrm funclib.FuncIntrm
	appData := C.GoBytes(cbArgs.AppData, C.int(cbArgs.AppDataSize))
	if err := pmCmn.Decoder(pmCmn.GOB, appData, &intrm); err != nil {
		log.Errorf("failed to decode intermediate data: %v", err)
		return ctlplfl.FuncError(err)
	}
	mntResp := &ctlplfl.VdevMountInfo{}
	if err := pmCmn.Decoder(pmCmn.GOB, intrm.Response.([]byte), mntResp); err != nil {
		log.Errorf("failed to decode mount response: %v", err)
		return ctlplfl.FuncError(err)
	}
	currentTime := mntResp.LastUpdatedLTS

	mcKey := fmt.Sprintf("%s/%s/%s/%s", vdevKey, vdevID, ctlplfl.MntKey, ctlplfl.MOUNT_COUNTER)
	luKey := fmt.Sprintf("%s/%s/%s/%s", vdevKey, vdevID, ctlplfl.MntKey, ctlplfl.LAST_UPDATED_LTS)
	// TODO add mount keys as well

	var counter uint64 = 0
	var lastUpdated time.Time
	// Fetch current state using RangeRead
	vdevKey := fmt.Sprintf("%s/%s/", vdevKey, vdevID)
	mntStateRR, err := cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      vdevKey,
		BufSize:  cbArgs.ReplySize,
		Prefix:   vdevKey,
	})
	if err != nil {
		log.Errorf("failed to read mount state for vdev %s: %v", vdevID, err)
		return ctlplfl.FuncError(err)
	}

	if len(mntStateRR.ResultMap) > 0 {
		if val, ok := mntStateRR.ResultMap[mcKey]; ok {
			counter, _ = strconv.ParseUint(string(val), 10, 64)
		}
		if val, ok := mntStateRR.ResultMap[luKey]; ok {
			lastUpdated, _ = time.Parse(time.RFC3339, string(val))
		}

		if currentTime.Sub(lastUpdated) < ctlplfl.VDEV_MOUNT_LIMIT {
			log.Errorf("vdev %s mount request within active window", vdevID)
			return ctlplfl.FuncError(fmt.Errorf("mount request within active window"))
		}
		counter++
	}

	// Generate Token
	authtc := &auth.Token{
		Secret: []byte(ctlplfl.NISD_SECRET),
		TTL:    time.Minute * 30, // Mount tokens last longer
	}
	claims := map[string]any{
		"vi": vdevID,
		"mc": counter,
		"ia": currentTime.Format(time.RFC3339),
	}
	token, err := authtc.CreateToken(claims)
	if err != nil {
		log.Errorf("token creation failed: %v", err)
		return ctlplfl.FuncError(err)
	}

	log.Debugf("vdev token gen successful with vdev %v, mc %v, time %v, token %v", vdevID, counter, currentTime, token)

	vdevList := ParseEntities[ctlplfl.VdevCfg](mntStateRR.ResultMap, vdevParser{})
	if len(vdevList) == 0 {
		log.Errorf("vdev %s not found", req.VdevID)
		return ctlplfl.FuncError(fmt.Errorf("vdev not found"))
	}
	vdevCfg := vdevList[0]

	log.Debugf("vdev cfg parsing succesfull %s", vdevID)

	// Commit changes
	commitChgs := []funclib.CommitChg{
		{Key: []byte(mcKey), Value: []byte(strconv.FormatUint(counter, 10))},
		{Key: []byte(luKey), Value: []byte(currentTime.Format(time.RFC3339))},
	}
	if err := applyKV(commitChgs, cbArgs.Store); err != nil {
		log.Errorf("failed to commit state: %v", err)
		return ctlplfl.FuncError(err)
	}

	vdevCfg.VdevMountInfo.MountCounter = counter
	vdevCfg.VdevMountInfo.LastUpdatedLTS = currentTime
	vdevCfg.AccessToken = token

	log.Debugf("vdev %s mounted successfully, counter=%d", vdevID, counter)
	return ctlplfl.EncodeResponse(vdevCfg)
}

func ReadAllVdevInfo(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	rqResult, err := cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      vdevKey,
		BufSize:  cbArgs.ReplySize,
		Prefix:   vdevKey,
	})
	if err != nil {
		log.Error("RangeReadKV failure: ", err)
		return ctlplfl.FuncError(err)
	}

	// TODO: move this to parsing file
	vdevList := ParseEntities[ctlplfl.VdevCfg](rqResult.ResultMap, vdevParser{})
	log.Debugf("ReadAllVdevInfo: returning %d vdev(s)", len(vdevList))
	return ctlplfl.EncodeResponse(vdevList)
}

func ReadChunkNisd(args ...interface{}) (interface{}, error) {
	cbargs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	req := cpReq.Payload.(ctlplfl.GetReq)

	if err := req.ValidateRequest(); err != nil {
		log.Error("failed to validate request:", err)
		return ctlplfl.FuncError(err)
	}
	tc, err := ValidateToken(cpReq.Token)
	if err != nil {
		log.Errorf("ReadChunkNisd: token validation failed: %v", err)
		return ctlplfl.AuthError(err)
	}
	keys := strings.Split(strings.Trim(req.ID, "/"), "/")
	if len(keys) < 2 {
		log.Errorf("invalid request ID format %q: expected vdevID/chunkIndex", req.ID)
		return ctlplfl.FuncError(fmt.Errorf("invalid request ID format: expected vdevID/chunkIndex"))
	}
	vdevID, chunk := keys[0], keys[1]

	// Check authorization: ownership of the vdev implies access to its chunks
	if authorizer != nil {
		attributes := map[string]string{"vdev": vdevID}
		if !authorizer.Authorize(authz.ReadChunkNisd, tc.UserID, []string{tc.Role}, attributes, cbargs.Store, colmfamily) {
			log.Errorf("user %s with role %s not authorized to read chunk nisd for vdev %s", tc.UserID, tc.Role, vdevID)
			return ctlplfl.AuthError(fmt.Errorf("authorization failed"))
		}
	}

	vcKey := path.Clean(getConfKey(vdevKey, path.Join(vdevID, chunkKey, chunk))) + "/"

	log.Info("searching for key:", vcKey)

	rqResult, err := cbargs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      vcKey,
		BufSize:  cbargs.ReplySize,
		Prefix:   vcKey,
	})
	if err != nil {
		log.Error("RangeReadKV failure: ", err)
		return ctlplfl.FuncError(err)
	}

	var ids []string
	for _, v := range rqResult.ResultMap {
		ids = append(ids, string(v))
	}
	chunkInfo := ctlplfl.ChunkNisd{
		NisdUUIDs:   strings.Join(ids, ","),
		NumReplicas: uint8(len(rqResult.ResultMap)),
	}

	log.Debugf("ReadChunkNisd: returning chunk-nisd info for vdev %s chunk %s", vdevID, chunk)
	return ctlplfl.EncodeResponse(chunkInfo)

}

func WPNisdArgs(args ...interface{}) (interface{}, error) {
	cpReq := args[0].(ctlplfl.CPReq)
	nArgs := cpReq.Payload.(ctlplfl.NisdArgs)
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.WPNisdArgs); err != nil {
		log.Errorf("WPNisdArgs: auth failure: %v", err)
		return ctlplfl.WPAuthError(err)
	}
	resp := &ctlplfl.ResponseXML{
		Name:    "nisd-args",
		Success: true,
	}
	commitChgs := PopulateEntities[*ctlplfl.NisdArgs](&nArgs, nisdArgsPopulator{}, argsKey)
	funcIntrm := funclib.FuncIntrm{
		Changes:  commitChgs,
		Response: resp,
	}

	return pmCmn.Encoder(pmCmn.GOB, funcIntrm)
}

func RdNisdArgs(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.RdNisdArgs); err != nil {
		log.Errorf("RdNisdArgs: auth failure: %v", err)
		return ctlplfl.FuncError(err)
	}
	readResult, err := cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      argsKey,
		BufSize:  cbArgs.ReplySize,
		Prefix:   argsKey,
	})
	if err != nil {
		log.Error("Range read failure: ", err)
		return ctlplfl.FuncError(err)
	}
	var nisdArgs ctlplfl.NisdArgs
	for k, v := range readResult.ResultMap {
		parts := strings.Split(strings.Trim(k, "/"), "/")
		if len(parts) < 2 {
			continue
		}
		switch parts[BASE_UUID_PREFIX] {
		case DEFRAG:
			nisdArgs.Defrag, _ = strconv.ParseBool(string(v))
		case MBCCnt:
			nisdArgs.MBCCnt, _ = strconv.Atoi(string(v))
		case MergeHCnt:
			nisdArgs.MergeHCnt, _ = strconv.Atoi(string(v))
		case MCIReadCache:
			nisdArgs.MCIBReadCache, _ = strconv.Atoi(string(v))
		case DSYNC:
			nisdArgs.DSync = string(v)
		case S3:
			nisdArgs.S3 = string(v)
		case ALLOW_DEFRAG_MCIB_CACHE:
			nisdArgs.AllowDefragMCIBCache, _ = strconv.ParseBool(string(v))
		}
	}
	log.Debugf("RdNisdArgs: returning nisd args")
	return ctlplfl.EncodeResponse(nisdArgs)

}

// Deletes a Vdev, archives its data and refunds allocated space to NISDs.
// The caller must supply a valid JWT in req.UserToken; the WritePrep stage
// validates the token and checks RBAC permissions before any data is modified.
func APDeleteVdev(args ...interface{}) (interface{}, error) {
	cbArgs := args[1].(*PumiceDBServer.PmdbCbArgs)
	cpreq, ok1 := args[0].(ctlplfl.CPReq)
	if !ok1 {
		return nil, fmt.Errorf("invalid request type")
	}
	req, ok2 := cpreq.Payload.(ctlplfl.DeleteVdevReq)
	if !ok2 {
		return nil, fmt.Errorf("invalid request type")
	}

	err := req.Validate()
	if err != nil {
		log.Errorf("APDeleteVdev: invalid request for vdev %q: %v", req.ID, err)
		return ctlplfl.FuncError(err)
	}

	// Step 2: Authenticate the caller and verify RBAC permissions.
	// validateAndAuthorizeRBAC verifies the JWT token and checks that the
	// caller's role is allowed to perform "APDeleteVdev".
	tc, err := validateAndAuthorizeRBAC(cpreq.Token, authz.APDeleteVdev)
	if err != nil {
		log.Errorf("APDeleteVdev: RBAC authorization failed for vdev %q: %v", req.ID, err)
		return ctlplfl.FuncError(err)
	}

	resp := ctlplfl.ResponseXML{
		Name:    "vdev",
		ID:      req.ID,
		Success: false,
	}

	commitChgs := make([]funclib.CommitChg, 0)
	deleteChgs := make([]funclib.CommitChg, 0)
	nisdRefundMap := make(map[string]*ctlplfl.Nisd)
	var vdevName, vdevPFSID string

	log.Infof("APDeleteVdev: deleting vdev %q", req.ID)
	// Validate Vdev exists
	vdevKey := getConfKey(vdevKey, req.ID) // "v/<ID>"

	rrArgs := storageiface.RangeReadArgs{
		Key:      vdevKey,
		Prefix:   vdevKey,
		BufSize:  cbArgs.ReplySize,
		Selector: colmfamily,
	}
	for {
		rrOp, err := cbArgs.Store.RangeRead(rrArgs)
		if err != nil {
			log.Error("Range read failure ", err)
			resp.Error = err.Error()
			return ctlplfl.FuncError(err)
		}

		// Process Vdev keys
		for k, v := range rrOp.ResultMap {
			// Capture vdev name so we can remove the reverse-index entry.
			if strings.HasSuffix(k, "/"+cfgkey+"/"+NAME) {
				vdevName = string(v)
			}
			// Capture PFSID so we can remove the pfs-vdev reverse-index entry.
			if strings.HasSuffix(k, "/"+cfgkey+"/"+pfsKey) {
				vdevPFSID = string(v)
			}
			// Delete
			deleteChgs = append(deleteChgs, funclib.CommitChg{
				Key:   []byte(k),
				Value: nil,
			})

			// Check if it's a chunk allocation key
			// Key format: v/<ID>/c/<chunk>/R.<Replica> -> <NISD_ID>
			if strings.Contains(k, "/c/") && strings.Contains(k, "/R.") {
				nisdID := string(v)

				if _, ok := nisdRefundMap[nisdID]; !ok {
					key := getConfKey(NisdCfgKey, nisdID)
					rrargs := storageiface.RangeReadArgs{
						Selector:   colmfamily,
						Key:        key,
						BufSize:    cbArgs.ReplySize,
						Consistent: false,
						Prefix:     key,
					}
					nisdRR, err := cbArgs.Store.RangeRead(rrargs)
					if err != nil {
						log.Error("Range read failure ", err)
						return ctlplfl.FuncError(err)
					}
					nisdList := ParseEntities[ctlplfl.Nisd](nisdRR.ResultMap, NisdParser{})
					if len(nisdList) > 0 {
						nisdRefundMap[nisdID] = &nisdList[0]
					}
				}

				if nisd, ok := nisdRefundMap[nisdID]; ok {
					nisd.AvailableSize += ctlplfl.CHUNK_SIZE
				}

				revKey := fmt.Sprintf("%s/%s/%s", nisdKey, nisdID, req.ID)
				// delete nisd-vdev reverse mapping keys
				deleteChgs = append(deleteChgs, funclib.CommitChg{
					Key:   []byte(revKey),
					Value: nil,
				})
			}
		}
		if rrOp.LastKey == "" {
			break
		}
		// Advance to the next page
		rrArgs.Key = rrOp.LastKey
		rrArgs.SeqNum = rrOp.SeqNum
	}
	ownershipKey := fmt.Sprintf("/u/%s/v/%s", tc.UserID, req.ID)
	deleteChgs = append(deleteChgs, funclib.CommitChg{
		Key: []byte(ownershipKey),
	})

	// Process NISD refunds
	for nisdID, nisd := range nisdRefundMap {
		asKey := fmt.Sprintf("%s/%s", getConfKey(NisdCfgKey, nisdID), AVAIL_SPACE)
		commitChgs = append(commitChgs, funclib.CommitChg{
			Key:   []byte(asKey),
			Value: []byte(strconv.FormatInt(nisd.AvailableSize, 10)),
		})
	}

	// Remove vdev name reverse-index so the name can be reused.
	if vdevName != "" {
		deleteChgs = append(deleteChgs, funclib.CommitChg{
			Key:   []byte(fmt.Sprintf("%s/%s", vnameKey, vdevName)),
			Value: nil,
		})
	}

	// Remove pfs-vdev reverse-index entry.
	if vdevPFSID != "" {
		deleteChgs = append(deleteChgs, funclib.CommitChg{
			Key:   []byte(fmt.Sprintf("%s/%s/v/%s", pfsKey, vdevPFSID, req.ID)),
			Value: nil,
		})
	}

	err = applyKV(commitChgs, cbArgs.Store)
	if err != nil {
		log.Error("applyKV(): ", err)
		return ctlplfl.FuncError(err)
	}

	err = deleteKV(deleteChgs, cbArgs.Store)
	if err != nil {
		log.Errorf("APDeleteVdev: deleteKV failed for vdev %s: %v", req.ID, err)
		return ctlplfl.FuncError(err)
	}

	for nisdID, nisd := range nisdRefundMap {
		hrNisd, err := HR.GetNisdByPDUID(nisd.FailureDomain[ctlplfl.PDU_IDX], nisdID)
		if err != nil {
			log.Errorf("APDeleteVdev: GetNisdByPDUID failed for nisd %s: %v", nisdID, err)
			return ctlplfl.FuncError(err)
		}
		hrNisd.AvailableSize = nisd.AvailableSize
	}
	resp.Success = true
	log.Debugf("APDeleteVdev: vdev %s deleted successfully", req.ID)
	return ctlplfl.EncodeResponse(resp)
}

func WPPFSCfg(args ...interface{}) (interface{}, error) {
	cpReq := args[0].(ctlplfl.CPReq)
	pfs := cpReq.Payload.(ctlplfl.PFS)
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.WPPFSCfg); err != nil {
		log.Errorf("WPPFSCfg: auth failure: %v", err)
		return ctlplfl.WPAuthError(err)
	}

	if pfs.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, err
		}
		pfs.ID = id.String()
	}

	resp := &ctlplfl.ResponseXML{
		ID:      pfs.ID,
		Name:    pfs.Name,
		Success: true,
	}

	offset := rand.Intn(HR.GetEntityCnt(ctlplfl.DEVICE_IDX)) + 1

	commitChgs := []funclib.CommitChg{
		{Key: []byte(fmt.Sprintf("%s/%s/nm", pfsKey, pfs.ID)), Value: []byte(pfs.Name)},
		{Key: []byte(fmt.Sprintf("%s/%s/offset", pfsKey, pfs.ID)), Value: []byte(fmt.Sprintf("%d", offset))},
	}

	funcIntrm := funclib.FuncIntrm{
		Changes:  commitChgs,
		Response: resp,
	}
	return pmCmn.Encoder(pmCmn.GOB, funcIntrm)
}

func ReadPFSCfg(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	req := cpReq.Payload.(ctlplfl.GetReq)
	key := pfsKey
	if !req.GetAll {
		key = fmt.Sprintf("%s/%s", pfsKey, req.ID)
	}
	if _, err := validateAndAuthorizeRBAC(cpReq.Token, authz.ReadPFSCfg); err != nil {
		log.Errorf("ReadPFSCfg: auth failure: %v", err)
		return ctlplfl.AuthError(err)
	}
	readResult, err := cbArgs.Store.RangeRead(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      key,
		BufSize:  cbArgs.ReplySize,
		Prefix:   key,
	})
	if err != nil {
		log.Error("Range read failure: ", err)
		return ctlplfl.FuncError(err)
	}

	pfsMap := make(map[string]*ctlplfl.PFS)
	for k, v := range readResult.ResultMap {
		parts := strings.Split(strings.Trim(k, "/"), "/")
		if len(parts) < 2 || parts[0] != pfsKey {
			continue
		}
		id := parts[1]
		if _, ok := pfsMap[id]; !ok {
			pfsMap[id] = &ctlplfl.PFS{ID: id, VdevIDs: make([]string, 0)}
		}
		if len(parts) >= 3 {
			if parts[2] == "nm" {
				pfsMap[id].Name = string(v)
			} else if parts[2] == "offset" {
				offset, err := strconv.Atoi(string(v))
				if err == nil {
					pfsMap[id].Offset = offset
				}
			} else if parts[2] == "v" && len(parts) >= 4 { // pfs/<PFSID>/v/<VdevID> -> 1
				pfsMap[id].VdevIDs = append(pfsMap[id].VdevIDs, parts[3])
			}
		}
	}

	pfsList := make([]ctlplfl.PFS, 0, len(pfsMap))
	for _, p := range pfsMap {
		pfsList = append(pfsList, *p)
	}
	return ctlplfl.EncodeResponse(pfsList)
}
