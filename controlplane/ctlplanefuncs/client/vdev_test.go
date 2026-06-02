package clictlplanefuncs

import (
	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

// validateVdevDistribution validates that:
// 1) ec and replication are distributed in same fd passed by the filter
// 2) no duplicate replication and ec distribution (each replica on unique NISD)
// 3) every chunk has the same number of defined replicas/ ec
func validateVdevDistribution(t *testing.T, filter *cpLib.Filter, v *cpLib.Vdev) bool {
	if v == nil {
		t.Error("Vdev is nil")
		return false
	}

	// Group NISDs by chunk index
	chunkPlacements := make(map[int][]cpLib.Nisd)
	for _, nisdChunk := range v.NisdToChkMap {
		for _, chkIdx := range nisdChunk.Chunk {
			chunkPlacements[chkIdx] = append(chunkPlacements[chkIdx], nisdChunk.Nisd)
		}
	}

	expectedTotalBlocks := v.Cfg.TotalBlocksPerChunk()
	if expectedTotalBlocks == 0 {
		t.Errorf("Vdev %s has 0 expected blocks per chunk", v.Cfg.ID)
		return false
	}

	// Verify all chunks are present (from 0 to TotalChunks-1)
	for i := 0; i < int(v.Cfg.TotalChunks); i++ {
		placements, ok := chunkPlacements[i]
		if !ok {
			t.Errorf("Chunk %d is missing from distribution", i)
			return false
		}

		// 3) every chunk has the same number of defined replicas/ ec
		if len(placements) != expectedTotalBlocks {
			t.Errorf("Chunk %d has %d placements, expected %d", i, len(placements), expectedTotalBlocks)
			return false
		}

		// 2) no duplicate replication and ec distribution
		nisdIDs := make(map[string]bool)
		for _, nisd := range placements {
			if nisdIDs[nisd.ID] {
				t.Errorf("Chunk %d has duplicate placement on NISD %s", i, nisd.ID)
				return false
			}
			nisdIDs[nisd.ID] = true
		}

		// 1) ec and replication are distributed in same fd passed by the filter
		if filter != nil && filter.Type != cpLib.FD_ANY {
			fdIdx := cpLib.GetFDIdx(filter.Type)
			if fdIdx != -1 {
				for _, nisd := range placements {
					if len(nisd.FailureDomain) <= fdIdx {
						t.Errorf("Chunk %d placement on NISD %s has invalid failure domain length %d (expected at least %d)",
							i, nisd.ID, len(nisd.FailureDomain), fdIdx+1)
						return false
					}
					if nisd.FailureDomain[fdIdx] != filter.ID {
						t.Errorf("Chunk %d placement on NISD %s is outside filter FD %s (found %s)",
							i, nisd.ID, filter.ID, nisd.FailureDomain[fdIdx])
						return false
					}
				}
			}
		}
	}

	return true
}

// TestVdevLifecycle validates the full lifecycle of a Vdev under different redundancy schemes.
func TestVdevLifecycle(t *testing.T) {
	c := newClient(t)

	// Setup enough NISDs
	for i := 0; i < 10; i++ {
		n := cpLib.Nisd{
			PeerPort: uint16(9000 + i),
			ID:       uuid.NewString(),
			FailureDomain: []string{
				uuid.NewString(), uuid.NewString(), uuid.NewString(),
				fmt.Sprintf("dev-%d", i), fmt.Sprintf("dev-p-%d", i),
			},
			TotalSize:     100 * cpLib.CHUNK_SIZE,
			AvailableSize: 100 * cpLib.CHUNK_SIZE,
		}
		c.PutNisd(&n)
	}

	scenarios := []struct {
		name            string
		redundancy      cpLib.RedundancyMode
		totalReplicas   uint8
		totalDataBlks   uint8
		totalParityBlks uint8
		size            int64
	}{
		{name: "Replica1", redundancy: cpLib.RMReplica, totalReplicas: 1, size: 8 * cpLib.CHUNK_SIZE},
		{name: "Replica3", redundancy: cpLib.RMReplica, totalReplicas: 3, size: 8 * cpLib.CHUNK_SIZE},
		{name: "EC43", redundancy: cpLib.RMEC, totalDataBlks: 4, totalParityBlks: 3, size: 8 * cpLib.CHUNK_SIZE},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			vdevReq := &cpLib.VdevReq{
				Vdev: &cpLib.VdevConfig{
					Name:            sc.name,
					Size:            sc.size,
					Redundancy:      sc.redundancy,
					TotalReplicas:   sc.totalReplicas,
					TotalDataBlks:   sc.totalDataBlks,
					TotalParityBlks: sc.totalParityBlks,
				},
			}

			resp, err := c.CreateVdev(vdevReq)
			require.NoError(t, err)
			require.True(t, resp.Success)

			vdevList, err := c.GetVdevsWithChunkInfo(&cpLib.GetReq{ID: resp.ID})
			require.NoError(t, err)
			require.NotEmpty(t, vdevList)

			success := validateVdevDistribution(t, nil, &vdevList[0])
			assert.True(t, success)
		})
	}
}

// TestVdevCreationWithFilters validates Vdev creation with different failure domain filters.
func TestVdevCreationWithFilters(t *testing.T) {
	c := newClient(t)
	setupLargeHierarchy(t, c)
	adminToken := getAdminToken(t)

	tests := []struct {
		name       string
		filterType cpLib.FD
		filterID   string
	}{
		{"Filter-PDU", cpLib.FD_PDU, "9bc244bc-df29-11f0-a93b-277aec17e401"},
		{"Filter-Rack", cpLib.FD_RACK, "3f082930-df29-11f0-ab7b-4bd430991102"},
		{"Filter-HV", cpLib.FD_HV, "bde1f08a-df63-11f0-88ef-430ddec19902"},
		{"Filter-Device", cpLib.FD_DEVICE, "nvme-fb6358163008"},
		{"No-Filter", cpLib.FD_ANY, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c.SetToken(adminToken)
			vdevReq := &cpLib.VdevReq{
				Vdev: &cpLib.VdevConfig{
					Size:          16 * cpLib.CHUNK_SIZE,
					TotalReplicas: 1,
				},
				Filter: cpLib.Filter{Type: tc.filterType, ID: tc.filterID},
			}

			resp, err := c.CreateVdev(vdevReq)
			require.NoError(t, err)
			assert.True(t, resp.Success)

			vdevs, err := c.GetVdevsWithChunkInfo(&cpLib.GetReq{ID: resp.ID})
			require.NoError(t, err)
			require.NotEmpty(t, vdevs)

			success := validateVdevDistribution(t, &vdevReq.Filter, &vdevs[0])
			assert.True(t, success)
		})
	}
}

// TestVdevCreationWithInvalidFilters validates that Vdev creation fails with invalid filters.
func TestVdevCreationWithInvalidFilters(t *testing.T) {
	c := newClient(t)
	setupLargeHierarchy(t, c)
	adminToken := getAdminToken(t)

	tests := []struct {
		name        string
		filter      cpLib.Filter
		expectedErr string
	}{
		{name: "Invalid-FD-Type", filter: cpLib.Filter{Type: cpLib.FD(99), ID: "some-id"}, expectedErr: "invalid failure domain"},
		{name: "Malformed-UUID", filter: cpLib.Filter{Type: cpLib.FD_PDU, ID: "bad-uuid"}, expectedErr: "not found"},
		{name: "Non-Existent-ID", filter: cpLib.Filter{Type: cpLib.FD_RACK, ID: uuid.NewString()}, expectedErr: "not found"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c.SetToken(adminToken)
			_, err := c.CreateVdev(&cpLib.VdevReq{
				Vdev:   &cpLib.VdevConfig{Size: cpLib.CHUNK_SIZE, TotalReplicas: 2},
				Filter: tc.filter,
			})
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tc.expectedErr))
		})
	}
}

// TestVdevDeletion validates Vdev creation and subsequent deletion.
func TestVdevDeletion(t *testing.T) {
	c := newClient(t)

	nisd := cpLib.Nisd{
		PeerPort: 9400,
		ID:       uuid.NewString(),
		FailureDomain: []string{
			uuid.NewString(), uuid.NewString(), uuid.NewString(), "dev-del", "dev-del-p",
		},
		TotalSize:     15 * cpLib.CHUNK_SIZE,
		AvailableSize: 15 * cpLib.CHUNK_SIZE,
	}
	c.PutNisd(&nisd)

	resp, err := c.CreateVdev(&cpLib.VdevReq{
		Vdev: &cpLib.VdevConfig{Size: cpLib.CHUNK_SIZE, TotalReplicas: 1},
	})
	require.NoError(t, err)
	vdevID := resp.ID

	_, err = c.DeleteVdev(&cpLib.DeleteVdevReq{ID: vdevID})
	if err != nil {
		assert.Contains(t, err.Error(), "empty response buffer")
	}

	deleted, _ := checkIsVdevDeleted(t, c, vdevID)
	assert.True(t, deleted, "Vdev should be deleted")
}

// TestVdevCreationWithPFS validates Vdev creation associated with a PFS.
func TestVdevCreationWithPFS(t *testing.T) {
	c := newClient(t)
	pfsResp, err := c.PutPFS(&cpLib.PFS{Name: "test-pfs"})
	require.NoError(t, err)
	pfsID := pfsResp.ID

	for i := 0; i < 3; i++ {
		resp, err := c.CreateVdev(&cpLib.VdevReq{
			Vdev: &cpLib.VdevConfig{Size: cpLib.CHUNK_SIZE, TotalReplicas: 1, PFSID: pfsID},
		})
		assert.NoError(t, err)
		assert.True(t, resp.Success)
	}
}

// TestVdevOwnershipABAC validates Vdev access control based on ownership.
func TestVdevOwnershipABAC(t *testing.T) {
	if !authEnabled {
		t.Skip("ABAC test only runs when AUTH_ENABLED=true")
	}

	authClient, tearDown := newUserClient(t)
	defer tearDown()
	adminToken := getAdminToken(t)

	adminClient := newClientWithToken(t, adminToken)
	nisd := cpLib.Nisd{
		PeerPort: 9300,
		ID:       uuid.NewString(),
		FailureDomain: []string{uuid.NewString(), uuid.NewString(), uuid.NewString(), "dev-abac", "dev-abac-p"},
		TotalSize:     100 * cpLib.CHUNK_SIZE,
		AvailableSize: 100 * cpLib.CHUNK_SIZE,
	}
	adminClient.PutNisd(&nisd)

	u1Token := createRegularUserAndLogin(t, authClient, adminToken)
	c1 := newClientWithToken(t, u1Token)
	v1Resp, _ := c1.CreateVdev(&cpLib.VdevReq{Vdev: &cpLib.VdevConfig{Size: cpLib.CHUNK_SIZE, TotalReplicas: 1}})

	u2Token := createRegularUserAndLogin(t, authClient, adminToken)
	c2 := newClientWithToken(t, u2Token)

	_, err := c2.GetVdevConfig(&cpLib.GetReq{ID: v1Resp.ID})
	assert.Error(t, err)
}

// TestVdevNisdChunkQuery validates querying NISD and chunk mapping.
func TestVdevNisdChunkQuery(t *testing.T) {
	c := newClient(t)
	nisdID := uuid.NewString()
	nisd := cpLib.Nisd{
		PeerPort: 8101,
		ID:       nisdID,
		FailureDomain: []string{uuid.NewString(), uuid.NewString(), uuid.NewString(), "dev-q", "dev-q-p"},
		TotalSize:     100 * cpLib.CHUNK_SIZE,
		AvailableSize: 100 * cpLib.CHUNK_SIZE,
	}
	c.PutNisd(&nisd)

	resp, _ := c.CreateVdev(&cpLib.VdevReq{Vdev: &cpLib.VdevConfig{Size: cpLib.CHUNK_SIZE, TotalReplicas: 1}})
	
	readV, err := c.GetVdevConfig(&cpLib.GetReq{ID: resp.ID})
	assert.NoError(t, err)
	assert.NotNil(t, readV)

	nc, err := c.GetChunkNisd(&cpLib.GetReq{ID: path.Join(resp.ID, "0")})
	assert.NoError(t, err)
	assert.NotNil(t, nc)
}

// Helpers

func setupLargeHierarchy(t *testing.T, c *CliCFuncs) {
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

	pduCount := 1
	rackPerPdu := 2
	hvPerRack := 1
	devPerHv := 6
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
							TotalSize:     1869169767219,
							AvailableSize: 1869169767219,
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
	}
}

func checkIsVdevDeleted(t *testing.T, client *CliCFuncs, id string) (bool, error) {
	t.Helper()
	for attempt := 0; attempt < 5; attempt++ {
		_, err := client.GetVdevConfig(&cpLib.GetReq{ID: id})
		if err != nil {
			return true, err
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false, nil
}

func calcUsagePercent(total, available int64) int64 {
	if total == 0 { return 0 }
	return ((total - available) * 100) / total
}
