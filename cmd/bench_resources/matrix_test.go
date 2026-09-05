// matrix_test.go covers what gets measured: the published matrix's shape, the
// filter that selects part of it, and the per-surface tool call, whose name
// differs per surface and must exist on the surface it is used with.
package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// testSettings are matrix settings for a test: the given rounds, and the
// smoke series so a plan's shape can be checked without a thousand-credential
// list behind it.
func testSettings(rounds int) matrixSettings {
	return matrixSettings{rounds: rounds, steps: quickSeriesSteps, stepDuration: quickStepDuration}
}

// pointPlans keeps the plans that measure one point, leaving the series out.
func pointPlans(plans []scenarioPlan) []scenarioPlan {
	var out []scenarioPlan
	for _, plan := range plans {
		if !plan.isSeries() {
			out = append(out, plan)
		}
	}
	return out
}

// TestPublishedMatrix_CoversEveryAxis verifies the committed matrix exercises
// each axis the issue names: both transports, all three surfaces, telemetry
// both ways, and more than one client. A matrix that quietly lost an axis
// would publish a page that answers less than it claims to.
func TestPublishedMatrix_CoversEveryAxis(t *testing.T) {
	plans := publishedMatrix(testSettings(3))

	transports := map[string]bool{}
	surfaces := map[string]bool{}
	telemetry := map[bool]bool{}
	for _, plan := range pointPlans(plans) {
		transports[plan.Transport] = true
		surfaces[plan.Surface] = true
		telemetry[plan.Telemetry] = true
		if plan.Clients < 1 || plan.Parallel < 1 || plan.Rounds != 3 {
			t.Errorf("%s has an unusable shape: %+v", plan.ID, plan)
		}
	}
	for _, transport := range []string{transportStdio, transportHTTP} {
		t.Run(transport, func(t *testing.T) {
			if !transports[transport] {
				t.Errorf("the matrix never measures %s", transport)
			}
		})
	}
	for _, surface := range surfaceOrder {
		if !surfaces[surface] {
			t.Errorf("the matrix never measures the %s surface", surface)
		}
	}
	if !telemetry[true] || !telemetry[false] {
		t.Error("the matrix does not measure telemetry both on and off")
	}
}

// TestPublishedMatrix_ScenarioIDsAreUnique verifies no two scenarios share an
// id, since every figure and table looks them up by it.
func TestPublishedMatrix_ScenarioIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, plan := range publishedMatrix(testSettings(1)) {
		if seen[plan.ID] {
			t.Errorf("duplicate scenario id %q", plan.ID)
		}
		seen[plan.ID] = true
	}
}

// TestPublishedMatrix_OneSeriesPerSurface verifies the published matrix ends
// with a concurrency series on HTTP for every surface, each carrying the
// settings' counts and steady-phase length: the series is where the scale a
// shared deployment meets is measured, and a surface without one would be
// published from a handful of credentials alone.
func TestPublishedMatrix_OneSeriesPerSurface(t *testing.T) {
	settings := matrixSettings{rounds: 1, steps: []int{1, 5, 25}, stepDuration: 3 * time.Second}
	plans := publishedMatrix(settings)

	series := map[string]scenarioPlan{}
	for _, plan := range plans {
		if plan.isSeries() {
			series[plan.Surface] = plan
		}
	}
	for _, surface := range surfaceOrder {
		t.Run(surface, func(t *testing.T) {
			plan, ok := series[surface]
			if !ok {
				t.Fatalf("no series for the %s surface", surface)
			}
			if plan.ID != "http-"+surface+"-series" || plan.Transport != transportHTTP {
				t.Errorf("series plan %+v, want http-%s-series on HTTP", plan, surface)
			}
			if !reflect.DeepEqual(plan.Steps, settings.steps) || plan.StepDuration != settings.stepDuration {
				t.Errorf("series steps %v over %s, want the settings' %v over %s", plan.Steps, plan.StepDuration, settings.steps, settings.stepDuration)
			}
			if plan.Parallel < 1 || plan.Clients != 0 || plan.Rounds != 0 {
				t.Errorf("series plan %+v has a point's shape", plan)
			}
		})
	}
	// The point scenarios come first so their figures are on disk before the
	// hour a series can take begins.
	if plans[len(plans)-1].Surface != surfaceIndividual || !plans[len(plans)-1].isSeries() {
		t.Errorf("the matrix ends with %s, want the individual series", plans[len(plans)-1].ID)
	}
}

// TestDefaultSeriesSteps_AscendOverThreeDecades pins the published counts:
// they start at one credential, reach a thousand, and ascend, which is what
// parseSteps demands of a list given by hand.
func TestDefaultSeriesSteps_AscendOverThreeDecades(t *testing.T) {
	if defaultSeriesSteps[0] != 1 || defaultSeriesSteps[len(defaultSeriesSteps)-1] != 1000 {
		t.Errorf("default steps %v, want 1 through 1000", defaultSeriesSteps)
	}
	for i := 1; i < len(defaultSeriesSteps); i++ {
		if defaultSeriesSteps[i] <= defaultSeriesSteps[i-1] {
			t.Errorf("default steps %v do not ascend at index %d", defaultSeriesSteps, i)
		}
	}
	if _, err := parseSteps(strings.Trim(strings.Join(strings.Fields(fmt.Sprint(defaultSeriesSteps)), ","), "[]")); err != nil {
		t.Errorf("the default list would be refused if typed: %v", err)
	}
}

// TestQuickMatrix_IsSmallerAndStillTwoTransports verifies the smoke matrix is
// short enough to be worth running while still driving both transports and
// one series, which is the only reason to have one.
func TestQuickMatrix_IsSmallerAndStillTwoTransports(t *testing.T) {
	quick := quickMatrix(testSettings(1))
	if len(quick) >= len(publishedMatrix(testSettings(3))) {
		t.Error("the quick matrix is not smaller than the published one")
	}
	transports := map[string]bool{}
	seriesCount := 0
	for _, plan := range quick {
		transports[plan.Transport] = true
		if plan.isSeries() {
			seriesCount++
			if !reflect.DeepEqual(plan.Steps, quickSeriesSteps) || plan.StepDuration != quickStepDuration {
				t.Errorf("quick series %+v, want the smoke counts %v over %s", plan, quickSeriesSteps, quickStepDuration)
			}
		}
	}
	if len(transports) != 2 {
		t.Errorf("the quick matrix drives %d transports, want both", len(transports))
	}
	if seriesCount != 1 {
		t.Errorf("the quick matrix holds %d series, want one", seriesCount)
	}
}

// TestParseSteps_AcceptsAscendingCountsOnly verifies the -clients list is
// read as positive, ascending counts, and refuses everything else with the
// value named: a list that went down would measure the previous step's pool
// again under a smaller name, and a zero would admit nobody.
func TestParseSteps_AcceptsAscendingCountsOnly(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    []int
		wantErr string
	}{
		{name: "the smoke list", raw: "1,2,5", want: []int{1, 2, 5}},
		{name: "spaces and a trailing comma", raw: " 10, 20 ,50,", want: []int{10, 20, 50}},
		{name: "one count", raw: "7", want: []int{7}},
		{name: "empty", raw: "", wantErr: "no credential counts"},
		{name: "only commas", raw: ",,", wantErr: "no credential counts"},
		{name: "not a number", raw: "1,two", wantErr: `"two"`},
		{name: "zero", raw: "0,1", wantErr: `"0"`},
		{name: "negative", raw: "-5", wantErr: `"-5"`},
		{name: "descending", raw: "5,2", wantErr: "does not come after"},
		{name: "repeated", raw: "5,5", wantErr: "does not come after"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSteps(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("parseSteps(%q) = %v, %v; want an error saying %q", tc.raw, got, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSteps(%q): %v", tc.raw, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseSteps(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

// TestSelectPlans_Filters verifies the scenario filter keeps matrix order,
// accepts an empty filter as "everything", and refuses a name that matches
// nothing rather than silently measuring a smaller matrix.
func TestSelectPlans_Filters(t *testing.T) {
	plans := publishedMatrix(testSettings(1))

	tests := []struct {
		name    string
		filter  string
		wantIDs []string
		wantErr bool
	}{
		{name: "empty keeps all", filter: "", wantIDs: idsOf(plans)},
		{name: "one", filter: "http-meta", wantIDs: []string{"http-meta"}},
		{name: "a series", filter: "http-meta-series", wantIDs: []string{"http-meta-series"}},
		{
			name:    "several keep matrix order",
			filter:  "http-dynamic, stdio-dynamic",
			wantIDs: []string{"stdio-dynamic", "http-dynamic"},
		},
		{name: "typo", filter: "http-dinamic", wantErr: true},
		{name: "one good one bad", filter: "http-meta,nope", wantErr: true},
		{name: "only separators", filter: " , ,", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectPlans(plans, tc.filter)
			if tc.wantErr {
				if err == nil {
					t.Fatal("selectPlans accepted a filter that names no scenario")
				}
				return
			}
			if err != nil {
				t.Fatalf("selectPlans: %v", err)
			}
			gotIDs := idsOf(got)
			if strings.Join(gotIDs, ",") != strings.Join(tc.wantIDs, ",") {
				t.Errorf("selected %v, want %v", gotIDs, tc.wantIDs)
			}
		})
	}
}

// TestCallFor_EverySurface_NamesAToolThatSurfaceHas verifies each surface's
// tools/call target is named, carries the arguments that tool needs, and that
// an unknown surface is refused.
//
// The names themselves are asserted because they are surface-specific and
// wrong ones would only show up as a scenario reporting that every call
// failed, hours into a run.
func TestCallFor_EverySurface_NamesAToolThatSurfaceHas(t *testing.T) {
	tests := []struct {
		surface  string
		wantName string
		wantArg  string
	}{
		{surface: surfaceDynamic, wantName: "gitlab_find_action", wantArg: "query"},
		{surface: surfaceMeta, wantName: "gitlab_server", wantArg: "action"},
		{surface: surfaceIndividual, wantName: "gitlab_server_status"},
	}
	for _, tc := range tests {
		t.Run(tc.surface, func(t *testing.T) {
			call, err := callFor(tc.surface)
			if err != nil {
				t.Fatalf("callFor(%s): %v", tc.surface, err)
			}
			if call.Name != tc.wantName {
				t.Errorf("tool = %q, want %q", call.Name, tc.wantName)
			}
			if call.Detail == "" {
				t.Error("the call has no detail, so the published table would not say what was called")
			}
			if tc.wantArg != "" {
				if _, ok := call.Args[tc.wantArg]; !ok {
					t.Errorf("arguments %v do not carry %q", call.Args, tc.wantArg)
				}
			}
		})
	}

	if _, err := callFor("nonsense"); err == nil {
		t.Error("callFor accepted a surface that does not exist")
	}
}

// TestScenarioPlan_Describe_SaysEveryKnob verifies the progress line names
// what the scenario is, since a long run's output is the only record of which
// configuration produced which numbers while it is still running.
func TestScenarioPlan_Describe_SaysEveryKnob(t *testing.T) {
	plan := scenarioPlan{
		ID: "http-individual", Transport: transportHTTP, Surface: surfaceIndividual,
		Telemetry: true, Clients: 8, Parallel: 2, Rounds: 3,
	}
	got := plan.describe()
	for _, want := range []string{"http", "individual", "8 clients", "2 parallel", "telemetry on"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(got, want) {
				t.Errorf("describe() = %q, missing %q", got, want)
			}
		})
	}
	off := scenarioPlan{Transport: transportStdio, Surface: surfaceDynamic, Clients: 1, Parallel: 1}
	if !strings.Contains(off.describe(), "telemetry off") {
		t.Errorf("describe() = %q, missing telemetry off", off.describe())
	}

	series := seriesPlan(surfaceMeta, 4, matrixSettings{steps: []int{1, 2, 5, 10}, stepDuration: 10 * time.Second})
	for _, want := range []string{"http", "meta", "4 parallel", "4 credential counts", "from 1 to 10", "10s per step"} {
		t.Run("series "+want, func(t *testing.T) {
			if !strings.Contains(series.describe(), want) {
				t.Errorf("describe() = %q, missing %q", series.describe(), want)
			}
		})
	}
}

// idsOf lists scenario ids in order.
func idsOf(plans []scenarioPlan) []string {
	out := make([]string, 0, len(plans))
	for _, plan := range plans {
		out = append(out, plan.ID)
	}
	return out
}
