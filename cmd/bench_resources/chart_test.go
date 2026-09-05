// chart_test.go covers the SVG renderer: the axis arithmetic, the properties a
// published figure has to have, and above all determinism, since a chart that
// changed between renderings could never be verified against the record it
// claims to draw.
package main

import (
	"math"
	"strings"
	"testing"
)

// testPalette is a palette with distinguishable values, so a test can tell
// which token painted what.
func testPalette() palette {
	return palette{
		Scheme: schemeDark,
		Page:   "#111111", Plot: "#222222", Grid: "#333333",
		Text: "#eeeeee", Muted: "#999999",
		Series:    []string{"#aa0000", "#00bb00", "#0000cc"},
		Threshold: "#ff0000",
	}
}

// TestRenderBars_Deterministic_ProducesIdenticalOutput verifies two renderings
// of one spec are byte-identical. The -check gate compares committed SVGs
// against a re-rendering, so any instability here would make it fail at random
// and be turned off.
func TestRenderBars_Deterministic_ProducesIdenticalOutput(t *testing.T) {
	spec := barSpec{
		Title: "Title", Subtitle: "Subtitle", YAxis: "MiB",
		Categories: []string{"dynamic", "meta", "individual"},
		Series: []barSeries{
			{Label: "one", Values: []float64{1, 2, 3}},
			{Label: "two", Values: []float64{10, 20, 30}, High: []float64{15, 25, 35}},
		},
		Threshold: &thresholdLine{Value: 25, Label: "limit"},
	}
	first := renderBars(testPalette(), spec)
	second := renderBars(testPalette(), spec)
	if first != second {
		t.Error("two renderings of the same spec differ, so the -check gate could never pass reliably")
	}
}

// TestRenderBars_PublishedFigure_HasTheRequiredParts verifies the figure
// carries what makes it usable: an accessible label, its own painted ground,
// every category and series label, the value labels a reader takes the numbers
// from, and the threshold rule.
func TestRenderBars_PublishedFigure_HasTheRequiredParts(t *testing.T) {
	p := testPalette()
	svg := renderBars(p, barSpec{
		Title: "Resident memory", Subtitle: "per surface", YAxis: "MiB",
		Categories: []string{"dynamic", "individual"},
		Series: []barSeries{
			{Label: "stdio", Values: []float64{240, 300}},
			{Label: "HTTP", Values: []float64{250, 600}},
		},
		Threshold: &thresholdLine{Value: 512, Label: "512 MiB"},
	})

	for _, want := range []string{
		`role="img"`,
		`aria-label="Resident memory. per surface"`,
		"<title>Resident memory</title>",
		`fill="#111111"`, // the page ground, so the figure does not borrow one
		"dynamic", "individual", "stdio", "HTTP",
		">240<", ">600<", // value labels
		"512 MiB",
		`stroke-dasharray`, // the threshold rule
		"</svg>",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(svg, want) {
				t.Errorf("the figure does not contain %q", want)
			}
		})
	}
}

// TestRenderBars_LogScale_KeepsSmallValuesVisible verifies a spec spanning
// three orders of magnitude renders on decade grid lines, which is what makes
// a two-millisecond bar visible beside a six-second one.
func TestRenderBars_LogScale_KeepsSmallValuesVisible(t *testing.T) {
	svg := renderBars(testPalette(), barSpec{
		Title: "Startup", YAxis: "ms", Log: true,
		Categories: []string{"dynamic"},
		Series: []barSeries{
			{Label: "warm", Values: []float64{3}},
			{Label: "cold", Values: []float64{3000}},
		},
		Format: msLabel,
	})
	for _, want := range []string{">1.0<", ">10<", ">100<", ">1000<"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(svg, want) {
				t.Errorf("the log axis has no %s grid label", want)
			}
		})
	}
}

// TestRenderLines_PublishedFigure_HasTheRequiredParts verifies the ramp figure
// draws one polyline per series, labels its axes, and marks the threshold.
func TestRenderLines_PublishedFigure_HasTheRequiredParts(t *testing.T) {
	svg := renderLines(testPalette(), lineSpec{
		Title: "Ramp", Subtitle: "per credential", XAxis: "credentials", YAxis: "MiB",
		Series: []lineSeries{
			{Label: "dynamic", X: []float64{1, 2, 3}, Y: []float64{240, 290, 340}},
			{Label: "individual", X: []float64{1, 2, 3}, Y: []float64{300, 380, 460}},
		},
		Threshold: &thresholdLine{Value: 512, Label: "512 MiB"},
	})

	if strings.Count(svg, "<path") != 2 {
		t.Errorf("the figure has %d paths, want one per series", strings.Count(svg, "<path"))
	}
	for _, want := range []string{"credentials", "MiB", "dynamic", "individual", "512 MiB", "<circle"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(svg, want) {
				t.Errorf("the figure does not contain %q", want)
			}
		})
	}
}

// TestScale_Positions_StayInsideThePlot verifies both scales map their whole
// range into the plot area and clamp anything outside it, so a threshold above
// the data cannot be drawn off the figure.
func TestScale_Positions_StayInsideThePlot(t *testing.T) {
	linear, _ := linearScale(100)
	logarithmic, _ := logScale(1, 1000)

	tests := []struct {
		name  string
		scale scale
		value float64
	}{
		{name: "linear bottom", scale: linear, value: 0},
		{name: "linear top", scale: linear, value: 100},
		{name: "linear beyond", scale: linear, value: 1e9},
		{name: "log bottom", scale: logarithmic, value: 1},
		{name: "log top", scale: logarithmic, value: 1000},
		{name: "log zero", scale: logarithmic, value: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			y := tc.scale.pos(tc.value)
			if y < padT-0.001 || y > padT+plotH+0.001 {
				t.Errorf("pos(%v) = %v, outside the plot area [%d, %d]", tc.value, y, padT, padT+plotH)
			}
		})
	}
}

// TestNiceStep_RoundsToOneTwoOrFive verifies axis steps are numbers a reader
// can hold in their head, at every magnitude.
func TestNiceStep_RoundsToOneTwoOrFive(t *testing.T) {
	tests := []struct {
		raw  float64
		want float64
	}{
		{raw: 0, want: 1},
		{raw: 0.7, want: 1},
		{raw: 1.5, want: 2},
		{raw: 3, want: 5},
		{raw: 7, want: 10},
		{raw: 23, want: 50},
		{raw: 100, want: 100},
		{raw: 120, want: 200},
	}
	for _, tc := range tests {
		t.Run(msLabel(tc.raw), func(t *testing.T) {
			if got := niceStep(tc.raw); math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("niceStep(%v) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

// TestLinearScale_CoversTheData verifies the axis top is never below the
// largest value, which would draw a bar taller than the plot.
func TestLinearScale_CoversTheData(t *testing.T) {
	for _, maxValue := range []float64{0, 1, 37, 512, 1234} {
		t.Run(msLabel(maxValue), func(t *testing.T) {
			sc, ticks := linearScale(maxValue)
			if sc.hi < maxValue {
				t.Errorf("axis top %v is below the data maximum %v", sc.hi, maxValue)
			}
			if len(ticks) < 2 {
				t.Errorf("axis has %d ticks, want at least two", len(ticks))
			}
		})
	}
}

// TestXMLEscape_MarkupInLabels_IsNeutralized verifies a label carrying markup
// characters cannot break the document, since labels come from a measurement
// record rather than from this file.
func TestXMLEscape_MarkupInLabels_IsNeutralized(t *testing.T) {
	got := xmlEscape(`a & b <c> "d"`)
	if strings.ContainsAny(got, "<>") || strings.Contains(got, `"`) {
		t.Errorf("xmlEscape left markup in %q", got)
	}
	if !strings.Contains(got, "&amp;") {
		t.Errorf("xmlEscape did not escape the ampersand: %q", got)
	}
}

// TestRenderBars_RaggedSeries_DrawsWhatItHas verifies a series shorter than
// the category list, and a High shorter than its Values, draw the bars they
// have and no others, and that a spec with nothing positive in it still
// renders on a usable axis rather than dividing by zero.
func TestRenderBars_RaggedSeries_DrawsWhatItHas(t *testing.T) {
	svg := renderBars(testPalette(), barSpec{
		Title: "Ragged", Categories: []string{"a", "b", "c"},
		Series: []barSeries{
			{Label: "short", Values: []float64{1, 2}},
			// One tail above its value, one below it, and none for the third
			// bar: only the first is drawn.
			{Label: "half tails", Values: []float64{3, 4, 5}, High: []float64{6, 1}},
		},
	})
	// The plot ground and the two legend swatches are positioned rects too.
	if got := strings.Count(svg, `<rect x=`) - 1 - 2; got != 6 {
		t.Errorf("drew %d bars, want the five values plus one tail", got)
	}

	empty := renderBars(testPalette(), barSpec{
		Title: "Nothing", Categories: []string{"a"},
		Series: []barSeries{{Label: "zeros", Values: []float64{0}}},
	})
	if !strings.HasPrefix(empty, "<svg") || !strings.Contains(empty, "</svg>") {
		t.Error("a spec with no positive value did not render a document")
	}
}

// TestRenderLines_LogAxes_DashedPairsAndMarkers verifies the additions the
// series figures need: the X axis laid out by decades and labeled at the
// series' own points, a log Y axis, a dashed companion in its partner's
// color, a marker at the count a series stopped at, and a spec with one
// point or none that still renders.
func TestRenderLines_LogAxes_DashedPairsAndMarkers(t *testing.T) {
	p := testPalette()
	svg := renderLines(p, lineSpec{
		Title: "Series", XAxis: "credentials", YAxis: "ms", LogX: true, LogY: true, Format: msLabel,
		Series: []lineSeries{
			{Label: "dynamic p50", Group: "dynamic", X: []float64{1, 10, 100, 1000}, Y: []float64{10, 12, 30, 900}},
			{Label: "dynamic p99", Group: "dynamic", X: []float64{1, 10, 100, 1000}, Y: []float64{20, 40, 90, 3000}, Dashed: true},
			{Label: "meta p50", Group: "meta", X: []float64{1, 10}, Y: []float64{5, 6}},
		},
		Markers: []lineMarker{{X: 100, Label: "dynamic: stopped at 100"}},
	})

	for _, want := range []string{
		`stroke-dasharray="6 4"`, // the dashed p99
		`stroke-dasharray="4 3"`, // its legend entry
		`stroke-dasharray="3 3"`, // the marker rule
		"dynamic: stopped at 100",
		">1000<", ">100<", ">10<", // x labels at the points, and decade grid labels
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(svg, want) {
				t.Errorf("the figure does not contain %q", want)
			}
		})
	}
	// Grouping: the two dynamic lines share the first color and meta takes
	// the second, so the third palette color is never used.
	if strings.Contains(svg, p.Series[2]) {
		t.Error("the third series color was used, so the pair did not share its group's color")
	}
	if strings.Count(svg, `stroke="`+p.Series[0]+`" stroke-width="2.5" stroke-linejoin`) != 2 {
		t.Error("the two dynamic lines do not share the first color")
	}

	t.Run("deterministic", func(t *testing.T) {
		spec := lineSpec{Title: "t", LogX: true, Series: []lineSeries{{Label: "a", X: []float64{1, 1000}, Y: []float64{1, 2}}}}
		first, second := renderLines(p, spec), renderLines(p, spec)
		if first != second {
			t.Error("two renderings of the same spec differ")
		}
	})
	t.Run("one point", func(t *testing.T) {
		one := renderLines(p, lineSpec{Title: "one", LogX: true, Series: []lineSeries{{Label: "a", X: []float64{5}, Y: []float64{7}}}})
		if !strings.Contains(one, ">5<") || !strings.Contains(one, "<circle") {
			t.Error("a single point was not drawn in the middle of the axis")
		}
	})
	t.Run("no series", func(t *testing.T) {
		if none := renderLines(p, lineSpec{Title: "none", LogY: true}); !strings.HasPrefix(none, "<svg") {
			t.Error("an empty spec did not render a document")
		}
	})
	t.Run("nothing positive on a log axis", func(t *testing.T) {
		zeros := renderLines(p, lineSpec{Title: "zero", LogY: true, Series: []lineSeries{{Label: "a", X: []float64{1, 2}, Y: []float64{0, 0}}}})
		if !strings.HasPrefix(zeros, "<svg") {
			t.Error("a log axis over zeros did not render")
		}
	})
}

// TestLogScale_ExtremeInputs_Terminates verifies the decade walk ends for
// every input a partial record can carry.
//
// The walk used to advance by multiplying a value by ten, which does not
// advance when the value is zero, and a subnormal minimum underflows the
// decade floor to exactly zero. The loop then appended a tick per iteration
// until the process died, so this test is written to hang rather than to fail
// if that arithmetic comes back. It pins the ordinary case too, since a bound
// that terminated by returning nothing would satisfy liveness alone.
func TestLogScale_ExtremeInputs_Terminates(t *testing.T) {
	cases := []struct {
		name             string
		minimum, maximum float64
	}{
		{"ordinary decades", 1, 1000},
		{"subnormal minimum", 5e-324, 1000},
		{"both subnormal", 5e-324, 5e-324},
		{"infinite maximum", 1, math.Inf(1)},
		{"infinite minimum", math.Inf(1), math.Inf(1)},
		{"not a number", math.NaN(), math.NaN()},
		{"maximum below minimum", 1000, 1},
		{"zero minimum", 0, 10},
		{"largest finite", math.MaxFloat64, math.MaxFloat64},
		{"whole float range", 5e-324, math.MaxFloat64},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			axis, ticks := logScale(tc.minimum, tc.maximum)
			if len(ticks) == 0 {
				t.Fatalf("logScale(%v, %v) produced no ticks", tc.minimum, tc.maximum)
			}
			// Thirteen is the twelve-decade bound plus its closing tick.
			if len(ticks) > 13 {
				t.Errorf("logScale(%v, %v) produced %d ticks, want at most 13", tc.minimum, tc.maximum, len(ticks))
			}
			if axis.hi <= axis.lo {
				t.Errorf("axis = [%v, %v], want a strictly increasing range", axis.lo, axis.hi)
			}
			for i, tick := range ticks {
				if math.IsNaN(tick) || math.IsInf(tick, 0) {
					t.Errorf("tick %d = %v, want a finite value", i, tick)
				}
			}
		})
	}
}
