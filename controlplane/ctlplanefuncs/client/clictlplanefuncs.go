package clictlplanefuncs

import (
	"errors"
	"fmt"
	"sync/atomic"

	log "github.com/00pauln00/niova-lookout/pkg/xlog"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"

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

// SetToken sets the auth JWT token used for all subsequent requests.
func (ccf *CliCFuncs) SetToken(token string) {
	ccf.token = token
}

func (ccf *CliCFuncs) CreateSnap(vdev string, chunkSeq []uint64, snapName string) error {
	chks := make([]ctlplfl.ChunkXML, 0)
	for idx, seq := range chunkSeq {
		chks = append(chks,
			ctlplfl.ChunkXML{
				Idx: uint32(idx),
				Seq: seq,
			})
	}
	var Snap ctlplfl.SnapXML
	Snap.SnapName = snapName
	Snap.Vdev = vdev
	Snap.Chunks = chks

	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: Snap,
	}

	snapRes := ctlplfl.SnapResponseXML{}
	cpResp, err := ccf.put(cpReq, ctlplfl.CREATE_SNAP, &snapRes)
	if err != nil {
		log.Error("failed to fetch snap info: ", err)
		return err
	}

	if err := cpResp.Err(); err != nil {
		return err
	}

	if !snapRes.SnapName.Success {
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
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: device,
	}
	resp := &ctlplfl.ResponseXML{}
	cpResp, err := ccf.put(cpReq, ctlplfl.PUT_DEVICE, resp)
	if err != nil {
		log.Error("PutDevice failed: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return resp, nil
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
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: ncfg,
	}
	resp := &ctlplfl.ResponseXML{}
	cpResp, err := ccf.put(cpReq, ctlplfl.PUT_NISD, resp)
	if err != nil {
		log.Error("failed to update nisd info: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return resp, nil
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
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	ncfg := &ctlplfl.Nisd{}
	cpResp, err := ccf.get(cpReq, ctlplfl.GET_NISD, ncfg)
	if err != nil {
		log.Error("failed to fetch nisd info: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return ncfg, nil
}

func (ccf *CliCFuncs) CreateVdev(vdev *ctlplfl.VdevReq) (*ctlplfl.ResponseXML, error) {
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: vdev,
	}
	resp := &ctlplfl.ResponseXML{}
	cpResp, err := ccf.put(cpReq, ctlplfl.CREATE_VDEV, resp)
	if err != nil {
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return resp, nil
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
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: devp,
	}
	resp := &ctlplfl.ResponseXML{}
	cpResp, err := ccf.put(cpReq, ctlplfl.PUT_PARTITION, resp)
	if err != nil {
		log.Error("Put Partition failed: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return resp, nil
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
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	resp := &ctlplfl.ResponseXML{}
	cpResp, err := ccf.put(cpReq, ctlplfl.PUT_PDU, resp)
	if err != nil {
		log.Error("PutPDUs failed: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return resp, nil
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

func (ccf *CliCFuncs) PutRack(req *ctlplfl.Rack) (*ctlplfl.ResponseXML, error) {
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	resp := &ctlplfl.ResponseXML{}
	cpResp, err := ccf.put(cpReq, ctlplfl.PUT_RACK, resp)
	if err != nil {
		log.Error("PutRack failed: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return resp, nil
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
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	resp := &ctlplfl.ResponseXML{}
	cpResp, err := ccf.put(cpReq, ctlplfl.PUT_HYPERVISOR, resp)
	if err != nil {
		log.Error("PutHypervisor failed: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return resp, nil
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
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	resp := &ctlplfl.ResponseXML{}
	cpResp, err := ccf.put(cpReq, ctlplfl.PUT_NISD_ARGS, resp)
	if err != nil {
		log.Error("PutNisdArgs failed: ", err)
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return resp, nil
}

func (ccf *CliCFuncs) GetNisdArgs(req ctlplfl.GetReq) (ctlplfl.NisdArgs, error) {
	var args ctlplfl.NisdArgs
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	cpResp, err := ccf.get(cpReq, ctlplfl.GET_NISD_ARGS, &args)
	if err != nil {
		log.Error("failed to get nisd args: ", err)
		return args, err
	}

	if err := cpResp.Err(); err != nil {
		return args, err
	}

	return args, nil
}

func (ccf *CliCFuncs) GetVdevCfg(req *ctlplfl.GetReq) (ctlplfl.VdevCfg, error) {
	vdev := ctlplfl.VdevCfg{}
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	cpResp, err := ccf.get(cpReq, ctlplfl.GET_VDEV_INFO, &vdev)
	if err != nil {
		log.Error("Read Vdev Cfg failed: ", err)
		return vdev, err
	}

	if err := cpResp.Err(); err != nil {
		return vdev, err
	}

	return vdev, nil
}

func (ccf *CliCFuncs) GetVdevCfgs(req *ctlplfl.GetReq) ([]ctlplfl.VdevCfg, error) {
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	vdevs := make([]ctlplfl.VdevCfg, 0)
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

func (ccf *CliCFuncs) GetChunkNisd(req *ctlplfl.GetReq) (ctlplfl.ChunkNisd, error) {
	cn := ctlplfl.ChunkNisd{}
	log.Info("fetching chunk Info for:", req.ID)
	cpReq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	cpResp, err := ccf.get(cpReq, ctlplfl.GET_CHUNK_NISD, &cn)
	if err != nil {
		log.Error("GetHypervisor failed: ", err)
		return cn, err
	}

	if err := cpResp.Err(); err != nil {
		return cn, err
	}

	return cn, nil
}

func (ccf *CliCFuncs) DeleteVdev(req *ctlplfl.DeleteVdevReq) (*ctlplfl.ResponseXML, error) {
	resp := &ctlplfl.ResponseXML{}
	cpreq := &ctlplfl.CPReq{
		Token:   ccf.token,
		Payload: req,
	}
	cpResp, err := ccf.put(cpreq, ctlplfl.DELETE_VDEV, resp)
	if err != nil {
		return nil, err
	}

	if err := cpResp.Err(); err != nil {
		return nil, err
	}

	return resp, nil
}
