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
	"strings"
)

// Figure geometry. One place, so the light and dark renderings cannot drift
// apart and every chart on the page lines up with the others.
const (
	chartW = 900
	chartH = 480
	padL   = 76
	padR   = 30
	padT   = 92
	padB   = 76
	plotW  = chartW - padL - padR
	plotH  = chartH - padT - padB

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
	Title      string
	Subtitle   string
	YAxis      string
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
}

// lineSpec is a line chart over a numeric X axis.
type lineSpec struct {
	Title     string
	Subtitle  string
	XAxis     string
	YAxis     string
	Series    []lineSeries
	Format    func(float64) string
	Threshold *thresholdLine
}

// canvas accumulates SVG elements.
type canvas struct {
	sb strings.Builder
	p  palette
}

// newCanvas opens a figure with its ground painted: a transparent SVG would
// borrow whatever ground it lands on, and these are read on two of them.
func newCanvas(p palette, title, subtitle string) *canvas {
	c := &canvas{p: p}
	fmt.Fprintf(&c.sb,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" role="img" aria-label="%s">`+"\n",
		chartW, chartH, chartW, chartH, xmlEscape(title+". "+subtitle))
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

// legend paints one swatch and label per series, left to right under the
// title.
func (c *canvas) legend(labels []string) {
	x := float64(padL)
	y := 70.0
	for i, label := range labels {
		color := c.p.Series[i%len(c.p.Series)]
		fmt.Fprintf(&c.sb, `<rect x="%.1f" y="%.1f" width="11" height="11" rx="2" fill="%s"/>`+"\n", x, y-9, color)
		fmt.Fprintf(&c.sb, `<text x="%.1f" y="%.1f" %s font-size="11.5" fill="%s">%s</text>`+"\n",
			x+16, y, fontStack, c.p.Text, xmlEscape(label))
		x += 27 + textWidth(label, 11.5)
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
			padL+plotW/2.0, chartH-14, fontStack, c.p.Muted, xmlEscape(xAxis))
	}
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

	c := newCanvas(p, spec.Title, spec.Subtitle)
	labels := make([]string, 0, len(spec.Series))
	for _, series := range spec.Series {
		labels = append(labels, series.Label)
	}
	c.legend(labels)
	for _, tick := range ticks {
		c.gridLine(sc.pos(tick), format(tick))
	}
	c.axisLabels(spec.YAxis, "")

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

// renderLines draws a line chart over a numeric X axis.
func renderLines(p palette, spec lineSpec) string {
	format := spec.Format
	if format == nil {
		format = func(v float64) string { return fmt.Sprintf("%.0f", v) }
	}

	maxY, maxX, minX := 0.0, 0.0, math.MaxFloat64
	for _, series := range spec.Series {
		for i, y := range series.Y {
			maxY = math.Max(maxY, y)
			maxX = math.Max(maxX, series.X[i])
			minX = math.Min(minX, series.X[i])
		}
	}
	if spec.Threshold != nil {
		maxY = math.Max(maxY, spec.Threshold.Value)
	}
	if minX == math.MaxFloat64 {
		minX = 0
	}
	sc, ticks := linearScale(maxY * 1.1)
	xPos := func(v float64) float64 {
		if maxX == minX {
			return float64(padL) + float64(plotW)/2
		}
		return float64(padL) + (v-minX)/(maxX-minX)*float64(plotW)
	}

	c := newCanvas(p, spec.Title, spec.Subtitle)
	labels := make([]string, 0, len(spec.Series))
	for _, series := range spec.Series {
		labels = append(labels, series.Label)
	}
	c.legend(labels)
	for _, tick := range ticks {
		c.gridLine(sc.pos(tick), format(tick))
	}
	c.axisLabels(spec.YAxis, spec.XAxis)

	for x := minX; x <= maxX; x++ {
		fmt.Fprintf(&c.sb, `<text x="%.1f" y="%d" %s font-size="11" text-anchor="middle" fill="%s">%.0f</text>`+"\n",
			xPos(x), padT+plotH+20, fontStack, p.Muted, x)
	}

	for seriesIndex, series := range spec.Series {
		color := p.Series[seriesIndex%len(p.Series)]
		var path strings.Builder
		for i := range series.X {
			command := 'L'
			if i == 0 {
				command = 'M'
			}
			fmt.Fprintf(&path, "%c%.1f,%.1f", command, xPos(series.X[i]), sc.pos(series.Y[i]))
		}
		fmt.Fprintf(&c.sb, `<path d="%s" fill="none" stroke="%s" stroke-width="2.5" stroke-linejoin="round"/>`+"\n",
			path.String(), color)
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
