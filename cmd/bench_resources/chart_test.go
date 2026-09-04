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
