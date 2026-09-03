package merge

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeTemp writes content to a temp file and returns its path.
func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// loadBacklog is a test helper that runs BuildBacklogFromPaths and decodes the
// result for typed inspection. Tests assert on the decoded backlog struct.
func loadBacklog(t *testing.T, structPath, actionPath, metadataPath string) backlog {
	t.Helper()
	content, err := BuildBacklogFromPaths(structPath, actionPath, metadataPath)
	if err != nil {
		t.Fatalf("BuildBacklogFromPaths: %v", err)
	}
	var bl backlog
	if unmarshalErr := json.Unmarshal(content, &bl); unmarshalErr != nil {
		t.Fatalf("decode backlog: %v", unmarshalErr)
	}
	return bl
}

// TestBuildBacklog_MergesThreeStreams verifies the merge attributes each gap
// stream to the right package, attributes a shared service to every referencing
// package, and tallies the summary.
func TestBuildBacklog_MergesThreeStreams(t *testing.T) {
	dir := t.TempDir()
	structPath := writeTemp(t, dir, "struct.json", `{
	  "packages": [
	    {"package": "branches", "missing_input_count": 20, "missing_output_count": 4, "gaps": [{"kind":"input","mcp_type":"ProtectInput","sdk_type":"v2.ProtectRepositoryBranchesOptions","missing_fields":[{"tag":"name","sdk_type":"*string"}]}]}
	  ]
	}`)
	actionPath := writeTemp(t, dir, "action.json", `{
	  "services": [
	    {"service": "DiscussionsServiceInterface", "packages": ["mrdiscussions","issuediscussions"], "api_methods": 31, "covered_methods": 25, "missing_methods": ["AddEpicDiscussionNote","CreateEpicDiscussion"]},
	    {"service": "CleanServiceInterface", "packages": ["clean"], "api_methods": 4, "covered_methods": 4, "missing_methods": []}
	  ]
	}`)
	metadataPath := writeTemp(t, dir, "metadata.json", `{
	  "packages": [
	    {"package": "branches", "actions": 10, "findings": [
	      {"action":"branch.list","tool":"gitlab_branch_list","usage":"Use to execute branches domain action.","flags":["generic_usage","empty_related"]}
	    ]}
	  ]
	}`)

	bl := loadBacklog(t, structPath, actionPath, metadataPath)

	// branches gets struct + metadata; mrdiscussions + issuediscussions get the
	// shared service; clean (no missing methods) is absent.
	pkgs := indexPackages(bl)
	branches, ok := pkgs["branches"]
	if !ok {
		t.Fatal("branches missing from backlog")
	}
	if branches.Struct == nil || branches.Struct.MissingInput != 20 {
		t.Errorf("branches struct gaps not merged: %+v", branches.Struct)
	}
	if branches.Metadata == nil || branches.Metadata.GenericUsage != 1 || branches.Metadata.EmptyRelated != 1 {
		t.Errorf("branches metadata not merged: %+v", branches.Metadata)
	}
	for _, name := range []string{"mrdiscussions", "issuediscussions"} {
		t.Run(name, func(t *testing.T) {
			pkg, found := pkgs[name]
			if !found || len(pkg.Actions) != 1 || len(pkg.Actions[0].MissingMethods) != 2 {
				t.Errorf("%s did not receive shared service gap: %+v", name, pkg.Actions)
			}
		})
	}
	if _, found := pkgs["clean"]; found {
		t.Error("clean service has no missing methods and must not appear")
	}

	if bl.Summary.StructMissingInput != 20 || bl.Summary.StructMissingOutput != 4 {
		t.Errorf("struct summary = %d/%d, want 20/4", bl.Summary.StructMissingInput, bl.Summary.StructMissingOutput)
	}
	// Shared service attributed to two packages → counted twice in the summary.
	if bl.Summary.ActionMissingMethods != 4 {
		t.Errorf("action summary = %d, want 4 (2 methods × 2 packages)", bl.Summary.ActionMissingMethods)
	}
	if bl.Summary.MetaGenericUsage != 1 || bl.Summary.MetaEmptyRelated != 1 {
		t.Errorf("meta summary = %d/%d, want 1/1", bl.Summary.MetaGenericUsage, bl.Summary.MetaEmptyRelated)
	}
}

// TestBuildBacklog_PackagesSortedAndStructGapsPreserved verifies deterministic
// package ordering and that the struct gaps blob is passed through verbatim.
func TestBuildBacklog_PackagesSortedAndStructGapsPreserved(t *testing.T) {
	dir := t.TempDir()
	structPath := writeTemp(t, dir, "struct.json", `{"packages":[
	  {"package":"zeta","missing_input_count":1,"missing_output_count":0,"gaps":[{"kind":"input"}]},
	  {"package":"alpha","missing_input_count":0,"missing_output_count":2,"gaps":[{"kind":"output"}]}
	]}`)
	actionPath := writeTemp(t, dir, "action.json", `{"services":[]}`)
	metadataPath := writeTemp(t, dir, "metadata.json", `{"packages":[]}`)

	bl := loadBacklog(t, structPath, actionPath, metadataPath)
	if len(bl.Packages) != 2 || bl.Packages[0].Package != "alpha" || bl.Packages[1].Package != "zeta" {
		t.Fatalf("packages not sorted: %+v", bl.Packages)
	}
	var gaps []map[string]any
	if unmarshalErr := json.Unmarshal(bl.Packages[0].Struct.Gaps, &gaps); unmarshalErr != nil {
		t.Fatalf("struct gaps not valid JSON passthrough: %v", unmarshalErr)
	}
	if len(gaps) != 1 || gaps[0]["kind"] != "output" {
		t.Errorf("struct gaps passthrough corrupted: %+v", gaps)
	}
}

// TestBuildBacklog_SurfacesExtraOutput verifies the R-OUTPUT-EXTRA count flows
// from the struct auditor into the per-package struct section and the summary.
func TestBuildBacklog_SurfacesExtraOutput(t *testing.T) {
	dir := t.TempDir()
	structPath := writeTemp(t, dir, "struct.json", `{"packages":[
	  {"package":"issues","missing_input_count":0,"missing_output_count":0,"extra_output_count":2,"gaps":[{"kind":"output","mcp_type":"Output","sdk_type":"v2.Issue","extra_fields":[{"tag":"author_username","mcp_type":"string"},{"tag":"milestone_title","mcp_type":"string"}]}]}
	]}`)
	actionPath := writeTemp(t, dir, "action.json", `{"services":[]}`)
	metadataPath := writeTemp(t, dir, "metadata.json", `{"packages":[]}`)

	bl := loadBacklog(t, structPath, actionPath, metadataPath)
	pkgs := indexPackages(bl)
	issues, ok := pkgs["issues"]
	if !ok || issues.Struct == nil {
		t.Fatalf("issues struct section missing: %+v", issues)
	}
	if issues.Struct.ExtraOutput != 2 {
		t.Errorf("issues extra_output = %d, want 2", issues.Struct.ExtraOutput)
	}
	if bl.Summary.StructExtraOutput != 2 {
		t.Errorf("summary struct_extra_output = %d, want 2", bl.Summary.StructExtraOutput)
	}
	// Extra-field tags must survive the json.RawMessage passthrough.
	var gaps []map[string]any
	if unmarshalErr := json.Unmarshal(issues.Struct.Gaps, &gaps); unmarshalErr != nil {
		t.Fatalf("gaps passthrough invalid: %v", unmarshalErr)
	}
	if len(gaps) != 1 || gaps[0]["extra_fields"] == nil {
		t.Errorf("extra_fields not preserved in gaps passthrough: %+v", gaps)
	}
}

// TestReadJSON_MissingFile verifies a clear error when an input report is absent.
func TestReadJSON_MissingFile(t *testing.T) {
	var target structReport
	if err := readJSON(filepath.Join(t.TempDir(), "nope.json"), &target); err == nil {
		t.Fatal("expected error reading missing file")
	}
}

// TestBuildBacklogFromBytes_MatchesFromPaths is the differential guarantee: the
// in-memory bytes entry point MUST produce byte-identical output to the
// file-based entry point for the same inputs. This is the property that lets
// audit_1to1's all-scope mode reuse the merge pipeline without divergence.
func TestBuildBacklogFromBytes_MatchesFromPaths(t *testing.T) {
	dir := t.TempDir()
	structJSON := `{"packages":[{"package":"x","missing_input_count":3,"missing_output_count":1,"gaps":[{"kind":"input"}]}]}`
	actionJSON := `{"services":[{"service":"Svc","packages":["x"],"api_methods":10,"covered_methods":8,"missing_methods":["A","B"]}]}`
	metadataJSON := `{"packages":[{"package":"x","findings":[{"action":"x.list","flags":["generic_usage"]}]}]}`
	structPath := writeTemp(t, dir, "struct.json", structJSON)
	actionPath := writeTemp(t, dir, "action.json", actionJSON)
	metadataPath := writeTemp(t, dir, "metadata.json", metadataJSON)

	fromPaths, err := BuildBacklogFromPaths(structPath, actionPath, metadataPath)
	if err != nil {
		t.Fatalf("BuildBacklogFromPaths: %v", err)
	}
	fromBytes, err := BuildBacklogFromBytes([]byte(structJSON), []byte(actionJSON), []byte(metadataJSON))
	if err != nil {
		t.Fatalf("BuildBacklogFromBytes: %v", err)
	}
	if !bytes.Equal(fromPaths, fromBytes) {
		t.Errorf("paths and bytes outputs differ:\npaths: %s\nbytes: %s", fromPaths, fromBytes)
	}
}

func indexPackages(bl backlog) map[string]backlogPackage {
	out := make(map[string]backlogPackage, len(bl.Packages))
	for _, pkg := range bl.Packages {
		out[pkg.Package] = pkg
	}
	return out
}
