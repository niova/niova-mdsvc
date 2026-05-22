package srvctlplanefuncs

import (
	"C"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

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

	cfgkey       = "cfg"
	NisdCfgKey   = "n_cfg"
	deviceCfgKey = "d_cfg"
	sdeviceKey   = "sd"
	parentInfo   = "pi"
	pduKey       = "p"
	rackKey      = "r"
	nisdKey      = "n"
	vdevKey      = "v"
	chunkKey     = "c"
	hvKey        = "hv"
	ptKey        = "pt"
	argsKey      = "na"
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

// GetRangeReadArgs extracts pagination state from cpReq.Page and returns a
// ready-to-use RangeReadArgs plus the lastKey for the parser.
func GetRangeReadArgs(page *ctlplfl.Pagination, baseKey, selector string, bufSize int64) (storageiface.RangeReadArgs, string) {
	var lastKey string
	var seqNo uint64
	var consistent bool

	if page != nil {
		seqNo, lastKey = page.GetTokenData()
		consistent = page.Consistent
	}

	rrargs := storageiface.RangeReadArgs{
		Selector:   selector,
		Key:        baseKey,
		Prefix:     baseKey,
		BufSize:    bufSize,
		SeqNum:     seqNo,
		Consistent: consistent,
	}

	if lastKey != "" {
		rrargs.Key = lastKey
	}

	return rrargs, lastKey
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
	itr, err := cbArgs.Store.NewRangeIterator(rrargs)
	if err != nil {
		log.Error("Range read failure ", err)
		return ctlplfl.FuncError(err)
	}
	defer itr.Close()

	log.Tracef("ReadSnapForVdev: range read for vdev %s", Snap.Vdev)
	for itr.Valid() {
		key, _ := itr.GetKV()
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
		itr.Next()
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
	log.Trace("fetching nisd details for key : ", NisdCfgKey)

	rrargs, lastKey := GetRangeReadArgs(cpReq.Page, NisdCfgKey, colmfamily, cbArgs.ReplySize)
	itr, err := cbArgs.Store.NewRangeIterator(rrargs)
	if err != nil {
		log.Error("Range read failure ", err)
		return ctlplfl.FuncError(err)
	}
	defer itr.Close()

	nisdList, nextKey := ParseEntitiesPaginated[ctlplfl.Nisd](itr, NisdParser{}, lastKey)
	log.Debugf("ReadAllNisdConfigs: returning %d nisd configs", len(nisdList))
	return ctlplfl.EncodePagedResponse(nisdList, nextKey, itr.GetSeqNum())
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
	itr, err := cbArgs.Store.NewRangeIterator(rrargs)
	if err != nil {
		log.Error("Range read failure ", err)
		return ctlplfl.FuncError(err)
	}
	defer itr.Close()

	nisdList, _ := ParseEntitiesPaginated[ctlplfl.Nisd](itr, NisdParser{}, "")
	log.Debugf("ReadNisdConfig: returning nisd config for key %s", key)
	return ctlplfl.EncodeResponse(nisdList[0])
}

func getNisdList(cbArgs *PumiceDBServer.PmdbCbArgs) ([]ctlplfl.Nisd, error) {
	itr, err := cbArgs.Store.NewRangeIterator(storageiface.RangeReadArgs{
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
	defer itr.Close()

	nisdList, _ := ParseEntitiesPaginated[ctlplfl.Nisd](itr, NisdParser{}, "")
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

	rrargs, lastKey := GetRangeReadArgs(cpReq.Page, key, colmfamily, cbArgs.ReplySize)
	itr, err := cbArgs.Store.NewRangeIterator(rrargs)
	if err != nil {
		log.Error("Range read failure ", err)
		return ctlplfl.FuncError(err)
	}
	defer itr.Close()

	deviceList, nextKey := ParseEntitiesPaginated[ctlplfl.Device](itr, deviceWithPartitionParser{}, lastKey)
	log.Debugf("RdDeviceInfo: returning device info for key %s", key)
	return ctlplfl.EncodePagedResponse(deviceList, nextKey, itr.GetSeqNum())
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

	err = req.Vdev.Init()
	if err != nil {
		log.Errorf("failed to initialize vdev: %v", err)
		return ctlplfl.WPFuncError(err)
	}
	log.Infof("initializing vdev: %+v for user: %s", req.Vdev, tc.UserID)
	key := getConfKey(vdevKey, req.Vdev.ID)
	for _, field := range []string{SIZE, NUM_CHUNKS, NUM_REPLICAS} {
		var value string
		switch field {
		case SIZE:
			value = strconv.Itoa(int(req.Vdev.Size))
		case NUM_CHUNKS:
			value = strconv.Itoa(int(req.Vdev.NumChunks))
		case NUM_REPLICAS:
			value = strconv.Itoa(int(req.Vdev.NumReplica))
		default:
			continue
		}
		commitChgs = append(commitChgs, funclib.CommitChg{
			Key:   []byte(fmt.Sprintf("%s/%s/%s", key, cfgkey, field)),
			Value: []byte(value),
		})
	}

	// Add ownership key to mark user as owner of this vdev
	ownershipKey := fmt.Sprintf("/u/%s/v/%s", tc.UserID, req.Vdev.ID)
	commitChgs = append(commitChgs, funclib.CommitChg{
		Key:   []byte(ownershipKey),
		Value: []byte("1"),
	})
	log.Infof("added ownership key: %s for vdev: %s", ownershipKey, req.Vdev.ID)

	//Fill in FuncIntrm structure
	funcIntrm := funclib.FuncIntrm{
		Changes: commitChgs,
		Response: ctlplfl.ResponseXML{
			ID: req.Vdev.ID,
		},
	}
	return pmCmn.Encoder(pmCmn.GOB, funcIntrm)
}

func allocateNisdPerChunk(req *ctlplfl.VdevReq, fd int, chunk string,
	commitChgs *[]funclib.CommitChg,
	nisdMap *btree.Map[string, *ctlplfl.NisdVdevAlloc]) error {

	if fd < 0 || fd >= ctlplfl.FD_MAX {
		return fmt.Errorf("invalid failure domain: %d", fd)
	}

	pickedNISD := make(map[string]struct{})
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
			nisd, err := HR.PickNISD(en, pickedNISD, nisdMap)
			if err != nil {
				log.Error("PickNISD():failed to pick NISD: ", err)
				return err
			}
			genAllocationKV(req.Vdev.ID, chunk, nisd, r, commitChgs)
		}
		return nil
	}

	// Deterministic entity start index
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

			nisd, lastErr = HR.PickNISD(ent, pickedNISD, nisdMap)
			if lastErr == nil {
				picked = curIdx
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

func allocateNisdPerVdev(req *ctlplfl.VdevReq, fd int, nisdMap *btree.Map[string, *ctlplfl.NisdVdevAlloc]) ([]funclib.CommitChg, error) {
	commitCh := make([]funclib.CommitChg, 0)
	for i := 0; i < int(req.Vdev.NumChunks); i++ {
		log.Debugf("allocating nisd for chunk: %d, from fd: %d ", i, fd)
		err := allocateNisdPerChunk(req, fd, strconv.Itoa(i), &commitCh, nisdMap)
		if err != nil {
			err = fmt.Errorf("failed to allocate nisd from fd: %d, %v", fd, err)
			log.Error(err)
			return nil, err
		}
	}
	return commitCh, nil
}

func AllocNISDs(req *ctlplfl.VdevReq, allocMap *btree.Map[string, *ctlplfl.NisdVdevAlloc], txn *funclib.FuncIntrm) error {

	if req.Filter.Type != ctlplfl.FD_ANY {
		return allocateNisdsAtFailureDomain(req, ctlplfl.GetFDIdx(req.Filter.Type), allocMap, txn)
	}

	startFD, err := HR.GetFDLevel(int(req.Vdev.NumReplica))
	if err != nil {
		return fmt.Errorf("failed to resolve failure domain: %w", err)
	}

	for fd := startFD; fd < ctlplfl.FD_MAX; fd++ {
		if err := allocateNisdsAtFailureDomain(req, fd, allocMap, txn); err != nil {
			log.Error("allocation failed, retrying next failure domain: ", err)
			allocMap.Clear()
			continue
		}
		return nil
	}

	return fmt.Errorf("unable to allocate NISDs for vdev %s", req.Vdev.ID)
}

func allocateNisdsAtFailureDomain(req *ctlplfl.VdevReq, fd int,
	allocMap *btree.Map[string, *ctlplfl.NisdVdevAlloc], txn *funclib.FuncIntrm) error {

	log.Debugf("selected fd %d for vdev ID: %s", fd, req.Vdev.ID)

	changes, err := allocateNisdPerVdev(req, fd, allocMap)
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
		alloc.Ptr.AvailableSize = alloc.AvailableSize
		if alloc.Ptr.AvailableSize < ctlplfl.CHUNK_SIZE {
			log.Debugf(
				"removing nisd %s, available space %f GB below chunk size",
				alloc.Ptr.ID,
				BytesToGB(alloc.AvailableSize),
			)
			HR.DeleteNisd(alloc.Ptr)
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
	if !ok4 {
		log.Errorf("APCreateVdev: invalid response type in decoded intermediate data")
		return ctlplfl.FuncError(fmt.Errorf("invalid response type"))
	}

	req.Vdev.ID = resp.ID
	req.Vdev.NumChunks = uint32(ctlplfl.Count8GBChunks(req.Vdev.Size))
	if err := AllocNISDs(&req, allocMap, &intrm); err != nil {
		log.Errorf("APCreateVdev: NISD allocation failed for vdev %s: %v", req.Vdev.ID, err)
		return ctlplfl.FuncError(err)
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

	rrargs, lastKey := GetRangeReadArgs(cpReq.Page, key, colmfamily, cbArgs.ReplySize)
	itr, err := cbArgs.Store.NewRangeIterator(rrargs)
	if err != nil {
		log.Error("Range read failure ", err)
		return ctlplfl.FuncError(err)
	}
	defer itr.Close()

	pt, nextKey := ParseEntitiesPaginated[ctlplfl.DevicePartition](itr, ptParser{}, lastKey)
	log.Debugf("ReadPartition: returning %d partition(s) for key %s", len(pt), key)
	return ctlplfl.EncodePagedResponse(pt, nextKey, itr.GetSeqNum())
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

	rrargs, lastKey := GetRangeReadArgs(cpReq.Page, key, colmfamily, cbArgs.ReplySize)
	itr, err := cbArgs.Store.NewRangeIterator(rrargs)
	if err != nil {
		log.Error("Range read failure: ", err)
		return ctlplfl.FuncError(err)
	}
	defer itr.Close()

	pduList, nextKey := ParseEntitiesPaginated[ctlplfl.PDU](itr, pduParser{}, lastKey)
	log.Debugf("ReadPDUCfg: returning %d PDU config(s)", len(pduList))
	return ctlplfl.EncodePagedResponse(pduList, nextKey, itr.GetSeqNum())
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

	rrargs, lastKey := GetRangeReadArgs(cpReq.Page, key, colmfamily, cbArgs.ReplySize)
	itr, err := cbArgs.Store.NewRangeIterator(rrargs)
	if err != nil {
		log.Error("Range read failure ", err)
		return ctlplfl.FuncError(err)
	}
	defer itr.Close()

	rackList, nextKey := ParseEntitiesPaginated[ctlplfl.Rack](itr, rackParser{}, lastKey)
	log.Debugf("ReadRackCfg: returning %d rack config(s)", len(rackList))
	return ctlplfl.EncodePagedResponse(rackList, nextKey, itr.GetSeqNum())
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

	rrargs, lastKey := GetRangeReadArgs(cpReq.Page, key, colmfamily, cbArgs.ReplySize)
	itr, err := cbArgs.Store.NewRangeIterator(rrargs)
	if err != nil {
		log.Error("Range read failure ", err)
		return ctlplfl.FuncError(err)
	}
	defer itr.Close()

	hvList, nextKey := ParseEntitiesPaginated[ctlplfl.Hypervisor](itr, hvParser{}, lastKey)
	log.Debugf("ReadHyperVisorCfg: returning %d hypervisor config(s)", len(hvList))
	return ctlplfl.EncodePagedResponse(hvList, nextKey, itr.GetSeqNum())
}

func ReadVdevsInfoWithChunkMapping(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	req := cpReq.Payload.(ctlplfl.GetReq)
	if req.ID == "" {
		return ctlplfl.FuncError(fmt.Errorf("Invalid request: ID is required"))
	}
	tc, err := ValidateToken(cpReq.Token)
	if err != nil {
		log.Errorf("ReadVdevsInfoWithChunkMapping: token validation failed: %v", err)
		return ctlplfl.FuncError(err)
	}
	log.Info("ReadVdevsInfoWithChunkMapping: token validation passed")
	if authorizer != nil {
		if req.GetAll {
			// Listing all vdevs with chunk info: admin only, same restriction as ReadAllVdevInfo
			if !authorizer.Authorize(authz.ReadAllVdevInfo, tc.UserID, []string{tc.Role}, map[string]string{}, nil, "") {
				log.Errorf("user %s with role %s not authorized to list all vdevs with chunk info", tc.UserID, tc.Role)
				return ctlplfl.AuthError(fmt.Errorf("User is not authorized"))
			} // ← add this closing brace
		} else {
			// Specific vdev: verify RBAC + ABAC ownership
			attributes := map[string]string{"vdev": req.ID}
			if !authorizer.Authorize(authz.ReadVdevsInfoWithChunkMapping, tc.UserID, []string{tc.Role}, attributes, cbArgs.Store, colmfamily) {
				log.Errorf("user %s with role %s not authorized to read vdev %s with chunk info", tc.UserID, tc.Role, req.ID)
				return ctlplfl.AuthError(fmt.Errorf("User is not authorized"))
			}
		}
	}
	log.Info("ReadVdevsInfoWithChunkMapping: authorization passed")

	nisdItr, err := cbArgs.Store.NewRangeIterator(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      NisdCfgKey,
		BufSize:  cbArgs.ReplySize,
		Prefix:   NisdCfgKey,
	})
	if err != nil {
		log.Error("Range read failure: ", err)
		return ctlplfl.FuncError(err)
	}
	defer nisdItr.Close()
	key := getConfKey(vdevKey, req.ID)
	log.Info("Read Vdevs :", key)
	// ParseEntitiesMap now returns map[string]Entity
	nisdEntityMap := ParseEntitiesMap(nisdItr, NisdParser{})

	vdevItr, err := cbArgs.Store.NewRangeIterator(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      key,
		BufSize:  cbArgs.ReplySize,
		Prefix:   key,
	})
	if err != nil {
		log.Error("Range read failure: ", err)
		return ctlplfl.FuncError(err)
	}
	defer vdevItr.Close()

	vdevMap := make(map[string]*ctlplfl.Vdev)
	// top-level map: vdevID -> (nisdID -> *NisdChunk)
	vdevNisdChunkMap := make(map[string]map[string]*ctlplfl.NisdChunk)

	for vdevItr.Valid() {
		k, value := vdevItr.GetKV()
		log.Info("ReadVdevsInfoWithChunkMapping: processing key ", string(k))
		parts := strings.Split(strings.Trim(k, "/"), "/")
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
				if sz, err := strconv.ParseInt(value, 10, 64); err == nil {
					vdev.Cfg.Size = sz
				}
			case NUM_CHUNKS:
				if nc, err := strconv.ParseUint(value, 10, 32); err == nil {
					vdev.Cfg.NumChunks = uint32(nc)
				}
			case NUM_REPLICAS:
				if nr, err := strconv.ParseUint(value, 10, 8); err == nil {
					vdev.Cfg.NumReplica = uint8(nr)
				}

			}

		} else if parts[VDEV_CFG_C_KEY] == chunkKey {

			nisdID := value

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
		vdevItr.Next()
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
	start := time.Now()
	log.Debug("ReadVdevInfo: entered function")

	// ---- Args parsing ----
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	req := cpReq.Payload.(ctlplfl.GetReq)

	log.Debugf("ReadVdevInfo: request received for vdevID=%s", req.ID)

	// ---- Request validation ----
	if err := req.ValidateRequest(); err != nil {
		log.Errorf("ReadVdevInfo: invalid request: %v", err)
		return ctlplfl.FuncError(fmt.Errorf("Invalid Request"))
	}
	log.Debug("ReadVdevInfo: request validation successful")

	// ---- Token validation ----
	tc, err := ValidateToken(cpReq.Token)
	if err != nil {
		log.Errorf("ReadVdevInfo: token validation failed: %v", err)
		return ctlplfl.AuthError(fmt.Errorf("Invalid Token"))
	}
	log.Debugf("ReadVdevInfo: token validated for user=%s role=%s", tc.UserID, tc.Role)

	// ---- Authorization ----
	if authorizer != nil {
		attributes := map[string]string{"vdev": req.ID}
		log.Debug("ReadVdevInfo: performing authorization check")

		if !authorizer.Authorize(
			authz.ReadVdevInfo,
			tc.UserID,
			[]string{tc.Role},
			attributes,
			cbArgs.Store,
			colmfamily,
		) {
			log.Errorf("ReadVdevInfo: authorization failed for user %s role %s vdev %s",
				tc.UserID, tc.Role, req.ID)
			return ctlplfl.AuthError(fmt.Errorf("User is not authorized"))
		}
		log.Debug("ReadVdevInfo: authorization successful")
	}

	// ---- Iterator creation ----
	vKey := getConfKey(vdevKey, req.ID)
	log.Debugf("ReadVdevInfo: creating iterator for key=%s", vKey)

	iterStart := time.Now()
	vdevItr, err := cbArgs.Store.NewRangeIterator(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      vKey,
		BufSize:  cbArgs.ReplySize,
		Prefix:   vKey,
	})
	if err != nil {
		log.Error("ReadVdevInfo: RangeReadKV failure: ", err)
		return ctlplfl.FuncError(err)
	}
	log.Debugf("ReadVdevInfo: iterator created in %v", time.Since(iterStart))
	defer func() {
		log.Debug("ReadVdevInfo: closing iterator")
		vdevItr.Close()
	}()

	// ---- Token creation ----
	log.Debug("ReadVdevInfo: creating auth token")
	authtc := &auth.Token{
		Secret: []byte(ctlplfl.NISD_SECRET),
		TTL:    time.Minute,
	}

	claims := map[string]any{
		"vdevID": req.ID,
	}

	tokenStart := time.Now()
	authtoken, err := authtc.CreateToken(claims)
	if err != nil {
		log.Error("ReadVdevInfo: token creation failed: ", err)
		return ctlplfl.FuncError(err)
	}
	log.Debugf("ReadVdevInfo: token created in %v", time.Since(tokenStart))

	// ---- Parsing ----
	log.Debug("ReadVdevInfo: parsing iterator results")

	parseStart := time.Now()
	vdevList := ParseEntities[ctlplfl.VdevCfg](vdevItr, vdevParser{})
	log.Debugf("ReadVdevInfo: parsing completed in %v, count=%d",
		time.Since(parseStart), len(vdevList))

	if len(vdevList) == 0 {
		log.Warnf("ReadVdevInfo: no vdev found for ID=%s", req.ID)
		return ctlplfl.FuncError(fmt.Errorf("vdev not found"))
	}

	vdevInfo := vdevList[0]
	vdevInfo.AuthToken = authtoken

	// ---- Response ----
	log.Debugf("ReadVdevInfo: returning response for vdevID=%s (total time=%v)",
		req.ID, time.Since(start))

	return ctlplfl.EncodeResponse(vdevInfo)
}
func ReadAllVdevInfo(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)

	rrargs, lastKey := GetRangeReadArgs(cpReq.Page, vdevKey, colmfamily, cbArgs.ReplySize)
	vdevItr, err := cbArgs.Store.NewRangeIterator(rrargs)
	if err != nil {
		log.Error("RangeReadKV failure: ", err)
		return ctlplfl.FuncError(err)
	}
	defer vdevItr.Close()

	vdevList, nextKey := ParseEntitiesPaginated[ctlplfl.VdevCfg](vdevItr, vdevParser{}, lastKey)
	log.Debugf("ReadAllVdevInfo: returning %d vdev(s)", len(vdevList))
	return ctlplfl.EncodePagedResponse(vdevList, nextKey, vdevItr.GetSeqNum())
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

	itr, err := cbargs.Store.NewRangeIterator(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      vcKey,
		BufSize:  cbargs.ReplySize,
		Prefix:   vcKey,
	})
	if err != nil {
		log.Error("RangeReadKV failure: ", err)
		return ctlplfl.FuncError(err)
	}
	defer itr.Close()

	var ids []string
	for itr.Valid() {
		ids = append(ids, string(itr.Value()))
		itr.Next()
	}
	chunkIdx, _ := strconv.Atoi(chunk)
	chunkInfo := ctlplfl.ChunkInfo{
		ChunkIdx:    chunkIdx,
		NisdUUIDs:   ids,
		NumReplicas: uint8(len(ids)),
	}

	log.Debugf("ReadChunkNisd: returning chunk-nisd info for vdev %s chunk %s", vdevID, chunk)
	return ctlplfl.EncodeResponse(chunkInfo)

}

// ReadChunksInfoPaginated performs a paginated range-read over all chunk-to-NISD
// mappings stored under a single vdev (v/<vdevID>/c/...). Each page contains up to
// 4 MB of JSON-serialised ChunkInfo objects. Pagination follows the same
// continuation-token pattern used by ReadAllVdevInfo, ReadRackCfg, etc.
func ReadChunksInfoPaginated(args ...interface{}) (interface{}, error) {
	cbArgs := args[0].(*PumiceDBServer.PmdbCbArgs)
	cpReq := args[1].(ctlplfl.CPReq)
	req := cpReq.Payload.(ctlplfl.GetReq)

	if err := req.ValidateRequest(); err != nil {
		log.Error("ReadChunksInfoPaginated: invalid request:", err)
		return ctlplfl.FuncError(err)
	}

	tc, err := ValidateToken(cpReq.Token)
	if err != nil {
		log.Errorf("ReadChunksInfoPaginated: token validation failed: %v", err)
		return ctlplfl.AuthError(err)
	}

	if authorizer != nil {
		attributes := map[string]string{"vdev": req.ID}
		if !authorizer.Authorize(authz.ReadChunksInfoPaginated, tc.UserID, []string{tc.Role},
			attributes, cbArgs.Store, colmfamily) {
			log.Errorf("ReadChunksInfoPaginated: user %s role %s not authorized for vdev %s",
				tc.UserID, tc.Role, req.ID)
			return ctlplfl.AuthError(fmt.Errorf("authorization failed"))
		}
	}

	// Range key: v/<vdevID>/c/
	chunkPrefix := path.Clean(getConfKey(vdevKey, path.Join(req.ID, chunkKey))) + "/"

	rrArgs, lastKey := GetRangeReadArgs(cpReq.Page, chunkPrefix, colmfamily, cbArgs.ReplySize)
	itr, err := cbArgs.Store.NewRangeIterator(rrArgs)
	if err != nil {
		log.Error("ReadChunksInfoPaginated: range read failure: ", err)
		return ctlplfl.FuncError(err)
	}
	defer itr.Close()

	// Use objIDIdx=3 so the paginator groups by parts[3] (chunk index)
	// rather than parts[1] (vdevID), ensuring each ChunkInfo is complete.
	chunks, nextKey := ParseEntitiesPaginated[ctlplfl.ChunkInfo](
		itr, chunkInfoParser{}, lastKey, 3)

	log.Debugf("ReadChunksInfoPaginated: vdev=%s returning %d chunks",
		req.ID, len(chunks))
	return ctlplfl.EncodePagedResponse(chunks, nextKey, itr.GetSeqNum())
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
	argsItr, err := cbArgs.Store.NewRangeIterator(storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      argsKey,
		BufSize:  cbArgs.ReplySize,
		Prefix:   argsKey,
	})
	if err != nil {
		log.Error("Range read failure: ", err)
		return ctlplfl.FuncError(err)
	}
	defer argsItr.Close()

	var nisdArgs ctlplfl.NisdArgs
	for argsItr.Valid() {
		k, v := argsItr.GetKV()
		parts := strings.Split(strings.Trim(k, "/"), "/")
		if len(parts) >= 2 {
			switch parts[BASE_UUID_PREFIX] {
			case DEFRAG:
				nisdArgs.Defrag, _ = strconv.ParseBool(v)
			case MBCCnt:
				nisdArgs.MBCCnt, _ = strconv.Atoi(v)
			case MergeHCnt:
				nisdArgs.MergeHCnt, _ = strconv.Atoi(v)
			case MCIReadCache:
				nisdArgs.MCIBReadCache, _ = strconv.Atoi(v)
			case DSYNC:
				nisdArgs.DSync = v
			case S3:
				nisdArgs.S3 = v
			case ALLOW_DEFRAG_MCIB_CACHE:
				nisdArgs.AllowDefragMCIBCache, _ = strconv.ParseBool(v)
			}
		}
		argsItr.Next()
	}
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

	log.Infof("APDeleteVdev: deleting vdev %q", req.ID)
	// Validate Vdev exists
	vdevKey := getConfKey(vdevKey, req.ID) // "v/<ID>"

	rrArgs := storageiface.RangeReadArgs{
		Selector: colmfamily,
		Key:      vdevKey,
		BufSize:  cbArgs.ReplySize,
		Prefix:   vdevKey,
	}

	rrOp, err := cbArgs.Store.NewRangeIterator(rrArgs)
	if err != nil {
		log.Error("Range read failure ", err)
		resp.Error = err.Error()
		return ctlplfl.FuncError(err)
	}
	defer rrOp.Close()
	// Process Vdev keys
	for rrOp.Valid() {
		// Delete
		k := rrOp.Key()
		v := rrOp.Value()
		log.Info("delete itr kv: ", k, v)
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
				nisdItr, err := cbArgs.Store.NewRangeIterator(rrargs)
				if err != nil {
					log.Error("Range read failure ", err)
					resp.Error = err.Error()
					return pmCmn.Encoder(pmCmn.GOB, resp)
				}
				defer nisdItr.Close()
				nisdList, _ := ParseEntitiesPaginated[ctlplfl.Nisd](nisdItr, NisdParser{}, "")
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
		rrOp.Next()
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
