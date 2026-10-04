package main

import (
	"reflect"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// fineGrainedRuntime is the fixture runtime with one fine-grained session on
// dynamic and one on individual beside the classic ones, and calls of every
// kind the fine-grained fold tells apart.
func fineGrainedRuntime() *runtimeRecords {
	rt := fixtureRuntime()
	dynamic := fixtureSession(dynamicDefault, true)
	dynamic.Label += "/fine-grained"
	dynamic.Credential = e2ecalls.CredentialFineGrained
	dynamic.Tools = []string{"gitlab_execute_action", "gitlab_find_action"}
	individual := fixtureSession(individualDefault, true)
	individual.Label += "/fine-grained"
	individual.Credential = e2ecalls.CredentialFineGrained
	individual.Tools = []string{"gitlab_issue_list", "gitlab_project_get"}
	// A second session of the same shape listing one more tool: the shape row
	// counts both and lists the union.
	again := fixtureSession(individualDefault, true)
	again.Credential = e2ecalls.CredentialFineGrained
	again.Tools = []string{"gitlab_issue_list", "gitlab_issue_create"}
	rt.sessions = append(rt.sessions, dynamic, individual, again)

	specs := []callSpec{
		{test: "TestFineGrained_Reads", action: "issue.list", dispatched: "issue.list", shape: dynamicDefault},
		{test: "TestFineGrained_Reads", action: "issue.list", dispatched: "issue.list", shape: dynamicDefault},
		{
			test: "TestFineGrained_Writes", expectation: toolutil.RefusalFineGrained, action: "issue.create",
			outcome: e2ecalls.RefusedOutcome(toolutil.RefusalFineGrained), shape: dynamicDefault,
		},
		{
			test: "TestFineGrained_Writes", expectation: "unknown_action", action: "issue.create",
			outcome: e2ecalls.RefusedOutcome(toolutil.RefusalUnknownAction), shape: dynamicDefault,
		},
		{
			test: "TestFineGrained_Outside", expectation: "tool_error", action: "project.get", dispatched: "project.get",
			outcome: e2ecalls.OutcomeToolError, shape: individualDefault,
		},
		{test: "TestFineGrained_Failing", action: "project.list", dispatched: "project.list", status: e2ecalls.StatusFailed, shape: individualDefault},
		{test: "TestFineGrained_Unobserved", action: "issue.list", shape: individualDefault},
		{test: "TestFineGrained_Raw", purpose: e2ecalls.PurposeRaw, expectation: e2ecalls.ExpectationAny, shape: individualDefault},
		{test: "TestFineGrained_Manifest", method: methodReadResource, target: "gitlab://tools/issue.list", shape: dynamicDefault},
	}
	for _, spec := range specs {
		call := fixtureCall(spec)
		call.Credential = e2ecalls.CredentialFineGrained
		rt.calls = append(rt.calls, call)
	}
	return rt
}

// TestClassify_FineGrainedLines_AreKeptOutOfTheClassicFigures holds the
// separation the fold exists for: the classic classification of a runtime is
// exactly what it was without its fine-grained lines, and those lines are
// counted in a section of their own.
func TestClassify_FineGrainedLines_AreKeptOutOfTheClassicFigures(t *testing.T) {
	classic := buildReport(classify(fixtureRuntime(), fixtureCatalog()))
	mixed := buildReport(classify(fineGrainedRuntime(), fixtureCatalog()))

	if classic.FineGrained != nil {
		t.Errorf("a runtime with no fine-grained session reported a section: %+v", classic.FineGrained)
	}
	if mixed.FineGrained == nil {
		t.Fatal("the fine-grained lines were counted nowhere")
	}
	fine := mixed.FineGrained
	mixed.FineGrained = nil
	if !reflect.DeepEqual(classic, mixed) {
		t.Error("the fine-grained lines changed the classic report")
	}
	if fine.Sessions != 3 || fine.Calls != 8 {
		t.Errorf("sessions = %d, calls = %d; want 3 sessions and the 8 tool calls", fine.Sessions, fine.Calls)
	}
	wantShapes := []fineGrainedShapeRow{
		{Surface: dynamicDefault.surface, Mode: dynamicDefault.mode, Sessions: 1, Tools: 2},
		{Surface: individualDefault.surface, Mode: individualDefault.mode, Sessions: 2, Tools: 3},
	}
	if !reflect.DeepEqual(fine.Shapes, wantShapes) {
		t.Errorf("shapes = %+v, want %+v", fine.Shapes, wantShapes)
	}
}

// TestClassify_FineGrainedCells_CountHowEachCallEnded walks the cells: a call
// a passing test saw run, a refusal under each reason, GitLab's own error a
// test wanted, and the calls that say nothing about the action (a failing
// test, a span that never named the action), with the two counters a floor
// reads.
func TestClassify_FineGrainedCells_CountHowEachCallEnded(t *testing.T) {
	fine := buildReport(classify(fineGrainedRuntime(), fixtureCatalog())).FineGrained

	want := []fineGrainedCellRow{
		{
			Surface: dynamicDefault.surface, Mode: dynamicDefault.mode, Action: "issue.create",
			Refused: map[string]int{toolutil.RefusalFineGrained: 1, toolutil.RefusalUnknownAction: 1}, Tests: []string{"TestFineGrained_Writes"},
		},
		{Surface: dynamicDefault.surface, Mode: dynamicDefault.mode, Action: "issue.list", Asserted: 2, Tests: []string{"TestFineGrained_Reads"}},
		{Surface: individualDefault.surface, Mode: individualDefault.mode, Action: "issue.list", Unjudged: 1, Tests: []string{"TestFineGrained_Unobserved"}},
		{Surface: individualDefault.surface, Mode: individualDefault.mode, Action: "project.get", ErrorPath: 1, Tests: []string{"TestFineGrained_Outside"}},
		{Surface: individualDefault.surface, Mode: individualDefault.mode, Action: "project.list", Unjudged: 1, Tests: []string{"TestFineGrained_Failing"}},
	}
	if !reflect.DeepEqual(fine.Cells, want) {
		t.Errorf("cells = %+v\nwant %+v", fine.Cells, want)
	}
	if fine.Asserted != 1 || fine.Refused != 1 {
		t.Errorf("asserted = %d, refused = %d; want one cell each", fine.Asserted, fine.Refused)
	}
}

// TestIsFineGrained_OnlyTheFineGrainedKind verifies a line with no kind, the
// shape of every line written before the harness recorded one, is classic.
func TestIsFineGrained_OnlyTheFineGrainedKind(t *testing.T) {
	cases := map[string]bool{"": false, e2ecalls.CredentialClassic: false, e2ecalls.CredentialFineGrained: true, "other": false}
	for kind, want := range cases {
		t.Run(kind, func(t *testing.T) {
			if got := isFineGrained(kind); got != want {
				t.Errorf("isFineGrained(%q) = %t, want %t", kind, got, want)
			}
		})
	}
}

// TestCheckFineGrained_Floors_Applied verifies the floors under every
// selector the runtime matches: none declared, a run with no fine-grained
// session, counts below each floor, and a run that clears them.
func TestCheckFineGrained_Floors_Applied(t *testing.T) {
	recorded := fineGrainedFloors
	t.Cleanup(func() { fineGrainedFloors = recorded })
	fineGrainedFloors = map[string]fineGrainedFloor{"ce": {Asserted: 4, Refused: 2}, "community/free": {Asserted: 1, Refused: 1}}

	cases := []struct {
		name    string
		runtime string
		fine    *fineGrainedReport
		want    []string
	}{
		{name: "no floor declared", runtime: "enterprise/ultimate", want: nil},
		{name: "no fine-grained session", runtime: "community/free", want: []string{
			"no session on a fine-grained token ran on community/free, and community/free records a floor for them",
			"no session on a fine-grained token ran on community/free, and ce records a floor for them",
		}},
		{name: "below the floors", runtime: "community/free", fine: &fineGrainedReport{Asserted: 3, Refused: 1}, want: []string{
			"3 actions run by a fine-grained session on community/free, below the floor of 4 recorded for ce",
			"1 actions refused to a fine-grained session for its credential on community/free, below the floor of 2 recorded for ce",
		}},
		{name: "passes", runtime: "community/free", fine: &fineGrainedReport{Asserted: 4, Refused: 2}, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := &checkResult{Passed: true}
			checkFineGrained(&report{Runtime: tc.runtime, FineGrained: tc.fine}, []string{"ce"}, result)
			if !reflect.DeepEqual(result.Findings, tc.want) || result.Passed != (len(tc.want) == 0) {
				t.Errorf("checkFineGrained() = passed %t, findings %q; want %q", result.Passed, result.Findings, tc.want)
			}
		})
	}
}
