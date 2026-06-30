package clictlplanefuncs

// placement_model_test.go
//
// The in-memory data model that every placement check is built on. Two ideas:
//
//  1. HierarchyModel — the storage topology (PDU > Rack > Hypervisor > Device >
//     NISD) plus per-NISD capacity, reconstructed from the FailureDomain array
//     that each registered NISD carries. It can be built from a live cluster
//     (modelFromCluster) or from a declarative spec (see placement_topology_test.go),
//     so the *same* assertions run in pure unit tests and in integration tests.
//
//  2. PlacementRow — one row per placement: the "Chunk Placement Matrix" that the
//     task spec calls the source of truth. Every metric in the suite is a pure
//     reduction over []PlacementRow + HierarchyModel, which is what keeps the
//     validation logic algorithm-agnostic and reusable.

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

// Level identifies a failure-domain level. The order matches the NISD
// FailureDomain layout (PDU..Device) with NISD appended as the leaf.
type Level int

const (
	LevelPDU Level = iota
	LevelRack
	LevelHV
	LevelDevice
	LevelNISD
	levelCount
)

// AllLevels is the canonical iteration order used by reports and assertions.
var AllLevels = []Level{LevelPDU, LevelRack, LevelHV, LevelDevice, LevelNISD}

func (l Level) Name() string {
	switch l {
	case LevelPDU:
		return "PDU"
	case LevelRack:
		return "Rack"
	case LevelHV:
		return "Hypervisor"
	case LevelDevice:
		return "Device"
	case LevelNISD:
		return "NISD"
	default:
		return "?"
	}
}

// TypeClass selects which placements a metric counts. ClassData deliberately
// folds Replica together with Data: for a replicated vdev the replicas ARE the
// payload, so "data fairness" must include them, while ClassParity stays EC-only.
type TypeClass int

const (
	ClassAll TypeClass = iota
	ClassData
	ClassParity
)

func (tc TypeClass) Name() string {
	switch tc {
	case ClassData:
		return "data"
	case ClassParity:
		return "parity"
	default:
		return "all"
	}
}

func (tc TypeClass) match(t cpLib.ChunkType) bool {
	switch tc {
	case ClassData:
		return t == cpLib.Data || t == cpLib.Replica
	case ClassParity:
		return t == cpLib.Parity
	default:
		return true
	}
}

// NisdInfo is one NISD resolved to its full hierarchy path plus capacity.
// Keys holds the entity id at every Level (Keys[LevelNISD] == ID). Partition is
// the raw FailureDomain[PARTITION_IDX] value, kept so a synthetic topology can
// be registered to a live cluster and read back identically.
type NisdInfo struct {
	ID            string
	Partition     string
	Keys          [levelCount]string
	TotalSize     int64
	AvailableSize int64
}

// HierarchyModel is an indexed, read-only view of the topology.
type HierarchyModel struct {
	nisds map[string]NisdInfo
}

func newHierarchyModel() *HierarchyModel {
	return &HierarchyModel{nisds: make(map[string]NisdInfo)}
}

func (m *HierarchyModel) add(n NisdInfo) { m.nisds[n.ID] = n }

func (m *HierarchyModel) Nisd(id string) (NisdInfo, bool) {
	n, ok := m.nisds[id]
	return n, ok
}

func (m *HierarchyModel) NisdCount() int { return len(m.nisds) }

// keyAt returns the entity id of nisdID at lvl, or "" if the NISD is unknown.
func (m *HierarchyModel) keyAt(nisdID string, lvl Level) string {
	if n, ok := m.nisds[nisdID]; ok {
		return n.Keys[lvl]
	}
	return ""
}

// Entities returns the sorted unique entity ids present at lvl.
func (m *HierarchyModel) Entities(lvl Level) []string {
	seen := make(map[string]struct{})
	for _, n := range m.nisds {
		seen[n.Keys[lvl]] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// EntityCount is the number of distinct entities at lvl (the theoretical maximum
// diversity a single vdev could reach at that level).
func (m *HierarchyModel) EntityCount(lvl Level) int { return len(m.Entities(lvl)) }

// CapacityByEntity sums NISD AvailableSize under each entity at lvl. This drives
// proportional-allocation checks: an entity's fair share is its capacity share.
func (m *HierarchyModel) CapacityByEntity(lvl Level) map[string]int64 {
	out := make(map[string]int64)
	for _, n := range m.nisds {
		out[n.Keys[lvl]] += n.AvailableSize
	}
	return out
}

// NisdsSorted returns all NISDs ordered by ID (deterministic iteration).
func (m *HierarchyModel) NisdsSorted() []NisdInfo {
	out := make([]NisdInfo, 0, len(m.nisds))
	for _, n := range m.nisds {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Group buckets NISDs by their entity id at lvl (e.g. all NISDs per device).
func (m *HierarchyModel) Group(lvl Level) map[string][]NisdInfo {
	out := make(map[string][]NisdInfo)
	for _, n := range m.NisdsSorted() {
		k := n.Keys[lvl]
		out[k] = append(out[k], n)
	}
	return out
}

// Scoped returns a sub-model containing only NISDs under entity id at lvl. Used
// when a vdev was created with a failure-domain Filter, so fairness/diversity is
// judged against the eligible set rather than the whole cluster.
func (m *HierarchyModel) Scoped(lvl Level, id string) *HierarchyModel {
	sub := newHierarchyModel()
	for _, n := range m.nisds {
		if n.Keys[lvl] == id {
			sub.add(n)
		}
	}
	return sub
}

// modelFromCluster rebuilds the HierarchyModel from the live NISD registry.
func modelFromCluster(t *testing.T, c *CliCFuncs) *HierarchyModel {
	t.Helper()
	nisds, err := c.GetNisds(cpLib.GetReq{GetAll: true})
	require.NoError(t, err, "GetNisds for hierarchy model")
	require.NotEmpty(t, nisds, "cluster has no NISDs registered")

	m := newHierarchyModel()
	for _, n := range nisds {
		require.GreaterOrEqual(t, len(n.FailureDomain), cpLib.FD_MAX,
			"NISD %s has short FailureDomain", n.ID)
		var keys [levelCount]string
		keys[LevelPDU] = n.FailureDomain[cpLib.PDU_IDX]
		keys[LevelRack] = n.FailureDomain[cpLib.RACK_IDX]
		keys[LevelHV] = n.FailureDomain[cpLib.HV_IDX]
		keys[LevelDevice] = n.FailureDomain[cpLib.DEVICE_IDX]
		keys[LevelNISD] = n.ID
		m.add(NisdInfo{
			ID:            n.ID,
			Partition:     n.FailureDomain[cpLib.PARTITION_IDX],
			Keys:          keys,
			TotalSize:     n.TotalSize,
			AvailableSize: n.AvailableSize,
		})
	}
	return m
}

// PlacementRow is one placement in the Chunk Placement Matrix. Keys is the
// resolved hierarchy path so reductions never need a second lookup.
type PlacementRow struct {
	PFS       string
	Vdev      string
	VdevID    string
	ChunkID   uint32
	Type      cpLib.ChunkType
	ChunkSize int64
	Seq       int // replica index (Replica) or EC stripe index (Data/Parity)
	NisdID    string
	Keys      [levelCount]string
}

// collectVdevRows reads every chunk of one vdev and flattens its placements into
// PlacementRows, resolving each NISD through the model.
func collectVdevRows(t *testing.T, c *CliCFuncs, m *HierarchyModel,
	vdevID, vdevName, pfsName string, chunkSize int64) []PlacementRow {
	t.Helper()
	chunks, err := c.GetChunks(&cpLib.GetReq{ID: vdevID})
	require.NoError(t, err, "GetChunks for vdev %s", vdevName)

	rows := make([]PlacementRow, 0, len(chunks)*3)
	for _, ch := range chunks {
		for _, p := range ch.Placements {
			n, ok := m.Nisd(p.NisdID)
			require.True(t, ok, "placement on unknown NISD %s (vdev %s chunk %d)",
				p.NisdID, vdevName, ch.Index)
			rows = append(rows, PlacementRow{
				PFS:       pfsName,
				Vdev:      vdevName,
				VdevID:    vdevID,
				ChunkID:   ch.Index,
				Type:      p.Type,
				ChunkSize: chunkSize,
				Seq:       int(p.Sequence),
				NisdID:    p.NisdID,
				Keys:      n.Keys,
			})
		}
	}
	return rows
}

// rowsForVdev filters a row set down to a single vdev (used by per-vdev checks).
func rowsForVdev(rows []PlacementRow, vdevID string) []PlacementRow {
	out := make([]PlacementRow, 0, len(rows))
	for _, r := range rows {
		if r.VdevID == vdevID {
			out = append(out, r)
		}
	}
	return out
}

// CountsByLevel reduces rows to "placements per entity" at one level for one
// type class. eligible (when non-nil) seeds every known entity with 0 so an
// entity the algorithm never picked still counts against fairness — without it,
// an ignored device would be invisible to the stats.
func CountsByLevel(rows []PlacementRow, lvl Level, class TypeClass, eligible *HierarchyModel) map[string]int {
	counts := make(map[string]int)
	if eligible != nil {
		for _, e := range eligible.Entities(lvl) {
			counts[e] = 0
		}
	}
	for _, r := range rows {
		if !class.match(r.Type) {
			continue
		}
		counts[r.Keys[lvl]]++
	}
	return counts
}

// countValues extracts the value multiset from a counts map for ComputeStats.
func countValues(counts map[string]int) []float64 {
	out := make([]float64, 0, len(counts))
	for _, v := range counts {
		out = append(out, float64(v))
	}
	return out
}

// statsByLevel is the common "counts -> stats" pipeline used everywhere.
func statsByLevel(rows []PlacementRow, lvl Level, class TypeClass, eligible *HierarchyModel) DistStats {
	return ComputeStats(countValues(CountsByLevel(rows, lvl, class, eligible)))
}
