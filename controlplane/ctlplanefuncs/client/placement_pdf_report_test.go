package clictlplanefuncs

// placement_pdf_report_test.go
//
// Generates an HTML placement-stats report that can be opened in a browser and
// printed to PDF (File → Print → Save as PDF). No external dependencies needed.
//
// The central type is ScenarioDoc, which integration tests fill as they run.
// Validation failures are recorded into the document under the relevant
// hierarchy level — tests never call t.Errorf/t.Fatalf for placement checks.
// Only API-level errors (cluster communication, vdev creation) still use
// require, because those reflect misconfigured test infrastructure, not the
// placement algorithm.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

// LevelSection holds per-level stats and all validation results collected
// during one scenario.
type LevelSection struct {
	Lvl          Level
	DataStats    DistStats
	ParityStats  DistStats
	AllStats     DistStats
	DataCounts   map[string]int
	ParityCounts map[string]int
	Checks       []CheckResult
}

// ScenarioDoc collects everything needed to render the HTML report for one
// scenario run.
type ScenarioDoc struct {
	ScenarioName string
	Why          string
	Validates    string
	Timestamp    time.Time
	Model        *HierarchyModel
	Plans        []vdevPlan
	Rows         []PlacementRow
	Levels       []LevelSection
	GlobalChecks []CheckResult
}

func newScenarioDoc(name, why, validates string) *ScenarioDoc {
	doc := &ScenarioDoc{
		ScenarioName: name,
		Why:          why,
		Validates:    validates,
		Timestamp:    time.Now(),
	}
	for _, lvl := range AllLevels {
		doc.Levels = append(doc.Levels, LevelSection{Lvl: lvl})
	}
	return doc
}

// addGlobal records a check that is not tied to one hierarchy level (vdev
// structure, chunk uniqueness, capacity compliance).
func (d *ScenarioDoc) addGlobal(r CheckResult) {
	d.GlobalChecks = append(d.GlobalChecks, r)
}

// addLevelCheck records a check under its hierarchy level.
func (d *ScenarioDoc) addLevelCheck(lvl Level, r CheckResult) {
	for i := range d.Levels {
		if d.Levels[i].Lvl == lvl {
			d.Levels[i].Checks = append(d.Levels[i].Checks, r)
			return
		}
	}
}

// finalise computes per-level statistics once all rows have been collected.
func (d *ScenarioDoc) finalise(rows []PlacementRow, eligible *HierarchyModel, plans []vdevPlan) {
	d.Rows = rows
	d.Model = eligible
	d.Plans = plans
	for i, ls := range d.Levels {
		lvl := ls.Lvl
		d.Levels[i].DataStats    = statsByLevel(rows, lvl, ClassData,   eligible)
		d.Levels[i].ParityStats  = statsByLevel(rows, lvl, ClassParity, eligible)
		d.Levels[i].AllStats     = statsByLevel(rows, lvl, ClassAll,    eligible)
		d.Levels[i].DataCounts   = CountsByLevel(rows, lvl, ClassData,   eligible)
		d.Levels[i].ParityCounts = CountsByLevel(rows, lvl, ClassParity, eligible)
	}
}

// writeHTMLReport writes the rendered HTML to
// ./placement_report_<scenarioName>.html and logs the path.
func writeHTMLReport(t *testing.T, d *ScenarioDoc) {
	t.Helper()
	safe := strings.NewReplacer("/", "_", " ", "_").Replace(d.ScenarioName)
	path := filepath.Join(".", fmt.Sprintf("placement_report_%s.html", safe))
	if err := os.WriteFile(path, []byte(renderHTMLReport(d)), 0644); err != nil {
		t.Logf("warning: could not write HTML report: %v", err)
		return
	}
	t.Logf("HTML report written: %s (open in browser → File → Print → Save as PDF)", path)
}

// renderHTMLReport builds the complete HTML string from a ScenarioDoc.
func renderHTMLReport(d *ScenarioDoc) string {
	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Placement Stats: ` + he(d.ScenarioName) + `</title>
<style>
body{font-family:Arial,sans-serif;margin:2em;color:#222;font-size:14px}
h1{background:#2c3e50;color:#fff;padding:.5em 1em;border-radius:4px;margin-bottom:.3em}
h2{background:#34495e;color:#fff;padding:.3em .8em;margin-top:1.5em;border-radius:3px}
h3{color:#2c3e50;border-bottom:2px solid #2c3e50;padding-bottom:.2em;margin-top:1.2em}
table{border-collapse:collapse;width:100%;margin:.5em 0 1em;font-size:.9em}
th{background:#ecf0f1;text-align:left;padding:5px 10px;border:1px solid #bdc3c7;white-space:nowrap}
td{padding:5px 10px;border:1px solid #bdc3c7}
tr:nth-child(even){background:#f9f9f9}
.pass{color:#27ae60;font-weight:bold}
.fail{color:#c0392b;font-weight:bold}
.reason{margin:2px 0 2px 1.5em;color:#555;font-size:.88em}
.meta{color:#7f8c8d;font-style:italic;margin-bottom:.5em}
.section{margin-bottom:1.5em}
ul{margin:.3em 0;padding-left:1.5em}
li{margin:.25em 0}
@media print{h2{-webkit-print-color-adjust:exact;print-color-adjust:exact}}
</style>
</head>
<body>
`)

	// ── Title block ────────────────────────────────────────────────────────────
	fmt.Fprintf(&b, "<h1>Placement Stats Report: %s</h1>\n", he(d.ScenarioName))
	fmt.Fprintf(&b, "<p class=\"meta\">Generated: %s</p>\n", d.Timestamp.Format("2006-01-02 15:04:05"))
	if d.Why != "" {
		fmt.Fprintf(&b, "<p><strong>Purpose:</strong> %s</p>\n", he(d.Why))
	}
	if d.Validates != "" {
		fmt.Fprintf(&b, "<p><strong>Validates:</strong> %s</p>\n", he(d.Validates))
	}

	// ── 1. Topology hierarchy ──────────────────────────────────────────────────
	if d.Model != nil {
		b.WriteString("<h2>1. Topology Hierarchy</h2>\n<div class=\"section\">\n")
		b.WriteString("<table><tr>")
		for _, lvl := range AllLevels {
			fmt.Fprintf(&b, "<th>%s</th>", lvl.Name())
		}
		b.WriteString("</tr><tr>")
		for _, lvl := range AllLevels {
			fmt.Fprintf(&b, "<td>%d</td>", d.Model.EntityCount(lvl))
		}
		b.WriteString("</tr></table>\n</div>\n")
	}

	// ── 2. Vdev summary ────────────────────────────────────────────────────────
	fmt.Fprintf(&b, "<h2>2. Vdevs (%d total)</h2>\n<div class=\"section\">\n", len(d.Plans))
	b.WriteString("<table><tr><th>Name</th><th>Type</th><th>DataBlks</th><th>ParityBlks</th><th>Chunks</th><th>Total Size</th></tr>\n")
	for _, p := range d.Plans {
		typ := "Replica"
		if p.isEC() {
			typ = "EC"
		}
		totalSize := p.chunks * cpLib.CHUNK_SIZE
		fmt.Fprintf(&b, "<tr><td>%s</td><td>%s</td><td>%d</td><td>%d</td><td>%d</td><td>%s</td></tr>\n",
			he(p.name), typ, p.dataBlk, p.parityBlk, p.chunks, gibStr(totalSize))
	}
	b.WriteString("</table>\n</div>\n")

	// ── 3. Per-level hierarchy statistics ─────────────────────────────────────
	b.WriteString("<h2>3. Hierarchy Statistics</h2>\n")
	for _, ls := range d.Levels {
		fmt.Fprintf(&b, "<div class=\"section\"><h3>%s</h3>\n", ls.Lvl.Name())

		// Stats table (data / parity / all)
		b.WriteString("<table>\n<tr><th>Class</th><th>Count</th><th>Mean</th><th>Min</th>" +
			"<th>Max</th><th>Range</th><th>StdDev</th><th>CV</th><th>Jain</th><th>Max/Min</th><th>Skew</th></tr>\n")
		for _, row := range []struct {
			label string
			s     DistStats
		}{
			{"data", ls.DataStats},
			{"parity", ls.ParityStats},
			{"all", ls.AllStats},
		} {
			s := row.s
			fmt.Fprintf(&b,
				"<tr><td>%s</td><td>%d</td><td>%.2f</td><td>%.0f</td><td>%.0f</td>"+
					"<td>%.0f</td><td>%.3f</td><td>%.3f</td><td>%.3f</td><td>%.3f</td><td>%.3f</td></tr>\n",
				row.label, s.Count, s.Mean, s.Min, s.Max, s.Range, s.StdDev, s.CV, s.Jain, s.MaxMinRatio, s.Skew)
		}
		b.WriteString("</table>\n")

		// Per-entity distribution
		if len(ls.DataCounts) > 0 {
			b.WriteString("<p><strong>Per-entity distribution:</strong></p>\n")
			b.WriteString("<table><tr><th>Entity</th><th>Data</th><th>Parity</th><th>Total</th></tr>\n")
			for _, e := range sortedKeys(ls.DataCounts) {
				dc := ls.DataCounts[e]
				pc := ls.ParityCounts[e]
				fmt.Fprintf(&b, "<tr><td>%s</td><td>%d</td><td>%d</td><td>%d</td></tr>\n",
					he(shortID(e)), dc, pc, dc+pc)
			}
			b.WriteString("</table>\n")
		}

		// Validation results for this level
		if len(ls.Checks) > 0 {
			b.WriteString("<p><strong>Validation results:</strong></p>\n<ul>\n")
			for _, cr := range ls.Checks {
				writeCheckItem(&b, cr)
			}
			b.WriteString("</ul>\n")
		}

		b.WriteString("</div>\n")
	}

	// ── 4. Global validations ──────────────────────────────────────────────────
	if len(d.GlobalChecks) > 0 {
		b.WriteString("<h2>4. Global Validations</h2>\n<div class=\"section\"><ul>\n")
		for _, cr := range d.GlobalChecks {
			writeCheckItem(&b, cr)
		}
		b.WriteString("</ul>\n</div>\n")
	}

	b.WriteString("</body>\n</html>\n")
	return b.String()
}

func writeCheckItem(b *strings.Builder, cr CheckResult) {
	if cr.OK {
		fmt.Fprintf(b, "<li><span class=\"pass\">&#10003; PASS</span> %s</li>\n", he(cr.Name))
	} else {
		fmt.Fprintf(b, "<li><span class=\"fail\">&#10007; FAIL</span> %s\n", he(cr.Name))
		for _, r := range cr.Reasons {
			fmt.Fprintf(b, "<div class=\"reason\">&rarr; %s</div>\n", he(r))
		}
		b.WriteString("</li>\n")
	}
}

// he HTML-escapes a string for safe embedding.
func he(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&#34;")
	return s
}
