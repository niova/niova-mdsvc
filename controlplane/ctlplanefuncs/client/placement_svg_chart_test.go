package clictlplanefuncs

// placement_svg_chart_test.go
//
// Minimal, dependency-free SVG line-chart renderer used by the comparison
// report's per-entity graphs. Plain inline SVG (no JS framework, no CDN
// fetch) so the report keeps working fully offline and prints cleanly to
// PDF — consistent with the rest of this package's report renderers. Hover
// tooltips come from native browser/PDF-viewer <title> support; zooming is
// native SVG/browser zoom, so no script is needed for either.

import (
	"fmt"
	"math"
	"strings"
)

// chartSeries is one line: a named, colored, axis-scoped sequence of values
// aligned 1:1 with the chart's shared x-axis labels.
type chartSeries struct {
	name   string
	color  string
	axis   string // "left" or "right"
	values []float64
}

// chartColor is the fixed name -> color assignment shared by every chart in
// the report, so e.g. "Data" is the same blue in every scenario's chart —
// required for the eye to compare charts across scenarios.
var chartColor = map[string]string{
	"Data":                "#2980b9",
	"Parity":              "#e67e22",
	"Total":               "#27ae60",
	"Available Size (GB)": "#8e44ad",
	"Used Size (GB)":      "#c0392b",
}

// renderLineChart draws one SVG line chart: a polyline + circle markers per
// series, gridlines with tick labels on the left axis (and right axis when
// any series uses it), a legend, and a native hover tooltip on every marker.
// labels is the shared x-axis (one per data point); every series must supply
// len(values) == len(labels). rightLabel == "" means no series uses the right
// axis and it is not drawn.
func renderLineChart(labels []string, series []chartSeries, leftLabel, rightLabel string) string {
	if len(labels) == 0 || len(series) == 0 {
		return ""
	}
	hasRight := rightLabel != ""

	const (
		marginTop    = 46 // legend row
		marginBottom = 74 // rotated x-axis labels
		marginLeft   = 56
	)
	marginRight := 20
	if hasRight {
		marginRight = 60
	}

	plotW := 40 * len(labels)
	if plotW < 480 {
		plotW = 480
	}
	const plotH = 260

	totalW := marginLeft + plotW + marginRight
	totalH := marginTop + plotH + marginBottom

	var maxLeft, maxRight float64
	for _, s := range series {
		for _, v := range s.values {
			if s.axis == "right" {
				if v > maxRight {
					maxRight = v
				}
			} else if v > maxLeft {
				maxLeft = v
			}
		}
	}
	isPct := strings.Contains(rightLabel, "%")
	switch {
	case isPct:
		maxRight = 100 // percentages always scale 0-100 so utilization is visually comparable across charts
	case maxRight == 0:
		maxRight = 1
	default:
		maxRight = niceCeil(maxRight)
	}
	if maxLeft == 0 {
		maxLeft = 1
	} else {
		maxLeft = niceCeil(maxLeft)
	}

	xAt := func(i int) float64 {
		if len(labels) == 1 {
			return float64(marginLeft) + float64(plotW)/2
		}
		return float64(marginLeft) + float64(i)*float64(plotW)/float64(len(labels)-1)
	}
	yAtLeft := func(v float64) float64 { return float64(marginTop) + float64(plotH) - (v/maxLeft)*float64(plotH) }
	yAtRight := func(v float64) float64 { return float64(marginTop) + float64(plotH) - (v/maxRight)*float64(plotH) }

	var b strings.Builder
	fmt.Fprintf(&b, `<div class="chart-wrap"><svg viewBox="0 0 %d %d" width="%d" height="%d" xmlns="http://www.w3.org/2000/svg" class="chart">`,
		totalW, totalH, totalW, totalH)

	// Plot background + horizontal gridlines/tick labels (5 bands).
	fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="#fafbfc" stroke="#ddd"/>`,
		marginLeft, marginTop, plotW, plotH)
	for i := 0; i <= 4; i++ {
		frac := float64(i) / 4
		y := float64(marginTop) + float64(plotH)*(1-frac)
		fmt.Fprintf(&b, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="#e8e8e8" stroke-width="1"/>`,
			marginLeft, y, marginLeft+plotW, y)
		fmt.Fprintf(&b, `<text x="%d" y="%.1f" font-size="10" fill="#555" text-anchor="end">%s</text>`,
			marginLeft-6, y+3, fmtTick(maxLeft*frac))
		if hasRight {
			fmt.Fprintf(&b, `<text x="%d" y="%.1f" font-size="10" fill="#555" text-anchor="start">%s</text>`,
				marginLeft+plotW+6, y+3, fmtTick(maxRight*frac))
		}
	}

	// Axis titles (rotated, flush to the left/right edge).
	fmt.Fprintf(&b, `<text x="14" y="%d" font-size="11" fill="#333" text-anchor="middle" transform="rotate(-90 14 %d)">%s</text>`,
		marginTop+plotH/2, marginTop+plotH/2, he(leftLabel))
	if hasRight {
		fmt.Fprintf(&b, `<text x="%d" y="%d" font-size="11" fill="#333" text-anchor="middle" transform="rotate(-90 %d %d)">%s</text>`,
			totalW-14, marginTop+plotH/2, totalW-14, marginTop+plotH/2, he(rightLabel))
	}

	// X-axis labels, rotated so entity ids don't overlap.
	for i, lab := range labels {
		x, y := xAt(i), float64(marginTop+plotH+14)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="10" fill="#444" text-anchor="end" transform="rotate(-40 %.1f %.1f)">%s</text>`,
			x, y, x, y, he(lab))
	}

	// Series: one polyline + one marker-with-tooltip per point.
	for _, s := range series {
		yAt := yAtLeft
		if s.axis == "right" {
			yAt = yAtRight
		}
		var pts strings.Builder
		for i, v := range s.values {
			if i > 0 {
				pts.WriteString(" ")
			}
			fmt.Fprintf(&pts, "%.1f,%.1f", xAt(i), yAt(v))
		}
		fmt.Fprintf(&b, `<polyline points="%s" fill="none" stroke="%s" stroke-width="2.5"/>`, pts.String(), s.color)
		for i, v := range s.values {
			cx, cy := xAt(i), yAt(v)
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="3.5" fill="%s"><title>%s: %s = %s</title></circle>`,
				cx, cy, s.color, he(labels[i]), he(s.name), fmtTick(v))
		}
	}

	// Legend: one row above the plot, left-aligned under the plot's left edge.
	lx, ly := marginLeft, 20
	for _, s := range series {
		fmt.Fprintf(&b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-width="3"/>`, lx, ly, lx+18, ly, s.color)
		fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="3.5" fill="%s"/>`, lx+9, ly, s.color)
		fmt.Fprintf(&b, `<text x="%d" y="%d" font-size="11" fill="#333">%s</text>`, lx+24, ly+4, he(s.name))
		lx += 24 + len(s.name)*7 + 20
	}

	b.WriteString(`</svg></div>`)
	return b.String()
}

// niceCeil rounds v up to a "nice" 1/2/5×10^n number so axis gridlines land
// on readable values instead of arbitrary fractions.
func niceCeil(v float64) float64 {
	if v <= 0 {
		return 1
	}
	exp := math.Floor(math.Log10(v))
	base := math.Pow(10, exp)
	f := v / base
	switch {
	case f <= 1:
		f = 1
	case f <= 2:
		f = 2
	case f <= 5:
		f = 5
	default:
		f = 10
	}
	return f * base
}

// fmtTick formats an axis tick / tooltip value: whole numbers print without a
// decimal point, everything else to one decimal place.
func fmtTick(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}
