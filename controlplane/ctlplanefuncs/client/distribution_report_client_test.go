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

func redundancyName(r cpLib.RedundancyMode) string {
	switch r {
	case cpLib.RMReplica:
		return "Replica"
	case cpLib.RMEC32K:
		return "EC32K"
	case cpLib.RMEC64K:
		return "EC64K"
	case cpLib.RMEC128K:
		return "EC128K"
	default:
		return fmt.Sprintf("Unknown(%d)", int(r))
	}
}

// TestECDistributionReport creates a large hierarchy then spawns multiple vdevs
// with different EC/replica configurations and prints:
//  1. Per-vdev chunk placement table (nisd, device, block type, sequence)
//  2. Per-nisd pick count broken down by data, parity, and replica blocks
//  3. Per-device pick count broken down by data, parity, and replica blocks
func TestECDistributionReport(t *testing.T) {
	c := newClient(t)
	adminToken := getAdminToken(t)
	c.SetToken(adminToken)

	setupLargeHierarchy(t, c)

	// Build nisd -> device lookup from the registered NISDs.
	allNisds, err := c.GetNisds(cpLib.GetReq{GetAll: true})
	require.NoError(t, err)
	nisdToDevice := make(map[string]string, len(allNisds))
	for _, n := range allNisds {
		nisdToDevice[n.ID] = n.FailureDomain[cpLib.DEVICE_IDX]
	}

	type vdevSpec struct {
		name         string
		redundancy   cpLib.RedundancyMode
		dataBlkCnt   uint8
		parityBlkCnt uint8
		size         int64
	}

	specs := []vdevSpec{
		{"replica1", cpLib.RMReplica, 1, 0, 8 * cpLib.CHUNK_SIZE},
		{"replica3", cpLib.RMReplica, 3, 0, 16 * cpLib.CHUNK_SIZE},
		{"ec32k4p2", cpLib.RMEC32K, 4, 2, 16 * cpLib.CHUNK_SIZE},
		{"ec32k4p3", cpLib.RMEC32K, 4, 3, 24 * cpLib.CHUNK_SIZE},
		{"ec64k6p3", cpLib.RMEC64K, 6, 3, 32 * cpLib.CHUNK_SIZE},
		{"ec128k8p4", cpLib.RMEC128K, 8, 4, 32 * cpLib.CHUNK_SIZE},
	}

	type blockCounts struct {
		data    int
		parity  int
		replica int
	}
	total := func(bc blockCounts) int { return bc.data + bc.parity + bc.replica }

	nisdCounts := make(map[string]*blockCounts)
	deviceCounts := make(map[string]*blockCounts)

	// Pre-populate with zero counts so every known nisd/device appears in summary.
	for nID, dID := range nisdToDevice {
		nisdCounts[nID] = &blockCounts{}
		if _, ok := deviceCounts[dID]; !ok {
			deviceCounts[dID] = &blockCounts{}
		}
	}

	type vdevResult struct {
		spec   vdevSpec
		vdevID string
		chunks []cpLib.Chunk
	}
	results := make([]vdevResult, 0, len(specs))

	for _, sp := range specs {
		resp, err := c.CreateVdev(&cpLib.VdevReq{
			Vdev: &cpLib.VdevConfig{
				Name:         sp.name,
				Size:         sp.size,
				Redundancy:   sp.redundancy,
				DataBlkCnt:   sp.dataBlkCnt,
				ParityBlkCnt: sp.parityBlkCnt,
			},
		})
		require.NoError(t, err, "create vdev %s", sp.name)

		chunks, err := c.GetChunks(&cpLib.GetReq{ID: resp.ID})
		require.NoError(t, err, "get chunks for vdev %s", sp.name)

		results = append(results, vdevResult{spec: sp, vdevID: resp.ID, chunks: chunks})

		for _, chunk := range chunks {
			for _, p := range chunk.Placements {
				bc := nisdCounts[p.NisdID]
				if bc == nil {
					bc = &blockCounts{}
					nisdCounts[p.NisdID] = bc
				}
				devID := nisdToDevice[p.NisdID]
				dbc := deviceCounts[devID]
				if dbc == nil {
					dbc = &blockCounts{}
					deviceCounts[devID] = dbc
				}
				switch p.Type {
				case cpLib.Data:
					bc.data++
					dbc.data++
				case cpLib.Parity:
					bc.parity++
					dbc.parity++
				case cpLib.Replica:
					bc.replica++
					dbc.replica++
				}
			}
		}
	}

	// ── Section 1: per-vdev chunk placement table ──────────────────────────────
	for _, r := range results {
		t.Logf("\n=== Vdev: %s (%s, data=%d, parity=%d) | %d chunks ===",
			r.spec.name, redundancyName(r.spec.redundancy),
			r.spec.dataBlkCnt, r.spec.parityBlkCnt, len(r.chunks))
		t.Logf("  %-5s  %-36s  %-22s  %-6s  %s",
			"Chunk", "NisdID", "Device", "Type", "Seq")
		for _, chunk := range r.chunks {
			// Sort placements by sequence so output is deterministic.
			placed := make([]cpLib.ChunkPlacement, len(chunk.Placements))
			copy(placed, chunk.Placements)
			sort.Slice(placed, func(i, j int) bool {
				if placed[i].Type != placed[j].Type {
					return placed[i].Type < placed[j].Type
				}
				return placed[i].Sequence < placed[j].Sequence
			})
			for _, p := range placed {
				devID := nisdToDevice[p.NisdID]
				t.Logf("  %-5d  %-36s  %-22s  %-6s  %d",
					chunk.Index, p.NisdID, devID, cpLib.ChunkPrefix(p.Type), p.Sequence)
			}
		}
	}

	// ── Section 2: per-nisd pick count summary ─────────────────────────────────
	t.Logf("\n=== NISD Pick Count Summary ===")
	t.Logf("  %-36s  %6s  %6s  %7s  %5s", "NisdID", "Data", "Parity", "Replica", "Total")

	nisdIDs := make([]string, 0, len(nisdCounts))
	for id := range nisdCounts {
		nisdIDs = append(nisdIDs, id)
	}
	sort.Strings(nisdIDs)

	for _, id := range nisdIDs {
		bc := nisdCounts[id]
		t.Logf("  %-36s  %6d  %6d  %7d  %5d",
			id, bc.data, bc.parity, bc.replica, total(*bc))
	}

	// ── Section 3: per-device pick count summary ───────────────────────────────
	t.Logf("\n=== Device Pick Count Summary ===")
	t.Logf("  %-22s  %6s  %6s  %7s  %5s", "Device", "Data", "Parity", "Replica", "Total")

	devIDs := make([]string, 0, len(deviceCounts))
	for id := range deviceCounts {
		devIDs = append(devIDs, id)
	}
	sort.Strings(devIDs)

	for _, id := range devIDs {
		bc := deviceCounts[id]
		t.Logf("  %-22s  %6d  %6d  %7d  %5d",
			id, bc.data, bc.parity, bc.replica, total(*bc))
	}
}

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
				Name:       name,
				Size:       16 * cpLib.CHUNK_SIZE,
				Redundancy: cpLib.RMReplica,
				DataBlkCnt: 3,
				PFSID:      pfs1ID,
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
				Name:         name,
				Size:         32 * cpLib.CHUNK_SIZE,
				Redundancy:   cpLib.RMEC32K,
				DataBlkCnt:   4,
				ParityBlkCnt: 3,
				PFSID:        pfs2ID,
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
				Name:       name,
				Size:       8 * cpLib.CHUNK_SIZE,
				Redundancy: cpLib.RMReplica,
				DataBlkCnt: 2,
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
