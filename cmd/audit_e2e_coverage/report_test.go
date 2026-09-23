package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
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
	if rep.Summary.CatalogActions != 12 || rep.Summary.L1 != 3 || rep.Summary.L2 != 2 || rep.Summary.L3 != 0 || rep.Summary.TestCalls != 26 {
		t.Errorf("Summary = %+v, want 12 actions, L1 3, L2 2, L3 0, 26 test calls", rep.Summary)
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

// TestRunRows_SeveralPackages_SortedByPackage verifies that the run rows are
// published in package order whatever order the shards were read in, which
// is what lets a reader diff two runs' rows line by line.
func TestRunRows_SeveralPackages_SortedByPackage(t *testing.T) {
	rows := runRows(&runtimeRecords{runs: []*e2ecalls.Run{
		{Package: "ee", Status: e2ecalls.RunStarted},
		{Package: "ce", Status: e2ecalls.RunStarted},
		{Package: "common", Status: e2ecalls.RunStarted},
	}})

	var packages []string
	for _, row := range rows {
		packages = append(packages, row.Package)
	}
	if want := []string{"ce", "common", "ee"}; !reflect.DeepEqual(packages, want) {
		t.Errorf("run rows = %q, want %q", packages, want)
	}
}

// TestRunRows_Fields_CopiedFromTheRunLine verifies that a run row carries
// every field of its run line under the name the record publishes it by, on
// a line where no two values agree: the fixture shards leave half of these
// empty, so two columns exchanged there would read the same.
func TestRunRows_Fields_CopiedFromTheRunLine(t *testing.T) {
	profile := e2ecalls.FixtureProfile{Runner: true, Bitbucket: true}
	rows := runRows(&runtimeRecords{runs: []*e2ecalls.Run{{
		Package: "ee", Requirement: "licensed", Status: e2ecalls.RunRefused, Reason: "no license",
		Filter: "^TestEpic", RunID: "20260912t110000z-def", Commit: "6bd82ea6", GitLabVersion: "18.4.0-ee",
		TierConfirmed: true, Fixtures: profile,
	}}})

	want := []runRow{{
		Package: "ee", Requirement: "licensed", Status: e2ecalls.RunRefused, Reason: "no license",
		Filter: "^TestEpic", RunID: "20260912t110000z-def", Commit: "6bd82ea6", GitLabVersion: "18.4.0-ee",
		TierConfirmed: true, Fixtures: profile,
	}}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("runRows() = %+v, want %+v", rows, want)
	}
}

// TestSessionRows_TwoLinesOneShape_CountsWhatWasFolded verifies the session
// row of a shape two session lines opened: the lines are counted, each
// listing is the union of the two, and every count is its own listing's. The
// listings are sized so that no two counts agree, since a shape serving one
// resource and one prompt cannot tell the two columns apart.
func TestSessionRows_TwoLinesOneShape_CountsWhatWasFolded(t *testing.T) {
	first := fixtureSession(dynamicDefault, true)
	first.Tools = []string{"gitlab_execute_action", "gitlab_find_action", "gitlab_issue"}
	first.Resources = []string{"gitlab://groups", "gitlab://projects"}
	first.ResourceTemplates = []string{"gitlab://project/{project_id}"}
	first.Prompts = []string{"summarize_issue"}
	second := fixtureSession(dynamicDefault, false)
	second.Tools = []string{"gitlab_issue", "gitlab_project", "gitlab_server"}
	second.Resources = []string{"gitlab://projects", "gitlab://users", "gitlab://me"}
	second.ResourceTemplates = []string{"gitlab://project/{project_id}", "gitlab://group/{group_id}", "gitlab://user/{user_id}"}
	second.Prompts = []string{"summarize_issue"}
	// The second line called a tool and saw no span, which is what marks the
	// folded shape unobserved.
	rows := sessionRows(classify(&runtimeRecords{
		sessions:     []*e2ecalls.Session{first, second},
		toolSessions: map[*e2ecalls.Session]bool{second: true},
	}, fixtureCatalog()))

	want := []sessionRow{{
		Surface: config.ToolSurfaceDynamic, Mode: modeDefault, Sessions: 2,
		Tools: 5, Resources: 4, ResourceTemplates: 3, Prompts: 1, DispatchObserved: false,
	}}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("sessionRows() = %+v, want %+v", rows, want)
	}
}

// TestBuildReport_SubscriptionDelivery_Marked verifies that the subscription
// row a notification reached says so and the others do not, which the report
// can only find by the key the notification was filed under: the capability
// surface and the kind, with no shape.
func TestBuildReport_SubscriptionDelivery_Marked(t *testing.T) {
	rep := fixtureReport()
	delivered := map[string]bool{}
	for _, row := range rep.Capabilities[capabilitySubscriptions] {
		if row.Capabilities == config.CapabilitySurfaceFull {
			delivered[row.Target] = row.Delivered
		}
	}
	if !delivered["issue"] || delivered["pipeline"] {
		t.Errorf("delivered = issue %t, pipeline %t; want only the issue kind", delivered["issue"], delivered["pipeline"])
	}
}

// TestCapabilitySurfaceRows_EachSurface_CountsWhatItServed verifies the rows
// the capability-grain histograms (resources, prompts, completions,
// subscriptions) are counted against, one per capability surface, full
// first. Each figure is what that surface served of its kind: the
// resources without the tool-manifest pair, the subscribable kinds on full
// only, and the shapes and sessions its lines came from. The two surfaces are
// given listings that differ in every column, so no two figures can be
// exchanged and still read right.
func TestCapabilitySurfaceRows_EachSurface_CountsWhatItServed(t *testing.T) {
	rt := fixtureRuntime()
	rt.calls = nil
	offering := fixtureSession(dynamicDefault, true)
	offering.Prompts = []string{"summarize_issue", "triage_issue"}
	offering.Completions = []string{"summarize_issue project_id", "triage_issue issue_iid", "triage_issue project_id"}
	lean := minimalSession(dynamicDefault)
	lean.Completions = []string{manifestDetail + " id"}
	rt.sessions = append(rt.sessions, offering, lean, minimalSession(metaDefault))

	rows := capabilitySurfaceRows(classify(rt, fixtureCatalog()))

	want := []capabilitySurfaceRow{
		{
			Capabilities: config.CapabilitySurfaceFull, Sessions: 6, Shapes: 5, Resources: 4, Prompts: 2,
			Completions: 3, SubscribableKinds: len(subscribableKinds(nil)),
		},
		{Capabilities: config.CapabilitySurfaceMinimal, Sessions: 2, Shapes: 2, Completions: 1},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("capabilitySurfaceRows() = %+v, want %+v", rows, want)
	}
}

// TestCellRows_CapabilityGrain_CarriesOnlyItsCoordinates verifies what a
// published cell row names: a prompt, counted once per capability surface,
// carries that surface and no surface or mode key at all, while a tool
// manifest row carries all three and an action cell's row the two it always
// had. A prompt row that said dynamic/default would read as a prompt rendered
// on that one shape.
func TestCellRows_CapabilityGrain_CarriesOnlyItsCoordinates(t *testing.T) {
	rep := fixtureReport()
	cases := []struct {
		name string
		rows []cellRow
		want string
	}{
		{
			name: "a prompt",
			rows: rep.Capabilities[capabilityPrompts],
			want: `{"capabilities":"full","target":"summarize_issue","state":"asserted","tests":["TestPrompts"],"calls":1}`,
		},
		{
			name: "a tool manifest read",
			rows: rep.Capabilities[capabilityToolManifest],
			want: `{"surface":"meta","mode":"default","capabilities":"full","target":"gitlab://tools/{id}","state":"asserted","tests":["TestManifest"],"calls":1}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.rows) != 1 {
				t.Fatalf("rows = %+v, want the one cell a call reached, absent ones left out", tc.rows)
			}
			encoded, err := json.Marshal(tc.rows[0])
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if string(encoded) != tc.want {
				t.Errorf("row = %s, want %s", encoded, tc.want)
			}
		})
	}
	for _, row := range rep.Cells {
		if row.Capabilities != "" || row.Surface == "" || row.Mode == "" {
			t.Errorf("action cell %+v, want a surface and a mode and no capability surface", row)
		}
	}
}

// TestCellRows_Order_ShapeThenCapabilitySurfaceThenTarget verifies the order
// the cell rows are published in, which is what lets two reports of one
// classification be compared line by line: the surfaces in level order, the
// rows carrying no surface after the three and a surface that is none of them
// after those, by name; then the mode, the capability surface and the target.
// The cells come out of a map, so the listing is drawn many times over.
func TestCellRows_Order_ShapeThenCapabilitySurfaceThenTarget(t *testing.T) {
	keys := []cellKey{
		{shape: shapeKey{surface: "zeta", mode: modeDefault}, action: "a"},
		{capabilities: config.CapabilitySurfaceMinimal, action: "x"},
		{shape: individualDefault, action: "a"},
		{shape: shapeKey{surface: config.ToolSurfaceDynamic, mode: modeSafe}, action: "a"},
		{capabilities: config.CapabilitySurfaceFull, action: "x"},
		{shape: dynamicDefault, action: "b"},
		{shape: metaDefault, action: "a"},
		{capabilities: config.CapabilitySurfaceFull, action: "a"},
		{shape: dynamicDefault, action: "a"},
		{shape: shapeKey{surface: "alpha", mode: modeDefault}, action: "a"},
	}
	cells := map[cellKey]*cell{}
	for _, key := range keys {
		found := newCell(key)
		found.state = stateAsserted
		cells[key] = found
	}
	want := []string{
		"dynamic/default/ a", "dynamic/default/ b", "dynamic/safe/ a", "meta/default/ a", "individual/default/ a",
		"//full a", "//full x", "//minimal x", "alpha/default/ a", "zeta/default/ a",
	}
	for range 32 {
		var got []string
		for _, row := range cellRows(cells) {
			got = append(got, row.Surface+"/"+row.Mode+"/"+row.Capabilities+" "+row.Target)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("cellRows() order = %q, want %q", got, want)
		}
	}
}

// TestBuildReport_ShapeRows_InLevelOrder verifies the order the session rows
// and the uncalled-tool rows are published in, the default surface first and
// the modes by name within a surface, whatever order the shapes were folded
// in.
func TestBuildReport_ShapeRows_InLevelOrder(t *testing.T) {
	want := []string{"dynamic/default", "meta/default", "meta/read-only", "individual/default", "individual/safe"}
	for range 16 {
		rep := fixtureReport()
		var sessions, uncalled []string
		for _, row := range rep.Sessions {
			sessions = append(sessions, row.Surface+"/"+row.Mode)
		}
		for _, row := range rep.UncalledTools {
			uncalled = append(uncalled, row.Surface+"/"+row.Mode)
		}
		if !slices.Equal(sessions, want) || !slices.Equal(uncalled, want) {
			t.Fatalf("session rows %q and uncalled rows %q, want both %q", sessions, uncalled, want)
		}
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
//
// The two histogram tables are padded and their keys are in backticks because
// they are [renderStateTable]'s, the same drawing the committed coverage page
// carries; the summary used to spell its own unpadded copy of the same columns.
//
// The fixture records one mismatch and one unresolved tool, which is two
// counts that agree, so a second unresolved tool is added to the report before
// it is drawn: the line names each count, and two that read the same could
// not tell which was which.
func TestWriteMarkdownSummary_Fixture_Document(t *testing.T) {
	rep := fixtureReport()
	rep.Check = &checkResult{Passed: false, Findings: []string{"no test call was recorded on community/free"}}
	rep.Baseline = &baselineResult{BaselineReached: 4, Reached: 3, Lost: []string{"meta/default issue.create asserted"}}
	rep.UnresolvedTools = append(rep.UnresolvedTools, unresolvedTool{
		Test: "TestRaw", Surface: config.ToolSurfaceMeta, Mode: modeDefault, Tool: "gitlab_nope_either", Outcome: e2ecalls.OutcomeProtocolError,
	})
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
		"- Test calls: 26; dispatch mismatches: 1; unresolved tools: 2",
		"",
		"| Surface      | asserted | unobserved | sweep-only | error-path-only | refused-only | preview-only | cleanup-only | unasserted | unservable | skipped | failed | absent |",
		"| ------------ | -------: | ---------: | ---------: | --------------: | -----------: | -----------: | -----------: | ---------: | ---------: | ------: | -----: | -----: |",
		"| `dynamic`    |        2 |          0 |          0 |               1 |            1 |            0 |            0 |          0 |          1 |       0 |      1 |      6 |",
		"| `meta`       |        2 |          1 |          0 |               0 |            0 |            0 |            1 |          0 |          1 |       1 |      0 |      6 |",
		"| `individual` |        0 |          0 |          1 |               0 |            0 |            0 |            0 |          0 |          3 |       0 |      0 |      8 |",
		"",
		"| Capability      | asserted | unobserved | sweep-only | error-path-only | refused-only | preview-only | cleanup-only | unasserted | unservable | skipped | failed | absent |",
		"| --------------- | -------: | ---------: | ---------: | --------------: | -----------: | -----------: | -----------: | ---------: | ---------: | ------: | -----: | -----: |",
		"| `completions`   |        0 |          0 |          0 |               0 |            0 |            0 |            0 |          0 |          0 |       0 |      0 |      0 |",
		"| `elicitation`   |        1 |          0 |          0 |               1 |            0 |            0 |            0 |          0 |          1 |       0 |      0 |      2 |",
		"| `modes`         |        4 |          0 |          0 |               0 |            0 |            0 |            0 |          0 |          0 |       0 |      0 |      0 |",
		"| `prompts`       |        1 |          0 |          0 |               0 |            0 |            0 |            0 |          0 |          0 |       0 |      0 |      0 |",
		"| `resources`     |        3 |          0 |          0 |               0 |            0 |            0 |            0 |          0 |          0 |       0 |      0 |      1 |",
		"| `subscriptions` |        2 |          0 |          0 |               0 |            0 |            0 |            0 |          0 |          0 |       0 |      0 |     24 |",
		"| `tool_manifest` |        1 |          0 |          0 |               0 |            0 |            0 |            0 |          0 |          0 |       0 |      0 |      9 |",
		"",
		// The fixture sessions list no completion and no gitlab://nowhere,
		// so those two calls are named apart rather than counted above.
		"Called outside what any session listed, and so counted in none of the rows above:",
		"",
		"- `completions` `summarize_issue project_id` on `full`: asserted, by `TestCompletions`",
		"- `resources` `gitlab://nowhere` on `full`: error-path-only, by `TestResources`",
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

// TestWriteStateTable_EmptyHistogram_WritesNothing verifies the one thing the
// summary's table keeps that the page's does not.
//
// A run's summary is a section of a document every step of the job writes into,
// and a runtime that classified no capability at all would otherwise leave a
// header with no rows under it, which reads as a table whose data went missing.
func TestWriteStateTable_EmptyHistogram_WritesNothing(t *testing.T) {
	var out bytes.Buffer

	writeStateTable(&out, "Capability", nil)

	if out.Len() > 0 {
		t.Errorf("writeStateTable() wrote %q for an empty histogram, want nothing", out.String())
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
