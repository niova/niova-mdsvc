package clictlplanefuncs

// placement_comparison_report_test.go
//
// Renders one HTML report comparing several ScenarioDocs side by side (e.g.
// balancedequal vs balanceddifferentsized vs unbalancedequal vs
// unbalanceddifferentsized), instead of one file per scenario. This is the
// right lens for the topology matrix in TestPlacementDistribution, where the
// interesting question is how a stat changes *across* hierarchies rather than
// what it is in isolation.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

// writeComparisonHTMLReport writes a single HTML report comparing every
// scenario in docs to ./placement_report_comparison.html.
func writeComparisonHTMLReport(t *testing.T, docs []*ScenarioDoc) {
	t.Helper()
	timestamp := time.Now().Format("20060102_150405")
	path := filepath.Join(".", fmt.Sprintf("placement_report_comparison_%s.html", timestamp))
	if err := os.WriteFile(path, []byte(renderComparisonHTMLReport(docs)), 0644); err != nil {
		t.Logf("warning: could not write comparison HTML report: %v", err)
		return
	}
	t.Logf("Comparison HTML report written: %s (open in browser → File → Print → Save as PDF)", path)
}

// checkFamily strips scenario-specific detail (fairness thresholds,
// proportional tolerance) from a check name so the same validation rule can
// be pivoted into one row across scenarios that use different thresholds.
func checkFamily(name string) string {
	if strings.HasPrefix(name, "fairness ") {
		if i := strings.Index(name, " ["); i >= 0 {
			return name[:i]
		}
	}
	if strings.HasPrefix(name, "proportional ") {
		if i := strings.Index(name, " (tol"); i >= 0 {
			return name[:i]
		}
	}
	return name
}

// scenarioChecks flattens a scenario's global + Device/NISD-level checks into
// one map keyed by check family, merging repeated checks (e.g. one per vdev)
// into a single pass/fail per family. PDU/Rack/HV checks are excluded to match
// the hierarchy-stats sections, which no longer report on those levels.
func scenarioChecks(d *ScenarioDoc) (order []string, byFamily map[string]CheckResult) {
	byFamily = make(map[string]CheckResult)
	add := func(cr CheckResult) {
		fam := checkFamily(cr.Name)
		existing, ok := byFamily[fam]
		if !ok {
			existing = CheckResult{Name: fam, Desc: cr.Desc, OK: true}
			order = append(order, fam)
		}
		existing.OK = existing.OK && cr.OK
		existing.Reasons = append(existing.Reasons, cr.Reasons...)
		byFamily[fam] = existing
	}
	for _, cr := range d.GlobalChecks {
		add(cr)
	}
	for _, ls := range d.Levels {
		if ls.Lvl == LevelPDU || ls.Lvl == LevelRack || ls.Lvl == LevelHV {
			continue
		}
		for _, cr := range ls.Checks {
			add(cr)
		}
	}
	return order, byFamily
}

// buildComparisonChecks runs scenarioChecks over every doc and returns the
// union of check families in first-seen order plus one lookup map per doc.
func buildComparisonChecks(docs []*ScenarioDoc) (order []string, perScenario []map[string]CheckResult) {
	perScenario = make([]map[string]CheckResult, len(docs))
	seen := make(map[string]bool)
	for i, d := range docs {
		famOrder, byFamily := scenarioChecks(d)
		perScenario[i] = byFamily
		for _, fam := range famOrder {
			if !seen[fam] {
				seen[fam] = true
				order = append(order, fam)
			}
		}
	}
	return order, perScenario
}

// firstDesc returns the description recorded for fam by whichever scenario
// happened to run it (identical across scenarios in practice).
func firstDesc(perScenario []map[string]CheckResult, fam string) string {
	for _, m := range perScenario {
		if cr, ok := m[fam]; ok && cr.Desc != "" {
			return cr.Desc
		}
	}
	return ""
}

// levelSection returns d's LevelSection for lvl.
func levelSection(d *ScenarioDoc, lvl Level) (LevelSection, bool) {
	for _, ls := range d.Levels {
		if ls.Lvl == lvl {
			return ls, true
		}
	}
	return LevelSection{}, false
}

// spaceUtilization is the fraction of the eligible pool's available capacity
// that the placement consumed (allocated bytes / available bytes). It answers
// "how full did this workload make the cluster", independent of topology size,
// and is comparable across scenarios because every scenario runs the same vdev
// set. ok is false when capacity is unknown.
func spaceUtilization(d *ScenarioDoc) (util float64, ok bool) {
	if d.Model == nil {
		return 0, false
	}
	var avail, allocated int64
	for _, v := range d.Model.CapacityByEntity(LevelNISD) {
		avail += v
	}
	for _, r := range d.Rows {
		allocated += r.ChunkSize
	}
	if avail == 0 {
		return 0, false
	}
	return float64(allocated) / float64(avail), true
}

// uniquenessUtilization measures how well the placement spread each chunk's
// blocks across distinct entities at lvl, as a fraction of the best achievable.
// For each chunk the ideal is min(blocksInChunk, entitiesAvailable) distinct
// entities (any more is impossible); the score is the mean over all chunks of
// distinctUsed / idealDistinct. 1.0 means every chunk fanned its blocks out as
// widely as the topology allows — the ideal for fault tolerance. This is the
// diversity counterpart to space utilization and directly reflects the chunk
// diversity/uniqueness checks. ok is false when there are no placements.
func uniquenessUtilization(d *ScenarioDoc, lvl Level) (util float64, ok bool) {
	if d.Model == nil {
		return 0, false
	}
	maxEntities := d.Model.EntityCount(lvl)
	type chunkKey struct {
		vdev  string
		chunk uint32
	}
	distinct := make(map[chunkKey]map[string]struct{})
	blocks := make(map[chunkKey]int)
	for _, r := range d.Rows {
		k := chunkKey{r.VdevID, r.ChunkID}
		if distinct[k] == nil {
			distinct[k] = make(map[string]struct{})
		}
		distinct[k][r.Keys[lvl]] = struct{}{}
		blocks[k]++
	}
	if len(distinct) == 0 {
		return 0, false
	}
	var sum float64
	for k, set := range distinct {
		ideal := blocks[k]
		if maxEntities < ideal {
			ideal = maxEntities
		}
		if ideal == 0 {
			continue
		}
		sum += float64(len(set)) / float64(ideal)
	}
	return sum / float64(len(distinct)), true
}

// pctStr renders a 0..1 ratio as a percentage string, or "n/a" when !ok.
func pctStr(v float64, ok bool) string {
	if !ok {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", v*100)
}

// renderComparisonHTMLReport builds the complete HTML string comparing docs.
func renderComparisonHTMLReport(docs []*ScenarioDoc) string {
	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Placement Stats Comparison</title>
<style>
body{font-family:Arial,sans-serif;margin:2em;color:#222;font-size:14px}
h1{background:#2c3e50;color:#fff;padding:.5em 1em;border-radius:4px;margin-bottom:.3em}
h2{background:#34495e;color:#fff;padding:.3em .8em;margin-top:1.5em;border-radius:3px}
h3{color:#2c3e50;border-bottom:2px solid #2c3e50;padding-bottom:.2em;margin-top:1.2em}
table{border-collapse:collapse;width:100%;margin:.5em 0 1em;font-size:.9em}
th{background:#ecf0f1;text-align:left;padding:5px 10px;border:1px solid #bdc3c7;white-space:nowrap}
td{padding:5px 10px;border:1px solid #bdc3c7;vertical-align:top}
tr:nth-child(even){background:#f9f9f9}
.pass{color:#27ae60;font-weight:bold}
.fail{color:#c0392b;font-weight:bold}
.na{color:#95a5a6}
.desc{color:#7f8c8d;font-size:.85em;font-style:italic;margin-top:2px}
.reason{margin:2px 0;color:#555;font-size:.85em}
.meta{color:#7f8c8d;font-style:italic;margin-bottom:.5em}
.section{margin-bottom:1.5em}
details summary{cursor:pointer}
@media print{h2{-webkit-print-color-adjust:exact;print-color-adjust:exact}}
</style>
</head>
<body>
`)

	fmt.Fprintf(&b, "<h1>Placement Stats Comparison Report</h1>\n")
	fmt.Fprintf(&b, "<p class=\"meta\">Generated: %s | %d scenarios</p>\n",
		time.Now().Format("2006-01-02 15:04:05"), len(docs))

	// ── Scenario legend ──────────────────────────────────────────────────────
	b.WriteString("<h2>Scenarios</h2>\n<div class=\"section\">\n")
	b.WriteString("<table><tr><th>Scenario</th><th>Purpose</th><th>Validates</th></tr>\n")
	for _, d := range docs {
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%s</td></tr>\n",
			he(d.ScenarioName), he(d.Why), he(d.Validates))
	}
	b.WriteString("</table>\n</div>\n")

	// ── 1. Cluster capacity & topology comparison ────────────────────────────
	b.WriteString("<h2>1. Cluster Capacity &amp; Topology</h2>\n<div class=\"section\">\n")
	b.WriteString("<table><tr><th>Scenario</th><th>Total Capacity</th><th>Available</th><th>Allocated</th><th>Free</th><th>Space Util</th>")
	for _, lvl := range AllLevels {
		fmt.Fprintf(&b, "<th>%s</th>", lvl.Name())
	}
	b.WriteString("</tr>\n")
	for _, d := range docs {
		if d.Model == nil {
			continue
		}
		var totalCap, availCap int64
		for _, v := range totalCapByLevel(d.Model, LevelNISD) {
			totalCap += v
		}
		for _, v := range d.Model.CapacityByEntity(LevelNISD) {
			availCap += v
		}
		var allocated int64
		for _, r := range d.Rows {
			allocated += r.ChunkSize
		}
		util, ok := spaceUtilization(d)
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td>",
			he(d.ScenarioName), gibStr(totalCap), gibStr(availCap), gibStr(allocated), gibStr(availCap-allocated), pctStr(util, ok))
		for _, lvl := range AllLevels {
			fmt.Fprintf(&b, "<td>%d</td>", d.Model.EntityCount(lvl))
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</table>\n")
	b.WriteString("<p class=\"desc\">Space Util = allocated / available capacity: how full the workload made the eligible NISD pool.</p>\n")

	// Device-size breakdown per scenario: heterogeneous topologies place devices
	// of several distinct capacities, and the size mix is what makes proportional
	// allocation meaningful, so show it alongside the totals.
	b.WriteString("<p><strong>Device sizes (count of devices / NISDs at each distinct capacity):</strong></p>\n")
	b.WriteString("<table><tr><th>Scenario</th><th>Device Size</th><th>Device Count</th><th>NISD Count</th></tr>\n")
	for _, d := range docs {
		if d.Model == nil {
			continue
		}
		sizes, devCounts, nisdCounts := deviceSizeHistogram(d.Model)
		for i, sz := range sizes {
			name := ""
			if i == 0 {
				name = he(d.ScenarioName)
			}
			fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%d</td><td>%d</td></tr>\n",
				name, gibStr(sz), devCounts[sz], nisdCounts[sz])
		}
	}
	b.WriteString("</table>\n</div>\n")

	// ── 2. Vdev summary (identical across scenarios: standardVdevPlans) ──────
	if len(docs) > 0 {
		fmt.Fprintf(&b, "<h2>2. Vdevs (%d total, same set run in every scenario)</h2>\n<div class=\"section\">\n", len(docs[0].Plans))
		b.WriteString("<table><tr><th>Name</th><th>Type</th><th>DataBlks</th><th>ParityBlks</th><th>Chunks</th><th>Total Size</th></tr>\n")
		for _, p := range docs[0].Plans {
			typ := "Replica"
			if p.isEC() {
				typ = "EC"
			}
			totalSize := p.chunks * cpLib.CHUNK_SIZE
			fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td>%d</td><td>%s</td></tr>\n",
				he(p.name), typ, p.dataBlk, p.parityBlk, p.chunks, gibStr(totalSize))
		}
		b.WriteString("</table>\n</div>\n")
	}

	// ── 3. Utilization metrics ───────────────────────────────────────────────
	// Two single-number scores per scenario that summarise the placement from
	// two orthogonal angles: how much capacity it consumed (space) and how well
	// it spread each chunk for fault tolerance (uniqueness).
	b.WriteString("<h2>3. Utilization Metrics</h2>\n<div class=\"section\">\n")
	b.WriteString("<table><tr><th>Scenario</th><th>Space Utilization</th>" +
		"<th>Uniqueness Utilization (Device)</th><th>Uniqueness Utilization (NISD)</th></tr>\n")
	for _, d := range docs {
		spaceU, spaceOK := spaceUtilization(d)
		devU, devOK := uniquenessUtilization(d, LevelDevice)
		nisdU, nisdOK := uniquenessUtilization(d, LevelNISD)
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n",
			he(d.ScenarioName), pctStr(spaceU, spaceOK), pctStr(devU, devOK), pctStr(nisdU, nisdOK))
	}
	b.WriteString("</table>\n")
	b.WriteString("<p class=\"desc\"><strong>Space Utilization</strong> = allocated / available capacity — how full the workload made the eligible pool.</p>\n")
	b.WriteString("<p class=\"desc\"><strong>Uniqueness Utilization</strong> = mean over chunks of (distinct entities a chunk's blocks landed on) / (best achievable = min(blocks in chunk, entities available)). 100% means every chunk fanned its blocks across as many distinct entities as the topology allows — the ideal for fault tolerance.</p>\n")
	b.WriteString("</div>\n")

	// ── 4. Hierarchy statistics comparison ───────────────────────────────────
	b.WriteString("<h2>4. Hierarchy Statistics Comparison</h2>\n")
	for _, lvl := range []Level{LevelDevice, LevelNISD} {
		fmt.Fprintf(&b, "<div class=\"section\"><h3>%s</h3>\n", lvl.Name())
		for _, cls := range []struct {
			label string
			pick  func(ls LevelSection) DistStats
		}{
			{"Data", func(ls LevelSection) DistStats { return ls.DataStats }},
			{"Parity", func(ls LevelSection) DistStats { return ls.ParityStats }},
			{"All", func(ls LevelSection) DistStats { return ls.AllStats }},
		} {
			fmt.Fprintf(&b, "<p><strong>%s</strong></p>\n", cls.label)
			b.WriteString("<table>\n<tr><th>Scenario</th><th>Count</th><th>Mean</th><th>Min</th>" +
				"<th>Max</th><th>Range</th><th>StdDev</th><th>CV</th><th>Jain</th><th>Max/Min</th><th>Skew</th></tr>\n")
			for _, d := range docs {
				ls, ok := levelSection(d, lvl)
				if !ok {
					continue
				}
				s := cls.pick(ls)
				fmt.Fprintf(&b,
					"<tr><td>%s</td><td>%d</td><td>%.2f</td><td>%.0f</td><td>%.0f</td>"+
						"<td>%.0f</td><td>%.3f</td><td>%.3f</td><td>%.3f</td><td>%.3f</td><td>%.3f</td></tr>\n",
					he(d.ScenarioName), s.Count, s.Mean, s.Min, s.Max, s.Range, s.StdDev, s.CV, s.Jain, s.MaxMinRatio, s.Skew)
			}
			b.WriteString("</table>\n")
		}

		// Per-entity distribution: one small table per scenario, since the entity
		// ids differ across topologies and cannot be pivoted into shared columns.
		b.WriteString("<p><strong>Per-entity distribution:</strong></p>\n")
		for _, d := range docs {
			ls, ok := levelSection(d, lvl)
			if !ok || len(ls.DataCounts) == 0 {
				continue
			}
			fmt.Fprintf(&b, "<p class=\"desc\">%s</p>\n", he(d.ScenarioName))
			b.WriteString("<table><tr><th>Entity</th><th>Data</th><th>Parity</th><th>Total</th></tr>\n")
			for _, e := range sortedKeys(ls.DataCounts) {
				dc := ls.DataCounts[e]
				pc := ls.ParityCounts[e]
				fmt.Fprintf(&b, "<tr><td>%s</td><td>%d</td><td>%d</td><td>%d</td></tr>\n",
					he(shortID(e)), dc, pc, dc+pc)
			}
			b.WriteString("</table>\n")
		}
		b.WriteString("</div>\n")
	}

	// ── 4. Validation comparison ─────────────────────────────────────────────
	// Pivoted so each row is one validation rule (family) and each column is a
	// scenario: "n/a" means that scenario never ran the check (e.g. proportional
	// allocation is only evaluated for the different-sized topologies).
	b.WriteString("<h2>5. Validation Comparison</h2>\n<div class=\"section\">\n")
	order, perScenario := buildComparisonChecks(docs)
	b.WriteString("<table><tr><th>Validation</th>")
	for _, d := range docs {
		fmt.Fprintf(&b, "<th>%s</th>", he(d.ScenarioName))
	}
	b.WriteString("</tr>\n")
	for _, fam := range order {
		fmt.Fprintf(&b, "<tr><td><strong>%s</strong>", he(fam))
		if desc := firstDesc(perScenario, fam); desc != "" {
			fmt.Fprintf(&b, "<div class=\"desc\">%s</div>", he(desc))
		}
		b.WriteString("</td>")
		for i := range docs {
			cr, ok := perScenario[i][fam]
			if !ok {
				b.WriteString("<td class=\"na\">n/a</td>")
				continue
			}
			if cr.OK {
				b.WriteString("<td><span class=\"pass\">&#10003; PASS</span></td>")
			} else {
				b.WriteString("<td><details><summary><span class=\"fail\">&#10007; FAIL</span></summary>")
				for _, r := range cr.Reasons {
					fmt.Fprintf(&b, "<div class=\"reason\">&rarr; %s</div>", he(r))
				}
				b.WriteString("</details></td>")
			}
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</table>\n</div>\n")

	b.WriteString("</body>\n</html>\n")
	return b.String()
}
