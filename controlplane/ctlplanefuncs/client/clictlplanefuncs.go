package clictlplanefuncs

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	log "github.com/00pauln00/niova-lookout/pkg/xlog"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	restapi "github.com/00pauln00/niova-mdsvc/controlplane/restapi"

	pmCmn "github.com/00pauln00/niova-pumicedb/go/pkg/pumicecommon"
	sd "github.com/00pauln00/niova-pumicedb/go/pkg/utils/servicediscovery"
)

const (
	LOG_LEVEL = "Info"
)

// Client side interface for control plane functions
type CliCFuncs struct {
	appUUID  string
	writeSeq atomic.Uint64
	sdObj    *sd.ServiceDiscoveryHandler
	encType  pmCmn.Format
	token    string // Auth JWT token for CPReq
}

func InitCliCFuncs(appUUID string, key string, gossipConfigPath string, logPath string) *CliCFuncs {
	ccf := CliCFuncs{
		appUUID: appUUID,
		encType: pmCmn.JSON, // Default encoding type
	}

	ccf.sdObj = &sd.ServiceDiscoveryHandler{
		HTTPRetry: 10,
		SerfRetry: 5,
		RaftUUID:  key,
	}
	stop := make(chan int)
	logL := LOG_LEVEL
	if logPath == "" {
		logPath = ctlplfl.DefaultLogPath()
	}
	log.InitXlog(logPath, &logL)
	log.Info("Staring Client API using gossip path: ", gossipConfigPath)
	go func() {
		err := ccf.sdObj.StartClientAPI(stop, gossipConfigPath)
		if err != nil {
			log.Fatal("Error while starting client API : ", err)
		}
	}()
	log.Info("Successfully initialized controlplane client: ", appUUID)

	return &ccf
}

func (ccf *CliCFuncs) request(rqb []byte, urla string, isWrite bool) ([]byte, error) {
	ccf.sdObj.TillReady("PROXY", 5)
	rsp, err := ccf.sdObj.Request(rqb, "/func?"+urla, isWrite)
	if err != nil {
		log.Error("failed to send request to server: ", err)
		return nil, err
	}
	return rsp, nil
}

func (ccf *CliCFuncs) get(cpReq *ctlplfl.CPReq, urla string, target any) (*ctlplfl.CPResp, error) {
	url := "name=" + urla
	rqb, err := pmCmn.Encoder(ccf.encType, cpReq)
	if err != nil {
		log.Error("failed to encode data: ", err)
		return nil, err
	}

	rsb, err := ccf.request(rqb, url, false)
	if err != nil {
		log.Error("request failed: ", err)
		return nil, err
	}
	if rsb == nil {
		return nil, fmt.Errorf("failed to fetch response from control plane: %v", err)
	}

	cpResp := &ctlplfl.CPResp{
		Payload: target,
	}
	err = pmCmn.Decoder(ccf.encType, rsb, cpResp)
	if err != nil {
		log.Error("failed to decode response in get: ", err)
		return nil, err
	}
	return cpResp, nil
}

// ---- REST transport for endpoints migrated to the proxy's REST API ----
//
// These helpers talk to the proxy's REST routes (e.g. /vdev, /create_vdev)
// instead of the legacy /func envelope. Only the migrated endpoints use them;
// the remaining methods keep using request/get/put above until they migrate.

// restHeaders builds the common headers for a REST call. The auth token, when
// set, is sent as a Bearer token; writes additionally declare a JSON body.
func (ccf *CliCFuncs) restHeaders(write bool) map[string]string {
	h := make(map[string]string)
	if write {
		h["Content-Type"] = "application/json"
	}
	if ccf.token != "" {
		h["Authorization"] = "Bearer " + ccf.token
	}
	return h
}

// restResult converts a transport result into (body, error), turning any non-2xx
// HTTP status into an error carrying the proxy's JSON error message.
func restResult(body []byte, status int, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		var er restapi.ErrorResponse
		if json.Unmarshal(body, &er) == nil && er.Error != "" {
			return nil, fmt.Errorf("control plane returned %d: %s", status, er.Error)
		}
		return nil, fmt.Errorf("control plane returned %d: %s", status, string(body))
	}
	return body, nil
}

// decodeEnvelope unmarshals an APIResponse[T] body and returns the payload.
// Non-user endpoints report method errors inside the envelope (HTTP 200,
// status:<0); this surfaces them as a Go error so callers see failures
// uniformly, whether they arrive as an HTTP error (restResult) or an envelope
// error. On success with no payload it returns the zero value and a nil error.
func decodeEnvelope[T any](body []byte) (T, error) {
	var zero T
	var resp restapi.APIResponse[T]
	if err := json.Unmarshal(body, &resp); err != nil {
		return zero, err
	}
	if resp.Status != restapi.StatusOK {
		if resp.Error != "" {
			return zero, errors.New(resp.Error)
		}
		return zero, errors.New("control plane request failed")
	}
	if resp.Payload == nil {
		return zero, nil
	}
	return *resp.Payload, nil
}

// restGet issues a GET to a migrated REST endpoint (path includes any query).
func (ccf *CliCFuncs) restGet(path string) ([]byte, error) {
	ccf.sdObj.TillReady("PROXY", 5)
	body, status, err := ccf.sdObj.RESTRequest(http.MethodGet, path, nil, ccf.restHeaders(false))
	return restResult(body, status, err)
}

// restPost issues a POST to a migrated REST endpoint, supplying the PumiceDB
// write idempotency key via the X-RNCUI header (required by the proxy).
func (ccf *CliCFuncs) restPost(path string, jsonBody []byte, rncui string) ([]byte, error) {
	ccf.sdObj.TillReady("PROXY", 5)
	headers := ccf.restHeaders(true)
	headers["X-RNCUI"] = rncui
	body, status, err := ccf.sdObj.RESTRequest(http.MethodPost, path, jsonBody, headers)
	return restResult(body, status, err)
}

// restDelete issues a DELETE to a migrated REST endpoint, supplying the PumiceDB
// write idempotency key via the X-RNCUI header (required by the proxy for
// writes), and maps the generic WriteResponse back to a ResponseXML.
func (ccf *CliCFuncs) restDelete(path string) (*ctlplfl.ResponseXML, error) {
	ccf.sdObj.TillReady("PROXY", 5)
	headers := ccf.restHeaders(false)
	headers["X-RNCUI"] = ccf.nextRncui()
	body, status, err := ccf.sdObj.RESTRequest(http.MethodDelete, path, nil, headers)
	body, err = restResult(body, status, err)
	if err != nil {
		return nil, err
	}
	payload, err := decodeEnvelope[restapi.WritePayload](body)
	if err != nil {
		return nil, err
	}
	return &ctlplfl.ResponseXML{ID: payload.ID, Success: true}, nil
}

// nextRncui returns the next client write idempotency key, matching the legacy
// _put scheme so writes stay dedup-stable per client.
func (ccf *CliCFuncs) nextRncui() string {
	seq := ccf.writeSeq.Add(1) - 1
	return fmt.Sprintf("%s:0:0:0:%d", ccf.appUUID, seq)
}

// restWrite POSTs a DTO to a write endpoint and maps the generic WriteResponse
// back to the internal ResponseXML the callers expect.
func (ccf *CliCFuncs) restWrite(path string, dto any) (*ctlplfl.ResponseXML, error) {
	jb, err := json.Marshal(dto)
	if err != nil {
		return nil, err
	}
	body, err := ccf.restPost(path, jb, ccf.nextRncui())
	if err != nil {
		return nil, err
	}
	payload, err := decodeEnvelope[restapi.WritePayload](body)
	if err != nil {
		return nil, err
	}
	return &ctlplfl.ResponseXML{ID: payload.ID, Success: true}, nil
}

// restPut issues a PUT to a migrated REST endpoint, supplying the PumiceDB write
// idempotency key via the X-RNCUI header (required by the proxy for writes).
func (ccf *CliCFuncs) restPut(path string, jsonBody []byte, rncui string) ([]byte, error) {
	ccf.sdObj.TillReady("PROXY", 5)
	headers := ccf.restHeaders(true)
	headers["X-RNCUI"] = rncui
	body, status, err := ccf.sdObj.RESTRequest(http.MethodPut, path, jsonBody, headers)
	return restResult(body, status, err)
}

// restWriteResource PUTs an entity DTO to PUT /api/resource?type=rtype (the
// TiDB-style infra upsert) and maps the WriteResponse back to a ResponseXML.
func (ccf *CliCFuncs) restWriteResource(rtype string, dto any) (*ctlplfl.ResponseXML, error) {
	jb, err := json.Marshal(dto)
	if err != nil {
		return nil, err
	}
	body, err := ccf.restPut("/api/resource?type="+url.QueryEscape(rtype), jb, ccf.nextRncui())
	if err != nil {
		return nil, err
	}
	payload, err := decodeEnvelope[restapi.WritePayload](body)
	if err != nil {
		return nil, err
	}
	// Legacy /func entity writes returned the entity id in ResponseXML.Name; the
	// REST WritePayload carries it as id. Populate both so callers that read
	// either field stay compatible.
	return &ctlplfl.ResponseXML{ID: payload.ID, Name: payload.ID, Success: true}, nil
}

// getResourceList fetches GET /api/resource?type=rtype. A non-empty id narrows
// the request to a single entity (GET /api/resource?type=rtype&id=id).
func (ccf *CliCFuncs) getResourceList(rtype, id string) (restapi.GetResourcePayload, error) {
	path := "/api/resource?type=" + url.QueryEscape(rtype)
	if id != "" {
		path += "&id=" + url.QueryEscape(id)
	}
	body, err := ccf.restGet(path)
	if err != nil {
		return restapi.GetResourcePayload{}, err
	}
	return decodeEnvelope[restapi.GetResourcePayload](body)
}

// ---- restapi DTO -> cpLib mapping (for REST reads) ----

func pduFromRest(p restapi.PDU) ctlplfl.PDU {
	return ctlplfl.PDU{ID: p.ID, Name: p.Name, Location: p.Location, Specification: p.Specification, PowerCapacity: p.PowerCap}
}

func rackFromRest(r restapi.Rack) ctlplfl.Rack {
	return ctlplfl.Rack{ID: r.ID, PDUID: r.PDUID, Name: r.Name, Location: r.Location, Specification: r.Specification}
}

func hypervisorFromRest(h restapi.Hypervisor) ctlplfl.Hypervisor {
	return ctlplfl.Hypervisor{ID: h.ID, RackID: h.RackID, Name: h.Name, PortRange: h.PortRange, SSHPort: h.SSHPort, RDMAEnabled: h.RDMAEnabled, IPAddrs: h.IPAddrs}
}

func deviceFromRest(d restapi.Device) ctlplfl.Device {
	return ctlplfl.Device{ID: d.ID, HypervisorID: d.HypervisorID, Name: d.Name, DevicePath: d.DevicePath, SerialNumber: d.SerialNumber, State: uint16(d.State), Size: d.Size, FailureDomain: d.FailureDomain}
}

func nisdFromRest(n restapi.NISD) ctlplfl.Nisd {
	out := ctlplfl.Nisd{ID: n.ID, PeerPort: uint16(n.PeerPort), FailureDomain: n.FailureDomain, TotalSize: n.TotalSize, AvailableSize: n.AvailableSize, SocketPath: n.SocketPath}
	for _, ni := range n.NetInfo {
		out.NetInfo = append(out.NetInfo, ctlplfl.NetworkInfo{IPAddr: ni.IPAddr, Port: uint16(ni.Port)})
	}
	out.NetInfoCnt = len(out.NetInfo)
	return out
}

func partitionFromRest(p restapi.Partition) ctlplfl.DevicePartition {
	return ctlplfl.DevicePartition{PartitionID: p.PartitionID, PartitionPath: p.PartitionPath, NISDUUID: p.NISDUUID, DevID: p.DevID, Size: p.Size}
}

func pfsFromRest(p restapi.PFS) ctlplfl.PFS {
	return ctlplfl.PFS{ID: p.ID, Name: p.Name, Offset: p.Offset, VdevIDs: p.VdevIDs}
}

// SetToken sets the auth JWT token used for all subsequent requests.
func (ccf *CliCFuncs) SetToken(token string) {
	ccf.token = token
}

func (ccf *CliCFuncs) CreateSnap(vdev string, chunkSeq []uint64, snapName string) error {
	dto := restapi.CreateSnapRequest{
		VdevID:   vdev,
		SnapName: snapName,
		ChunkSeq: chunkSeq,
	}
	jb, err := json.Marshal(dto)
	if err != nil {
		return err
	}
	body, err := ccf.restPost("/api/snap", jb, ccf.nextRncui())
	if err != nil {
		log.Error("failed to create snap: ", err)
		return err
	}

	if _, err := decodeEnvelope[restapi.CreateSnapPayload](body); err != nil {
		return err
	}
	return nil
}

func (ccf *CliCFuncs) ReadSnapByName(name string) ([]byte, error) {
	urla := fmt.Sprintf("%s=%s", ctlplfl.NAME, ctlplfl.READ_SNAP_NAME)

	var snap ctlplfl.SnapXML
	snap.SnapName = name
	rqb, err := pmCmn.Encoder(ccf.encType, snap)
	if err != nil {
		return nil, err
	}

	return ccf.request(rqb, urla, false)
}

func (ccf *CliCFuncs) ReadSnapForVdev(vdev string) ([]byte, error) {
	urla := fmt.Sprintf("%s=%s", ctlplfl.NAME, ctlplfl.READ_SNAP_VDEV)

	var snap ctlplfl.SnapXML
	snap.Vdev = vdev
	rqb, err := pmCmn.Encoder(ccf.encType, snap)
	if err != nil {
		return nil, err
	}

	return ccf.request(rqb, urla, false)
}

func (ccf *CliCFuncs) PutDevice(device *ctlplfl.Device) (*ctlplfl.ResponseXML, error) {
	if device == nil {
		return nil, fmt.Errorf("put_device: nil device")
	}
	// Child partitions are written separately via PutPartition; the single-entity
	// REST body carries only the device record.
	dto := restapi.Device{
		ID:            device.ID,
		HypervisorID:  device.HypervisorID,
		Name:          device.Name,
		DevicePath:    device.DevicePath,
		SerialNumber:  device.SerialNumber,
		State:         int(device.State),
		Size:          device.Size,
		FailureDomain: device.FailureDomain,
	}
	return ccf.restWriteResource("device", dto)
}

// TODO make changes to use new GetRequest struct
func (ccf *CliCFuncs) GetDevices(req ctlplfl.GetReq) ([]ctlplfl.Device, error) {
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	dev := make([]ctlplfl.Device, 0)
	cpResp, err := ccf.get(cpReq, ctlplfl.GET_DEVICE, &dev)
	if err != nil {
		log.Error("failed to get device info: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return dev, nil
}

func (ccf *CliCFuncs) PutNisd(ncfg *ctlplfl.Nisd) (*ctlplfl.ResponseXML, error) {
	if ncfg == nil {
		return nil, fmt.Errorf("put_nisd: nil nisd")
	}
	dto := restapi.NISD{
		ID:            ncfg.ID,
		PeerPort:      int(ncfg.PeerPort),
		FailureDomain: ncfg.FailureDomain,
		TotalSize:     ncfg.TotalSize,
		AvailableSize: ncfg.AvailableSize,
		SocketPath:    ncfg.SocketPath,
	}
	for _, ni := range ncfg.NetInfo {
		dto.NetInfo = append(dto.NetInfo, restapi.NetworkInfo{IPAddr: ni.IPAddr, Port: int(ni.Port)})
	}
	return ccf.restWriteResource("nisd", dto)
}

func (ccf *CliCFuncs) GetNisds(req ctlplfl.GetReq) ([]ctlplfl.Nisd, error) {
	req.GetAll = true
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	ncfg := make([]ctlplfl.Nisd, 0)
	cpResp, err := ccf.get(cpReq, ctlplfl.GET_NISD_LIST, &ncfg)
	if err != nil {
		log.Error("failed to fetch nisd info: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return ncfg, nil
}

func (ccf *CliCFuncs) GetNisd(req ctlplfl.GetReq) (*ctlplfl.Nisd, error) {
	body, err := ccf.restGet("/api/nisd?id=" + url.QueryEscape(req.ID))
	if err != nil {
		log.Error("failed to fetch nisd info: ", err)
		return nil, err
	}

	resp, err := decodeEnvelope[restapi.GetNisdPayload](body)
	if err != nil {
		log.Error("failed to decode nisd response: ", err)
		return nil, err
	}

	ncfg := &ctlplfl.Nisd{
		ID:       resp.ID,
		PeerPort: uint16(resp.PeerPort),
	}
	for _, ni := range resp.NetInfo {
		ncfg.NetInfo = append(ncfg.NetInfo, ctlplfl.NetworkInfo{IPAddr: ni.IPAddr, Port: uint16(ni.Port)})
	}
	ncfg.NetInfoCnt = len(ncfg.NetInfo)
	return ncfg, nil
}

func (ccf *CliCFuncs) CreateVdev(vdev *ctlplfl.VdevReq) (*ctlplfl.ResponseXML, error) {
	if vdev == nil || vdev.Vdev == nil {
		return nil, fmt.Errorf("create_vdev: nil vdev request")
	}
	// NOTE: the REST /create_vdev contract carries size_bytes, num_replicas, the
	// optional failure-domain filter, and an optional PFS link. Name is a
	// niova-only extension the proxy forwards for name-based lookup.
	reqBody := restapi.CreateVdevRequest{
		SizeBytes:    vdev.Vdev.Size,
		DataBlkCnt:   int(vdev.Vdev.DataBlkCnt),
		ParityBlkCnt: int(vdev.Vdev.ParityBlkCnt),
		Redundancy:   int(vdev.Vdev.Redundancy),
		Name:         vdev.Vdev.Name,
	}
	// Forward failure-domain scoping. FD_ANY means no scoping, so omit it.
	if vdev.Filter.Type != ctlplfl.FD_ANY {
		reqBody.FailureDomain = ctlplfl.FDName(vdev.Filter.Type)
	}
	if vdev.Filter.ID != "" {
		reqBody.EntityIDs = []string{vdev.Filter.ID}
	}
	// Forward the PFS link as id or name (server accepts either); prefer the id.
	if vdev.Vdev.PFSID != "" {
		reqBody.PFS = vdev.Vdev.PFSID
	} else if vdev.Vdev.PFSName != "" {
		reqBody.PFS = vdev.Vdev.PFSName
	}
	jb, err := json.Marshal(reqBody)
	if err != nil {
		log.Error("failed to encode create_vdev request: ", err)
		return nil, err
	}

	body, err := ccf.restPost("/api/vdev", jb, ccf.nextRncui())
	if err != nil {
		log.Error("CreateVdev failed: ", err)
		return nil, err
	}

	resp, err := decodeEnvelope[restapi.CreateVdevPayload](body)
	if err != nil {
		log.Error("failed to decode create_vdev response: ", err)
		return nil, err
	}

	return &ctlplfl.ResponseXML{ID: resp.VdevID, Success: true}, nil
}

func (ccf *CliCFuncs) GetVdevsWithChunkInfo(req *ctlplfl.GetReq) ([]ctlplfl.Vdev, error) {
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	vdevs := make([]ctlplfl.Vdev, 0)
	cpResp, err := ccf.get(cpReq, ctlplfl.GET_VDEV_CHUNK_INFO, &vdevs)
	if err != nil {
		log.Error("GetVdevsWithChunkInfo failed: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return vdevs, nil
}

func (ccf *CliCFuncs) PutPartition(devp *ctlplfl.DevicePartition) (*ctlplfl.ResponseXML, error) {
	if devp == nil {
		return nil, fmt.Errorf("put_partition: nil partition")
	}
	dto := restapi.Partition{
		PartitionID:   devp.PartitionID,
		PartitionPath: devp.PartitionPath,
		NISDUUID:      devp.NISDUUID,
		DevID:         devp.DevID,
		Size:          devp.Size,
	}
	return ccf.restWriteResource("partition", dto)
}

// GetPartition lists the partitions of the device given by req.ID (all
// partitions when req.GetAll) via GET /api/resource?type=partition.
func (ccf *CliCFuncs) GetPartition(req ctlplfl.GetReq) ([]ctlplfl.DevicePartition, error) {
	id := req.ID
	if req.GetAll {
		id = ""
	}
	resp, err := ccf.getResourceList(string(ctlplfl.ResourcePartition), id)
	if err != nil {
		log.Error("Get Partition failed: ", err)
		return nil, err
	}
	pts := make([]ctlplfl.DevicePartition, 0, len(resp.Resources))
	for _, raw := range resp.Resources {
		var p restapi.Partition
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
		pts = append(pts, partitionFromRest(p))
	}
	return pts, nil
}

func (ccf *CliCFuncs) PutPDU(req *ctlplfl.PDU) (*ctlplfl.ResponseXML, error) {
	if req == nil {
		return nil, fmt.Errorf("put_pdu: nil pdu")
	}
	dto := restapi.PDU{
		ID:            req.ID,
		Name:          req.Name,
		Location:      req.Location,
		Specification: req.Specification,
		PowerCap:      req.PowerCapacity,
	}
	return ccf.restWriteResource("pdu", dto)
}

func (ccf *CliCFuncs) GetPDUs(req *ctlplfl.GetReq) ([]ctlplfl.PDU, error) {
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	pdus := make([]ctlplfl.PDU, 0)
	cpResp, err := ccf.get(cpReq, ctlplfl.GET_PDU, &pdus)
	if err != nil {
		log.Error("GetPDUs failed: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return pdus, nil
}

func (ccf *CliCFuncs) PutPFS(req *ctlplfl.PFS) (*ctlplfl.ResponseXML, error) {
	if req == nil {
		return nil, fmt.Errorf("put_pfs: nil pfs")
	}
	dto := restapi.PFS{
		ID:      req.ID,
		Name:    req.Name,
		Offset:  req.Offset,
		VdevIDs: req.VdevIDs,
	}
	return ccf.restWrite("/api/pfs", dto)
}

func (ccf *CliCFuncs) GetPFS(req *ctlplfl.GetReq) ([]ctlplfl.PFS, error) {
	path := "/api/pfs"
	if req != nil && req.ID != "" {
		path += "?id=" + url.QueryEscape(req.ID)
	}
	body, err := ccf.restGet(path)
	if err != nil {
		log.Error("GetPFS failed: ", err)
		return nil, err
	}
	resp, err := decodeEnvelope[restapi.GetPFSPayload](body)
	if err != nil {
		return nil, err
	}
	pfss := make([]ctlplfl.PFS, 0, len(resp.PFS))
	for _, p := range resp.PFS {
		pfss = append(pfss, pfsFromRest(p))
	}
	return pfss, nil
}

func (ccf *CliCFuncs) PutRack(req *ctlplfl.Rack) (*ctlplfl.ResponseXML, error) {
	if req == nil {
		return nil, fmt.Errorf("put_rack: nil rack")
	}
	dto := restapi.Rack{
		ID:            req.ID,
		PDUID:         req.PDUID,
		Name:          req.Name,
		Location:      req.Location,
		Specification: req.Specification,
	}
	return ccf.restWriteResource("rack", dto)
}

func (ccf *CliCFuncs) GetRacks(req *ctlplfl.GetReq) ([]ctlplfl.Rack, error) {
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	racks := make([]ctlplfl.Rack, 0)
	cpResp, err := ccf.get(cpReq, ctlplfl.GET_RACK, &racks)
	if err != nil {
		log.Error("GetRacks failed: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return racks, nil
}

func (ccf *CliCFuncs) PutHypervisor(req *ctlplfl.Hypervisor) (*ctlplfl.ResponseXML, error) {
	if req == nil {
		return nil, fmt.Errorf("put_hypervisor: nil hypervisor")
	}
	dto := restapi.Hypervisor{
		ID:          req.ID,
		RackID:      req.RackID,
		Name:        req.Name,
		PortRange:   req.PortRange,
		SSHPort:     req.SSHPort,
		RDMAEnabled: req.RDMAEnabled,
		IPAddrs:     req.IPAddrs,
	}
	return ccf.restWriteResource("hypervisor", dto)
}

func (ccf *CliCFuncs) GetHypervisor(req *ctlplfl.GetReq) ([]ctlplfl.Hypervisor, error) {
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	hypervisors := make([]ctlplfl.Hypervisor, 0)
	cpResp, err := ccf.get(cpReq, ctlplfl.GET_HYPERVISOR, &hypervisors)
	if err != nil {
		log.Error("GetHypervisor failed: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return hypervisors, nil
}

func (ccf *CliCFuncs) PutNisdArgs(req *ctlplfl.NisdArgs) (*ctlplfl.ResponseXML, error) {
	if req == nil {
		return nil, fmt.Errorf("put_nisd_args: nil args")
	}
	dto := restapi.NisdArgs{
		Defrag:               req.Defrag,
		AllowDefragMCIBCache: req.AllowDefragMCIBCache,
		MBCCnt:               req.MBCCnt,
		MergeHCnt:            req.MergeHCnt,
		MCIBReadCache:        req.MCIBReadCache,
		S3:                   req.S3,
		DSync:                req.DSync,
	}
	return ccf.restWrite("/api/nisd_args", dto)
}

func (ccf *CliCFuncs) GetNisdArgs(req ctlplfl.GetReq) (ctlplfl.NisdArgs, error) {
	var args ctlplfl.NisdArgs
	body, err := ccf.restGet("/api/nisd_args")
	if err != nil {
		log.Error("failed to get nisd args: ", err)
		return args, err
	}
	resp, err := decodeEnvelope[restapi.GetNisdArgsPayload](body)
	if err != nil {
		return args, err
	}
	a := resp.NisdArgs
	args = ctlplfl.NisdArgs{
		Defrag:               a.Defrag,
		AllowDefragMCIBCache: a.AllowDefragMCIBCache,
		MBCCnt:               a.MBCCnt,
		MergeHCnt:            a.MergeHCnt,
		MCIBReadCache:        a.MCIBReadCache,
		S3:                   a.S3,
		DSync:                a.DSync,
	}
	return args, nil
}

func (ccf *CliCFuncs) GetVdevConfig(req *ctlplfl.GetReq) (ctlplfl.VdevConfig, error) {
	vdev := ctlplfl.VdevConfig{}
	body, err := ccf.restGet("/api/vdev?id=" + url.QueryEscape(req.ID))
	if err != nil {
		log.Error("Read Vdev Cfg failed: ", err)
		return vdev, err
	}

	resp, err := decodeEnvelope[restapi.GetVdevPayload](body)
	if err != nil {
		log.Error("failed to decode vdev response: ", err)
		return vdev, err
	}

	// REST /vdev carries a subset of VdevConfig; only these fields are populated.
	vdev.ID = resp.ID
	vdev.Name = resp.Name
	vdev.Size = resp.Size
	vdev.ChunkCnt = uint32(resp.NumChunks)
	vdev.DataBlkCnt = uint8(resp.NumReplicas)
	vdev.FilterType = resp.FailureDomain
	return vdev, nil
}

func (ccf *CliCFuncs) GetVdevConfigs(req *ctlplfl.GetReq) ([]ctlplfl.VdevConfig, error) {
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	vdevs := make([]ctlplfl.VdevConfig, 0)
	cpResp, err := ccf.get(cpReq, ctlplfl.GET_ALL_VDEV, &vdevs)
	if err != nil {
		log.Error("Read Vdev Cfg failed: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return vdevs, nil
}

func (ccf *CliCFuncs) PutDeviceInfo(device *ctlplfl.Device) (*ctlplfl.ResponseXML, error) {
	return ccf.PutDevice(device)
}

func (ccf *CliCFuncs) GetNisdListWithAvailSize(req *ctlplfl.GetReq) ([]ctlplfl.NisdListAvailSize, error) {
	req.Fields = ctlplfl.NisdAvailSizeFields
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: *req,
	}
	nl := make([]ctlplfl.NisdListAvailSize, 0)
	cpResp, err := ccf.get(cpReq, ctlplfl.GET_NISD_AVAILABLE_SIZES, &nl)
	if err != nil {
		log.Error("Read nisd list with available size failed: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}
	return nl, nil
}

func (ccf *CliCFuncs) GetChunkNisd(req *ctlplfl.GetReq) (ctlplfl.ChunkNisd, error) {
	cn := ctlplfl.ChunkNisd{}
	log.Info("fetching chunk Info for:", req.ID)

	// The legacy request ID is "<vdevID>/<chunkIdx>"; the REST endpoint takes
	// the vdev id and chunk index as separate query parameters.
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return cn, fmt.Errorf("invalid chunk id %q: expected \"<vdevID>/<chunkIdx>\"", req.ID)
	}
	path := "/api/chunk?vdev_id=" + url.QueryEscape(parts[0]) + "&chunk_idx=" + url.QueryEscape(parts[1])

	body, err := ccf.restGet(path)
	if err != nil {
		log.Error("GetChunkNisd failed: ", err)
		return cn, err
	}

	resp, err := decodeEnvelope[restapi.GetChunkPayload](body)
	if err != nil {
		log.Error("failed to decode chunk response: ", err)
		return cn, err
	}

	cn.NisdIDs = strings.Join(resp.NisdIDs, ",")
	return cn, nil
}

func (ccf *CliCFuncs) GetResources(req *ctlplfl.GetResourceReq) (*ctlplfl.ResourceListResp, error) {
	// A specific id (with GetAll unset) narrows the fetch to one entity; GetAll
	// (or an empty id) lists all of that type.
	id := ""
	if !req.GetAll {
		id = req.ID
	}
	resp, err := ccf.getResourceList(string(req.ResourceType), id)
	if err != nil {
		log.Errorf("GetResources failed for resource type %q: %v", req.ResourceType, err)
		return nil, err
	}
	out := &ctlplfl.ResourceListResp{ResourceType: req.ResourceType}
	// The proxy returns a single generic `resources` array (TiDB shape); every
	// element is the DTO for the requested type. Decode each into that type.
	for _, raw := range resp.Resources {
		switch req.ResourceType {
		case ctlplfl.ResourcePDU:
			var v restapi.PDU
			if err := json.Unmarshal(raw, &v); err != nil {
				return nil, err
			}
			out.PDUs = append(out.PDUs, pduFromRest(v))
		case ctlplfl.ResourceRack:
			var v restapi.Rack
			if err := json.Unmarshal(raw, &v); err != nil {
				return nil, err
			}
			out.Racks = append(out.Racks, rackFromRest(v))
		case ctlplfl.ResourceHypervisor:
			var v restapi.Hypervisor
			if err := json.Unmarshal(raw, &v); err != nil {
				return nil, err
			}
			out.Hypervisors = append(out.Hypervisors, hypervisorFromRest(v))
		case ctlplfl.ResourceDevice:
			var v restapi.Device
			if err := json.Unmarshal(raw, &v); err != nil {
				return nil, err
			}
			out.Devices = append(out.Devices, deviceFromRest(v))
		case ctlplfl.ResourceNisd:
			var v restapi.NISD
			if err := json.Unmarshal(raw, &v); err != nil {
				return nil, err
			}
			out.Nisds = append(out.Nisds, nisdFromRest(v))
		case ctlplfl.ResourcePartition:
			var v restapi.Partition
			if err := json.Unmarshal(raw, &v); err != nil {
				return nil, err
			}
			out.Partitions = append(out.Partitions, partitionFromRest(v))
		}
	}
	return out, nil
}

func (ccf *CliCFuncs) GetChunk(req *ctlplfl.GetReq) (ctlplfl.Chunk, error) {
	cn := ctlplfl.Chunk{}
	log.Info("fetching chunk Info for:", req.ID)
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	cpResp, err := ccf.get(cpReq, ctlplfl.GET_CHUNK, &cn)
	if err != nil {
		log.Error("GetChunk failed: ", err)
		return cn, err
	}

	if err := cpResp.Err(); err != nil {
		return cn, err
	}

	return cn, nil
}

func (ccf *CliCFuncs) GetChunks(req *ctlplfl.GetReq) ([]ctlplfl.Chunk, error) {
	var chunks []ctlplfl.Chunk
	log.Info("fetching bulk chunks for:", req.ID)
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	cpResp, err := ccf.get(cpReq, ctlplfl.GET_CHUNK, &chunks)
	if err != nil {
		log.Error("GetChunks failed: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return chunks, nil
}

func (ccf *CliCFuncs) DeleteVdev(req *ctlplfl.DeleteVdevReq) (*ctlplfl.ResponseXML, error) {
	if req == nil || req.ID == "" {
		return nil, fmt.Errorf("delete_vdev: missing id")
	}
	return ccf.restDelete("/api/vdev/" + url.PathEscape(req.ID))
}

func (ccf *CliCFuncs) MountVdev(req *ctlplfl.MountVdevRequest) (ctlplfl.VdevConfig, error) {
	vdev := ctlplfl.VdevConfig{}
	if req == nil {
		return vdev, fmt.Errorf("mount_vdev: nil request")
	}
	dto := restapi.MountVdevRequest{VdevID: req.VdevID}
	jb, err := json.Marshal(dto)
	if err != nil {
		return vdev, err
	}
	body, err := ccf.restPost("/api/mount_vdev", jb, ccf.nextRncui())
	if err != nil {
		log.Error("MountVdev failed for vdev: ", req.VdevID, " with error: ", err)
		return vdev, err
	}

	// The proxy returns the flat snake_case MountVdevPayload (mount info +
	// AccessToken); map it back into the internal VdevConfig the callers expect.
	p, err := decodeEnvelope[restapi.MountVdevPayload](body)
	if err != nil {
		return vdev, err
	}
	vdev = ctlplfl.VdevConfig{
		ID:            p.ID,
		Name:          p.Name,
		Size:          p.Size,
		ChunkCnt:      uint32(p.NumChunks),
		DataBlkCnt:    uint8(p.NumReplica),
		VdevMountInfo: ctlplfl.VdevMountInfo{MountCounter: p.MountCounter, LastUpdatedLTS: p.LastUpdatedLTS},
		PFSID:         p.PFSID,
		AccessToken:   p.AccessToken,
	}
	return vdev, nil
}
