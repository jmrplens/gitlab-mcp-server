// chart_test.go covers the SVG renderer: the axis arithmetic, the properties a
// published figure has to have, and above all determinism, since a chart that
// changed between renderings could never be verified against the record it
// claims to draw.
package main

import (
	"fmt"
	"math"
	"slices"
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

// TestLinearXTicks_ThinnedToWhatTheWidthCarries verifies a linear axis labels
// every whole number it has room for and no more, and always keeps the two
// counts a reader looks for, the first and the last.
//
// The credential ramp is the case this was written against: sixty-four points
// across a 794-pixel plot leave 12.6 pixels per label while two digits need
// about fourteen, so the published axis read as one continuous number. The
// assertion is on the gap rather than on a stride, because the stride that
// fits is a property of the canvas rather than a number worth pinning.
func TestLinearXTicks_ThinnedToWhatTheWidthCarries(t *testing.T) {
	tests := []struct {
		name     string
		extent   lineExtent
		wantSome []float64
	}{
		// Eight credentials: every one of them fits.
		{name: "a short ramp keeps every point", extent: lineExtent{minX: 1, maxX: 8}, wantSome: []float64{1, 2, 3, 4, 5, 6, 7, 8}},
		{name: "the published ramp is thinned", extent: lineExtent{minX: 1, maxX: 64}, wantSome: []float64{1, 5, 60, 64}},
		{name: "a thousand points still reads", extent: lineExtent{minX: 1, maxX: 1000}, wantSome: []float64{1, 1000}},
		// Far from zero the width is still what decides: eleven counts across
		// the whole plot have room for every label, which an axis measured as
		// though it ran from zero would thin to its two ends.
		{name: "a short ramp far from zero keeps every point", extent: lineExtent{minX: 100, maxX: 110}, wantSome: []float64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110}},
		{name: "one point", extent: lineExtent{minX: 3, maxX: 3}, wantSome: []float64{3}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ticks := linearXTicks(tc.extent)
			if ticks[0] != tc.extent.minX || ticks[len(ticks)-1] != tc.extent.maxX {
				t.Errorf("ticks run %v..%v, want the first and the last point kept", ticks[0], ticks[len(ticks)-1])
			}
			for _, want := range tc.wantSome {
				if !slices.Contains(ticks, want) {
					t.Errorf("ticks %v do not label %v", ticks, want)
				}
			}
			if tc.extent.maxX == tc.extent.minX {
				// One point is labeled once: its first and last are the same
				// count, and printing it twice stacks two labels in one place.
				if len(ticks) != 1 {
					t.Errorf("ticks = %v, want the one point labeled once", ticks)
				}
				return
			}
			assertTicksApart(t, ticks, tc.extent)
		})
	}
}

// assertTicksApart checks every pair of adjacent labels on a linear axis
// sits at least one label's width apart, the widest label being the last.
func assertTicksApart(t *testing.T, ticks []float64, extent lineExtent) {
	t.Helper()
	perUnit := float64(plotW) / (extent.maxX - extent.minX)
	widest := textWidth(fmt.Sprintf("%.0f", extent.maxX), xTickFontSize)
	for i := 1; i < len(ticks); i++ {
		if gap := (ticks[i] - ticks[i-1]) * perUnit; gap < widest {
			t.Errorf("labels %v and %v are %.1f px apart, closer than the %.1f px a label takes",
				ticks[i-1], ticks[i], gap, widest)
		}
	}
}

// TestPlaceEndLabels_SeparatesWhatWouldOverprint verifies the values drawn
// beside the last point of each line are spread when they collide, left alone
// when they cannot collide, and dropped when the plot has no room.
//
// series-latency is the case: six lines finishing between 1791 and 16559 ms
// on a log axis put all six labels within seventy pixels, and the published
// figure carried them written over one another. The numbers the issue that
// reported it quotes are themselves misreadings of that smudge, which is the
// best evidence there is that they could not be read.
func TestPlaceEndLabels_SeparatesWhatWouldOverprint(t *testing.T) {
	const x = 800

	t.Run("a crowded group is spread around where it sat", func(t *testing.T) {
		crowded := []endLabel{
			{x: x, y: 130, width: 30, text: "16559"},
			{x: x, y: 134, width: 26, text: "8857"},
			{x: x, y: 137, width: 26, text: "6808"},
			{x: x, y: 139, width: 26, text: "4974"},
		}
		placed := placeEndLabels(crowded)
		if len(placed) != len(crowded) {
			t.Fatalf("placed %d of %d labels, want all of them: the plot has room", len(placed), len(crowded))
		}
		assertEndLabelsSeparated(t, placed)
		assertEndLabelsInsideThePlot(t, placed)
		// Spread around where they wanted to be rather than hung below the
		// first, or every label but one ends up sitting on the line it
		// belongs to.
		if placed[0].y >= 130 {
			t.Errorf("the topmost label stayed at %.1f, so the group was not centered on itself", placed[0].y)
		}
	})

	// The processor-time figure is this case: two lines a hair apart along the
	// bottom of the plot, where spreading them would push the lower one under
	// the axis.
	t.Run("a group along the bottom is lifted back inside", func(t *testing.T) {
		const floor = padT + plotH
		got := placeEndLabels([]endLabel{
			{x: x, y: floor - 2, width: 22, text: "9.18"},
			{x: x, y: floor - 1, width: 22, text: "8.33"},
		})
		if len(got) != 2 {
			t.Fatalf("placed %d of 2 labels, want both: the plot has room above them", len(got))
		}
		assertEndLabelsSeparated(t, got)
		assertEndLabelsInsideThePlot(t, got)
	})

	t.Run("labels at different counts are left alone", func(t *testing.T) {
		apart := []endLabel{{x: 300, y: 200, width: 26, text: "one"}, {x: 800, y: 201, width: 26, text: "two"}}
		got := placeEndLabels(apart)
		if len(got) != 2 || got[0].y != 200 || got[1].y != 201 {
			t.Errorf("placed = %+v, want both where they were: their extents do not overlap", got)
		}
	})

	t.Run("more labels than the plot can hold", func(t *testing.T) {
		var many []endLabel
		for i := range 40 {
			many = append(many, endLabel{x: x, y: float64(padT + i), width: 20, text: "v"})
		}
		got := placeEndLabels(many)
		if len(got) == 0 || len(got) == len(many) {
			t.Fatalf("placed %d of %d, want some dropped and some kept", len(got), len(many))
		}
		assertEndLabelsInsideThePlot(t, got)
	})

	t.Run("nothing to place", func(t *testing.T) {
		if got := placeEndLabels(nil); len(got) != 0 {
			t.Errorf("placeEndLabels(nil) = %+v, want nothing", got)
		}
	})
}

// assertEndLabelsSeparated fails when two labels of one placement are closer
// than the pitch a value needs to be read on its own, which is the whole
// property the placement exists for. The labels come back in ascending y, so
// neighbors are the only pair that can collide.
func assertEndLabelsSeparated(t *testing.T, placed []endLabel) {
	t.Helper()
	for i := 1; i < len(placed); i++ {
		if gap := placed[i].y - placed[i-1].y; gap < endLabelPitch {
			t.Errorf("%q and %q are %.1f apart, want at least %v", placed[i-1].text, placed[i].text, gap, endLabelPitch)
		}
	}
}

// assertEndLabelsInsideThePlot fails when a label that was kept sits outside
// the plot area, where it would be drawn over the legend or under the axis.
func assertEndLabelsInsideThePlot(t *testing.T, placed []endLabel) {
	t.Helper()
	for _, label := range placed {
		if label.y < padT || label.y > padT+plotH {
			t.Errorf("a kept label sits at %.1f, outside the plot [%d, %d]", label.y, padT, padT+plotH)
		}
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
		// The upper ends of the one and two bands belong to them: a raw step
		// of exactly two or five is already round and must not be rounded up.
		{raw: 2, want: 2},
		{raw: 5, want: 5},
		{raw: 500, want: 500},
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

// TestLinearXTicks_AMultipleCrowdingAnEnd_IsDropped verifies a stride multiple
// that lands within a label's width of either end is left out, while both ends
// themselves are kept.
//
// The ends are the two counts a reader looks for, so they are never dropped;
// what has to give is the neighbor, and it has to give at both ends. The
// extent here is chosen so one multiple crowds each: at 4 to 104 the stride is
// 5, so 5 sits one unit past the first tick and 100 four units short of the
// last, and both are nearer than the 34.15 pixels a three-digit label needs.
// Without the guard the axis prints "4 5" and "100 104" on top of each other,
// which is the smudge the whole rule exists to prevent.
func TestLinearXTicks_AMultipleCrowdingAnEnd_IsDropped(t *testing.T) {
	ticks := linearXTicks(lineExtent{minX: 4, maxX: 104})

	if len(ticks) == 0 {
		t.Fatal("linearXTicks produced no ticks")
	}
	if ticks[0] != 4 || ticks[len(ticks)-1] != 104 {
		t.Errorf("ticks run %v..%v, want the extent's own ends 4..104", ticks[0], ticks[len(ticks)-1])
	}
	cases := []struct {
		name string
		tick float64
		want bool
	}{
		{name: "the multiple one unit past the first tick", tick: 5, want: false},
		{name: "the multiple four units short of the last", tick: 100, want: false},
		{name: "a multiple clear of the first tick", tick: 10, want: true},
		{name: "a multiple clear of the last", tick: 95, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := slices.Contains(ticks, tc.tick); got != tc.want {
				t.Errorf("tick %v present = %v, want %v; the axis is labeled at %v", tc.tick, got, tc.want, ticks)
			}
		})
	}
}

// TestSeriesColor_AnUngroupedSeries_TakesNoGroupColor verifies the group list a
// grouped series is colored by counts only the series that name a group.
//
// The paired latency figure mixes the two shapes: a solid p50 and a dashed p99
// share one hue per surface, and any series with no group of its own is
// colored by position instead. Counting an ungrouped series as a group would
// shift every later group one color along, so the second group would be
// painted with the third palette entry and two figures on the same page would
// disagree about which color a surface is.
func TestSeriesColor_AnUngroupedSeries_TakesNoGroupColor(t *testing.T) {
	p := testPalette()
	spec := lineSpec{Series: []lineSeries{
		{Label: "ungrouped"},
		{Label: "a p50", Group: "a"},
		{Label: "a p99", Group: "a", Dashed: true},
		{Label: "b p50", Group: "b"},
	}}

	tests := []struct {
		name  string
		index int
		want  string
	}{
		{name: "ungrouped takes its own position", index: 0, want: p.Series[0]},
		{name: "the first group takes the first color", index: 1, want: p.Series[0]},
		{name: "its dashed companion shares that color", index: 2, want: p.Series[0]},
		{name: "the second group takes the second color", index: 3, want: p.Series[1]},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := seriesColor(p, spec, tc.index); got != tc.want {
				t.Errorf("seriesColor(%d) = %q, want %q", tc.index, got, tc.want)
			}
		})
	}
}

// TestRenderLines_ASeriesWithNoPoints_CarriesNoEndLabel verifies the value
// drawn beside a line's last point is only drawn for a line that has one.
//
// A filtered record leaves a surface with no ramp at all, and the series is
// still built so the legend names it. Reading the last point of it would index
// past the end of an empty slice; skipping the label is what the guard does,
// and the figure then carries one value rather than two.
func TestRenderLines_ASeriesWithNoPoints_CarriesNoEndLabel(t *testing.T) {
	p := testPalette()
	// endLabelFontSize is used by nothing else in the document, so counting it
	// counts end labels exactly.
	marker := fmt.Sprintf(`font-size="%.1f"`, endLabelFontSize)

	both := renderLines(p, lineSpec{Title: "both measured", Series: []lineSeries{
		{Label: "measured", X: []float64{1, 2}, Y: []float64{10, 20}},
		{Label: "also measured", X: []float64{1, 2}, Y: []float64{30, 40}},
	}})
	if got := strings.Count(both, marker); got != 2 {
		t.Fatalf("two measured series drew %d end labels, want 2", got)
	}

	one := renderLines(p, lineSpec{Title: "one unmeasured", Series: []lineSeries{
		{Label: "measured", X: []float64{1, 2}, Y: []float64{10, 20}},
		{Label: "unmeasured"},
	}})
	if got := strings.Count(one, marker); got != 1 {
		t.Errorf("a series with no points drew %d end labels, want the one measured series' own", got)
	}

	// A series that stopped after its first step has a last point too, and it
	// is the only value that series has to show.
	single := renderLines(p, lineSpec{Title: "one step", Series: []lineSeries{
		{Label: "stopped at once", X: []float64{1}, Y: []float64{10}},
	}})
	if got := strings.Count(single, marker); got != 1 {
		t.Errorf("a series of one point drew %d end labels, want its one value", got)
	}
}

// TestScale_Pos_MapsTheEndsAndTheMiddle verifies a scale puts its floor on the
// plot's bottom edge, its ceiling on the top edge and the value halfway between
// them in the middle, on a linear axis whose floor is not zero and on a
// logarithmic one.
//
// Every linear axis a figure builds starts at zero, where a value and its
// distance from the floor are the same number, so this is the one place the
// floor's part in the arithmetic is held to anything.
func TestScale_Pos_MapsTheEndsAndTheMiddle(t *testing.T) {
	const bottom, top, middle = float64(padT + plotH), float64(padT), padT + plotH/2.0
	cases := []struct {
		name  string
		scale scale
		value float64
		want  float64
	}{
		{name: "linear floor", scale: scale{lo: 10, hi: 20}, value: 10, want: bottom},
		{name: "linear middle", scale: scale{lo: 10, hi: 20}, value: 15, want: middle},
		{name: "linear ceiling", scale: scale{lo: 10, hi: 20}, value: 20, want: top},
		{name: "log floor", scale: scale{lo: 1, hi: 100, log: true}, value: 1, want: bottom},
		{name: "log middle decade", scale: scale{lo: 1, hi: 100, log: true}, value: 10, want: middle},
		{name: "log ceiling", scale: scale{lo: 1, hi: 100, log: true}, value: 100, want: top},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.scale.pos(tc.value); math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("pos(%v) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// TestLogScale_FloorsAndCeilings_CoverWhatTheyAreGiven verifies the decades a
// log axis spans for the inputs its guards rewrite: a floor that is not a
// positive finite number becomes a tenth, a ceiling that is not above the
// floor becomes a decade above it, and a ceiling a hair above a power of ten
// still opens a decade of its own.
//
// Termination is held by the test above; this one holds the answers, because
// a guard that rewrote its input to the wrong value would terminate just as
// promptly and draw the data off the axis.
func TestLogScale_FloorsAndCeilings_CoverWhatTheyAreGiven(t *testing.T) {
	cases := []struct {
		name             string
		minimum, maximum float64
		wantLo, wantHi   float64
	}{
		{name: "a zero floor is a tenth", minimum: 0, maximum: 10, wantLo: 0.1, wantHi: 10},
		{name: "an infinite floor is a tenth", minimum: math.Inf(1), maximum: math.Inf(1), wantLo: 0.1, wantHi: 1},
		{name: "one value spans the decade above it", minimum: 5, maximum: 5, wantLo: 1, wantHi: 100},
		{name: "an infinite ceiling is a decade above the floor", minimum: 1, maximum: math.Inf(1), wantLo: 1, wantHi: 10},
		// log10 of the float just above ten rounds to exactly one, so the
		// ceiling's own decade is the floor's, and only the one-decade minimum
		// keeps the axis from being a single line.
		{name: "a ceiling a hair above a decade", minimum: 10, maximum: math.Nextafter(10, 11), wantLo: 10, wantHi: 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			axis, _ := logScale(tc.minimum, tc.maximum)
			if !closeTo(axis.lo, tc.wantLo) || !closeTo(axis.hi, tc.wantHi) {
				t.Errorf("logScale(%v, %v) = [%v, %v], want [%v, %v]", tc.minimum, tc.maximum, axis.lo, axis.hi, tc.wantLo, tc.wantHi)
			}
		})
	}
}

// closeTo compares two axis values relative to their size, since a decade
// computed with math.Pow need not be the literal a test spells.
func closeTo(got, want float64) bool {
	return math.Abs(got-want) <= 1e-12*math.Abs(want)
}

// TestRenderBars_AxisFromTheData verifies the three ways a bar chart's axis is
// decided by its values rather than by a default: a zero among positive bars
// does not drag a log axis a decade lower, a chart of nothing but zeros is
// drawn on the axis a chart whose tallest bar is one gets, and a p99 equal to
// its p50 draws no extension.
func TestRenderBars_AxisFromTheData(t *testing.T) {
	p := testPalette()
	t.Run("a zero bar is not the floor of a log axis", func(t *testing.T) {
		svg := renderBars(p, barSpec{
			Title: "log", Log: true, Format: msLabel, Categories: []string{"a", "b", "c"},
			Series: []barSeries{{Label: "s", Values: []float64{0, 5, 100}}},
		})
		labels := gridLabels(svg)
		if slices.Contains(labels, "0.10") || !slices.Contains(labels, "1.0") {
			t.Errorf("grid labels %v, want the axis to start at the decade of the smallest positive bar", labels)
		}
	})
	t.Run("all zeros share the axis of a tallest bar of one", func(t *testing.T) {
		tenths := func(v float64) string { return fmt.Sprintf("%.1f", v) }
		zeros := renderBars(p, barSpec{Title: "z", Format: tenths, Categories: []string{"a"}, Series: []barSeries{{Label: "s", Values: []float64{0}}}})
		one := renderBars(p, barSpec{Title: "o", Format: tenths, Categories: []string{"a"}, Series: []barSeries{{Label: "s", Values: []float64{1}}}})
		if got, want := gridLabels(zeros), gridLabels(one); !slices.Equal(got, want) {
			t.Errorf("an all-zero chart is labeled %v, want the %v of a chart whose tallest bar is one", got, want)
		}
	})
	t.Run("a p99 equal to its p50 draws no extension", func(t *testing.T) {
		const extension = `fill-opacity="0.3"`
		equal := renderBars(p, barSpec{Title: "e", Categories: []string{"a"}, Series: []barSeries{{Label: "s", Values: []float64{40}, High: []float64{40}}}})
		if got := strings.Count(equal, extension); got != 0 {
			t.Errorf("a p99 equal to its p50 drew %d extensions, want none", got)
		}
		above := renderBars(p, barSpec{Title: "a", Categories: []string{"a"}, Series: []barSeries{{Label: "s", Values: []float64{40}, High: []float64{41}}}})
		if got := strings.Count(above, extension); got != 1 {
			t.Errorf("a p99 above its p50 drew %d extensions, want one", got)
		}
	})
}

// gridLabels lists the value labels of a figure's horizontal grid, top to
// bottom, which are the only text drawn right-aligned against the plot's left
// edge.
func gridLabels(svg string) []string {
	prefix := fmt.Sprintf(`<text x="%d" y="`, padL-8)
	var out []string
	for line := range strings.SplitSeq(svg, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		start := strings.Index(line, ">") + 1
		end := strings.LastIndex(line, "</text>")
		out = append(out, line[start:end])
	}
	return out
}

// TestExtentOf_ASmallestPositiveY_IsTheLogFloor verifies a zero among a line's
// values is not taken for its smallest, since a log axis cannot place zero and
// a floor pulled down to it would squeeze every real value into the top of the
// plot.
func TestExtentOf_ASmallestPositiveY_IsTheLogFloor(t *testing.T) {
	e := extentOf(lineSpec{Series: []lineSeries{{Label: "a", X: []float64{1, 2, 3}, Y: []float64{0, 5, 50}}}})
	if e.minY != 5 || e.maxY != 50 || e.minX != 1 || e.maxX != 3 {
		t.Errorf("extent = %+v, want Y from the smallest positive value 5 to 50 over X 1 to 3", e)
	}
}

// TestXMapper_PlacesTheEndsAndTheMiddle verifies the horizontal mapping puts
// the first count on the plot's left edge, the last on its right, a single
// count in the middle, and on a log axis the geometric middle halfway, from a
// first count that is not one.
//
// A log axis whose first count is one has a floor of zero decades, where the
// floor's part in the arithmetic cannot be seen; the series start there, so
// only a test starting elsewhere can hold it.
func TestXMapper_PlacesTheEndsAndTheMiddle(t *testing.T) {
	const left, right, middle = float64(padL), float64(padL + plotW), float64(padL) + float64(plotW)/2
	cases := []struct {
		name   string
		logX   bool
		extent lineExtent
		value  float64
		want   float64
	}{
		{name: "one count sits in the middle", extent: lineExtent{minX: 7, maxX: 7}, value: 7, want: middle},
		{name: "one count on a log axis sits in the middle", logX: true, extent: lineExtent{minX: 7, maxX: 7}, value: 7, want: middle},
		{name: "log first count", logX: true, extent: lineExtent{minX: 10, maxX: 1000}, value: 10, want: left},
		{name: "log middle decade", logX: true, extent: lineExtent{minX: 10, maxX: 1000}, value: 100, want: middle},
		{name: "log last count", logX: true, extent: lineExtent{minX: 10, maxX: 1000}, value: 1000, want: right},
		{name: "linear first count", extent: lineExtent{minX: 10, maxX: 30}, value: 10, want: left},
		{name: "linear middle", extent: lineExtent{minX: 10, maxX: 30}, value: 20, want: middle},
		{name: "linear last count", extent: lineExtent{minX: 10, maxX: 30}, value: 30, want: right},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := xMapper(lineSpec{LogX: tc.logX}, tc.extent)(tc.value); math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("x(%v) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// TestRenderLines_Markers_SpanThePlotAndStackTheirLabels verifies a marker is
// a rule from the plot's top edge to its bottom edge at its count, and that
// the labels of two markers are stacked down their rules a line apart, just
// left of each, so two series stopping near one count do not write over each
// other.
func TestRenderLines_Markers_SpanThePlotAndStackTheirLabels(t *testing.T) {
	spec := lineSpec{
		Title:   "markers",
		Series:  []lineSeries{{Label: "a", X: []float64{1, 4}, Y: []float64{10, 20}}},
		Markers: []lineMarker{{X: 2, Label: "first stop"}, {X: 3, Label: "second stop"}},
	}
	svg := renderLines(testPalette(), spec)
	xPos := xMapper(spec, extentOf(spec))
	for i, marker := range spec.Markers {
		x := xPos(marker.X)
		rule := fmt.Sprintf(`<line x1="%.1f" y1="%d" x2="%.1f" y2="%d"`, x, padT, x, padT+plotH)
		label := fmt.Sprintf(`<text x="%.1f" y="%d" %s font-size="11" text-anchor="end" fill="%s">%s</text>`,
			x-4, padT+14*(i+1), fontStack, testPalette().Threshold, marker.Label)
		t.Run(marker.Label, func(t *testing.T) {
			if !strings.Contains(svg, rule) {
				t.Errorf("no rule %q from the top of the plot to its bottom", rule)
			}
			if !strings.Contains(svg, label) {
				t.Errorf("no label %q stacked down the rule", label)
			}
		})
	}
}

// TestPlaceEndLabels_GroupsByOverlapWhateverTheOrder verifies which labels are
// spread is decided by where they sit rather than by the order the series
// came in: two that overlap are separated, labels that touch count as
// overlapping, and labels at three distinct counts are all left where they
// were.
//
// The first case is laid out so that sorting the labels by anything but their
// left edges groups a label with the wrong neighbors and leaves the one pair
// that overlaps drawn on top of each other.
func TestPlaceEndLabels_GroupsByOverlapWhateverTheOrder(t *testing.T) {
	cases := []struct {
		name   string
		labels []endLabel
	}{
		{name: "an overlapping pair among labels given out of order", labels: []endLabel{
			{x: 200, width: 70, y: 200, text: "a"},
			{x: 350, width: 40, y: 150, text: "b"},
			{x: 300, width: 70, y: 300, text: "c"},
			{x: 220, width: 60, y: 200, text: "d"},
		}},
		{name: "two labels that touch", labels: []endLabel{
			{x: 300, width: 30, y: 200, text: "left"},
			{x: 330, width: 30, y: 200, text: "right"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			placed := placeEndLabels(slices.Clone(tc.labels))
			if len(placed) != len(tc.labels) {
				t.Fatalf("placed %d of %d labels, want all of them", len(placed), len(tc.labels))
			}
			for i, a := range placed {
				for _, b := range placed[i+1:] {
					overlap := max(a.x-a.width, b.x-b.width) <= min(a.x, b.x)
					if overlap && math.Abs(a.y-b.y) < endLabelPitch {
						t.Errorf("%q and %q overlap across and are %.1f apart, want at least %v", a.text, b.text, math.Abs(a.y-b.y), endLabelPitch)
					}
				}
			}
		})
	}

	t.Run("three labels at three counts stay where they were", func(t *testing.T) {
		apart := []endLabel{
			{x: 200, width: 26, y: 200, text: "one"},
			{x: 500, width: 26, y: 200, text: "two"},
			{x: 800, width: 26, y: 200, text: "three"},
		}
		for _, label := range placeEndLabels(slices.Clone(apart)) {
			if label.y != 200 {
				t.Errorf("%q moved to %.1f, want it left at 200: nothing overlaps it", label.text, label.y)
			}
		}
	})
}
