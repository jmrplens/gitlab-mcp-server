// main_test.go covers the shared command plumbing: registering a tool
// surface on an in-process server through the stub GitLab client, the JSON
// entry conversion, and the two audit entry points in their JSON mode.
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/auditshared"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// metadataJSON is the report printed by the metadata view with -json.
type metadataJSON struct {
	View            string      `json:"view"`
	IndividualTools int         `json:"individual_tools"`
	MetaTools       int         `json:"meta_tools"`
	Violations      int         `json:"violations"`
	Entries         []jsonEntry `json:"entries"`
}

// outputJSONReport is the report printed by the output view with -json.
type outputJSONReport struct {
	View            string      `json:"view"`
	IndividualTools int         `json:"individual_tools"`
	MetaTools       int         `json:"meta_tools"`
	Findings        int         `json:"findings"`
	Entries         []jsonEntry `json:"entries"`
}

// stubClient returns the in-process GitLab client the audits register
// against, closing its backing server when the test ends.
func stubClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	client, cleanup := auditshared.NewStubGitLabClient(auditshared.StubToken)
	t.Cleanup(cleanup)
	return client
}

// withSurface serves both views, and the edition-tier rule's three
// listings, from list instead of the registered surface until the test ends.
// Nothing reads the client then, so a test using it may pass nil.
func withSurface(t *testing.T, list func(tier edition.Tier, meta bool) []*mcp.Tool) {
	t.Helper()
	original := listSurface
	t.Cleanup(func() { listSurface = original })
	listSurface = func(_ *gitlabclient.Client, tier edition.Tier, meta bool) []*mcp.Tool {
		return list(tier, meta)
	}
}

// cleanTool returns a tool no metadata or output rule reports: a two-word
// name that neither reads nor deletes, a description long enough that says
// what it returns and what to see next and claims no tier, a title, hints,
// and a locked-down input schema beside an output schema. Each case below
// spoils one of those.
func cleanTool(name string) *mcp.Tool {
	return &mcp.Tool{
		Name:         name,
		Title:        "A tool no rule reports",
		Description:  "Changes a widget. Returns: the widget. See also: nothing.",
		Annotations:  &mcp.ToolAnnotations{},
		InputSchema:  map[string]any{"type": "object", "additionalProperties": false},
		OutputSchema: map[string]any{"type": "object"},
	}
}

// TestListTools_MetaSurfaceIsRegisteredAndLockedDown verifies the meta
// surface registers through the in-memory transport and every tool arrives
// with a gitlab_ name, a unique name, and the locked-down map input schema
// the audits inspect. The individual surface is exercised by the two audit
// entry points below, which register both surfaces themselves.
func TestListTools_MetaSurfaceIsRegisteredAndLockedDown(t *testing.T) {
	listed := listTools(stubClient(t), true)
	if len(listed) < 20 {
		t.Fatalf("listTools(meta=true) returned %d tools, want at least 20", len(listed))
	}

	seen := make(map[string]bool, len(listed))
	for _, tool := range listed {
		if seen[tool.Name] {
			t.Errorf("duplicate tool name %q", tool.Name)
		}
		seen[tool.Name] = true
		if !strings.HasPrefix(tool.Name, "gitlab_") {
			t.Errorf("tool name %q does not start with gitlab_", tool.Name)
		}
		if _, ok := tool.InputSchema.(map[string]any); !ok {
			t.Errorf("tool %q input schema = %T, want a map", tool.Name, tool.InputSchema)
		}
	}
}

// TestToEntries_ConvertsViolations verifies the JSON entry conversion keeps
// the tool, category and detail of every violation in order, and returns an
// empty (not nil) slice for no violations.
func TestToEntries_ConvertsViolations(t *testing.T) {
	t.Parallel()

	got := toEntries([]violation{
		{tool: "gitlab_a", category: "naming", detail: "bad name"},
		{tool: "gitlab_b", category: "description", detail: "too short"},
	})
	want := []jsonEntry{
		{Tool: "gitlab_a", Category: "naming", Detail: "bad name"},
		{Tool: "gitlab_b", Category: "description", Detail: "too short"},
	}
	if len(got) != len(want) {
		t.Fatalf("toEntries() = %+v, want %+v", got, want)
	}
	for i := range want {
		t.Run(want[i].Tool, func(t *testing.T) {
			t.Parallel()
			if got[i] != want[i] {
				t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
			}
		})
	}
	if empty := toEntries(nil); len(empty) != 0 {
		t.Errorf("toEntries(nil) = %+v, want empty", empty)
	}
}

// TestRunMetadataAudit_JSONViewIsSelfConsistent verifies the metadata view
// registers both surfaces, names itself in the JSON report, counts the tools
// it audited, and emits exactly one entry per violation it counted.
func TestRunMetadataAudit_JSONViewIsSelfConsistent(t *testing.T) {
	// Not parallel: captureStdout rebinds os.Stdout and outputJSON is global.
	outputJSON = true
	t.Cleanup(func() { outputJSON = false })

	out := captureStdout(t, func() { runMetadataAudit(stubClient(t)) })

	var got metadataJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode metadata report: %v\n%s", err, out)
	}
	if got.View != "metadata" {
		t.Errorf("view = %q, want metadata", got.View)
	}
	if got.IndividualTools <= got.MetaTools {
		t.Errorf("individual_tools = %d, meta_tools = %d; want the individual surface to be larger", got.IndividualTools, got.MetaTools)
	}
	if got.MetaTools == 0 {
		t.Error("meta_tools = 0, want the meta surface to register tools")
	}
	if got.Violations != len(got.Entries) {
		t.Errorf("violations = %d but %d entries were listed", got.Violations, len(got.Entries))
	}
}

// TestRunOutputAudit_JSONViewIsSelfConsistent verifies the output view names
// itself in the JSON report, counts the same two surfaces, and lists one
// entry per finding, each carrying a category.
func TestRunOutputAudit_JSONViewIsSelfConsistent(t *testing.T) {
	// Not parallel: captureStdout rebinds os.Stdout and outputJSON is global.
	outputJSON = true
	t.Cleanup(func() { outputJSON = false })

	out := captureStdout(t, func() { runOutputAudit(stubClient(t)) })

	var got outputJSONReport
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode output report: %v\n%s", err, out)
	}
	if got.View != "output" {
		t.Errorf("view = %q, want output", got.View)
	}
	if got.IndividualTools == 0 || got.MetaTools == 0 {
		t.Errorf("individual_tools = %d, meta_tools = %d; want both surfaces registered", got.IndividualTools, got.MetaTools)
	}
	if got.Findings != len(got.Entries) {
		t.Errorf("findings = %d but %d entries were listed", got.Findings, len(got.Entries))
	}
	for _, entry := range got.Entries {
		if entry.Category == "" {
			t.Errorf("entry %+v has no category", entry)
			break
		}
	}
}

// TestAuditRouteOutputSchema_CatalogRoutesDeclareSchemas verifies the
// catalog-backed route audit builds the real action catalog through the stub
// client and reports nothing, which is the invariant the surface keeps.
func TestAuditRouteOutputSchema_CatalogRoutesDeclareSchemas(t *testing.T) {
	if got := auditRouteOutputSchema(stubClient(t)); len(got) != 0 {
		t.Fatalf("auditRouteOutputSchema() = %+v, want no findings", got)
	}
}

// TestReportGate_Violations_ArePrintedAndCounted checks what -check writes
// instead of the report: a count, one line per violation naming its subject,
// its category and its detail, and the same count returned as the exit
// condition.
func TestReportGate_Violations_ArePrintedAndCounted(t *testing.T) {
	// Not parallel: captureStdout rebinds os.Stdout.
	var got int
	out := captureStdout(t, func() {
		got = reportGate("metadata", []violation{
			{"gitlab_project_get", "edition-tier", "the description states Ultimate"},
		})
	})

	if got != 1 {
		t.Errorf("reportGate() = %d, want 1", got)
	}
	// The line is pinned whole: the subject first, the category bracketed and
	// the detail after the colon, so a reader can grep the list by tool.
	const want = "metadata: 1 violation(s)\n  gitlab_project_get [edition-tier]: the description states Ultimate\n"
	if out != want {
		t.Errorf("reportGate() printed %q, want %q", out, want)
	}
}

// TestReportGate_NoViolations_SaysSo checks the green line, so a run that
// gates on nothing still says which view it read.
func TestReportGate_NoViolations_SaysSo(t *testing.T) {
	// Not parallel: captureStdout rebinds os.Stdout.
	var got int
	out := captureStdout(t, func() { got = reportGate("output", nil) })

	if got != 0 {
		t.Errorf("reportGate() = %d, want 0", got)
	}
	if !strings.Contains(out, "output: no violations") {
		t.Errorf("reportGate() printed %q, want the green line", out)
	}
}

// TestAuditViews_CheckMode_ReadsBothViews runs the command's own entry point
// the way -check does: both views are read, each reports, and the exit
// condition is what they add up to.
//
// It does not assert that the total is zero, and could not: the Markdown
// registry is global and cannot be unregistered, so the formatters other
// tests in this package register to prove a rule fires are in the surface
// this walk reads. That the tree itself gates on nothing is asserted view by
// view instead: the metadata view by
// TestRunMetadataAudit_ServedSurface_ReportsOnlyThisPackagesOwnFormatters,
// which sets this package's own formatters aside, and the output view by the
// -check case of TestRunMain_CommandLine_DecidesTheExitCodeAndWhatIsRefused.
func TestAuditViews_CheckMode_ReadsBothViews(t *testing.T) {
	// Not parallel: captureStdout rebinds os.Stdout and checkMode is global.
	checkMode = true
	t.Cleanup(func() { checkMode = false })

	var got int
	out := captureStdout(t, func() { got = auditViews("all") })

	if got < 0 {
		t.Errorf("auditViews(all) = %d, want the violations the two views counted", got)
	}
	for _, want := range []string{"metadata:", "output: no violations"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(out, want) {
				t.Errorf("auditViews(all) printed %q, want it to carry %q", out, want)
			}
		})
	}
}

// TestAuditViews_UnknownView_ReadsNothing checks the one branch main's own
// validation keeps from happening, so a later caller cannot reach a view
// that silently audits everything.
func TestAuditViews_UnknownView_ReadsNothing(t *testing.T) {
	if got := auditViews("neither"); got != 0 {
		t.Errorf("auditViews(neither) = %d, want 0", got)
	}
}

// gatingRow is the list row a formatter registered below renders for every
// element, which gives the metadata view one violation of its own.
type gatingRow struct{ Name string }

// gatingList is the output type that formatter renders: one field, a list of
// rows, which is the shape the constant-index rule reads.
type gatingList struct{ Rows []gatingRow }

// registerGatingViolation registers a formatter that prints its first row in
// every row's place, so the metadata view gates on something this test owns
// rather than on whatever the tree happens to carry.
//
// The registry is global and has nothing to unregister with, so this outlives
// the test; the rule's own test skips every violation whose type is declared
// in package main for that reason. Registering twice would be refused and
// recorded as a registration problem, hence the [sync.Once].
func registerGatingViolation() {
	gatingViolationOnce.Do(func() {
		toolutil.RegisterMarkdown(func(v gatingList) string {
			var b strings.Builder
			b.WriteString("| Name |\n| --- |\n")
			for range v.Rows {
				b.WriteString("| " + v.Rows[0].Name + " |\n")
			}
			return b.String()
		})
	})
}

// gatingViolationOnce keeps the gating formatter to one registration.
var gatingViolationOnce sync.Once

// TestRunMain_CommandLine_DecidesTheExitCodeAndWhatIsRefused drives the
// command's own entry point over every command line it accepts and every one
// it refuses.
//
// The exit code is the whole contract of a gate: 2 says the command line was
// wrong and nothing was audited, 1 says -check found something, and 0 says it
// did not. Until this ran, the four refusals and the gating comparison were
// reachable only from a process, so a command line that silently audited
// everything, or a -check that exited 0 on a violation, would have failed
// nothing.
func TestRunMain_CommandLine_DecidesTheExitCodeAndWhatIsRefused(t *testing.T) {
	// Not parallel: captureStdout rebinds os.Stdout and runMain writes the
	// two package-level flag values.
	registerGatingViolation()
	t.Cleanup(func() { outputJSON, checkMode = false, false })

	cases := []struct {
		name   string
		args   []string
		want   int
		stderr string
		stdout string
	}{
		{
			name: "a view the command does not have is refused",
			args: []string{"-view=neither"}, want: 2,
			stderr: `invalid -view "neither"`,
		},
		{
			name: "a flag the command does not have is refused",
			args: []string{"-nosuchflag"}, want: 2,
			stderr: "flag provided but not defined",
		},
		{
			name: "help is the one parse failure that exits clean",
			args: []string{"-h"}, want: 0,
			stderr: "which audit view to run",
		},
		{
			name: "json without a view would emit two documents",
			args: []string{"-json"}, want: 2,
			stderr: "-json requires -view=metadata or -view=output",
		},
		{
			name: "json and check contradict each other on what stdout carries",
			args: []string{"-json", "-check", "-view=metadata"}, want: 2,
			stderr: "-check and -json are alternatives",
		},
		{
			name: "check over a view that gates on nothing exits 0",
			args: []string{"-check", "-view=output"}, want: 0,
			stdout: "output: no violations",
		},
		{
			name: "check over a view that gates on something exits 1",
			args: []string{"-check", "-view=metadata"}, want: 1,
			stdout: "main.gatingList",
		},
		{
			name: "the same violation without check reports and exits 0",
			args: []string{"-view=metadata"}, want: 0,
			stdout: "# MCP Tool Metadata Audit Report",
		},
		{
			name: "json over one view writes that view's report",
			args: []string{"-json", "-view=output"}, want: 0,
			stdout: `"view":"output"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			var got int
			out := captureStdout(t, func() { got = runMain(tc.args, &stderr) })

			if got != tc.want {
				t.Errorf("runMain(%v) = %d, want %d (stderr: %s)", tc.args, got, tc.want, stderr.String())
			}
			if tc.stderr != "" && !strings.Contains(stderr.String(), tc.stderr) {
				t.Errorf("runMain(%v) wrote %q to stderr, want it to carry %q", tc.args, stderr.String(), tc.stderr)
			}
			if tc.stderr == "" && stderr.Len() != 0 {
				t.Errorf("runMain(%v) wrote %q to stderr, want nothing", tc.args, stderr.String())
			}
			if tc.stdout != "" && !strings.Contains(out, tc.stdout) {
				t.Errorf("runMain(%v) wrote %q to stdout, want it to carry %q", tc.args, truncate(out), tc.stdout)
			}
		})
	}
}

// TestRunMain_CheckMetadata_GatesOnTheViolationItPrints holds the count the
// gate exits on to the list it printed, so a -check run cannot exit 1 while
// naming nothing or print a list while exiting 0.
func TestRunMain_CheckMetadata_GatesOnTheViolationItPrints(t *testing.T) {
	// Not parallel: captureStdout rebinds os.Stdout and runMain writes the
	// two package-level flag values.
	registerGatingViolation()
	t.Cleanup(func() { outputJSON, checkMode = false, false })

	var stderr bytes.Buffer
	var got int
	out := captureStdout(t, func() { got = runMain([]string{"-check", "-view=metadata"}, &stderr) })

	if got != 1 {
		t.Fatalf("runMain(-check -view=metadata) = %d, want 1", got)
	}
	if !strings.Contains(out, "main.gatingList") || !strings.Contains(out, constantIndexCategory) {
		t.Errorf("the gate printed %q, want the constant-index violation it exited on", truncate(out))
	}
	if strings.Contains(out, "# MCP Tool Metadata Audit Report") {
		t.Errorf("the gate printed the full report as well as the violation list:\n%s", truncate(out))
	}
}

// truncate shortens a captured report so a failure message names the head of
// it rather than eleven hundred tools.
func truncate(out string) string {
	const limit = 2000
	if len(out) <= limit {
		return out
	}
	return out[:limit] + "…"
}
