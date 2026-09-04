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
func TestBytesLabel_ScalesWithSize(t *testing.T) {
	tests := []struct {
		bytes int
		want  string
	}{
		{bytes: 512, want: "512 B"},
		{bytes: 12000, want: "12 KB"},
		{bytes: 3227321, want: "3.1 MB"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
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
