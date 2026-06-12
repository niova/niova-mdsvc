package clictlplanefuncs

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

type VdevReportInfo struct {
	ID        string
	Name      string
	PFSName   string
	Chunks    []cpLib.Chunk
	VdevSize  int64
	NumChunks uint32
}

func TestGenerateDistributionReport(t *testing.T) {
	c := newClient(t)
	adminToken := getAdminToken(t)
	c.SetToken(adminToken)

	// 1. setupHierarchy - already present in vdev_test.go
	setupLargeHierarchy(t, c)

	vdevInfos := make([]*VdevReportInfo, 0)

	// 2. Create multiple PFS instances
	// PFS 1: 3 VDEVs, 16GB, RMReplica 3
	pfs1Name := "pfs-1"
	pfs1Resp, err := c.PutPFS(&cpLib.PFS{Name: pfs1Name})
	require.NoError(t, err)
	pfs1ID := pfs1Resp.ID

	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("vdevp1%d", i)
		vdevReq := &cpLib.VdevReq{
			Vdev: &cpLib.VdevConfig{
				Name:          name,
				Size:          16 * cpLib.CHUNK_SIZE,
				Redundancy:    cpLib.RMReplica,
				TotalDataBlks: 3,
				PFSID:         pfs1ID,
			},
		}
		resp, err := c.CreateVdev(vdevReq)
		require.NoError(t, err)
		vdevInfos = append(vdevInfos, &VdevReportInfo{ID: resp.ID, Name: name, PFSName: pfs1Name, VdevSize: vdevReq.Vdev.Size})
	}

	// PFS 2: 2 VDEVs, 32GB, RMEC 4+3
	pfs2Name := "pfs-2"
	pfs2Resp, err := c.PutPFS(&cpLib.PFS{Name: pfs2Name})
	require.NoError(t, err)
	pfs2ID := pfs2Resp.ID

	for i := 0; i < 2; i++ {
		name := fmt.Sprintf("vdevp2%d", i)
		vdevReq := &cpLib.VdevReq{
			Vdev: &cpLib.VdevConfig{
				Name:            name,
				Size:            32 * cpLib.CHUNK_SIZE,
				Redundancy:      cpLib.RMEC,
				TotalDataBlks:   4,
				TotalParityBlks: 3,
				PFSID:           pfs2ID,
			},
		}
		resp, err := c.CreateVdev(vdevReq)
		require.NoError(t, err)
		vdevInfos = append(vdevInfos, &VdevReportInfo{ID: resp.ID, Name: name, PFSName: pfs2Name, VdevSize: vdevReq.Vdev.Size})
	}

	// 3. Create standalone VDEVs
	for i := 0; i < 2; i++ {
		name := fmt.Sprintf("vdevsa%d", i)
		vdevReq := &cpLib.VdevReq{
			Vdev: &cpLib.VdevConfig{
				Name:          name,
				Size:          8 * cpLib.CHUNK_SIZE,
				Redundancy:    cpLib.RMReplica,
				TotalDataBlks: 2,
			},
		}
		resp, err := c.CreateVdev(vdevReq)
		require.NoError(t, err)
		vdevInfos = append(vdevInfos, &VdevReportInfo{ID: resp.ID, Name: name, PFSName: "standalone", VdevSize: vdevReq.Vdev.Size})
	}

	// Need a map to resolve nisdID to deviceID for section 1
	nisds, err := c.GetNisds(cpLib.GetReq{GetAll: true})
	require.NoError(t, err)
	nisdToDevice := make(map[string]string)
	for _, n := range nisds {
		nisdToDevice[n.ID] = n.FailureDomain[cpLib.DEVICE_IDX]
	}

	// Data collection maps
	deviceUsage := make(map[string]int64)
	nisdUsage := make(map[string]int64)

	deviceCounts := make(map[string]map[string]int) // scope -> deviceID -> count
	nisdCounts := make(map[string]map[string]int)   // scope -> nisdID -> count

	maxChunks := uint32(0)

	// 4. Use ReadChunk() (GetChunks bulk) to retrieve placement information
	for _, vi := range vdevInfos {
		chunks, err := c.GetChunks(&cpLib.GetReq{ID: vi.ID})
		require.NoError(t, err)
		vi.Chunks = chunks
		vi.NumChunks = uint32(len(chunks))
		if vi.NumChunks > maxChunks {
			maxChunks = vi.NumChunks
		}

		if _, ok := deviceCounts[vi.PFSName]; !ok {
			deviceCounts[vi.PFSName] = make(map[string]int)
			nisdCounts[vi.PFSName] = make(map[string]int)
		}

		for _, chunk := range chunks {
			for _, p := range chunk.Placements {
				nisdUsage[p.NisdID] += cpLib.CHUNK_SIZE
				nisdCounts[vi.PFSName][p.NisdID]++

				devID := nisdToDevice[p.NisdID]
				deviceUsage[devID] += cpLib.CHUNK_SIZE
				deviceCounts[vi.PFSName][devID]++
			}
		}
	}

	// Ensure all units are represented in counts (even with 0) for each scope
	for _, vi := range vdevInfos {
		for nID, dID := range nisdToDevice {
			if _, ok := nisdCounts[vi.PFSName][nID]; !ok {
				nisdCounts[vi.PFSName][nID] = 0
			}
			if _, ok := deviceCounts[vi.PFSName][dID]; !ok {
				deviceCounts[vi.PFSName][dID] = 0
			}
		}
	}

	// 5. Generate CSV Report
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Section 1: Device Distribution Matrix
	fmt.Fprintln(&buf, "### Device Distribution Matrix")
	header := []string{"pfs_name", "vdev_name"}
	for i := uint32(0); i < maxChunks; i++ {
		header = append(header, fmt.Sprintf("chunk_%d", i))
	}
	w.Write(header)
	for _, vi := range vdevInfos {
		row := []string{vi.PFSName, vi.Name}
		for i := uint32(0); i < maxChunks; i++ {
			if i < vi.NumChunks {
				// Show all devices for the chunk joined by comma or just first?
				// User example show single value. I'll show first device.
				devs := []string{}
				for _, p := range vi.Chunks[i].Placements {
					devID := nisdToDevice[p.NisdID]
					if len(devID) > 4 {
						devID = devID[len(devID)-4:]
					}
					devs = append(devs, devID)
				}
				row = append(row, strings.Join(devs, "|"))
			} else {
				row = append(row, "n/a")
			}
		}
		w.Write(row)
	}
	w.Flush()
	fmt.Fprintln(&buf, "")

	// Section 2: NISD Distribution Matrix
	fmt.Fprintln(&buf, "### NISD Distribution Matrix")
	w.Write(header)
	for _, vi := range vdevInfos {
		row := []string{vi.PFSName, vi.Name}
		for i := uint32(0); i < maxChunks; i++ {
			if i < vi.NumChunks {
				nisds := []string{}
				for _, p := range vi.Chunks[i].Placements {
					nisdID := p.NisdID
					if len(nisdID) > 4 {
						nisdID = nisdID[len(nisdID)-4:]
					}
					nisds = append(nisds, nisdID)
				}
				row = append(row, strings.Join(nisds, "|"))
			} else {
				row = append(row, "n/a")
			}
		}
		w.Write(row)
	}
	w.Flush()
	fmt.Fprintln(&buf, "")

	// Section 3: Device Utilization Summary
	fmt.Fprintln(&buf, "### Device Utilization Summary")
	w.Write([]string{"device_id", "allocated_bytes", "utilization_pct"})
	deviceIDs := make([]string, 0, len(deviceUsage))
	for id := range deviceUsage {
		deviceIDs = append(deviceIDs, id)
	}
	sort.Strings(deviceIDs)
	for _, id := range deviceIDs {
		pct := (float64(deviceUsage[id]) / float64(1869169767219)) * 100 // Using size from setupLargeHierarchy
		w.Write([]string{id, strconv.FormatInt(deviceUsage[id], 10), fmt.Sprintf("%.2f%%", pct)})
	}
	w.Flush()
	fmt.Fprintln(&buf, "")

	// Section 4: NISD Utilization Summary
	fmt.Fprintln(&buf, "### NISD Utilization Summary")
	w.Write([]string{"nisd_id", "allocated_bytes", "utilization_pct"})
	nisdIDs := make([]string, 0, len(nisdUsage))
	for id := range nisdUsage {
		nisdIDs = append(nisdIDs, id)
	}
	sort.Strings(nisdIDs)
	for _, id := range nisdIDs {
		pct := (float64(nisdUsage[id]) / float64(1869169767219)) * 100
		w.Write([]string{id, strconv.FormatInt(nisdUsage[id], 10), fmt.Sprintf("%.2f%%", pct)})
	}
	w.Flush()
	fmt.Fprintln(&buf, "")

	// Section 5: Distribution Statistics (StdDev)
	fmt.Fprintln(&buf, "### Distribution Statistics")
	w.Write([]string{"scope", "device_stddev", "nisd_stddev"})

	getStdDev := func(counts map[string]int) float64 {
		if len(counts) == 0 {
			return 0
		}
		var values []float64
		var sum float64
		for _, v := range counts {
			values = append(values, float64(v))
			sum += float64(v)
		}
		mean := sum / float64(len(values))
		var sqSum float64
		for _, v := range values {
			sqSum += (v - mean) * (v - mean)
		}
		return math.Sqrt(sqSum / float64(len(values)))
	}

	scopes := []string{"pfs-1", "pfs-2", "standalone"}
	overallDeviceCounts := make(map[string]int)
	overallNisdCounts := make(map[string]int)

	for _, scope := range scopes {
		w.Write([]string{scope, fmt.Sprintf("%.2f", getStdDev(deviceCounts[scope])), fmt.Sprintf("%.2f", getStdDev(nisdCounts[scope]))})
		for k, v := range deviceCounts[scope] {
			overallDeviceCounts[k] += v
		}
		for k, v := range nisdCounts[scope] {
			overallNisdCounts[k] += v
		}
	}
	w.Write([]string{"overall", fmt.Sprintf("%.2f", getStdDev(overallDeviceCounts)), fmt.Sprintf("%.2f", getStdDev(overallNisdCounts))})
	w.Flush()

	// Write to file
	reportPath := "./distribution_report.csv"
	err = os.WriteFile(reportPath, buf.Bytes(), 0644)
	assert.NoError(t, err)
	t.Logf("Report generated at %s", reportPath)
}
