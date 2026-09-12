package main

import (
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// TestReachedSet_PassingTests_CreditsOnly verifies what the superset check
// compares: every credit a passing test earned, one key per credit, with
// failed, skipped and unobserved left out.
func TestReachedSet_PassingTests_CreditsOnly(t *testing.T) {
	c := classify(fixtureRuntime(), fixtureCatalog())
	reached := reachedSet(c)

	cases := []struct {
		name string
		key  reachedKey
		want bool
	}{
		{name: "asserted", key: reachedKey{surface: "dynamic", mode: modeDefault, action: "issue.list", credit: creditAsserted}, want: true},
		{name: "sweep", key: reachedKey{surface: "individual", mode: modeDefault, action: "issue.list", credit: creditSweep}, want: true},
		{name: "error path", key: reachedKey{surface: "dynamic", mode: modeDefault, action: "issue.create", credit: creditErrorPath}, want: true},
		{name: "refused", key: reachedKey{surface: "dynamic", mode: modeDefault, action: "issue.delete", credit: creditRefused}, want: true},
		{name: "cleanup", key: reachedKey{surface: "meta", mode: modeDefault, action: "issue.delete", credit: creditCleanup}, want: true},
		{name: "preview", key: reachedKey{surface: "individual", mode: modeSafe, action: "issue.create", credit: creditPreview}, want: true},
		{name: "unobserved is not reached", key: reachedKey{surface: "meta", mode: modeDefault, action: "issue.list", credit: creditUnobserved}, want: false},
		{name: "failed is not reached", key: reachedKey{surface: "dynamic", mode: modeDefault, action: "project.get", credit: creditFailed}, want: false},
		{name: "skipped is not reached", key: reachedKey{surface: "meta", mode: modeDefault, action: "project.get", credit: creditSkipped}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reached[tc.key]; got != tc.want {
				t.Errorf("reached[%s] = %t, want %t", tc.key, got, tc.want)
			}
		})
	}
}

// TestCompareBaseline_DifferentRuns_LossesAndGains verifies the comparison both ways: a
// baseline credit the new run lacks is a loss, spelled with all four parts,
// and a new credit the baseline lacks is counted as gained.
func TestCompareBaseline_DifferentRuns_LossesAndGains(t *testing.T) {
	current := classify(fixtureRuntime(), fixtureCatalog())

	baselineRT := fixtureRuntime()
	baselineRT.calls = []*e2ecalls.Call{
		fixtureCall(callSpec{test: "TestOld", action: "issue.list", dispatched: "issue.list", shape: dynamicDefault}),
		fixtureCall(callSpec{test: "TestOld", action: "issue.create", dispatched: "issue.create", shape: metaDefault}),
	}
	baseline := classify(baselineRT, fixtureCatalog())

	result := compareBaseline(current, baseline, "old-dir")
	if result.BaselineDirectory != "old-dir" || result.BaselineReached != 2 {
		t.Errorf("baseline = %s with %d reached, want old-dir with 2", result.BaselineDirectory, result.BaselineReached)
	}
	if want := []string{"meta/default issue.create asserted"}; !reflect.DeepEqual(result.Lost, want) {
		t.Errorf("Lost = %q, want %q", result.Lost, want)
	}
	if result.Gained != result.Reached-1 {
		t.Errorf("Gained = %d, want every reached key but the shared one (%d)", result.Gained, result.Reached-1)
	}
}

// TestCompareBaseline_Superset_NoLoss verifies the passing case: a run that
// reaches everything the baseline did loses nothing.
func TestCompareBaseline_Superset_NoLoss(t *testing.T) {
	current := classify(fixtureRuntime(), fixtureCatalog())
	result := compareBaseline(current, current, "same")
	if len(result.Lost) != 0 || result.Gained != 0 {
		t.Errorf("compareBaseline(self) lost %q and gained %d, want nothing", result.Lost, result.Gained)
	}
}
