// figures_test.go covers turning a record into figures: which scenario each
// series is drawn from, what happens to a partial record, and the language
// bundles the Spanish page depends on.
package main

import (
	"reflect"
	"strings"
	"testing"
)

// TestBuildFigures_NamesAreStableAndUnique verifies the figure set is the one
// the documentation pages embed by name. A renamed figure would leave a page
// pointing at a file that is no longer written.
func TestBuildFigures_NamesAreStableAndUnique(t *testing.T) {
	want := []string{"memory", "memory-ramp", "startup", "latency"}
	var got []string
	for _, fig := range buildFigures(sampleRun(), englishLabels()) {
		got = append(got, fig.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("figures = %v, want %v", got, want)
	}
}

// TestMemorySpec_DrawsTheThreeSizingNumbers verifies the sizing figure takes
// one process from stdio and both ends of the credential ramp from HTTP, and
// that its threshold is the 512 MiB floor the deployment documentation claims.
func TestMemorySpec_DrawsTheThreeSizingNumbers(t *testing.T) {
	run := sampleRun()
	spec := memorySpec(run, englishLabels())

	if !reflect.DeepEqual(spec.Categories, surfaceOrder) {
		t.Errorf("categories = %v, want %v", spec.Categories, surfaceOrder)
	}
	if len(spec.Series) != 3 {
		t.Fatalf("the figure has %d series, want three", len(spec.Series))
	}
	stdio, _ := baseScenario(run, transportStdio, surfaceDynamic)
	httpScenario, _ := baseScenario(run, transportHTTP, surfaceDynamic)
	if spec.Series[0].Values[0] != stdio.Memory.OneClientMiB {
		t.Errorf("stdio series = %v, want %v", spec.Series[0].Values[0], stdio.Memory.OneClientMiB)
	}
	if spec.Series[1].Values[0] != httpScenario.Memory.OneClientMiB {
		t.Errorf("HTTP one-credential series = %v, want %v", spec.Series[1].Values[0], httpScenario.Memory.OneClientMiB)
	}
	if spec.Series[2].Values[0] != httpScenario.Memory.AllClientsMiB {
		t.Errorf("HTTP all-credentials series = %v, want %v", spec.Series[2].Values[0], httpScenario.Memory.AllClientsMiB)
	}
	if !strings.Contains(spec.Series[2].Label, "4") {
		t.Errorf("the label %q does not say how many credentials it is", spec.Series[2].Label)
	}
	if spec.Threshold == nil || spec.Threshold.Value != 512 {
		t.Errorf("threshold = %+v, want the 512 MiB floor", spec.Threshold)
	}
}

// TestRampSpec_OnePointPerCredential verifies the ramp figure draws the HTTP
// scenarios only, with one point per credential admitted.
func TestRampSpec_OnePointPerCredential(t *testing.T) {
	run := sampleRun()
	spec := rampSpec(run, englishLabels())

	if len(spec.Series) != len(surfaceOrder) {
		t.Fatalf("the ramp has %d series, want one per surface", len(spec.Series))
	}
	httpScenario, _ := baseScenario(run, transportHTTP, surfaceDynamic)
	if len(spec.Series[0].X) != httpScenario.Clients {
		t.Errorf("the first line has %d points, want %d", len(spec.Series[0].X), httpScenario.Clients)
	}
	if spec.Series[0].Y[0] != httpScenario.Ramp[0].RSSMiB {
		t.Errorf("the first point is %v, want the first ramp sample %v", spec.Series[0].Y[0], httpScenario.Ramp[0].RSSMiB)
	}
}

// TestStartupSpec_ContrastsColdWithWarm verifies the startup figure is drawn
// on a log axis from the three moments that make registration visible.
func TestStartupSpec_ContrastsColdWithWarm(t *testing.T) {
	spec := startupSpec(sampleRun(), englishLabels())
	if !spec.Log {
		t.Error("the startup figure is not on a log axis, so the warm bars would be invisible")
	}
	if len(spec.Series) != 3 {
		t.Fatalf("the figure has %d series, want ready, cold and warm", len(spec.Series))
	}
	if spec.Series[1].Values[0] <= spec.Series[2].Values[0] {
		t.Error("the cold tools/list is not slower than the warm one, which is the point of the figure")
	}
}

// TestLatencySpec_CarriesP50AndP99 verifies the latency figure draws medians
// with the tail behind them, per method, for the transports and surfaces that
// bracket the range.
func TestLatencySpec_CarriesP50AndP99(t *testing.T) {
	spec := latencySpec(sampleRun(), englishLabels())

	want := []string{"resources/list", "tools/call", "tools/list"}
	if !reflect.DeepEqual(spec.Categories, want) {
		t.Errorf("categories = %v, want %v", spec.Categories, want)
	}
	for _, series := range spec.Series {
		if len(series.High) != len(series.Values) {
			t.Errorf("series %q has %d p99 values for %d medians", series.Label, len(series.High), len(series.Values))
		}
		for i := range series.Values {
			if series.High[i] < series.Values[i] {
				t.Errorf("series %q p99 %v is below its p50 %v", series.Label, series.High[i], series.Values[i])
			}
		}
	}
}

// TestPresentSurfaces_PartialRecord_DrawsWhatWasMeasured verifies a record
// from a filtered run yields figures over the surfaces it actually has, rather
// than inventing empty categories or panicking on a missing scenario.
func TestPresentSurfaces_PartialRecord_DrawsWhatWasMeasured(t *testing.T) {
	run := sampleRun()
	run.Scenarios = run.Scenarios[:1] // stdio, dynamic only

	if got := presentSurfaces(run); !reflect.DeepEqual(got, []string{surfaceDynamic}) {
		t.Errorf("presentSurfaces = %v, want just dynamic", got)
	}
	for _, fig := range buildFigures(run, englishLabels()) {
		if svg := fig.Render(testPalette()); !strings.HasPrefix(svg, "<svg") {
			t.Errorf("%s did not render from a partial record", fig.Name)
		}
	}
}

// TestBaseScenario_IgnoresTelemetryRuns verifies the baseline figures never
// pick up a telemetry scenario, which would mix a comparison into the numbers
// it is supposed to be compared against.
func TestBaseScenario_IgnoresTelemetryRuns(t *testing.T) {
	run := sampleRun()
	run.Scenarios[3].Telemetry = true // the http-dynamic entry

	if _, ok := baseScenario(run, transportHTTP, surfaceDynamic); ok {
		t.Error("baseScenario returned a telemetry run as the baseline")
	}
}

// TestLabelBundles_AreCompleteAndDistinct verifies both languages fill every
// field the renderers read, and that the Spanish bundle is actually
// translated: a copied English string would ship English on the Spanish page,
// which the site's language rule forbids.
func TestLabelBundles_AreCompleteAndDistinct(t *testing.T) {
	english, spanish := englishLabels(), spanishLabels()

	for _, l := range []labels{english, spanish} {
		t.Run(l.Code, func(t *testing.T) {
			assertNoEmptyFields(t, l)
			for _, name := range []string{"memory", "memory-ramp", "startup", "latency"} {
				if l.FigureAlt[name] == "" {
					t.Errorf("no alternative text for the %s figure", name)
				}
			}
			for _, key := range []string{"summary", "startup", "latency"} {
				if l.TableCaption[key] == "" {
					t.Errorf("no caption for the %s table", key)
				}
			}
		})
	}

	if english.MemoryTitle == spanish.MemoryTitle {
		t.Error("the Spanish figure title is the English one")
	}
	if reflect.DeepEqual(english.SummaryHead, spanish.SummaryHead) {
		t.Error("the Spanish table headers are the English ones")
	}
}

// assertNoEmptyFields fails for every string, slice or map field of a label
// bundle that was left blank, naming it. A missing figure title or table
// header would otherwise show up as a hole in a published page.
func assertNoEmptyFields(t *testing.T, l labels) {
	t.Helper()
	value := reflect.ValueOf(l)
	for i := range value.NumField() {
		field := value.Field(i)
		name := value.Type().Field(i).Name
		empty := (field.Kind() == reflect.String && field.String() == "") ||
			((field.Kind() == reflect.Slice || field.Kind() == reflect.Map) && field.Len() == 0)
		if empty {
			t.Errorf("%s is empty", name)
		}
	}
}

// TestBytesLabel_ScalesWithSize verifies payload sizes are printed in the unit
// a reader expects at each magnitude, since the three surfaces span bytes to
// megabytes.
//
// The units are decimal, and the cases below pin that rather than assume it:
// each threshold is checked on both sides, and the three real tools/list sizes
// are checked against the figure a division by 1000 gives. Dividing by 1024
// under an "MB" label is the defect this pins, and it published a `meta`
// tools/list of 598,510 bytes as 584 KB.
func TestBytesLabel_ScalesWithSize(t *testing.T) {
	tests := []struct {
		name  string
		bytes int
		want  string
	}{
		{name: "bytes below the kilobyte threshold", bytes: 512, want: "512 B"},
		{name: "one byte below a kilobyte", bytes: 999, want: "999 B"},
		{name: "exactly one kilobyte", bytes: 1000, want: "1 KB"},
		{name: "a dynamic tools/list", bytes: 12142, want: "12 KB"},
		{name: "a meta tools/list", bytes: 598510, want: "599 KB"},
		{name: "one byte below a megabyte rounds within its own unit", bytes: 999999, want: "1000 KB"},
		{name: "exactly one megabyte", bytes: 1000000, want: "1.0 MB"},
		{name: "an individual tools/list", bytes: 3235932, want: "3.2 MB"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := bytesLabel(tc.bytes); got != tc.want {
				t.Errorf("bytesLabel(%d) = %q, want %q", tc.bytes, got, tc.want)
			}
		})
	}
}

// TestMsLabel_PrecisionFollowsMagnitude verifies a sub-millisecond figure and
// a six-second one are both readable on the same axis.
func TestMsLabel_PrecisionFollowsMagnitude(t *testing.T) {
	tests := []struct {
		value float64
		want  string
	}{
		{value: 0, want: "0"},
		{value: 0.573, want: "0.57"},
		{value: 3.14, want: "3.1"},
		{value: 2841.7, want: "2842"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := msLabel(tc.value); got != tc.want {
				t.Errorf("msLabel(%v) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestScenarioLabel_NamesTransportSurfaceAndTelemetry verifies a table row can
// be told apart from every other row in the record.
func TestScenarioLabel_NamesTransportSurfaceAndTelemetry(t *testing.T) {
	plain := scenarioLabel(Scenario{Transport: transportHTTP, Surface: surfaceMeta})
	if plain != "http, meta" {
		t.Errorf("scenarioLabel = %q, want \"http, meta\"", plain)
	}
	withTelemetry := scenarioLabel(Scenario{Transport: transportStdio, Surface: surfaceDynamic, Telemetry: true})
	if !strings.Contains(withTelemetry, "telemetry") {
		t.Errorf("scenarioLabel = %q, want it to say telemetry is on", withTelemetry)
	}
}

// TestJoinNonEmpty_SkipsBlanks verifies the note joiner never leaves a
// dangling separator.
func TestJoinNonEmpty_SkipsBlanks(t *testing.T) {
	if got := joinNonEmpty(", ", "a", "", "  ", "b"); got != "a, b" {
		t.Errorf("joinNonEmpty = %q, want \"a, b\"", got)
	}
	if got := joinNonEmpty(", "); got != "" {
		t.Errorf("joinNonEmpty of nothing = %q, want empty", got)
	}
}

// TestFigures_StdioOnlyRecord_OmitsTheTransportItNeverMeasured verifies a
// record holding one transport draws no series for the other.
//
// presentSurfaces admits such a record, and the specs used to discard the
// second result of baseScenario and append the zero value of the Scenario it
// did not find. The sizing figure then drew an HTTP bar of zero MiB labeled
// "HTTP, 0 credentials", which states something about the server rather than
// about the run, and the startup figure filled every surface with zeros, which
// reads as a surface that starts instantly.
func TestFigures_StdioOnlyRecord_OmitsTheTransportItNeverMeasured(t *testing.T) {
	run := sampleRun()
	var stdioOnly []Scenario
	for _, scenario := range run.Scenarios {
		if scenario.Transport == transportStdio {
			stdioOnly = append(stdioOnly, scenario)
		}
	}
	run.Scenarios = stdioOnly
	l := englishLabels()

	memory := memorySpec(run, l)
	if len(memory.Series) != 1 {
		t.Fatalf("sizing figure has %d series, want only the stdio one", len(memory.Series))
	}
	if memory.Series[0].Label != l.MemoryStdioOne {
		t.Errorf("the surviving series is %q, want %q", memory.Series[0].Label, l.MemoryStdioOne)
	}

	// Both of these are drawn from HTTP alone, so neither has anything to say
	// about this record.
	if series := startupSpec(run, l).Series; len(series) != 0 {
		t.Errorf("startup figure has %d series, want none", len(series))
	}
	if series := rampSpec(run, l).Series; len(series) != 0 {
		t.Errorf("ramp figure has %d series, want none", len(series))
	}

	var drawn []string
	for _, fig := range buildFigures(run, l) {
		drawn = append(drawn, fig.Name)
	}
	if !reflect.DeepEqual(drawn, []string{"memory", "latency"}) {
		t.Errorf("figures = %v, want only the two a stdio record can draw", drawn)
	}
}

// TestCallDetail_TranslatesOnlyWhatItKnows verifies the Spanish page renders
// the recorded call descriptions in Spanish and leaves tool names alone.
//
// The record stays English, because it is a project artifact and because the
// same column carries tool names, which are not words in any language. The
// Spanish latency table was printing "smallest listing" and "whole surface"
// verbatim.
func TestCallDetail_TranslatesOnlyWhatItKnows(t *testing.T) {
	spanish := spanishLabels()
	if got := spanish.callDetail("whole surface"); got != "la superficie completa" {
		t.Errorf("callDetail(%q) = %q, want it translated", "whole surface", got)
	}
	if got := spanish.callDetail("gitlab_find_action"); got != "gitlab_find_action" {
		t.Errorf("callDetail of a tool name = %q, want it unchanged", got)
	}
	// Every description the benchmark records has to be translatable in both
	// bundles, or a page silently prints the other language.
	for _, bundle := range []labels{englishLabels(), spanish} {
		for _, detail := range recordedCallDetails {
			t.Run(bundle.Code+"/"+detail, func(t *testing.T) {
				if _, ok := bundle.CallDetail[detail]; !ok {
					t.Errorf("the %s bundle has no wording for the recorded description %q", bundle.Code, detail)
				}
			})
		}
	}
}

// TestOrderedSeries_SurfaceOrderAndNoEmptyOnes verifies the series are
// drawn in the order the documentation introduces the surfaces, and that
// one with no steps is left out rather than drawn as a line of nothing.
func TestOrderedSeries_SurfaceOrderAndNoEmptyOnes(t *testing.T) {
	var got []string
	for _, series := range orderedSeries(sampleSeriesRun()) {
		got = append(got, series.Surface)
	}
	if !reflect.DeepEqual(got, surfaceOrder) {
		t.Errorf("orderedSeries = %v, want %v", got, surfaceOrder)
	}
	if series := orderedSeries(sampleRun()); len(series) != 0 {
		t.Errorf("a record with no series yielded %d", len(series))
	}
}

// TestSeriesMemorySpec_BudgetThresholdAndStopMarkers verifies the memory
// figure draws one line per surface from the peaks, the budget as the
// threshold rule, and a marker for every series that stopped early.
func TestSeriesMemorySpec_BudgetThresholdAndStopMarkers(t *testing.T) {
	run := sampleSeriesRun()
	spec := seriesMemorySpec(run, englishLabels())

	if !spec.LogX || spec.LogY {
		t.Errorf("axes LogX=%v LogY=%v, want a log X over a linear Y", spec.LogX, spec.LogY)
	}
	if len(spec.Series) != 3 {
		t.Fatalf("%d series, want one per surface", len(spec.Series))
	}
	dynamic := spec.Series[0]
	if dynamic.Label != surfaceDynamic || !reflect.DeepEqual(dynamic.X, []float64{1, 5}) || !reflect.DeepEqual(dynamic.Y, []float64{220, 500}) {
		t.Errorf("dynamic line = %+v, want the peaks at 1 and 5 credentials", dynamic)
	}
	if spec.Threshold == nil || spec.Threshold.Value != 4000 || !strings.Contains(spec.Threshold.Label, "4000") {
		t.Errorf("threshold = %+v, want the 4000 MiB budget", spec.Threshold)
	}
	wantMarkers := []lineMarker{
		{X: 5, Label: "dynamic: stopped at 5"},
		{X: 5, Label: "individual: stopped at 5"},
	}
	if !reflect.DeepEqual(spec.Markers, wantMarkers) {
		t.Errorf("markers = %+v, want %+v", spec.Markers, wantMarkers)
	}

	t.Run("no budget draws no threshold", func(t *testing.T) {
		unbudgeted := sampleSeriesRun()
		for i := range unbudgeted.Series {
			unbudgeted.Series[i].BudgetMiB = 0
		}
		if unbudgetedSpec := seriesMemorySpec(unbudgeted, englishLabels()); unbudgetedSpec.Threshold != nil {
			t.Errorf("threshold = %+v on series that had no budget", unbudgetedSpec.Threshold)
		}
	})
}

// TestSeriesLatencySpec_PairsMedianAndTailPerSurface verifies the latency
// figure carries two lines per surface, the tail dashed and grouped with its
// median so they share a color, on log axes.
func TestSeriesLatencySpec_PairsMedianAndTailPerSurface(t *testing.T) {
	spec := seriesLatencySpec(sampleSeriesRun(), englishLabels())

	if !spec.LogX || !spec.LogY {
		t.Errorf("axes LogX=%v LogY=%v, want both logarithmic", spec.LogX, spec.LogY)
	}
	if len(spec.Series) != 6 {
		t.Fatalf("%d series, want a median and a tail per surface", len(spec.Series))
	}
	for i := 0; i < len(spec.Series); i += 2 {
		median, tail := spec.Series[i], spec.Series[i+1]
		t.Run(median.Group, func(t *testing.T) {
			if median.Dashed || !tail.Dashed {
				t.Errorf("median dashed=%v tail dashed=%v, want only the tail dashed", median.Dashed, tail.Dashed)
			}
			if median.Group == "" || median.Group != tail.Group {
				t.Errorf("groups %q and %q, want the pair grouped by surface", median.Group, tail.Group)
			}
			if !strings.HasSuffix(median.Label, " p50") || !strings.HasSuffix(tail.Label, " p99") {
				t.Errorf("labels %q and %q, want p50 and p99", median.Label, tail.Label)
			}
			for j := range median.Y {
				if tail.Y[j] < median.Y[j] {
					t.Errorf("tail %v below median %v at point %d", tail.Y[j], median.Y[j], j)
				}
			}
		})
	}
}

// TestSeriesCPUSpec_OneLinePerSurface verifies the CPU figure draws the
// per-call processor time of each surface over the credential count.
func TestSeriesCPUSpec_OneLinePerSurface(t *testing.T) {
	spec := seriesCPUSpec(sampleSeriesRun(), englishLabels())
	if len(spec.Series) != 3 || !spec.LogX {
		t.Fatalf("spec %+v, want three lines over a log X", spec)
	}
	if meta := spec.Series[1]; meta.Label != surfaceMeta || !reflect.DeepEqual(meta.Y, []float64{1, 1.1, 1.4}) {
		t.Errorf("meta line = %+v, want the CPU per call of its three steps", meta)
	}
	if spec.Format(1.234) != "1.23" {
		t.Errorf("format(1.234) = %q, want two decimals", spec.Format(1.234))
	}
}

// TestBuildFigures_WithSeries_AddsTheThreeSeriesFigures verifies the figure
// set grows by the three series figures when a record holds a series, in a
// stable order and under the names the pages embed, and that every figure
// renders.
func TestBuildFigures_WithSeries_AddsTheThreeSeriesFigures(t *testing.T) {
	want := []string{"memory", "memory-ramp", "startup", "latency", "series-memory", "series-latency", "series-cpu"}
	figures := buildFigures(sampleSeriesRun(), englishLabels())
	var got []string
	for _, fig := range figures {
		got = append(got, fig.Name)
		if svg := fig.Render(testPalette()); !strings.HasPrefix(svg, "<svg") {
			t.Errorf("%s did not render", fig.Name)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("figures = %v, want %v", got, want)
	}
	for _, l := range []labels{englishLabels(), spanishLabels()} {
		t.Run(l.Code, func(t *testing.T) {
			for _, name := range want[4:] {
				t.Run(name, func(t *testing.T) {
					if l.FigureAlt[name] == "" {
						t.Errorf("the %s bundle has no alternative text for %s", l.Code, name)
					}
				})
			}
		})
	}
}

// TestSpecs_LabelBothAxes verifies every figure names what its axes are, in
// the units they are in.
//
// A bar chart's categories carry their own labels, but "dynamic, meta,
// individual" only reads as a tool surface to somebody who already knows, and
// a figure has to be legible away from the page that says so. The vertical
// axis is asserted for the same reason: a resident set with no unit on it is
// three orders of magnitude ambiguous.
func TestSpecs_LabelBothAxes(t *testing.T) {
	run := sampleSeriesRun()
	l := englishLabels()

	bars := map[string]barSpec{
		figureMemory:  memorySpec(run, l),
		figureStartup: startupSpec(run, l),
		figureLatency: latencySpec(run, l),
	}
	for name, spec := range bars {
		t.Run(name, func(t *testing.T) {
			if spec.XAxis == "" {
				t.Errorf("the %s figure does not say what its categories are", name)
			}
			if spec.YAxis == "" {
				t.Errorf("the %s figure does not say what its values are", name)
			}
		})
	}

	lines := map[string]lineSpec{
		figureMemoryRamp:    rampSpec(run, l),
		figureSeriesMemory:  seriesMemorySpec(run, l),
		figureSeriesLatency: seriesLatencySpec(run, l),
		figureSeriesCPU:     seriesCPUSpec(run, l),
	}
	for name, spec := range lines {
		t.Run(name, func(t *testing.T) {
			if spec.XAxis == "" {
				t.Errorf("the %s figure does not label its horizontal axis", name)
			}
			if spec.YAxis == "" {
				t.Errorf("the %s figure does not label its vertical axis", name)
			}
		})
	}
}

// TestChartProvenance_NamesTheMachineAndTheBuild verifies the line every
// figure carries along its bottom edge says which machine and which build the
// numbers came from, in the language of the page the figure is going on.
//
// A chart travels: it is embedded, screenshotted and quoted away from the page
// that states those, so a figure whose provenance was missing would be a
// memory curve nobody could act on. The Spanish bundle is asserted beside the
// English one because a rendered SVG is content rather than chrome, and a
// Spanish page carrying an English sentence is half translated.
func TestChartProvenance_NamesTheMachineAndTheBuild(t *testing.T) {
	run := sampleRun()
	for _, tc := range []struct {
		l    labels
		want []string
	}{
		{englishLabels(), []string{"Measured on", "Test CPU", "8 logical CPUs", "linux/amd64", "go1.27.1", "Build 2.7.6 (01234567)", "2026-09-04"}},
		{spanishLabels(), []string{"Medido en", "Test CPU", "8 CPU lógicas", "linux/amd64", "go1.27.1", "Compilación 2.7.6 (01234567)", "2026-09-04"}},
	} {
		t.Run(tc.l.Code, func(t *testing.T) {
			got := chartProvenance(run, tc.l)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("provenance %q does not mention %q", got, want)
				}
			}
			// The kernel and the installed memory belong to the sentence under
			// the measurements, which has the width for them; on a chart they
			// would push the build off the canvas.
			for _, unwanted := range []string{"6.1.0", "61"} {
				if strings.Contains(got, unwanted) {
					t.Errorf("provenance %q carries %q, which the short host description drops", got, unwanted)
				}
			}
		})
	}
}

// TestChartProvenance_BundleWithoutATemplate_DrawsNothing verifies a label
// bundle carrying no provenance template yields an empty string rather than a
// format verb printed at a reader.
func TestChartProvenance_BundleWithoutATemplate_DrawsNothing(t *testing.T) {
	if got := chartProvenance(sampleRun(), labels{Code: "xx"}); got != "" {
		t.Errorf("chartProvenance with no template = %q, want empty", got)
	}
}

// TestBuildFigures_EveryFigureCarriesTheProvenance verifies the stamp is
// applied in buildFigures rather than in each spec builder, so a figure added
// later cannot be drawn without one.
func TestBuildFigures_EveryFigureCarriesTheProvenance(t *testing.T) {
	run := sampleSeriesRun()
	want := chartProvenance(run, englishLabels())
	if want == "" {
		t.Fatal("the sample record produced no provenance to look for")
	}
	for _, fig := range buildFigures(run, englishLabels()) {
		t.Run(fig.Name, func(t *testing.T) {
			svg := fig.Render(testPalette())
			if !strings.Contains(svg, xmlEscape(want)) {
				t.Errorf("figure %s does not carry its provenance", fig.Name)
			}
		})
	}
}

// TestMeasurementDay_TrimsATimestampAndLeavesAnythingElse verifies the chart
// footer prints a date where the record holds an RFC 3339 timestamp, and
// prints whatever it holds otherwise: a record written by hand for a test is
// still worth naming.
func TestMeasurementDay_TrimsATimestampAndLeavesAnythingElse(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"rfc3339", "2026-09-04T00:00:00Z", "2026-09-04"},
		{"date only", "2026-09-04", "2026-09-04"},
		{"too short", "2026-09", "2026-09"},
		{"not a date", "sometime last Tuesday", "sometime last Tuesday"},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := sampleRun()
			run.GeneratedAt = tc.in
			if got := measurementDay(run); got != tc.want {
				t.Errorf("measurementDay(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestBuildLabel_FallsBackWhenTheRecordNamesNoBuild verifies a record with
// neither a version nor a commit is described as "unknown" rather than leaving
// an empty slot in the sentence, which would read as a build with no name.
func TestBuildLabel_FallsBackWhenTheRecordNamesNoBuild(t *testing.T) {
	for _, tc := range []struct {
		name   string
		server ServerInfo
		want   string
	}{
		{"version and commit", ServerInfo{Version: "2.7.6", Commit: "0123456789abcdef"}, "2.7.6 (01234567)"},
		{"version only", ServerInfo{Version: "2.7.6"}, "2.7.6"},
		{"short commit kept whole", ServerInfo{Version: "2.7.6", Commit: "abc"}, "2.7.6 (abc)"},
		{"nothing", ServerInfo{}, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := sampleRun()
			run.Server = tc.server
			if got := buildLabel(run); got != tc.want {
				t.Errorf("buildLabel = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestLatencySpec_MissingMethod_DrawsZero verifies a scenario that never
// measured one of the three methods gets a zero bar for it rather than a
// shifted one: the bars are positional, so dropping the entry would move
// the next method's numbers under the wrong label.
func TestLatencySpec_MissingMethod_DrawsZero(t *testing.T) {
	run := sampleRun()
	for i := range run.Scenarios {
		var kept []MethodLatency
		for _, latency := range run.Scenarios[i].Latency {
			if latency.Method != methodToolsCall {
				kept = append(kept, latency)
			}
		}
		run.Scenarios[i].Latency = kept
	}
	spec := latencySpec(run, englishLabels())
	for _, series := range spec.Series {
		t.Run(series.Label, func(t *testing.T) {
			if len(series.Values) != 3 || series.Values[1] != 0 || series.High[1] != 0 {
				t.Errorf("series %+v, want a zero in the tools/call position", series)
			}
		})
	}
}
