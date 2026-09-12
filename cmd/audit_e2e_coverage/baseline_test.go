package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// TestJoinBaselineResults_UnjudgedShards_TakeTheStreamBeside verifies the
// join the old suite's baseline needs: its shards carry no verdict, so the
// gotestsum stream recorded beside the directory, at <dir>.results.json, is
// what gives its calls one. Without it every baseline call classified as
// failed and the superset check compared against nothing, which is how the
// S09 baseline passed every run vacuously.
func TestJoinBaselineResults_UnjudgedShards_TakeTheStreamBeside(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "baseline-ce")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	rt := &runtimeRecords{dir: dir, key: "community/free", packages: map[*e2ecalls.Call]string{}}
	passed := fixtureCall(callSpec{test: "TestOld_Passed", action: "issue.list", dispatched: "issue.list", shape: dynamicDefault})
	failed := fixtureCall(callSpec{test: "TestOld_Failed", action: "issue.create", dispatched: "issue.create", shape: metaDefault})
	// The old suite's recorder wrote no verdict, which the fixture builder
	// cannot spell since it fills an empty one in.
	passed.TestStatus, failed.TestStatus = "", ""
	rt.calls = []*e2ecalls.Call{passed, failed}
	rt.packages[passed], rt.packages[failed] = "suite", "suite"

	writeFile(t, dir+baselineResultsSuffix, strings.Join([]string{
		`{"Time":"2026-09-01T10:00:00Z","Action":"run","Package":"example.com/old/test/e2e/suite","Test":"TestOld_Passed"}`,
		`{"Time":"2026-09-01T10:00:01Z","Action":"pass","Package":"example.com/old/test/e2e/suite","Test":"TestOld_Passed","Elapsed":1}`,
		`{"Time":"2026-09-01T10:00:02Z","Action":"run","Package":"example.com/old/test/e2e/suite","Test":"TestOld_Failed"}`,
		`{"Time":"2026-09-01T10:00:03Z","Action":"fail","Package":"example.com/old/test/e2e/suite","Test":"TestOld_Failed","Elapsed":1}`,
		"",
	}, "\n"))

	if err := joinBaselineResults([]*runtimeRecords{rt}); err != nil {
		t.Fatalf("joinBaselineResults() error = %v, want the stream beside the directory joined", err)
	}
	if passed.TestStatus != e2ecalls.StatusPassed || failed.TestStatus != e2ecalls.StatusFailed {
		t.Errorf("verdicts after the join = (%q, %q), want (passed, failed)", passed.TestStatus, failed.TestStatus)
	}
	reached := reachedSet(classify(rt, fixtureCatalog()))
	if !reached[reachedKey{surface: "dynamic", mode: modeDefault, action: "issue.list", credit: creditAsserted}] {
		t.Errorf("the passed baseline call is not reached after the join: %v", reached)
	}
	if reached[reachedKey{surface: "meta", mode: modeDefault, action: "issue.create", credit: creditAsserted}] {
		t.Errorf("the failed baseline call is reached after the join: %v", reached)
	}
}

// TestJoinBaselineResults_UnjudgedShardsWithoutAStream_Refused verifies that
// a baseline that could only reach nothing is refused, naming the file it
// looked for, instead of being compared against.
func TestJoinBaselineResults_UnjudgedShardsWithoutAStream_Refused(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "baseline-ee")
	call := fixtureCall(callSpec{test: "TestOld", action: "issue.list", dispatched: "issue.list", shape: dynamicDefault})
	call.TestStatus = ""
	rt := &runtimeRecords{dir: dir, key: "enterprise/ultimate", calls: []*e2ecalls.Call{call}, packages: map[*e2ecalls.Call]string{call: "suite"}}

	err := joinBaselineResults([]*runtimeRecords{rt})
	if !errors.Is(err, errBaselineUnjudged) {
		t.Fatalf("joinBaselineResults() error = %v, want %v", err, errBaselineUnjudged)
	}
	if !strings.Contains(err.Error(), dir+baselineResultsSuffix) {
		t.Errorf("the refusal does not name the stream it looked for: %v", err)
	}
}

// TestJoinBaselineResults_UnreadableStream_IsItsOwnError verifies that a
// stream which is there and cannot be read is reported as that, and not as
// a missing one: the two are different things to fix.
func TestJoinBaselineResults_UnreadableStream_IsItsOwnError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "baseline-ee")
	// A directory where the stream should be opens and cannot be parsed.
	if err := os.MkdirAll(dir+baselineResultsSuffix, 0o750); err != nil {
		t.Fatal(err)
	}
	call := fixtureCall(callSpec{test: "TestOld", action: "issue.list", dispatched: "issue.list", shape: dynamicDefault})
	call.TestStatus = ""
	rt := &runtimeRecords{dir: dir, key: "enterprise/ultimate", calls: []*e2ecalls.Call{call}, packages: map[*e2ecalls.Call]string{call: "suite"}}

	err := joinBaselineResults([]*runtimeRecords{rt})
	if err == nil || errors.Is(err, errBaselineUnjudged) {
		t.Fatalf("joinBaselineResults() error = %v, want a read error that is not the missing-stream refusal", err)
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("the error does not name the baseline: %v", err)
	}
}

// TestJoinBaselineResults_JudgedShards_LeftAlone verifies that a baseline
// the harness wrote, whose calls carry their verdicts, is not made to look
// for a stream it does not need.
func TestJoinBaselineResults_JudgedShards_LeftAlone(t *testing.T) {
	rt := fixtureRuntime()
	rt.dir = filepath.Join(t.TempDir(), "nowhere")
	if err := joinBaselineResults([]*runtimeRecords{rt}); err != nil {
		t.Errorf("joinBaselineResults() error = %v on shards that carry verdicts, want nil", err)
	}
}

// TestCompareBaseline_WeakerCredits_MetByStrongerOnes verifies the one
// ordering the comparison admits: a cleanup or sweep credit of the old
// suite is met by the same cell asserted in the new one, since all three say
// the action ran and answered, while a refusal, an error path or a preview
// is met by nothing but itself.
func TestCompareBaseline_WeakerCredits_MetByStrongerOnes(t *testing.T) {
	current := classify(fixtureRuntime(), fixtureCatalog())
	// The fixture asserts issue.list on dynamic and previews issue.create on
	// individual in safe mode; it refuses, and never asserts, issue.delete on
	// dynamic.
	cases := []struct {
		name string
		spec callSpec
		lost bool
	}{
		{name: "cleanup met by asserted", spec: callSpec{test: "TestOld", purpose: e2ecalls.PurposeCleanup, expectation: e2ecalls.ExpectationAny, action: "issue.list", dispatched: "issue.list", shape: dynamicDefault}},
		{name: "sweep met by asserted", spec: callSpec{test: "TestOld", purpose: e2ecalls.PurposeSweep, action: "issue.list", dispatched: "issue.list", shape: dynamicDefault}},
		{name: "cleanup met by sweep", spec: callSpec{test: "TestOld", purpose: e2ecalls.PurposeCleanup, expectation: e2ecalls.ExpectationAny, action: "issue.list", dispatched: "issue.list", shape: individualDefault}},
		{name: "asserted not met by cleanup", spec: callSpec{test: "TestOld", action: "issue.delete", dispatched: "issue.delete", shape: metaDefault}, lost: true},
		{name: "asserted not met by a refusal", spec: callSpec{test: "TestOld", action: "issue.delete", dispatched: "issue.delete", shape: dynamicDefault}, lost: true},
		{name: "refusal not met by asserted", spec: callSpec{test: "TestOld", expectation: "needs_confirmation", action: "issue.list", dispatched: "issue.list", outcome: e2ecalls.RefusedOutcome("needs_confirmation"), shape: dynamicDefault}, lost: true},
		{name: "error path not met by a preview", spec: callSpec{test: "TestOld", expectation: "tool_error", action: "issue.create", dispatched: "issue.create", outcome: e2ecalls.OutcomeToolError, shape: individualSafe}, lost: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baselineRT := fixtureRuntime()
			baselineRT.calls = []*e2ecalls.Call{fixtureCall(tc.spec)}
			result := compareBaseline(current, classify(baselineRT, fixtureCatalog()), "old")
			if result.BaselineReached != 1 {
				t.Fatalf("the baseline reached %d keys, want the one the case names", result.BaselineReached)
			}
			if got := len(result.Lost) == 1; got != tc.lost {
				t.Errorf("lost = %q, want lost=%t", result.Lost, tc.lost)
			}
		})
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
