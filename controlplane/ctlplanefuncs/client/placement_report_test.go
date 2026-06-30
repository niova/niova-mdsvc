package clictlplanefuncs

// placement_report_test.go
//
// Human-readable renderers for the eight required output tables. Every renderer
// is a pure function of the Chunk Placement Matrix (+ HierarchyModel), so the
// reports and the assertions can never disagree — they read the same source of
// truth. Integration tests Logf these and optionally dump the matrix to CSV.

import (
	"fmt"
	"sort"
	"strings"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

// shortID trims long UUID/device ids to their last 8 chars for compact tables.
func shortID(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[len(s)-8:]
}

func typeLabel(t cpLib.ChunkType) string { return cpLib.ChunkPrefix(t) }

// gibStr renders a byte count in GiB for readability.
func gibStr(b int64) string { return fmt.Sprintf("%.1fG", float64(b)/float64(gib)) }

// ── 1. Chunk Placement Matrix (source of truth) ─────────────────────────────
func reportChunkMatrix(rows []PlacementRow) string {
	var b strings.Builder
	fmt.Fprintln(&b, "### 1. Chunk Placement Matrix")
	fmt.Fprintf(&b, "%-10s %-8s %-5s %-4s %-6s %-4s %-8s %-8s %-8s %-10s %-8s\n",
		"PFS", "Vdev", "Chunk", "Type", "Size", "Seq", "PDU", "Rack", "HV", "Device", "NISD")
	sorted := append([]PlacementRow{}, rows...)
	sort.Slice(sorted, func(i, j int) bool {
		a, c := sorted[i], sorted[j]
		if a.Vdev != c.Vdev {
			return a.Vdev < c.Vdev
		}
		if a.ChunkID != c.ChunkID {
			return a.ChunkID < c.ChunkID
		}
		if a.Type != c.Type {
			return a.Type < c.Type
		}
		return a.Seq < c.Seq
	})
	for _, r := range sorted {
		fmt.Fprintf(&b, "%-10s %-8s %-5d %-4s %-6s %-4d %-8s %-8s %-8s %-10s %-8s\n",
			r.PFS, r.Vdev, r.ChunkID, typeLabel(r.Type), gibStr(r.ChunkSize), r.Seq,
			shortID(r.Keys[LevelPDU]), shortID(r.Keys[LevelRack]), shortID(r.Keys[LevelHV]),
			shortID(r.Keys[LevelDevice]), shortID(r.NisdID))
	}
	return b.String()
}

// ── 2. Vdev Placement Matrix ────────────────────────────────────────────────
func reportVdevMatrix(rows []PlacementRow) string {
	var b strings.Builder
	fmt.Fprintln(&b, "### 2. Vdev Placement Matrix")
	byVdev := groupRows(rows, func(r PlacementRow) string { return r.Vdev })
	for _, vdev := range sortedKeys(byVdev) {
		fmt.Fprintf(&b, "Vdev %s:\n", vdev)
		byChunk := make(map[uint32][]PlacementRow)
		for _, r := range byVdev[vdev] {
			byChunk[r.ChunkID] = append(byChunk[r.ChunkID], r)
		}
		chunks := make([]uint32, 0, len(byChunk))
		for c := range byChunk {
			chunks = append(chunks, c)
		}
		sort.Slice(chunks, func(i, j int) bool { return chunks[i] < chunks[j] })
		for _, c := range chunks {
			placements := byChunk[c]
			sort.Slice(placements, func(i, j int) bool {
				if placements[i].Type != placements[j].Type {
					return placements[i].Type < placements[j].Type
				}
				return placements[i].Seq < placements[j].Seq
			})
			parts := make([]string, 0, len(placements))
			for _, p := range placements {
				parts = append(parts, fmt.Sprintf("%s%d:%s/%s",
					typeLabel(p.Type), p.Seq, shortID(p.Keys[LevelDevice]), shortID(p.NisdID)))
			}
			fmt.Fprintf(&b, "  chunk %-3d -> %s\n", c, strings.Join(parts, "  "))
		}
	}
	return b.String()
}

// ── 3. Hierarchy Distribution Summary ───────────────────────────────────────
func reportHierarchyDistribution(rows []PlacementRow, model *HierarchyModel) string {
	var b strings.Builder
	fmt.Fprintln(&b, "### 3. Hierarchy Distribution Summary (data / parity / total)")
	for _, lvl := range AllLevels {
		fmt.Fprintf(&b, "-- %s --\n", lvl.Name())
		dataC := CountsByLevel(rows, lvl, ClassData, model)
		parC := CountsByLevel(rows, lvl, ClassParity, model)
		for _, e := range sortedKeys(dataC) {
			fmt.Fprintf(&b, "  %-12s  data=%-4d parity=%-4d total=%-4d\n",
				shortID(e), dataC[e], parC[e], dataC[e]+parC[e])
		}
	}
	return b.String()
}

// ── 4. Capacity Summary (Device + NISD) ─────────────────────────────────────
func reportCapacity(rows []PlacementRow, model *HierarchyModel) string {
	var b strings.Builder
	fmt.Fprintln(&b, "### 4. Capacity Summary")
	for _, lvl := range []Level{LevelDevice, LevelNISD} {
		fmt.Fprintf(&b, "-- %s --\n", lvl.Name())
		fmt.Fprintf(&b, "  %-12s %-8s %-8s %-8s %-8s %-7s %-9s %-9s\n",
			"ID", "Total", "Avail", "Alloc", "Free", "Util%", "ExpShare", "ActShare")
		alloc := allocByLevel(rows, lvl)
		capByE := model.CapacityByEntity(lvl)
		totalByE := totalCapByLevel(model, lvl)
		var totCap int64
		for _, v := range capByE {
			totCap += v
		}
		totalAlloc := int64(0)
		for _, v := range alloc {
			totalAlloc += v
		}
		for _, e := range sortedKeys(capByE) {
			avail := capByE[e]
			total := totalByE[e]
			a := alloc[e]
			free := avail - a
			util := 0.0
			if total > 0 {
				util = float64(a) / float64(total) * 100
			}
			expShare := 0.0
			if totCap > 0 {
				expShare = float64(avail) / float64(totCap)
			}
			actShare := 0.0
			if totalAlloc > 0 {
				actShare = float64(a) / float64(totalAlloc)
			}
			fmt.Fprintf(&b, "  %-12s %-8s %-8s %-8s %-8s %-7.2f %-9.3f %-9.3f\n",
				shortID(e), gibStr(total), gibStr(avail), gibStr(a), gibStr(free), util, expShare, actShare)
		}
	}
	return b.String()
}

// ── 5. Distribution Statistics ──────────────────────────────────────────────
func reportStatistics(rows []PlacementRow, model *HierarchyModel) string {
	var b strings.Builder
	fmt.Fprintln(&b, "### 5. Distribution Statistics")
	fmt.Fprintf(&b, "%-12s %-8s %6s %4s %4s %5s %8s %7s %6s %6s %7s %6s\n",
		"Level", "Class", "Mean", "Min", "Max", "Range", "Var", "StdDev", "CV", "Jain", "Max/Min", "Skew")
	for _, lvl := range AllLevels {
		for _, class := range []TypeClass{ClassData, ClassParity, ClassAll} {
			s := statsByLevel(rows, lvl, class, model)
			fmt.Fprintf(&b, "%-12s %-8s %6.2f %4.0f %4.0f %5.0f %8.2f %7.3f %6.3f %6.3f %7.3f %6.3f\n",
				lvl.Name(), class.Name(), s.Mean, s.Min, s.Max, s.Range, s.Variance,
				s.StdDev, s.CV, s.Jain, s.MaxMinRatio, s.Skew)
		}
	}
	return b.String()
}

// ── 6. Diversity Metrics (per vdev, vs theoretical max) ─────────────────────
func reportDiversity(rows []PlacementRow, model *HierarchyModel) string {
	var b strings.Builder
	fmt.Fprintln(&b, "### 6. Diversity Metrics (unique used / theoretical max)")
	byVdev := groupRows(rows, func(r PlacementRow) string { return r.Vdev })
	fmt.Fprintf(&b, "  %-10s %-10s %-10s %-12s %-10s %-10s\n",
		"Vdev", "PDUs", "Racks", "Hypervisors", "Devices", "NISDs")
	for _, vdev := range sortedKeys(byVdev) {
		vr := byVdev[vdev]
		cells := make([]string, 0, len(AllLevels))
		for _, lvl := range AllLevels {
			used := uniqueCount(vr, lvl)
			maxv := min2(len(vr), model.EntityCount(lvl))
			cells = append(cells, fmt.Sprintf("%d/%d", used, maxv))
		}
		fmt.Fprintf(&b, "  %-10s %-10s %-10s %-12s %-10s %-10s\n",
			vdev, cells[0], cells[1], cells[2], cells[3], cells[4])
	}
	return b.String()
}

// ── 7. Parity Distribution ──────────────────────────────────────────────────
func reportParityDistribution(rows []PlacementRow, model *HierarchyModel) string {
	var b strings.Builder
	fmt.Fprintln(&b, "### 7. Parity Distribution (parity placements per level)")
	for _, lvl := range AllLevels {
		counts := CountsByLevel(rows, lvl, ClassParity, model)
		s := ComputeStats(countValues(counts))
		if s.Sum == 0 {
			fmt.Fprintf(&b, "-- %s -- (no parity)\n", lvl.Name())
			continue
		}
		fmt.Fprintf(&b, "-- %s -- mean=%.2f max=%.0f min=%.0f Jain=%.3f\n",
			lvl.Name(), s.Mean, s.Max, s.Min, s.Jain)
		for _, e := range sortedKeys(counts) {
			if counts[e] == 0 {
				continue
			}
			fmt.Fprintf(&b, "  %-12s parity=%d\n", shortID(e), counts[e])
		}
	}
	return b.String()
}

// ── 8. Rebalance Metrics ────────────────────────────────────────────────────
func reportRebalance(ms MovementStats) string {
	var b strings.Builder
	fmt.Fprintln(&b, "### 8. Rebalance Metrics")
	fmt.Fprintf(&b, "  placements before : %d\n", ms.Before)
	fmt.Fprintf(&b, "  placements after  : %d\n", ms.After)
	fmt.Fprintf(&b, "  chunks moved      : %d (%.1f%%)\n", ms.Moved, ms.MovementPct*100)
	fmt.Fprintf(&b, "  bytes moved       : %s\n", gibStr(ms.BytesMoved))
	fmt.Fprintf(&b, "  moved onto new    : %d (%.1f%% of moved)\n", ms.MovedOntoNew, ms.OntoNewPct*100)
	return b.String()
}

// renderFullReport concatenates every section; integration tests Logf this.
func renderFullReport(rows []PlacementRow, model *HierarchyModel) string {
	sections := []string{
		reportChunkMatrix(rows),
		reportVdevMatrix(rows),
		reportHierarchyDistribution(rows, model),
		reportCapacity(rows, model),
		reportStatistics(rows, model),
		reportDiversity(rows, model),
		reportParityDistribution(rows, model),
	}
	return strings.Join(sections, "\n")
}

// ── small shared helpers ────────────────────────────────────────────────────

func groupRows(rows []PlacementRow, keyFn func(PlacementRow) string) map[string][]PlacementRow {
	out := make(map[string][]PlacementRow)
	for _, r := range rows {
		k := keyFn(r)
		out[k] = append(out[k], r)
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func uniqueCount(rows []PlacementRow, lvl Level) int {
	seen := make(map[string]struct{})
	for _, r := range rows {
		seen[r.Keys[lvl]] = struct{}{}
	}
	return len(seen)
}

// allocByLevel sums allocated bytes per entity at lvl.
func allocByLevel(rows []PlacementRow, lvl Level) map[string]int64 {
	out := make(map[string]int64)
	for _, r := range rows {
		out[r.Keys[lvl]] += r.ChunkSize
	}
	return out
}

// totalCapByLevel sums NISD TotalSize per entity at lvl.
func totalCapByLevel(model *HierarchyModel, lvl Level) map[string]int64 {
	out := make(map[string]int64)
	for _, n := range model.nisds {
		out[n.Keys[lvl]] += n.TotalSize
	}
	return out
}
