// main_test.go covers the shared command plumbing: registering a tool
// surface on an in-process server through the stub GitLab client, the JSON
// entry conversion, and the two audit entry points in their JSON mode.
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/auditclient"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
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
	client, cleanup := auditclient.NewMock()
	t.Cleanup(cleanup)
	return client
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
