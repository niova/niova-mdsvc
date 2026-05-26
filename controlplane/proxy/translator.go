package main

import (
	"fmt"
	"net/http"
	"strings"

	log "github.com/00pauln00/niova-lookout/pkg/xlog"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"

	pmLib "github.com/00pauln00/niova-pumicedb/go/pkg/pumicecommon"
)

func GetEncodingType(r *http.Request) pmLib.Format {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		ct = r.Header.Get("Accept")
	}
	ct = strings.ToLower(ct)
	res := strings.Split(ct, "/")
	return pmLib.Format(res[1])
}

func GetReqStruct(name string) any {
	switch name {
	case cpLib.GET_RACK, cpLib.GET_NISD, cpLib.GET_DEVICE, cpLib.GET_PDU, cpLib.GET_HYPERVISOR, cpLib.GET_PARTITION, cpLib.GET_VDEV, cpLib.GET_CHUNK_NISD, userlib.LoginAPI, cpLib.GET_VDEV_CHUNK_INFO, cpLib.GET_VDEV_INFO:
		return &cpLib.GetReq{}
	case cpLib.PUT_RACK:
		return &cpLib.Rack{}
	case cpLib.PUT_DEVICE:
		return &cpLib.Device{}
	case cpLib.PUT_HYPERVISOR:
		return &cpLib.Hypervisor{}
	case cpLib.PUT_NISD:
		return &cpLib.Nisd{}
	case cpLib.PUT_PDU:
		return &cpLib.PDU{}
	case cpLib.PUT_PARTITION:
		return &cpLib.DevicePartition{}
	case cpLib.PUT_NISD_ARGS:
		return &cpLib.NisdArgs{}
	case cpLib.CREATE_SNAP, cpLib.READ_SNAP_NAME, cpLib.READ_SNAP_VDEV:
		return &cpLib.SnapXML{}
	case cpLib.CREATE_VDEV:
		return &cpLib.VdevReq{}
	case userlib.PutUserAPI, userlib.AdminUserAPI:
		return &userlib.UserReq{}
	case userlib.GetUserAPI:
		return &userlib.GetReq{}
	case cpLib.DELETE_VDEV:
		return &cpLib.DeleteVdevReq{}
	default:
		return &cpLib.GetReq{}
	}
}

func GetRespStruct(name string) any {
	switch name {
	case cpLib.GET_RACK:
		return &[]cpLib.Rack{}
	case cpLib.GET_DEVICE:
		return &[]cpLib.Device{}
	case cpLib.GET_CHUNK_NISD:
		return &cpLib.ChunkNisd{}
	case cpLib.GET_VDEV_INFO:
		return &cpLib.VdevCfg{}
	case cpLib.GET_ALL_VDEV:
		return &[]cpLib.VdevCfg{}
	case cpLib.GET_HYPERVISOR:
		return &[]cpLib.Hypervisor{}
	case cpLib.GET_NISD_LIST:
		return &[]cpLib.Nisd{}
	case cpLib.GET_NISD:
		return &cpLib.Nisd{}
	case cpLib.GET_PDU:
		return &[]cpLib.PDU{}
	case cpLib.GET_PARTITION:
		return &[]cpLib.DevicePartition{}
	case cpLib.GET_VDEV_CHUNK_INFO, cpLib.GET_VDEV:
		return &[]*cpLib.Vdev{}
	case cpLib.READ_SNAP_NAME, cpLib.READ_SNAP_VDEV:
		return &cpLib.SnapXML{}
	case cpLib.CREATE_SNAP:
		return &cpLib.SnapResponseXML{}
	case cpLib.GET_NISD_ARGS:
		return &cpLib.NisdArgs{}
	case cpLib.PUT_RACK, cpLib.PUT_DEVICE, cpLib.PUT_HYPERVISOR, cpLib.PUT_NISD, cpLib.PUT_PDU, cpLib.PUT_PARTITION, cpLib.PUT_NISD_ARGS, cpLib.CREATE_VDEV:
		return &cpLib.ResponseXML{}
	case userlib.PutUserAPI, userlib.AdminUserAPI:
		return &userlib.UserResp{}
	case userlib.GetUserAPI:
		return &[]userlib.UserResp{}
	case userlib.LoginAPI:
		return &userlib.LoginResp{}
	case cpLib.DELETE_VDEV:
		return &cpLib.ResponseXML{}
	}
	return nil
}

// DecodeCPReq decodes the incoming request body into a CPReq envelope.
// It pre-populates the Payload field to guide the decoder, allowing it
// to fill the correct struct in a single pass.
func DecodeCPReq(enctype pmLib.Format, name string, req []byte) (*cpLib.CPReq, error) {
	// Pre-populate payload to guide the decoder in a single pass
	typedPayload := GetReqStruct(name)
	cpReq := &cpLib.CPReq{
		Payload: typedPayload,
	}

	log.Tracef("decoding CPReq for %s with %v decoder", name, enctype)
	err := pmLib.Decoder(enctype, req, cpReq)
	if err != nil {
		log.Error("failed to decode CPReq: ", err)
		return nil, err
	}

	// For downstream compatibility (e.g., GOB encoding of the envelope),
	// we ensure the value type (not pointer) is stored in the interface.
	if cpReq.Payload != nil {
		cpReq.Payload = derefPtr(cpReq.Payload)
	}

	return cpReq, nil
}

// derefPtr dereferences a pointer to return the underlying value.
// This ensures GOB can encode the Payload correctly as a concrete type.
func derefPtr(v any) any {
	switch p := v.(type) {
	case *cpLib.GetReq:
		return *p
	case *cpLib.Rack:
		return *p
	case *cpLib.Device:
		return *p
	case *cpLib.Hypervisor:
		return *p
	case *cpLib.Nisd:
		return *p
	case *cpLib.PDU:
		return *p
	case *cpLib.DevicePartition:
		return *p
	case *cpLib.NisdArgs:
		return *p
	case *cpLib.SnapXML:
		return *p
	case *cpLib.VdevReq:
		return *p
	case *userlib.UserReq:
		return *p
	case *userlib.GetReq:
		return *p
	case *cpLib.DeleteVdevReq:
		return *p
	default:
		return v
	}
}

func EncodeResponse(enctype pmLib.Format, name string, resp *[]byte) error {

	if resp == nil {
		log.Error("EncodeResponse: response pointer is nil")
		return fmt.Errorf("response pointer is nil")
	}

	log.Infof("EncodeResponse called | func=%s respSize=%d", name, len(*resp))

	if len(*resp) == 0 {
		log.Error("EncodeResponse: empty response buffer, cannot decode")
		return fmt.Errorf("empty response buffer")
	}

	res := GetRespStruct(name)
	cpresp := &cpLib.CPResp{
		Payload: res,
	}
	err := pmLib.Decoder(pmLib.GOB, *resp, cpresp)
	if err != nil {
		log.Errorf("gob: failed to decode response | func=%s size=%d err=%v", name, len(*resp), err)
		return err
	}
	log.Tracef("encoding response type %s, with %v encoder", name, enctype)
	*resp, err = pmLib.Encoder(enctype, cpresp)
	if err != nil {
		log.Errorf("%v: failed to encode response for %s: %v", enctype, name, err)
		return err
	}

	log.Tracef("Response encoded successfully | newSize=%d", len(*resp))

	return nil
}
