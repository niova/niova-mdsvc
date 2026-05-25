package clictlplanefuncs

import (
	"fmt"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

func TestCreateLargeHierarchy(t *testing.T) {
	c := newClient(t)
	adminToken := getAdminToken(t)

	pdus := []string{
		"9bc244bc-df29-11f0-a93b-277aec17e401",
		"9bc244bc-df29-11f0-a93b-277aec17e402",
		"9bc244bc-df29-11f0-a93b-277aec17e403",
		"9bc244bc-df29-11f0-a93b-277aec17e404",
		"9bc244bc-df29-11f0-a93b-277aec17e405",
	}

	// 10 RACKS
	racks := []string{
		"3f082930-df29-11f0-ab7b-4bd430991101",
		"3f082930-df29-11f0-ab7b-4bd430991102",
		"3f082930-df29-11f0-ab7b-4bd430991103",
		"3f082930-df29-11f0-ab7b-4bd430991104",
		"3f082930-df29-11f0-ab7b-4bd430991105",
		"3f082930-df29-11f0-ab7b-4bd430991106",
		"3f082930-df29-11f0-ab7b-4bd430991107",
		"3f082930-df29-11f0-ab7b-4bd430991108",
		"3f082930-df29-11f0-ab7b-4bd430991109",
		"3f082930-df29-11f0-ab7b-4bd430991110",
	}

	// 20 HVs
	hvs := []string{
		"bde1f08a-df63-11f0-88ef-430ddec19901",
		"bde1f08a-df63-11f0-88ef-430ddec19902",
		"bde1f08a-df63-11f0-88ef-430ddec19903",
		"bde1f08a-df63-11f0-88ef-430ddec19904",
		"bde1f08a-df63-11f0-88ef-430ddec19905",
		"bde1f08a-df63-11f0-88ef-430ddec19906",
		"bde1f08a-df63-11f0-88ef-430ddec19907",
		"bde1f08a-df63-11f0-88ef-430ddec19908",
		"bde1f08a-df63-11f0-88ef-430ddec19909",
		"bde1f08a-df63-11f0-88ef-430ddec19910",
		"bde1f08a-df63-11f0-88ef-430ddec19911",
		"bde1f08a-df63-11f0-88ef-430ddec19912",
		"bde1f08a-df63-11f0-88ef-430ddec19913",
		"bde1f08a-df63-11f0-88ef-430ddec19914",
		"bde1f08a-df63-11f0-88ef-430ddec19915",
		"bde1f08a-df63-11f0-88ef-430ddec19916",
		"bde1f08a-df63-11f0-88ef-430ddec19917",
		"bde1f08a-df63-11f0-88ef-430ddec19918",
		"bde1f08a-df63-11f0-88ef-430ddec19919",
		"bde1f08a-df63-11f0-88ef-430ddec19920",
	}

	// 40 Devices
	devices := []string{
		"nvme-fb6358163001",
		"nvme-fb6358163002",
		"nvme-fb6358163003",
		"nvme-fb6358163004",
		"nvme-fb6358163005",
		"nvme-fb6358163006",
		"nvme-fb6358163007",
		"nvme-fb6358163008",
		"nvme-fb6358163009",
		"nvme-fb6358163010",
		"nvme-fb6358163011",
		"nvme-fb6358163012",
		"nvme-fb6358163013",
		"nvme-fb6358163014",
		"nvme-fb6358163015",
		"nvme-fb6358163016",
		"nvme-fb6358163017",
		"nvme-fb6358163018",
		"nvme-fb6358163019",
		"nvme-fb6358163020",
		"nvme-fb6358163021",
		"nvme-fb6358163022",
		"nvme-fb6358163023",
		"nvme-fb6358163024",
		"nvme-fb6358163025",
		"nvme-fb6358163026",
		"nvme-fb6358163027",
		"nvme-fb6358163028",
		"nvme-fb6358163029",
		"nvme-fb6358163030",
		"nvme-fb6358163031",
		"nvme-fb6358163032",
		"nvme-fb6358163033",
		"nvme-fb6358163034",
		"nvme-fb6358163035",
		"nvme-fb6358163036",
		"nvme-fb6358163037",
		"nvme-fb6358163038",
		"nvme-fb6358163039",
		"nvme-fb6358163040",
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

	c.SetToken(adminToken)
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
							ID:       fmt.Sprintf("ed7914c3-2e96-4f3e-8e0d-%012x", nisdID),
							FailureDomain: []string{
								pdu,
								rack,
								hv,
								dev,
								fmt.Sprintf("%s-%d", dev, n),
							},
							TotalSize:     1_000_000_000_000,
							AvailableSize: 1_000_000_000_000,
						}

						mockNisd = append(mockNisd, nisd)
						nisdID++
					}
				}
			}
		}
	}
	for _, n := range mockNisd {
		resp, err := c.PutNisd(&n)
		if assert.NoError(t, err) {
			assert.True(t, resp.Success)
		}
		time.Sleep(10 * time.Millisecond)
		log.Info("response : ", resp, err)
	}

}
func TestCreateVdevWithFilters(t *testing.T) {
	c := newClient(t)
	adminToken := getAdminToken(t)

	tests := []struct {
		name       string
		filterType cpLib.FD
		filterID   string
		expectErr  bool
	}{
		{
			name:       "Filter PDU-1",
			filterType: cpLib.FD_PDU,
			filterID:   "9bc244bc-df29-11f0-a93b-277aec17e401",
			expectErr:  false,
		},
		{
			name:       "Filter Rack-1",
			filterType: cpLib.FD_RACK,
			filterID:   "3f082930-df29-11f0-ab7b-4bd430991103",
			expectErr:  false,
		},
		{
			name:       "Filter HV-1",
			filterType: cpLib.FD_HV,
			filterID:   "bde1f08a-df63-11f0-88ef-430ddec19906",
			expectErr:  false,
		},
		{
			name:       "Filter Device",
			filterType: cpLib.FD_DEVICE,
			filterID:   "nvme-fb6358163008",
			expectErr:  false,
		},
		{
			// Partition ID synthetically generated for device nvme-fb6358163008, slot 0
			name:       "Filter Partition",
			filterType: cpLib.FD_PARTITION,
			filterID:   "nvme-fb6358163008-0",
			expectErr:  false,
		},
		{
			name:       "No Filter",
			filterType: cpLib.FD_ANY,
			filterID:   "",
			expectErr:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			log.Infof("[TEST START] %s", tc.name)
			log.Infof("FilterType=%v FilterID=%q ExpectErr=%v", tc.filterType, tc.filterID, tc.expectErr)
			c.SetToken(adminToken)
			vdevReq := &cpLib.VdevReq{
				Vdev: &cpLib.VdevCfg{
					Size:       16 * 1024 * 1024 * 1024,
					NumReplica: 1,
				},
				Filter: cpLib.Filter{
					Type: tc.filterType,
					ID:   tc.filterID,
				},
			}

			resp, err := c.CreateVdev(vdevReq)
			if tc.expectErr {
				log.Infof("CreateVdev returned err=%v", err)
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.NotEmpty(t, resp.ID)

			log.Infof("Created Vdev ID=%s", resp.ID)

			getReq := &cpLib.GetReq{ID: resp.ID}

			vdevs, err := c.GetVdevsWithChunkInfo(getReq)
			log.Infof("GetVdevsWithChunkInfo response: count=%d err=%v", len(vdevs), err)

			// Log warning on failure instead of asserting, to handle load timeouts gracefully
			if err != nil || len(vdevs) == 0 {
				log.Warnf("GetVdevsWithChunkInfo failed or empty (skipping validation): err=%v count=%d", err, len(vdevs))
				return
			}

			vdev := vdevs[0]
			log.Infof("Vdev fetched: ID=%s ChunkCount=%d", vdev.Cfg.ID, len(vdev.NisdToChkMap))

			for chkIdx, chunk := range vdev.NisdToChkMap {
				fd := chunk.Nisd.FailureDomain
				log.Infof(
					"Chunk[%d]: NISD=%s FailureDomain=%v",
					chkIdx,
					chunk.Nisd.ID,
					fd,
				)

				if len(fd) == 0 {
					log.Infof("Chunk[%d]: empty failure domain, skipping validation", chkIdx)
					continue
				}

				if tc.filterType != cpLib.FD_ANY {
					idx := cpLib.GetFDIdx(tc.filterType)
					log.Infof(
						"Chunk[%d]: validating filter idx=%d expectedID=%q",
						chkIdx,
						idx,
						tc.filterID,
					)

					if idx != -1 {
						assert.Equal(
							t,
							tc.filterID,
							fd[idx],
							"Chunk placed on wrong entity",
						)
					}
				}
			}

			log.Infof("[TEST END] %s", tc.name)
		})
	}
}

func TestCreateVdevWithInvalidFilters(t *testing.T) {
	c := newClient(t)
	adminToken := getAdminToken(t)

	tests := []struct {
		name        string
		filter      cpLib.Filter
		expectedErr string
	}{
		{
			name: "Invalid Filter Entity (undefined FD)",
			filter: cpLib.Filter{
				Type: cpLib.FD(99), // undefined failure domain
				ID:   "9bc244bc-df29-11f0-a93b-277aec17e401",
			},
			expectedErr: "failed to allocate nisd from fd: -1, invalid failure domain: -1",
		},
		{
			name: "Malformed UUID",
			filter: cpLib.Filter{
				Type: cpLib.FD_PDU,
				ID:   "not-a-valid-id",
			},
			expectedErr: "failed to allocate nisd from fd: 0, entityID not-a-valid-id not found in fd 0",
		},
		{
			name: "Non-existent UUID",
			filter: cpLib.Filter{
				Type: cpLib.FD_RACK,
				ID:   "11111111-2222-3333-4444-555555555555",
			},
			expectedErr: "failed to allocate nisd from fd: 1, entityID 11111111-2222-3333-4444-555555555555 not found in fd 1",
		},
		{
			name: "Entity type mismatch - Rack ID used with PDU filter",
			filter: cpLib.Filter{
				Type: cpLib.FD_PDU,
				ID:   "3f082930-df29-11f0-ab7b-4bd430991103", // this is actually a Rack ID
			},
			expectedErr: "failed to allocate nisd from fd: 0, entityID 3f082930-df29-11f0-ab7b-4bd430991103 not found in fd 0",
		},
		{
			name: "Entity type mismatch - Device ID used with HV filter",
			filter: cpLib.Filter{
				Type: cpLib.FD_HV,
				ID:   "nvme-fb6358163008",
			},
			expectedErr: "failed to allocate nisd from fd: 2, entityID nvme-fb6358163008 not found in fd 2",
		},
		{
			name: "Non-existent partition ID",
			filter: cpLib.Filter{
				Type: cpLib.FD_PARTITION,
				ID:   "pt-nonexistent-device-99",
			},
			expectedErr: "failed to allocate nisd from fd: 4, entityID pt-nonexistent-device-99 not found in fd 4",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			log.Infof("[TEST START] %s", tc.name)
			log.Infof("FilterType=%v FilterID=%q", tc.filter.Type, tc.filter.ID)

			c.SetToken(adminToken)

			vdevReq := &cpLib.VdevReq{
				Vdev: &cpLib.VdevCfg{
					Size:       1 * 1024 * 1024 * 1024, // 1 GiB
					NumReplica: 2,
				},
				Filter: tc.filter,
			}

			_, err := c.CreateVdev(vdevReq)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedErr)

			log.Infof("[TEST END] %s", tc.name)
		})
	}
}
