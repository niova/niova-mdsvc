package clictlplanefuncs

// placement_pfs_test.go
//
// Focused PFS coverage: several vdevs sharing one PFS, run as a same-sized
// group and a different-sized group, across a topology with multiple PDUs.
//
// This targets the PFS device-selection policy in
// server/srvctlplanefuncs.go's allocateNisdPerChunk (usePFSRoundRobin): a
// PFS-backed vdev rotates devices round-robin by a persisted offset so that
// consecutive vdevs sharing a PFS land on different devices, whereas a
// standalone vdev fills by available space instead. That property is only
// visible once every vdev sharing a PFS is looked at together, so the checks
// here run at two grains:
//
//   - per vdev: chunk-NISD uniqueness (the same hard invariant
//     evalChunkUniqueness enforces elsewhere) expressed as a printed
//     percentage, plus NISD/Device spread against the achievable maximum.
//   - per PFS (all its vdevs combined): Device/NISD fairness, i.e. whether
//     the PFS's total placements land on all eligible devices/NISDs evenly
//     or close to it -- gated by enforcePlacementStats like every other
//     statistical target in this suite.
//
// PREREQUISITE: a running control plane (see the niova-mdsvc-testing-guide
// skill). Like the rest of this package's integration tests, TestMain fatals
// the whole binary if RAFT_ID/GOSSIP_NODES_PATH are unset.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

// ── PFS vdev groups ──────────────────────────────────────────────────────

// pfsVdevGroup is a set of vdev plans meant to share one PFS. code is a
// short, name-safe tag used to build unique vdev/PFS names; label is the
// human-readable form used in report/check text.
type pfsVdevGroup struct {
	code  string
	label string
	plans []vdevPlan
}

// sameSizedPFSGroup: n vdevs sharing one PFS, identical redundancy and chunk
// count. With size held constant, any unevenness left in the aggregate
// Device/NISD counts can only come from the PFS round-robin device rotation
// itself.
func sameSizedPFSGroup(n int) pfsVdevGroup {
	g := pfsVdevGroup{code: "same", label: "same-sized"}
	for i := 0; i < n; i++ {
		g.plans = append(g.plans, vdevPlan{
			name:       fmt.Sprintf("pfssame%02d", i+1),
			redundancy: cpLib.RMReplica,
			dataBlk:    3,
			chunks:     12, // held constant across every vdev
			usePFS:     true,
		})
	}
	return g
}

// diffSizedPFSGroup: n vdevs sharing one PFS, same redundancy but a distinct
// chunk count per vdev, so size -- not redundancy mode -- is the only axis
// that varies from sameSizedPFSGroup. Sizes step 6, 12, 18, ... (6*(i+1)) so
// every vdev in the PFS is a different size.
func diffSizedPFSGroup(n int) pfsVdevGroup {
	g := pfsVdevGroup{code: "diff", label: "different-sized"}
	for i := 0; i < n; i++ {
		g.plans = append(g.plans, vdevPlan{
			name:       fmt.Sprintf("pfsdiff%02d", i+1),
			redundancy: cpLib.RMReplica,
			dataBlk:    3,
			chunks:     int64(6 * (i + 1)), // distinct size per vdev
			usePFS:     true,
		})
	}
	return g
}

// ── Per-vdev uniqueness metric ───────────────────────────────────────────

// vdevUniqueness is a numeric, human-printable summary of one vdev's own
// chunk-NISD placement: UniquePct expresses the same hard invariant
// evalChunkUniqueness checks (no chunk reuses a NISD across its own blocks)
// as a percentage instead of a bool, and the NISD/Device "used/max" pair
// shows how close the vdev's own placements came to the achievable spread --
// min(blocks placed, eligible entities), the same ceiling evalVdevSpread
// checks against, so a small vdev is never penalized for not reaching every
// NISD in the cluster.
type vdevUniqueness struct {
	TotalBlocks int
	Violations  int // blocks beyond the first sharing a (chunk,NISD) pair
	UniquePct   float64
	NisdUsed    int
	NisdMax     int
	DeviceUsed  int
	DeviceMax   int
}

func computeVdevUniqueness(rows []PlacementRow, eligible *HierarchyModel) vdevUniqueness {
	counts := make(map[uint32]map[string]int)
	for _, r := range rows {
		if counts[r.ChunkID] == nil {
			counts[r.ChunkID] = make(map[string]int)
		}
		counts[r.ChunkID][r.NisdID]++
	}
	violations := 0
	for _, nisdCounts := range counts {
		for _, cnt := range nisdCounts {
			if cnt > 1 {
				violations += cnt - 1
			}
		}
	}
	total := len(rows)
	pct := 100.0
	if total > 0 {
		pct = 100 * float64(total-violations) / float64(total)
	}
	return vdevUniqueness{
		TotalBlocks: total,
		Violations:  violations,
		UniquePct:   pct,
		NisdUsed:    uniqueCount(rows, LevelNISD),
		NisdMax:     min2(total, eligible.EntityCount(LevelNISD)),
		DeviceUsed:  uniqueCount(rows, LevelDevice),
		DeviceMax:   min2(total, eligible.EntityCount(LevelDevice)),
	}
}

func (u vdevUniqueness) String() string {
	return fmt.Sprintf("%.1f%% unique (%d/%d conflict-free blocks), NISD spread %d/%d, Device spread %d/%d",
		u.UniquePct, u.TotalBlocks-u.Violations, u.TotalBlocks, u.NisdUsed, u.NisdMax, u.DeviceUsed, u.DeviceMax)
}

// reportPerVdevUniqueness prints the numeric uniqueness value and NISD/Device
// spread for every vdev in rows -- the per-vdev answer to "are chunks landing
// on distinct devices/NISDs, and how close is that to what's achievable".
func reportPerVdevUniqueness(rows []PlacementRow, eligible *HierarchyModel) string {
	var b strings.Builder
	fmt.Fprintln(&b, "### Per-Vdev Uniqueness & Spread")
	fmt.Fprintf(&b, "  %-10s %-14s %-8s %-10s %-16s %-16s\n",
		"PFS", "Vdev", "Blocks", "Unique%", "NISD used/max", "Device used/max")
	byVdev := groupRows(rows, func(r PlacementRow) string { return r.Vdev })
	for _, vdev := range sortedKeys(byVdev) {
		vr := byVdev[vdev]
		u := computeVdevUniqueness(vr, eligible)
		pfsName := ""
		if len(vr) > 0 {
			pfsName = vr[0].PFS
		}
		fmt.Fprintf(&b, "  %-10s %-14s %-8d %-9.1f%% %-16s %-16s\n",
			shortID(pfsName), vdev, u.TotalBlocks, u.UniquePct,
			fmt.Sprintf("%d/%d", u.NisdUsed, u.NisdMax),
			fmt.Sprintf("%d/%d", u.DeviceUsed, u.DeviceMax))
	}
	return b.String()
}

// ── Test body ─────────────────────────────────────────────────────────────

// runPFSGroup creates one PFS containing every plan in g (scoped to pdu),
// records per-vdev structure/uniqueness/diversity into doc (as runScenario
// does for the main matrix), and evaluates PFS-aggregate Device/NISD
// fairness -- the check that actually exercises the round-robin device
// rotation, since that policy's effect only shows up once every vdev sharing
// the PFS is combined.
func runPFSGroup(t *testing.T, c *CliCFuncs, model, eligible *HierarchyModel, pdu, scopeTag string,
	g pfsVdevGroup, thr FairnessThresholds, doc *ScenarioDoc) []PlacementRow {
	t.Helper()

	var pfs pfsRef
	var pfsRows []PlacementRow
	for _, p := range g.plans {
		vdevID, pfsName := createScopedVdev(t, c, p, pdu, scopeTag, &pfs)
		rows := collectVdevRows(t, c, model, vdevID, p.name, pfsName, cpLib.CHUNK_SIZE)

		wantData, wantParity, wantReplica := p.wantBlocks()
		structRes := evalVdevStructure(rows, int(p.chunks), wantData, wantParity, wantReplica)
		structRes.Name = fmt.Sprintf("vdev structure [%s/%s]", g.label, p.name)
		doc.addGlobal(structRes)

		u := computeVdevUniqueness(rows, eligible)
		uniqRes := evalChunkUniqueness(rows)
		uniqRes.Name = fmt.Sprintf("chunk NISD uniqueness [%s/%s]: %s", g.label, p.name, u.String())
		doc.addGlobal(uniqRes)

		for _, lvl := range AllLevels {
			doc.addLevelCheck(lvl, evalChunkDiversity(rows, eligible, lvl))
		}

		pfsRows = append(pfsRows, rows...)
	}

	// PFS-aggregate fairness: the property the round-robin device rotation
	// actually promises ("consecutive vdevs sharing a PFS land on different
	// devices") is only visible once every vdev in the PFS is combined.
	for _, lvl := range []Level{LevelDevice, LevelNISD} {
		res, stats := evalFairness(pfsRows, eligible, lvl, ClassData, thr)
		res.Name = fmt.Sprintf("PFS aggregate fairness @ %s [%s, %s]", lvl.Name(), g.label, thr.Description)
		doc.addLevelCheck(lvl, res)
		check(t, res, enforcePlacementStats)
		t.Logf("PFS[%s] %s spread: Jain=%.3f CV=%.3f Max/Min=%.3f mean=%.1f (near-equal target: Jain>=%.2f CV<=%.2f)",
			g.label, lvl.Name(), stats.Jain, stats.CV, stats.MaxMinRatio, stats.Mean, thr.MinJain, thr.MaxCV)
	}

	return pfsRows
}

// TestPlacementPFSMultiVdev focuses on the PFS device-selection policy:
// consecutive vdevs sharing one PFS should rotate across devices, so the
// interesting fairness question is not "is any one vdev's own placement
// even" (a single small vdev cannot spread past its own block count) but
// "once every vdev sharing a PFS is combined, is the PFS's total load spread
// evenly across devices/NISDs" -- checked for both a PFS whose vdevs are all
// the same size and one whose vdevs are deliberately different sizes. Runs
// across several PDUs (3 below) so the policy is exercised on more than one
// failure domain.
func TestPlacementPFSMultiVdev(t *testing.T) {
	c := newClient(t)
	c.SetToken(getAdminToken(t))

	const sz = 400 * gib   // generous headroom (50 chunks/NISD); ~380 blocks land across 24 NISDs per PDU
	const numPDUs = 3      // exercise the PFS policy across several PDUs, not just one
	const numPFSVdevs = 4  // vdevs created under each shared PFS

	// numPDUs PDUs, each 2 racks x 2 HVs x 3 devices x 2 NISDs = 12 devices / 24 NISDs per PDU.
	b := balancedEqual(uniquePrefix("pfsmv"), numPDUs, 2, 2, 3, 2, sz)
	model := b.Register(t, c)

	groups := []struct {
		g   pfsVdevGroup
		thr FairnessThresholds
	}{
		{sameSizedPFSGroup(numPFSVdevs), EqualSizedThresholds()},
		{diffSizedPFSGroup(numPFSVdevs), GeneralThresholds()},
	}

	var docs []*ScenarioDoc
	for i, pdu := range b.pduIDs {
		pduLabel := fmt.Sprintf("PDU%d", i+1)
		t.Run(pduLabel, func(t *testing.T) {
			eligible := model.Scoped(LevelPDU, pdu)
			require.GreaterOrEqualf(t, eligible.EntityCount(LevelNISD), 3,
				"%s has too few NISDs for a replica-3 vdev", pduLabel)

			doc := newScenarioDoc(
				fmt.Sprintf("PFS multi-vdev — %s", pduLabel),
				"Two PFS instances, each holding multiple vdevs: one where every vdev is the same size, one where every vdev is a different size.",
				"PFS device-selection (round-robin by persisted offset) spreads combined placements evenly across devices/NISDs, for both uniform and mixed vdev sizes within a PFS",
			)
			t.Logf("PURPOSE: %s", doc.Why)
			t.Logf("VALIDATES: %s", doc.Validates)

			var allRows []PlacementRow
			var allPlans []vdevPlan
			for _, tc := range groups {
				scopeTag := fmt.Sprintf("%s%s%s", b.prefix, pduLabel, tc.g.code)
				rows := runPFSGroup(t, c, model, eligible, pdu, scopeTag, tc.g, tc.thr, doc)
				allRows = append(allRows, rows...)
				allPlans = append(allPlans, tc.g.plans...)
			}

			doc.addGlobal(evalCapacityCompliance(allRows, model))
			doc.finalise(allRows, eligible, allPlans)
			writeHTMLReport(t, doc)
			docs = append(docs, doc)

			t.Logf("\n%s", reportPerVdevUniqueness(allRows, eligible))
			t.Logf("\n%s", reportHierarchyDistribution(allRows, eligible))
		})
	}

	writeComparisonHTMLReport(t, docs)
}
