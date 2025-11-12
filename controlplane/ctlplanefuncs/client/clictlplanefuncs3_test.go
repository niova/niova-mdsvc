package clictlplanefuncs

import (
	"testing"
	"fmt"
	"context"
    "time"
    "golang.org/x/sync/errgroup"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var (
	Test_PDUs  = make(map[string]cpLib.PDU)
	Test_Racks = make(map[string]cpLib.Rack)
	Test_Hypervisors = make(map[string]cpLib.Hypervisor)
	Test_Devices = make(map[string]cpLib.Device)
	Test_Nisds = make(map[string]cpLib.Nisd)
)

func TestBulkPDUsAndRacks(t *testing.T) {
	c := newClient(t)

	// -------------------------
	// 1) Create 5 PDUs
	// -------------------------
	locations := []string{
		"us-east", "us-west", "us-central", "us-south", "us-north",
	}

	var pduList []cpLib.PDU

	for i := 1; i <= 5; i++ {
		pdu := cpLib.PDU{
			ID:            fmt.Sprintf("pdu-%02d-uuid", i),
			Name:          fmt.Sprintf("pdu-%d", i),
			Location:      locations[i-1],
			PowerCapacity: "20Kw",
			Specification: fmt.Sprintf("spec-%d", i),
		}

		resp, err := c.PutPDU(&pdu)
		assert.NoError(t, err)
		assert.True(t, resp.Success)

		pduList = append(pduList, pdu)
	}

	// GET all PDUs
	allPdus, err := c.GetPDUs(&cpLib.GetReq{GetAll: true})
	assert.NoError(t, err)
	assert.Equal(t, len(pduList), len(allPdus), "Mismatch PDU count")

	// Store in global PDUs map
	for _, p := range allPdus {
		Test_PDUs[p.ID] = p
	}

	log.Infof("Successfully created and validated %d PDUs", len(allPdus))

	// -------------------------
	// 2) Create 2 racks for each PDU (10 racks total)
	// -------------------------
	totalRacks := 0
	for _, pdu := range allPdus {

		for r := 1; r <= 2; r++ {
			rack := cpLib.Rack{
				ID:            fmt.Sprintf("%s-rack-%02d", pdu.ID, r),
				PDUID:         pdu.ID,
				Name:          fmt.Sprintf("%s-rack-%d", pdu.Name, r),
				Location:      pdu.Location,
				Specification: fmt.Sprintf("spec-%s-%d", pdu.Name, r),
			}

			resp, err := c.PutRack(&rack)
			assert.NoError(t, err)
			assert.True(t, resp.Success)
			totalRacks++
		}
	}

	// GET all racks
	rackResp, err := c.GetRacks(&cpLib.GetReq{GetAll: true})
	assert.NoError(t, err)
	assert.Equal(t, totalRacks, len(rackResp), "Mismatch total rack count")

	// Store in global Racks map
	for _, r := range rackResp {
		Test_Racks[r.ID] = r
	}

	log.Infof("Successfully created and validated %d racks", len(rackResp))

	// -------------------------
	// 3) Create 2 Hypervisors per Rack
	// -------------------------
	hvCounter := 1

	for _, rack := range Test_Racks {

		for i := 1; i <= 2; i++ {
			hv := cpLib.Hypervisor{
				ID:        fmt.Sprintf("hv-%03d-uuid", hvCounter),
				Name:      fmt.Sprintf("hv-%03d", hvCounter),
				RackID:    rack.ID,
				IPAddress: fmt.Sprintf("10.0.%d.%d", hvCounter/255, hvCounter%255),
				PortRange: "8000-9000",
				SSHPort:   fmt.Sprintf("%d", 6000+hvCounter),
			}

			resp, err := c.PutHypervisor(&hv)
			assert.NoError(t, err)
			assert.True(t, resp.Success)

			Test_Hypervisors[hv.ID] = hv
			hvCounter++
		}
	}

	log.Infof("Created %d Hypervisors", len(Test_Hypervisors))

	// GET ALL hypervisors
	allHvs, err := c.GetHypervisor(&cpLib.GetReq{GetAll: true})
	assert.NoError(t, err)
	assert.Equal(t, len(Test_Hypervisors), len(allHvs))

	// Overwrite global map
	Test_Hypervisors = make(map[string]cpLib.Hypervisor)
	for _, hv := range allHvs {
		Test_Hypervisors[hv.ID] = hv
	}

	// -------------------------
	// 4) Create 2 Devices per Hypervisor
	// -------------------------
	deviceCounter := 1

	for _, hv := range Test_Hypervisors {

		for d := 1; d <= 2; d++ {

			devID := fmt.Sprintf("dev-%03d-uuid", deviceCounter)
			partID := fmt.Sprintf("part-%03d-uuid", deviceCounter)

			device := cpLib.Device{
				ID:            devID,
				SerialNumber:  fmt.Sprintf("SN-%06d", deviceCounter),
				State:         2,
				HypervisorID:  hv.ID,
				FailureDomain: fmt.Sprintf("fd-%02d", d),
				DevicePath:    fmt.Sprintf("/dev/%s/%d", hv.Name, d),
				Name:          fmt.Sprintf("device-%03d", deviceCounter),
				Size:          int64(1_000_000 * deviceCounter),
				Partitions: []cpLib.DevicePartition{
					{
						PartitionID:   partID,
						PartitionPath: fmt.Sprintf("/dev/%s/%d/part1", hv.Name, d),
						NISDUUID:      fmt.Sprintf("nisd-%03d", deviceCounter),
						DevID:         devID,
						Size:          int64(500_000),
					},
				},
			}

			// PUT device
			resp, err := c.PutDevice(&device)
			assert.NoError(t, err)
			assert.True(t, resp.Success)

			Test_Devices[device.ID] = device
			deviceCounter++
		}
	}

	log.Infof("Created %d Hypervisors", len(Test_Hypervisors))

	// GET ALL devices
	allDevices, err := c.GetDevices(&cpLib.GetReq{GetAll: true})
	assert.NoError(t, err)
	assert.Equal(t, len(Test_Devices), len(allDevices))

	// Overwrite global map
	Test_Devices = make(map[string]cpLib.Hypervisor)
	for _, d := range allDevices {
		Test_Devices[d.ID] = d
	}

	log.Infof("Successfully created and validated %d devices", len(Test_Devices))

}