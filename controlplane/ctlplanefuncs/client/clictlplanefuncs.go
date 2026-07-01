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

func (ccf *CliCFuncs) _put(urla string, rqb []byte) ([]byte, error) {
	seq := ccf.writeSeq.Add(1) - 1
	rncui := fmt.Sprintf("%s:0:0:0:%d", ccf.appUUID, seq)
	urla += "&rncui=" + rncui
	rsb, err := ccf.request(rqb, urla, true)
	return rsb, err
}

func (ccf *CliCFuncs) put(cpReq *ctlplfl.CPReq, urla string, target any) (*ctlplfl.CPResp, error) {
	url := "name=" + urla
	rqb, err := pmCmn.Encoder(ccf.encType, cpReq)
	if err != nil {
		log.Error("failed to encode data: ", err)
		return nil, err
	}

	rsb, err := ccf._put(url, rqb)
	if err != nil {
		log.Error("failed to send request(_put): ", err)
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
		log.Error("failed to decode response in put: ", err)
		return nil, err
	}

	return cpResp, nil
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
	var resp restapi.WriteResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &ctlplfl.ResponseXML{ID: resp.ID, Success: resp.Success, Error: resp.Error}, nil
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
	var resp restapi.WriteResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &ctlplfl.ResponseXML{ID: resp.ID, Success: resp.Success, Error: resp.Error}, nil
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
	var resp restapi.WriteResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	// Legacy /func entity writes returned the entity id in ResponseXML.Name; the
	// REST WriteResponse carries it as id. Populate both so callers that read
	// either field stay compatible.
	return &ctlplfl.ResponseXML{ID: resp.ID, Name: resp.ID, Success: resp.Success, Error: resp.Error}, nil
}

// getResourceList fetches GET /api/resource?type=rtype. A non-empty id narrows
// the request to a single entity (GET /api/resource?type=rtype&id=id).
func (ccf *CliCFuncs) getResourceList(rtype, id string) (restapi.GetResourceResponse, error) {
	var resp restapi.GetResourceResponse
	path := "/api/resource?type=" + url.QueryEscape(rtype)
	if id != "" {
		path += "&id=" + url.QueryEscape(id)
	}
	body, err := ccf.restGet(path)
	if err != nil {
		return resp, err
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return resp, err
	}
	return resp, nil
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

	var resp restapi.CreateSnapResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return err
	}
	if !resp.Success {
		return errors.New("Snap not created")
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

	var resp restapi.GetNisdResponse
	if err := json.Unmarshal(body, &resp); err != nil {
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
	// NOTE: the REST /create_vdev contract carries only size_bytes and
	// num_replicas. Name, PFS, and failure-domain Filter are not part of the
	// REST contract and are intentionally dropped here.
	reqBody := restapi.CreateVdevRequest{
		SizeBytes:   vdev.Vdev.Size,
		NumReplicas: int(vdev.Vdev.DataBlkCnt),
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

	var resp restapi.CreateVdevResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		log.Error("failed to decode create_vdev response: ", err)
		return nil, err
	}

	return &ctlplfl.ResponseXML{ID: resp.VdevID, Success: resp.Success, Error: resp.Error}, nil
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

func (ccf *CliCFuncs) GetPartition(req ctlplfl.GetReq) ([]ctlplfl.DevicePartition, error) {
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	pts := make([]ctlplfl.DevicePartition, 0)
	cpResp, err := ccf.get(cpReq, ctlplfl.GET_PARTITION, &pts)
	if err != nil {
		log.Error("Get Partition failed: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
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
	var resp restapi.GetPFSResponse
	if err := json.Unmarshal(body, &resp); err != nil {
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
	var resp restapi.GetNisdArgsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
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

	var resp restapi.GetVdevResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		log.Error("failed to decode vdev response: ", err)
		return vdev, err
	}

	// REST /vdev carries a subset of VdevConfig; only these fields are populated.
	vdev.ID = resp.ID
	vdev.Size = resp.Size
	vdev.ChunkCnt = uint32(resp.NumChunks)
	vdev.DataBlkCnt = uint8(resp.NumReplicas)
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

	var resp restapi.GetChunkResponse
	if err := json.Unmarshal(body, &resp); err != nil {
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
	for _, p := range resp.PDUs {
		out.PDUs = append(out.PDUs, pduFromRest(p))
	}
	for _, x := range resp.Racks {
		out.Racks = append(out.Racks, rackFromRest(x))
	}
	for _, x := range resp.Hypervisors {
		out.Hypervisors = append(out.Hypervisors, hypervisorFromRest(x))
	}
	for _, x := range resp.Devices {
		out.Devices = append(out.Devices, deviceFromRest(x))
	}
	for _, x := range resp.Nisds {
		out.Nisds = append(out.Nisds, nisdFromRest(x))
	}
	for _, x := range resp.Partitions {
		out.Partitions = append(out.Partitions, partitionFromRest(x))
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

	// The proxy returns the full VdevCfg (mount info + AccessToken); decode it
	// straight into the return value.
	if err := json.Unmarshal(body, &vdev); err != nil {
		return vdev, err
	}
	return vdev, nil
}
