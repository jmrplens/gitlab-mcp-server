package main

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// resultsFixture is the committed go test -json stream.
func resultsFixture() string {
	return filepath.Join("testdata", "results", "e2e-log.json")
}

// commonPath is the import path of the fixture stream's one package.
const commonPath = "example.com/e2efake/test/e2e/gitlab/common"

// commonResult builds a verdict of the fixture stream's package.
func commonResult(status string, elapsed float64) testResult {
	return testResult{Status: status, Elapsed: elapsed, Package: commonPath}
}

// TestReadResults_Stream_Verdicts verifies that the stream is reduced to one
// verdict per package and test, subtests included, with the elapsed time
// kept, and that output and package-level events are passed over.
func TestReadResults_Stream_Verdicts(t *testing.T) {
	results, err := readResults(resultsFixture())
	if err != nil {
		t.Fatalf("readResults() error = %v", err)
	}
	want := testResults{
		{pkg: "common", test: "TestIssue_List"}:                 commonResult(e2ecalls.StatusPassed, 1.25),
		{pkg: "common", test: "TestIssue_Delete"}:               commonResult(e2ecalls.StatusPassed, 0.5),
		{pkg: "common", test: "TestProject_Get/dynamic"}:        commonResult(e2ecalls.StatusFailed, 0.1),
		{pkg: "common", test: "TestProject_Get"}:                commonResult(e2ecalls.StatusFailed, 0.2),
		{pkg: "common", test: "TestResources_Read"}:             commonResult(e2ecalls.StatusPassed, 0.3),
		{pkg: "common", test: "TestPipeline_Create"}:            commonResult(e2ecalls.StatusSkipped, 0),
		{pkg: "common", test: "TestMR_Approvals/ApprovalState"}: commonResult(e2ecalls.StatusPassed, 0),
		{pkg: "common", test: "TestMR_Approvals"}:               commonResult(e2ecalls.StatusPassed, 0),
	}
	if !reflect.DeepEqual(results, want) {
		t.Errorf("readResults() = %+v, want %+v", results, want)
	}
}

// TestParseResults_BadStreams_Refused verifies the two refusals: a line that is not
// JSON, and a stream with no verdict at all.
func TestParseResults_BadStreams_Refused(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr error
		wantMsg string
	}{
		{name: "not json", input: "{\"Action\":\"pass\",\"Test\":\"T\"}\nnot json\n", wantMsg: "results line 2"},
		{name: "no verdict", input: "{\"Action\":\"output\",\"Test\":\"T\"}\n\n", wantErr: errNoVerdict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseResults(strings.NewReader(tc.input))
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("parseResults() error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantMsg != "" && (err == nil || !strings.Contains(err.Error(), tc.wantMsg)) {
				t.Errorf("parseResults() error = %v, want one mentioning %q", err, tc.wantMsg)
			}
		})
	}
	if _, err := readResults(filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Error("readResults() accepted a file that does not exist")
	}
}

// TestJoinResults_Stream_FillsOverridesAndListsIdleTests verifies the join: a call
// without a status takes the stream's, a subtest falls back to its parent, a
// call whose own status the stream contradicts is overridden and counted, a
// test the stream never saw is counted unmatched, and a test that passed
// while calling nothing is listed with its duration.
func TestJoinResults_Stream_FillsOverridesAndListsIdleTests(t *testing.T) {
	results, err := readResults(resultsFixture())
	if err != nil {
		t.Fatalf("readResults() error = %v", err)
	}
	rt := &runtimeRecords{calls: []*e2ecalls.Call{
		{Test: "TestIssue_List"},
		{Test: "TestProject_Get/dynamic/individual"},
		{Test: "TestIssue_Delete", TestStatus: e2ecalls.StatusFailed},
		{Test: "TestUnknown", TestStatus: e2ecalls.StatusPassed},
		{Test: "TestResources_Read", TestStatus: e2ecalls.StatusPassed},
	}}
	join := joinResults(rt, results)

	if rt.calls[0].TestStatus != e2ecalls.StatusPassed {
		t.Errorf("call without status = %q, want passed from the stream", rt.calls[0].TestStatus)
	}
	if rt.calls[1].TestStatus != e2ecalls.StatusFailed {
		t.Errorf("subtest without a verdict = %q, want its parent's failed", rt.calls[1].TestStatus)
	}
	if rt.calls[2].TestStatus != e2ecalls.StatusPassed {
		t.Errorf("contradicted status = %q, want the stream's passed", rt.calls[2].TestStatus)
	}
	want := resultsJoin{
		Tests: 8, Filled: 2, Overridden: 1, Unmatched: 1,
		TestsWithoutCalls: []idleTest{
			{Package: "common", Test: "TestMR_Approvals", Elapsed: 0},
			{Package: "common", Test: "TestMR_Approvals/ApprovalState", Elapsed: 0},
		},
	}
	if !reflect.DeepEqual(join, want) {
		t.Errorf("joinResults() = %+v, want %+v", join, want)
	}
}

// TestJoinResults_SameTestNameInTwoPackages_KeptApart verifies that a
// verdict is looked up in the package the call's shard names: with one
// TestIssue_List passing in common and another failing in ee, each package's
// call takes its own package's verdict, and neither is listed as a test
// without calls. A call whose package the stream never reported falls back
// to the name, and only where one package reports it.
func TestJoinResults_SameTestNameInTwoPackages_KeptApart(t *testing.T) {
	results, err := parseResults(strings.NewReader(strings.Join([]string{
		`{"Action":"pass","Package":"example.com/m/test/e2e/gitlab/common","Test":"TestIssue_List","Elapsed":1}`,
		`{"Action":"fail","Package":"example.com/m/test/e2e/gitlab/ee","Test":"TestIssue_List","Elapsed":2}`,
		`{"Action":"pass","Package":"example.com/m/test/e2e/gitlab/ee","Test":"TestEpic_List","Elapsed":3}`,
		`{"Action":"pass","Package":"example.com/m/test/e2e/gitlab/ce","Test":"TestOnlyHere","Elapsed":4}`,
	}, "\n")))
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	commonCall := &e2ecalls.Call{Test: "TestIssue_List"}
	eeCall := &e2ecalls.Call{Test: "TestIssue_List/sub"}
	unknownAmbiguous := &e2ecalls.Call{Test: "TestIssue_List"}
	unknownUnique := &e2ecalls.Call{Test: "TestEpic_List"}
	unreported := &e2ecalls.Call{Test: "TestOnlyHere"}
	rt := &runtimeRecords{
		calls: []*e2ecalls.Call{commonCall, eeCall, unknownAmbiguous, unknownUnique, unreported},
		packages: map[*e2ecalls.Call]string{
			commonCall: "common", eeCall: "ee", unreported: "suite",
		},
	}
	join := joinResults(rt, results)

	cases := []struct {
		name string
		call *e2ecalls.Call
		want string
	}{
		{name: "common takes common's pass", call: commonCall, want: e2ecalls.StatusPassed},
		{name: "ee's subtest takes ee's fail", call: eeCall, want: e2ecalls.StatusFailed},
		{name: "an unknown package is not matched where two packages report the name", call: unknownAmbiguous, want: ""},
		{name: "an unknown package is matched where one package reports the name", call: unknownUnique, want: e2ecalls.StatusPassed},
		{name: "a package the stream never reported is matched by name", call: unreported, want: e2ecalls.StatusPassed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.call.TestStatus != tc.want {
				t.Errorf("status = %q, want %q", tc.call.TestStatus, tc.want)
			}
		})
	}
	want := resultsJoin{Tests: 4, Filled: 4, Unmatched: 1}
	if !reflect.DeepEqual(join, want) {
		t.Errorf("joinResults() = %+v, want %+v: every passing test had a call", join, want)
	}
}

// TestJoinResults_Ancestors_AllMarkedCalled verifies that a call made from a
// nested subtest counts for every test above it: a passed middle subtest
// whose child called is not a test without calls, and neither is the Test
// function.
func TestJoinResults_Ancestors_AllMarkedCalled(t *testing.T) {
	results, err := parseResults(strings.NewReader(strings.Join([]string{
		`{"Action":"pass","Package":"example.com/m/test/e2e/gitlab/common","Test":"TestA","Elapsed":1}`,
		`{"Action":"pass","Package":"example.com/m/test/e2e/gitlab/common","Test":"TestA/parent","Elapsed":1}`,
		`{"Action":"pass","Package":"example.com/m/test/e2e/gitlab/common","Test":"TestA/parent/child","Elapsed":1}`,
		`{"Action":"pass","Package":"example.com/m/test/e2e/gitlab/common","Test":"TestA/other","Elapsed":0}`,
	}, "\n")))
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	call := &e2ecalls.Call{Test: "TestA/parent/child"}
	rt := &runtimeRecords{calls: []*e2ecalls.Call{call}, packages: map[*e2ecalls.Call]string{call: "common"}}
	join := joinResults(rt, results)

	want := []idleTest{{Package: "common", Test: "TestA/other", Elapsed: 0}}
	if !reflect.DeepEqual(join.TestsWithoutCalls, want) {
		t.Errorf("TestsWithoutCalls = %+v, want only the sibling that called nothing", join.TestsWithoutCalls)
	}
}

// TestAncestorTests_Names_WalkedUp verifies the name walk the ancestor marking
// rests on: every prefix up to the Test function, the name itself first.
func TestAncestorTests_Names_WalkedUp(t *testing.T) {
	cases := []struct {
		name string
		want []string
	}{
		{name: "TestA", want: []string{"TestA"}},
		{name: "TestA/b", want: []string{"TestA/b", "TestA"}},
		{name: "TestA/b/c", want: []string{"TestA/b/c", "TestA/b", "TestA"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ancestorTests(tc.name); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ancestorTests(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestPackageName_ImportPaths_Reduced verifies the reduction of a stream's
// import path to the name a run line carries, and that no path stays empty.
func TestPackageName_ImportPaths_Reduced(t *testing.T) {
	cases := []struct {
		importPath string
		want       string
	}{
		{importPath: "example.com/m/test/e2e/gitlab/common", want: "common"},
		{importPath: "example.com/m/test/e2e/suite", want: "suite"},
		{importPath: "single", want: "single"},
		{importPath: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.importPath, func(t *testing.T) {
			if got := packageName(tc.importPath); got != tc.want {
				t.Errorf("packageName(%q) = %q, want %q", tc.importPath, got, tc.want)
			}
		})
	}
}

// TestVerdictStatus_Actions_Mapped verifies which go test actions are verdicts.
func TestVerdictStatus_Actions_Mapped(t *testing.T) {
	cases := []struct {
		action  string
		want    string
		verdict bool
	}{
		{action: "pass", want: e2ecalls.StatusPassed, verdict: true},
		{action: "fail", want: e2ecalls.StatusFailed, verdict: true},
		{action: "skip", want: e2ecalls.StatusSkipped, verdict: true},
		{action: "run", verdict: false},
		{action: "output", verdict: false},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			got, verdict := verdictStatus(tc.action)
			if got != tc.want || verdict != tc.verdict {
				t.Errorf("verdictStatus(%q) = (%q, %t), want (%q, %t)", tc.action, got, verdict, tc.want, tc.verdict)
			}
		})
	}
}
