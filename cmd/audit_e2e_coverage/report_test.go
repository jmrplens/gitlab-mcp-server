package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// fixtureReport is the report over the fixture runtime.
func fixtureReport() *report {
	return buildReport(classify(fixtureRuntime(), fixtureCatalog()))
}

// TestBuildReport_Fixture_LevelsAndRows verifies the headline: the levels read off
// the default-mode cells, the action rows carrying one state per surface,
// the session rows, and the absent cells left out of the cell list.
func TestBuildReport_Fixture_LevelsAndRows(t *testing.T) {
	rep := fixtureReport()

	if want := (levels{
		L1: []string{"environment.protected_get", "interactive.issue_create", "issue.list"},
		L2: []string{"interactive.issue_create", "issue.list"},
	}); !reflect.DeepEqual(rep.Levels, want) {
		t.Errorf("Levels = %+v, want %+v", rep.Levels, want)
	}
	if rep.Summary.CatalogActions != 12 || rep.Summary.L1 != 3 || rep.Summary.L2 != 2 || rep.Summary.L3 != 0 || rep.Summary.TestCalls != 25 {
		t.Errorf("Summary = %+v, want 12 actions, L1 3, L2 2, L3 0, 25 test calls", rep.Summary)
	}
	var issueList actionRow
	for _, row := range rep.Actions {
		if row.ID == "issue.list" {
			issueList = row
		}
	}
	want := actionRow{ID: "issue.list", Domain: "issue", Tier: "free", L1: true, L2: true, States: map[string]state{
		config.ToolSurfaceDynamic: stateAsserted, config.ToolSurfaceMeta: stateUnobserved, config.ToolSurfaceIndividual: stateSweepOnly,
	}}
	if !reflect.DeepEqual(issueList, want) {
		t.Errorf("issue.list row = %+v, want %+v", issueList, want)
	}
	for _, row := range rep.Cells {
		if row.State == stateAbsent {
			t.Errorf("cell %s/%s %s is absent and was published", row.Surface, row.Mode, row.Target)
		}
	}
	if len(rep.Sessions) != 5 || rep.Sessions[0].Surface != config.ToolSurfaceDynamic || rep.Sessions[1].Mode != modeDefault {
		t.Errorf("Sessions = %+v, want five shapes with dynamic first", rep.Sessions)
	}
	if rep.Summary.States[config.ToolSurfaceDynamic][stateAbsent] != 6 {
		t.Errorf("dynamic absent = %d, want 6", rep.Summary.States[config.ToolSurfaceDynamic][stateAbsent])
	}
}

// TestBuildReport_SubscriptionDelivery_Marked verifies that the subscription
// row a notification reached says so, and the others do not.
func TestBuildReport_SubscriptionDelivery_Marked(t *testing.T) {
	rep := fixtureReport()
	delivered := map[string]bool{}
	for _, row := range rep.Capabilities[capabilitySubscriptions] {
		if row.Surface == config.ToolSurfaceDynamic && row.Mode == modeDefault {
			delivered[row.Target] = row.Delivered
		}
	}
	if !delivered["issue"] || delivered["pipeline"] {
		t.Errorf("delivered = issue %t, pipeline %t; want only the issue kind", delivered["issue"], delivered["pipeline"])
	}
}

// TestBuildReport_UncalledTools_PerShape verifies the list of served tools
// no call named, per shape.
func TestBuildReport_UncalledTools_PerShape(t *testing.T) {
	rep := fixtureReport()
	var individual uncalledRow
	for _, row := range rep.UncalledTools {
		if row.Surface == config.ToolSurfaceIndividual && row.Mode == modeDefault {
			individual = row
		}
	}
	for _, tool := range []string{"gitlab_issue_list", "gitlab_project_get"} {
		t.Run(tool, func(t *testing.T) {
			if slices.Contains(individual.Tools, tool) {
				t.Errorf("%s was called on individual and is listed as uncalled", tool)
			}
		})
	}
	if len(individual.Tools) != len(servedTools)-2 {
		t.Errorf("uncalled on individual = %d tools, want every served tool but the two called", len(individual.Tools))
	}
}

// TestWriteGapTSV_Fixture_WorkList pins the whole work list for the fixture: one row
// per action not asserted on any surface with its state on each surface, and
// the summary line.
func TestWriteGapTSV_Fixture_WorkList(t *testing.T) {
	var out bytes.Buffer
	writeGapTSV(&out, fixtureReport())
	want := strings.Join([]string{
		"admin.list\tadmin\tfree\tdynamic=unservable\tmeta=unservable\tindividual=unservable",
		"environment.get\tenvironment\tfree\tdynamic=absent\tmeta=absent\tindividual=absent",
		"issue.create\tissue\tfree\tdynamic=error-path-only\tmeta=absent\tindividual=absent",
		"issue.delete\tissue\tfree\tdynamic=refused-only\tmeta=cleanup-only\tindividual=absent",
		"merge_train.list\tmerge_train\tpremium\tdynamic=absent\tmeta=absent\tindividual=absent",
		"project.get\tproject\tfree\tdynamic=failed\tmeta=skipped\tindividual=absent",
		"project.list\tproject\tfree\tdynamic=absent\tmeta=absent\tindividual=absent",
		"repository.file_history\trepository\tfree\tdynamic=absent\tmeta=absent\tindividual=unservable",
		"server.health_check\tserver\tfree\tdynamic=absent\tmeta=absent\tindividual=unservable",
		"e2e coverage community/free: L1 3/12 (25.0%), L2 2, L3 0, 9 actions not asserted on any surface",
		"",
	}, "\n")
	if out.String() != want {
		t.Errorf("work list =\n%s\nwant\n%s", out.String(), want)
	}
}

// TestWriteGapTSV_NoSession_Named verifies the spelling of a surface no
// session ran on, which is not the same as absent.
func TestWriteGapTSV_NoSession_Named(t *testing.T) {
	rt := fixtureRuntime()
	rt.sessions = rt.sessions[:1]
	rt.calls = nil
	var out bytes.Buffer
	writeGapTSV(&out, buildReport(classify(rt, fixtureCatalog())))
	if !strings.Contains(out.String(), "issue.list\tissue\tfree\tdynamic=absent\tmeta=no-session\tindividual=no-session\n") {
		t.Errorf("work list does not name the surfaces without a session:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "L1 0/12 (0.0%)") {
		t.Errorf("work list summary is wrong:\n%s", out.String())
	}
}

// TestWriteMarkdownSummary_Fixture_Document pins the whole summary for the fixture
// with a check and a baseline verdict on it, which is what a CI step puts in
// GITHUB_STEP_SUMMARY.
func TestWriteMarkdownSummary_Fixture_Document(t *testing.T) {
	rep := fixtureReport()
	rep.Check = &checkResult{Passed: false, Findings: []string{"no test call was recorded on community/free"}}
	rep.Baseline = &baselineResult{BaselineReached: 4, Reached: 3, Lost: []string{"meta/default issue.create asserted"}}
	var out bytes.Buffer
	writeMarkdownSummary(&out, []*report{rep})
	want := strings.Join([]string{
		"## E2E coverage",
		"",
		"### community/free",
		"",
		"- Catalog actions: 12",
		"- L1 (asserted on any surface): 3 (25.0%)",
		"- L2 (asserted on dynamic): 2",
		"- L3 (asserted on all three surfaces): 0",
		"- Test calls: 25; dispatch mismatches: 1; unresolved tools: 1",
		"",
		"| Surface | asserted | unobserved | sweep-only | error-path-only | refused-only | preview-only | cleanup-only | unasserted | unservable | skipped | failed | absent |",
		"| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |",
		"| dynamic | 2 | 0 | 0 | 1 | 1 | 0 | 0 | 0 | 1 | 0 | 1 | 6 |",
		"| meta | 2 | 1 | 0 | 0 | 0 | 0 | 1 | 0 | 1 | 1 | 0 | 6 |",
		"| individual | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 3 | 0 | 0 | 8 |",
		"",
		"| Capability | asserted | unobserved | sweep-only | error-path-only | refused-only | preview-only | cleanup-only | unasserted | unservable | skipped | failed | absent |",
		"| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |",
		"| completions | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |",
		"| elicitation | 1 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 2 |",
		"| modes | 4 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |",
		"| prompts | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 4 |",
		"| resources | 3 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 17 |",
		"| subscriptions | 2 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 128 |",
		"",
		"- Check: FAILED",
		"  - no test call was recorded on community/free",
		"",
		"- Baseline: FAILED (4 reached in the baseline, 1 lost)",
		"  - meta/default issue.create asserted",
		"",
	}, "\n")
	if out.String() != want {
		t.Errorf("summary =\n%s\nwant\n%s", out.String(), want)
	}
}

// TestWriteJSON_Report_RoundTrips verifies that the JSON report decodes back into
// the same rows, which is the contract a CI step reading the file rests on.
func TestWriteJSON_Report_RoundTrips(t *testing.T) {
	rep := fixtureReport()
	var out bytes.Buffer
	if err := writeJSON(&out, []*report{rep}); err != nil {
		t.Fatalf("writeJSON() error = %v", err)
	}
	var decoded []*report
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded) != 1 || decoded[0].Runtime != rep.Runtime || len(decoded[0].Cells) != len(rep.Cells) {
		t.Errorf("round trip lost rows: %d reports, %d cells", len(decoded), len(decoded[0].Cells))
	}
	if !reflect.DeepEqual(decoded[0].Levels, rep.Levels) {
		t.Errorf("Levels = %+v after the round trip, want %+v", decoded[0].Levels, rep.Levels)
	}
}

// TestWriteJSON_WriterError_Reported verifies that a writer that fails is
// reported rather than swallowed.
func TestWriteJSON_WriterError_Reported(t *testing.T) {
	if err := writeJSON(failingWriter{}, []*report{fixtureReport()}); err == nil {
		t.Error("writeJSON() error = nil on a failing writer")
	}
}

// failingWriter refuses every write.
type failingWriter struct{}

// Write fails.
func (failingWriter) Write([]byte) (int, error) { return 0, errWriteRefused }

// errWriteRefused is what failingWriter answers.
var errWriteRefused = &writeError{}

// writeError is a distinct error type for the failing writer.
type writeError struct{}

// Error spells it.
func (*writeError) Error() string { return "write refused" }

// TestPercent_Whole_ZeroOfNothing verifies the share helper's one edge.
func TestPercent_Whole_ZeroOfNothing(t *testing.T) {
	if got := percent(0, 0); got != 0 {
		t.Errorf("percent(0, 0) = %v, want 0", got)
	}
	if got := percent(1, 4); got != 25 {
		t.Errorf("percent(1, 4) = %v, want 25", got)
	}
}
