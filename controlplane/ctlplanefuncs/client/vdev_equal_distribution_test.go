package clictlplanefuncs

import (
	"fmt"
	"sync"
	"testing"
	"time"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestEqualVdevDistribution(t *testing.T) {
	c := newClient(t)

	// 2 PDUs
	pdus := []string{
		"b7e91a4d-df29-11f0-9f6a-8a7c12e9a3bc001",
		"b7e91a4d-df29-11f0-9f6a-8a7c12e9a3bc002",
	}

	// 2 RACKS per PDU
	racks := []string{
		"c4f29b91-df29-11f0-8c3a-6f1d9ab7e001",
		"c4f29b91-df29-11f0-8c3a-6f1d9ab7e002",
		"c4f29b91-df29-11f0-8c3a-6f1d9ab7e003",
		"c4f29b91-df29-11f0-8c3a-6f1d9ab7e004",
	}

	// 2 HVs per Rack
	hvs := []string{
		"4e72c1ad-df29-11f0-a8f4-6d91b2e5c1001",
		"4e72c1ad-df29-11f0-a8f4-6d91b2e5c1002",
		"4e72c1ad-df29-11f0-a8f4-6d91b2e5c1003",
		"4e72c1ad-df29-11f0-a8f4-6d91b2e5c1004",
		"4e72c1ad-df29-11f0-a8f4-6d91b2e5c1005",
		"4e72c1ad-df29-11f0-a8f4-6d91b2e5c1006",
		"4e72c1ad-df29-11f0-a8f4-6d91b2e5c1007",
		"4e72c1ad-df29-11f0-a8f4-6d91b2e5c1008",
	}

	// 2 Devices per HVs
	devices := []string{
		"nvme-cf91a2e71001",
		"nvme-cf91a2e71002",
		"nvme-cf91a2e71003",
		"nvme-cf91a2e71004",
		"nvme-cf91a2e71005",
		"nvme-cf91a2e71006",
		"nvme-cf91a2e71007",
		"nvme-cf91a2e71008",
		"nvme-cf91a2e71009",
		"nvme-cf91a2e71010",
		"nvme-cf91a2e71011",
		"nvme-cf91a2e71012",
		"nvme-cf91a2e71013",
		"nvme-cf91a2e71014",
		"nvme-cf91a2e71015",
		"nvme-cf91a2e71016",
	}

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
		{ID: racks[1], PDUID: pdus[0], Name: "rack-2", Location: "us-east", Specification: "rack2-spec"},
		{ID: racks[2], PDUID: pdus[1], Name: "rack-3", Location: "us-west", Specification: "rack3-spec"},
    	{ID: racks[3], PDUID: pdus[1], Name: "rack-4", Location: "us-west", Specification: "rack4-spec"},
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
		{RackID: racks[2], ID: hvs[4], IPAddrs: []string{"127.0.0.1", "127.0.0.1"}, PortRange: "1000-2000", SSHPort: "5998", Name: "hv-5"},
    	{RackID: racks[2], ID: hvs[5], IPAddrs: []string{"127.0.0.1", "127.0.0.1"}, PortRange: "3000-4000", SSHPort: "6998", Name: "hv-6"},
    	{RackID: racks[3], ID: hvs[6], IPAddrs: []string{"127.0.0.1", "127.0.0.1"}, PortRange: "6000-7000", SSHPort: "7998", Name: "hv-7"},
    	{RackID: racks[3], ID: hvs[7], IPAddrs: []string{"127.0.0.1", "127.0.0.1"}, PortRange: "7000-8000", SSHPort: "8998", Name: "hv-8"},
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
		{
			ID:            devices[8],
			SerialNumber:  "SN123456789",
			State:         1,
			HypervisorID:  hvs[4],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path9",
			Name:          "dev-9",
		},
		{
			ID:            devices[9],
			SerialNumber:  "SN123456790",
			State:         1,
			HypervisorID:  hvs[4],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path10",
			Name:          "dev-10",
		},
		{
			ID:            devices[10],
			SerialNumber:  "SN123456791",
			State:         1,
			HypervisorID:  hvs[5],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path11",
			Name:          "dev-11",
		},
		{
			ID:            devices[11],
			SerialNumber:  "SN123456792",
			State:         1,
			HypervisorID:  hvs[5],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path12",
			Name:          "dev-12",
		},
		{
			ID:            devices[12],
			SerialNumber:  "SN123456793",
			State:         1,
			HypervisorID:  hvs[6],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path13",
			Name:          "dev-13",
		},
		{
			ID:            devices[13],
			SerialNumber:  "SN123456794",
			State:         1,
			HypervisorID:  hvs[6],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path14",
			Name:          "dev-14",
		},
		{
			ID:            devices[14],
			SerialNumber:  "SN123456795",
			State:         1,
			HypervisorID:  hvs[7],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path15",
			Name:          "dev-15",
		},
		{
			ID:            devices[15],
			SerialNumber:  "SN123456796",
			State:         1,
			HypervisorID:  hvs[7],
			FailureDomain: "fd-01",
			DevicePath:    "/temp/path16",
			Name:          "dev-16",
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
		},
		
	}

	for _, n := range mockNisds {
		resp, err := c.PutNisd(&n)
		assert.NoError(t, err)
		assert.True(t, resp.Success)
	}

	mockNisd := make([]cpLib.Nisd, 0, 160)

	pduCount := len(pdus)
	rackPerPdu := 2
	hvPerRack := 2
	devPerHv := 2
	nisdPerDev := 4

	rackIdx := 0
	hvIdx := 0
	devIdx := 0

	nisdID := 1

	for p := 0; p < pduCount; p++ {
		pdu := pdus[p]

		for r := 0; r < rackPerPdu; r++ {
			rack := racks[rackIdx]
			rackIdx++

			for h := 0; h < hvPerRack; h++ {
				hv := hvs[hvIdx]
				hvIdx++

				for d := 0; d < devPerHv; d++ {
					dev := devices[devIdx]
					devIdx++

					for n := 0; n < nisdPerDev; n++ {
						nisd := cpLib.Nisd{
							PeerPort: 8000 + uint16(nisdID),
							ID:       fmt.Sprintf("ab8015c3-2a96-4f3a-5a0b-%012x", nisdID),
							FailureDomain: []string{
								pdu,
								rack,
								hv,
								dev,
							},
							TotalSize:     1024 * 1024 * 1024 * 1024,    // 1 TB
							AvailableSize: 1024 * 1024 * 1024 * 1024,	 // 1 TB
						}

						mockNisd = append(mockNisd, nisd)
						nisdID++
					}
				}
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for _, n := range mockNisd {
			resp, err := c.PutNisd(&n)
			assert.NoError(t, err)
			assert.True(t, resp.Success)
			time.Sleep(10 * time.Millisecond)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 144; i++ {
			vdev := &cpLib.VdevReq{
				Vdev: &cpLib.VdevCfg{
					Size:       1024 * 1024 * 1024 * 1024,
					NumReplica: 1,
				},
			}
			_, err := c.CreateVdev(vdev)
			assert.NoError(t, err)
			time.Sleep(30 * time.Millisecond)
		}
	}()

	wg.Wait()

	log.Info("Created vdev: ")

}