package merge

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// emptyEnumReport is the enum stream of a clean tree, for the cases that are
// about the other three streams.
const emptyEnumReport = `{"packages":[]}`

// The four report documents the counter and key-name cases share. One package
// is named by every stream, and no two numbers the merge copies across agree,
// so a value that lands under the wrong name is visible rather than hidden
// behind a sibling that happens to read the same.
const (
	countedStructReport = `{"packages":[{"package":"x","missing_input_count":11,"missing_output_count":12,"extra_output_count":13,"gaps":[{"kind":"input"}]}]}`
	countedActionReport = `{"services":[{"service":"Svc","packages":["x"],"api_methods":31,"covered_methods":22,"missing_methods":["A","B","C","D","E","F"]}]}`
	countedMetaReport   = `{"packages":[{"package":"x","findings":[
	  {"action":"x.a","tool":"gitlab_x_a","usage":"Use to reach a.","flags":["generic_usage","generic_usage"]},
	  {"action":"x.b","flags":["aliases_only_toolname","aliases_only_toolname","aliases_only_toolname"]},
	  {"action":"x.c","flags":["empty_related","empty_related","empty_related","empty_related"]},
	  {"action":"x.d","flags":["weak_individual_description","weak_individual_description","weak_individual_description","weak_individual_description","weak_individual_description"]}
	]}]}`
	countedEnumReport = `{"packages":[{"package":"x","missing_values":14,"extra_values":15,"findings":[{"field":"state"}]}]}`
)

// loadBacklog is a test helper that runs BuildBacklogFromPaths and decodes the
// result for typed inspection. Tests assert on the decoded backlog struct.
// The enum stream is the clean one unless a case writes its own.
func loadBacklog(t *testing.T, structPath, actionPath, metadataPath string) backlog {
	t.Helper()
	return loadBacklogWithEnums(t, structPath, actionPath, metadataPath, writeTemp(t, filepath.Dir(structPath), "enums.json", emptyEnumReport))
}

// loadBacklogWithEnums is loadBacklog with an explicit enum report path.
func loadBacklogWithEnums(t *testing.T, structPath, actionPath, metadataPath, enumPath string) backlog {
	t.Helper()
	content, err := BuildBacklogFromPaths(structPath, actionPath, metadataPath, enumPath)
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

// TestBuildBacklog_EnumStream_AttachesValueGapsAndSkipsCleanPackages
// verifies the fourth stream: a package with a value gap gets an enums
// section carrying the counts and the findings verbatim, a package the enum
// report lists with no gap (a full report of a clean package) is left out,
// and the summary sums the two counters.
func TestBuildBacklog_EnumStream_AttachesValueGapsAndSkipsCleanPackages(t *testing.T) {
	dir := t.TempDir()
	structPath := writeTemp(t, dir, "struct.json", `{"packages":[]}`)
	actionPath := writeTemp(t, dir, "action.json", `{"services":[]}`)
	metadataPath := writeTemp(t, dir, "metadata.json", `{"packages":[]}`)
	enumPath := writeTemp(t, dir, "enums.json", `{"packages":[
	  {"package":"todos","fields":1,"missing_values":0,"extra_values":3,"findings":[{"action":"user.todo_list","kind":"input","field":"action","sdk_enum":"TodoAction","extra_values":["unmergeable"]}]},
	  {"package":"branches","fields":2,"missing_values":6,"extra_values":0,"findings":[{"action":"branch.protect","kind":"input","field":"push_access_level","sdk_enum":"AccessLevelValue","missing_values":["5","10"]}]},
	  {"package":"clean","fields":4,"missing_values":0,"extra_values":0,"findings":[]}
	]}`)

	bl := loadBacklogWithEnums(t, structPath, actionPath, metadataPath, enumPath)
	pkgs := indexPackages(bl)
	if len(bl.Packages) != 2 || bl.Packages[0].Package != "branches" || bl.Packages[1].Package != "todos" {
		t.Fatalf("packages = %+v, want branches and todos only, sorted", bl.Packages)
	}
	branches := pkgs["branches"]
	if branches.Enums == nil || branches.Enums.MissingValues != 6 || branches.Enums.ExtraValues != 0 {
		t.Errorf("branches enums = %+v, want 6 missing / 0 extra", branches.Enums)
	}
	var findings []map[string]any
	if unmarshalErr := json.Unmarshal(branches.Enums.Findings, &findings); unmarshalErr != nil {
		t.Fatalf("enum findings passthrough invalid: %v", unmarshalErr)
	}
	if len(findings) != 1 || findings[0]["sdk_enum"] != "AccessLevelValue" {
		t.Errorf("enum findings passthrough corrupted: %+v", findings)
	}
	if todos := pkgs["todos"]; todos.Enums == nil || todos.Enums.ExtraValues != 3 {
		t.Errorf("todos enums = %+v, want 3 extra", todos.Enums)
	}
	if bl.Summary.EnumMissingValues != 6 || bl.Summary.EnumExtraValues != 3 || bl.Summary.Packages != 2 {
		t.Errorf("summary = %+v, want 6 missing / 3 extra over 2 packages", bl.Summary)
	}
	if !strings.Contains(bl.Note, "enum stream") {
		t.Errorf("note = %q, want it to say what the enum stream is", bl.Note)
	}
}

// TestMarshalBacklog_EncoderFailure_IsReported reaches the encoding branch
// through the seam, since a backlog of strings, ints and raw JSON never fails
// to encode on its own.
func TestMarshalBacklog_EncoderFailure_IsReported(t *testing.T) {
	original := marshalIndent
	t.Cleanup(func() { marshalIndent = original })
	marshalIndent = func(any, string, string) ([]byte, error) { return nil, errors.New("boom") }

	if _, err := marshalBacklog(backlog{}); err == nil || !strings.Contains(err.Error(), "marshal backlog: boom") {
		t.Fatalf("marshalBacklog = %v, want the encoder failure", err)
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
	enumJSON := `{"packages":[{"package":"x","missing_values":1,"extra_values":0,"findings":[{"action":"x.list","field":"state","missing_values":["locked"]}]}]}`
	structPath := writeTemp(t, dir, "struct.json", structJSON)
	actionPath := writeTemp(t, dir, "action.json", actionJSON)
	metadataPath := writeTemp(t, dir, "metadata.json", metadataJSON)
	enumPath := writeTemp(t, dir, "enums.json", enumJSON)

	fromPaths, err := BuildBacklogFromPaths(structPath, actionPath, metadataPath, enumPath)
	if err != nil {
		t.Fatalf("BuildBacklogFromPaths: %v", err)
	}
	fromBytes, err := BuildBacklogFromBytes([]byte(structJSON), []byte(actionJSON), []byte(metadataJSON), []byte(enumJSON))
	if err != nil {
		t.Fatalf("BuildBacklogFromBytes: %v", err)
	}
	if !bytes.Equal(fromPaths, fromBytes) {
		t.Errorf("paths and bytes outputs differ:\npaths: %s\nbytes: %s", fromPaths, fromBytes)
	}
}

// TestMergeBacklog_EveryCounter_LandsUnderTheFieldItNames verifies each number
// the merge copies across reaches the field that names it, over a fixture
// where no two of them agree.
//
// Two counters holding the same number cannot say which of them was filled.
// The action stream's api_methods and covered_methods were read by no
// assertion at all, and the four metadata flags were each tallied exactly
// once everywhere, so any pair of them could have traded places and every
// test would have read the same.
func TestMergeBacklog_EveryCounter_LandsUnderTheFieldItNames(t *testing.T) {
	decode := func(document string, target any) {
		t.Helper()
		if err := json.Unmarshal([]byte(document), target); err != nil {
			t.Fatalf("decode %T: %v", target, err)
		}
	}
	var structRep structReport
	decode(countedStructReport, &structRep)
	var actionRep actionReport
	decode(countedActionReport, &actionRep)
	var metaRep metadataReport
	decode(countedMetaReport, &metaRep)
	var enumRep enumReport
	decode(countedEnumReport, &enumRep)

	bl := mergeBacklog(structRep, actionRep, metaRep, enumRep)

	if len(bl.Packages) != 1 {
		t.Fatalf("packages = %+v, want the one package every stream names", bl.Packages)
	}
	pkg := bl.Packages[0]
	if pkg.Struct == nil || pkg.Struct.MissingInput != 11 || pkg.Struct.MissingOutput != 12 || pkg.Struct.ExtraOutput != 13 {
		t.Errorf("struct gaps = %+v, want 11 missing input / 12 missing output / 13 extra output", pkg.Struct)
	}
	wantAction := actionGap{Service: "Svc", APIMethods: 31, CoveredMethods: 22, MissingMethods: []string{"A", "B", "C", "D", "E", "F"}}
	if len(pkg.Actions) != 1 || !reflect.DeepEqual(pkg.Actions[0], wantAction) {
		t.Errorf("action gaps = %+v, want [%+v]", pkg.Actions, wantAction)
	}
	if pkg.Metadata == nil || pkg.Metadata.GenericUsage != 2 || pkg.Metadata.AliasesOnlyToolname != 3 ||
		pkg.Metadata.EmptyRelated != 4 || pkg.Metadata.WeakIndividualDescription != 5 {
		t.Errorf("metadata counters = %+v, want 2/3/4/5 in flag order", pkg.Metadata)
	}
	if pkg.Enums == nil || pkg.Enums.MissingValues != 14 || pkg.Enums.ExtraValues != 15 {
		t.Errorf("enum gaps = %+v, want 14 missing / 15 extra", pkg.Enums)
	}
	wantSummary := backlogSummary{
		Packages: 1, StructMissingInput: 11, StructMissingOutput: 12, StructExtraOutput: 13,
		ActionMissingMethods: 6, MetaGenericUsage: 2, MetaAliasesOnlyToolname: 3,
		MetaEmptyRelated: 4, MetaWeakIndividualDescription: 5,
		EnumMissingValues: 14, EnumExtraValues: 15,
	}
	if bl.Summary != wantSummary {
		t.Errorf("summary = %+v, want %+v", bl.Summary, wantSummary)
	}
}

// TestBuildBacklogFromBytes_PublishedKeys_CarryTheValueEachNames reads the
// merged document as untyped JSON and holds every key the backlog publishes to
// the value that belongs under it.
//
// Every other case here decodes the output back through this package's own
// types, which makes the json tags invisible: two tags traded between adjacent
// fields are written one way and read back the other, so the round trip agrees
// with itself while the committed artifact says the opposite. Reading the
// document as a map is what names the key rather than the Go field.
func TestBuildBacklogFromBytes_PublishedKeys_CarryTheValueEachNames(t *testing.T) {
	content, err := BuildBacklogFromBytes([]byte(countedStructReport), []byte(countedActionReport), []byte(countedMetaReport), []byte(countedEnumReport))
	if err != nil {
		t.Fatalf("BuildBacklogFromBytes: %v", err)
	}
	var document map[string]any
	if unmarshalErr := json.Unmarshal(content, &document); unmarshalErr != nil {
		t.Fatalf("decode the merged document: %v", unmarshalErr)
	}

	cases := []struct {
		name string
		path []any
		want any
	}{
		{name: "schema_version", path: []any{"schema_version"}, want: float64(1)},
		{name: "summary.packages", path: []any{"summary", "packages"}, want: float64(1)},
		{name: "summary.struct_missing_input", path: []any{"summary", "struct_missing_input"}, want: float64(11)},
		{name: "summary.struct_missing_output", path: []any{"summary", "struct_missing_output"}, want: float64(12)},
		{name: "summary.struct_extra_output", path: []any{"summary", "struct_extra_output"}, want: float64(13)},
		{name: "summary.action_missing_methods", path: []any{"summary", "action_missing_methods"}, want: float64(6)},
		{name: "summary.meta_generic_usage", path: []any{"summary", "meta_generic_usage"}, want: float64(2)},
		{name: "summary.meta_aliases_only_toolname", path: []any{"summary", "meta_aliases_only_toolname"}, want: float64(3)},
		{name: "summary.meta_empty_related", path: []any{"summary", "meta_empty_related"}, want: float64(4)},
		{name: "summary.meta_weak_individual_description", path: []any{"summary", "meta_weak_individual_description"}, want: float64(5)},
		{name: "summary.enum_missing_values", path: []any{"summary", "enum_missing_values"}, want: float64(14)},
		{name: "summary.enum_extra_values", path: []any{"summary", "enum_extra_values"}, want: float64(15)},
		{name: "package", path: []any{"packages", 0, "package"}, want: "x"},
		{name: "struct.missing_input", path: []any{"packages", 0, "struct", "missing_input"}, want: float64(11)},
		{name: "struct.missing_output", path: []any{"packages", 0, "struct", "missing_output"}, want: float64(12)},
		{name: "struct.extra_output", path: []any{"packages", 0, "struct", "extra_output"}, want: float64(13)},
		{name: "struct.gaps", path: []any{"packages", 0, "struct", "gaps", 0, "kind"}, want: "input"},
		{name: "actions.service", path: []any{"packages", 0, "actions", 0, "service"}, want: "Svc"},
		{name: "actions.api_methods", path: []any{"packages", 0, "actions", 0, "api_methods"}, want: float64(31)},
		{name: "actions.covered_methods", path: []any{"packages", 0, "actions", 0, "covered_methods"}, want: float64(22)},
		{name: "actions.missing_methods", path: []any{"packages", 0, "actions", 0, "missing_methods", 5}, want: "F"},
		{name: "metadata.generic_usage", path: []any{"packages", 0, "metadata", "generic_usage"}, want: float64(2)},
		{name: "metadata.aliases_only_toolname", path: []any{"packages", 0, "metadata", "aliases_only_toolname"}, want: float64(3)},
		{name: "metadata.empty_related", path: []any{"packages", 0, "metadata", "empty_related"}, want: float64(4)},
		{name: "metadata.weak_individual_description", path: []any{"packages", 0, "metadata", "weak_individual_description"}, want: float64(5)},
		{name: "metadata.findings.action", path: []any{"packages", 0, "metadata", "findings", 0, "action"}, want: "x.a"},
		{name: "metadata.findings.tool", path: []any{"packages", 0, "metadata", "findings", 0, "tool"}, want: "gitlab_x_a"},
		{name: "metadata.findings.usage", path: []any{"packages", 0, "metadata", "findings", 0, "usage"}, want: "Use to reach a."},
		{name: "metadata.findings.flags", path: []any{"packages", 0, "metadata", "findings", 0, "flags", 0}, want: "generic_usage"},
		{name: "enums.missing_values", path: []any{"packages", 0, "enums", "missing_values"}, want: float64(14)},
		{name: "enums.extra_values", path: []any{"packages", 0, "enums", "extra_values"}, want: float64(15)},
		{name: "enums.findings", path: []any{"packages", 0, "enums", "findings", 0, "field"}, want: "state"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := jsonAt(t, document, tc.path...); got != tc.want {
				t.Errorf("%v = %#v, want %#v", tc.path, got, tc.want)
			}
		})
	}
}

// jsonAt walks a decoded JSON document by object key and array index so an
// assertion names the published path rather than the Go field behind it.
func jsonAt(t *testing.T, node any, path ...any) any {
	t.Helper()
	for i, step := range path {
		switch key := step.(type) {
		case string:
			object, ok := node.(map[string]any)
			if !ok {
				t.Fatalf("%v: %v is not an object", path[:i+1], node)
			}
			if node, ok = object[key]; !ok {
				t.Fatalf("%v: no such key", path[:i+1])
			}
		case int:
			array, ok := node.([]any)
			if !ok || key >= len(array) {
				t.Fatalf("%v: %v is not an array of at least %d", path[:i+1], node, key+1)
			}
			node = array[key]
		}
	}
	return node
}

// TestMergeBacklog_CleanPackage_KeepsOnlyItsStructSection verifies which
// streams report a package that has nothing to report: the struct stream
// carries one whatever its counts, and the other three leave it out.
//
// The asymmetry is deliberate: the struct report is a full census when
// -gaps-only is off, while the action, metadata and enum streams each skip an
// entry with no finding. It is the one property of the four merge
// functions that no branch flip can reach: adding a skip to the struct stream,
// or dropping one from any of the others, is a change of behavior every other
// case here is blind to, since they all drive gapped packages.
func TestMergeBacklog_CleanPackage_KeepsOnlyItsStructSection(t *testing.T) {
	decode := func(document string, target any) {
		t.Helper()
		if err := json.Unmarshal([]byte(document), target); err != nil {
			t.Fatalf("decode %T: %v", target, err)
		}
	}
	var structRep structReport
	decode(`{"packages":[{"package":"x","missing_input_count":0,"missing_output_count":0,"extra_output_count":0}]}`, &structRep)
	var actionRep actionReport
	decode(`{"services":[{"service":"Svc","packages":["x"],"api_methods":7,"covered_methods":7,"missing_methods":[]}]}`, &actionRep)
	var metaRep metadataReport
	decode(`{"packages":[{"package":"x","findings":[]}]}`, &metaRep)
	var enumRep enumReport
	decode(`{"packages":[{"package":"x","missing_values":0,"extra_values":0,"findings":[]}]}`, &enumRep)

	bl := mergeBacklog(structRep, actionRep, metaRep, enumRep)

	if len(bl.Packages) != 1 || bl.Packages[0].Package != "x" {
		t.Fatalf("packages = %+v, want the one package the struct stream lists", bl.Packages)
	}
	pkg := bl.Packages[0]
	if pkg.Struct == nil || !reflect.DeepEqual(*pkg.Struct, structGaps{}) {
		t.Errorf("struct section = %+v, want a zeroed one rather than none", pkg.Struct)
	}
	if len(pkg.Actions) != 0 || pkg.Metadata != nil || pkg.Enums != nil {
		t.Errorf("actions/metadata/enums = %+v/%+v/%+v, want all three to leave a clean package out", pkg.Actions, pkg.Metadata, pkg.Enums)
	}
	if want := (backlogSummary{Packages: 1}); bl.Summary != want {
		t.Errorf("summary = %+v, want %+v", bl.Summary, want)
	}
}

// TestMergeBacklog_InputAlreadyInOrder_IsLeftInIt verifies the two sorts do
// not disturb input that already arrives ordered: the services of one package
// listed alphabetically, and enough packages that the map iteration behind
// them cannot present every pair the same way round.
//
// It is the other side of the ordering tests above, which all hand the merge
// something out of order. A comparator is only shown to decide anything when
// it has answered both ways.
func TestMergeBacklog_InputAlreadyInOrder_IsLeftInIt(t *testing.T) {
	var actionRep actionReport
	document := `{"services":[
	  {"service":"AlphaServiceInterface","packages":["a","b","c","d","e","f"],"api_methods":2,"covered_methods":1,"missing_methods":["A"]},
	  {"service":"ZetaServiceInterface","packages":["a"],"api_methods":2,"covered_methods":1,"missing_methods":["Z"]}
	]}`
	if err := json.Unmarshal([]byte(document), &actionRep); err != nil {
		t.Fatalf("decode the action report: %v", err)
	}

	bl := mergeBacklog(structReport{}, actionRep, metadataReport{}, enumReport{})

	names := make([]string, 0, len(bl.Packages))
	for _, pkg := range bl.Packages {
		names = append(names, pkg.Package)
	}
	if !reflect.DeepEqual(names, []string{"a", "b", "c", "d", "e", "f"}) {
		t.Errorf("packages = %v, want them sorted", names)
	}
	first := bl.Packages[0]
	if len(first.Actions) != 2 || first.Actions[0].Service != "AlphaServiceInterface" || first.Actions[1].Service != "ZetaServiceInterface" {
		t.Errorf("a's services = %+v, want the alphabetical order they arrived in", first.Actions)
	}
}

func indexPackages(bl backlog) map[string]backlogPackage {
	out := make(map[string]backlogPackage, len(bl.Packages))
	for _, pkg := range bl.Packages {
		out[pkg.Package] = pkg
	}
	return out
}

// TestBuildBacklogFromPaths_BadInputs_NameTheFailingFile verifies each of
// the three report files is read and parsed in turn, and that a missing or
// malformed one fails the merge with its own path in the error.
func TestBuildBacklogFromPaths_BadInputs_NameTheFailingFile(t *testing.T) {
	dir := t.TempDir()
	good := map[string]string{
		"struct":   writeTemp(t, dir, "struct.json", `{"packages":[]}`),
		"action":   writeTemp(t, dir, "action.json", `{"services":[]}`),
		"metadata": writeTemp(t, dir, "metadata.json", `{"packages":[]}`),
		"enums":    writeTemp(t, dir, "enums.json", emptyEnumReport),
	}
	malformed := writeTemp(t, dir, "malformed.json", `{"packages": [`)
	missing := filepath.Join(dir, "missing.json")

	cases := []struct {
		name     string
		paths    map[string]string
		wantPath string
		wantErr  string
	}{
		{name: "missing_struct_report", paths: map[string]string{"struct": missing}, wantPath: missing, wantErr: "read "},
		{name: "missing_action_report", paths: map[string]string{"action": missing}, wantPath: missing, wantErr: "read "},
		{name: "missing_metadata_report", paths: map[string]string{"metadata": missing}, wantPath: missing, wantErr: "read "},
		{name: "missing_enum_report", paths: map[string]string{"enums": missing}, wantPath: missing, wantErr: "read "},
		{name: "malformed_struct_report", paths: map[string]string{"struct": malformed}, wantPath: malformed, wantErr: "parse "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			paths := maps.Clone(good)
			maps.Copy(paths, tc.paths)
			_, err := BuildBacklogFromPaths(paths["struct"], paths["action"], paths["metadata"], paths["enums"])
			if err == nil || !strings.Contains(err.Error(), tc.wantErr+tc.wantPath) {
				t.Fatalf("BuildBacklogFromPaths error = %v, want it to contain %q", err, tc.wantErr+tc.wantPath)
			}
		})
	}
}

// TestBuildBacklogFromBytes_BadInputs_NameTheFailingStream verifies each
// in-memory report is parsed in turn and a malformed one fails the merge
// naming its stream.
func TestBuildBacklogFromBytes_BadInputs_NameTheFailingStream(t *testing.T) {
	structJSON, actionJSON, metadataJSON := `{"packages":[]}`, `{"services":[]}`, `{"packages":[]}`
	cases := []struct {
		name    string
		inputs  [4]string
		wantErr string
	}{
		{name: "struct", inputs: [4]string{"{", actionJSON, metadataJSON, emptyEnumReport}, wantErr: "parse struct report"},
		{name: "action", inputs: [4]string{structJSON, "[]", metadataJSON, emptyEnumReport}, wantErr: "parse action report"},
		{name: "metadata", inputs: [4]string{structJSON, actionJSON, "nope", emptyEnumReport}, wantErr: "parse metadata report"},
		{name: "enums", inputs: [4]string{structJSON, actionJSON, metadataJSON, "{"}, wantErr: "parse enum report"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildBacklogFromBytes([]byte(tc.inputs[0]), []byte(tc.inputs[1]), []byte(tc.inputs[2]), []byte(tc.inputs[3]))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("BuildBacklogFromBytes error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// TestMergeBacklog_Streams_CountEveryFlagAndSortServices verifies the pure
// merge: every metadata flag is tallied under its counter, a metadata
// package without findings is left out, a package referenced by two gapped
// services lists them sorted by service, and the summary sums it all.
func TestMergeBacklog_Streams_CountEveryFlagAndSortServices(t *testing.T) {
	var actionRep actionReport
	if err := json.Unmarshal([]byte(`{"services":[
	  {"service":"ZetaServiceInterface","packages":["issues"],"api_methods":3,"covered_methods":2,"missing_methods":["Z"]},
	  {"service":"AlphaServiceInterface","packages":["issues"],"api_methods":5,"covered_methods":3,"missing_methods":["A","B"]}
	]}`), &actionRep); err != nil {
		t.Fatal(err)
	}
	var metaRep metadataReport
	if err := json.Unmarshal([]byte(`{"packages":[
	  {"package":"issues","findings":[
	    {"action":"issue.list","flags":["generic_usage","aliases_only_toolname"]},
	    {"action":"issue.get","flags":["empty_related","weak_individual_description","unknown"]}
	  ]},
	  {"package":"clean","findings":[]}
	]}`), &metaRep); err != nil {
		t.Fatal(err)
	}

	bl := mergeBacklog(structReport{}, actionRep, metaRep, enumReport{})
	if len(bl.Packages) != 1 || bl.Packages[0].Package != "issues" {
		t.Fatalf("packages = %+v, want only issues (clean has no findings)", bl.Packages)
	}
	issues := bl.Packages[0]
	if len(issues.Actions) != 2 || issues.Actions[0].Service != "AlphaServiceInterface" || issues.Actions[1].Service != "ZetaServiceInterface" {
		t.Errorf("issues actions = %+v, want the two services sorted by name", issues.Actions)
	}
	wantMeta := &metadataGaps{GenericUsage: 1, AliasesOnlyToolname: 1, EmptyRelated: 1, WeakIndividualDescription: 1, Findings: metaRep.Packages[0].Findings}
	if issues.Metadata == nil || !reflectEqualMeta(issues.Metadata, wantMeta) {
		t.Errorf("issues metadata = %+v, want %+v", issues.Metadata, wantMeta)
	}
	wantSummary := backlogSummary{Packages: 1, ActionMissingMethods: 3, MetaGenericUsage: 1, MetaAliasesOnlyToolname: 1, MetaEmptyRelated: 1, MetaWeakIndividualDescription: 1}
	if bl.Summary != wantSummary {
		t.Errorf("summary = %+v, want %+v", bl.Summary, wantSummary)
	}
	if bl.SchemaVersion != 1 || !strings.HasPrefix(bl.Note, "Merged 1:1 audit backlog.") {
		t.Errorf("backlog header = version %d, note %q", bl.SchemaVersion, bl.Note)
	}
}

// reflectEqualMeta compares two metadata sections by their JSON form, since
// the findings carry omitempty fields that make a direct comparison brittle.
func reflectEqualMeta(a, b *metadataGaps) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(ja, jb)
}
