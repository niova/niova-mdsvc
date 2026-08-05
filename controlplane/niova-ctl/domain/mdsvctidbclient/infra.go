package mdsvctidbclient

import (
	"encoding/json"
	"net/url"
	"strings"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	"github.com/00pauln00/niova-mdsvc/controlplane/niova-ctl/domain"
)

// flexBool decodes both JSON booleans and 0/1 integers into a bool.
// mdsvc-tidb's PUT /api/resource accepts a real JSON boolean for
// rdma_enabled but its GET /api/resource echoes it back as 0/1 (confirmed
// against the live server), so a plain `bool` field would fail to decode.
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	switch strings.TrimSpace(string(data)) {
	case "true", "1":
		*b = true
	case "false", "0", "null":
		*b = false
	default:
		var v bool
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		*b = flexBool(v)
	}
	return nil
}

// --- wire shapes (mdsvc-tidb's internal/models field names) ---

type wirePDU struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name,omitempty"`
	Location      string `json:"location,omitempty"`
	Specification string `json:"specification,omitempty"`
	PowerCap      string `json:"power_cap,omitempty"`
}

type wireRack struct {
	ID            string `json:"id,omitempty"`
	PDUID         string `json:"pdu_id,omitempty"`
	Name          string `json:"name,omitempty"`
	Location      string `json:"location,omitempty"`
	Specification string `json:"specification,omitempty"`
}

type wireHypervisor struct {
	ID          string   `json:"id,omitempty"`
	RackID      string   `json:"rack_id,omitempty"`
	Name        string   `json:"name,omitempty"`
	PortRange   string   `json:"port_range,omitempty"`
	SSHPort     string   `json:"ssh_port,omitempty"`
	RDMAEnabled flexBool `json:"rdma_enabled"`
	IPAddrs     []string `json:"ip_addrs,omitempty"`
}

type wireDevice struct {
	ID            string `json:"id,omitempty"`
	HypervisorID  string `json:"hypervisor_id,omitempty"`
	Name          string `json:"name,omitempty"`
	DevicePath    string `json:"device_path,omitempty"`
	SerialNumber  string `json:"serial_number,omitempty"`
	State         int    `json:"state"`
	Size          int64  `json:"size,omitempty"`
	FailureDomain string `json:"failure_domain,omitempty"`
}

// --- PDU/Rack/Hypervisor/Device <-> wire conversions ---

func pduToWire(p *ctlplfl.PDU) wirePDU {
	return wirePDU{ID: p.ID, Name: p.Name, Location: p.Location, Specification: p.Specification, PowerCap: p.PowerCapacity}
}

func rackToWire(r *ctlplfl.Rack) wireRack {
	return wireRack{ID: r.ID, PDUID: r.PDUID, Name: r.Name, Location: r.Location, Specification: r.Specification}
}

func hypervisorToWire(h *ctlplfl.Hypervisor) wireHypervisor {
	return wireHypervisor{ID: h.ID, RackID: h.RackID, Name: h.Name, PortRange: h.PortRange, SSHPort: h.SSHPort, RDMAEnabled: flexBool(h.RDMAEnabled), IPAddrs: h.IPAddrs}
}

func deviceToWire(d *ctlplfl.Device) wireDevice {
	return wireDevice{ID: d.ID, HypervisorID: d.HypervisorID, Name: d.Name, DevicePath: d.DevicePath, SerialNumber: d.SerialNumber, State: int(d.State), Size: d.Size, FailureDomain: d.FailureDomain}
}

// --- writes: PUT /api/resource?type=... ---

func (c *Client) PutPDU(pdu *ctlplfl.PDU) error {
	return c.put("/api/resource", url.Values{"type": {"pdu"}}, pduToWire(pdu), nil)
}

func (c *Client) PutRack(rack *ctlplfl.Rack) error {
	return c.put("/api/resource", url.Values{"type": {"rack"}}, rackToWire(rack), nil)
}

func (c *Client) PutHypervisor(hv *ctlplfl.Hypervisor) error {
	return c.put("/api/resource", url.Values{"type": {"hypervisor"}}, hypervisorToWire(hv), nil)
}

func (c *Client) PutDevice(dev *ctlplfl.Device) error {
	return c.put("/api/resource", url.Values{"type": {"device"}}, deviceToWire(dev), nil)
}

// --- reads: GET /api/resource?type=... (flat lists) ---

func (c *Client) getPDUs() ([]ctlplfl.PDU, error) {
	var wire []wirePDU
	if err := c.getResourceList("pdu", &wire); err != nil {
		return nil, err
	}
	out := make([]ctlplfl.PDU, len(wire))
	for i, w := range wire {
		out[i] = ctlplfl.PDU{ID: w.ID, Name: w.Name, Location: w.Location, Specification: w.Specification, PowerCapacity: w.PowerCap}
	}
	return out, nil
}

func (c *Client) getRacks() ([]ctlplfl.Rack, error) {
	var wire []wireRack
	if err := c.getResourceList("rack", &wire); err != nil {
		return nil, err
	}
	out := make([]ctlplfl.Rack, len(wire))
	for i, w := range wire {
		out[i] = ctlplfl.Rack{ID: w.ID, PDUID: w.PDUID, Name: w.Name, Location: w.Location, Specification: w.Specification}
	}
	return out, nil
}

func (c *Client) getHypervisors() ([]ctlplfl.Hypervisor, error) {
	var wire []wireHypervisor
	if err := c.getResourceList("hypervisor", &wire); err != nil {
		return nil, err
	}
	out := make([]ctlplfl.Hypervisor, len(wire))
	for i, w := range wire {
		out[i] = ctlplfl.Hypervisor{ID: w.ID, RackID: w.RackID, Name: w.Name, PortRange: w.PortRange, SSHPort: w.SSHPort, RDMAEnabled: bool(w.RDMAEnabled), IPAddrs: w.IPAddrs}
	}

	// Server-side gap: GET /api/resource?type=hypervisor never returns
	// ip_addrs, unlike GET /api/infra's nested tree. Work around it by
	// fetching that tree just for the IPs and merging them in by ID.
	// Standalone (rack-less) hypervisors aren't covered — that tree is rooted at PDU→Rack.
	if ips, err := c.hypervisorIPsFromInfra(); err == nil {
		for i := range out {
			if len(out[i].IPAddrs) == 0 {
				out[i].IPAddrs = ips[out[i].ID]
			}
		}
	}

	return out, nil
}

// wireInfraTree mirrors just the id/ip_addrs slice of GET /api/infra's
// nested payload — the only fields hypervisorIPsFromInfra needs.
type wireInfraTree struct {
	PDUs []struct {
		Racks []struct {
			Hypervisors []struct {
				ID      string   `json:"id"`
				IPAddrs []string `json:"ip_addrs"`
			} `json:"hypervisors"`
		} `json:"racks"`
	} `json:"pdus"`
}

// hypervisorIPsFromInfra returns hypervisor ID -> IP addresses by walking
// GET /api/infra's tree, which (unlike the flat hypervisor resource list —
// see getHypervisors) does include them.
func (c *Client) hypervisorIPsFromInfra() (map[string][]string, error) {
	var resp struct {
		Infra wireInfraTree `json:"infra"`
	}
	if err := c.get("/api/infra", nil, &resp); err != nil {
		return nil, err
	}
	ips := make(map[string][]string)
	for _, p := range resp.Infra.PDUs {
		for _, r := range p.Racks {
			for _, hv := range r.Hypervisors {
				ips[hv.ID] = hv.IPAddrs
			}
		}
	}
	return ips, nil
}

func (c *Client) getDevices() ([]ctlplfl.Device, error) {
	var wire []wireDevice
	if err := c.getResourceList("device", &wire); err != nil {
		return nil, err
	}
	out := make([]ctlplfl.Device, len(wire))
	for i, w := range wire {
		out[i] = ctlplfl.Device{ID: w.ID, HypervisorID: w.HypervisorID, Name: w.Name, DevicePath: w.DevicePath, SerialNumber: w.SerialNumber, State: uint16(w.State), Size: w.Size, FailureDomain: w.FailureDomain}
	}
	return out, nil
}

// getResourceList decodes GET /api/resource?type=<rtype>'s "resources" array
// (a raw JSON array in the envelope) into out (a pointer to a slice).
func (c *Client) getResourceList(rtype string, out any) error {
	var resp struct {
		Type      string          `json:"type"`
		Resources json.RawMessage `json:"resources"`
	}
	if err := c.get("/api/resource", url.Values{"type": {rtype}}, &resp); err != nil {
		return err
	}
	if len(resp.Resources) == 0 || string(resp.Resources) == "null" {
		return nil
	}
	return json.Unmarshal(resp.Resources, out)
}

// RefreshTopology fetches PDUs/Racks/Hypervisors/Devices as flat lists and
// stitches them via domain.StitchTopology. mdsvc-tidb has no device_partition
// resource type at all (see constants.APIResourceTypeIndexes/
// APIMutableResourceTypeIndexes in mdsvc-tidb itself — DevicePartition is in
// neither), so unlike niova-mdsvc there's no partitions fetch here — an empty
// slice is passed through, matching StitchTopology's "no partitions" shape.
func (c *Client) RefreshTopology(token string) ([]ctlplfl.PDU, []ctlplfl.Rack, []ctlplfl.Hypervisor, error) {
	c.SetToken(token)

	pdus, err := c.getPDUs()
	if err != nil {
		return nil, nil, nil, err
	}
	racks, err := c.getRacks()
	if err != nil {
		return nil, nil, nil, err
	}
	hvs, err := c.getHypervisors()
	if err != nil {
		return nil, nil, nil, err
	}
	devices, err := c.getDevices()
	if err != nil {
		return nil, nil, nil, err
	}

	stitchedPDUs, stitchedRacks, standaloneHVs := domain.StitchTopology(pdus, racks, hvs, devices, nil)
	return stitchedPDUs, stitchedRacks, standaloneHVs, nil
}
