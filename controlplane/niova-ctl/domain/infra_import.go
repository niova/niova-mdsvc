package domain

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

type ImportNetInfo struct {
	IPAddr string `json:"ip_addr"`
	Port   uint16 `json:"port"`
}

type ImportNISD struct {
	ID            string          `json:"id,omitempty"`
	PeerPort      uint16          `json:"peer_port,omitempty"`
	TotalSize     int64           `json:"total_size,omitempty"`
	AvailableSize int64           `json:"available_size,omitempty"`
	SocketPath    string          `json:"socket_path,omitempty"`
	NetInfo       []ImportNetInfo `json:"net_info,omitempty"`
}

type ImportDevice struct {
	ID            string       `json:"id,omitempty"`
	Name          string       `json:"name,omitempty"`
	DevicePath    string       `json:"device_path,omitempty"`
	SerialNumber  string       `json:"serial_number,omitempty"`
	State         int          `json:"state,omitempty"`
	Size          int64        `json:"size,omitempty"`
	FailureDomain string       `json:"failure_domain,omitempty"`
	NISDs         []ImportNISD `json:"nisds,omitempty"`
}

type ImportHypervisor struct {
	ID          string         `json:"id,omitempty"`
	Name        string         `json:"name,omitempty"`
	PortRange   string         `json:"port_range,omitempty"`
	SSHPort     string         `json:"ssh_port,omitempty"`
	RDMAEnabled bool           `json:"rdma_enabled,omitempty"`
	IPAddrs     []string       `json:"ip_addrs,omitempty"`
	Devices     []ImportDevice `json:"devices,omitempty"`
}

type ImportRack struct {
	ID            string             `json:"id,omitempty"`
	Name          string             `json:"name,omitempty"`
	Location      string             `json:"location,omitempty"`
	Specification string             `json:"specification,omitempty"`
	Hypervisors   []ImportHypervisor `json:"hypervisors,omitempty"`
}

type ImportPDU struct {
	ID            string       `json:"id,omitempty"`
	Name          string       `json:"name,omitempty"`
	Location      string       `json:"location,omitempty"`
	Specification string       `json:"specification,omitempty"`
	PowerCap      string       `json:"power_cap,omitempty"`
	Racks         []ImportRack `json:"racks,omitempty"`
}

// ImportInfraRequest is the top-level document ImportInfra accepts.
type ImportInfraRequest struct {
	PDUs []ImportPDU `json:"pdus"`
}

// --- results ---

// ImportOutcome records what happened to one node in the tree.
type ImportOutcome struct {
	Kind string // "pdu", "rack", "hypervisor", "device", "partition", "nisd"
	ID   string
	Name string // best-effort label — Name field, or SocketPath/DevicePath for nodes without one
	Err  error  // nil on success
}

// ImportResult is the full report of an ImportInfra run — every node
// attempted, in the order it was attempted, success or failure.
type ImportResult struct {
	Outcomes []ImportOutcome
}

// Counts returns how many nodes were created successfully vs. failed.
func (r *ImportResult) Counts() (created, failed int) {
	for _, o := range r.Outcomes {
		if o.Err != nil {
			failed++
		} else {
			created++
		}
	}
	return created, failed
}

// Failures returns just the failed outcomes, in attempt order.
func (r *ImportResult) Failures() []ImportOutcome {
	var out []ImportOutcome
	for _, o := range r.Outcomes {
		if o.Err != nil {
			out = append(out, o)
		}
	}
	return out
}

func (r *ImportResult) record(kind, id, name string, err error) {
	r.Outcomes = append(r.Outcomes, ImportOutcome{Kind: kind, ID: id, Name: name, Err: err})
}

// idOrNew returns id trimmed, or a freshly minted UUID if it was empty —
// matching mdsvc-tidb's "IDs can be omitted (server-generated)" contract
// for every node type in the JSON shape above.
func idOrNew(id string) string {
	if id = strings.TrimSpace(id); id != "" {
		return id
	}
	return uuid.NewString()
}

// ParseImportInfra decodes and lightly validates an import document without
// creating anything — used to build a confirmation summary (node counts)
// before ImportInfra actually starts writing.
func ParseImportInfra(data []byte) (*ImportInfraRequest, error) {
	var req ImportInfraRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if len(req.PDUs) == 0 {
		return nil, fmt.Errorf(`document has no "pdus" — nothing to import`)
	}
	return &req, nil
}

// Counts returns how many of each node type a parsed request contains —
// PDUs, racks, hypervisors, devices, NISDs, in that order.
func (req *ImportInfraRequest) Counts() (pdus, racks, hvs, devices, nisds int) {
	pdus = len(req.PDUs)
	for _, p := range req.PDUs {
		racks += len(p.Racks)
		for _, r := range p.Racks {
			hvs += len(r.Hypervisors)
			for _, h := range r.Hypervisors {
				devices += len(h.Devices)
				for _, d := range h.Devices {
					nisds += len(d.NISDs)
				}
			}
		}
	}
	return pdus, racks, hvs, devices, nisds
}

func ImportInfra(client ControlPlaneClient, req *ImportInfraRequest) *ImportResult {
	result := &ImportResult{}
	partWriter, hasPartitions := client.(PartitionWriter)

	for _, pn := range req.PDUs {
		pdu := ctlplfl.PDU{
			ID:            idOrNew(pn.ID),
			Name:          pn.Name,
			Location:      pn.Location,
			Specification: pn.Specification,
			PowerCapacity: pn.PowerCap,
		}
		err := client.PutPDU(&pdu)
		result.record("pdu", pdu.ID, pdu.Name, err)
		if err != nil {
			continue
		}

		for _, rn := range pn.Racks {
			rack := ctlplfl.Rack{
				ID:            idOrNew(rn.ID),
				PDUID:         pdu.ID,
				Name:          rn.Name,
				Location:      rn.Location,
				Specification: rn.Specification,
			}
			err := client.PutRack(&rack)
			result.record("rack", rack.ID, rack.Name, err)
			if err != nil {
				continue
			}

			for _, hn := range rn.Hypervisors {
				hv := ctlplfl.Hypervisor{
					ID:          idOrNew(hn.ID),
					RackID:      rack.ID,
					Name:        hn.Name,
					PortRange:   hn.PortRange,
					SSHPort:     hn.SSHPort,
					RDMAEnabled: hn.RDMAEnabled,
					IPAddrs:     hn.IPAddrs,
				}
				if hv.SSHPort == "" {
					hv.SSHPort = "22" // matches mdsvc-tidb's documented default
				}
				err := client.PutHypervisor(&hv)
				result.record("hypervisor", hv.ID, hv.Name, err)
				if err != nil {
					continue
				}

				for _, dn := range hn.Devices {
					dev := ctlplfl.Device{
						ID:            idOrNew(dn.ID),
						HypervisorID:  hv.ID,
						Name:          dn.Name,
						DevicePath:    dn.DevicePath,
						SerialNumber:  dn.SerialNumber,
						State:         uint16(dn.State),
						Size:          dn.Size,
						FailureDomain: dn.FailureDomain,
					}
					if dev.FailureDomain == "" {
						dev.FailureDomain = "fd-" + dev.ID // matches mdsvc-tidb's documented default
					}
					err := client.PutDevice(&dev)
					result.record("device", dev.ID, dev.Name, err)
					if err != nil {
						continue
					}

					for _, nn := range dn.NISDs {
						importNISD(client, partWriter, hasPartitions, result, pdu.ID, rack.ID, hv.ID, dev.ID, nn)
					}
				}
			}
		}
	}

	return result
}

// importNISD creates one NISD node, synthesizing a partition ahead of it
// when the backend requires one (see ImportInfra's doc comment).
func importNISD(client ControlPlaneClient, partWriter PartitionWriter, hasPartitions bool, result *ImportResult,
	pduID, rackID, hvID, devID string, nn ImportNISD) {

	fd := []string{pduID, rackID, hvID, devID}
	nisdID := idOrNew(nn.ID)

	if hasPartitions {
		part := ctlplfl.DevicePartition{
			PartitionID:   uuid.NewString(),
			DevID:         devID,
			PartitionPath: nn.SocketPath,
			NISDUUID:      nisdID,
			Size:          nn.TotalSize,
		}
		if err := partWriter.PutPartition(&part); err != nil {
			result.record("partition", part.PartitionID, part.PartitionPath, err)
			return // no valid partition to hang the NISD off of
		}
		result.record("partition", part.PartitionID, part.PartitionPath, nil)
		fd = append(fd, part.PartitionID)
	}

	availSize := nn.AvailableSize
	if availSize == 0 {
		availSize = nn.TotalSize // matches mdsvc-tidb's documented default
	}
	var netInfo ctlplfl.NetInfoList
	for _, ni := range nn.NetInfo {
		netInfo = append(netInfo, ctlplfl.NetworkInfo{IPAddr: ni.IPAddr, Port: ni.Port})
	}

	nisd := ctlplfl.Nisd{
		ID:            nisdID,
		PeerPort:      nn.PeerPort,
		FailureDomain: fd,
		TotalSize:     nn.TotalSize,
		AvailableSize: availSize,
		SocketPath:    nn.SocketPath,
		NetInfo:       netInfo,
	}
	err := client.PutNisd(&nisd)
	result.record("nisd", nisd.ID, nisd.SocketPath, err)
}
