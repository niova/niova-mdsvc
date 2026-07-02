package clictlplanefuncs

// placement_distribution_test.go
//
// The integration test matrix: the smallest set of cluster-backed tests that
// covers every (topology x vdev x capacity) behaviour the placement algorithm
// must exhibit. Each test:
//
//   1. registers a self-contained topology (fresh UUIDs every run),
//   2. scopes every vdev to that topology's PDU via a FailureDomain Filter, so
//      the eligible NISD set is deterministic even though the cluster is shared
//      with other test files,
//   3. collects the Chunk Placement Matrix, and
//   4. runs the reusable validators + emits the full report.
//
// Hard invariants (structure, uniqueness, NISD/Device diversity, capacity) fail
// the test. Statistical-quality targets (fairness, proportionality, higher-level
// diversity, parity balance) are advisory by default and become hard when
// enforcePlacementStats is set — see docs/placement_test_design.md.
//
// PREREQUISITE: a running control plane (see the niova-mdsvc-testing-guide
// skill). These tests are skipped automatically when RAFT_ID is unset.

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

// enforcePlacementStats flips fairness/proportional/parity checks from advisory
// to hard. Leave false until the algorithm under test is known to satisfy the
// thresholds, then set true (or `go test -run ... ` with it patched) to lock in
// a regression guard.
var enforcePlacementStats = false

// vdevPlan is one vdev to create. It spans all six required configurations:
// Replica, Single-Replica, EC, and each of those optionally inside a PFS.
type vdevPlan struct {
	name       string
	redundancy cpLib.RedundancyMode
	dataBlk    uint8
	parityBlk  uint8
	chunks     int64
	usePFS     bool
}

func (p vdevPlan) isEC() bool { return p.redundancy != cpLib.RMReplica }

// wantBlocks returns the expected per-chunk composition (data, parity, replica).
func (p vdevPlan) wantBlocks() (data, parity, replica int) {
	if p.isEC() {
		return int(p.dataBlk), int(p.parityBlk), 0
	}
	return 0, 0, int(p.dataBlk)
}

// blocksPerChunk is the total placements each chunk produces.
func (p vdevPlan) blocksPerChunk() int {
	d, par, rep := p.wantBlocks()
	return d + par + rep
}

// standardVdevPlans covers all six vdev configurations in one pass, so a single
// topology scenario validates every redundancy/PFS combination without
// repeating the (expensive) hierarchy setup.
func standardVdevPlans() []vdevPlan {
	return []vdevPlan{
		{name: "replica2x7", redundancy: cpLib.RMReplica, dataBlk: 2, chunks: 10},
		{name: "replica4k19", redundancy: cpLib.RMReplica, dataBlk: 4, chunks: 16},
		{name: "ec6p3m42", redundancy: cpLib.RMEC32K, dataBlk: 6, parityBlk: 3, chunks: 18},

		{name: "replica5q91", redundancy: cpLib.RMReplica, dataBlk: 5, chunks: 10},
		{name: "replica3z04", redundancy: cpLib.RMReplica, dataBlk: 3, chunks: 12},
		{name: "ec8p2r77", redundancy: cpLib.RMEC32K, dataBlk: 4, parityBlk: 2, chunks: 20},

		// {name: "replica1n58", redundancy: cpLib.RMReplica, dataBlk: 1, chunks: 8},
		{name: "replica6a63", redundancy: cpLib.RMReplica, dataBlk: 6, chunks: 12},
		{name: "ec5p2h12", redundancy: cpLib.RMEC32K, dataBlk: 5, parityBlk: 2, chunks: 14},

		// {name: "replica7w35", redundancy: cpLib.RMReplica, dataBlk: 7, chunks: 28},
		// {name: "replica2c88", redundancy: cpLib.RMReplica, dataBlk: 2, chunks: 12},
		// {name: "ec4p4t51", redundancy: cpLib.RMEC32K, dataBlk: 4, parityBlk: 4, chunks: 16},

		// {name: "replica8j29", redundancy: cpLib.RMReplica, dataBlk: 8, chunks: 32},
		// {name: "replica5p74", redundancy: cpLib.RMReplica, dataBlk: 5, chunks: 15},
		// {name: "ec7p3v06", redundancy: cpLib.RMEC32K, dataBlk: 7, parityBlk: 3, chunks: 20},

		// {name: "replica4e93", redundancy: cpLib.RMReplica, dataBlk: 4, chunks: 16},
		// {name: "replica2l17", redundancy: cpLib.RMReplica, dataBlk: 2, chunks: 8},
		// {name: "ec3p2y68", redundancy: cpLib.RMEC32K, dataBlk: 3, parityBlk: 2, chunks: 10},

		// {name: "replica6f40", redundancy: cpLib.RMReplica, dataBlk: 6, chunks: 18},
		// {name: "replica3u85", redundancy: cpLib.RMReplica, dataBlk: 3, chunks: 15},
		// {name: "ec9p3g24", redundancy: cpLib.RMEC32K, dataBlk: 9, parityBlk: 3, chunks: 24},

		// {name: "replica5d56", redundancy: cpLib.RMReplica, dataBlk: 5, chunks: 25},
		// {name: "replica7s31", redundancy: cpLib.RMReplica, dataBlk: 7, chunks: 21},
		// {name: "ec6p4b79", redundancy: cpLib.RMEC32K, dataBlk: 6, parityBlk: 4, chunks: 20},

		// {name: "replica9r62", redundancy: cpLib.RMReplica, dataBlk: 9, chunks: 36},
		// {name: "replica4x11", redundancy: cpLib.RMReplica, dataBlk: 4, chunks: 20},
		// // {name: "ec8p4n95", redundancy: cpLib.RMEC32K, dataBlk: 8, parityBlk: 4, chunks: 24},
		// {name: "ec6p3pfs", redundancy: cpLib.RMEC64K, dataBlk: 6, parityBlk: 3, chunks: 12, usePFS: true},
		// {name: "replica3pfs", redundancy: cpLib.RMReplica, dataBlk: 3, chunks: 8, usePFS: true},
		// {name: "singlereplicapfs", redundancy: cpLib.RMReplica, dataBlk: 1, chunks: 8, usePFS: true},
	}
}

// uniquePrefix yields a per-scenario id prefix so topologies never collide.
func uniquePrefix(tag string) string { return fmt.Sprintf("%s%s", tag, uuid.NewString()[:8]) }

// pfsRef memoises a lazily-created PFS so the PFS-backed plans in one scenario
// share a single PFS instance.
type pfsRef struct {
	id   string
	name string
}

func (r *pfsRef) ensure(t *testing.T, c *CliCFuncs, scopeTag string) {
	t.Helper()
	if r.id != "" {
		return
	}
	r.name = fmt.Sprintf("pfs%s", scopeTag)
	resp, err := c.PutPFS(&cpLib.PFS{Name: r.name})
	require.NoError(t, err, "PutPFS")
	r.id = resp.ID
}

// createScopedVdev creates one vdev confined to pdu and returns its id + the PFS
// name it belongs to (empty when standalone).
func createScopedVdev(t *testing.T, c *CliCFuncs, p vdevPlan, pdu, scopeTag string, pfs *pfsRef) (string, string) {
	t.Helper()
	cfg := &cpLib.VdevConfig{
		Name:         fmt.Sprintf("%s%s", scopeTag, p.name),
		Size:         p.chunks * cpLib.CHUNK_SIZE,
		Redundancy:   p.redundancy,
		DataBlkCnt:   p.dataBlk,
		ParityBlkCnt: p.parityBlk,
	}
	pfsName := ""
	if p.usePFS {
		pfs.ensure(t, c, scopeTag)
		cfg.PFSID = pfs.id
		pfsName = pfs.name
	}
	resp, err := c.CreateVdev(&cpLib.VdevReq{
		Vdev:   cfg,
		Filter: cpLib.Filter{Type: cpLib.FD_PDU, ID: pdu},
	})
	time.Sleep(1 * time.Second) // allow placement to settle before GetChunks
	require.NoError(t, err, "CreateVdev %s", p.name)
	require.True(t, resp.Success)
	return resp.ID, pfsName
}

// runScenario is the shared body for every topology row in the matrix. It builds
// the topology, creates the standard vdev set scoped to its PDU, validates each
// vdev and the aggregate distribution, and finalises doc for reporting. The
// caller writes the (combined, cross-scenario) HTML report once every row has run.
//
// Placement validation results (structure, uniqueness, diversity, fairness,
// capacity) are recorded into doc without calling t.Errorf — failures appear in
// the HTML report under the relevant hierarchy level. Only API-level failures
// (cluster errors, vdev creation) still use require, as those indicate
// misconfigured infrastructure rather than algorithm behaviour.
func runScenario(t *testing.T, c *CliCFuncs, b *topoBuilder, plans []vdevPlan,
	fairnessThr FairnessThresholds, proportional bool, propTol float64, doc *ScenarioDoc) {

	scopeTag := b.prefix
	model := b.Register(t, c)
	pdu := b.PrimaryPDU()
	eligible := model.Scoped(LevelPDU, pdu)

	// Sanity: the scoped topology must be able to host the widest stripe.
	maxBlocks := 0
	for _, p := range plans {
		if bp := p.blocksPerChunk(); bp > maxBlocks {
			maxBlocks = bp
		}
	}
	require.GreaterOrEqualf(t, eligible.EntityCount(LevelNISD), maxBlocks,
		"topology has too few NISDs (%d) for widest stripe (%d)",
		eligible.EntityCount(LevelNISD), maxBlocks)

	var allRows []PlacementRow
	var pfs pfsRef
	for _, p := range plans {
		vdevID, pfsName := createScopedVdev(t, c, p, pdu, scopeTag, &pfs)
		rows := collectVdevRows(t, c, model, vdevID, p.name, pfsName, cpLib.CHUNK_SIZE)

		// ── Per-vdev validations (recorded into doc, not asserted) ──
		wantData, wantParity, wantReplica := p.wantBlocks()
		structRes := evalVdevStructure(rows, int(p.chunks), wantData, wantParity, wantReplica)
		structRes.Name = fmt.Sprintf("vdev structure [%s]", p.name)
		doc.addGlobal(structRes)

		doc.addGlobal(evalChunkUniqueness(rows))

		for _, lvl := range []Level{LevelNISD, LevelDevice} {
			doc.addLevelCheck(lvl, evalChunkDiversity(rows, eligible, lvl))
		}
		for _, lvl := range []Level{LevelHV, LevelRack, LevelPDU} {
			doc.addLevelCheck(lvl, evalChunkDiversity(rows, eligible, lvl))
		}

		allRows = append(allRows, rows...)
	}

	// ── Aggregate validations ──
	doc.addGlobal(evalCapacityCompliance(allRows, model))

	// ── Fairness and proportionality across the whole scope ──
	// NOTE: PDU/Rack/HV distribution checks are disabled for now — only
	// Device/NISD-level evenness is evaluated. Re-add the higher levels to this
	// list when hierarchy-level distribution becomes relevant.
	for _, class := range []TypeClass{ClassData, ClassParity} {
		for _, lvl := range []Level{ /* LevelPDU, LevelRack, LevelHV, */ LevelDevice, LevelNISD} {
			res, _ := evalFairness(allRows, eligible, lvl, class, fairnessThr)
			doc.addLevelCheck(lvl, res)
		}
	}
	if proportional {
		for _, lvl := range []Level{LevelDevice, LevelNISD} {
			doc.addLevelCheck(lvl, evalProportional(allRows, eligible, lvl, ClassData, propTol))
		}
	}

	doc.finalise(allRows, eligible, plans)
	t.Logf("\n%s", renderFullReport(allRows, eligible))
}

// TestPlacementDistribution is the core matrix: the four (balanced|unbalanced) x
// (equal|different-sized) combinations, each running all six vdev configs. The
// device-addition rows live in TestPlacementRebalance; edge cases in
// TestPlacementEdgeCases. Together these are the smallest set that exercises
// every cell of the required test matrix.
func TestPlacementDistribution(t *testing.T) {
	c := newClient(t)
	c.SetToken(getAdminToken(t))

	// Fixed-total matrix: every scenario below produces the SAME total NISD
	// count (48), the SAME total cluster capacity (9600 GiB), and runs the SAME
	// vdev workload (standardVdevPlans). Only the hierarchy shape and/or device
	// size mix differs between rows, so any difference in the resulting
	// fairness/proportionality stats is attributable solely to hierarchy
	// configuration — never to one scenario simply having a bigger or smaller
	// cluster. See placement_topology_test.go's "Fixed-total presets" section
	// for the exact device counts behind each number below.
	const uniform = 200 * gib // baseline per-NISD size, used by scenarios 1 and 3

	// Scenario 2's 15/6/3 medium/small/large mix must sum to the same total as
	// 24 uniform devices: 15*medium + 6*small + 3*large == 24*medium requires
	// 6*(medium-small) == 3*(large-medium). medium=200, small=100, large=400
	// satisfies it: 6*100 == 3*200.
	const mixMedium = 200 * gib
	const mixSmall = 100 * gib
	const mixLarge = 400 * gib

	// Scenario 4's per-rack sizes (12/6/6 devices in racks A/B/C) must satisfy
	// 12*rackA + 6*rackB + 6*rackC == 24*uniform == 4800*gib. rackA=100,
	// rackB=150, rackC=450 gives 1200+900+2700 == 4800.
	const rackASize = 100 * gib
	const rackBSize = 150 * gib
	const rackCSize = 450 * gib

	scenarios := []struct {
		name         string
		build        func() *topoBuilder
		thr          FairnessThresholds
		proportional bool
		propTol      float64
		// why/validates document the unique reason this row exists.
		why       string
		validates string
	}{
		{
			name:      "balanced hierarchy with equal sized devices",
			build:     func() *topoBuilder { return balancedFixedTotal(uniquePrefix("baleq"), uniform) },
			thr:       EqualSizedThresholds(),
			why:       "control case: symmetric tree, identical capacity",
			validates: "near-perfect even spread is achievable and achieved (rules 6-15, strict)",
		},
		{
			name: "balanced hierarchy with different sized devices",
			build: func() *topoBuilder {
				return balancedFixedTotalMixedSized(uniquePrefix("baldiff"), mixMedium, mixSmall, mixLarge)
			},
			thr:          GeneralThresholds(),
			proportional: true,
			propTol:      0.12,
			why:          "same structure and same total capacity as scenario 1, but a 15/6/3 medium/small/large device mix",
			validates:    "allocation tracks capacity, not just entity count (rule 17)",
		},
		{
			name:      "unbalanced hierarchy with equal sized devices",
			build:     func() *topoBuilder { return unbalancedFixedTotal(uniquePrefix("unbeq"), uniform) },
			thr:       GeneralThresholds(),
			why:       "same total capacity and NISD count as scenario 1, but asymmetric rack/HV fan-out (rack A alone holds half the fleet)",
			validates: "device/NISD evenness holds while rack/HV counts scale with fan-out (rules 7-10, 21)",
		},
		{
			name: "unbalanced hierarchy with different sized devices",
			build: func() *topoBuilder {
				return unbalancedFixedTotalMixedSized(uniquePrefix("unbdiff"), rackASize, rackBSize, rackCSize)
			},
			thr:          GeneralThresholds(),
			proportional: true,
			propTol:      0.15,
			why:          "same rack/HV shape as scenario 3 and same total capacity as every other scenario, but the densely-populated rack holds many small devices while the sparse rack holds fewer large ones",
			validates:    "proportional allocation under combined structural + capacity asymmetry (rules 16-18)",
		},
	}

	docs := make([]*ScenarioDoc, 0, len(scenarios))
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			t.Logf("PURPOSE: %s", sc.why)
			t.Logf("VALIDATES: %s", sc.validates)
			doc := newScenarioDoc(sc.name, sc.why, sc.validates)
			runScenario(t, c, sc.build(), standardVdevPlans(), sc.thr, sc.proportional, sc.propTol, doc)
			docs = append(docs, doc)
		})
	}

	// One comparisonreport across all rows of the matrix, instead of a
	// separate file per scenario — the interesting signal is how stats change
	// across topologies (balanced vs unbalanced, equal vs different-sized).
	writeComparisonHTMLReport(t, docs)
}

// TestPlacementRebalance covers the two device-addition rows of the matrix
// (balanced + unbalanced). It snapshots a vdev's placements, grows the scoped
// PDU with new devices, re-reads the SAME vdev, and evaluates whether the move
// was minimal and targeted at the new capacity (rules 23-25).
//
// The rebalance evaluation is advisory by default (like the other statistical
// targets) so the test is informative whether or not the algorithm rebalances
// on growth: it reports exactly how much moved and where. The hard invariant
// re-checked here is that whatever placements result are still unique per chunk.
// Flip enforcePlacementStats to require minimal movement.
func TestPlacementRebalance(t *testing.T) {
	c := newClient(t)
	c.SetToken(getAdminToken(t))

	const sz = 200 * gib

	cases := []struct {
		name  string
		build func() *topoBuilder
	}{
		{"balanced", func() *topoBuilder { return balancedEqual(uniquePrefix("rbbal"), 1, 4, 1, 4, 1, sz) }},
		{"unbalanced", func() *topoBuilder { return unbalancedOnePDU(uniquePrefix("rbunb"), sz) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.build()
			model := b.Register(t, c)
			pdu := b.PrimaryPDU()

			devicesBefore := model.Scoped(LevelPDU, pdu).Entities(LevelDevice)
			nisdsBefore := model.Scoped(LevelPDU, pdu).EntityCount(LevelNISD)

			// A single replica-3 vdev gives stable, easily-diffed placements.
			plan := vdevPlan{name: "rebal", redundancy: cpLib.RMReplica, dataBlk: 3, chunks: 16}
			var pfs pfsRef
			vdevID, _ := createScopedVdev(t, c, plan, pdu, b.prefix, &pfs)
			before := collectVdevRows(t, c, model, vdevID, plan.name, "", cpLib.CHUNK_SIZE)
			AssertChunkUniqueness(t, before)

			// Grow the PDU: add one rack with several new devices (~25% more NISDs).
			addedNisds := maxInt(1, nisdsBefore/4)
			b.addRack(pdu, 1, addedNisds, 1, uniformSize(sz))
			model2 := b.Register(t, c)
			newDevices := setDiff(model2.Scoped(LevelPDU, pdu).Entities(LevelDevice), devicesBefore)
			require.NotEmpty(t, newDevices, "device addition should introduce new devices")
			newSet := toSet(newDevices)
			isNew := func(r PlacementRow) bool { _, ok := newSet[r.Keys[LevelDevice]]; return ok }

			after := collectVdevRows(t, c, model2, vdevID, plan.name, "", cpLib.CHUNK_SIZE)
			AssertChunkUniqueness(t, after) // hard: still valid after growth

			nisdsAfter := model2.Scoped(LevelPDU, pdu).EntityCount(LevelNISD)
			optimalPct := float64(nisdsAfter-nisdsBefore) / float64(nisdsAfter)

			res, ms := evalRebalance(before, after, isNew, optimalPct, 0.6)
			check(t, res, enforcePlacementStats)
			t.Logf("optimal movement ~%.1f%%\n%s", optimalPct*100, reportRebalance(ms))
		})
	}
}

// TestPlacementEdgeCases bundles the bias-exposing degenerate topologies into one
// table. Each case is intentionally tiny: it asserts only the invariants that
// must hold even at the boundary, which is exactly where placement bugs hide.
func TestPlacementEdgeCases(t *testing.T) {
	c := newClient(t)
	c.SetToken(getAdminToken(t))
	const sz = 64 * gib

	cases := []struct {
		name  string
		build func() *topoBuilder
		plan  vdevPlan
		// expectCreateErr: the request is infeasible and must be rejected.
		expectCreateErr bool
		why             string
	}{
		{
			name: "single device single nisd",
			build: func() *topoBuilder {
				b := newTopo(uniquePrefix("e1nisd"))
				p := b.startPDU()
				b.addRack(p, 1, 1, 1, uniformSize(sz))
				return b
			},
			plan: vdevPlan{name: "sr", redundancy: cpLib.RMReplica, dataBlk: 1, chunks: 4},
			why:  "single replica on a single NISD: the no-diversity floor must still place every chunk",
		},
		{
			name: "single rack many devices",
			build: func() *topoBuilder {
				b := newTopo(uniquePrefix("e1rack"))
				p := b.startPDU()
				b.addRack(p, 2, 4, 1, uniformSize(sz))
				return b
			},
			plan: vdevPlan{name: "ec4p2", redundancy: cpLib.RMEC32K, dataBlk: 4, parityBlk: 2, chunks: 4},
			why:  "all candidates share one rack: rack/HV diversity is impossible, device diversity must still hold",
		},
		{
			name: "more chunks than nisd",
			build: func() *topoBuilder {
				b := newTopo(uniquePrefix("emorechunks"))
				p := b.startPDU()
				b.addRack(p, 1, 3, 1, uniformSize(sz))
				return b
			},
			plan: vdevPlan{name: "sr", redundancy: cpLib.RMReplica, dataBlk: 1, chunks: 20},
			why:  "20 single-replica chunks over 3 NISDs: each NISD must be reused evenly (rule 10/20)",
		},
		{
			name: "more nisd than chunks",
			build: func() *topoBuilder {
				b := newTopo(uniquePrefix("emorenisds"))
				p := b.startPDU()
				b.addRack(p, 2, 6, 2, uniformSize(sz))
				return b
			},
			plan: vdevPlan{name: "sr", redundancy: cpLib.RMReplica, dataBlk: 1, chunks: 3},
			why:  "few chunks over many NISDs: must not pile onto a subset (bias detection, rule 19)",
		},
		{
			name: "stripe wider than failure domains",
			build: func() *topoBuilder {
				b := newTopo(uniquePrefix("ewidestripe"))
				p := b.startPDU()
				b.addRack(p, 1, 3, 1, uniformSize(sz))
				return b
			},
			plan:            vdevPlan{name: "ec4p2", redundancy: cpLib.RMEC32K, dataBlk: 4, parityBlk: 2, chunks: 2},
			expectCreateErr: true,
			why:             "6-block stripe needs 6 NISDs but only 3 exist: creation must fail, not silently double up",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("PURPOSE: %s", tc.why)
			doc := newScenarioDoc(tc.name, tc.why, "")
			b := tc.build()
			model := b.Register(t, c)
			pdu := b.PrimaryPDU()
			eligible := model.Scoped(LevelPDU, pdu)

			cfg := &cpLib.VdevConfig{
				Name:         fmt.Sprintf("%s%s", b.prefix, tc.plan.name),
				Size:         tc.plan.chunks * cpLib.CHUNK_SIZE,
				Redundancy:   tc.plan.redundancy,
				DataBlkCnt:   tc.plan.dataBlk,
				ParityBlkCnt: tc.plan.parityBlk,
			}
			resp, err := c.CreateVdev(&cpLib.VdevReq{Vdev: cfg, Filter: cpLib.Filter{Type: cpLib.FD_PDU, ID: pdu}})

			if tc.expectCreateErr {
				require.Error(t, err, "infeasible stripe must be rejected")
				return
			}
			require.NoError(t, err)
			require.True(t, resp.Success)

			rows := collectVdevRows(t, c, model, resp.ID, tc.plan.name, "", cpLib.CHUNK_SIZE)

			// Validations recorded into the doc, not asserted.
			wantData, wantParity, wantReplica := tc.plan.wantBlocks()
			doc.addGlobal(evalVdevStructure(rows, int(tc.plan.chunks), wantData, wantParity, wantReplica))
			doc.addGlobal(evalChunkUniqueness(rows))
			doc.addGlobal(evalCapacityCompliance(rows, model))
			doc.addLevelCheck(LevelDevice, evalChunkDiversity(rows, eligible, LevelDevice))

			// Spread check: did placements cover the available NISDs rather than
			// pile onto a subset? (bias detection, rules 19-20)
			doc.addLevelCheck(LevelNISD, evalVdevSpread(rows, eligible, LevelNISD))

			doc.finalise(rows, eligible, []vdevPlan{tc.plan})
			writeHTMLReport(t, doc)
			t.Logf("\n%s", reportHierarchyDistribution(rows, eligible))
		})
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func toSet(xs []string) map[string]struct{} {
	out := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		out[x] = struct{}{}
	}
	return out
}
