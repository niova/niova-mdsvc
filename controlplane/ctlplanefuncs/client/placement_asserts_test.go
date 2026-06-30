package clictlplanefuncs

// placement_asserts_test.go
//
// The reusable validation library. Every helper is a pure reduction over
// []PlacementRow (+ HierarchyModel) and comes in two layers:
//
//   - eval*  -> returns a CheckResult (ok + human reasons). Pure, side-effect
//              free, and unit-tested in TestPlacementValidationHelpers below.
//   - Assert* -> thin wrapper that fails the *testing.T. Used by integration
//              tests so a violation is reported with a precise reason.
//
// Because the helpers take a level and a TypeClass as parameters, the same dozen
// functions cover all 25 validation rules in the spec across every hierarchy
// level and every vdev configuration — that is the "non-redundant, reusable"
// requirement. The mapping rule->helper is documented in docs/placement_test_design.md.

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

// CheckResult is the uniform outcome of every evaluator.
type CheckResult struct {
	Name    string
	OK      bool
	Reasons []string
}

func newCheck(name string) CheckResult { return CheckResult{Name: name, OK: true} }

func (r *CheckResult) fail(reason string) {
	r.OK = false
	r.Reasons = append(r.Reasons, reason)
}

func (r *CheckResult) finalize() CheckResult {
	sort.Strings(r.Reasons) // deterministic reporting
	return *r
}

// assertOn fails t when the check did not pass, listing every reason.
func (r CheckResult) assertOn(t *testing.T) bool {
	t.Helper()
	if !r.OK {
		t.Errorf("[%s] FAILED:\n  - %s", r.Name, strings.Join(r.Reasons, "\n  - "))
	}
	return r.OK
}

// logCheck reports a check as advisory: it never fails the test, but surfaces
// PASS/violation in the log. Used for "quality target" rules (strict-bar
// fairness, proportional allocation, best-effort higher-level diversity) whose
// satisfaction depends on how sophisticated the algorithm under test is, as
// opposed to the correctness floors enforced with assertOn.
func logCheck(t *testing.T, r CheckResult) bool {
	t.Helper()
	if r.OK {
		t.Logf("[ADVISORY PASS] %s", r.Name)
	} else {
		t.Logf("[ADVISORY CHECK] %s:\n  - %s", r.Name, strings.Join(r.Reasons, "\n  - "))
	}
	return r.OK
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// check routes a result to either a hard failure (assertOn) or an advisory log
// (logCheck) based on `hard`. The integration suite enforces correctness floors
// hard and treats statistical-quality targets as advisory by default; flipping
// enforcePlacementStats makes the targets hard once an algorithm is known to
// meet them.
func check(t *testing.T, r CheckResult, hard bool) bool {
	t.Helper()
	if hard {
		return r.assertOn(t)
	}
	return logCheck(t, r)
}

// ── 0. Vdev structure ───────────────────────────────────────────────────────
// Foundational invariant: the right number of chunks, each with exactly the
// expected block composition. For replicated vdevs wantReplica = replica count
// (DataBlkCnt) and wantData/wantParity are 0; for EC, wantData/wantParity are
// the stripe widths and wantReplica is 0.
func evalVdevStructure(rows []PlacementRow, wantChunks, wantData, wantParity, wantReplica int) CheckResult {
	res := newCheck("vdev structure (chunk count & block composition)")
	byChunk := make(map[uint32]map[cpLib.ChunkType]int)
	for _, r := range rows {
		if byChunk[r.ChunkID] == nil {
			byChunk[r.ChunkID] = make(map[cpLib.ChunkType]int)
		}
		byChunk[r.ChunkID][r.Type]++
	}
	if len(byChunk) != wantChunks {
		res.fail(fmt.Sprintf("got %d chunks, want %d", len(byChunk), wantChunks))
	}
	chunks := make([]uint32, 0, len(byChunk))
	for c := range byChunk {
		chunks = append(chunks, c)
	}
	sort.Slice(chunks, func(i, j int) bool { return chunks[i] < chunks[j] })
	for _, c := range chunks {
		m := byChunk[c]
		if m[cpLib.Data] != wantData || m[cpLib.Parity] != wantParity || m[cpLib.Replica] != wantReplica {
			res.fail(fmt.Sprintf("chunk %d: data=%d parity=%d replica=%d, want data=%d parity=%d replica=%d",
				c, m[cpLib.Data], m[cpLib.Parity], m[cpLib.Replica], wantData, wantParity, wantReplica))
		}
	}
	return res.finalize()
}

// ── 1. Chunk uniqueness ─────────────────────────────────────────────────────
// Rule: a single chunk must never place two of its blocks on the same NISD.
func evalChunkUniqueness(rows []PlacementRow) CheckResult {
	res := newCheck("chunk NISD uniqueness")
	type key struct {
		vdev  string
		chunk uint32
	}
	perChunk := make(map[key]map[string]int)
	for _, r := range rows {
		k := key{r.VdevID, r.ChunkID}
		if perChunk[k] == nil {
			perChunk[k] = make(map[string]int)
		}
		perChunk[k][r.NisdID]++
	}
	for k, nisdCounts := range perChunk {
		for nisd, cnt := range nisdCounts {
			if cnt > 1 {
				res.fail(fmt.Sprintf("vdev %s chunk %d: %d blocks on NISD %s", k.vdev, k.chunk, cnt, nisd))
			}
		}
	}
	return res.finalize()
}

// ── 2. Failure-domain diversity (per chunk, per level) ──────────────────────
// Rules 1-5: maximise distinct entities used by each chunk at the given level.
// The achievable maximum is min(blocksPerChunk, eligibleEntities); a good
// algorithm reaches it.
func evalChunkDiversity(rows []PlacementRow, eligible *HierarchyModel, lvl Level) CheckResult {
	res := newCheck(fmt.Sprintf("per-chunk %s diversity", lvl.Name()))
	maxEnt := eligible.EntityCount(lvl)

	type key struct {
		vdev  string
		chunk uint32
	}
	groups := make(map[key][]PlacementRow)
	for _, r := range rows {
		k := key{r.VdevID, r.ChunkID}
		groups[k] = append(groups[k], r)
	}
	for k, rs := range groups {
		ideal := min2(len(rs), maxEnt)
		seen := make(map[string]struct{})
		for _, r := range rs {
			seen[r.Keys[lvl]] = struct{}{}
		}
		if len(seen) < ideal {
			res.fail(fmt.Sprintf("vdev %s chunk %d: %d distinct %ss, want %d",
				k.vdev, k.chunk, len(seen), lvl.Name(), ideal))
		}
	}
	return res.finalize()
}

// evalVdevSpread checks how many distinct entities a whole vdev touches at lvl,
// versus the achievable maximum min(totalPlacements, eligibleEntities). Unlike
// per-chunk diversity it aggregates across chunks, which is the right lens when
// there are few placements and many entities ("more NISDs than chunks"): it
// catches piling everything onto a subset without being fooled by the many
// necessarily-empty entities that would sink a Jain-style fairness score.
func evalVdevSpread(rows []PlacementRow, eligible *HierarchyModel, lvl Level) CheckResult {
	res := newCheck(fmt.Sprintf("vdev %s spread", lvl.Name()))
	ideal := min2(len(rows), eligible.EntityCount(lvl))
	distinct := uniqueCount(rows, lvl)
	if distinct < ideal {
		res.fail(fmt.Sprintf("touched %d distinct %ss, achievable max is %d (entities piled onto a subset)",
			distinct, lvl.Name(), ideal))
	}
	return res.finalize()
}

// ── 3. Distribution fairness (per level, per type class) ────────────────────
// Rules 6-15 (and bias detection 19-22): placement counts across eligible
// entities at lvl must satisfy the statistical thresholds.
func evalFairness(rows []PlacementRow, eligible *HierarchyModel, lvl Level, class TypeClass, thr FairnessThresholds) (CheckResult, DistStats) {
	s := statsByLevel(rows, lvl, class, eligible)
	ok, reasons := s.Check(thr)
	res := CheckResult{
		Name:    fmt.Sprintf("fairness %s @ %s [%s]", class.Name(), lvl.Name(), thr.Description),
		OK:      ok,
		Reasons: reasons,
	}
	return res.finalize(), s
}

// ── 4. Capacity compliance ──────────────────────────────────────────────────
// Rules 16,18: bytes allocated to a NISD must never exceed its available size.
func evalCapacityCompliance(rows []PlacementRow, model *HierarchyModel) CheckResult {
	res := newCheck("capacity compliance")
	alloc := make(map[string]int64)
	for _, r := range rows {
		alloc[r.NisdID] += r.ChunkSize
	}
	ids := make([]string, 0, len(alloc))
	for id := range alloc {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		info, ok := model.Nisd(id)
		if !ok {
			res.fail(fmt.Sprintf("placement on unknown NISD %s", id))
			continue
		}
		if alloc[id] > info.AvailableSize {
			res.fail(fmt.Sprintf("NISD %s overcommitted: %d allocated > %d available",
				id, alloc[id], info.AvailableSize))
		}
	}
	return res.finalize()
}

// ── 5. Proportional allocation ──────────────────────────────────────────────
// Rule 17: for different-sized storage, an entity's share of placements should
// track its share of available capacity, within tol (absolute share points).
// For equal-sized storage this degenerates to "evenly distributed".
func evalProportional(rows []PlacementRow, model *HierarchyModel, lvl Level, class TypeClass, tol float64) CheckResult {
	res := newCheck(fmt.Sprintf("proportional %s @ %s (tol %.0f%%)", class.Name(), lvl.Name(), tol*100))
	counts := CountsByLevel(rows, lvl, class, model)
	capByE := model.CapacityByEntity(lvl)

	totalPlace := 0
	for _, v := range counts {
		totalPlace += v
	}
	var totalCap int64
	for e := range counts {
		totalCap += capByE[e]
	}
	if totalPlace == 0 || totalCap == 0 {
		return res.finalize()
	}

	ents := make([]string, 0, len(counts))
	for e := range counts {
		ents = append(ents, e)
	}
	sort.Strings(ents)
	for _, e := range ents {
		expShare := float64(capByE[e]) / float64(totalCap)
		actShare := float64(counts[e]) / float64(totalPlace)
		if diff := actShare - expShare; diff > tol || diff < -tol {
			res.fail(fmt.Sprintf("%s %s: actual share %.3f vs expected %.3f (cap-proportional)",
				lvl.Name(), shortID(e), actShare, expShare))
		}
	}
	return res.finalize()
}

// ── 6. Rebalance efficiency / minimal movement ──────────────────────────────
// Rules 23-25: after topology growth, the algorithm must move enough data to
// rebalance but no more than necessary, and moved data should land on the new
// capacity.
type MovementStats struct {
	Before       int
	After        int
	Moved        int
	BytesMoved   int64
	MovementPct  float64 // Moved / Before
	MovedOntoNew int
	OntoNewPct   float64 // MovedOntoNew / Moved
}

// blockKey identifies the same logical block across a before/after snapshot.
type blockKey struct {
	vdev  string
	chunk uint32
	typ   cpLib.ChunkType
	seq   int
}

func keyOf(r PlacementRow) blockKey {
	return blockKey{r.VdevID, r.ChunkID, r.Type, r.Seq}
}

// computeMovement diffs two placement snapshots. isNew reports whether a row now
// sits on capacity that did not exist in the "before" topology.
func computeMovement(before, after []PlacementRow, isNew func(PlacementRow) bool) MovementStats {
	bmap := make(map[blockKey]PlacementRow, len(before))
	for _, r := range before {
		bmap[keyOf(r)] = r
	}
	ms := MovementStats{Before: len(before), After: len(after)}
	for _, r := range after {
		b, ok := bmap[keyOf(r)]
		if ok && b.NisdID != r.NisdID {
			ms.Moved++
			ms.BytesMoved += r.ChunkSize
			if isNew(r) {
				ms.MovedOntoNew++
			}
		}
	}
	if ms.Before > 0 {
		ms.MovementPct = float64(ms.Moved) / float64(ms.Before)
	}
	if ms.Moved > 0 {
		ms.OntoNewPct = float64(ms.MovedOntoNew) / float64(ms.Moved)
	}
	return ms
}

// evalRebalance checks the movement is within [optimalPct*(1-slack),
// optimalPct*(1+slack)] and that moved blocks predominantly land on new capacity.
// optimalPct is the theoretical minimum movement fraction = newCapacityShare.
func evalRebalance(before, after []PlacementRow, isNew func(PlacementRow) bool, optimalPct, slack float64) (CheckResult, MovementStats) {
	res := newCheck("rebalance minimal movement")
	ms := computeMovement(before, after, isNew)
	lo := optimalPct * (1 - slack)
	hi := optimalPct * (1 + slack)
	if ms.MovementPct > hi {
		res.fail(fmt.Sprintf("moved %.1f%% > %.1f%% ceiling (excessive redistribution)",
			ms.MovementPct*100, hi*100))
	}
	if ms.MovementPct < lo {
		res.fail(fmt.Sprintf("moved %.1f%% < %.1f%% floor (did not rebalance onto new capacity)",
			ms.MovementPct*100, lo*100))
	}
	if ms.Moved > 0 && ms.OntoNewPct < 0.90 {
		res.fail(fmt.Sprintf("only %.0f%% of moved blocks landed on new capacity (expected ~100%%)",
			ms.OntoNewPct*100))
	}
	return res.finalize(), ms
}

// ── Assert* wrappers (used by the integration suite) ────────────────────────

func AssertChunkUniqueness(t *testing.T, rows []PlacementRow) {
	t.Helper()
	evalChunkUniqueness(rows).assertOn(t)
}

// AssertVdevDiversity checks every chunk reaches maximum diversity at the given
// levels, against the eligible (possibly filter-scoped) model. With no levels it
// defaults to all. Callers pass the levels they want enforced as hard failures
// (NISD/Device per rules 1-2); higher levels are usually checked via logCheck
// because the spec only asks to maximise them "whenever possible" (rules 3-5).
func AssertVdevDiversity(t *testing.T, rows []PlacementRow, eligible *HierarchyModel, levels ...Level) {
	t.Helper()
	if len(levels) == 0 {
		levels = AllLevels
	}
	for _, lvl := range levels {
		evalChunkDiversity(rows, eligible, lvl).assertOn(t)
	}
}

// LogVdevDiversity reports per-chunk diversity at the given levels as advisory.
func LogVdevDiversity(t *testing.T, rows []PlacementRow, eligible *HierarchyModel, levels ...Level) {
	t.Helper()
	for _, lvl := range levels {
		logCheck(t, evalChunkDiversity(rows, eligible, lvl))
	}
}

func AssertFairness(t *testing.T, rows []PlacementRow, eligible *HierarchyModel, lvl Level, class TypeClass, thr FairnessThresholds) DistStats {
	t.Helper()
	res, s := evalFairness(rows, eligible, lvl, class, thr)
	res.assertOn(t)
	return s
}

// AssertFairnessAllLevels applies one threshold set to data and parity across
// every hierarchy level — the bulk of rules 6-15 in a single call.
func AssertFairnessAllLevels(t *testing.T, rows []PlacementRow, eligible *HierarchyModel, thr FairnessThresholds) {
	t.Helper()
	for _, class := range []TypeClass{ClassData, ClassParity} {
		for _, lvl := range AllLevels {
			res, _ := evalFairness(rows, eligible, lvl, class, thr)
			res.assertOn(t)
		}
	}
}

func AssertCapacityCompliance(t *testing.T, rows []PlacementRow, model *HierarchyModel) {
	t.Helper()
	evalCapacityCompliance(rows, model).assertOn(t)
}

func AssertProportional(t *testing.T, rows []PlacementRow, model *HierarchyModel, lvl Level, class TypeClass, tol float64) {
	t.Helper()
	evalProportional(rows, model, lvl, class, tol).assertOn(t)
}

// AssertNoBias is bias detection (rules 19-22): it is fairness under the strict
// equal-sized thresholds, which is exactly "do not repeatedly favour one entity
// when equivalent alternatives exist".
func AssertNoBias(t *testing.T, rows []PlacementRow, eligible *HierarchyModel, lvl Level, class TypeClass) {
	t.Helper()
	res, _ := evalFairness(rows, eligible, lvl, class, EqualSizedThresholds())
	res.assertOn(t)
}

func AssertRebalanceMinimal(t *testing.T, before, after []PlacementRow, isNew func(PlacementRow) bool, optimalPct, slack float64) MovementStats {
	t.Helper()
	res, ms := evalRebalance(before, after, isNew, optimalPct, slack)
	res.assertOn(t)
	return ms
}

// ════════════════════════════════════════════════════════════════════════════
// Pure meta-test: proves the validators accept good distributions and reject
// bad ones. Runs without a cluster, so the trustworthiness of the whole suite
// is itself under test.
// ════════════════════════════════════════════════════════════════════════════

func mkRow(vdev string, chunk uint32, typ cpLib.ChunkType, n NisdInfo, seq int) PlacementRow {
	return PlacementRow{
		Vdev: vdev, VdevID: vdev, ChunkID: chunk, Type: typ,
		ChunkSize: cpLib.CHUNK_SIZE, Seq: seq, NisdID: n.ID, Keys: n.Keys,
	}
}

func TestPlacementValidationHelpers(t *testing.T) {
	// Balanced, equal-sized reference topology: 2x2x2x2x2.
	model := balancedEqual("mt", 2, 2, 2, 2, 2, 100*gib).BuildModel()
	require.Equal(t, 2, model.EntityCount(LevelPDU))
	require.Equal(t, 4, model.EntityCount(LevelRack))
	require.Equal(t, 8, model.EntityCount(LevelHV))
	require.Equal(t, 16, model.EntityCount(LevelDevice))
	require.Equal(t, 32, model.EntityCount(LevelNISD))

	nisds := model.NisdsSorted()
	deviceIDs := model.Entities(LevelDevice)
	devGroups := model.Group(LevelDevice)

	t.Run("fairness accepts even, rejects biased", func(t *testing.T) {
		// Even: every NISD gets exactly 3 placements -> perfect at all levels.
		var even []PlacementRow
		for _, n := range nisds {
			for c := 0; c < 3; c++ {
				even = append(even, mkRow("v-even", uint32(c), cpLib.Replica, n, 0))
			}
		}
		for _, lvl := range AllLevels {
			res, s := evalFairness(even, model, lvl, ClassData, EqualSizedThresholds())
			require.Truef(t, res.OK, "even data should pass at %s: %v (Jain=%.3f CV=%.3f)",
				lvl.Name(), res.Reasons, s.Jain, s.CV)
		}
		require.True(t, evalCapacityCompliance(even, model).OK, "24GB per 100GB NISD is compliant")

		// Biased: everything on a single NISD -> must fail fairness everywhere.
		var biased []PlacementRow
		for c := 0; c < 9; c++ {
			biased = append(biased, mkRow("v-bias", uint32(c), cpLib.Replica, nisds[0], 0))
		}
		for _, lvl := range AllLevels {
			res, _ := evalFairness(biased, model, lvl, ClassData, EqualSizedThresholds())
			require.Falsef(t, res.OK, "biased data should fail fairness at %s", lvl.Name())
		}
	})

	t.Run("uniqueness and diversity", func(t *testing.T) {
		// Good chunk: EC 4+3 across 7 distinct devices (one NISD each).
		var good []PlacementRow
		for i := 0; i < 7; i++ {
			n := devGroups[deviceIDs[i]][0]
			typ := cpLib.Data
			if i >= 4 {
				typ = cpLib.Parity
			}
			good = append(good, mkRow("v-good", 0, typ, n, i))
		}
		require.True(t, evalChunkUniqueness(good).OK)
		require.True(t, evalChunkDiversity(good, model, LevelDevice).OK, "7 distinct devices is maximal")
		require.True(t, evalChunkDiversity(good, model, LevelNISD).OK)

		// Duplicate NISD in a chunk -> uniqueness must fail.
		dup := append([]PlacementRow{}, good...)
		dup[1] = mkRow("v-dup", 0, cpLib.Data, devGroups[deviceIDs[0]][0], 1) // same NISD as block 0
		for i := range dup {
			dup[i].VdevID, dup[i].Vdev = "v-dup", "v-dup"
		}
		require.False(t, evalChunkUniqueness(dup).OK, "duplicate NISD must be caught")

		// Two blocks on two NISDs of the SAME device -> uniqueness OK, device
		// diversity must fail (only 6 distinct devices for 7 blocks).
		var sameDev []PlacementRow
		d0 := devGroups[deviceIDs[0]] // device 0 has 2 NISDs
		sameDev = append(sameDev, mkRow("v-sd", 0, cpLib.Data, d0[0], 0))
		sameDev = append(sameDev, mkRow("v-sd", 0, cpLib.Data, d0[1], 1))
		for i := 1; i < 6; i++ { // 5 more distinct devices
			sameDev = append(sameDev, mkRow("v-sd", 0, cpLib.Data, devGroups[deviceIDs[i]][0], i+1))
		}
		require.True(t, evalChunkUniqueness(sameDev).OK, "distinct NISDs even on same device")
		require.False(t, evalChunkDiversity(sameDev, model, LevelDevice).OK, "device diversity short by one")
	})

	t.Run("proportional allocation tracks capacity", func(t *testing.T) {
		// 4 devices, one NISD each: dev0=150G, dev1..3=50G. Total 300G.
		dm := balancedDifferentSized("mtp", 1, 1, 1, 4, 1, 50*gib, 150*gib).BuildModel()
		dDevs := dm.Entities(LevelDevice)
		dGroups := dm.Group(LevelDevice)
		// Expected shares: 0.5, 0.1667, 0.1667, 0.1667.
		want := map[int]int{0: 6, 1: 2, 2: 2, 3: 2} // proportional counts, total 12
		var prop []PlacementRow
		for i := 0; i < 4; i++ {
			n := dGroups[dDevs[i]][0]
			for c := 0; c < want[i]; c++ {
				prop = append(prop, mkRow("v-prop", uint32(c), cpLib.Replica, n, i))
			}
		}
		require.True(t, evalProportional(prop, dm, LevelDevice, ClassData, 0.10).OK,
			"capacity-proportional counts should pass")

		// Even counts (3 each) ignore that dev0 is 3x larger -> must fail.
		var flat []PlacementRow
		for i := 0; i < 4; i++ {
			n := dGroups[dDevs[i]][0]
			for c := 0; c < 3; c++ {
				flat = append(flat, mkRow("v-flat", uint32(c), cpLib.Replica, n, i))
			}
		}
		require.False(t, evalProportional(flat, dm, LevelDevice, ClassData, 0.10).OK,
			"ignoring capacity must fail proportional check")
	})

	t.Run("rebalance after device addition", func(t *testing.T) {
		b := balancedEqual("mtr", 1, 1, 1, 4, 1, 100*gib) // 4 devices
		before4 := b.BuildModel()
		bDevs := before4.Entities(LevelDevice)
		bGroups := before4.Group(LevelDevice)

		// 8 single-replica chunks spread 2 per device.
		var before []PlacementRow
		for i := 0; i < 8; i++ {
			n := bGroups[bDevs[i%4]][0]
			before = append(before, mkRow("rv", uint32(i), cpLib.Replica, n, 0))
		}

		// Add a 5th device. Optimal movement share = 1/5 = 0.20.
		b.addBranch(1, 1, 1, 1, uniformSize(100*gib))
		after5 := b.BuildModel()
		newDevs := setDiff(after5.Entities(LevelDevice), bDevs)
		require.Len(t, newDevs, 1)
		newNisd := after5.Group(LevelDevice)[newDevs[0]][0]
		isNew := func(r PlacementRow) bool { return r.Keys[LevelDevice] == newDevs[0] }

		// Good: move exactly 2 of 8 blocks (25%) onto the new device.
		good := append([]PlacementRow{}, before...)
		good[0] = mkRow("rv", 0, cpLib.Replica, newNisd, 0)
		good[1] = mkRow("rv", 1, cpLib.Replica, newNisd, 0)
		res, ms := evalRebalance(before, good, isNew, 0.20, 0.5)
		require.Truef(t, res.OK, "minimal rebalance should pass: %v (moved %.0f%%)", res.Reasons, ms.MovementPct*100)
		require.InDelta(t, 1.0, ms.OntoNewPct, 1e-9)

		// Bad: reshuffle every block onto the new device (100% movement).
		var churn []PlacementRow
		for i := 0; i < 8; i++ {
			churn = append(churn, mkRow("rv", uint32(i), cpLib.Replica, newNisd, 0))
		}
		res, _ = evalRebalance(before, churn, isNew, 0.20, 0.5)
		require.False(t, res.OK, "excessive redistribution must fail")

		// Bad: no movement at all after growth -> under-rebalanced.
		res, _ = evalRebalance(before, before, isNew, 0.20, 0.5)
		require.False(t, res.OK, "ignoring new capacity must fail")
	})
}

func setDiff(all, base []string) []string {
	in := make(map[string]struct{}, len(base))
	for _, b := range base {
		in[b] = struct{}{}
	}
	var out []string
	for _, a := range all {
		if _, ok := in[a]; !ok {
			out = append(out, a)
		}
	}
	return out
}
