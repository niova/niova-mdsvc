package mdsvctidbclient

import (
	"net/url"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

type wireNetInfo struct {
	IPAddr string `json:"ip_addr"`
	Port   int    `json:"port"`
}

// wireNISD mirrors mdsvc-tidb's models.NISD. Failure-domain is four explicit
// parent-resource IDs here, where niova-ctl's ctlplfl.Nisd carries a single
// FailureDomain []string — see FdToWire/FdFromWire below for the (best
// effort) mapping between the two.
type wireNISD struct {
	ID            string        `json:"id,omitempty"`
	PeerPort      int           `json:"peer_port"`
	FDPduID       string        `json:"fd_pdu_id,omitempty"`
	FDRackID      string        `json:"fd_rack_id,omitempty"`
	FDHvID        string        `json:"fd_hv_id,omitempty"`
	FDDeviceID    string        `json:"fd_device_id,omitempty"`
	TotalSize     int64         `json:"total_size,omitempty"`
	AvailableSize int64         `json:"available_size,omitempty"`
	SocketPath    string        `json:"socket_path,omitempty"`
	NetInfo       []wireNetInfo `json:"net_info,omitempty"`
}

// nisdFailureDomain packs mdsvc-tidb's four FK fields into the single
// []string Nisd.FailureDomain expects ([pdu, rack, hv, device], empty
// entries dropped). The NISD form doesn't collect these, so writes leave
// them empty — a known limitation, not solved here.
func nisdFailureDomain(w wireNISD) []string {
	var fd []string
	for _, id := range []string{w.FDPduID, w.FDRackID, w.FDHvID, w.FDDeviceID} {
		if id != "" {
			fd = append(fd, id)
		}
	}
	return fd
}

func nisdToWire(n *ctlplfl.Nisd) wireNISD {
	w := wireNISD{
		ID:            n.ID,
		PeerPort:      int(n.PeerPort),
		TotalSize:     n.TotalSize,
		AvailableSize: n.AvailableSize,
		SocketPath:    n.SocketPath,
	}
	for _, ni := range n.NetInfo {
		w.NetInfo = append(w.NetInfo, wireNetInfo{IPAddr: ni.IPAddr, Port: int(ni.Port)})
	}
	// Best-effort: if FailureDomain happens to carry [pdu, rack, hv, device]
	// (the order we read it back in), thread it through on write too.
	fd := n.FailureDomain
	if len(fd) > 0 {
		w.FDPduID = fd[0]
	}
	if len(fd) > 1 {
		w.FDRackID = fd[1]
	}
	if len(fd) > 2 {
		w.FDHvID = fd[2]
	}
	if len(fd) > 3 {
		w.FDDeviceID = fd[3]
	}
	return w
}

func nisdFromWire(w wireNISD) ctlplfl.Nisd {
	n := ctlplfl.Nisd{
		ID:            w.ID,
		PeerPort:      uint16(w.PeerPort),
		FailureDomain: nisdFailureDomain(w),
		TotalSize:     w.TotalSize,
		AvailableSize: w.AvailableSize,
		SocketPath:    w.SocketPath,
		NetInfoCnt:    len(w.NetInfo),
	}
	for _, ni := range w.NetInfo {
		n.NetInfo = append(n.NetInfo, ctlplfl.NetworkInfo{IPAddr: ni.IPAddr, Port: uint16(ni.Port)})
	}
	return n
}

func (c *Client) PutNisd(nisd *ctlplfl.Nisd) error {
	return c.put("/api/resource", url.Values{"type": {"nisd"}}, nisdToWire(nisd), nil)
}

func (c *Client) GetNisds() ([]ctlplfl.Nisd, error) {
	var wire []wireNISD
	if err := c.getResourceList("nisd", &wire); err != nil {
		return nil, err
	}
	out := make([]ctlplfl.Nisd, len(wire))
	for i, w := range wire {
		out[i] = nisdFromWire(w)
	}
	return out, nil
}
