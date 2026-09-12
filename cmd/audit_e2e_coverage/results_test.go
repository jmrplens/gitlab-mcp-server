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

// TestReadResults_Stream_Verdicts verifies that the stream is reduced to one
// verdict per test, subtests included, with the elapsed time kept, and that
// output and package-level events are passed over.
func TestReadResults_Stream_Verdicts(t *testing.T) {
	results, err := readResults(resultsFixture())
	if err != nil {
		t.Fatalf("readResults() error = %v", err)
	}
	want := testResults{
		"TestIssue_List":                 {Status: e2ecalls.StatusPassed, Elapsed: 1.25, Package: "example.com/e2efake/test/e2e/gitlab/common"},
		"TestIssue_Delete":               {Status: e2ecalls.StatusPassed, Elapsed: 0.5, Package: "example.com/e2efake/test/e2e/gitlab/common"},
		"TestProject_Get/dynamic":        {Status: e2ecalls.StatusFailed, Elapsed: 0.1, Package: "example.com/e2efake/test/e2e/gitlab/common"},
		"TestProject_Get":                {Status: e2ecalls.StatusFailed, Elapsed: 0.2, Package: "example.com/e2efake/test/e2e/gitlab/common"},
		"TestResources_Read":             {Status: e2ecalls.StatusPassed, Elapsed: 0.3, Package: "example.com/e2efake/test/e2e/gitlab/common"},
		"TestPipeline_Create":            {Status: e2ecalls.StatusSkipped, Package: "example.com/e2efake/test/e2e/gitlab/common"},
		"TestMR_Approvals/ApprovalState": {Status: e2ecalls.StatusPassed, Package: "example.com/e2efake/test/e2e/gitlab/common"},
		"TestMR_Approvals":               {Status: e2ecalls.StatusPassed, Package: "example.com/e2efake/test/e2e/gitlab/common"},
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
			{Test: "TestMR_Approvals", Elapsed: 0},
			{Test: "TestMR_Approvals/ApprovalState", Elapsed: 0},
		},
	}
	if !reflect.DeepEqual(join, want) {
		t.Errorf("joinResults() = %+v, want %+v", join, want)
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
