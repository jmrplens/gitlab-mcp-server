// matrix_test.go covers what gets measured: the published matrix's shape, the
// filter that selects part of it, and the per-surface tool call, whose name
// differs per surface and must exist on the surface it is used with.
package main

import (
	"strings"
	"testing"
)

// TestPublishedMatrix_CoversEveryAxis verifies the committed matrix exercises
// each axis the issue names: both transports, all three surfaces, telemetry
// both ways, and more than one client. A matrix that quietly lost an axis
// would publish a page that answers less than it claims to.
func TestPublishedMatrix_CoversEveryAxis(t *testing.T) {
	plans := publishedMatrix(3)

	transports := map[string]bool{}
	surfaces := map[string]bool{}
	telemetry := map[bool]bool{}
	for _, plan := range plans {
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
	for _, plan := range publishedMatrix(1) {
		if seen[plan.ID] {
			t.Errorf("duplicate scenario id %q", plan.ID)
		}
		seen[plan.ID] = true
	}
}

// TestQuickMatrix_IsSmallerAndStillTwoTransports verifies the smoke matrix is
// short enough to be worth running while still driving both transports, which
// is the only reason to have one.
func TestQuickMatrix_IsSmallerAndStillTwoTransports(t *testing.T) {
	quick := quickMatrix()
	if len(quick) >= len(publishedMatrix(3)) {
		t.Error("the quick matrix is not smaller than the published one")
	}
	transports := map[string]bool{}
	for _, plan := range quick {
		transports[plan.Transport] = true
	}
	if len(transports) != 2 {
		t.Errorf("the quick matrix drives %d transports, want both", len(transports))
	}
}

// TestSelectPlans_Filters verifies the scenario filter keeps matrix order,
// accepts an empty filter as "everything", and refuses a name that matches
// nothing rather than silently measuring a smaller matrix.
func TestSelectPlans_Filters(t *testing.T) {
	plans := publishedMatrix(1)

	tests := []struct {
		name    string
		filter  string
		wantIDs []string
		wantErr bool
	}{
		{name: "empty keeps all", filter: "", wantIDs: idsOf(plans)},
		{name: "one", filter: "http-meta", wantIDs: []string{"http-meta"}},
		{
			name:    "several keep matrix order",
			filter:  "http-dynamic, stdio-dynamic",
			wantIDs: []string{"stdio-dynamic", "http-dynamic"},
		},
		{name: "typo", filter: "http-dinamic", wantErr: true},
		{name: "one good one bad", filter: "http-meta,nope", wantErr: true},
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
}

// idsOf lists scenario ids in order.
func idsOf(plans []scenarioPlan) []string {
	out := make([]string, 0, len(plans))
	for _, plan := range plans {
		out = append(out, plan.ID)
	}
	return out
}
