package clictlplanefuncs

import (
	"testing"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)
// 2 PDUs
var pdus = []string{
	"a1d3f8c2-df29-11f0-b14e-3c9eaf21d540001",
	"a1d3f8c2-df29-11f0-b14e-3c9eaf21d540002",
}

// 2 RACKS
var racks = []string{
	"b7e91a4d-df29-11f0-9f6a-8a42bc7de111",
	"b7e91a4d-df29-11f0-9f6a-8a42bc7de112",
}

// 4 HVs
var hvs = []string{
	"c02ab9e7-df29-11f0-8d3c-5e91f4aa98231",
	"c02ab9e7-df29-11f0-8d3c-5e91f4aa98232",
	"c02ab9e7-df29-11f0-8d3c-5e91f4aa98233",
	"c02ab9e7-df29-11f0-8d3c-5e91f4aa98234",
}

// 8 Devices
var devices = []string{
	"nvme-ab6358164001",
	"nvme-ab6358164002",
	"nvme-ab6358164003",
	"nvme-ab6358164004",
	"nvme-ab6358164005",
	"nvme-ab6358164006",
	"nvme-ab6358164007",
	"nvme-ab6358164008",
}

func TestCreateHierarchy(t *testing.T) {
	c := newClient(t)

	PDUs := []cpLib.PDU{
		{	ID: 		   pdus[0],
			Name:          "pdu-1",
			Location:      "us-east",
			PowerCapacity: "15Kw",
			Specification: "specification1",
		},
		{	ID: 		   pdus[1],
			Name:          "pdu-2",
			Location:      "us-west",
			PowerCapacity: "15Kw",
			Specification: "specification2",
		},
	}

	// PUT multiple PDUs
	for _, p := range PDUs {
		resp, err := c.PutPDU(&p)
		assert.NoError(t, err)
		assert.True(t, resp.Success)
	}

	Racks := []cpLib.Rack{
		{ID: racks[0], PDUID: pdus[0], Name: "rack-1", Location: "us-east", Specification: "rack1-spec"},
		{ID: racks[1], PDUID: pdus[1], Name: "rack-2", Location: "us-west", Specification: "rack2-spec"},
	}

	// PUT multiple Racks
	for _, r := range Racks {
		resp, err := c.PutRack(&r)
		assert.NoError(t, err)
		assert.True(t, resp.Success)
	}

	Hypervisors := []cpLib.Hypervisor{
		{RackID: racks[0], ID: hvs[0], IPAddrs: []string{"127.0.0.1", "127.0.0.1"}, PortRange: "8000-9000", SSHPort: "5999", Name: "hv-1"},
		{RackID: racks[0], ID: hvs[1], IPAddrs: []string{"127.0.0.1", "127.0.0.1"}, PortRange: "5000-7000", SSHPort: "6999", Name: "hv-2"},
		{RackID: racks[1], ID: hvs[2], IPAddrs: []string{"127.0.0.1", "127.0.0.1"}, PortRange: "2000-3000", SSHPort: "7999", Name: "hv-3"},
		{RackID: racks[1], ID: hvs[3], IPAddrs: []string{"127.0.0.1", "127.0.0.1"}, PortRange: "4000-5000", SSHPort: "8999", Name: "hv-4"},
	}

	// PUT multiple hypervisor
	for _, hv := range Hypervisors {
		resp, err := c.PutHypervisor(&hv)
		assert.NoError(t, err)
		assert.True(t, resp.Success)
	}

	mockDevices := []cpLib.Device{
		{
			ID:            devices[0],
			SerialNumber:  "SN123456781",
			State:         1,
			HypervisorID:  hvs[0],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path1",
			Name:          "dev-1",
		},
		{
			ID:            devices[1],
			SerialNumber:  "SN123456782",
			State:         1,
			HypervisorID:  hvs[0],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path2",
			Name:          "dev-2",
		},
		{
			ID:            devices[2],
			SerialNumber:  "SN123456783",
			State:         1,
			HypervisorID:  hvs[1],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path3",
			Name:          "dev-3",
		},
		{
			ID:            devices[3],
			SerialNumber:  "SN123456784",
			State:         1,
			HypervisorID:  hvs[1],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path4",
			Name:          "dev-4",
		},
		{
			ID:            devices[4],
			SerialNumber:  "SN123456785",
			State:         1,
			HypervisorID:  hvs[2],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path5",
			Name:          "dev-5",
		},
		{
			ID:            devices[5],
			SerialNumber:  "SN123456786",
			State:         1,
			HypervisorID:  hvs[2],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path6",
			Name:          "dev-6",
		},
		{
			ID:            devices[6],
			SerialNumber:  "SN123456787",
			State:         1,
			HypervisorID:  hvs[3],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path7",
			Name:          "dev-7",
		},
		{
			ID:            devices[7],
			SerialNumber:  "SN123456788",
			State:         1,
			HypervisorID:  hvs[3],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path8",
			Name:          "dev-8",
		},

	}

	// PUT multiple devices
	for _, d := range mockDevices {
		resp, err := c.PutDevice(&d)
		assert.NoError(t, err)
		assert.True(t, resp.Success)
	}

	mockNisds := []cpLib.Nisd{
		cpLib.Nisd{
			PeerPort: 8000,
			ID:       "d8f4c311-df29-11f0-a27b-19cd40ab5601",
			FailureDomain: []string{
				pdus[0],
				racks[0],
				hvs[0],
				devices[0],
			},
			TotalSize:     1024*1024*1024*1024,    // 1 TB
			AvailableSize: 1024*1024*1024*1024,
			SocketPath:    "/path/sockets1",
			NetInfo: cpLib.NetInfoList{
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.1",
					Port:   5444,
				},
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.2",
					Port:   6444,
				},
			},
			NetInfoCnt: 2,
		},
		cpLib.Nisd{
			PeerPort: 8001,
			ID:       "d8f4c311-df29-11f0-a27b-19cd40ab5602",
			FailureDomain: []string{
				pdus[0],
				racks[0],
				hvs[0],
				devices[1],
			},
			TotalSize:     1024*1024*1024*1024,
			AvailableSize: 1024*1024*1024*1024,
			SocketPath:    "/path/sockets2",
			NetInfo: cpLib.NetInfoList{
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.1",
					Port:   5444,
				},
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.2",
					Port:   6444,
				},
			},
			NetInfoCnt: 2,
		},
		cpLib.Nisd{
			PeerPort: 8002,
			ID:       "d8f4c311-df29-11f0-a27b-19cd40ab5603",
			FailureDomain: []string{
				pdus[0],
				racks[0],
				hvs[1],
				devices[2],
			},
			TotalSize:     1024*1024*1024*1024,
			AvailableSize: 1024*1024*1024*1024,
			SocketPath:    "/path/sockets3",
			NetInfo: cpLib.NetInfoList{
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.1",
					Port:   5444,
				},
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.2",
					Port:   6444,
				},
			},
			NetInfoCnt: 2,
		},
		cpLib.Nisd{
			PeerPort: 8003,
			ID:       "d8f4c311-df29-11f0-a27b-19cd40ab5604",
			FailureDomain: []string{
				pdus[0],
				racks[0],
				hvs[1],
				devices[3],
			},
			TotalSize:     1024*1024*1024*1024,
			AvailableSize: 1024*1024*1024*1024,
			SocketPath:    "/path/sockets4",
			NetInfo: cpLib.NetInfoList{
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.1",
					Port:   5444,
				},
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.2",
					Port:   6444,
				},
			},
			NetInfoCnt: 2,
		},
		cpLib.Nisd{
			PeerPort: 8004,
			ID:       "d8f4c311-df29-11f0-a27b-19cd40ab5605",
			FailureDomain: []string{
				pdus[0],
				racks[0],
				hvs[2],
				devices[4],
			},
			TotalSize:     1024*1024*1024*1024,
			AvailableSize: 1024*1024*1024*1024,
			SocketPath:    "/path/sockets5",
			NetInfo: cpLib.NetInfoList{
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.1",
					Port:   5444,
				},
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.2",
					Port:   6444,
				},
			},
			NetInfoCnt: 2,
		},
		cpLib.Nisd{
			PeerPort: 8005,
			ID:       "d8f4c311-df29-11f0-a27b-19cd40ab5606",
			FailureDomain: []string{
				pdus[1],
				racks[1],
				hvs[2],
				devices[5],
			},
			TotalSize:     1024*1024*1024*1024,
			AvailableSize: 1024*1024*1024*1024,
			SocketPath:    "/path/sockets6",
			NetInfo: cpLib.NetInfoList{
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.1",
					Port:   5444,
				},
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.2",
					Port:   6444,
				},
			},
			NetInfoCnt: 2,
		},
		cpLib.Nisd{
			PeerPort: 8006,
			ID:       "d8f4c311-df29-11f0-a27b-19cd40ab5607",
			FailureDomain: []string{
				pdus[1],
				racks[1],
				hvs[3],
				devices[6],
			},
			TotalSize:     1024*1024*1024*1024,
			AvailableSize: 1024*1024*1024*1024,
			SocketPath:    "/path/sockets7",
			NetInfo: cpLib.NetInfoList{
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.1",
					Port:   5444,
				},
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.2",
					Port:   6444,
				},
			},
			NetInfoCnt: 2,
		},
		cpLib.Nisd{
			PeerPort: 8007,
			ID:       "d8f4c311-df29-11f0-a27b-19cd40ab5608",
			FailureDomain: []string{
				pdus[1],
				racks[1],
				hvs[3],
				devices[7],
			},
			TotalSize:     1024*1024*1024*1024,
			AvailableSize: 1024*1024*1024*1024,
			SocketPath:    "/path/sockets8",
			NetInfo: cpLib.NetInfoList{
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.1",
					Port:   5444,
				},
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.2",
					Port:   6444,
				},
			},
			NetInfoCnt: 2,
		},
		cpLib.Nisd{
			PeerPort: 8008,
			ID:       "d8f4c311-df29-11f0-a27b-19cd40ab5609",
			FailureDomain: []string{
				pdus[1],
				racks[1],
				hvs[0],
				devices[0],
			},
			TotalSize:     1024*1024*1024*1024,
			AvailableSize: 1024*1024*1024*1024,
			SocketPath:    "/path/sockets9",
			NetInfo: cpLib.NetInfoList{
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.1",
					Port:   5444,
				},
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.2",
					Port:   6444,
				},
			},
			NetInfoCnt: 2,
		},
		cpLib.Nisd{
			PeerPort: 8009,
			ID:       "d8f4c311-df29-11f0-a27b-19cd40ab5610",
			FailureDomain: []string{
				pdus[1],
				racks[1],
				hvs[1],
				devices[1],
			},
			TotalSize:     1024*1024*1024*1024,
			AvailableSize: 1024*1024*1024*1024,
			SocketPath:    "/path/sockets10",
			NetInfo: cpLib.NetInfoList{
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.1",
					Port:   5444,
				},
				cpLib.NetworkInfo{
					IPAddr: "192.168.0.0.2",
					Port:   6444,
				},
			},
			NetInfoCnt: 2,
		},
		
	}

	for _, n := range mockNisds {
		resp, err := c.PutNisd(&n)
		assert.NoError(t, err)
		assert.True(t, resp.Success)
	}

	vdev := &cpLib.VdevReq{
		Vdev: &cpLib.VdevCfg{
			Size: 1024 * 1024 * 1024 * 1024,    // 1 TB
			NumReplica: 2,
		},
	}

	resp, err := c.CreateVdev(vdev)
	assert.NoError(t, err, "failed to create vdev")
	assert.NotEmpty(t, resp, "vdev ID should not be empty")
	log.Info("Created vdev: ", resp.ID)
}

func TestCreateVdevAfterLeaderKill(t *testing.T) {
	c := newClient(t)

	vdev1 := &cpLib.VdevReq{
		Vdev: &cpLib.VdevCfg{
			Size: 200 * 1024 * 1024 * 1024,     // 200 GB
			NumReplica: 2,
		},
	}

	resp, err := c.CreateVdev(vdev1)
	assert.NoError(t, err, "failed to create vdev1")
	assert.NotEmpty(t, resp, "vdev ID should not be empty")
	log.Info("Created vdev1: ", resp.ID)
}

func TestCreateVdevAfterLeaderRestart(t *testing.T) {
	c := newClient(t)

	Nisds := []cpLib.Nisd{
		cpLib.Nisd{
			PeerPort: 7000,
			ID:       "d8f4c311-df29-11f0-a27b-19cd40ab5611",
			FailureDomain: []string{
				pdus[0],
				racks[0],
				hvs[2],
				devices[2],
			},
			TotalSize:     1024*1024*1024*1024,    // 1 TB
			AvailableSize: 1024*1024*1024*1024,
		},
		cpLib.Nisd{
			PeerPort: 7001,
			ID:       "d8f4c311-df29-11f0-a27b-19cd40ab5612",
			FailureDomain: []string{
				pdus[1],
				racks[1],
				hvs[3],
				devices[3],
			},
			TotalSize:     1024*1024*1024*1024,
			AvailableSize: 1024*1024*1024*1024,
		},
	}

	for _, n := range Nisds {
		resp, err := c.PutNisd(&n)
		assert.NoError(t, err)
		assert.True(t, resp.Success)
	}

	vdev2 := &cpLib.VdevReq{
		Vdev: &cpLib.VdevCfg{
			Size: 200 * 1024 * 1024 * 1024,     // 200 GB
			NumReplica: 2,
		},
	}

	resp, err := c.CreateVdev(vdev2)
	assert.NoError(t, err, "failed to create vdev2")
	assert.NotEmpty(t, resp, "vdev ID should not be empty")
	log.Info("Created vdev2: ", resp.ID)
}