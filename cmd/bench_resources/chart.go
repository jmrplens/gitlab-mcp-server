// chart.go draws the figures, in Go, as SVG, with no browser and no
// JavaScript anywhere in the pipeline.
//
// Hand-drawn geometry rather than a plotting library for the reasons the
// sibling project's perfmon charts give: the same record must always produce
// byte-identical SVGs, so a committed chart can be verified rather than
// trusted; the palette has to be the site's own; and a log axis, a threshold
// rule and value labels on the bars are all things a general-purpose library
// makes harder than two hundred lines of arithmetic does.

package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// Figure geometry. One place, so the light and dark renderings cannot drift
// apart and every chart on the page lines up with the others.
// The canvas is twenty pixels taller than the plot needs so that every figure
// can carry the host and the build it was measured on along its bottom edge. A
// chart travels: it is embedded, screenshotted and quoted away from the page
// that states the machine, and a resident-set curve with no machine attached
// is a number nobody can act on.
const (
	chartW = 900
	chartH = 500
	padL   = 76
	padR   = 30
	padT   = 92
	padB   = 96
	plotW  = chartW - padL - padR
	plotH  = chartH - padT - padB

	// xAxisTitleY and provenanceY are measured from the plot rather than from
	// the bottom edge, so growing the canvas moves the footer and leaves the
	// axis title where it sits under its tick labels.
	xAxisTitleY = padT + plotH + 42
	provenanceY = chartH - 12

	fontStack = `font-family="ui-sans-serif,system-ui,-apple-system,Segoe UI,sans-serif"`
)

// thresholdLine is a labeled horizontal rule, used for the memory limits an
// operator is choosing between.
type thresholdLine struct {
	Value float64
	Label string
}

// barSeries is one bar per category.
type barSeries struct {
	Label  string
	Values []float64
	// High, when set, is drawn behind the value as a faint extension. The
	// latency figure uses it for p99: a chart of medians alone hides exactly
	// the tail an operator is sizing timeouts for.
	High []float64
}

// barSpec is a grouped bar chart.
type barSpec struct {
	Title    string
	Subtitle string
	YAxis    string
	// XAxis names what the categories are. A bar chart's categories carry
	// their own labels, but "dynamic, meta, individual" only reads as a tool
	// surface to somebody who already knows, and a figure has to be legible
	// away from the page that says so.
	XAxis string
	// Provenance is the machine and build the figure was measured on, drawn
	// along the bottom edge so the chart carries it wherever it is quoted.
	Provenance string
	Categories []string
	Series     []barSeries
	// Log selects a base-10 vertical axis, which the startup and latency
	// figures need: a warm ping and a cold tools/list are three orders of
	// magnitude apart, and on a linear axis one of them is invisible.
	Log       bool
	Format    func(float64) string
	Threshold *thresholdLine
}

// lineSeries is one polyline.
type lineSeries struct {
	Label string
	X     []float64
	Y     []float64
	// Dashed draws the line dashed, for a companion of another series: the
	// latency figure pairs a solid p50 with a dashed p99.
	Dashed bool
	// Group names the series this one shares a color with. Empty colors the
	// series by its own index, which is what every single-line-per-surface
	// figure wants; the paired figure groups by surface so a reader can tell
	// a surface's tail from its median without a second legend.
	Group string
}

// lineMarker is a labeled vertical rule at one X, used for the credential
// count a series stopped at.
type lineMarker struct {
	X     float64
	Label string
}

// lineSpec is a line chart over a numeric X axis.
type lineSpec struct {
	Title    string
	Subtitle string
	XAxis    string
	YAxis    string
	// Provenance is the machine and build the figure was measured on, drawn
	// along the bottom edge so the chart carries it wherever it is quoted.
	Provenance string
	Series     []lineSeries
	Format     func(float64) string
	Threshold  *thresholdLine
	// LogX lays the X axis out in decades and labels it at the series' own
	// points rather than at every integer, which is what a credential count
	// running from one to a thousand needs: on a linear axis the first nine
	// steps would sit inside the first pixel.
	LogX bool
	// LogY selects a base-10 vertical axis, as the bar charts have it.
	LogY bool
	// Markers are drawn as dashed vertical rules in the threshold color.
	Markers []lineMarker
}

// canvas accumulates SVG elements.
type canvas struct {
	sb strings.Builder
	p  palette
}

// newCanvas opens a figure with its ground painted: a transparent SVG would
// borrow whatever ground it lands on, and these are read on two of them.
//
// The provenance joins the accessible name as well as the bottom edge, because
// a reader who cannot see the footer is exactly the reader who cannot fall back
// to the surrounding prose either.
func newCanvas(p palette, title, subtitle, provenance string) *canvas {
	c := &canvas{p: p}
	fmt.Fprintf(&c.sb,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" role="img" aria-label="%s">`+"\n",
		chartW, chartH, chartW, chartH, xmlEscape(joinNonEmpty(". ", title, subtitle, provenance)))
	fmt.Fprintf(&c.sb, "<title>%s</title>\n", xmlEscape(title))
	fmt.Fprintf(&c.sb, `<rect width="%d" height="%d" fill="%s"/>`+"\n", chartW, chartH, p.Page)
	fmt.Fprintf(&c.sb, `<text x="%d" y="30" %s font-size="17" font-weight="600" fill="%s">%s</text>`+"\n",
		padL-46, fontStack, p.Text, xmlEscape(title))
	if subtitle != "" {
		fmt.Fprintf(&c.sb, `<text x="%d" y="50" %s font-size="12" fill="%s">%s</text>`+"\n",
			padL-46, fontStack, p.Muted, xmlEscape(subtitle))
	}
	fmt.Fprintf(&c.sb, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`+"\n",
		padL, padT, plotW, plotH, p.Plot)
	return c
}

// close finishes the figure.
func (c *canvas) close() string {
	c.sb.WriteString("</svg>\n")
	return c.sb.String()
}

// legendEntry is one item of a legend: a label, the color it stands for,
// and whether the series it names is drawn dashed.
type legendEntry struct {
	label  string
	color  string
	dashed bool
}

// legend paints one swatch and label per series, left to right under the
// title, colored by position.
func (c *canvas) legend(labels []string) {
	entries := make([]legendEntry, 0, len(labels))
	for i, label := range labels {
		entries = append(entries, legendEntry{label: label, color: c.p.Series[i%len(c.p.Series)]})
	}
	c.legendEntries(entries)
}

// legendEntries paints a legend whose colors and line styles are given: a
// filled swatch for a solid series, a dashed stroke for a dashed one.
func (c *canvas) legendEntries(entries []legendEntry) {
	x := float64(padL)
	y := 70.0
	for _, entry := range entries {
		if entry.dashed {
			fmt.Fprintf(&c.sb, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="2.5" stroke-dasharray="4 3"/>`+"\n",
				x, y-3.5, x+11, y-3.5, entry.color)
		} else {
			fmt.Fprintf(&c.sb, `<rect x="%.1f" y="%.1f" width="11" height="11" rx="2" fill="%s"/>`+"\n", x, y-9, entry.color)
		}
		fmt.Fprintf(&c.sb, `<text x="%.1f" y="%.1f" %s font-size="11.5" fill="%s">%s</text>`+"\n",
			x+16, y, fontStack, c.p.Text, xmlEscape(entry.label))
		x += 27 + textWidth(entry.label, 11.5)
	}
}

// axisLabels writes the vertical axis title and, when given, the horizontal
// one.
func (c *canvas) axisLabels(yAxis, xAxis string) {
	if yAxis != "" {
		mid := padT + plotH/2.0
		fmt.Fprintf(&c.sb,
			`<text x="18" y="%.1f" %s font-size="12" fill="%s" text-anchor="middle" transform="rotate(-90 18 %.1f)">%s</text>`+"\n",
			mid, fontStack, c.p.Muted, mid, xmlEscape(yAxis))
	}
	if xAxis != "" {
		fmt.Fprintf(&c.sb, `<text x="%.1f" y="%d" %s font-size="12" text-anchor="middle" fill="%s">%s</text>`+"\n",
			padL+plotW/2.0, xAxisTitleY, fontStack, c.p.Muted, xmlEscape(xAxis))
	}
}

// provenance writes the machine and build along the bottom edge, so a figure
// lifted out of its page still says what it is a measurement of.
func (c *canvas) provenance(text string) {
	if text == "" {
		return
	}
	fmt.Fprintf(&c.sb, `<text x="%d" y="%d" %s font-size="10" fill="%s">%s</text>`+"\n",
		padL-46, provenanceY, fontStack, c.p.Muted, xmlEscape(text))
}

// gridLine draws one horizontal rule with its value label.
func (c *canvas) gridLine(y float64, label string) {
	fmt.Fprintf(&c.sb, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="%s" stroke-width="1"/>`+"\n",
		padL, y, padL+plotW, y, c.p.Grid)
	fmt.Fprintf(&c.sb, `<text x="%d" y="%.1f" %s font-size="11" text-anchor="end" fill="%s">%s</text>`+"\n",
		padL-8, y+4, fontStack, c.p.Muted, xmlEscape(label))
}

// threshold draws a labeled dashed rule in the threshold color.
func (c *canvas) threshold(y float64, label string) {
	fmt.Fprintf(&c.sb, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="%s" stroke-width="1.5" stroke-dasharray="6 4"/>`+"\n",
		padL, y, padL+plotW, y, c.p.Threshold)
	fmt.Fprintf(&c.sb, `<text x="%d" y="%.1f" %s font-size="11" text-anchor="end" fill="%s">%s</text>`+"\n",
		padL+plotW-6, y-6, fontStack, c.p.Threshold, xmlEscape(label))
}

// scale maps a value to a vertical coordinate.
type scale struct {
	lo, hi float64
	log    bool
}

// pos returns the y coordinate for a value.
func (s scale) pos(v float64) float64 {
	if s.log {
		if v <= 0 {
			v = s.lo
		}
		ratio := (math.Log10(v) - math.Log10(s.lo)) / (math.Log10(s.hi) - math.Log10(s.lo))
		return padT + (1-clamp01(ratio))*plotH
	}
	ratio := (v - s.lo) / (s.hi - s.lo)
	return padT + (1-clamp01(ratio))*plotH
}

// clamp01 keeps a ratio inside the plot even when a value sits outside the
// axis, which a threshold line legitimately can.
func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

// linearScale picks a rounded axis covering zero to max.
func linearScale(maxValue float64) (axis scale, ticks []float64) {
	if maxValue <= 0 {
		maxValue = 1
	}
	step := niceStep(maxValue / 5)
	hi := math.Ceil(maxValue/step) * step
	for v := 0.0; v <= hi+step/2; v += step {
		ticks = append(ticks, v)
	}
	return scale{lo: 0, hi: hi}, ticks
}

// logScale picks whole decades covering the data, with a tick per decade.
//
// The inputs are clamped to values a decade walk can terminate on: a figure
// drawn from a partial record can arrive here with nothing positive in it, and
// an infinity as the ceiling would loop until the run was killed.
func logScale(minValue, maxValue float64) (axis scale, ticks []float64) {
	const maxDecades = 12
	if minValue <= 0 || math.IsInf(minValue, 0) || math.IsNaN(minValue) {
		minValue = 0.1
	}
	if maxValue <= minValue || math.IsInf(maxValue, 0) || math.IsNaN(maxValue) {
		maxValue = minValue * 10
	}
	// The walk is over exponents rather than over values. Multiplying a value
	// by ten can fail to advance it: a subnormal minValue underflows
	// math.Pow to zero, and zero times ten is zero, so the loop that clamping
	// the inputs was supposed to bound would still append ticks until the
	// process died. Counting decades cannot fail to advance.
	//
	// The exponents are clamped as well as the span, because float64 holds
	// 10^308 and overflows at 10^309: a maximum near the largest float would
	// otherwise put the ceiling at infinity and take every tick with it.
	const maxExp = 308
	loExp := min(max(math.Floor(math.Log10(minValue)), -maxExp), maxExp-1)
	hiExp := min(max(math.Ceil(math.Log10(maxValue)), loExp+1), maxExp)
	if hiExp > loExp+maxDecades {
		hiExp = loExp + maxDecades
	}
	for exp := loExp; exp <= hiExp; exp++ {
		ticks = append(ticks, math.Pow(10, exp))
	}
	return scale{lo: math.Pow(10, loExp), hi: math.Pow(10, hiExp), log: true}, ticks
}

// niceStep rounds a step up to 1, 2 or 5 times a power of ten, so axis labels
// are numbers a reader can hold in their head.
func niceStep(raw float64) float64 {
	if raw <= 0 {
		return 1
	}
	magnitude := math.Pow(10, math.Floor(math.Log10(raw)))
	switch normalized := raw / magnitude; {
	case normalized <= 1:
		return magnitude
	case normalized <= 2:
		return 2 * magnitude
	case normalized <= 5:
		return 5 * magnitude
	default:
		return 10 * magnitude
	}
}

// renderBars draws a grouped bar chart.
func renderBars(p palette, spec barSpec) string {
	format := spec.Format
	if format == nil {
		format = func(v float64) string { return fmt.Sprintf("%.0f", v) }
	}

	maxValue, minValue := 0.0, math.MaxFloat64
	for _, series := range spec.Series {
		for _, v := range append(append([]float64{}, series.Values...), series.High...) {
			maxValue = math.Max(maxValue, v)
			if v > 0 {
				minValue = math.Min(minValue, v)
			}
		}
	}
	if spec.Threshold != nil {
		maxValue = math.Max(maxValue, spec.Threshold.Value)
	}
	// A partial record can leave a figure with nothing positive in it, which
	// is a legitimate state: a filtered run measures some surfaces and not
	// others. Both scales need a floor and a ceiling that exist, or the log
	// axis walks decades from 1e308 forever.
	if minValue == math.MaxFloat64 || minValue <= 0 {
		minValue = 1
	}
	if maxValue <= 0 {
		maxValue = 1
	}

	var sc scale
	var ticks []float64
	if spec.Log {
		sc, ticks = logScale(minValue, maxValue)
	} else {
		sc, ticks = linearScale(maxValue * 1.12)
	}

	c := newCanvas(p, spec.Title, spec.Subtitle, spec.Provenance)
	labels := make([]string, 0, len(spec.Series))
	for _, series := range spec.Series {
		labels = append(labels, series.Label)
	}
	c.legend(labels)
	for _, tick := range ticks {
		c.gridLine(sc.pos(tick), format(tick))
	}
	c.axisLabels(spec.YAxis, spec.XAxis)
	c.provenance(spec.Provenance)

	bandW := float64(plotW) / float64(len(spec.Categories))
	groupW := bandW * 0.78
	barW := groupW / float64(len(spec.Series))
	baseline := sc.pos(sc.lo)

	for categoryIndex, category := range spec.Categories {
		bandX := float64(padL) + float64(categoryIndex)*bandW
		groupX := bandX + (bandW-groupW)/2
		for seriesIndex, series := range spec.Series {
			if categoryIndex >= len(series.Values) {
				continue
			}
			c.bar(barGeometry{
				x:        groupX + float64(seriesIndex)*barW,
				width:    barW,
				baseline: baseline,
				value:    sc.pos(series.Values[categoryIndex]),
				high:     highPosition(sc, series, categoryIndex),
				color:    p.Series[seriesIndex%len(p.Series)],
				label:    format(series.Values[categoryIndex]),
			})
		}
		fmt.Fprintf(&c.sb, `<text x="%.1f" y="%d" %s font-size="12.5" text-anchor="middle" fill="%s">%s</text>`+"\n",
			bandX+bandW/2, padT+plotH+20, fontStack, p.Text, xmlEscape(category))
	}

	if spec.Threshold != nil {
		c.threshold(sc.pos(spec.Threshold.Value), spec.Threshold.Label)
	}
	return c.close()
}

// barGeometry is one bar, already reduced to coordinates.
type barGeometry struct {
	x, width, baseline, value float64
	// high is the top of the faint extension, or zero when there is none.
	high  float64
	color string
	label string
}

// highPosition returns the coordinate of a series' high value for one
// category, or zero when that series carries none or it does not exceed the
// value itself.
func highPosition(sc scale, series barSeries, categoryIndex int) float64 {
	if categoryIndex >= len(series.High) {
		return 0
	}
	if series.High[categoryIndex] <= series.Values[categoryIndex] {
		return 0
	}
	return sc.pos(series.High[categoryIndex])
}

// bar draws one bar, its optional faint extension, and its value label.
func (c *canvas) bar(g barGeometry) {
	labelY := g.value
	if g.high > 0 {
		fmt.Fprintf(&c.sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" fill-opacity="0.3"/>`+"\n",
			g.x+1, g.high, g.width-2, math.Max(1, g.baseline-g.high), g.color)
		// The label goes above the tallest thing drawn here, or it would sit
		// inside the faint p99 extension and read as its value rather than the
		// p50's.
		labelY = g.high
	}
	fmt.Fprintf(&c.sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`+"\n",
		g.x+1, g.value, g.width-2, math.Max(1, g.baseline-g.value), g.color)
	fmt.Fprintf(&c.sb, `<text x="%.1f" y="%.1f" %s font-size="10" text-anchor="middle" fill="%s">%s</text>`+"\n",
		g.x+g.width/2, labelY-4, fontStack, c.p.Text, xmlEscape(g.label))
}

// lineExtent is the range a line chart's data spans, with the smallest
// positive Y kept for a log axis.
type lineExtent struct {
	minX, maxX, minY, maxY float64
}

// extentOf measures a spec's series, folding the threshold into the Y range
// so the rule is always inside the plot.
func extentOf(spec lineSpec) lineExtent {
	e := lineExtent{minX: math.MaxFloat64, minY: math.MaxFloat64}
	for _, series := range spec.Series {
		for i, y := range series.Y {
			e.maxY = math.Max(e.maxY, y)
			if y > 0 {
				e.minY = math.Min(e.minY, y)
			}
			e.maxX = math.Max(e.maxX, series.X[i])
			e.minX = math.Min(e.minX, series.X[i])
		}
	}
	if spec.Threshold != nil {
		e.maxY = math.Max(e.maxY, spec.Threshold.Value)
	}
	if e.minX == math.MaxFloat64 {
		e.minX = 0
	}
	if e.minY == math.MaxFloat64 {
		e.minY = 1
	}
	return e
}

// xMapper returns the horizontal coordinate function for a spec: linear
// between the data's ends, or by decades when the axis is logarithmic. One
// point alone sits in the middle either way.
func xMapper(spec lineSpec, e lineExtent) func(float64) float64 {
	return func(v float64) float64 {
		if e.maxX == e.minX {
			return float64(padL) + float64(plotW)/2
		}
		if spec.LogX {
			lo, hi := math.Log10(math.Max(e.minX, 1)), math.Log10(math.Max(e.maxX, 1))
			return float64(padL) + clamp01((math.Log10(math.Max(v, 1))-lo)/(hi-lo))*float64(plotW)
		}
		return float64(padL) + (v-e.minX)/(e.maxX-e.minX)*float64(plotW)
	}
}

// xTicks lists where the horizontal axis is labeled: every integer across
// the range on a linear axis, and the series' own points on a logarithmic
// one, deduplicated and in order.
func xTicks(spec lineSpec, e lineExtent) []float64 {
	if !spec.LogX {
		var ticks []float64
		for x := e.minX; x <= e.maxX; x++ {
			ticks = append(ticks, x)
		}
		return ticks
	}
	seen := map[float64]bool{}
	var ticks []float64
	for _, series := range spec.Series {
		for _, x := range series.X {
			if !seen[x] {
				seen[x] = true
				ticks = append(ticks, x)
			}
		}
	}
	slices.Sort(ticks)
	return ticks
}

// seriesColor picks a series' color: its group's when it names one, so a
// dashed companion shares its partner's hue, and its own position otherwise.
func seriesColor(p palette, spec lineSpec, index int) string {
	series := spec.Series[index]
	if series.Group == "" {
		return p.Series[index%len(p.Series)]
	}
	var groups []string
	for _, other := range spec.Series {
		if other.Group != "" && !slices.Contains(groups, other.Group) {
			groups = append(groups, other.Group)
		}
	}
	return p.Series[slices.Index(groups, series.Group)%len(p.Series)]
}

// renderLines draws a line chart over a numeric X axis.
func renderLines(p palette, spec lineSpec) string {
	format := spec.Format
	if format == nil {
		format = func(v float64) string { return fmt.Sprintf("%.0f", v) }
	}

	e := extentOf(spec)
	var sc scale
	var ticks []float64
	if spec.LogY {
		sc, ticks = logScale(e.minY, e.maxY)
	} else {
		sc, ticks = linearScale(e.maxY * 1.1)
	}
	xPos := xMapper(spec, e)

	c := newCanvas(p, spec.Title, spec.Subtitle, spec.Provenance)
	entries := make([]legendEntry, 0, len(spec.Series))
	for i, series := range spec.Series {
		entries = append(entries, legendEntry{label: series.Label, color: seriesColor(p, spec, i), dashed: series.Dashed})
	}
	c.legendEntries(entries)
	for _, tick := range ticks {
		c.gridLine(sc.pos(tick), format(tick))
	}
	c.axisLabels(spec.YAxis, spec.XAxis)
	c.provenance(spec.Provenance)

	for _, x := range xTicks(spec, e) {
		fmt.Fprintf(&c.sb, `<text x="%.1f" y="%d" %s font-size="11" text-anchor="middle" fill="%s">%.0f</text>`+"\n",
			xPos(x), padT+plotH+20, fontStack, p.Muted, x)
	}

	for seriesIndex, series := range spec.Series {
		color := seriesColor(p, spec, seriesIndex)
		var path strings.Builder
		for i := range series.X {
			command := 'L'
			if i == 0 {
				command = 'M'
			}
			fmt.Fprintf(&path, "%c%.1f,%.1f", command, xPos(series.X[i]), sc.pos(series.Y[i]))
		}
		dash := ""
		if series.Dashed {
			dash = ` stroke-dasharray="6 4"`
		}
		fmt.Fprintf(&c.sb, `<path d="%s" fill="none" stroke="%s" stroke-width="2.5" stroke-linejoin="round"%s/>`+"\n",
			path.String(), color, dash)
		for i := range series.X {
			fmt.Fprintf(&c.sb, `<circle cx="%.1f" cy="%.1f" r="3" fill="%s"/>`+"\n",
				xPos(series.X[i]), sc.pos(series.Y[i]), color)
		}
		// The last point carries the value, which is the one a reader takes
		// away: what the deployment weighs with every client attached.
		last := len(series.X) - 1
		if last >= 0 {
			fmt.Fprintf(&c.sb, `<text x="%.1f" y="%.1f" %s font-size="10.5" text-anchor="end" fill="%s">%s</text>`+"\n",
				xPos(series.X[last])-6, sc.pos(series.Y[last])-8, fontStack, color, xmlEscape(format(series.Y[last])))
		}
	}

	if spec.Threshold != nil {
		c.threshold(sc.pos(spec.Threshold.Value), spec.Threshold.Label)
	}
	for i, marker := range spec.Markers {
		x := xPos(marker.X)
		fmt.Fprintf(&c.sb, `<line x1="%.1f" y1="%d" x2="%.1f" y2="%d" stroke="%s" stroke-width="1.5" stroke-dasharray="3 3"/>`+"\n",
			x, padT, x, padT+plotH, p.Threshold)
		// Stacked down the rule, so two series that stopped at the same
		// count do not write over each other.
		fmt.Fprintf(&c.sb, `<text x="%.1f" y="%d" %s font-size="11" text-anchor="end" fill="%s">%s</text>`+"\n",
			x-4, padT+14+14*i, fontStack, p.Threshold, xmlEscape(marker.Label))
	}
	return c.close()
}

// textWidth estimates a label's width, which is all the legend layout needs:
// the figures are drawn with a system stack whose metrics are unknown here,
// and 0.55em per character is close enough for spacing swatches.
func textWidth(text string, fontSize float64) float64 {
	return float64(len([]rune(text))) * fontSize * 0.55
}

// xmlEscape makes text safe inside an SVG document.
func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
