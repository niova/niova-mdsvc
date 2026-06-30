package main

import (
	"net/http"
	"strings"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	restapi "github.com/00pauln00/niova-mdsvc/controlplane/restapi"
)

// ---- cpLib <-> restapi DTO mapping helpers ----

func toRestPDU(p cpLib.PDU) restapi.PDU {
	return restapi.PDU{
		ID:            p.ID,
		Name:          p.Name,
		Location:      p.Location,
		Specification: p.Specification,
		PowerCap:      p.PowerCapacity,
	}
}

func toRestRack(r cpLib.Rack) restapi.Rack {
	return restapi.Rack{
		ID:            r.ID,
		PDUID:         r.PDUID,
		Name:          r.Name,
		Location:      r.Location,
		Specification: r.Specification,
	}
}

func toRestHypervisor(h cpLib.Hypervisor) restapi.Hypervisor {
	return restapi.Hypervisor{
		ID:          h.ID,
		RackID:      h.RackID,
		Name:        h.Name,
		PortRange:   h.PortRange,
		SSHPort:     h.SSHPort,
		RDMAEnabled: h.RDMAEnabled,
		IPAddrs:     h.IPAddrs,
	}
}

func toRestDevice(d cpLib.Device) restapi.Device {
	return restapi.Device{
		ID:            d.ID,
		HypervisorID:  d.HypervisorID,
		Name:          d.Name,
		DevicePath:    d.DevicePath,
		SerialNumber:  d.SerialNumber,
		State:         int(d.State),
		Size:          d.Size,
		FailureDomain: d.FailureDomain,
	}
}

func toRestNISD(n cpLib.Nisd) restapi.NISD {
	out := restapi.NISD{
		ID:            n.ID,
		PeerPort:      int(n.PeerPort),
		FailureDomain: n.FailureDomain,
		TotalSize:     n.TotalSize,
		AvailableSize: n.AvailableSize,
		SocketPath:    n.SocketPath,
	}
	for _, ni := range n.NetInfo {
		out.NetInfo = append(out.NetInfo, restapi.NetworkInfo{IPAddr: ni.IPAddr, Port: int(ni.Port)})
	}
	return out
}

func toRestPartition(p cpLib.DevicePartition) restapi.Partition {
	return restapi.Partition{
		PartitionID:   p.PartitionID,
		PartitionPath: p.PartitionPath,
		NISDUUID:      p.NISDUUID,
		DevID:         p.DevID,
		Size:          p.Size,
	}
}

func toRestPFS(p cpLib.PFS) restapi.PFS {
	return restapi.PFS{
		ID:      p.ID,
		Name:    p.Name,
		Offset:  p.Offset,
		VdevIDs: p.VdevIDs,
	}
}

// ---- PUT /api/resource?type= ----

// handlePutResource upserts a single infra entity selected by the ?type= query
// parameter (PUT_PDU/PUT_RACK/PUT_HYPERVISOR/PUT_DEVICE/PUT_NISD/PUT_PARTITION).
// It is the TiDB-style replacement for the former per-entity write endpoints.
func (handler *proxyHandler) handlePutResource(w http.ResponseWriter, r *http.Request) {
	rtype := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	if rtype == "" {
		restapi.WriteError(w, http.StatusBadRequest, "missing required query parameter: type")
		return
	}

	switch cpLib.ResourceType(rtype) {
	case cpLib.ResourcePDU:
		var req restapi.PDU
		if !decodeJSONBody(w, r, &req) {
			return
		}
		if req.ID == "" {
			restapi.WriteError(w, http.StatusBadRequest, "id is required")
			return
		}
		handler.runEntityWrite(w, r, cpLib.PUT_PDU, cpLib.PDU{
			ID:            req.ID,
			Name:          req.Name,
			Location:      req.Location,
			Specification: req.Specification,
			PowerCapacity: req.PowerCap,
		}, req.ID)

	case cpLib.ResourceRack:
		var req restapi.Rack
		if !decodeJSONBody(w, r, &req) {
			return
		}
		if req.ID == "" {
			restapi.WriteError(w, http.StatusBadRequest, "id is required")
			return
		}
		handler.runEntityWrite(w, r, cpLib.PUT_RACK, cpLib.Rack{
			ID:            req.ID,
			PDUID:         req.PDUID,
			Name:          req.Name,
			Location:      req.Location,
			Specification: req.Specification,
		}, req.ID)

	case cpLib.ResourceHypervisor:
		var req restapi.Hypervisor
		if !decodeJSONBody(w, r, &req) {
			return
		}
		if req.ID == "" {
			restapi.WriteError(w, http.StatusBadRequest, "id is required")
			return
		}
		handler.runEntityWrite(w, r, cpLib.PUT_HYPERVISOR, cpLib.Hypervisor{
			ID:          req.ID,
			RackID:      req.RackID,
			Name:        req.Name,
			PortRange:   req.PortRange,
			SSHPort:     req.SSHPort,
			RDMAEnabled: req.RDMAEnabled,
			IPAddrs:     req.IPAddrs,
		}, req.ID)

	case cpLib.ResourceDevice:
		var req restapi.Device
		if !decodeJSONBody(w, r, &req) {
			return
		}
		if req.ID == "" {
			restapi.WriteError(w, http.StatusBadRequest, "id is required")
			return
		}
		handler.runEntityWrite(w, r, cpLib.PUT_DEVICE, cpLib.Device{
			ID:            req.ID,
			HypervisorID:  req.HypervisorID,
			Name:          req.Name,
			DevicePath:    req.DevicePath,
			SerialNumber:  req.SerialNumber,
			State:         uint16(req.State),
			Size:          req.Size,
			FailureDomain: req.FailureDomain,
		}, req.ID)

	case cpLib.ResourceNisd:
		var req restapi.NISD
		if !decodeJSONBody(w, r, &req) {
			return
		}
		if req.ID == "" {
			restapi.WriteError(w, http.StatusBadRequest, "id is required")
			return
		}
		nisd := cpLib.Nisd{
			ID:            req.ID,
			PeerPort:      uint16(req.PeerPort),
			FailureDomain: req.FailureDomain,
			TotalSize:     req.TotalSize,
			AvailableSize: req.AvailableSize,
			SocketPath:    req.SocketPath,
		}
		for _, ni := range req.NetInfo {
			nisd.NetInfo = append(nisd.NetInfo, cpLib.NetworkInfo{IPAddr: ni.IPAddr, Port: uint16(ni.Port)})
		}
		nisd.NetInfoCnt = len(nisd.NetInfo)
		handler.runEntityWrite(w, r, cpLib.PUT_NISD, nisd, req.ID)

	case cpLib.ResourcePartition:
		var req restapi.Partition
		if !decodeJSONBody(w, r, &req) {
			return
		}
		if req.PartitionID == "" {
			restapi.WriteError(w, http.StatusBadRequest, "partition_id is required")
			return
		}
		handler.runEntityWrite(w, r, cpLib.PUT_PARTITION, cpLib.DevicePartition{
			PartitionID:   req.PartitionID,
			PartitionPath: req.PartitionPath,
			NISDUUID:      req.NISDUUID,
			DevID:         req.DevID,
			Size:          req.Size,
		}, req.PartitionID)

	default:
		restapi.WriteError(w, http.StatusBadRequest, "unsupported resource type: %s", rtype)
	}
}

// ---- GET /api/resource?type= ----

// getResourcesByType fetches one resource type's list via GET_ALL_RESOURCES.
func (handler *proxyHandler) getResourcesByType(r *http.Request, rtype cpLib.ResourceType) (cpLib.ResourceListResp, *cpLib.CPResp, error) {
	cpReq := cpLib.CPReq{
		Token:   tokenFromRequest(r),
		Payload: cpLib.GetResourceReq{ResourceType: rtype, GetAll: true},
	}
	var rl cpLib.ResourceListResp
	cpResp, err := handler.callFunc(cpLib.GET_ALL_RESOURCES, cpReq, false, "", 0, &rl)
	return rl, cpResp, err
}

// handleGetResource lists infra entities of the ?type= given. Maps to
// GET_ALL_RESOURCES (ReadAllResources) and returns the matching slice.
func (handler *proxyHandler) handleGetResource(w http.ResponseWriter, r *http.Request) {
	rtype := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	if rtype == "" {
		restapi.WriteError(w, http.StatusBadRequest, "missing required query parameter: type")
		return
	}
	switch cpLib.ResourceType(rtype) {
	case cpLib.ResourcePDU, cpLib.ResourceRack, cpLib.ResourceHypervisor,
		cpLib.ResourceDevice, cpLib.ResourceNisd, cpLib.ResourcePartition:
	default:
		restapi.WriteError(w, http.StatusBadRequest, "unsupported resource type: %s", rtype)
		return
	}

	rl, cpResp, err := handler.getResourcesByType(r, cpLib.ResourceType(rtype))
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}

	resp := restapi.GetResourceResponse{Success: true, Type: rtype}
	switch cpLib.ResourceType(rtype) {
	case cpLib.ResourcePDU:
		for _, p := range rl.PDUs {
			resp.PDUs = append(resp.PDUs, toRestPDU(p))
		}
	case cpLib.ResourceRack:
		for _, x := range rl.Racks {
			resp.Racks = append(resp.Racks, toRestRack(x))
		}
	case cpLib.ResourceHypervisor:
		for _, x := range rl.Hypervisors {
			resp.Hypervisors = append(resp.Hypervisors, toRestHypervisor(x))
		}
	case cpLib.ResourceDevice:
		for _, x := range rl.Devices {
			resp.Devices = append(resp.Devices, toRestDevice(x))
		}
	case cpLib.ResourceNisd:
		for _, x := range rl.Nisds {
			resp.Nisds = append(resp.Nisds, toRestNISD(x))
		}
	case cpLib.ResourcePartition:
		for _, x := range rl.Partitions {
			resp.Partitions = append(resp.Partitions, toRestPartition(x))
		}
	}
	restapi.WriteJSON(w, http.StatusOK, resp)
}

// ---- GET /api/pfs?id= ----

// handleGetPFS returns PFS metadata. Maps to GET_PFS (ReadPFS).
func (handler *proxyHandler) handleGetPFS(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	cpReq := cpLib.CPReq{
		Token:   tokenFromRequest(r),
		Payload: cpLib.GetReq{ID: id, GetAll: id == ""},
	}
	var pfs []cpLib.PFS
	cpResp, err := handler.callFunc(cpLib.GET_PFS, cpReq, false, "", 0, &pfs)
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}
	resp := restapi.GetPFSResponse{Success: true}
	for _, p := range pfs {
		resp.PFS = append(resp.PFS, toRestPFS(p))
	}
	restapi.WriteJSON(w, http.StatusOK, resp)
}

// ---- GET /api/nisd_args ----

// handleGetNisdArgs returns the singleton NISD launch-arguments record. Maps to
// GET_NISD_ARGS (ReadNisdArgs).
func (handler *proxyHandler) handleGetNisdArgs(w http.ResponseWriter, r *http.Request) {
	cpReq := cpLib.CPReq{Token: tokenFromRequest(r), Payload: cpLib.GetReq{}}
	var args cpLib.NisdArgs
	cpResp, err := handler.callFunc(cpLib.GET_NISD_ARGS, cpReq, false, "", 0, &args)
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}
	restapi.WriteJSON(w, http.StatusOK, restapi.GetNisdArgsResponse{
		Success: true,
		NisdArgs: restapi.NisdArgs{
			Defrag:               args.Defrag,
			AllowDefragMCIBCache: args.AllowDefragMCIBCache,
			MBCCnt:               args.MBCCnt,
			MergeHCnt:            args.MergeHCnt,
			MCIBReadCache:        args.MCIBReadCache,
			S3:                   args.S3,
			DSync:                args.DSync,
		},
	})
}

// NOTE: GET /api/chunks (paginated chunk->NISD listing over GET_VDEV_CHUNK_INFO)
// is intentionally not implemented for now: ranging over a vdev's chunks works
// differently in mdsvc and is deferred. The single-chunk GET /api/chunk read
// (handleGetChunk) remains available.

// ---- GET /api/infra ----

// handleGetInfra assembles the full PDU -> Rack -> Hypervisor -> Device -> NISD
// hierarchy from GET_ALL_RESOURCES (one call per level) and nests it by the
// stored foreign keys. NISDs attach to devices via the device id carried in
// Nisd.FailureDomain (ordered [pdu, rack, hv, device, partition]). This is a
// non-atomic read assembled proxy-side.
func (handler *proxyHandler) handleGetInfra(w http.ResponseWriter, r *http.Request) {
	pduRL, cpResp, err := handler.getResourcesByType(r, cpLib.ResourcePDU)
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}
	rackRL, cpResp, err := handler.getResourcesByType(r, cpLib.ResourceRack)
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}
	hvRL, cpResp, err := handler.getResourcesByType(r, cpLib.ResourceHypervisor)
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}
	devRL, cpResp, err := handler.getResourcesByType(r, cpLib.ResourceDevice)
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}
	nisdRL, cpResp, err := handler.getResourcesByType(r, cpLib.ResourceNisd)
	if err != nil || cpErrOf(cpResp) != nil {
		writeCPError(w, err, cpErrOf(cpResp))
		return
	}

	// NISDs grouped by their parent device id (FailureDomain[3]).
	const deviceFDIdx = 3
	nisdsByDev := make(map[string][]restapi.NISDNode)
	for _, n := range nisdRL.Nisds {
		if len(n.FailureDomain) <= deviceFDIdx {
			continue
		}
		devID := n.FailureDomain[deviceFDIdx]
		node := restapi.NISDNode{
			ID:            n.ID,
			PeerPort:      int(n.PeerPort),
			TotalSize:     n.TotalSize,
			AvailableSize: n.AvailableSize,
			SocketPath:    n.SocketPath,
		}
		for _, ni := range n.NetInfo {
			node.NetInfo = append(node.NetInfo, restapi.NetworkInfo{IPAddr: ni.IPAddr, Port: int(ni.Port)})
		}
		nisdsByDev[devID] = append(nisdsByDev[devID], node)
	}

	// Devices grouped by hypervisor id, carrying their NISDs.
	devsByHV := make(map[string][]restapi.DeviceNode)
	for _, d := range devRL.Devices {
		devsByHV[d.HypervisorID] = append(devsByHV[d.HypervisorID], restapi.DeviceNode{
			ID:            d.ID,
			Name:          d.Name,
			DevicePath:    d.DevicePath,
			SerialNumber:  d.SerialNumber,
			State:         int(d.State),
			Size:          d.Size,
			FailureDomain: d.FailureDomain,
			NISDs:         nisdsByDev[d.ID],
		})
	}

	// Hypervisors grouped by rack id, carrying their devices.
	hvsByRack := make(map[string][]restapi.HypervisorNode)
	for _, h := range hvRL.Hypervisors {
		hvsByRack[h.RackID] = append(hvsByRack[h.RackID], restapi.HypervisorNode{
			ID:          h.ID,
			Name:        h.Name,
			PortRange:   h.PortRange,
			SSHPort:     h.SSHPort,
			RDMAEnabled: h.RDMAEnabled,
			IPAddrs:     h.IPAddrs,
			Devices:     devsByHV[h.ID],
		})
	}

	// Racks grouped by pdu id, carrying their hypervisors.
	racksByPDU := make(map[string][]restapi.RackNode)
	for _, rk := range rackRL.Racks {
		racksByPDU[rk.PDUID] = append(racksByPDU[rk.PDUID], restapi.RackNode{
			ID:            rk.ID,
			Name:          rk.Name,
			Location:      rk.Location,
			Specification: rk.Specification,
			Hypervisors:   hvsByRack[rk.ID],
		})
	}

	infra := restapi.CreateInfraRequest{}
	for _, p := range pduRL.PDUs {
		infra.PDUs = append(infra.PDUs, restapi.PDUNode{
			ID:            p.ID,
			Name:          p.Name,
			Location:      p.Location,
			Specification: p.Specification,
			PowerCap:      p.PowerCapacity,
			Racks:         racksByPDU[p.ID],
		})
	}
	restapi.WriteJSON(w, http.StatusOK, restapi.GetInfraResponse{Success: true, Infra: infra})
}
