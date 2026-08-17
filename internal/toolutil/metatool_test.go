// metatool_test.go tests the generic meta-tool dispatch infrastructure:
// UnmarshalParams, WrapAction, WrapVoidAction, MakeMetaHandler,
// defaultFormatResult, ValidActionsString, MetaToolSchema,
// Route, DestructiveRoute, DeriveAnnotations,
// and composite wrappers (RouteAction, RouteVoidAction,
// RouteActionWithRequest, DestructiveAction, DestructiveVoidAction,
// DestructiveActionWithRequest).
package toolutil

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// Helpers.

// testInput defines parameters for the test operation.
type testInput struct {
	Name string `json:"name"`
	ID   int    `json:"id"`
}

// routeRequestTestInput avoids sharing schema cache state with testInput.
type routeRequestTestInput struct {
	Name string `json:"name"`
	ID   int    `json:"id"`
}

// destructiveFuncTestInput avoids sharing schema cache state with testInput.
type destructiveFuncTestInput struct {
	Name string `json:"name"`
	ID   int    `json:"id"`
}

// testInt64Input defines parameters for the test int64 operation.
type testInt64Input struct {
	ProjectID StringOrInt `json:"project_id"`
	MRIID     int64       `json:"merge_request_iid"`
	Message   string      `json:"message,omitempty"`
}

// testAliasInput defines parameters used to verify common LLM-facing aliases
// are normalized before typed meta-tool input decoding.
type testAliasInput struct {
	Query        string      `json:"query"`
	MRIID        int64       `json:"merge_request_iid"`
	ProjectID    StringOrInt `json:"project_id"`
	LinkURL      string      `json:"link_url"`
	Labels       string      `json:"labels"`
	SourceBranch string      `json:"source_branch"`
	TargetBranch string      `json:"target_branch"`
	Environment  string      `json:"environment_scope"`
	AutoMerge    bool        `json:"auto_merge"`
	Variables    []string    `json:"variables,omitempty"`
}

// testProjectPathInput defines parameters for the test project path operation.
type testProjectPathInput struct {
	ProjectPath string `json:"project_path"`
}

// testGroupPathInput defines parameters for the test group path operation.
type testGroupPathInput struct {
	GroupPath string `json:"group_path"`
}

// testFullPathInput defines parameters for the test full path operation.
type testFullPathInput struct {
	FullPath string `json:"full_path"`
}

// testPausedInput defines parameters for the test paused operation.
type testPausedInput struct {
	Paused bool `json:"paused"`
}

// testPackageFilePathInput defines parameters for the test package file path operation.
type testPackageFilePathInput struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
}

type testAccessEntry struct {
	AccessLevel       *int   `json:"access_level,omitempty"`
	RequiredApprovals *int64 `json:"required_approvals,omitempty"`
}

type testStructuredInput struct {
	PushAccessLevel    int               `json:"push_access_level,omitempty"`
	DeployAccessLevels []testAccessEntry `json:"deploy_access_levels,omitempty"`
	ApprovalRules      []testAccessEntry `json:"approval_rules,omitempty"`
}

type testPaginationInput struct {
	First int `json:"first,omitempty"`
}

type testNoteInput struct {
	NoteID int `json:"note_id"`
}

// testRequiredInput defines parameters for the test required operation.
type testRequiredInput struct {
	Name string `json:"name" jsonschema:"Resource name,required"`
}

// testOutput represents the response from the test operation.
type testOutput struct {
	Result string `json:"result"`
}

// UnmarshalParams tests.

// TestUnmarshalParams verifies successful round-trip from map → JSON → struct.
func TestUnmarshalParams(t *testing.T) {
	params := map[string]any{"name": "proj", "id": float64(42)}
	got, err := UnmarshalParams[testInput](params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "proj" || got.ID != 42 {
		t.Errorf("got %+v, want {Name:proj ID:42}", got)
	}
}

// TestUnmarshalParams_InvalidType verifies UnmarshalParams returns an error
// when the params map contains a value incompatible with the target type.
func TestUnmarshalParams_InvalidType(t *testing.T) {
	params := map[string]any{"id": "not-a-number"}
	_, err := UnmarshalParams[testInput](params)
	if err == nil {
		t.Fatal("expected error for type mismatch, got nil")
	}
}

// TestUnmarshalParams_CoercesStringToInt64 verifies that numeric strings
// like "17" are coerced to int64 values, fixing the common LLM behavior
// of sending numbers as JSON strings.
func TestUnmarshalParams_CoercesStringToInt64(t *testing.T) {
	params := map[string]any{
		"project_id":        "42",
		"merge_request_iid": "17",
		"message":           "merge commit",
	}
	got, err := UnmarshalParams[testInt64Input](params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ProjectID.String() != "42" {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, "42")
	}
	if got.MRIID != 17 {
		t.Errorf("MRIID = %d, want 17", got.MRIID)
	}
	if got.Message != "merge commit" {
		t.Errorf("Message = %q, want %q", got.Message, "merge commit")
	}
}

// TestUnmarshalParams_CoercionNotNeeded verifies that params with correct
// types (numbers as numbers) still work without coercion.
func TestUnmarshalParams_CoercionNotNeeded(t *testing.T) {
	params := map[string]any{
		"project_id":        float64(42),
		"merge_request_iid": float64(17),
	}
	got, err := UnmarshalParams[testInt64Input](params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MRIID != 17 {
		t.Errorf("MRIID = %d, want 17", got.MRIID)
	}
}

// TestUnmarshalParams_CoercionInvalidString verifies that non-numeric strings
// in int64 fields still produce an error after coercion retry.
func TestUnmarshalParams_CoercionInvalidString(t *testing.T) {
	params := map[string]any{
		"project_id":        "my-project",
		"merge_request_iid": "not-a-number",
	}
	_, err := UnmarshalParams[testInt64Input](params)
	if err == nil {
		t.Fatal("expected error for non-numeric string in int64 field")
	}
}

// TestUnmarshalParams_NormalizesCommonAliases verifies that common aliases
// used by LLMs are normalized before strict JSON decoding.
//
// The test sends search, mr_iid, project_path, link, from/to branch aliases,
// environment, legacy auto-merge wording, and scalar variables. It asserts the
// decoded struct receives the canonical fields and normalized collection forms.
func TestUnmarshalParams_NormalizesCommonAliases(t *testing.T) {
	params := map[string]any{
		"search":                       "bug",
		"mr_iid":                       "17",
		"project_path":                 "group/project",
		"link":                         "https://example.test",
		"labels":                       []any{"bug", "urgent"},
		"from":                         "feature",
		"to":                           "main",
		"environment":                  "production",
		"merge_when_pipeline_succeeds": true,
		"variables":                    "DEPLOY_ENV=prod",
	}

	got, err := UnmarshalParams[testAliasInput](params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Query != "bug" || got.MRIID != 17 || got.ProjectID.String() != "group/project" {
		t.Fatalf("basic aliases = %+v, want query bug, MR 17, project group/project", got)
	}
	if got.LinkURL != "https://example.test" || got.Labels != "bug,urgent" {
		t.Fatalf("link/labels aliases = %+v, want link_url and CSV labels", got)
	}
	if got.SourceBranch != "feature" || got.TargetBranch != "main" {
		t.Fatalf("branch aliases = %+v, want feature -> main", got)
	}
	if got.Environment != "production" || !got.AutoMerge {
		t.Fatalf("environment/auto_merge aliases = %+v", got)
	}
	if len(got.Variables) != 1 || got.Variables[0] != "DEPLOY_ENV=prod" {
		t.Fatalf("variables = %#v, want single-item string slice", got.Variables)
	}
}

// TestUnmarshalParams_CanonicalAliasesWin verifies runtime alias normalization
// removes an alias when the canonical field is already present.
func TestUnmarshalParams_CanonicalAliasesWin(t *testing.T) {
	got, err := UnmarshalParams[testAliasInput](map[string]any{
		"query":  "canonical",
		"search": "alias",
	})
	if err != nil {
		t.Fatalf("UnmarshalParams() error = %v", err)
	}
	if got.Query != "canonical" {
		t.Fatalf("Query = %q, want canonical", got.Query)
	}
}

// TestUnmarshalParams_NormalizesActiveAndFilePathAliases verifies runtime-only
// alias branches used by package and schedule style actions.
func TestUnmarshalParams_NormalizesActiveAndFilePathAliases(t *testing.T) {
	paused, err := UnmarshalParams[testPausedInput](map[string]any{"active": false})
	if err != nil {
		t.Fatalf("UnmarshalParams(active) error = %v", err)
	}
	if !paused.Paused {
		t.Fatal("Paused = false, want true when active=false")
	}

	path, err := UnmarshalParams[testPackageFilePathInput](map[string]any{"file_path": "packages/npm/package.tgz"})
	if err != nil {
		t.Fatalf("UnmarshalParams(file_path) error = %v", err)
	}
	if path.Path != "packages/npm" || path.Filename != "package.tgz" {
		t.Fatalf("path = %+v, want packages/npm + package.tgz", path)
	}
}

// TestUnmarshalParams_CoercesStructuredAccessLevelShapes verifies that UnmarshalParams
// coerces human-readable access-level names to their integer codes across scalar fields,
// nested object fields (deploy_access_levels), and object arrays (approval_rules).
func TestUnmarshalParams_CoercesStructuredAccessLevelShapes(t *testing.T) {
	got, err := UnmarshalParams[testStructuredInput](map[string]any{
		"push_access_level":    "Maintainer",
		"deploy_access_levels": map[string]any{"group_access_level": "Developer"},
		"approval_rules":       []any{map[string]any{"access_level": "Maintainer", "required_approval_count": 1}},
	})
	if err != nil {
		t.Fatalf("UnmarshalParams() error = %v", err)
	}
	if got.PushAccessLevel != 40 {
		t.Fatalf("PushAccessLevel = %d, want 40", got.PushAccessLevel)
	}
	if len(got.DeployAccessLevels) != 1 || got.DeployAccessLevels[0].AccessLevel == nil || *got.DeployAccessLevels[0].AccessLevel != 30 {
		t.Fatalf("DeployAccessLevels = %+v, want developer access level", got.DeployAccessLevels)
	}
	if len(got.ApprovalRules) != 1 || got.ApprovalRules[0].AccessLevel == nil || *got.ApprovalRules[0].AccessLevel != 40 || got.ApprovalRules[0].RequiredApprovals == nil || *got.ApprovalRules[0].RequiredApprovals != 1 {
		t.Fatalf("ApprovalRules = %+v, want maintainer required approval", got.ApprovalRules)
	}
}

// TestGitLabRoleAccessLevel_RejectsOutOfRangeIntegers verifies gitLabRoleAccessLevel
// rejects values that overflow the valid access-level range (oversized strings, ints,
// and floats) while still accepting an in-range numeric string.
func TestGitLabRoleAccessLevel_RejectsOutOfRangeIntegers(t *testing.T) {
	for _, value := range []any{"9223372036854775807", int64(1 << 62), float64(1e20)} {
		if got, ok := gitLabRoleAccessLevel(value); ok {
			t.Fatalf("gitLabRoleAccessLevel(%v) = %d, true; want rejected", value, got)
		}
	}

	if got, ok := gitLabRoleAccessLevel("60"); !ok || got != 60 {
		t.Fatalf("gitLabRoleAccessLevel(60) = %d, %t; want 60, true", got, ok)
	}
}

// TestUnmarshalParams_CoercesPaginationBoolean verifies that UnmarshalParams maps a
// boolean pagination flag (first=true) to the default page size rather than failing to
// decode it into the integer pagination field.
func TestUnmarshalParams_CoercesPaginationBoolean(t *testing.T) {
	got, err := UnmarshalParams[testPaginationInput](map[string]any{"first": true})
	if err != nil {
		t.Fatalf("UnmarshalParams() error = %v", err)
	}
	if got.First != 100 {
		t.Fatalf("First = %d, want 100", got.First)
	}
}

// TestUnmarshalParams_DropsDiscussionIDWhenNoteIDIsCanonical verifies that UnmarshalParams
// keeps the canonical note_id and drops a conflicting discussion_id so the decoded input
// carries only the authoritative identifier.
func TestUnmarshalParams_DropsDiscussionIDWhenNoteIDIsCanonical(t *testing.T) {
	got, err := UnmarshalParams[testNoteInput](map[string]any{"note_id": 44, "discussion_id": "abc123"})
	if err != nil {
		t.Fatalf("UnmarshalParams() error = %v", err)
	}
	if got.NoteID != 44 {
		t.Fatalf("NoteID = %d, want 44", got.NoteID)
	}
}

// TestRouteFunc_HandlerPaths verifies RouteFunc executes typed handlers and
// returns decode errors before invoking them when parameters are invalid.
func TestRouteFunc_HandlerPaths(t *testing.T) {
	type routeInput struct {
		Name string `json:"name" jsonschema:"Name,required"`
		ID   int    `json:"id"`
	}
	type routeOutput struct {
		Message string `json:"message"`
	}

	called := false
	route := RouteFunc(func(_ context.Context, input routeInput) (routeOutput, error) {
		called = true
		return routeOutput{Message: input.Name}, nil
	})
	if route.Destructive || route.InputSchema == nil || route.OutputSchema == nil {
		t.Fatalf("route metadata = %+v, want non-destructive route with schemas", route)
	}

	result, err := route.Handler(context.Background(), map[string]any{"name": "project", "id": 7})
	if err != nil {
		t.Fatalf("RouteFunc handler error = %v", err)
	}
	if !called {
		t.Fatal("RouteFunc handler did not invoke typed function")
	}
	out, ok := result.(routeOutput)
	if !ok || out.Message != "project" {
		t.Fatalf("RouteFunc result = %#v, want routeOutput message", result)
	}

	called = false
	if _, err = route.Handler(context.Background(), map[string]any{"id": "not-a-number"}); err == nil {
		t.Fatal("RouteFunc handler error = nil, want decode error")
	}
	if called {
		t.Fatal("RouteFunc invoked typed function after decode error")
	}
}

// TestJSONFieldReflectionHelpers verifies required-field and JSON field-name
// reflection handles embedded structs, anonymous pointers, ignored fields, and
// non-struct inputs.
func TestJSONFieldReflectionHelpers(t *testing.T) {
	type embeddedRequired struct {
		EmbeddedName string `json:"embedded_name" jsonschema:"Embedded name,required"`
	}
	type namedInput struct {
		*embeddedRequired
		Plain      string `jsonschema:"Plain,required"`
		Tagged     string `json:"tagged,omitempty" jsonschema:"Tagged, required"`
		Ignored    string `json:"-" jsonschema:"Ignored,required"`
		_          string
		Anonymous  struct{ Value string }
		StringList []string `json:"string_list,omitempty"`
	}

	required := requiredJSONFieldNames(reflect.TypeFor[namedInput]())
	if !reflect.DeepEqual(required, []string{"Plain", "embedded_name", "tagged"}) {
		t.Fatalf("requiredJSONFieldNames() = %#v", required)
	}
	if got := requiredJSONFieldNames(reflect.TypeFor[*namedInput]()); !reflect.DeepEqual(got, required) {
		t.Fatalf("requiredJSONFieldNames(pointer) = %#v, want %#v", got, required)
	}
	if got := requiredJSONFieldNames(reflect.TypeFor[int]()); got != nil {
		t.Fatalf("requiredJSONFieldNames(non-struct) = %#v, want nil", got)
	}

	fields := jsonFieldNames(reflect.TypeFor[namedInput]())
	for _, want := range []string{"embedded_name", "Plain", "tagged", "Anonymous", "string_list"} {
		if _, ok := fields[want]; !ok {
			t.Fatalf("jsonFieldNames() missing %q in %#v", want, fields)
		}
	}
	for _, unwanted := range []string{"-", "_"} {
		if _, ok := fields[unwanted]; ok {
			t.Fatalf("jsonFieldNames() contains %q in %#v", unwanted, fields)
		}
	}
	if got := jsonFieldNames(reflect.TypeFor[string]()); got != nil {
		t.Fatalf("jsonFieldNames(non-struct) = %#v, want nil", got)
	}
	if got := jsonFieldNames(nil); got != nil {
		t.Fatalf("jsonFieldNames(nil) = %#v, want nil", got)
	}

	// Pointer-to-struct is dereferenced in the for-loop body to expose the
	// underlying struct fields. The result must match the direct value.
	direct := jsonFieldNames(reflect.TypeFor[namedInput]())
	viaPtr := jsonFieldNames(reflect.TypeFor[*namedInput]())
	if !reflect.DeepEqual(direct, viaPtr) {
		t.Fatalf("jsonFieldNames() = direct:%#v viaPtr:%#v, want equal", direct, viaPtr)
	}

	// Empty struct has no JSON field names and so the alias normalizer
	// returns the params untouched. This covers the "len(fields) == 0"
	// branch inside normalizeParamAliases.
	type emptyInput struct{}
	gotParams := normalizeParamAliases(map[string]any{"alias": "x"}, reflect.TypeFor[emptyInput]())
	if gotParams["alias"] != "x" {
		t.Fatalf("normalizeParamAliases(empty struct) = %v, want alias:x untouched", gotParams)
	}

	fieldTypes := jsonFieldTypes(reflect.TypeFor[*namedInput]())
	if fieldTypes["embedded_name"].Kind() != reflect.String || fieldTypes["string_list"].Kind() != reflect.Slice {
		t.Fatalf("jsonFieldTypes() = %#v, want embedded string and string slice", fieldTypes)
	}
	if _, ok := fieldTypes["_"]; ok {
		t.Fatalf("jsonFieldTypes() contains blank field in %#v", fieldTypes)
	}
}

// TestSplitPackageFilePath_EdgeCases verifies package file-path splitting for
// root files, nested paths, and slash-only values.
func TestSplitPackageFilePath_EdgeCases(t *testing.T) {
	tests := []struct {
		path     string
		wantDir  string
		wantFile string
	}{
		{path: "", wantDir: ".", wantFile: ""},
		{path: "/", wantDir: ".", wantFile: ""},
		{path: "package.tgz", wantDir: ".", wantFile: "package.tgz"},
		{path: "/packages/npm/package.tgz/", wantDir: "packages/npm", wantFile: "package.tgz"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			dir, filename := splitPackageFilePath(tt.path)
			if dir != tt.wantDir || filename != tt.wantFile {
				t.Fatalf("splitPackageFilePath(%q) = %q/%q, want %q/%q", tt.path, dir, filename, tt.wantDir, tt.wantFile)
			}
		})
	}
}

// TestUnmarshalParams_CoercesNumericPathAliasesToStrings verifies numeric IDs
// remain usable after alias normalization rewrites them to path-style fields.
func TestUnmarshalParams_CoercesNumericPathAliasesToStrings(t *testing.T) {
	project, err := UnmarshalParams[testProjectPathInput](map[string]any{"project_id": float64(42)})
	if err != nil {
		t.Fatalf("UnmarshalParams(project_id) error = %v", err)
	}
	if project.ProjectPath != "42" {
		t.Fatalf("ProjectPath = %q, want 42", project.ProjectPath)
	}

	group, err := UnmarshalParams[testGroupPathInput](map[string]any{"group_id": float64(7)})
	if err != nil {
		t.Fatalf("UnmarshalParams(group_id) error = %v", err)
	}
	if group.GroupPath != "7" {
		t.Fatalf("GroupPath = %q, want 7", group.GroupPath)
	}

	full, err := UnmarshalParams[testFullPathInput](map[string]any{"group_id": float64(9)})
	if err != nil {
		t.Fatalf("UnmarshalParams(full_path) error = %v", err)
	}
	if full.FullPath != "9" {
		t.Fatalf("FullPath = %q, want 9", full.FullPath)
	}
}

// TestNormalizeParamAliasesForSchema_NormalizesAndCoerces verifies schema-led
// normalization covers common aliases, active/paused inversion, numeric
// coercion, numeric string IDs, string arrays, and comma-separated label
// fields without needing a Go struct type.
func TestNormalizeParamAliasesForSchema_NormalizesAndCoerces(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"query":                    map[string]any{"type": "string"},
			"merge_request_iid":        map[string]any{"type": "integer"},
			"project_id":               map[string]any{"type": "string"},
			"link_url":                 map[string]any{"type": "string"},
			"labels":                   map[string]any{"type": "string"},
			"source_branch":            map[string]any{"type": "string"},
			"target_branch":            map[string]any{"type": "string"},
			"environment_scope":        map[string]any{"type": "string"},
			"auto_merge":               map[string]any{"type": "boolean"},
			"paused":                   map[string]any{"type": "boolean"},
			"assignee_ids":             map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
			"variables":                map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"weight":                   map[string]any{"type": "number"},
			"path":                     map[string]any{"type": "string"},
			"filename":                 map[string]any{"type": "string"},
			"note":                     map[string]any{"type": "string"},
			"destination_storage_name": map[string]any{"type": "string"},
		},
	}
	params := map[string]any{
		"search":                       "bug",
		"mr_iid":                       "17",
		"project_path":                 float64(42),
		"link":                         "https://example.test",
		"labels":                       []any{"bug", "urgent"},
		"from":                         "feature",
		"to":                           "main",
		"environment":                  "production",
		"merge_when_pipeline_succeeds": true,
		"active":                       false,
		"assignee_ids":                 []any{"10", "11"},
		"variables":                    "DEPLOY_ENV=prod",
		"weight":                       "3.5",
		"file_path":                    "packages/npm/package.tgz",
		"body":                         "review note",
		"shard":                        "default",
	}

	got := NormalizeParamAliasesForSchema(params, schema)
	if got["query"] != "bug" || got["merge_request_iid"] != int64(17) || got["project_id"] != "42" {
		t.Fatalf("basic normalized values = %#v", got)
	}
	if got["link_url"] != "https://example.test" || got["labels"] != "bug,urgent" {
		t.Fatalf("link/labels values = %#v", got)
	}
	if got["source_branch"] != "feature" || got["target_branch"] != "main" {
		t.Fatalf("branch values = %#v", got)
	}
	if got["environment_scope"] != "production" || got["auto_merge"] != true || got["paused"] != true {
		t.Fatalf("environment/auto_merge/paused values = %#v", got)
	}
	if !reflect.DeepEqual(got["assignee_ids"], []any{int64(10), int64(11)}) {
		t.Fatalf("assignee_ids = %#v", got["assignee_ids"])
	}
	if !reflect.DeepEqual(got["variables"], []string{"DEPLOY_ENV=prod"}) {
		t.Fatalf("variables = %#v", got["variables"])
	}
	if got["weight"] != 3.5 {
		t.Fatalf("weight = %#v", got["weight"])
	}
	if got["path"] != "packages/npm" || got["filename"] != "package.tgz" {
		t.Fatalf("file_path split values = %#v", got)
	}
	if got["note"] != "review note" || got["destination_storage_name"] != "default" {
		t.Fatalf("note/storage shard values = %#v", got)
	}
	for _, alias := range []string{"search", "mr_iid", "project_path", "link", "from", "to", "environment", "merge_when_pipeline_succeeds", "active", "file_path", "body", "shard"} {
		if _, exists := got[alias]; exists {
			t.Fatalf("alias %q still present in %#v", alias, got)
		}
	}
}

// TestNormalizeParamAliasesForSchemaWithExplanation verifies schema-led alias
// explanations report parameter names without exposing parameter values.
func TestNormalizeParamAliasesForSchemaWithExplanation(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"query":             map[string]any{"type": "string"},
			"merge_request_iid": map[string]any{"type": "integer"},
		},
	}
	params := map[string]any{"search": "private text", "mr_iid": 7}

	normalized, explanations := NormalizeParamAliasesForSchemaWithExplanation(params, schema)
	if normalized["query"] != "private text" || normalized["merge_request_iid"] != 7 {
		t.Fatalf("normalized = %#v, want query and merge_request_iid", normalized)
	}
	if len(explanations) != 2 {
		t.Fatalf("explanations = %+v, want two explanations", explanations)
	}
	for _, explanation := range explanations {
		if explanation.Source != "schema_common" {
			t.Fatalf("explanation = %+v, want schema_common source", explanation)
		}
		if strings.Contains(explanation.Notes, "private") || explanation.Alias == "private text" || explanation.Canonical == "private text" {
			t.Fatalf("explanation = %+v, leaked parameter value", explanation)
		}
	}
}

// TestNormalizeParamAliasesForSchemaWithExplanation_IDAlias verifies NormalizeParamAliasesForSchemaWithExplanation when ID alias.
func TestNormalizeParamAliasesForSchemaWithExplanation_IDAlias(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"name":       map[string]any{"type": "string"},
			"project_id": map[string]any{"type": "string"},
		},
	}
	normalized, explanations := NormalizeParamAliasesForSchemaWithExplanation(map[string]any{"id": "group/project"}, schema)
	if normalized["project_id"] != "group/project" {
		t.Fatalf("normalized = %#v, want project_id alias", normalized)
	}
	if len(explanations) != 1 || explanations[0].Alias != "id" || explanations[0].Canonical != "project_id" {
		t.Fatalf("explanations = %+v, want id -> project_id", explanations)
	}
}

// TestNormalizeParamAliasesForSchemaWithExplanation_IDAliasAmbiguous verifies NormalizeParamAliasesForSchemaWithExplanation when ID alias ambiguous.
func TestNormalizeParamAliasesForSchemaWithExplanation_IDAliasAmbiguous(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
			"group_id":   map[string]any{"type": "string"},
		},
	}
	_, explanations := NormalizeParamAliasesForSchemaWithExplanation(map[string]any{"id": "group/project"}, schema)
	if len(explanations) != 0 {
		t.Fatalf("explanations = %+v, want no ambiguous id alias explanation", explanations)
	}
}

// TestNormalizeParamAliasesForSchemaWithExplanation_EmptyAndRejectedIDAlias verifies NormalizeParamAliasesForSchemaWithExplanation when empty and rejected ID alias.
func TestNormalizeParamAliasesForSchemaWithExplanation_EmptyAndRejectedIDAlias(t *testing.T) {
	if _, explanations := NormalizeParamAliasesForSchemaWithExplanation(nil, map[string]any{"properties": map[string]any{"project_id": map[string]any{"type": "string"}}}); explanations != nil {
		t.Fatalf("explanations = %+v, want nil for empty params", explanations)
	}
	if _, explanations := NormalizeParamAliasesForSchemaWithExplanation(map[string]any{"id": "group/project"}, map[string]any{"properties": map[string]any{"id": map[string]any{"type": "string"}}}); len(explanations) != 0 {
		t.Fatalf("explanations = %+v, want empty when schema accepts id", explanations)
	}
	if _, explanations := NormalizeParamAliasesForSchemaWithExplanation(map[string]any{"id": "group/project"}, map[string]any{"properties": map[string]any{"name": map[string]any{"type": "string"}}}); len(explanations) != 0 {
		t.Fatalf("explanations = %+v, want empty without a canonical _id field", explanations)
	}
}

// TestNormalizeParamAliasesForSchema_CanonicalWins verifies aliases are
// dropped when the canonical parameter is already present and the schema does
// not accept the alias.
func TestNormalizeParamAliasesForSchema_CanonicalWins(t *testing.T) {
	schema := map[string]any{"properties": map[string]any{"query": map[string]any{"type": "string"}}}
	params := map[string]any{"query": "canonical", "search": "alias"}

	got := NormalizeParamAliasesForSchema(params, schema)
	if got["query"] != "canonical" {
		t.Fatalf("query = %#v, want canonical", got["query"])
	}
	if _, exists := got["search"]; exists {
		t.Fatalf("search alias still present: %#v", got)
	}
}

// TestNormalizeParamAliasesForSchema_ObservedDynamicAliases verifies aliases
// seen in dynamic execution traces normalize only when the selected schema
// exposes the canonical field.
func TestNormalizeParamAliasesForSchema_ObservedDynamicAliases(t *testing.T) {
	tests := map[string]struct {
		schema map[string]any
		params map[string]any
		want   map[string]any
	}{
		"id to project_id": {
			schema: map[string]any{"properties": map[string]any{"project_id": map[string]any{"type": "integer"}}},
			params: map[string]any{"id": "42"},
			want:   map[string]any{"project_id": int64(42)},
		},
		"id to group_id": {
			schema: map[string]any{"properties": map[string]any{"group_id": map[string]any{"type": "string"}}},
			params: map[string]any{"id": 99},
			want:   map[string]any{"group_id": "99"},
		},
		"id to user_id": {
			schema: map[string]any{"properties": map[string]any{"user_id": map[string]any{"type": "integer"}}},
			params: map[string]any{"id": "123"},
			want:   map[string]any{"user_id": int64(123)},
		},
		"iid to only iid field": {
			schema: map[string]any{"properties": map[string]any{"epic_iid": map[string]any{"type": "integer"}}},
			params: map[string]any{"iid": "16"},
			want:   map[string]any{"epic_iid": int64(16)},
		},
		"ambiguous id is preserved": {
			schema: map[string]any{"properties": map[string]any{"project_id": map[string]any{"type": "integer"}, "group_id": map[string]any{"type": "integer"}}},
			params: map[string]any{"id": "42"},
			want:   map[string]any{"id": "42"},
		},
		"ambiguous iid is preserved": {
			schema: map[string]any{"properties": map[string]any{"epic_iid": map[string]any{"type": "integer"}, "child_iid": map[string]any{"type": "integer"}}},
			params: map[string]any{"iid": "42"},
			want:   map[string]any{"iid": "42"},
		},
		"branch to branch_name": {
			schema: map[string]any{"properties": map[string]any{"branch_name": map[string]any{"type": "string"}}},
			params: map[string]any{"branch": "main"},
			want:   map[string]any{"branch_name": "main"},
		},
		"branch to ref": {
			schema: map[string]any{"properties": map[string]any{"ref": map[string]any{"type": "string"}}},
			params: map[string]any{"branch": "main"},
			want:   map[string]any{"ref": "main"},
		},
		"feature_flag_name to name": {
			schema: map[string]any{"properties": map[string]any{"name": map[string]any{"type": "string"}}},
			params: map[string]any{"feature_flag_name": "eval-flag"},
			want:   map[string]any{"name": "eval-flag"},
		},
		"emoji_name to name": {
			schema: map[string]any{"properties": map[string]any{"name": map[string]any{"type": "string"}}},
			params: map[string]any{"emoji_name": "eyes"},
			want:   map[string]any{"name": "eyes"},
		},
		"award_emoji to name": {
			schema: map[string]any{"properties": map[string]any{"name": map[string]any{"type": "string"}}},
			params: map[string]any{"award_emoji": "eyes"},
			want:   map[string]any{"name": "eyes"},
		},
		"award to name": {
			schema: map[string]any{"properties": map[string]any{"name": map[string]any{"type": "string"}}},
			params: map[string]any{"award": "thumbsup"},
			want:   map[string]any{"name": "thumbsup"},
		},
		"time_estimate to duration": {
			schema: map[string]any{"properties": map[string]any{"duration": map[string]any{"type": "string"}}},
			params: map[string]any{"time_estimate": "1h"},
			want:   map[string]any{"duration": "1h"},
		},
		"environment to name when schema only names protected environment": {
			schema: map[string]any{"properties": map[string]any{"name": map[string]any{"type": "string"}}},
			params: map[string]any{"environment": "staging"},
			want:   map[string]any{"name": "staging"},
		},
		"environment_id to environment when schema names protected environment": {
			schema: map[string]any{"properties": map[string]any{"environment": map[string]any{"type": "string"}}},
			params: map[string]any{"environment_id": "staging"},
			want:   map[string]any{"environment": "staging"},
		},
		"environment_id is preserved when schema accepts environment_id": {
			schema: map[string]any{"properties": map[string]any{"environment_id": map[string]any{"type": "integer"}}},
			params: map[string]any{"environment_id": "42"},
			want:   map[string]any{"environment_id": int64(42)},
		},
		"url encoded project path is decoded": {
			schema: map[string]any{"properties": map[string]any{"project_id": map[string]any{"type": "string"}}},
			params: map[string]any{"project_id": "my-org%2Ftools%2Fgitlab-mcp-server"},
			want:   map[string]any{"project_id": "my-org/tools/gitlab-mcp-server"},
		},
		"alias is preserved when schema accepts alias": {
			schema: map[string]any{"properties": map[string]any{"name": map[string]any{"type": "string"}, "emoji_name": map[string]any{"type": "string"}}},
			params: map[string]any{"emoji_name": "eyes"},
			want:   map[string]any{"emoji_name": "eyes"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := NormalizeParamAliasesForSchema(tc.params, tc.schema)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("NormalizeParamAliasesForSchema() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestNormalizeParamAliasesForSchema_IgnoresSchemasWithoutProperties verifies
// params are returned unchanged when no schema properties are available.
func TestNormalizeParamAliasesForSchema_IgnoresSchemasWithoutProperties(t *testing.T) {
	params := map[string]any{"search": "bug"}
	got := NormalizeParamAliasesForSchema(params, map[string]any{"type": "object"})
	if !reflect.DeepEqual(got, params) {
		t.Fatalf("got %#v, want unchanged %#v", got, params)
	}
}

// TestRequiredMissingAndUnknownParamNames_SchemaValidation_ReturnsSortedMissingAndUnknown verifies required parameter sorting,
// missing required detection, and unknown parameter detection from JSON Schema
// properties.
func TestRequiredMissingAndUnknownParamNames_SchemaValidation_ReturnsSortedMissingAndUnknown(t *testing.T) {
	schema := map[string]any{
		"required": []any{"project_id", "name", ""},
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
			"name":       map[string]any{"type": "string"},
		},
	}
	if got := requiredParamNames(schema); !reflect.DeepEqual(got, []string{"name", "project_id"}) {
		t.Fatalf("requiredParamNames() = %#v", got)
	}
	if got := missingRequiredParamNames(schema, map[string]any{"project_id": "group/project"}); !reflect.DeepEqual(got, []string{"name"}) {
		t.Fatalf("missingRequiredParamNames() = %#v", got)
	}
	if !hasUnknownParamNames(schema, map[string]any{"unknown": true}) {
		t.Fatal("hasUnknownParamNames() = false, want true")
	}
	if hasUnknownParamNames(schema, map[string]any{"name": "value"}) {
		t.Fatal("known params reported as unknown")
	}
}

// TestUnmarshalParams_RejectsUnknownField verifies that params containing a
// key that is not declared on the target type produce an actionable error
// (mirroring the JSON Schema additionalProperties:false lockdown applied to
// tools/list responses) so an LLM that mistypes a parameter name receives a
// clear "unknown field" diagnostic instead of having the value silently
// dropped.
func TestUnmarshalParams_RejectsUnknownField(t *testing.T) {
	params := map[string]any{
		"name":           "proj",
		"id":             float64(42),
		"unknown_field!": "should-fail",
	}
	_, err := UnmarshalParams[testInput](params)
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("expected error to mention 'unknown field', got: %v", err)
	}
}

// TestCoerceNumericStrings verifies the coercion helper directly.
func TestCoerceNumericStrings(t *testing.T) {
	params := map[string]any{
		"int_val":    "42",
		"float_val":  "3.14",
		"str_val":    "hello",
		"number_val": float64(99),
		"bool_val":   true,
	}
	got := coerceNumericStrings(params)

	if v, ok := got["int_val"].(int64); !ok || v != 42 {
		t.Errorf("int_val = %v (%T), want int64(42)", got["int_val"], got["int_val"])
	}
	if v, ok := got["float_val"].(float64); !ok || v != 3.14 {
		t.Errorf("float_val = %v (%T), want float64(3.14)", got["float_val"], got["float_val"])
	}
	if v, ok := got["str_val"].(string); !ok || v != "hello" {
		t.Errorf("str_val = %v (%T), want string(hello)", got["str_val"], got["str_val"])
	}
	if v, ok := got["number_val"].(float64); !ok || v != 99 {
		t.Errorf("number_val = %v (%T), want float64(99)", got["number_val"], got["number_val"])
	}
	if v, ok := got["bool_val"].(bool); !ok || !v {
		t.Errorf("bool_val = %v (%T), want bool(true)", got["bool_val"], got["bool_val"])
	}
}

// TestCoercionHelpers_CoverNumericAndSchemaBranches verifies lower-level
// coercion helpers across integer, unsigned, float, slice, and schema paths.
func TestCoercionHelpers_CoverNumericAndSchemaBranches(t *testing.T) {
	assertNumericIDStringBranches(t)
	assertNumericValueCoercionBranches(t)
	assertSliceValueCoercionBranches(t)
	assertSchemaCoercionBranches(t)
	assertStringListAndJSONHelpers(t)
}

func assertNumericIDStringBranches(t *testing.T) {
	t.Helper()
	numericValues := []any{int(1), int8(2), int16(3), int32(4), int64(5), uint(6), uint8(7), uint16(8), uint32(9), uint64(10), json.Number("11"), float32(12), float64(13)}
	for _, value := range numericValues {
		if text, ok := numericIDString(value); !ok || text == "" {
			t.Fatalf("numericIDString(%T %[1]v) = %q/%v, want numeric string", value, text, ok)
		}
	}
	for _, value := range []any{json.Number("1.5"), float64(1.2), "12"} {
		if text, ok := numericIDString(value); ok || text != "" {
			t.Fatalf("numericIDString(%T %[1]v) = %q/%v, want empty false", value, text, ok)
		}
	}
	if text, ok := integerFloatString(1.5); ok || text != "" {
		t.Fatalf("integerFloatString(1.5) = %q/%v, want empty false", text, ok)
	}
}

func assertNumericValueCoercionBranches(t *testing.T) {
	t.Helper()
	unsigned, changed, err := coerceUnsignedIntegerValue("count", "7")
	if err != nil || !changed || unsigned != uint64(7) {
		t.Fatalf("coerceUnsignedIntegerValue() = %#v/%v/%v", unsigned, changed, err)
	}
	for _, value := range []string{"-1", "bad"} {
		if _, _, unsignedErr := coerceUnsignedIntegerValue("count", value); unsignedErr == nil {
			t.Fatalf("coerceUnsignedIntegerValue(%q) error = nil, want error", value)
		}
	}
	floatValue, changed, err := coerceFloatValue("weight", "3.5")
	if err != nil || !changed || floatValue != 3.5 {
		t.Fatalf("coerceFloatValue() = %#v/%v/%v", floatValue, changed, err)
	}
	if _, _, floatErr := coerceFloatValue("weight", "bad"); floatErr == nil {
		t.Fatal("coerceFloatValue(bad) error = nil, want error")
	}

	if value, valueChanged, valueErr := coerceValueForTargetType("count", "8", reflect.TypeFor[uint]()); valueErr != nil || !valueChanged || value != uint64(8) {
		t.Fatalf("coerceValueForTargetType(uint) = %#v/%v/%v", value, valueChanged, valueErr)
	}
	if value, valueChanged, valueErr := coerceValueForTargetType("weight", "4.25", reflect.TypeFor[*float64]()); valueErr != nil || !valueChanged || value != 4.25 {
		t.Fatalf("coerceValueForTargetType(*float64) = %#v/%v/%v", value, valueChanged, valueErr)
	}
	if value, valueChanged, valueErr := coerceValueForTargetType("name", "project", reflect.TypeFor[string]()); valueErr != nil || valueChanged || value != "project" {
		t.Fatalf("coerceValueForTargetType(string) = %#v/%v/%v", value, valueChanged, valueErr)
	}
}

func assertSliceValueCoercionBranches(t *testing.T) {
	t.Helper()
	sliceValue, changed, err := coerceSliceValueForTargetType("ids", []string{"1", "2"}, reflect.TypeFor[int64]())
	if err != nil || !changed || !reflect.DeepEqual(sliceValue, []any{int64(1), int64(2)}) {
		t.Fatalf("coerceSliceValueForTargetType() = %#v/%v/%v", sliceValue, changed, err)
	}
	if _, _, sliceErr := coerceSliceValueForTargetType("ids", []any{"bad"}, reflect.TypeFor[int64]()); sliceErr == nil {
		t.Fatal("coerceSliceValueForTargetType(bad) error = nil, want error")
	}
	if value, sliceChanged, sliceErr := coerceSliceValueForTargetType("names", []string{"a"}, reflect.TypeFor[string]()); sliceErr != nil || sliceChanged || !reflect.DeepEqual(value, []string{"a"}) {
		t.Fatalf("coerceSliceValueForTargetType(non-numeric) = %#v/%v/%v", value, sliceChanged, sliceErr)
	}
	if value, valueChanged, valueErr := coerceValueForTargetType("ids", []any{int64(1)}, reflect.TypeFor[[]int64]()); valueErr != nil || valueChanged || !reflect.DeepEqual(value, []any{int64(1)}) {
		t.Fatalf("coerceValueForTargetType([]int64 unchanged) = %#v/%v/%v", value, valueChanged, valueErr)
	}
	if items, ok := sliceItems(42); ok || items != nil {
		t.Fatalf("sliceItems(non-slice) = %#v/%v, want nil false", items, ok)
	}
}

func assertSchemaCoercionBranches(t *testing.T) {
	t.Helper()
	integerArraySchema := map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}
	arrayValue, changed := coerceSchemaArrayValue([]string{"1", "2"}, integerArraySchema)
	if !changed || !reflect.DeepEqual(arrayValue, []any{int64(1), int64(2)}) {
		t.Fatalf("coerceSchemaArrayValue() = %#v/%v", arrayValue, changed)
	}
	for _, property := range []any{"not-map", map[string]any{}, map[string]any{"items": map[string]any{"type": "string"}}} {
		if value, arrayChanged := coerceSchemaArrayValue([]string{"1"}, property); arrayChanged || !reflect.DeepEqual(value, []string{"1"}) {
			t.Fatalf("coerceSchemaArrayValue(%#v) = %#v/%v, want unchanged", property, value, arrayChanged)
		}
	}
	if !schemaPropertyHasType(map[string]any{"type": []string{"integer", "string"}}, "string") {
		t.Fatal("schemaPropertyHasType([]string) = false, want true")
	}
	if !schemaPropertyHasType(map[string]any{"type": []any{"integer", "string"}}, "integer") {
		t.Fatal("schemaPropertyHasType([]any) = false, want true")
	}
	if schemaPropertyHasType("not-map", "string") {
		t.Fatal("schemaPropertyHasType(non-map) = true, want false")
	}
	if schemaPropertyIsStringArray("not-map") || schemaPropertyIsStringArray(map[string]any{"type": "array"}) {
		t.Fatal("schemaPropertyIsStringArray() accepted invalid schema")
	}
	if schemaPropertyIsString("not-map") {
		t.Fatal("schemaPropertyIsString(non-map) = true, want false")
	}
}

func assertStringListAndJSONHelpers(t *testing.T) {
	t.Helper()
	if value, integerErr := integerFromString("3.0"); integerErr != nil || value != 3 {
		t.Fatalf("integerFromString(3.0) = %d/%v, want 3", value, integerErr)
	}
	if _, emptyErr := integerFromString(""); emptyErr == nil {
		t.Fatal("integerFromString(empty) error = nil, want error")
	}
	for _, value := range []any{[]any{"a", 2}, 42} {
		if csv, ok := stringListToCSV(value); ok || csv != "" {
			t.Fatalf("stringListToCSV(%#v) = %q/%v, want empty false", value, csv, ok)
		}
	}
	params := map[string]any{"labels": "bug"}
	if got := coerceSingleStringArraysForSchema(params, map[string]any{}); !reflect.DeepEqual(got, params) {
		t.Fatalf("coerceSingleStringArraysForSchema(no props) = %#v", got)
	}
	if got := coerceStringListParamsForSchema(params, map[string]any{}); !reflect.DeepEqual(got, params) {
		t.Fatalf("coerceStringListParamsForSchema(no props) = %#v", got)
	}
	if jsonFieldTypes(nil) != nil || jsonFieldTypes(reflect.TypeFor[int]()) != nil {
		t.Fatal("jsonFieldTypes(non-struct) returned non-nil")
	}
}

// WrapAction / WrapVoidAction tests.

// TestWrapAction verifies that WrapAction produces an ActionFunc that
// deserializes params, calls the typed handler, and returns its result.
func TestWrapAction(t *testing.T) {
	fn := func(_ context.Context, _ *gitlabclient.Client, in testInput) (testOutput, error) {
		return testOutput{Result: "hello " + in.Name}, nil
	}
	action := WrapAction(nil, fn)
	got, err := action(context.Background(), map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, ok := got.(testOutput)
	if !ok {
		t.Fatalf("result type = %T, want testOutput", got)
	}
	if out.Result != "hello world" {
		t.Errorf("Result = %q, want %q", out.Result, "hello world")
	}
}

// TestWrapAction_UnmarshalError verifies WrapAction returns an error when
// params cannot be deserialized into the input struct.
func TestWrapAction_UnmarshalError(t *testing.T) {
	fn := func(_ context.Context, _ *gitlabclient.Client, in testInput) (testOutput, error) {
		return testOutput{}, nil
	}
	action := WrapAction(nil, fn)
	_, err := action(context.Background(), map[string]any{"id": "bad"})
	if err == nil {
		t.Fatal("expected error for bad params, got nil")
	}
}

// TestWrapVoidAction verifies that WrapVoidAction wraps a void handler
// and returns nil result on success.
func TestWrapVoidAction(t *testing.T) {
	called := false
	fn := func(_ context.Context, _ *gitlabclient.Client, in testInput) error {
		called = true
		return nil
	}
	action := WrapVoidAction(nil, fn)
	got, err := action(context.Background(), map[string]any{"name": "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil result, got %v", got)
	}
	if !called {
		t.Error("handler was not called")
	}
}

// TestWrapVoidAction_UnmarshalError verifies WrapVoidAction returns an error
// when params cannot be deserialized.
func TestWrapVoidAction_UnmarshalError(t *testing.T) {
	fn := func(_ context.Context, _ *gitlabclient.Client, in testInput) error {
		return nil
	}
	action := WrapVoidAction(nil, fn)
	_, err := action(context.Background(), map[string]any{"id": "bad"})
	if err == nil {
		t.Fatal("expected error for bad params, got nil")
	}
}

// TestWrapActionWithRequest verifies that WrapActionWithRequest extracts the
// MCP request from context and passes it to the handler.
func TestWrapActionWithRequest(t *testing.T) {
	var gotReq *mcp.CallToolRequest
	fn := func(_ context.Context, req *mcp.CallToolRequest, _ *gitlabclient.Client, in testInput) (testOutput, error) {
		gotReq = req
		return testOutput{Result: "hello " + in.Name}, nil
	}
	action := WrapActionWithRequest(nil, fn)

	fakeReq := &mcp.CallToolRequest{}
	ctx := ContextWithRequest(context.Background(), fakeReq)
	got, err := action(ctx, map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotReq != fakeReq {
		t.Error("expected handler to receive the request from context")
	}
	out, ok := got.(testOutput)
	if !ok {
		t.Fatalf("result type = %T, want testOutput", got)
	}
	if out.Result != "hello world" {
		t.Errorf("Result = %q, want %q", out.Result, "hello world")
	}
}

// TestWrapActionWithRequest_NilContext verifies that WrapActionWithRequest
// passes nil when no request is stored in context.
func TestWrapActionWithRequest_NilContext(t *testing.T) {
	var gotReq *mcp.CallToolRequest
	fn := func(_ context.Context, req *mcp.CallToolRequest, _ *gitlabclient.Client, in testInput) (testOutput, error) {
		gotReq = req
		return testOutput{Result: "ok"}, nil
	}
	action := WrapActionWithRequest(nil, fn)
	_, err := action(context.Background(), map[string]any{"name": "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotReq != nil {
		t.Error("expected nil request when context has no request")
	}
}

// TestRequestFromContext_Absent verifies that RequestFromContext returns nil
// when no request is stored in the context.
func TestRequestFromContext_Absent(t *testing.T) {
	if RequestFromContext(context.Background()) != nil {
		t.Error("expected nil from empty context")
	}
}

// MakeMetaHandler.

// TestMakeMetaHandler_ValidAction verifies MakeMetaHandler dispatches to
// the correct action handler and returns a formatted result.
func TestMakeMetaHandler_ValidAction(t *testing.T) {
	routes := ActionMap{
		"greet": Route(func(_ context.Context, params map[string]any) (any, error) {
			return map[string]string{"msg": "hi"}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	req := &mcp.CallToolRequest{}
	input := MetaToolInput{Action: "greet", Params: map[string]any{}}
	result, raw, err := handler(context.Background(), req, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	m, ok := raw.(map[string]string)
	if !ok || m["msg"] != "hi" {
		t.Errorf("raw = %v, want map[msg:hi]", raw)
	}
}

// TestMakeMetaHandler_EmptyAction verifies MakeMetaHandler returns an error
// when the action field is empty.
func TestMakeMetaHandler_EmptyAction(t *testing.T) {
	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return struct{}{}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	result, raw, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{})
	if err != nil {
		t.Fatalf("unexpected protocol error: %v", err)
	}
	if raw != nil {
		t.Fatalf("raw result = %#v, want nil", raw)
	}
	if got := metaErrorText(t, result); got != "test_tool: 'action' is required. Valid actions: list" {
		t.Fatalf("error text = %q", got)
	}
}

// TestMakeMetaHandler_UnknownAction verifies MakeMetaHandler returns an error
// for an unrecognized action name.
func TestMakeMetaHandler_UnknownAction(t *testing.T) {
	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return struct{}{}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	result, raw, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{Action: "bogus"})
	if err != nil {
		t.Fatalf("unexpected protocol error: %v", err)
	}
	if raw != nil {
		t.Fatalf("raw result = %#v, want nil", raw)
	}
	if got := metaErrorText(t, result); got != `test_tool: unknown action "bogus". Valid actions: list` {
		t.Fatalf("error text = %q", got)
	}
}

// TestMakeMetaHandler_ActionAlias verifies that dotted action aliases resolve
// to canonical meta-tool route names.
//
// The test registers project.milestone_list and calls the handler with
// milestone.list, then asserts the typed output is returned. This protects the
// compatibility layer for user-facing action names.
func TestMakeMetaHandler_ActionAlias(t *testing.T) {
	routes := ActionMap{
		"project.milestone_list": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return testOutput{Result: "ok"}, nil
		}),
	}
	handler := MakeMetaHandler("gitlab_project", routes, nil)

	_, raw, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{Action: "milestone.list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, ok := raw.(testOutput)
	if !ok || out.Result != "ok" {
		t.Fatalf("raw = %#v, want test output", raw)
	}
}

// TestMakeMetaHandler_EnvironmentGetByNameUsesProtectedGet verifies that a
// named protected environment fetch is not routed to the numeric environment
// get action when models send the generic get action.
func TestMakeMetaHandler_EnvironmentGetByNameUsesProtectedGet(t *testing.T) {
	routes := ActionMap{
		"get": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return testOutput{Result: "environment-get"}, nil
		}),
		"protected_get": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return testOutput{Result: "protected-get"}, nil
		}),
	}
	handler := MakeMetaHandler("gitlab_environment", routes, nil)

	_, raw, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{Action: "get", Params: map[string]any{"environment_id": "staging"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, ok := raw.(testOutput)
	if !ok || out.Result != "protected-get" {
		t.Fatalf("raw = %#v, want protected-get output", raw)
	}

	_, raw, err = handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{Action: "get", Params: map[string]any{"environment_id": "42"}})
	if err != nil {
		t.Fatalf("unexpected numeric get error: %v", err)
	}
	out, ok = raw.(testOutput)
	if !ok || out.Result != "environment-get" {
		t.Fatalf("numeric raw = %#v, want environment-get output", raw)
	}
}

// TestNormalizeActionAlias_DynamicCompatibilityAliases verifies dynamic-surface
// compatibility aliases map to canonical meta-tool action IDs.
func TestNormalizeActionAlias_DynamicCompatibilityAliases(t *testing.T) {
	routes := ActionMap{
		"storage_move.schedule_project":      {},
		"mr_review.changes_get":              {},
		"mr_review.draft_note_publish_all":   {},
		"package.list":                       {},
		"project.hook_list":                  {},
		"external_status_check.list_project": {},
		"access.deploy_token_create_project": {},
		"project.member_delete":              {},
		"project.member_edit":                {},
		"merge_request.spent_time_add":       {},
		"merge_request.time_estimate_set":    {},
		"job.token_scope_list_inbound":       {},
		"package.file_list":                  {},
		"audit_event.list_group":             {},
		"release.list":                       {},
		"ci_variable.create":                 {},
		"ci_variable.group_create":           {},
		"access.deploy_key_add":              {},
		"branch.update_protected":            {},
		"release.link_create":                {},
		"feature_flags.ff_user_list_create":  {},
		"feature_flags.ff_user_list_delete":  {},
		"feature_flags.ff_user_list_list":    {},
		"issue.create":                       {},
		"server.health_check":                {},
		"job.download_single_artifact":       {},
		"issue.link_create":                  {},
		"issue.note_delete":                  {},
		"issue.note_get":                     {},
		"issue.note_list":                    {},
		"issue.note_update":                  {},
		"repository.tree":                    {},
		"repository.file_get":                {},
		"repository.file_raw":                {},
		"pipeline.schedule_create_variable":  {},
		"pipeline.schedule_delete_variable":  {},
		"pipeline.schedule_edit_variable":    {},
		"project.badge_edit":                 {},
		"release.link_list":                  {},
		"merge_request.emoji_mr_create":      {},
		"merge_request.emoji_mr_delete":      {},
		"merge_request.spent_time_reset":     {},
		"issue.note_create":                  {},
		"interactive.issue_create":           {},
		"epic_board_list":                    {},
		"epic_discussion_update_note":        {},
		"epic_discussion_delete_note":        {},
	}

	tests := map[string]string{
		"project.schedule_storage_move":              "storage_move.schedule_project",
		"merge_request.changes":                      "mr_review.changes_get",
		"project.hooks.list":                         "project.hook_list",
		"project.status_check_list":                  "external_status_check.list_project",
		"project.status_checks.list":                 "external_status_check.list_project",
		"ci_job_token_scope.inbound_allowlist.list":  "job.token_scope_list_inbound",
		"deploy_token.create":                        "access.deploy_token_create_project",
		"deploy_key.create":                          "access.deploy_key_add",
		"branch.update_protection":                   "branch.update_protected",
		"project_member.update":                      "project.member_edit",
		"project_member.edit":                        "project.member_edit",
		"project.member_remove":                      "project.member_delete",
		"project_member.remove":                      "project.member_delete",
		"mr_review.draft_notes_publish":              "mr_review.draft_note_publish_all",
		"mr_review.publish":                          "mr_review.draft_note_publish_all",
		"package.list_generic":                       "package.list",
		"package.files":                              "package.file_list",
		"group.audit_events":                         "audit_event.list_group",
		"project.releases.list":                      "release.list",
		"release.asset_link.create":                  "release.link_create",
		"variable.create":                            "ci_variable.create",
		"group.variable.create":                      "ci_variable.group_create",
		"merge_request.add_spent_time":               "merge_request.spent_time_add",
		"merge_request.set_time_estimate":            "merge_request.time_estimate_set",
		"merge_request.time_estimate":                "merge_request.time_estimate_set",
		"merge_request.time_spent_add":               "merge_request.spent_time_add",
		"feature_flag_user_list.create":              "feature_flags.ff_user_list_create",
		"feature_flag_user_list.delete":              "feature_flags.ff_user_list_delete",
		"feature_flags.feature_flag_user_list":       "feature_flags.ff_user_list_list",
		"feature_flags.feature_flag_user_list_list":  "feature_flags.ff_user_list_list",
		"feature_flags.feature_flag_user_lists_list": "feature_flags.ff_user_list_list",
		"gitlab_issue.create":                        "issue.create",
		"gitlab_server.health_check":                 "server.health_check",
		"job.artifact_download":                      "job.download_single_artifact",
		"issue.link":                                 "issue.link_create",
		"issue.note.create":                          "issue.note_create",
		"issue.note.delete":                          "issue.note_delete",
		"issue.note.get":                             "issue.note_get",
		"issue.note.list":                            "issue.note_list",
		"issue.note.update":                          "issue.note_update",
		"repository_tree":                            "repository.tree",
		"repository_tree.list":                       "repository.tree",
		"repository_file.get":                        "repository.file_get",
		"repository_file.read":                       "repository.file_get",
		"repository_files.get_raw_file":              "repository.file_raw",
		"pipeline.schedule_variable_create":          "pipeline.schedule_create_variable",
		"pipeline.schedule_variable_delete":          "pipeline.schedule_delete_variable",
		"pipeline.schedule_variable_update":          "pipeline.schedule_edit_variable",
		"project.badge_update":                       "project.badge_edit",
		"release.create_link":                        "release.link_create",
		"release_link.link_list":                     "release.link_list",
		"merge_request.emoji_mr_award_create":        "merge_request.emoji_mr_create",
		"merge_request.emoji_mr_award_delete":        "merge_request.emoji_mr_delete",
		"merge_request.time_spent_reset":             "merge_request.spent_time_reset",
		"generic_package.list":                       "package.list",
		"issue_note.create":                          "issue.note_create",
		"issue_note.delete":                          "issue.note_delete",
		"issue_note.get":                             "issue.note_get",
		"issue_note.list":                            "issue.note_list",
		"issue_note.update":                          "issue.note_update",
		"gitlab_interactive_issue.create":            "interactive.issue_create",
		"group_board_list":                           "epic_board_list",
		"epic_discussion_note_update":                "epic_discussion_update_note",
		"epic_discussion_note_delete":                "epic_discussion_delete_note",
	}
	for alias, want := range tests {
		t.Run(alias, func(t *testing.T) {
			if got := NormalizeActionAlias(alias, routes); got != want {
				t.Fatalf("NormalizeActionAlias(%q) = %q, want %q", alias, got, want)
			}
		})
	}
	if got := NormalizeActionAlias("repository_file.read", ActionMap{}); got != "repository_file.read" {
		t.Fatalf("NormalizeActionAlias without canonical route = %q, want unchanged", got)
	}
	if got := NormalizeActionAlias("", routes); got != "" {
		t.Fatalf("NormalizeActionAlias empty action = %q, want empty", got)
	}
}

// TestParamValidationError_Unwrap verifies ParamValidationError when unwrap.
func TestParamValidationError_Unwrap(t *testing.T) {
	if got := (*ParamValidationError)(nil).Unwrap(); got != nil {
		t.Fatalf("nil ParamValidationError unwrap = %v, want nil", got)
	}
	baseErr := errors.New("decode failed")
	validationErr := &ParamValidationError{Err: baseErr}
	if !errors.Is(validationErr, baseErr) {
		t.Fatalf("errors.Is(%v, %v) = false, want true", validationErr, baseErr)
	}
	if got := (&ParamValidationError{}).Error(); got != "invalid params" {
		t.Fatalf("empty ParamValidationError error = %q, want invalid params", got)
	}
}

// TestMakeMetaHandler_ParamValidationErrorIsToolError verifies that parameter
// type errors are returned as MCP tool errors instead of protocol errors.
//
// The test sends an invalid string for an integer field and asserts that the
// handler returns an IsError result containing the action-specific validation
// message while leaving the raw output nil.
func TestMakeMetaHandler_ParamValidationErrorIsToolError(t *testing.T) {
	routes := ActionMap{
		"create": RouteAction[testInput, testOutput](nil, func(context.Context, *gitlabclient.Client, testInput) (testOutput, error) {
			return testOutput{}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)

	result, raw, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{
		Action: "create",
		Params: map[string]any{"id": "not-an-int"},
	})
	if err != nil {
		t.Fatalf("unexpected protocol error: %v", err)
	}
	if raw != nil {
		t.Fatalf("raw result = %#v, want nil", raw)
	}
	if got := metaErrorText(t, result); !strings.Contains(got, `test_tool/create: invalid params for this action`) {
		t.Fatalf("error text = %q, want validation tool error", got)
	}
}

// TestMakeMetaHandler_MissingRequiredParamsIsToolError verifies that missing
// required nested params are reported as MCP tool errors.
//
// The test registers a route with a required name field, omits params entirely,
// and asserts the returned tool-error text points at both params and name.
func TestMakeMetaHandler_MissingRequiredParamsIsToolError(t *testing.T) {
	routes := ActionMap{
		"create": RouteAction[testRequiredInput, testOutput](nil, func(context.Context, *gitlabclient.Client, testRequiredInput) (testOutput, error) {
			return testOutput{}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)

	result, raw, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{Action: "create"})
	if err != nil {
		t.Fatalf("unexpected protocol error: %v", err)
	}
	if raw != nil {
		t.Fatalf("raw result = %#v, want nil", raw)
	}
	got := metaErrorText(t, result)
	if !strings.Contains(got, "params' is required") || !strings.Contains(got, "name") {
		t.Fatalf("error text = %q, want required params", got)
	}
}

// metaErrorText supports meta error text assertions in toolutil tests.
func metaErrorText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil || !result.IsError {
		t.Fatalf("result = %#v, want IsError result", result)
	}
	if len(result.Content) == 0 {
		t.Fatal("error result content is empty")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] = %T, want TextContent", result.Content[0])
	}
	return text.Text
}

// TestMakeMetaHandler_CustomFormatter verifies MakeMetaHandler uses a custom
// FormatResultFunc when provided.
func TestMakeMetaHandler_CustomFormatter(t *testing.T) {
	routes := ActionMap{
		"ping": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return "pong", nil
		}),
	}
	customFmt := func(raw any) *mcp.CallToolResult {
		return SuccessResult("CUSTOM:" + raw.(string))
	}
	handler := MakeMetaHandler("test_tool", routes, customFmt)
	result, _, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{Action: "ping"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok || tc.Text != "CUSTOM:pong" {
		t.Errorf("result text = %q, want %q", tc.Text, "CUSTOM:pong")
	}
}

// TestMakeMetaHandler_NilFormatterResult_UsesDefaultFormatter verifies that a
// nil custom formatter result falls back to the default formatter.
func TestMakeMetaHandler_NilFormatterResult_UsesDefaultFormatter(t *testing.T) {
	routes := ActionMap{
		"stats": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return map[string]int{"count": 5}, nil
		}),
	}
	formatter := func(any) *mcp.CallToolResult {
		return nil
	}
	handler := MakeMetaHandler("test_tool", routes, formatter)

	result, raw, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{Action: "stats"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("result = %#v, want successful fallback result", result)
	}
	if len(result.Content) == 0 {
		t.Fatal("fallback result content is empty")
	}
	m, ok := raw.(map[string]int)
	if !ok || m["count"] != 5 {
		t.Fatalf("structured content = %#v, want count 5", raw)
	}
}

// TestMakeMetaHandler_IsErrorResult_OmitsStructuredContent verifies that error
// formatter results do not expose structured content.
func TestMakeMetaHandler_IsErrorResult_OmitsStructuredContent(t *testing.T) {
	routes := ActionMap{
		"blocked": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return map[string]string{"status": "blocked"}, nil
		}),
	}
	formatter := func(any) *mcp.CallToolResult {
		return ErrorResult("blocked")
	}
	handler := MakeMetaHandler("test_tool", routes, formatter)

	result, raw, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{Action: "blocked"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("result = %#v, want IsError result", result)
	}
	if raw != nil {
		t.Fatalf("structured content = %#v, want nil for IsError result", raw)
	}
}

// defaultFormatResult.

// TestDefaultFormatResult_NilResult verifies "ok" text for nil result.
func TestDefaultFormatResult_Nil(t *testing.T) {
	got := defaultFormatResult(nil)
	tc := got.Content[0].(*mcp.TextContent)
	if tc.Text != "ok" {
		t.Errorf("text = %q, want %q", tc.Text, "ok")
	}
}

// TestDefaultFormatResult_JSONResult verifies JSON serialization for non-nil.
func TestDefaultFormatResult_JSON(t *testing.T) {
	got := defaultFormatResult(map[string]int{"count": 5})
	tc := got.Content[0].(*mcp.TextContent)
	var m map[string]int
	if err := json.Unmarshal([]byte(tc.Text), &m); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if m["count"] != 5 {
		t.Errorf("count = %d, want 5", m["count"])
	}
}

// ValidActionsString.

// TestValidActionsString verifies sorted comma-separated output.
func TestValidActionsString(t *testing.T) {
	routes := ActionMap{
		"delete": Route(nil),
		"create": Route(nil),
		"list":   Route(nil),
	}
	got := ValidActionsString(routes)
	if got != "create, delete, list" {
		t.Errorf("got %q, want %q", got, "create, delete, list")
	}
}

// MetaToolSchema.

// TestMetaToolSchema verifies the generated JSON Schema contains the
// action enum and params property.
func TestMetaToolSchema(t *testing.T) {
	routes := ActionMap{
		"get":  Route(nil),
		"list": Route(nil),
	}
	schema := MetaToolSchema(routes)
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("missing properties")
	}
	actionProp := props["action"].(map[string]any)
	enumVals := actionProp["enum"].([]string)
	if len(enumVals) != 2 || enumVals[0] != "get" || enumVals[1] != "list" {
		t.Errorf("enum = %v, want [get list]", enumVals)
	}
	required := schema["required"].([]any)
	if len(required) != 1 || required[0] != "action" {
		t.Errorf("required = %v, want [action]", required)
	}
}

// TestMetaToolSchema_OpaqueDefault verifies that the default opaque mode
// does NOT emit a oneOf branch list and keeps params as an open object.
func TestMetaToolSchema_OpaqueDefault(t *testing.T) {
	routes := ActionMap{
		"get":  Route(nil),
		"list": Route(nil),
	}
	schema := MetaToolSchema(routes)
	if _, has := schema["oneOf"]; has {
		t.Error("opaque schema should not contain oneOf")
	}
	props := schema["properties"].(map[string]any)
	paramsProp := props["params"].(map[string]any)
	if paramsProp["additionalProperties"] != true {
		t.Errorf("params.additionalProperties = %v, want true", paramsProp["additionalProperties"])
	}
	desc, _ := paramsProp["description"].(string)
	if desc != metaToolParamsDescription {
		t.Errorf("params.description = %q, want %q", desc, metaToolParamsDescription)
	}
	if !strings.Contains(desc, "gitlab://tools/{tool}.{action}") {
		t.Error("params.description should mention the schema resource URI")
	}
	if strings.Contains(desc, "unknown keys") {
		t.Error("params.description should avoid wording that conflicts with openWorldHint")
	}
}

// TestBuildMetaToolSchema_FullEmitsOneOf verifies that full mode produces
// a oneOf branch per action with action pinned to a const.
func TestBuildMetaToolSchema_FullEmitsOneOf(t *testing.T) {
	routes := ActionMap{
		"create": RouteAction[testInput, testOutput](nil, nil),
		"get":    RouteAction[testInput, testOutput](nil, nil),
	}
	schema := BuildMetaToolSchema(routes, MetaParamSchemaFull)

	branches, ok := schema["oneOf"].([]any)
	if !ok {
		t.Fatalf("oneOf missing or wrong type: %T", schema["oneOf"])
	}
	if len(branches) != 2 {
		t.Fatalf("oneOf len = %d, want 2", len(branches))
	}
	wantActions := []string{"create", "get"} // sorted
	for i, b := range branches {
		bm := b.(map[string]any)
		bp := bm["properties"].(map[string]any)
		ap := bp["action"].(map[string]any)
		if ap["const"] != wantActions[i] {
			t.Errorf("branch[%d].action.const = %v, want %q", i, ap["const"], wantActions[i])
		}
		paramsBranch := bp["params"].(map[string]any)
		// Full mode should preserve the reflected schema, which carries a
		// type or a $ref pointing into $defs.
		_, hasType := paramsBranch["type"]
		_, hasRef := paramsBranch["$ref"]
		_, hasProps := paramsBranch["properties"]
		if !hasType && !hasRef && !hasProps {
			t.Errorf("branch[%d].params lacks type/$ref/properties: %v", i, paramsBranch)
		}
	}
}

// TestBuildMetaToolSchema_CompactStripsDescriptions verifies that compact
// mode drops description strings from params property entries.
func TestBuildMetaToolSchema_CompactStripsDescriptions(t *testing.T) {
	routes := ActionMap{
		"get": RouteAction[testInput, testOutput](nil, nil),
	}
	schema := BuildMetaToolSchema(routes, MetaParamSchemaCompact)

	branches := schema["oneOf"].([]any)
	if len(branches) != 1 {
		t.Fatalf("oneOf len = %d, want 1", len(branches))
	}
	bp := branches[0].(map[string]any)["properties"].(map[string]any)
	paramsBranch := bp["params"].(map[string]any)
	if paramsBranch["additionalProperties"] != true {
		t.Errorf("compact params.additionalProperties = %v, want true", paramsBranch["additionalProperties"])
	}
	props, ok := paramsBranch["properties"].(map[string]any)
	if !ok {
		t.Fatalf("compact params has no properties map: %v", paramsBranch)
	}
	for name, raw := range props {
		entry := raw.(map[string]any)
		if _, hasDesc := entry["description"]; hasDesc {
			t.Errorf("compact field %q retains description", name)
		}
	}
}

// TestBuildMetaToolSchema_UnknownModeFallsBackToOpaque verifies unknown
// modes silently degrade to the opaque envelope.
func TestBuildMetaToolSchema_UnknownModeFallsBackToOpaque(t *testing.T) {
	routes := ActionMap{"get": Route(nil)}
	schema := BuildMetaToolSchema(routes, "verbose")
	if _, has := schema["oneOf"]; has {
		t.Error("unknown mode should not emit oneOf")
	}
}

// TestMetaToolDescriptionPrefix_FormatsLiteralExample checks that the prefix
// embeds a representative action and the resource pointer for the given tool
// name. Empty routes return an empty string.
func TestMetaToolDescriptionPrefix_FormatsLiteralExample(t *testing.T) {
	routes := ActionMap{"create": Route(nil), "list": Route(nil), "delete": Route(nil)}
	got := MetaToolDescriptionPrefix("gitlab_widget", routes)

	wantExample := `Use {"action":"list","params":{...}}`
	if !strings.Contains(got, wantExample) {
		t.Errorf("prefix missing literal example, got: %q", got)
	}
	wantEnvelope := "only top-level keys are action and params"
	if !strings.Contains(got, wantEnvelope) {
		t.Errorf("prefix missing envelope guidance, got: %q", got)
	}
	wantPointer := "Action params schema: gitlab://tools/gitlab_widget.<action>"
	if !strings.Contains(got, wantPointer) {
		t.Errorf("prefix missing resource pointer, got: %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Errorf("prefix should end with blank line separator, got: %q", got)
	}

	if MetaToolDescriptionPrefix("gitlab_empty", ActionMap{}) != "" {
		t.Error("empty routes should yield empty prefix")
	}
}

// TestMetaToolDescriptionPrefix_PrefersReadableExampleAction verifies the
// usage example prefers a read action over a mutating action when both are
// available.
func TestMetaToolDescriptionPrefix_PrefersReadableExampleAction(t *testing.T) {
	routes := ActionMap{"archive": Route(nil), "create": Route(nil), "delete": Route(nil), "get": Route(nil), "update": Route(nil)}
	got := MetaToolDescriptionPrefix("gitlab_widget", routes)

	if !strings.Contains(got, `Use {"action":"get","params":{...}}`) {
		t.Fatalf("prefix action example = %q, want get", got)
	}
}

// TestMetaToolDescriptionPrefix_IncludesParameterGuidance verifies generated
// guidance appears only when routes define role-sensitive parameter metadata.
func TestMetaToolDescriptionPrefix_IncludesParameterGuidance(t *testing.T) {
	guidance := map[string]ParameterGuidance{
		"project_id": {
			SemanticRole:     "scope_owner_project",
			ValueSource:      "Owning project whose allowlist is being changed.",
			CommonConfusions: []string{"Do not use the project being removed as project_id."},
		},
	}
	routes := ActionMap{
		"token_scope_remove_project": Route(nil).WithParameterGuidance(guidance),
	}
	guidance["project_id"] = ParameterGuidance{SemanticRole: "changed"}

	got := MetaToolDescriptionPrefix("gitlab_job", routes)
	for _, want := range []string{
		"Parameter guidance:",
		"token_scope_remove_project.project_id: scope_owner_project",
		"source: Owning project whose allowlist is being changed.",
		"avoid: Do not use the project being removed as project_id.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prefix missing %q: %q", want, got)
		}
	}

	description := got + "Manage GitLab CI job token scope."
	if stripped := StripMetaToolDescriptionPrefix(description); stripped != "Manage GitLab CI job token scope." {
		t.Fatalf("StripMetaToolDescriptionPrefix() = %q, want base description", stripped)
	}
}

// TestMetaToolDescriptionPrefix_IncludesActionGuidance verifies generated
// descriptions surface per-action usage hints for meta-tool selection.
func TestMetaToolDescriptionPrefix_IncludesActionGuidance(t *testing.T) {
	routes := ActionMap{
		"metadata_get": Route(nil).WithUsage("Read GitLab instance metadata such as version and revision."),
		"settings_get": Route(nil).WithUsage("Read current GitLab application settings."),
	}

	got := MetaToolDescriptionPrefix("gitlab_admin", routes)
	for _, want := range []string{
		"Action guidance:",
		"metadata_get: Read GitLab instance metadata such as version and revision.",
		"settings_get: Read current GitLab application settings.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prefix missing %q: %q", want, got)
		}
	}

	description := got + "Manage GitLab instance administration."
	if stripped := StripMetaToolDescriptionPrefix(description); stripped != "Manage GitLab instance administration." {
		t.Fatalf("StripMetaToolDescriptionPrefix() = %q, want base description", stripped)
	}
}

// TestStripMetaToolDescriptionPrefix_StripsCurrentPrefix verifies the generated
// concise prefix is removed before documentation summaries are rendered.
func TestStripMetaToolDescriptionPrefix_StripsCurrentPrefix(t *testing.T) {
	description := MetaToolDescriptionPrefix("gitlab_issue", ActionMap{"create": Route(nil)}) + "Manage GitLab issues."

	got := StripMetaToolDescriptionPrefix(description)
	if got != "Manage GitLab issues." {
		t.Fatalf("StripMetaToolDescriptionPrefix() = %q, want real description", got)
	}
}

// TestStripMetaToolDescriptionPrefix_PreservesPrefixOnlyDescription verifies the
// defensive fallback for future callers that pass only the generated prefix.
func TestStripMetaToolDescriptionPrefix_PreservesPrefixOnlyDescription(t *testing.T) {
	description := MetaToolDescriptionPrefix("gitlab_issue", ActionMap{"create": Route(nil)})

	got := StripMetaToolDescriptionPrefix(description)
	if got != description {
		t.Fatalf("StripMetaToolDescriptionPrefix() = %q, want original description", got)
	}
}

// TestStripMetaToolDescriptionPrefix_StripsLegacyPrefix keeps README and llms
// generation compatible with descriptions emitted before the concise prefix.
func TestStripMetaToolDescriptionPrefix_StripsLegacyPrefix(t *testing.T) {
	description := "Example: {\"action\":\"create\",\"params\":{...}}\n" +
		"For the params schema of any action, read gitlab://tools/gitlab_issue.<action>.\n\n" +
		"Manage GitLab issues."

	got := StripMetaToolDescriptionPrefix(description)
	if got != "Manage GitLab issues." {
		t.Fatalf("StripMetaToolDescriptionPrefix() = %q, want real description", got)
	}
}

// TestStripMetaToolDescriptionPrefix_PreservesStandaloneExample verifies normal
// descriptions are left intact when only one generated-prefix line is present.
func TestStripMetaToolDescriptionPrefix_PreservesStandaloneExample(t *testing.T) {
	description := "Example: resolve this remote before listing projects. More details follow."

	got := StripMetaToolDescriptionPrefix(description)
	if got != description {
		t.Fatalf("StripMetaToolDescriptionPrefix() = %q, want original description", got)
	}
}

// TestStripMetaToolDescriptionPrefix_PreservesMultiLineWithoutPrefix verifies
// that a multi-line description that does not carry the generated meta-tool
// prefix is preserved unchanged (covers the !hasUsageExample || !hasSchemaHint
// branch).
func TestStripMetaToolDescriptionPrefix_PreservesMultiLineWithoutPrefix(t *testing.T) {
	description := "Use this tool to delete resources.\nFree-form documentation line."

	got := StripMetaToolDescriptionPrefix(description)
	if got != description {
		t.Fatalf("StripMetaToolDescriptionPrefix() = %q, want original description", got)
	}
}

// enrichWithHints.

// TestEnrichWithHints_AddsNextSteps verifies that enrichWithHints injects
// a next_steps field into the structured JSON content of an MCP tool result.
func TestEnrichWithHints_AddsNextSteps(t *testing.T) {
	type sampleOutput struct {
		Items []string `json:"items"`
		Count int      `json:"count"`
	}
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "## Results\n\n---\n💡 **Next steps:**\n- Get details\n- Delete item\n"},
		},
	}
	result := sampleOutput{Items: []string{"a", "b"}, Count: 2}
	enriched := enrichWithHints(result, callResult)

	raw, ok := enriched.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", enriched)
	}

	// Verify next_steps is the first field in the JSON.
	const prefix = `{"next_steps":`
	if !strings.HasPrefix(string(raw), prefix) {
		t.Errorf("JSON should start with %s, got: %.60s", prefix, string(raw))
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	stepsAny, ok := m["next_steps"].([]any)
	if !ok || len(stepsAny) != 2 {
		t.Fatalf("next_steps = %v, want 2 strings", m["next_steps"])
	}
	if stepsAny[0] != "Get details" || stepsAny[1] != "Delete item" {
		t.Errorf("steps = %v", stepsAny)
	}
	if m["count"] != float64(2) {
		t.Errorf("count = %v, want 2", m["count"])
	}
}

// TestEnrichWithHints_NoHintsSection verifies that enrichWithHints leaves
// the result unchanged when the markdown contains no hints section.
func TestEnrichWithHints_NoHintsSection(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "## Just a title\n"},
		},
	}
	original := map[string]string{"key": "val"}
	enriched := enrichWithHints(original, callResult)
	m, ok := enriched.(map[string]string)
	if !ok || m["key"] != "val" {
		t.Error("expected unchanged result when no hints")
	}
}

// TestEnrichWithHints_NilResult verifies that enrichWithHints handles a nil
// tool result without panicking.
func TestEnrichWithHints_NilResult(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "---\n💡 **Next steps:**\n- hint\n"},
		},
	}
	if got := enrichWithHints(nil, callResult); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// TestEnrichWithHints_NilCallResult verifies that enrichWithHints handles
// a nil CallToolResult without panicking.
func TestEnrichWithHints_NilCallResult(t *testing.T) {
	original := map[string]string{"key": "val"}
	enriched := enrichWithHints(original, nil)
	m, ok := enriched.(map[string]string)
	if !ok || m["key"] != "val" {
		t.Error("expected unchanged result for nil callResult")
	}
}

// TestMakeMetaHandler_EnrichesStructuredContent verifies that the meta-tool
// handler wrapper enriches structured JSON output with next_steps hints.
func TestMakeMetaHandler_EnrichesStructuredContent(t *testing.T) {
	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return map[string]any{"items": []string{"x"}}, nil
		}),
	}
	formatter := func(result any) *mcp.CallToolResult {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "## List\n\n---\n💡 **Next steps:**\n- View item\n"},
			},
		}
	}
	handler := MakeMetaHandler("test", routes, formatter)
	_, raw, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{Action: "list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rawMsg, ok := raw.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", raw)
	}
	var m map[string]any
	if unmarshalErr := json.Unmarshal(rawMsg, &m); unmarshalErr != nil {
		t.Fatalf("failed to unmarshal: %v", unmarshalErr)
	}
	stepsAny, ok := m["next_steps"].([]any)
	if !ok || len(stepsAny) != 1 || stepsAny[0] != "View item" {
		t.Errorf("next_steps = %v", m["next_steps"])
	}
}

// TestEnrichWithHints_NonObjectJSON verifies that enrichWithHints returns
// the result unchanged when it serializes to a non-object JSON value
// (e.g. a string or array).
func TestEnrichWithHints_NonObjectJSON(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "---\n💡 **Next steps:**\n- hint\n"},
		},
	}
	original := "just a string"
	enriched := enrichWithHints(original, callResult)
	s, ok := enriched.(string)
	if !ok || s != "just a string" {
		t.Errorf("expected unchanged string, got %T: %v", enriched, enriched)
	}
}

// TestEnrichWithHints_EmptyObject verifies that enrichWithHints correctly
// handles an empty JSON object (only "{}") by producing valid JSON with
// next_steps as the only field.
func TestEnrichWithHints_EmptyObject(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "---\n💡 **Next steps:**\n- do thing\n"},
		},
	}
	type empty struct{}
	enriched := enrichWithHints(empty{}, callResult)
	raw, ok := enriched.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", enriched)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("invalid JSON: %v — raw: %s", err, string(raw))
	}
	stepsAny, ok := m["next_steps"].([]any)
	if !ok || len(stepsAny) != 1 || stepsAny[0] != "do thing" {
		t.Errorf("next_steps = %v", m["next_steps"])
	}
}

// TestWrapActionWithRequest_UnmarshalError verifies that WrapActionWithRequest
// returns an error when params cannot be unmarshaled into the typed input.
func TestWrapActionWithRequest_UnmarshalError(t *testing.T) {
	fn := func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, in testInput) (testOutput, error) {
		return testOutput{Result: "should not reach"}, nil
	}
	action := WrapActionWithRequest(nil, fn)
	_, err := action(context.Background(), map[string]any{"name": 12345})
	if err == nil {
		t.Fatal("expected error for invalid params, got nil")
	}
}

// TestDefaultFormatResult_Unmarshalable verifies that defaultFormatResult
// falls back to fmt.Sprintf for types that cannot be JSON-marshaled.
func TestDefaultFormatResult_Unmarshalable(t *testing.T) {
	got := defaultFormatResult(func() {})
	tc, ok := got.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if tc.Text == "" {
		t.Error("expected non-empty fallback text")
	}
}

// TestMakeMetaHandler_DestructiveActionConfirmBypass verifies that
// MakeMetaHandler intercepts destructive actions with confirmation,
// and that the "confirm" param bypasses the prompt.
func TestMakeMetaHandler_DestructiveActionConfirmBypass(t *testing.T) {
	called := false
	routes := ActionMap{
		"delete": DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
			called = true
			return map[string]string{"status": "deleted"}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)

	// With "confirm": true, the action should proceed without elicitation.
	input := MetaToolInput{
		Action: "delete",
		Params: map[string]any{"id": float64(1), "confirm": true},
	}
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "test_tool"}}
	result, _, err := handler(context.Background(), req, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called — confirmation should have been bypassed")
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

// TestMakeMetaHandler_DestructiveActionYOLOMode verifies that YOLO_MODE
// bypasses confirmation for destructive meta-tool actions.
func TestMakeMetaHandler_DestructiveActionYOLOMode(t *testing.T) {
	t.Setenv("YOLO_MODE", "true")

	called := false
	routes := ActionMap{
		"token_revoke": DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
			called = true
			return map[string]string{"status": "revoked"}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	input := MetaToolInput{Action: "token_revoke", Params: map[string]any{}}
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "test_tool"}}
	_, _, err := handler(context.Background(), req, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called — YOLO_MODE should bypass confirmation")
	}
}

// TestMakeMetaHandler_NonDestructiveSkipsConfirm verifies that non-destructive
// actions are dispatched without any confirmation prompt.
func TestMakeMetaHandler_NonDestructiveSkipsConfirm(t *testing.T) {
	called := false
	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) {
			called = true
			return []string{"a", "b"}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	input := MetaToolInput{Action: "list", Params: map[string]any{}}
	_, _, err := handler(context.Background(), &mcp.CallToolRequest{}, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
}

// TestMakeMetaHandler_DestructiveNoElicitation verifies that when the client
// does not support elicitation, destructive actions fail closed: the handler
// must not run, and the error result must instruct the caller to re-send
// with confirm=true. With confirm=true the same call proceeds.
func TestMakeMetaHandler_DestructiveNoElicitation(t *testing.T) {
	called := false
	routes := ActionMap{
		"delete": DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
			called = true
			return map[string]string{"status": "deleted"}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "test_tool"}}

	result, _, err := handler(context.Background(), req, MetaToolInput{Action: "delete", Params: map[string]any{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("handler was called — destructive action must fail closed when elicitation is unsupported")
	}
	if result == nil || !result.IsError {
		t.Fatalf("result = %+v, want fail-closed error result", result)
	}
	if tc, ok := result.Content[0].(*mcp.TextContent); !ok || !strings.Contains(tc.Text, "confirm=true") {
		t.Errorf("guard result content = %+v, want confirm=true instruction", result.Content[0])
	}

	_, _, err = handler(context.Background(), req, MetaToolInput{Action: "delete", Params: map[string]any{"confirm": true}})
	if err != nil {
		t.Fatalf("unexpected error with confirm=true: %v", err)
	}
	if !called {
		t.Error("handler was not called — confirm=true must bypass the guard")
	}
}

// TestRoute_CreatesNonDestructiveRoute verifies that Route() creates an
// ActionRoute with Destructive=false.
func TestRoute_CreatesNonDestructiveRoute(t *testing.T) {
	fn := func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }
	r := Route(fn)
	if r.Destructive {
		t.Error("Route() should create non-destructive route")
	}
	if r.Handler == nil {
		t.Error("Route() should set Handler")
	}
}

// TestDestructiveRoute_CreatesDestructiveRoute verifies that DestructiveRoute()
// creates an ActionRoute with Destructive=true.
func TestDestructiveRoute_CreatesDestructiveRoute(t *testing.T) {
	fn := func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }
	r := DestructiveRoute(fn)
	if !r.Destructive {
		t.Error("DestructiveRoute() should create destructive route")
	}
	if r.Handler == nil {
		t.Error("DestructiveRoute() should set Handler")
	}
}

// TestRouteRequestFunc_InvokesHandlerWithRequest verifies request-aware routes
// decode input, attach schemas, and forward the MCP request from context.
func TestRouteRequestFunc_InvokesHandlerWithRequest(t *testing.T) {
	request := &mcp.CallToolRequest{}
	var receivedRequest *mcp.CallToolRequest
	route := RouteRequestFunc(func(_ context.Context, req *mcp.CallToolRequest, input routeRequestTestInput) (testOutput, error) {
		receivedRequest = req
		return testOutput{Result: input.Name}, nil
	})

	if route.Destructive {
		t.Fatal("RouteRequestFunc() Destructive = true, want false")
	}
	if route.InputSchema == nil || route.OutputSchema == nil || route.InputType == nil {
		t.Fatalf("route schemas/input type must be populated: %+v", route)
	}

	result, err := route.Handler(ContextWithRequest(context.Background(), request), map[string]any{"name": "project", "id": float64(7)})
	if err != nil {
		t.Fatalf("route handler error = %v", err)
	}
	output, ok := result.(testOutput)
	if !ok || output.Result != "project" {
		t.Fatalf("route handler result = %#v, want project output", result)
	}
	if receivedRequest != request {
		t.Fatalf("received request = %p, want %p", receivedRequest, request)
	}
}

// TestRouteRequestFunc_InvalidParamsReturnsZero verifies typed route wrappers
// return the zero output value when decoding input fails.
func TestRouteRequestFunc_InvalidParamsReturnsZero(t *testing.T) {
	route := RouteRequestFunc(func(_ context.Context, _ *mcp.CallToolRequest, input routeRequestTestInput) (testOutput, error) {
		return testOutput{Result: input.Name}, nil
	})

	result, err := route.Handler(context.Background(), map[string]any{"id": []any{"bad"}})
	if err == nil {
		t.Fatal("route handler error = nil, want decode error")
	}
	if result != (testOutput{}) {
		t.Fatalf("route handler result = %#v, want zero testOutput", result)
	}
}

// TestDestructiveFunc_SetsDestructive verifies DestructiveFunc keeps typed
// route behavior while marking the route as destructive.
func TestDestructiveFunc_SetsDestructive(t *testing.T) {
	route := DestructiveFunc(func(_ context.Context, input destructiveFuncTestInput) (testOutput, error) {
		return testOutput{Result: input.Name}, nil
	})
	if !route.Destructive {
		t.Fatal("DestructiveFunc() Destructive = false, want true")
	}

	result, err := route.Handler(context.Background(), map[string]any{"name": "delete", "id": float64(1)})
	if err != nil {
		t.Fatalf("route handler error = %v", err)
	}
	output, ok := result.(testOutput)
	if !ok || output.Result != "delete" {
		t.Fatalf("route handler result = %#v, want delete output", result)
	}
}

// TestDeriveAnnotations_AllNonDestructive verifies that DeriveAnnotations returns
// NonDestructiveMetaAnnotations when no route is destructive.
func TestDeriveAnnotations_AllNonDestructive(t *testing.T) {
	routes := ActionMap{
		"list":   Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }),
		"get":    Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }),
		"create": Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }),
	}
	ann := DeriveAnnotations(routes)
	if ann.DestructiveHint == nil || *ann.DestructiveHint != false {
		t.Error("all non-destructive routes should produce DestructiveHint=false")
	}
}

// TestDeriveAnnotations_HasDestructive verifies that DeriveAnnotations returns
// MetaAnnotations when at least one route is destructive.
func TestDeriveAnnotations_HasDestructive(t *testing.T) {
	routes := ActionMap{
		"list":   Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }),
		"delete": DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }),
	}
	ann := DeriveAnnotations(routes)
	if ann.DestructiveHint == nil || *ann.DestructiveHint != true {
		t.Error("routes with destructive action should produce DestructiveHint=true")
	}
}

// TestDeriveAnnotations_EmptyMap verifies that DeriveAnnotations handles an empty
// ActionMap gracefully (no destructive routes → NonDestructiveMetaAnnotations).
func TestDeriveAnnotations_EmptyMap(t *testing.T) {
	ann := DeriveAnnotations(ActionMap{})
	if ann.DestructiveHint == nil || *ann.DestructiveHint != false {
		t.Error("empty map should produce DestructiveHint=false")
	}
}

// TestDeriveAnnotationsWithTitle verifies that DeriveAnnotationsWithTitle
// delegates to DeriveAnnotations and sets Title from the tool name.
// Covers both destructive and non-destructive route maps.
func TestDeriveAnnotationsWithTitle(t *testing.T) {
	noop := func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }

	t.Run("non-destructive routes set title and DestructiveHint=false", func(t *testing.T) {
		routes := ActionMap{"list": Route(noop), "get": Route(noop)}
		ann := DeriveAnnotationsWithTitle("gitlab_branch", routes)
		if ann.Title != "Branch" {
			t.Errorf("Title = %q, want %q", ann.Title, "Branch")
		}
		if ann.DestructiveHint == nil || *ann.DestructiveHint != false {
			t.Error("non-destructive routes should produce DestructiveHint=false")
		}
	})

	t.Run("destructive routes set title and DestructiveHint=true", func(t *testing.T) {
		routes := ActionMap{"list": Route(noop), "delete": DestructiveRoute(noop)}
		ann := DeriveAnnotationsWithTitle("gitlab_merge_request", routes)
		if ann.Title != "Merge Request" {
			t.Errorf("Title = %q, want %q", ann.Title, "Merge Request")
		}
		if ann.DestructiveHint == nil || *ann.DestructiveHint != true {
			t.Error("destructive routes should produce DestructiveHint=true")
		}
	})
}

// TestReadOnlyMetaAnnotationsWithTitle verifies that ReadOnlyMetaAnnotationsWithTitle
// returns a copy of ReadOnlyMetaAnnotations with the Title set and all read-only
// fields preserved. Also verifies the shared singleton is not mutated.
func TestReadOnlyMetaAnnotationsWithTitle(t *testing.T) {
	ann := ReadOnlyMetaAnnotationsWithTitle("gitlab_search")

	if ann.Title != "Search" {
		t.Errorf("Title = %q, want %q", ann.Title, "Search")
	}
	if !ann.ReadOnlyHint {
		t.Error("ReadOnlyHint should be true")
	}
	if !ann.IdempotentHint {
		t.Error("IdempotentHint should be true")
	}
	if ann.DestructiveHint == nil || *ann.DestructiveHint != false {
		t.Error("DestructiveHint should be false")
	}
	if ann.OpenWorldHint == nil || *ann.OpenWorldHint != true {
		t.Error("OpenWorldHint should be true")
	}

	// Verify the shared singleton was not mutated.
	if ReadOnlyMetaAnnotations.Title != "" {
		t.Errorf("singleton Title mutated to %q, want empty", ReadOnlyMetaAnnotations.Title)
	}
}

// TestAddMetaTool_NilServerDoesNotPanic verifies nil-server registration is a
// no-op for callers that build optional tool surfaces.
func TestAddMetaTool_NilServerDoesNotPanic(t *testing.T) {
	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }),
	}

	AddMetaTool(nil, "gitlab_capture_noop", "Capture noop.", routes, nil, nil)
	AddReadOnlyMetaTool(nil, "gitlab_capture_readonly_noop", "Capture readonly noop.", routes, nil, nil)
}

// TestAddMetaTool_RegistersSharedMetadata verifies the shared registration
// helper applies the same metadata contract used by all action-dispatched
// meta-tools.
func TestAddMetaTool_RegistersSharedMetadata(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	routes := ActionMap{
		"delete": DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
			return struct{}{}, nil
		}),
	}

	AddMetaTool(server, "gitlab_test_meta", "Manage test metadata.", routes, nil, nil)

	tool := findTool(t, listToolsViaClient(t, server), "gitlab_test_meta")
	if !strings.Contains(tool.Description, "gitlab://tools/gitlab_test_meta.<action>") {
		t.Errorf("description missing schema resource hint: %q", tool.Description)
	}
	if !strings.Contains(tool.Description, "Manage test metadata.") {
		t.Errorf("description missing supplied body: %q", tool.Description)
	}
	if tool.Annotations == nil || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint != true {
		t.Fatal("destructive meta-tool should have DestructiveHint=true")
	}
	if tool.Annotations.Title != "Test Meta" {
		t.Errorf("annotation title = %q, want %q", tool.Annotations.Title, "Test Meta")
	}
	if tool.InputSchema == nil {
		t.Fatal("input schema is nil")
	}
	if tool.OutputSchema == nil {
		t.Fatal("output schema is nil")
	}
}

// TestAddReadOnlyMetaTool_RegistersReadOnlyMetadata verifies the read-only
// helper preserves read-only annotations while sharing the common schema and
// description contract.
func TestAddReadOnlyMetaTool_RegistersReadOnlyMetadata(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return struct{}{}, nil
		}),
	}

	AddReadOnlyMetaTool(server, "gitlab_test_read", "List test metadata.", routes, nil, nil)

	tool := findTool(t, listToolsViaClient(t, server), "gitlab_test_read")
	if !strings.Contains(tool.Description, "gitlab://tools/gitlab_test_read.<action>") {
		t.Errorf("description missing schema resource hint: %q", tool.Description)
	}
	if tool.Annotations == nil {
		t.Fatal("annotations are nil")
	}
	if !tool.Annotations.ReadOnlyHint {
		t.Error("ReadOnlyHint should be true")
	}
	if !tool.Annotations.IdempotentHint {
		t.Error("IdempotentHint should be true")
	}
	if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint != false {
		t.Error("DestructiveHint should be false")
	}
	if tool.Annotations.Title != "Test Read" {
		t.Errorf("annotation title = %q, want %q", tool.Annotations.Title, "Test Read")
	}
}

// TestMakeMetaHandler_MetadataDestructive_TriggersConfirm verifies that
// MakeMetaHandler reads route.Destructive to determine confirmation requirement.
func TestMakeMetaHandler_MetadataDestructive_TriggersConfirm(t *testing.T) {
	called := false
	routes := ActionMap{
		"delete": DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
			called = true
			return "ok", nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "test_tool"}}

	// Without confirm=true and without elicitation support, the destructive
	// route must fail closed before reaching the handler.
	result, _, err := handler(context.Background(), req, MetaToolInput{Action: "delete", Params: map[string]any{"id": float64(1)}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("handler was called without confirmation")
	}
	if result == nil || !result.IsError {
		t.Fatalf("result = %+v, want fail-closed error result", result)
	}

	// confirm=true bypasses the guard and reaches the handler.
	result, _, err = handler(context.Background(), req, MetaToolInput{Action: "delete", Params: map[string]any{"id": float64(1), "confirm": true}})
	if err != nil {
		t.Fatalf("unexpected error with confirm=true: %v", err)
	}
	if !called {
		t.Error("handler was not called with confirm=true")
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

// TestMakeMetaHandler_NonDestructive_SkipsConfirm verifies that non-destructive
// routes do not trigger confirmation.
func TestMakeMetaHandler_NonDestructive_SkipsConfirm(t *testing.T) {
	called := false
	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) {
			called = true
			return []string{"a", "b"}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	input := MetaToolInput{Action: "list", Params: map[string]any{}}
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "test_tool"}}

	result, _, err := handler(context.Background(), req, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
	if result == nil {
		t.Fatal("result is nil")
	}
}

// Composite wrapper metadata tests — verify that every wrapper type correctly
// sets (or clears) the Destructive flag on the resulting ActionRoute.

// TestCompositeWrappers_DestructiveMetadata verifies that all eight Route/DestructiveRoute
// wrapper functions produce ActionRoutes with the correct Destructive flag.
func TestCompositeWrappers_DestructiveMetadata(t *testing.T) {
	typedFn := func(_ context.Context, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, nil
	}
	voidFn := func(_ context.Context, _ *gitlabclient.Client, _ testInput) error {
		return nil
	}
	reqFn := func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, nil
	}
	rawFn := func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }

	tests := []struct {
		name            string
		route           ActionRoute
		wantDestructive bool
	}{
		{"Route", Route(rawFn), false},
		{"DestructiveRoute", DestructiveRoute(rawFn), true},
		{"RouteAction", RouteAction(nil, typedFn), false},
		{"RouteVoidAction", RouteVoidAction(nil, voidFn), false},
		{"RouteActionWithRequest", RouteActionWithRequest(nil, reqFn), false},
		{"DestructiveAction", DestructiveAction(nil, typedFn), true},
		{"DestructiveVoidAction", DestructiveVoidAction(nil, voidFn), true},
		{"DestructiveActionWithRequest", DestructiveActionWithRequest(nil, reqFn), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.route.Destructive != tt.wantDestructive {
				t.Errorf("Destructive = %v, want %v", tt.route.Destructive, tt.wantDestructive)
			}
			if tt.route.Handler == nil {
				t.Errorf("Handler is nil")
			}
		})
	}
}

// TestDeriveAnnotations_WithCompositeWrappers verifies that DeriveAnnotations
// correctly detects destructive routes produced by composite wrappers in a
// mixed route map (simulating real registration patterns).
func TestDeriveAnnotations_WithCompositeWrappers(t *testing.T) {
	typedFn := func(_ context.Context, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, nil
	}
	voidFn := func(_ context.Context, _ *gitlabclient.Client, _ testInput) error {
		return nil
	}

	tests := []struct {
		name                string
		routes              ActionMap
		wantDestructiveHint bool
	}{
		{
			name: "AllNonDestructive",
			routes: ActionMap{
				"list":   RouteAction(nil, typedFn),
				"get":    RouteAction(nil, typedFn),
				"create": RouteAction(nil, typedFn),
			},
			wantDestructiveHint: false,
		},
		{
			name: "OneDestructiveAction",
			routes: ActionMap{
				"list":   RouteAction(nil, typedFn),
				"get":    RouteAction(nil, typedFn),
				"delete": DestructiveVoidAction(nil, voidFn),
			},
			wantDestructiveHint: true,
		},
		{
			name: "MultipleDestructiveActions",
			routes: ActionMap{
				"list":   RouteAction(nil, typedFn),
				"delete": DestructiveVoidAction(nil, voidFn),
				"remove": DestructiveVoidAction(nil, voidFn),
				"revoke": DestructiveAction(nil, typedFn),
			},
			wantDestructiveHint: true,
		},
		{
			name: "OnlyDestructiveActions",
			routes: ActionMap{
				"delete": DestructiveVoidAction(nil, voidFn),
				"purge":  DestructiveVoidAction(nil, voidFn),
			},
			wantDestructiveHint: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ann := DeriveAnnotations(tt.routes)
			got := ann.DestructiveHint != nil && *ann.DestructiveHint
			if got != tt.wantDestructiveHint {
				t.Errorf("DestructiveHint = %v, want %v", got, tt.wantDestructiveHint)
			}
		})
	}
}

// TestMakeMetaHandler_CompositeWrapperConfirmation verifies that MakeMetaHandler
// correctly triggers (or skips) confirmation for routes built with composite
// wrappers, covering representative domain action patterns.
func TestMakeMetaHandler_CompositeWrapperConfirmation(t *testing.T) {
	var typedCalls, voidCalls int
	typedFn := func(_ context.Context, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		typedCalls++
		return testOutput{Result: "ok"}, nil
	}
	voidFn := func(_ context.Context, _ *gitlabclient.Client, _ testInput) error {
		voidCalls++
		return nil
	}

	routes := ActionMap{
		"list":   RouteAction(nil, typedFn),
		"get":    RouteAction(nil, typedFn),
		"create": RouteAction(nil, typedFn),
		"update": RouteAction(nil, typedFn),
		"delete": DestructiveVoidAction(nil, voidFn),
		"remove": DestructiveAction(nil, typedFn),
	}

	formatter := func(result any) *mcp.CallToolResult {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
		}
	}
	handler := MakeMetaHandler("test_domain", routes, formatter)

	tests := []struct {
		name       string
		action     string
		params     map[string]any
		wantCalled bool
	}{
		{name: "list", action: "list", params: map[string]any{}, wantCalled: true},
		{name: "get", action: "get", params: map[string]any{}, wantCalled: true},
		{name: "create", action: "create", params: map[string]any{}, wantCalled: true},
		{name: "update", action: "update", params: map[string]any{}, wantCalled: true},
		// Destructive composite wrappers without elicitation support fail
		// closed: the wrapped handler must not run and the result must
		// instruct re-sending with confirm=true.
		{name: "delete_blocked", action: "delete", params: map[string]any{}, wantCalled: false},
		{name: "remove_blocked", action: "remove", params: map[string]any{}, wantCalled: false},
		// Explicit confirm=true bypasses the guard for both wrapper kinds.
		{name: "delete_confirm", action: "delete", params: map[string]any{"confirm": true}, wantCalled: true},
		{name: "remove_confirm", action: "remove", params: map[string]any{"confirm": true}, wantCalled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callsBefore := typedCalls + voidCalls
			input := MetaToolInput{
				Action: tt.action,
				Params: tt.params,
			}

			req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "test_domain"}}

			result, _, err := handler(context.Background(), req, input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result == nil {
				t.Fatal("expected result but got nil")
			}
			called := typedCalls+voidCalls > callsBefore
			if called != tt.wantCalled {
				t.Fatalf("handler called = %v, want %v", called, tt.wantCalled)
			}
			if !tt.wantCalled {
				if !result.IsError {
					t.Errorf("blocked result.IsError = false, want true")
				}
				tc, ok := result.Content[0].(*mcp.TextContent)
				if !ok || !strings.Contains(tc.Text, "confirm=true") {
					t.Errorf("blocked result content = %+v, want confirm=true instruction", result.Content[0])
				}
			}
		})
	}
}

// --- OutputSchema tests (TASK-064/065/066/067/071) ---

// TestRouteAction_OutputSchema verifies RouteAction populates OutputSchema
// from the result type R.
func TestRouteAction_OutputSchema(t *testing.T) {
	client := &gitlabclient.Client{}
	route := RouteAction(client, func(_ context.Context, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, nil
	})
	if route.OutputSchema == nil {
		t.Fatal("expected OutputSchema to be non-nil for RouteAction[T,R]")
	}
	typ, ok := route.OutputSchema["type"]
	if !ok || typ != "object" {
		t.Errorf("expected OutputSchema type=object, got %v", typ)
	}
	props, propsOK := route.OutputSchema["properties"].(map[string]any)
	if !propsOK {
		t.Fatal("expected OutputSchema to have properties")
	}
	if _, hasResult := props["result"]; !hasResult {
		t.Error("expected OutputSchema to include 'result' property from testOutput struct")
	}
}

// TestDestructiveAction_OutputSchema verifies DestructiveAction populates OutputSchema.
func TestDestructiveAction_OutputSchema(t *testing.T) {
	client := &gitlabclient.Client{}
	route := DestructiveAction(client, func(_ context.Context, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, nil
	})
	if route.OutputSchema == nil {
		t.Fatal("expected OutputSchema to be non-nil for DestructiveAction[T,R]")
	}
	if !route.Destructive {
		t.Error("expected Destructive=true")
	}
}

// TestRouteActionWithRequest_OutputSchema verifies RouteActionWithRequest populates OutputSchema.
func TestRouteActionWithRequest_OutputSchema(t *testing.T) {
	client := &gitlabclient.Client{}
	route := RouteActionWithRequest(client, func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, nil
	})
	if route.OutputSchema == nil {
		t.Fatal("expected OutputSchema to be non-nil for RouteActionWithRequest[T,R]")
	}
}

// TestDestructiveActionWithRequest_OutputSchema verifies DestructiveActionWithRequest populates OutputSchema.
func TestDestructiveActionWithRequest_OutputSchema(t *testing.T) {
	client := &gitlabclient.Client{}
	route := DestructiveActionWithRequest(client, func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ testInput) (testOutput, error) {
		return testOutput{}, nil
	})
	if route.OutputSchema == nil {
		t.Fatal("expected OutputSchema to be non-nil")
	}
	if !route.Destructive {
		t.Error("expected Destructive=true")
	}
}

// TestRouteVoidAction_OutputSchema verifies void variants expose typed output schemas.
func TestRouteVoidAction_OutputSchema(t *testing.T) {
	client := &gitlabclient.Client{}
	route := RouteVoidAction(client, func(_ context.Context, _ *gitlabclient.Client, _ testInput) error {
		return nil
	})
	if route.OutputSchema == nil {
		t.Fatal("expected OutputSchema to be non-nil for void action")
	}
}

// TestDestructiveVoidAction_OutputSchema verifies destructive void variants expose typed output schemas.
func TestDestructiveVoidAction_OutputSchema(t *testing.T) {
	client := &gitlabclient.Client{}
	route := DestructiveVoidAction(client, func(_ context.Context, _ *gitlabclient.Client, _ testInput) error {
		return nil
	})
	if route.OutputSchema == nil {
		t.Fatal("expected OutputSchema to be non-nil for destructive void action")
	}
	if !route.Destructive {
		t.Error("expected Destructive=true")
	}
}

// TestSchemaForRoute_Caching verifies that SchemaForRoute returns the same
// map instance across multiple calls (cache hit).
func TestSchemaForRoute_Caching(t *testing.T) {
	s1 := SchemaForRoute[testOutput]()
	s2 := SchemaForRoute[testOutput]()
	if s1 == nil {
		t.Fatal("expected non-nil schema")
	}
	if s2 == nil {
		t.Fatal("expected non-nil schema on second call")
	}
	j1, err := json.Marshal(s1)
	if err != nil {
		t.Fatalf("marshal s1: %v", err)
	}
	j2, err := json.Marshal(s2)
	if err != nil {
		t.Fatalf("marshal s2: %v", err)
	}
	if string(j1) != string(j2) {
		t.Errorf("cached schemas differ:\n%s\n%s", j1, j2)
	}
}

// TestMetaToolOutputSchema_IsEnvelope verifies the envelope schema returned
// by MetaToolOutputSchema() contains cross-cutting fields and does NOT contain
// per-action schemas (regression test for TASK-067).
func TestMetaToolOutputSchema_IsEnvelope(t *testing.T) {
	schema := MetaToolOutputSchema()
	if schema["type"] != "object" {
		t.Errorf("expected type=object, got %v", schema["type"])
	}
	addProps, hasAddProps := schema["additionalProperties"]
	if !hasAddProps || addProps != true {
		t.Error("envelope schema must have additionalProperties=true")
	}
	props, propsOK := schema["properties"].(map[string]any)
	if !propsOK {
		t.Fatal("expected properties map")
	}
	if _, hasNextSteps := props["next_steps"]; !hasNextSteps {
		t.Error("expected next_steps in envelope properties")
	}
	if _, hasPagination := props["pagination"]; !hasPagination {
		t.Error("expected pagination in envelope properties")
	}
	// Envelope must NOT contain per-action output schemas.
	if _, hasResult := props["result"]; hasResult {
		t.Error("envelope should not contain per-action fields like 'result'")
	}
}

// TestRoute_OutputSchema_Nil verifies plain Route() has nil OutputSchema.
func TestRoute_OutputSchema_Nil(t *testing.T) {
	r := Route(func(_ context.Context, _ map[string]any) (any, error) {
		return "", nil
	})
	if r.OutputSchema != nil {
		t.Error("expected nil OutputSchema for plain Route()")
	}
}

// TestDestructiveRoute_OutputSchema_Nil verifies plain DestructiveRoute() has nil OutputSchema.
func TestDestructiveRoute_OutputSchema_Nil(t *testing.T) {
	r := DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
		return "", nil
	})
	if r.OutputSchema != nil {
		t.Error("expected nil OutputSchema for plain DestructiveRoute()")
	}
}

// --- Coverage tests for BuildMetaToolSchema helpers and supporting paths ---
// The following tests target branches not exercised by the higher-level
// tests above:
//
//   - SetMetaParamSchemaMode: package-level mode setter.
//   - resolveTopLevelRef: every fallback branch when $ref / $defs are
//     missing or malformed.
//   - compactParamsSchema: nil input, missing/non-object properties,
//     non-map property entries, and $ref resolution.
//   - buildMetaOneOf: per-action InputSchema = nil fallback.
//   - schemaForType: pointer dereference and cache hit paths.
//   - stripReservedKeys: presence of reserved keys mixed with real fields
//     (covers the "out[k] = v" copy branch).
//   - UnmarshalParams: double-failure path preserves the original error.
//   - enrichWithHints: non-object JSON short-circuit and non-text content
//     iteration.

// TestSetMetaParamSchemaMode_ValidValues verifies that each documented mode
// is accepted and round-trips through currentMetaParamSchemaMode.
func TestSetMetaParamSchemaMode_ValidValues(t *testing.T) {
	t.Cleanup(func() { SetMetaParamSchemaMode(MetaParamSchemaOpaque) })

	for _, mode := range []string{MetaParamSchemaOpaque, MetaParamSchemaCompact, MetaParamSchemaFull} {
		t.Run(mode, func(t *testing.T) {
			SetMetaParamSchemaMode(mode)
			if got := currentMetaParamSchemaMode(); got != mode {
				t.Errorf("currentMetaParamSchemaMode() = %q, want %q", got, mode)
			}
		})
	}
}

// TestSetMetaParamSchemaMode_InvalidCoercesToOpaque verifies that an unknown
// mode is silently coerced to "opaque" so misconfiguration cannot break the
// tools/list payload.
func TestSetMetaParamSchemaMode_InvalidCoercesToOpaque(t *testing.T) {
	t.Cleanup(func() { SetMetaParamSchemaMode(MetaParamSchemaOpaque) })

	SetMetaParamSchemaMode(MetaParamSchemaFull)
	SetMetaParamSchemaMode("nonsense")
	if got := currentMetaParamSchemaMode(); got != MetaParamSchemaOpaque {
		t.Errorf("invalid mode should coerce to opaque, got %q", got)
	}
}

// TestSetMetaParamSchemaModeScoped_RestoresPreviousMode verifies scoped mode
// overrides restore the prior global setting.
func TestSetMetaParamSchemaModeScoped_RestoresPreviousMode(t *testing.T) {
	SetMetaParamSchemaMode(MetaParamSchemaCompact)
	t.Cleanup(func() { SetMetaParamSchemaMode(MetaParamSchemaOpaque) })

	restore := SetMetaParamSchemaModeScoped(MetaParamSchemaFull)
	if got := currentMetaParamSchemaMode(); got != MetaParamSchemaFull {
		t.Fatalf("currentMetaParamSchemaMode() = %q, want %q", got, MetaParamSchemaFull)
	}
	restore()
	if got := currentMetaParamSchemaMode(); got != MetaParamSchemaCompact {
		t.Fatalf("restored mode = %q, want %q", got, MetaParamSchemaCompact)
	}
}

// TestResolveTopLevelRef_NoRef returns the schema unchanged when no
// top-level $ref is present.
func TestResolveTopLevelRef_NoRef(t *testing.T) {
	s := map[string]any{"type": "object"}
	got := resolveTopLevelRef(s)
	if !reflect.DeepEqual(got, s) {
		t.Errorf("expected schema returned unchanged, got %v", got)
	}
}

// TestResolveTopLevelRef_RefWithoutDefs returns the schema unchanged when
// $defs is absent; we cannot resolve the reference so the original wins.
func TestResolveTopLevelRef_RefWithoutDefs(t *testing.T) {
	s := map[string]any{"$ref": "#/$defs/Foo"}
	got := resolveTopLevelRef(s)
	if !reflect.DeepEqual(got, s) {
		t.Errorf("expected schema returned unchanged, got %v", got)
	}
}

// TestResolveTopLevelRef_RefWrongPrefix returns the schema unchanged when
// the $ref does not match the supported "#/$defs/" prefix.
func TestResolveTopLevelRef_RefWrongPrefix(t *testing.T) {
	s := map[string]any{
		"$ref":  "https://example.com/schema.json",
		"$defs": map[string]any{"Foo": map[string]any{"type": "object"}},
	}
	got := resolveTopLevelRef(s)
	if !reflect.DeepEqual(got, s) {
		t.Errorf("non-internal $ref should be returned unchanged, got %v", got)
	}
}

// TestResolveTopLevelRef_RefMissingTarget returns the schema unchanged when
// the referenced $defs entry does not exist.
func TestResolveTopLevelRef_RefMissingTarget(t *testing.T) {
	s := map[string]any{
		"$ref":  "#/$defs/Missing",
		"$defs": map[string]any{"Other": map[string]any{"type": "object"}},
	}
	got := resolveTopLevelRef(s)
	if !reflect.DeepEqual(got, s) {
		t.Errorf("missing target should return original schema, got %v", got)
	}
}

// TestResolveTopLevelRef_ResolvesValidRef returns the referenced $defs
// entry when the reference is well-formed.
func TestResolveTopLevelRef_ResolvesValidRef(t *testing.T) {
	target := map[string]any{
		"type":       "object",
		"properties": map[string]any{"id": map[string]any{"type": "integer"}},
	}
	s := map[string]any{
		"$ref":  "#/$defs/Foo",
		"$defs": map[string]any{"Foo": target},
	}
	got := resolveTopLevelRef(s)
	if !reflect.DeepEqual(got, target) {
		t.Errorf("expected ref to resolve to target, got %v", got)
	}
}

// TestCompactParamsSchema_Nil returns nil for a nil schema.
func TestCompactParamsSchema_Nil(t *testing.T) {
	if got := compactParamsSchema(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// TestCompactParamsSchema_NoProperties returns a permissive open object
// schema when the input has no `properties` field.
func TestCompactParamsSchema_NoProperties(t *testing.T) {
	got := compactParamsSchema(map[string]any{"type": "object"})
	if got["type"] != "object" {
		t.Errorf("type = %v, want object", got["type"])
	}
	if got["additionalProperties"] != true {
		t.Errorf("additionalProperties = %v, want true", got["additionalProperties"])
	}
	if _, hasProps := got["properties"]; hasProps {
		t.Errorf("expected no properties field, got %v", got["properties"])
	}
}

// TestCompactParamsSchema_NonObjectProperty replaces non-map property values
// (e.g. arrays, scalars) with empty schemas rather than panicking.
func TestCompactParamsSchema_NonObjectProperty(t *testing.T) {
	got := compactParamsSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"weird": "not-a-map",
			"id":    map[string]any{"type": "integer"},
		},
	})
	props, _ := got["properties"].(map[string]any)
	weird, _ := props["weird"].(map[string]any)
	if weird == nil {
		t.Fatalf("expected empty map for non-object property, got %v", props["weird"])
	}
	if len(weird) != 0 {
		t.Errorf("expected empty schema for non-object property, got %v", weird)
	}
	id, _ := props["id"].(map[string]any)
	if id["type"] != "integer" {
		t.Errorf("id type = %v, want integer", id["type"])
	}
}

// TestCompactParamsSchema_PropertyWithEnum keeps only type and enum fields
// per property; description and other metadata are dropped.
func TestCompactParamsSchema_PropertyWithEnum(t *testing.T) {
	got := compactParamsSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"state": map[string]any{
				"type":        "string",
				"enum":        []any{"open", "closed"},
				"description": "should be stripped",
			},
		},
	})
	props, _ := got["properties"].(map[string]any)
	state, _ := props["state"].(map[string]any)
	if state["type"] != "string" {
		t.Errorf("type = %v, want string", state["type"])
	}
	enum, ok := state["enum"].([]any)
	if !ok || len(enum) != 2 {
		t.Errorf("enum = %v, want [open closed]", state["enum"])
	}
	if _, has := state["description"]; has {
		t.Errorf("expected description to be stripped, got %v", state["description"])
	}
}

// TestCompactParamsSchema_ResolvesTopLevelRef inlines a $ref before
// compacting so the final schema reflects the referenced definition.
func TestCompactParamsSchema_ResolvesTopLevelRef(t *testing.T) {
	s := map[string]any{
		"$ref": "#/$defs/Foo",
		"$defs": map[string]any{
			"Foo": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "integer", "description": "x"},
				},
			},
		},
	}
	got := compactParamsSchema(s)
	props, _ := got["properties"].(map[string]any)
	id, _ := props["id"].(map[string]any)
	if id["type"] != "integer" {
		t.Errorf("expected $ref to resolve before compacting, got %v", got)
	}
	if _, has := got["$defs"]; has {
		t.Error("expected $defs to be dropped from compacted schema")
	}
}

// TestBuildMetaOneOf_NilInputSchemaFallsBackToOpenObject substitutes a
// permissive object schema when a route does not declare an InputSchema.
func TestBuildMetaOneOf_NilInputSchemaFallsBackToOpenObject(t *testing.T) {
	routes := ActionMap{
		"act": {Handler: func(_ context.Context, _ map[string]any) (any, error) {
			return struct{}{}, nil
		}},
	}
	branches := buildMetaOneOf(routes, []string{"act"}, false)
	if len(branches) != 1 {
		t.Fatalf("len(branches) = %d, want 1", len(branches))
	}
	branch, _ := branches[0].(map[string]any)
	props, _ := branch["properties"].(map[string]any)
	params, _ := props["params"].(map[string]any)
	if params["type"] != "object" {
		t.Errorf("nil InputSchema should fall back to type:object, got %v", params)
	}
	if params["additionalProperties"] != true {
		t.Errorf("nil InputSchema should set additionalProperties:true, got %v", params)
	}
}

// TestSchemaForType_PointerType dereferences pointer types so *T and T
// produce the same cached schema.
func TestSchemaForType_PointerType(t *testing.T) {
	type sample struct {
		Name string `json:"name"`
	}
	direct := schemaForType(reflect.TypeFor[sample]())
	pointer := schemaForType(reflect.TypeFor[*sample]())
	if direct == nil || pointer == nil {
		t.Fatal("expected non-nil schemas for both T and *T")
	}
	if !reflect.DeepEqual(direct, pointer) {
		t.Errorf("schemaForType(T) and schemaForType(*T) should be equal,\n  T  = %v\n  *T = %v", direct, pointer)
	}
}

// TestSchemaForType_CacheHit returns the cached schema for repeated calls
// with the same reflect.Type.
func TestSchemaForType_CacheHit(t *testing.T) {
	type cached struct {
		ID int `json:"id"`
	}
	first := schemaForType(reflect.TypeFor[cached]())
	second := schemaForType(reflect.TypeFor[cached]())
	if first == nil || second == nil {
		t.Fatal("expected non-nil schemas")
	}
	// Cache hit should return the same map pointer.
	if reflect.ValueOf(first).Pointer() != reflect.ValueOf(second).Pointer() {
		t.Error("expected cached schema map to be reused on second call")
	}
}

// TestInputSchemaForType_RequiredSuffixRemoved verifies requiredness is kept in
// the schema's required array instead of leaking into property descriptions.
func TestInputSchemaForType_RequiredSuffixRemoved(t *testing.T) {
	type requiredDescription struct {
		Name string `json:"name" jsonschema:"Resource name,required"`
	}

	schema := inputSchemaForType(reflect.TypeFor[requiredDescription]())
	properties := schema["properties"].(map[string]any)
	name := properties["name"].(map[string]any)
	if name["description"] != "Resource name" {
		t.Fatalf("description = %q, want Resource name", name["description"])
	}
	required := schema["required"].([]string)
	if !reflect.DeepEqual(required, []string{"name"}) {
		t.Fatalf("required = %v, want [name]", required)
	}
}

// TestInputSchemaForType_SecretFieldsWriteOnly verifies token inputs are marked
// write-only without relying on jsonschema-go tag support.
func TestInputSchemaForType_SecretFieldsWriteOnly(t *testing.T) {
	type embeddedSecret struct {
		SigningToken string `json:"signing_token,omitempty" jsonschema:"Signing token"`
	}
	type secretInput struct {
		embeddedSecret
		Token string `json:"token,omitempty" jsonschema:"Secret token"`
		Name  string `json:"name,omitempty" jsonschema:"Name"`
	}

	schema := inputSchemaForType(reflect.TypeFor[secretInput]())
	properties := schema["properties"].(map[string]any)
	for _, key := range []string{"token", "signing_token"} {
		property := properties[key].(map[string]any)
		if property["writeOnly"] != true {
			t.Fatalf("%s writeOnly = %v, want true", key, property["writeOnly"])
		}
	}
	name := properties["name"].(map[string]any)
	if _, ok := name["writeOnly"]; ok {
		t.Fatalf("name should not be writeOnly: %v", name)
	}
}

// TestInputSchemaForType_UnsupportedTypeReturnsNil verifies schema generation
// returns nil for input types that jsonschema cannot represent.
//
// The unsupported struct contains a channel field. The expected nil schema keeps
// callers from registering incomplete MCP input schemas for unsupported Go
// shapes.
func TestInputSchemaForType_UnsupportedTypeReturnsNil(t *testing.T) {
	type unsupportedInput struct {
		Ch chan int `json:"ch"`
	}

	if schema := inputSchemaForType(reflect.TypeFor[unsupportedInput]()); schema != nil {
		t.Fatalf("inputSchemaForType() = %v, want nil", schema)
	}
}

// TestInputSchemaForType_PointerInputCacheHit verifies pointer input schemas are
// cached and still mark secret fields write-only.
//
// The same pointer type is requested twice and should return the same cached map
// instance. The test also checks the token field metadata so cache hits do not
// bypass secret-field decoration.
func TestInputSchemaForType_PointerInputCacheHit(t *testing.T) {
	type pointerSecretInput struct {
		Token string `json:"token,omitempty"`
	}

	first := inputSchemaForType(reflect.TypeFor[*pointerSecretInput]())
	second := inputSchemaForType(reflect.TypeFor[*pointerSecretInput]())
	if first == nil || second == nil {
		t.Fatal("expected non-nil schemas")
	}
	if reflect.ValueOf(first).Pointer() != reflect.ValueOf(second).Pointer() {
		t.Fatal("expected cached input schema map to be reused")
	}
	properties := first["properties"].(map[string]any)
	token := properties["token"].(map[string]any)
	if token["writeOnly"] != true {
		t.Fatalf("token writeOnly = %v, want true", token["writeOnly"])
	}
}

// TestFieldTiers_ReadsTierTag verifies FieldTiers extracts the per-field
// `tier:` struct tag (keyed by JSON name, lowercased), flattens anonymous
// embeds, and returns nil when no field declares a tier.
func TestFieldTiers_ReadsTierTag(t *testing.T) {
	type embedded struct {
		Weight int `json:"weight,omitempty" tier:"Premium"`
	}
	type input struct {
		embedded
		Title  string `json:"title"`
		Health string `json:"health_status,omitempty" tier:"ultimate"`
		Hidden string `json:"-" tier:"premium"`
	}
	got := FieldTiers(reflect.TypeFor[input]())
	want := map[string]string{"weight": "premium", "health_status": "ultimate"}
	if len(got) != len(want) {
		t.Fatalf("FieldTiers = %v, want %v", got, want)
	}
	for name, tier := range want {
		if got[name] != tier {
			t.Errorf("FieldTiers[%q] = %q, want %q", name, got[name], tier)
		}
	}
	if _, ok := got["title"]; ok {
		t.Error("untagged field title must not appear")
	}
	if _, ok := got["-"]; ok {
		t.Error("json:\"-\" field must be skipped")
	}

	type plain struct {
		Name string `json:"name"`
	}
	if FieldTiers(reflect.TypeFor[plain]()) != nil {
		t.Error("FieldTiers should be nil when no field declares a tier")
	}
}

// TestMarkWriteOnlySecretFields_IgnoresMissingProperties verifies secret-field
// marking leaves schemas without a properties map unchanged: passing a primitive
// schema adds no writeOnly flag, preserving non-object schemas generated for unusual inputs.
func TestMarkWriteOnlySecretFields_IgnoresMissingProperties(t *testing.T) {
	schema := map[string]any{"type": "string"}
	markWriteOnlySecretFields(schema, reflect.TypeFor[string]())

	if _, ok := schema["writeOnly"]; ok {
		t.Fatalf("schema should not be marked writeOnly: %v", schema)
	}
}

// TestMarkWriteOnlySecretFields_IgnoresNonSchemaProperties verifies secret-field
// marking skips properties that are not schema maps.
//
// The token property is intentionally a string value. The helper should not
// mutate it, avoiding panics or type corruption when schema generation returns
// unexpected property metadata.
func TestMarkWriteOnlySecretFields_IgnoresNonSchemaProperties(t *testing.T) {
	type secretInput struct {
		Token string `json:"token,omitempty"`
	}
	schema := map[string]any{"properties": map[string]any{"token": "not-a-schema"}}
	markWriteOnlySecretFields(schema, reflect.TypeFor[secretInput]())

	properties := schema["properties"].(map[string]any)
	if properties["token"] != "not-a-schema" {
		t.Fatalf("token property mutated unexpectedly: %v", properties["token"])
	}
}

// TestSecretJSONFieldNames_EdgeCases verifies secret JSON field discovery handles
// pointers, embedded structs, ignored tags, and unexported fields.
//
// The test expects no fields for a non-struct pointer and only signing_token plus
// token for the struct with an embedded pointer. Ignored and private fields must
// not be reported as schema secrets.
func TestSecretJSONFieldNames_EdgeCases(t *testing.T) {
	type embeddedSecret struct {
		SigningToken string `json:"signing_token,omitempty"`
	}
	type secretInput struct {
		*embeddedSecret
		ignored string
		Ignored string `json:"-"`
		Token   string `json:"token,omitempty"`
	}

	_ = secretInput{ignored: "private"}
	if got := secretJSONFieldNames(reflect.TypeFor[*string]()); len(got) != 0 {
		t.Fatalf("non-struct secret fields = %v, want empty", got)
	}
	if got := secretJSONFieldNames(reflect.TypeFor[secretInput]()); !reflect.DeepEqual(got, []string{"signing_token", "token"}) {
		t.Fatalf("secret fields = %v, want [signing_token token]", got)
	}
}

// TestStripReservedKeys_MultipleKeys verifies that real keys are preserved
// when reserved keys are also present (covers the "copy" branch).
func TestStripReservedKeys_MultipleKeys(t *testing.T) {
	in := map[string]any{
		"confirm": true,
		"name":    "proj",
		"id":      42,
	}
	out := stripReservedKeys(in)
	if _, has := out["confirm"]; has {
		t.Error("expected confirm to be stripped")
	}
	if out["name"] != "proj" {
		t.Errorf("name = %v, want proj", out["name"])
	}
	if out["id"] != 42 {
		t.Errorf("id = %v, want 42", out["id"])
	}
	// Original map must not be mutated.
	if _, has := in["confirm"]; !has {
		t.Error("stripReservedKeys mutated the input map")
	}
}

// TestEnrichWithHints_NonObjectJSONFromArray returns the result unchanged
// when the marshaled JSON is an array (does not start with `{`).
func TestEnrichWithHints_NonObjectJSONFromArray(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Some output\n\n💡 **Next steps:**\n- do thing\n"},
		},
	}
	result := []string{"a", "b"}
	got := enrichWithHints(result, callResult)
	// Array inputs must be returned unchanged because we only enrich JSON
	// objects to keep the {next_steps, ...} contract well-defined.
	gotSlice, ok := got.([]string)
	if !ok || len(gotSlice) != 2 {
		t.Errorf("expected array result returned unchanged, got %v (%T)", got, got)
	}
}

// TestEnrichWithHints_NonTextContentSkipped iterates past non-text content
// blocks when looking for hints; only TextContent contributes to extraction.
func TestEnrichWithHints_NonTextContentSkipped(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			// Non-text content (e.g. resource link) must not panic and must
			// not be inspected for hints.
			&mcp.ResourceLink{URI: "gitlab://resource"},
		},
	}
	result := map[string]any{"ok": true}
	got := enrichWithHints(result, callResult)
	// No hints found → input must be returned unchanged.
	if !reflect.DeepEqual(got, result) {
		t.Errorf("expected result unchanged when no text content, got %v", got)
	}
}

// TestUnmarshalParams_DoubleFailureReturnsOriginalError confirms that when
// neither the strict pass nor the numeric-string-coerced retry succeed, the
// original error message is preserved (rather than the retry's).
func TestUnmarshalParams_DoubleFailureReturnsOriginalError(t *testing.T) {
	type strictInput struct {
		ID int `json:"id"`
	}
	// "id" cannot be coerced from a non-numeric string to int even after
	// numeric-string coercion, so both passes fail.
	_, err := UnmarshalParams[strictInput](map[string]any{"id": "not-a-number"})
	if err == nil {
		t.Fatal("expected error from double-failure path")
	}
	if !strings.Contains(err.Error(), "invalid params for this action") {
		t.Errorf("expected wrapped error message, got %q", err.Error())
	}
}

// routeSchemaTestInput defines parameters for the route schema test operation.
type routeSchemaTestInput struct {
	ID int `json:"id"`
}

// TestRouteVoidAction_ValidInput_ReturnsTypedOutput verifies that a void route
// returns a typed success output for valid input.
func TestRouteVoidAction_ValidInput_ReturnsTypedOutput(t *testing.T) {
	t.Parallel()

	route := RouteVoidAction((*gitlabclient.Client)(nil), func(_ context.Context, _ *gitlabclient.Client, input routeSchemaTestInput) error {
		if input.ID != 7 {
			t.Fatalf("input.ID = %d, want 7", input.ID)
		}
		return nil
	})

	if route.OutputSchema == nil {
		t.Fatal("RouteVoidAction OutputSchema is nil")
	}
	if route.Destructive {
		t.Fatal("RouteVoidAction marked route destructive")
	}

	result, err := route.Handler(context.Background(), map[string]any{"id": 7})
	if err != nil {
		t.Fatalf("route handler returned error: %v", err)
	}
	out, ok := result.(VoidOutput)
	if !ok {
		t.Fatalf("route handler result type = %T, want VoidOutput", result)
	}
	if out.Status != "success" {
		t.Fatalf("VoidOutput.Status = %q, want success", out.Status)
	}
	if out.Message == "" {
		t.Fatal("VoidOutput.Message is empty")
	}
}

// TestRouteVoidAction_InvalidInput_ReturnsError verifies that invalid input is
// rejected before the wrapped void handler runs.
func TestRouteVoidAction_InvalidInput_ReturnsError(t *testing.T) {
	t.Parallel()

	route := RouteVoidAction((*gitlabclient.Client)(nil), func(context.Context, *gitlabclient.Client, routeSchemaTestInput) error {
		t.Fatal("handler should not be called for invalid input")
		return nil
	})

	result, err := route.Handler(context.Background(), map[string]any{"unknown": true})
	if err == nil {
		t.Fatal("route handler returned nil error for invalid input")
	}
	if result != nil {
		t.Fatalf("route handler result = %#v, want nil", result)
	}
}

// TestDestructiveVoidAction_ValidInput_ReturnsTypedOutput verifies that a
// destructive void route returns DeleteOutput and marks the route destructive.
func TestDestructiveVoidAction_ValidInput_ReturnsTypedOutput(t *testing.T) {
	t.Parallel()

	route := DestructiveVoidAction((*gitlabclient.Client)(nil), func(_ context.Context, _ *gitlabclient.Client, input routeSchemaTestInput) error {
		if input.ID != 11 {
			t.Fatalf("input.ID = %d, want 11", input.ID)
		}
		return nil
	})

	if route.OutputSchema == nil {
		t.Fatal("DestructiveVoidAction OutputSchema is nil")
	}
	if !route.Destructive {
		t.Fatal("DestructiveVoidAction did not mark route destructive")
	}

	result, err := route.Handler(context.Background(), map[string]any{"id": 11})
	if err != nil {
		t.Fatalf("route handler returned error: %v", err)
	}
	out, ok := result.(DeleteOutput)
	if !ok {
		t.Fatalf("route handler result type = %T, want DeleteOutput", result)
	}
	if out.Status != "success" {
		t.Fatalf("DeleteOutput.Status = %q, want success", out.Status)
	}
	if out.Message == "" {
		t.Fatal("DeleteOutput.Message is empty")
	}
}

// TestDestructiveVoidAction_HandlerError_PropagatesError verifies that errors
// from destructive void handlers are returned unchanged.
func TestDestructiveVoidAction_HandlerError_PropagatesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("delete failed")
	route := DestructiveVoidAction((*gitlabclient.Client)(nil), func(context.Context, *gitlabclient.Client, routeSchemaTestInput) error {
		return wantErr
	})

	result, err := route.Handler(context.Background(), map[string]any{"id": 1})
	if !errors.Is(err, wantErr) {
		t.Fatalf("route handler error = %v, want %v", err, wantErr)
	}
	if result != nil {
		t.Fatalf("route handler result = %#v, want nil", result)
	}
}

// TestWithVoidOutput_NilResult_ReturnsSuccessOutput verifies that nil inner
// results are replaced with the configured success output.
func TestWithVoidOutput_NilResult_ReturnsSuccessOutput(t *testing.T) {
	t.Parallel()

	sentinel := struct{ OK bool }{OK: true}
	var nilResult any
	inner := func(_ context.Context, _ map[string]any) (any, error) { return nilResult, nil }
	wrapped := withVoidOutput(inner, sentinel)

	result, err := wrapped(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != sentinel {
		t.Fatalf("result = %#v, want %#v", result, sentinel)
	}
}

// TestWithVoidOutput_NonNilResult_PassesThrough verifies that non-nil inner
// results are returned unchanged.
func TestWithVoidOutput_NonNilResult_PassesThrough(t *testing.T) {
	t.Parallel()

	original := struct{ Val int }{Val: 42}
	inner := func(_ context.Context, _ map[string]any) (any, error) { return original, nil }
	wrapped := withVoidOutput(inner, struct{ OK bool }{OK: true})

	result, err := wrapped(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != original {
		t.Fatalf("result = %#v, want %#v", result, original)
	}
}

// TestWithVoidOutput_InnerError_PropagatesError verifies that inner handler
// errors bypass success-output substitution.
func TestWithVoidOutput_InnerError_PropagatesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("inner failure")
	inner := func(_ context.Context, _ map[string]any) (any, error) { return nil, wantErr }
	wrapped := withVoidOutput(inner, struct{}{})

	result, err := wrapped(context.Background(), nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
}

// TestDestructiveVoidActionWithRequest_ValidInput_ReturnsDeleteOutput verifies
// that request-aware destructive void routes return DeleteOutput on success.
func TestDestructiveVoidActionWithRequest_ValidInput_ReturnsDeleteOutput(t *testing.T) {
	t.Parallel()

	route := DestructiveVoidActionWithRequest((*gitlabclient.Client)(nil),
		func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ routeSchemaTestInput) error {
			return nil
		})

	if !route.Destructive {
		t.Fatal("expected Destructive = true")
	}
	if route.OutputSchema == nil {
		t.Fatal("expected OutputSchema to be set")
	}
	result, err := route.Handler(context.Background(), map[string]any{"id": 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, ok := result.(DeleteOutput)
	if !ok {
		t.Fatalf("result type = %T, want DeleteOutput", result)
	}
	if out.Status != "success" {
		t.Fatalf("Status = %q, want \"success\"", out.Status)
	}
}

// TestMetaToolVoidActions_ProtocolCall_ReturnsStructuredContent verifies that
// protocol-level calls to void actions include typed structured content.
func TestMetaToolVoidActions_ProtocolCall_ReturnsStructuredContent(t *testing.T) {
	routes := ActionMap{
		"delete": DestructiveVoidAction((*gitlabclient.Client)(nil), func(context.Context, *gitlabclient.Client, routeSchemaTestInput) error {
			return nil
		}),
		"void": RouteVoidAction((*gitlabclient.Client)(nil), func(context.Context, *gitlabclient.Client, routeSchemaTestInput) error {
			return nil
		}),
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	AddMetaTool(server, "test_meta", "Test meta tool.", routes, nil, nil)
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	voidResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "test_meta",
		Arguments: map[string]any{
			"action": "void",
			"params": map[string]any{"id": 1},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(void): %v", err)
	}
	if voidResult.IsError {
		t.Fatalf("CallTool(void) returned IsError result: %#v", voidResult)
	}
	if len(voidResult.Content) == 0 {
		t.Fatal("CallTool(void) returned no content")
	}
	var voidOut VoidOutput
	rawVoid, err := json.Marshal(voidResult.StructuredContent)
	if err != nil {
		t.Fatalf("marshal void structured content: %v", err)
	}
	err = json.Unmarshal(rawVoid, &voidOut)
	if err != nil {
		t.Fatalf("unmarshal void structured content: %v", err)
	}
	if voidOut.Status != "success" || voidOut.Message == "" {
		t.Fatalf("void structured content = %+v, want success status and message", voidOut)
	}

	deleteResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "test_meta",
		Arguments: map[string]any{
			"action": "delete",
			"params": map[string]any{"id": 1, "confirm": true},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(delete): %v", err)
	}
	if deleteResult.IsError {
		t.Fatalf("CallTool(delete) returned IsError result: %#v", deleteResult)
	}
	var deleteOut DeleteOutput
	rawDelete, err := json.Marshal(deleteResult.StructuredContent)
	if err != nil {
		t.Fatalf("marshal delete structured content: %v", err)
	}
	err = json.Unmarshal(rawDelete, &deleteOut)
	if err != nil {
		t.Fatalf("unmarshal delete structured content: %v", err)
	}
	if deleteOut.Status != "success" || deleteOut.Message == "" {
		t.Fatalf("delete structured content = %+v, want success status and message", deleteOut)
	}
}

// TestDestructiveVoidActionWithRequest_HandlerError_PropagatesError verifies
// that request-aware destructive void routes propagate handler errors.
func TestDestructiveVoidActionWithRequest_HandlerError_PropagatesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("request delete failed")
	route := DestructiveVoidActionWithRequest((*gitlabclient.Client)(nil),
		func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ routeSchemaTestInput) error {
			return wantErr
		})

	result, err := route.Handler(context.Background(), map[string]any{"id": 1})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
}

// TestWrapVoidActionWithRequest_Success_ReturnsNil verifies that request-aware
// void wrappers return nil output when the handler succeeds.
func TestWrapVoidActionWithRequest_Success_ReturnsNil(t *testing.T) {
	t.Parallel()

	wrapped := WrapVoidActionWithRequest((*gitlabclient.Client)(nil),
		func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ routeSchemaTestInput) error {
			return nil
		})

	result, err := wrapped(context.Background(), map[string]any{"id": 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
}

// TestWrapVoidActionWithRequest_Error_PropagatesError verifies that
// request-aware void wrappers return handler errors unchanged.
func TestWrapVoidActionWithRequest_Error_PropagatesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("wrap void request error")
	wrapped := WrapVoidActionWithRequest((*gitlabclient.Client)(nil),
		func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ routeSchemaTestInput) error {
			return wantErr
		})

	result, err := wrapped(context.Background(), map[string]any{"id": 1})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
}

// TestWrapVoidActionWithRequest_UnmarshalError_ReturnsError verifies that
// request-aware void wrappers reject invalid params before calling the handler.
func TestWrapVoidActionWithRequest_UnmarshalError_ReturnsError(t *testing.T) {
	t.Parallel()

	wrapped := WrapVoidActionWithRequest((*gitlabclient.Client)(nil),
		func(_ context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ routeSchemaTestInput) error {
			return nil
		})

	// Pass a param with the wrong type to trigger UnmarshalParams error.
	result, err := wrapped(context.Background(), map[string]any{"id": "not-an-int"})
	if err == nil {
		t.Fatal("expected error for invalid params, got nil")
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil on error", result)
	}
}

// TestNormalizeActionAliasForParams_EnvironmentRouting verifies that the
// param-shape-based routing for gitlab_environment/get dispatches to
// protected_get when the params contain a non-numeric environment identifier,
// and falls back to the plain get action otherwise.
func TestNormalizeActionAliasForParams_EnvironmentRouting(t *testing.T) {
	routes := ActionMap{
		"get":           Route(nil),
		"protected_get": Route(nil),
	}

	tests := []struct {
		name     string
		toolName string
		action   string
		params   map[string]any
		want     string
	}{
		{
			name:     "named environment routes to protected_get",
			toolName: "gitlab_environment",
			action:   "get",
			params:   map[string]any{"environment": "staging"},
			want:     "protected_get",
		},
		{
			name:     "non-numeric environment_id routes to protected_get",
			toolName: "gitlab_environment",
			action:   "get",
			params:   map[string]any{"environment_id": "staging"},
			want:     "protected_get",
		},
		{
			name:     "numeric environment_id stays on get",
			toolName: "gitlab_environment",
			action:   "get",
			params:   map[string]any{"environment_id": "42"},
			want:     "get",
		},
		{
			name:     "non-matching tool name returns action unchanged",
			toolName: "gitlab_project",
			action:   "get",
			params:   map[string]any{"environment": "staging"},
			want:     "get",
		},
		{
			name:     "non-get action returns unchanged",
			toolName: "gitlab_environment",
			action:   "list",
			params:   map[string]any{"environment": "staging"},
			want:     "list",
		},
		{
			name:     "protected_get missing from routes falls back to get",
			toolName: "gitlab_environment",
			action:   "get",
			params:   map[string]any{"environment": "staging"},
			want:     "get",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := routes
			if tc.name == "protected_get missing from routes falls back to get" {
				r = ActionMap{"get": Route(nil)}
			}
			got := NormalizeActionAliasForParams(tc.toolName, tc.action, tc.params, r)
			if got != tc.want {
				t.Fatalf("NormalizeActionAliasForParams(%q, %q, %v) = %q, want %q",
					tc.toolName, tc.action, tc.params, got, tc.want)
			}
		})
	}
}

// TestHasProtectedEnvironmentNameParam_VariousCases verifies the param-shape
// detection helper that distinguishes named environments from numeric IDs.
func TestHasProtectedEnvironmentNameParam_VariousCases(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
		want   bool
	}{
		{
			name:   "empty params returns false",
			params: map[string]any{},
			want:   false,
		},
		{
			name:   "nil params returns false",
			params: nil,
			want:   false,
		},
		{
			name:   "environment key present returns true",
			params: map[string]any{"environment": "staging"},
			want:   true,
		},
		{
			name:   "numeric environment_id returns false",
			params: map[string]any{"environment_id": "42"},
			want:   false,
		},
		{
			name:   "non-numeric environment_id returns true",
			params: map[string]any{"environment_id": "staging"},
			want:   true,
		},
		{
			name:   "float-like environment_id string returns true",
			params: map[string]any{"environment_id": "3.14"},
			want:   true,
		},
		{
			name:   "unrelated param does not match",
			params: map[string]any{"project_id": "my/project"},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hasProtectedEnvironmentNameParam(tc.params)
			if got != tc.want {
				t.Fatalf("hasProtectedEnvironmentNameParam(%v) = %v, want %v", tc.params, got, tc.want)
			}
		})
	}
}

// TestFieldTiers_NilType_ReturnsNil verifies that [FieldTiers] returns nil
// for a nil reflect.Type instead of panicking.
func TestFieldTiers_NilType_ReturnsNil(t *testing.T) {
	if got := FieldTiers(nil); got != nil {
		t.Errorf("FieldTiers(nil) = %v, want nil", got)
	}
}

// TestFieldTiers_PointerType_Dereferences verifies that [FieldTiers]
// dereferences pointer types to their struct element before collecting
// tier tags.
func TestFieldTiers_PointerType_Dereferences(t *testing.T) {
	type tiered struct {
		Plan string `json:"plan" tier:"premium"`
	}
	got := FieldTiers(reflect.TypeFor[*tiered]())
	if got["plan"] != "premium" {
		t.Errorf("FieldTiers(*tiered) = %v, want plan:premium", got)
	}
}

// TestFieldTiers_NonStructType_ReturnsNil verifies that [FieldTiers] returns
// nil for non-struct kinds, which carry no field tier metadata.
func TestFieldTiers_NonStructType_ReturnsNil(t *testing.T) {
	if got := FieldTiers(reflect.TypeFor[string]()); got != nil {
		t.Errorf("FieldTiers(string) = %v, want nil", got)
	}
}

// TestFieldTiers_UnexportedField_Skipped verifies that unexported,
// non-anonymous struct fields are skipped while exported tier-tagged fields
// are still collected.
func TestFieldTiers_UnexportedField_Skipped(t *testing.T) {
	got := FieldTiers(reflect.TypeOf(struct {
		hidden string // present only to exercise the unexported-field skip branch via reflection.
		Public string `json:"public" tier:"ultimate"`
	}{}))
	if len(got) != 1 || got["public"] != "ultimate" {
		t.Errorf("FieldTiers(unexported+exported) = %v, want only public:ultimate", got)
	}
}

// Schema-driven alias explanations.

// TestNormalizeParamAliasesForSchemaWithExplanation_IIDAlias verifies that a
// bare "iid" parameter is explained as an alias of the schema's single
// *_iid property when the schema does not accept "iid" directly.
func TestNormalizeParamAliasesForSchemaWithExplanation_IIDAlias(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mr_iid": map[string]any{"type": "integer"},
		},
	}
	params := map[string]any{"iid": 5}

	_, explanations := NormalizeParamAliasesForSchemaWithExplanation(params, schema)

	found := false
	for _, e := range explanations {
		if e.Alias == "iid" && e.Canonical == "mr_iid" {
			found = true
		}
	}
	if !found {
		t.Errorf("explanations = %+v, want iid → mr_iid entry", explanations)
	}
}

// TestNormalizeParamAliasesForSchemaWithExplanation_EnvironmentIDAlias
// verifies that "environment_id" is explained as an alias of the schema's
// "environment" property when the schema accepts "environment" but not
// "environment_id".
func TestNormalizeParamAliasesForSchemaWithExplanation_EnvironmentIDAlias(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"environment": map[string]any{"type": "string"},
		},
	}
	params := map[string]any{"environment_id": 7}

	_, explanations := NormalizeParamAliasesForSchemaWithExplanation(params, schema)

	found := false
	for _, e := range explanations {
		if e.Alias == "environment_id" && e.Canonical == "environment" {
			found = true
		}
	}
	if !found {
		t.Errorf("explanations = %+v, want environment_id → environment entry", explanations)
	}
}

// Encoded path identifiers.

// TestDecodeEncodedPathIdentifier_InvalidEscape_ReturnsFalse verifies that a
// value containing "%2f" but with a malformed trailing escape sequence is
// rejected (url.PathUnescape error) instead of being partially decoded.
func TestDecodeEncodedPathIdentifier_InvalidEscape_ReturnsFalse(t *testing.T) {
	decoded, ok := decodeEncodedPathIdentifier("group%2fproject%")
	if ok || decoded != "" {
		t.Errorf("decodeEncodedPathIdentifier(invalid escape) = (%q, %v), want (\"\", false)", decoded, ok)
	}
}

// File path alias normalization.

// TestNormalizeFilePathAlias_ExistingFilename_NoRewrite verifies that
// normalizeFilePathAlias leaves params untouched when the output already
// carries a "filename" value, so a caller-provided filename is never
// overwritten by the file_path split.
func TestNormalizeFilePathAlias_ExistingFilename_NoRewrite(t *testing.T) {
	out := map[string]any{"file_path": "docs/readme.md", "filename": "custom.md"}
	accepts := func(name string) bool { return name == "path" || name == "filename" }
	cloned := false
	clone := func() map[string]any {
		cloned = true
		return maps.Clone(out)
	}

	normalizeFilePathAlias(out, accepts, clone)

	if cloned {
		t.Error("normalizeFilePathAlias cloned params despite existing filename")
	}
	if _, hasPath := out["path"]; hasPath {
		t.Errorf("normalizeFilePathAlias added path key: %v", out)
	}
}

// JSON field reflection fallbacks.

// EmbeddedID is a named non-struct type embedded anonymously in test structs
// to exercise the empty-JSON-name fallback: anonymous fields yield an empty
// jsonFieldName, and non-struct kinds cannot be flattened, so the collectors
// must fall back to the Go field name. Exported so the embedded field is not
// skipped by the unexported-field guard.
type EmbeddedID int

// TestCollectJSONFieldNames_AnonymousNonStruct_UsesGoFieldName verifies that
// an anonymous embedded non-struct field (which has no JSON tag name and
// cannot be flattened) falls back to its Go field name when collecting
// accepted parameter names.
func TestCollectJSONFieldNames_AnonymousNonStruct_UsesGoFieldName(t *testing.T) {
	fields := map[string]struct{}{}
	collectJSONFieldNames(reflect.TypeOf(struct{ EmbeddedID }{}), fields)
	if _, ok := fields["EmbeddedID"]; !ok {
		t.Errorf("collectJSONFieldNames(embedded non-struct) = %v, want EmbeddedID key", fields)
	}
}

// TestCollectJSONFieldTypes_AnonymousNonStruct_UsesGoFieldName verifies that
// jsonFieldTypes falls back to the Go field name for an anonymous embedded
// non-struct field, mirroring encoding/json field promotion.
func TestCollectJSONFieldTypes_AnonymousNonStruct_UsesGoFieldName(t *testing.T) {
	fields := jsonFieldTypes(reflect.TypeOf(struct{ EmbeddedID }{}))
	if got, ok := fields["EmbeddedID"]; !ok || got.Kind() != reflect.Int {
		t.Errorf("jsonFieldTypes(embedded non-struct) = %v, want EmbeddedID:int", fields)
	}
}

// UnmarshalParams edge cases.

// TestUnmarshalParams_UnmarshalableValue_ReturnsValidationError verifies
// that a parameter value that cannot be serialized to JSON (a function)
// produces a ParamValidationError from the initial json.Marshal step.
func TestUnmarshalParams_UnmarshalableValue_ReturnsValidationError(t *testing.T) {
	type input struct {
		X any `json:"x"`
	}
	_, err := UnmarshalParams[input](map[string]any{"x": func() {}})
	if err == nil {
		t.Fatal("UnmarshalParams(func value) error = nil, want marshal error")
	}
	var validationErr *ParamValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("UnmarshalParams(func value) error = %T, want *ParamValidationError", err)
	}
}

// TestUnmarshalParams_MapTarget_RetriesWithBroadCoercion verifies the legacy
// retry path: for a map target (no reflectable JSON fields, so the typed
// coercion pipeline is skipped) a numeric string fails strict unmarshalling
// first and then succeeds after coerceNumericStrings converts it.
func TestUnmarshalParams_MapTarget_RetriesWithBroadCoercion(t *testing.T) {
	got, err := UnmarshalParams[map[string]int](map[string]any{"n": "5"})
	if err != nil {
		t.Fatalf("UnmarshalParams(map target, numeric string) error = %v", err)
	}
	if got["n"] != 5 {
		t.Errorf("UnmarshalParams(map target) = %v, want n:5", got)
	}
}

// TestUnmarshalParams_MapTarget_NaNRetryMarshalFails verifies that when the
// broad numeric coercion retry produces a value JSON cannot encode (the
// string "NaN" parses to a float64 NaN), the retry marshal fails and the
// original strict unmarshal error is preserved.
func TestUnmarshalParams_MapTarget_NaNRetryMarshalFails(t *testing.T) {
	_, err := UnmarshalParams[map[string]int](map[string]any{"n": "NaN"})
	if err == nil {
		t.Fatal("UnmarshalParams(map target, NaN string) error = nil, want error")
	}
	var validationErr *ParamValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("UnmarshalParams(NaN string) error = %T, want *ParamValidationError", err)
	}
}

// Structured value coercion.

// TestCoerceStructuredValue_PointerSliceElem_WrapsObject verifies that a
// single JSON object destined for a slice of struct pointers is wrapped in
// a one-element list after the pointer element type is dereferenced.
func TestCoerceStructuredValue_PointerSliceElem_WrapsObject(t *testing.T) {
	type rule struct {
		A int `json:"a"`
	}
	coerced, changed := coerceStructuredValue("rules", map[string]any{"a": 1}, reflect.TypeFor[[]*rule]())
	if !changed {
		t.Fatal("coerceStructuredValue(map for []*struct) changed = false, want true")
	}
	items, ok := coerced.([]any)
	if !ok || len(items) != 1 {
		t.Errorf("coerceStructuredValue(map for []*struct) = %v, want single-item list", coerced)
	}
}

// TestCoerceStructuredValue_StringForStructSlice_Unchanged verifies that a
// scalar string that is neither an object, a role name, nor a list is left
// untouched for a slice-of-struct field.
func TestCoerceStructuredValue_StringForStructSlice_Unchanged(t *testing.T) {
	type rule struct {
		A int `json:"a"`
	}
	coerced, changed := coerceStructuredValue("rules", "hello", reflect.TypeFor[[]rule]())
	if changed || coerced != "hello" {
		t.Errorf("coerceStructuredValue(string for []struct) = (%v, %v), want (hello, false)", coerced, changed)
	}
}

// TestCoerceStructuredValue_NonRoleScalarItems_Unchanged verifies that list
// items that are neither objects nor GitLab role names are kept as-is and
// the overall value is reported unchanged.
func TestCoerceStructuredValue_NonRoleScalarItems_Unchanged(t *testing.T) {
	type rule struct {
		A int `json:"a"`
	}
	value := []any{"not-a-role"}
	coerced, changed := coerceStructuredValue("rules", value, reflect.TypeFor[[]rule]())
	if changed {
		t.Errorf("coerceStructuredValue(scalar items) changed = true, want false")
	}
	if !reflect.DeepEqual(coerced, value) {
		t.Errorf("coerceStructuredValue(scalar items) = %v, want %v", coerced, value)
	}
}

// TestNormalizeStructuredObjectFields_NoReflectableFields_ReturnsValue
// verifies that objects targeted at a struct without JSON fields are
// returned unchanged.
func TestNormalizeStructuredObjectFields_NoReflectableFields_ReturnsValue(t *testing.T) {
	value := map[string]any{"x": 1}
	got := normalizeStructuredObjectFields(value, reflect.TypeFor[struct{}]())
	if !reflect.DeepEqual(got, value) {
		t.Errorf("normalizeStructuredObjectFields(empty struct) = %v, want %v", got, value)
	}
}

// TestNormalizeStructuredObjectFields_ApprovalCountWithoutPrincipal_DefaultsAccessLevel
// verifies that an approval-rule object carrying only a required approval
// count (no access_level/user_id/group_id principal) receives the
// Maintainer access level 40 default.
func TestNormalizeStructuredObjectFields_ApprovalCountWithoutPrincipal_DefaultsAccessLevel(t *testing.T) {
	type approvalRule struct {
		RequiredApprovals int `json:"required_approvals"`
		AccessLevel       int `json:"access_level"`
	}
	got := normalizeStructuredObjectFields(map[string]any{"required_approvals": 2}, reflect.TypeFor[approvalRule]())
	if got["access_level"] != 40 {
		t.Errorf("normalizeStructuredObjectFields(count only)[access_level] = %v, want 40", got["access_level"])
	}
}

// Numeric slice coercion.

// TestCoerceSliceValueForTargetType_PointerElem_CoercesStrings verifies that
// pointer element types are dereferenced before the numeric-kind check so
// numeric strings inside a []*int value are still coerced.
func TestCoerceSliceValueForTargetType_PointerElem_CoercesStrings(t *testing.T) {
	coerced, changed, err := coerceSliceValueForTargetType("ids", []any{"5"}, reflect.TypeFor[*int]())
	if err != nil {
		t.Fatalf("coerceSliceValueForTargetType([]*int) error = %v", err)
	}
	if !changed {
		t.Fatal("coerceSliceValueForTargetType([]*int) changed = false, want true")
	}
	items, ok := coerced.([]any)
	if !ok || len(items) != 1 || items[0] != int64(5) {
		t.Errorf("coerceSliceValueForTargetType([]*int) = %v, want [5]", coerced)
	}
}

// TestCoerceSliceValueForTargetType_NonSliceValue_Unchanged verifies that a
// non-list value for a numeric slice element type is returned unchanged
// without error.
func TestCoerceSliceValueForTargetType_NonSliceValue_Unchanged(t *testing.T) {
	coerced, changed, err := coerceSliceValueForTargetType("ids", 42, reflect.TypeFor[int]())
	if err != nil || changed || coerced != 42 {
		t.Errorf("coerceSliceValueForTargetType(non-slice) = (%v, %v, %v), want (42, false, nil)", coerced, changed, err)
	}
}

// Schema-driven coercion no-ops.

// TestCoerceSchemaParamValue_NumberProperty_NonNumericString_Unchanged
// verifies that values for a number-typed schema property that cannot be
// parsed as floats are passed through unchanged.
func TestCoerceSchemaParamValue_NumberProperty_NonNumericString_Unchanged(t *testing.T) {
	property := map[string]any{"type": "number"}
	got, changed := coerceSchemaParamValue("weight", "abc", property)
	if changed || got != "abc" {
		t.Errorf("coerceSchemaParamValue(number, non-numeric) = (%v, %v), want (abc, false)", got, changed)
	}
}

// TestCoerceSchemaArrayValue_NonSliceValue_Unchanged verifies that a scalar
// value for an integer-array schema property is returned unchanged when it
// cannot be interpreted as a list.
func TestCoerceSchemaArrayValue_NonSliceValue_Unchanged(t *testing.T) {
	property := map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "integer"},
	}
	got, changed := coerceSchemaArrayValue("abc", property)
	if changed || got != "abc" {
		t.Errorf("coerceSchemaArrayValue(non-slice) = (%v, %v), want (abc, false)", got, changed)
	}
}

// TestCoerceSingleStringArraysForSchema_NonStringValue_Skipped verifies that
// non-string values for a string-array schema property are not wrapped in a
// single-element list.
func TestCoerceSingleStringArraysForSchema_NonStringValue_Skipped(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"labels": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
	}
	params := map[string]any{"labels": 7}
	got := coerceSingleStringArraysForSchema(params, schema)
	if !reflect.DeepEqual(got, params) {
		t.Errorf("coerceSingleStringArraysForSchema(non-string) = %v, want unchanged %v", got, params)
	}
}

// MakeMetaHandler destructive confirmation and error propagation.

// TestMakeMetaHandler_DestructiveDeclined_ReturnsCancelled verifies that a
// destructive route is intercepted by the elicitation confirmation flow and
// that a user decline cancels the action without invoking the handler.
func TestMakeMetaHandler_DestructiveDeclined_ReturnsCancelled(t *testing.T) {
	t.Setenv("YOLO_MODE", "")
	t.Setenv("AUTOPILOT", "")

	ss := newConfirmSession(t, func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	})

	executed := false
	routes := ActionMap{
		"delete": DestructiveRoute(func(_ context.Context, _ map[string]any) (any, error) {
			executed = true
			return map[string]any{"ok": true}, nil
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)
	req := &mcp.CallToolRequest{Session: ss, Params: &mcp.CallToolParamsRaw{Name: "test_tool"}}

	result, raw, err := handler(context.Background(), req, MetaToolInput{Action: "delete", Params: map[string]any{}})
	if err != nil {
		t.Fatalf("handler(destructive declined) error = %v", err)
	}
	if executed {
		t.Error("destructive handler executed despite declined confirmation")
	}
	if raw != nil {
		t.Errorf("handler(destructive declined) raw = %v, want nil", raw)
	}
	if result == nil {
		t.Fatal("handler(destructive declined) result = nil, want cancellation result")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(tc.Text, "canceled") {
		t.Errorf("handler(destructive declined) content = %+v, want canceled message", result.Content)
	}
}

// TestMakeMetaHandler_HandlerError_Propagated verifies that a generic
// (non-validation) handler error is returned to the MCP framework unchanged
// instead of being converted into an in-band error result.
func TestMakeMetaHandler_HandlerError_Propagated(t *testing.T) {
	wantErr := errors.New("boom")
	routes := ActionMap{
		"fail": Route(func(_ context.Context, _ map[string]any) (any, error) {
			return nil, wantErr
		}),
	}
	handler := MakeMetaHandler("test_tool", routes, nil)

	result, raw, err := handler(context.Background(), &mcp.CallToolRequest{}, MetaToolInput{Action: "fail", Params: map[string]any{}})
	if !errors.Is(err, wantErr) {
		t.Errorf("handler(generic error) error = %v, want %v", err, wantErr)
	}
	if result != nil || raw != nil {
		t.Errorf("handler(generic error) result/raw = %v/%v, want nil/nil", result, raw)
	}
}

// TestMetaSchemaCompileKey_NonOpaqueMode_ReturnsEmpty verifies that schema
// compile caching is disabled (empty key) outside the default opaque
// META_PARAM_SCHEMA mode, where compact/full schemas have no cheap stable
// identity.
func TestMetaSchemaCompileKey_NonOpaqueMode_ReturnsEmpty(t *testing.T) {
	restore := SetMetaParamSchemaModeScoped(MetaParamSchemaCompact)
	defer restore()

	routes := ActionMap{
		"list": Route(func(_ context.Context, _ map[string]any) (any, error) { return map[string]any{}, nil }),
	}
	if key := metaSchemaCompileKey("gitlab_test", routes); key != "" {
		t.Errorf("metaSchemaCompileKey(compact mode) = %q, want empty", key)
	}
}

// enrichWithHints marshal failure.

// TestEnrichWithHints_UnmarshalableResult_ReturnsOriginal verifies that a
// result value JSON cannot encode (a channel) is returned unchanged even
// when the call result carries next-step hints.
func TestEnrichWithHints_UnmarshalableResult_ReturnsOriginal(t *testing.T) {
	callResult := &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: "Done.\n\n💡 **Next steps:**\n- Use gitlab_list_projects\n"},
	}}
	result := make(chan int)

	got := enrichWithHints(result, callResult)

	gotChan, ok := got.(chan int)
	if !ok || gotChan != result {
		t.Errorf("enrichWithHints(chan) = %v, want original channel", got)
	}
}

// aliasStructInput is a small struct with JSON-tagged fields used to drive
// reflect-based helpers that introspect a target type's fields.
type aliasStructInput struct {
	ProjectID StringOrInt `json:"project_id"`
	MRIID     int64       `json:"merge_request_iid"`
	Name      string      `json:"name"`
	Paused    bool        `json:"paused"`
}

// accessLevelEntryInput mirrors the GitLab protected-branch access level
// shape so structured-value coercion has a struct target to inspect.
type accessLevelEntryInput struct {
	AccessLevel       *int   `json:"access_level"`
	RequiredApprovals *int64 `json:"required_approvals"`
}

// TestStructHasJSONField verifies structHasJSONField returns true for
// fields with JSON tags, false for unexported or absent fields, and
// handles nil, pointer, and non-struct target types defensively.
func TestStructHasJSONField(t *testing.T) {
	target := reflect.TypeFor[aliasStructInput]()

	if !structHasJSONField(target, "project_id") {
		t.Error("structHasJSONField(project_id) = false, want true")
	}
	if !structHasJSONField(target, "merge_request_iid") {
		t.Error("structHasJSONField(merge_request_iid) = false, want true")
	}
	if structHasJSONField(target, "missing") {
		t.Error("structHasJSONField(missing) = true, want false")
	}
	if structHasJSONField(target, "MRIID") {
		t.Error("structHasJSONField(MRIID, no json tag) = true, want false")
	}
	if structHasJSONField(reflect.TypeFor[int](), "x") {
		t.Error("structHasJSONField(non-struct) = true, want false")
	}
	if structHasJSONField(nil, "x") {
		t.Error("structHasJSONField(nil) = true, want false")
	}
}

// TestValidGitLabRoleAccessLevelDirect verifies the access-level whitelist
// accepts the canonical 0/10/20/30/40/50/60 values and rejects every other
// integer, including negative and out-of-range values.
func TestValidGitLabRoleAccessLevelDirect(t *testing.T) {
	cases := []struct {
		value  int
		wantOK bool
	}{
		{0, true},
		{10, true},
		{20, true},
		{30, true},
		{40, true},
		{50, true},
		{60, true},
		{1, false},
		{5, false},
		{15, false},
		{25, false},
		{70, false},
		{100, false},
		{-1, false},
		{-10, false},
	}
	for _, tc := range cases {
		got, ok := validGitLabRoleAccessLevel(tc.value)
		if ok != tc.wantOK {
			t.Errorf("validGitLabRoleAccessLevel(%d) ok = %v, want %v", tc.value, ok, tc.wantOK)
		}
		if ok && got != tc.value {
			t.Errorf("validGitLabRoleAccessLevel(%d) = %d, want %d", tc.value, got, tc.value)
		}
	}
}

// TestValidGitLabRoleAccessLevelInt64Direct verifies the int64 variant
// rejects out-of-int-range and negative values before narrowing.
func TestValidGitLabRoleAccessLevelInt64Direct(t *testing.T) {
	for _, value := range []int64{0, 10, 20, 30, 40, 50, 60} {
		if got, ok := validGitLabRoleAccessLevelInt64(value); !ok || got != int(value) {
			t.Errorf("validGitLabRoleAccessLevelInt64(%d) = %d/%v, want %d/true", value, got, ok, value)
		}
	}
	for _, value := range []int64{-1, 5, 70, 1 << 40, 1 << 60} {
		if _, ok := validGitLabRoleAccessLevelInt64(value); ok {
			t.Errorf("validGitLabRoleAccessLevelInt64(%d) ok = true, want false", value)
		}
	}
}

// TestValidGitLabRoleAccessLevelFloat64Direct verifies the float64 variant
// only accepts whole-number values in the canonical set.
func TestValidGitLabRoleAccessLevelFloat64Direct(t *testing.T) {
	for _, value := range []float64{0, 10, 20, 30, 40, 50, 60} {
		if got, ok := validGitLabRoleAccessLevelFloat64(value); !ok || got != int(value) {
			t.Errorf("validGitLabRoleAccessLevelFloat64(%v) = %d/%v, want %d/true", value, got, ok, int(value))
		}
	}
	for _, value := range []float64{1.5, 5, 70, -10} {
		if _, ok := validGitLabRoleAccessLevelFloat64(value); ok {
			t.Errorf("validGitLabRoleAccessLevelFloat64(%v) ok = true, want false", value)
		}
	}
}

// TestNumericRoleAccessLevel verifies the type-switch dispatch accepts
// int, int64, and float64 numeric types and rejects everything else.
//
// The three numeric arms are not equivalent: int64 and float64 delegate to
// validGitLabRoleAccessLevel{Int64,Float64}, so the canonical access-level
// whitelist is enforced here for those two, while a plain int is narrowed and
// returned unchecked. The out-of-range assertions below cover only the
// validating arms for that reason.
func TestNumericRoleAccessLevel(t *testing.T) {
	if got, ok := numericRoleAccessLevel(int(30)); !ok || got != 30 {
		t.Errorf("numericRoleAccessLevel(int 30) = %d/%v, want 30/true", got, ok)
	}
	if got, ok := numericRoleAccessLevel(int64(40)); !ok || got != 40 {
		t.Errorf("numericRoleAccessLevel(int64 40) = %d/%v, want 40/true", got, ok)
	}
	if got, ok := numericRoleAccessLevel(float64(50)); !ok || got != 50 {
		t.Errorf("numericRoleAccessLevel(float64 50) = %d/%v, want 50/true", got, ok)
	}
	// int64 and float64 variants enforce the canonical access-level
	// whitelist, so out-of-range values are rejected at this layer.
	if _, ok := numericRoleAccessLevel(int64(70)); ok {
		t.Error("numericRoleAccessLevel(int64 70) ok = true, want false")
	}
	if _, ok := numericRoleAccessLevel(float64(70)); ok {
		t.Error("numericRoleAccessLevel(float64 70) ok = true, want false")
	}
	for _, value := range []any{"30", true, nil} {
		if _, ok := numericRoleAccessLevel(value); ok {
			t.Errorf("numericRoleAccessLevel(%T %v) ok = true, want false", value, value)
		}
	}
}

// TestAppendNormalizedRouteStringsDirect verifies the route-string
// helper dedupes, lowercases, trims whitespace, drops empty entries,
// and returns nil when nothing remains.
func TestAppendNormalizedRouteStringsDirect(t *testing.T) {
	got := appendNormalizedRouteStrings([]string{"  LIST ", "list"}, "GET", "post", " Post ")
	want := []string{"list", "get", "post"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}

	if isNil := appendNormalizedRouteStrings(nil); isNil != nil {
		t.Errorf("appendNormalizedRouteStrings(no values) = %v, want nil", isNil)
	}
	if isNil := appendNormalizedRouteStrings([]string{" "}); isNil != nil {
		t.Errorf("appendNormalizedRouteStrings(only whitespace) = %v, want nil", isNil)
	}
	if isNil := appendNormalizedRouteStrings([]string{}, "  ", "  "); isNil != nil {
		t.Errorf("appendNormalizedRouteStrings(only blank values) = %v, want nil", isNil)
	}
}

// TestCoercePaginationBooleanDirect verifies the boolean→page-size
// coercion handles page=1, others=100, non-bool, and unsupported
// parameter names.
func TestCoercePaginationBooleanDirect(t *testing.T) {
	intType := reflect.TypeFor[int]()

	// name == "page" → 1
	got, ok := coercePaginationBoolean("page", true, intType)
	if !ok || got != 1 {
		t.Errorf("coercePaginationBoolean(page, true) = %v/%v, want 1/true", got, ok)
	}

	// name in {first,last,per_page} → 100
	for _, name := range []string{"first", "last", "per_page"} {
		got, ok = coercePaginationBoolean(name, true, intType)
		if !ok || got != 100 {
			t.Errorf("coercePaginationBoolean(%s, true) = %v/%v, want 100/true", name, got, ok)
		}
	}

	// bool false → unchanged
	got, ok = coercePaginationBoolean("page", false, intType)
	if ok || got != false {
		t.Errorf("coercePaginationBoolean(page, false) = %v/%v, want false/false", got, ok)
	}

	// non-bool value → unchanged
	got, ok = coercePaginationBoolean("page", 5, intType)
	if ok || got != 5 {
		t.Errorf("coercePaginationBoolean(page, 5) = %v/%v, want 5/false", got, ok)
	}

	// unsupported param name → unchanged
	got, ok = coercePaginationBoolean("name", true, intType)
	if ok || got != true {
		t.Errorf("coercePaginationBoolean(name, true) = %v/%v, want true/false", got, ok)
	}

	// non-numeric target kind → unchanged
	stringType := reflect.TypeFor[string]()
	got, ok = coercePaginationBoolean("page", true, stringType)
	if ok || got != true {
		t.Errorf("coercePaginationBoolean(page, true, string) = %v/%v, want true/false", got, ok)
	}
}

// TestNormalizeAccessLevelScalarDirect verifies the access-level scalar
// normalizer coerces string role names to int, leaves unsupported names
// and non-access-level params untouched, and respects the target kind.
func TestNormalizeAccessLevelScalarDirect(t *testing.T) {
	intType := reflect.TypeFor[int]()
	stringType := reflect.TypeFor[string]()

	got, ok := normalizeAccessLevelScalar("push_access_level", "maintainer", intType)
	if !ok || got != 40 {
		t.Errorf("normalizeAccessLevelScalar(maintainer) = %v/%v, want 40/true", got, ok)
	}

	// name does not look like an access level → unchanged
	got, ok = normalizeAccessLevelScalar("title", "maintainer", intType)
	if ok || got != "maintainer" {
		t.Errorf("normalizeAccessLevelScalar(title) = %v/%v, want maintainer/false", got, ok)
	}

	// target kind is not numeric → unchanged
	got, ok = normalizeAccessLevelScalar("push_access_level", "maintainer", stringType)
	if ok || got != "maintainer" {
		t.Errorf("normalizeAccessLevelScalar(string target) = %v/%v, want maintainer/false", got, ok)
	}

	// pointer to int → still works
	got, ok = normalizeAccessLevelScalar("access_level", "owner", reflect.TypeFor[*int]())
	if !ok || got != 50 {
		t.Errorf("normalizeAccessLevelScalar(*int) = %v/%v, want 50/true", got, ok)
	}

	// unknown role name → unchanged
	got, ok = normalizeAccessLevelScalar("access_level", "wizard", intType)
	if ok || got != "wizard" {
		t.Errorf("normalizeAccessLevelScalar(wizard) = %v/%v, want wizard/false", got, ok)
	}
}

// TestIsStringSliceTypeAndIsStringTypeDirect verify the pointer-aware
// reflection helpers across a representative set of Go types.
func TestIsStringSliceTypeAndIsStringTypeDirect(t *testing.T) {
	cases := []struct {
		name     string
		typ      reflect.Type
		isString bool
		isSlice  bool
	}{
		{"string", reflect.TypeFor[string](), true, false},
		{"*string", reflect.TypeFor[*string](), true, false},
		{"[]string", reflect.TypeFor[[]string](), false, true},
		{"[]int", reflect.TypeFor[[]int](), false, false},
		{"int", reflect.TypeFor[int](), false, false},
		{"bool", reflect.TypeFor[bool](), false, false},
	}
	for _, tc := range cases {
		if got := isStringType(tc.typ); got != tc.isString {
			t.Errorf("%s: isStringType = %v, want %v", tc.name, got, tc.isString)
		}
		if got := isStringSliceType(tc.typ); got != tc.isSlice {
			t.Errorf("%s: isStringSliceType = %v, want %v", tc.name, got, tc.isSlice)
		}
	}
}

// TestCoerceStructuredValue_MapWithAccessLevelKey verifies the helper
// wraps a map containing the access_level key into a single-element slice.
func TestCoerceStructuredValue_MapWithAccessLevelKey(t *testing.T) {
	sliceType := reflect.TypeFor[[]accessLevelEntryInput]()
	value, ok := coerceStructuredValue("access_level", map[string]any{"access_level": 30}, sliceType)
	if !ok {
		t.Fatal("coerceStructuredValue(map) = false, want true")
	}
	gotSlice, isSlice := value.([]any)
	if !isSlice || len(gotSlice) != 1 {
		t.Fatalf("value = %v (type %T), want []any with 1 entry", value, value)
	}
}

// TestCoerceStructuredValue_MapWithoutAccessLevelKeyIsStillWrapped verifies
// the helper wraps any map input regardless of the keys it carries.
func TestCoerceStructuredValue_MapWithoutAccessLevelKeyIsStillWrapped(t *testing.T) {
	sliceType := reflect.TypeFor[[]accessLevelEntryInput]()
	value, ok := coerceStructuredValue("access_level", map[string]any{"name": "x"}, sliceType)
	if !ok {
		t.Fatal("coerceStructuredValue(map) = false, want true (map always wrapped)")
	}
	wrapped2, isSlice := value.([]any)
	if !isSlice || len(wrapped2) != 1 {
		t.Errorf("value = %v, want []any with 1 entry", value)
	}
}

// TestCoerceStructuredValue_ScalarAccessLevelString verifies a scalar role
// string is wrapped into a single-element slice keyed by access_level.
func TestCoerceStructuredValue_ScalarAccessLevelString(t *testing.T) {
	sliceType := reflect.TypeFor[[]accessLevelEntryInput]()
	value, ok := coerceStructuredValue("access_level", "maintainer", sliceType)
	if !ok {
		t.Fatal("coerceStructuredValue(maintainer) = false, want true")
	}
	wrapped, isSlice := value.([]any)
	if !isSlice || len(wrapped) != 1 {
		t.Fatalf("value = %v (type %T), want []any with 1 entry", value, value)
	}
	m, isMap := wrapped[0].(map[string]any)
	if !isMap || m["access_level"] != 40 {
		t.Errorf("wrapped[0] = %v, want {access_level: 40}", wrapped[0])
	}
}

// TestCoerceStructuredValue_SliceWithMixedRoleStringsAndMaps verifies a
// []any input containing role strings and a map is coerced element-wise.
func TestCoerceStructuredValue_SliceWithMixedRoleStringsAndMaps(t *testing.T) {
	sliceType := reflect.TypeFor[[]accessLevelEntryInput]()
	items := []any{"developer", map[string]any{"access_level": 60}}
	value, ok := coerceStructuredValue("access_level", items, sliceType)
	if !ok {
		t.Fatal("coerceStructuredValue([]any) = false, want true")
	}
	updated, isSlice := value.([]any)
	if !isSlice || len(updated) != 2 {
		t.Fatalf("value = %v, want []any with 2 entries", value)
	}
	first, isMap := updated[0].(map[string]any)
	if !isMap || first["access_level"] != 30 {
		t.Errorf("updated[0] = %v, want {access_level: 30}", updated[0])
	}
	if !reflect.DeepEqual(updated[1], items[1]) {
		t.Errorf("updated[1] = %v, want unchanged %v", updated[1], items[1])
	}
}

// TestCoerceStructuredValue_NonSliceTargetFallsThrough verifies the helper
// returns the input unchanged when the target type is not a slice.
func TestCoerceStructuredValue_NonSliceTargetFallsThrough(t *testing.T) {
	intType := reflect.TypeFor[int]()
	value, ok := coerceStructuredValue("title", "hello", intType)
	if ok || value != "hello" {
		t.Errorf("coerceStructuredValue(title) = %v/%v, want hello/false", value, ok)
	}
}

// TestCoerceStructuredValue_SliceWithNonStructElemUnchanged verifies the
// helper leaves the input unchanged when the slice element is not a struct.
func TestCoerceStructuredValue_SliceWithNonStructElemUnchanged(t *testing.T) {
	stringSliceType := reflect.TypeFor[[]string]()
	value, ok := coerceStructuredValue("access_level", "maintainer", stringSliceType)
	if ok {
		t.Errorf("coerceStructuredValue(non-struct slice) = %v/true, want false", value)
	}
}

// TestCoerceSingleStringSlicesDirect verifies the string→[]string
// promotion helper returns the input unchanged when the target type
// has no fields, when a value is not a string, and when the field is
// not a string slice type.
func TestCoerceSingleStringSlicesDirect(t *testing.T) {
	intType := reflect.TypeFor[int]()
	sliceStructType := reflect.TypeOf(struct {
		Labels []string `json:"labels"`
	}{})

	// non-struct target → unchanged
	params := map[string]any{"labels": "bug"}
	if got := coerceSingleStringSlices(params, intType); !reflect.DeepEqual(got, params) {
		t.Errorf("coerceSingleStringSlices(non-struct) = %+v, want %+v", got, params)
	}

	// value is not a string → unchanged
	params = map[string]any{"labels": 42}
	if got := coerceSingleStringSlices(params, sliceStructType); !reflect.DeepEqual(got, params) {
		t.Errorf("coerceSingleStringSlices(non-string value) = %+v, want %+v", got, params)
	}

	// happy path → wrapped as single-element slice
	params = map[string]any{"labels": "bug"}
	got := coerceSingleStringSlices(params, sliceStructType)
	if list, ok := got["labels"].([]string); !ok || len(list) != 1 || list[0] != "bug" {
		t.Errorf("labels = %v, want [bug]", got["labels"])
	}
}

// TestCoerceStringListParamsDirect verifies the comma-separated
// string helper for the labels-style field family.
func TestCoerceStringListParamsDirect(t *testing.T) {
	intType := reflect.TypeFor[int]()

	// non-struct target → unchanged
	params := map[string]any{"labels": "bug,feature"}
	if got := coerceStringListParams(params, intType); !reflect.DeepEqual(got, params) {
		t.Errorf("coerceStringListParams(non-struct) = %+v, want %+v", got, params)
	}

	// happy path → joined as single CSV string
	type labelsInput struct {
		Labels string `json:"labels"`
	}
	labelsType := reflect.TypeFor[labelsInput]()
	params = map[string]any{"labels": []string{"bug", "feature"}}
	got := coerceStringListParams(params, labelsType)
	if got["labels"] != "bug,feature" {
		t.Errorf("labels = %v, want bug,feature", got["labels"])
	}
}

// TestCoerceStringIDNumbersDirect verifies the numeric→string coercion
// for the id/_id/_iid parameter family.
func TestCoerceStringIDNumbersDirect(t *testing.T) {
	intType := reflect.TypeFor[int]()

	// non-struct target → unchanged
	params := map[string]any{"id": 42}
	if got := coerceStringIDNumbers(params, intType); !reflect.DeepEqual(got, params) {
		t.Errorf("coerceStringIDNumbers(non-struct) = %+v, want %+v", got, params)
	}

	// happy path → string conversion
	type idInput struct {
		ID int `json:"id"`
	}
	idType := reflect.TypeFor[idInput]()
	_ = idType
	// the target must be a string field for the coercion to apply
	type stringIDInput struct {
		ID string `json:"id"`
	}
	stringIDType := reflect.TypeFor[stringIDInput]()
	params = map[string]any{"id": 42}
	got := coerceStringIDNumbers(params, stringIDType)
	if got["id"] != "42" {
		t.Errorf("id = %v, want 42", got["id"])
	}
}

// TestCoerceNumericParamsDirect verifies the numeric coercion for the
// standard integer/float parameter shapes, including the empty-fields
// early return.
func TestCoerceNumericParamsDirect(t *testing.T) {
	intType := reflect.TypeFor[int]()

	// non-struct target → unchanged
	params := map[string]any{"count": "5"}
	got, err := coerceNumericParams(params, intType)
	if err != nil {
		t.Errorf("coerceNumericParams(non-struct) error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, params) {
		t.Errorf("coerceNumericParams(non-struct) = %+v, want %+v", got, params)
	}
}

// TestCoerceUnsignedIntegerValueDirect verifies the unsigned-integer
// coercion helper returns the input unchanged for non-string values.
func TestCoerceUnsignedIntegerValueDirect(t *testing.T) {
	// non-string value → unchanged
	value, changed, err := coerceUnsignedIntegerValue("count", 42)
	if err != nil || changed || value != 42 {
		t.Errorf("coerceUnsignedIntegerValue(int) = %v/%v/%v, want 42/false/nil", value, changed, err)
	}
}

// TestCoerceFloatValueDirect verifies the float coercion helper
// returns the input unchanged for non-string values.
func TestCoerceFloatValueDirect(t *testing.T) {
	// non-string value → unchanged
	value, changed, err := coerceFloatValue("weight", 3.14)
	if err != nil || changed || value != 3.14 {
		t.Errorf("coerceFloatValue(float) = %v/%v/%v, want 3.14/false/nil", value, changed, err)
	}
}

// TestCoerceSliceValueForTargetTypeDirect verifies the slice element
// coercion helper, including the non-numeric-element early return.
func TestCoerceSliceValueForTargetTypeDirect(t *testing.T) {
	stringType := reflect.TypeFor[string]()

	// non-numeric elem → unchanged
	value, changed, err := coerceSliceValueForTargetType("ids", []string{"a", "b"}, stringType)
	if err != nil || changed {
		t.Errorf("coerceSliceValueForTargetType(string elem) = %v/%v/%v, want no-op", value, changed, err)
	}
}

// TestCoerceSchemaParamTypesDirect verifies the schema-aware coercion
// returns the input unchanged for schemas without a "properties" map.
func TestCoerceSchemaParamTypesDirect(t *testing.T) {
	params := map[string]any{"id": 1}
	if got := coerceSchemaParamTypes(params, map[string]any{}); !reflect.DeepEqual(got, params) {
		t.Errorf("coerceSchemaParamTypes(no props) = %+v, want %+v", got, params)
	}
}

// TestCoerceSchemaArrayValueDirect verifies the array-value coercion
// returns the input unchanged for properties that are not "array" typed.
func TestCoerceSchemaArrayValueDirect(t *testing.T) {
	params := []string{"1", "2"}
	got, changed := coerceSchemaArrayValue(params, map[string]any{"type": "string"})
	if changed || !reflect.DeepEqual(got, params) {
		t.Errorf("coerceSchemaArrayValue(non-array) = %v/%v, want %+v/false", got, changed, params)
	}
}

// TestStringListToCSVDirect verifies the string-list→CSV helper
// rejects unsupported input types and joins homogeneous string slices.
func TestStringListToCSVDirect(t *testing.T) {
	// non-slice input → empty
	if csv, ok := stringListToCSV("not-a-list"); ok || csv != "" {
		t.Errorf("stringListToCSV(string) = %q/%v, want empty/false", csv, ok)
	}
	// []any with non-string entry → empty
	if csv, ok := stringListToCSV([]any{"a", 2}); ok || csv != "" {
		t.Errorf("stringListToCSV(mixed) = %q/%v, want empty/false", csv, ok)
	}
	// []any with all strings → joined CSV
	if csv, ok := stringListToCSV([]any{"a", "b"}); !ok || csv != "a,b" {
		t.Errorf("stringListToCSV([]any) = %q/%v, want a,b/true", csv, ok)
	}
	// []string → joined CSV (concrete slice type)
	if csv, ok := stringListToCSV([]string{"a", "b"}); !ok || csv != "a,b" {
		t.Errorf("stringListToCSV([]string) = %q/%v, want a,b/true", csv, ok)
	}
}

// TestCoerceSingleStringArraysForSchemaDirect verifies the
// schema-aware single-string→[]string coercion falls through for
// non-array properties.
func TestCoerceSingleStringArraysForSchemaDirect(t *testing.T) {
	params := map[string]any{"name": "alice"}
	schema := map[string]any{
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}
	if got := coerceSingleStringArraysForSchema(params, schema); !reflect.DeepEqual(got, params) {
		t.Errorf("coerceSingleStringArraysForSchema(non-array) = %+v, want %+v", got, params)
	}
}

// TestValidateMetaToolParamsDirect verifies the meta-tool input
// validator returns the "params is required" error for nil params
// with a non-empty required-params list.
func TestValidateMetaToolParamsDirect(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
		},
		"required": []any{"project_id"},
	}
	route := ActionRoute{InputSchema: schema}
	input := &MetaToolInput{Action: "get_project", Params: nil}

	result := validateMetaToolParams("tool", route, input)
	if result == nil {
		t.Fatal("validateMetaToolParams(nil params, required present) = nil, want error result")
	}
	if !result.IsError {
		t.Errorf("result.IsError = false, want true")
	}
}

// TestValidateMetaToolParamsMissingRequiredDirect covers the branch that
// fires when Params is provided but is missing required fields and does not
// contain unknown names (so the unknown-name short-circuit does not apply).
func TestValidateMetaToolParamsMissingRequiredDirect(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
		},
		"required": []any{"project_id"},
	}
	route := ActionRoute{InputSchema: schema}
	input := &MetaToolInput{Action: "get_project", Params: map[string]any{}}

	result := validateMetaToolParams("tool", route, input)
	if result == nil {
		t.Fatal("validateMetaToolParams(missing required) = nil, want error result")
	}
	if !result.IsError {
		t.Errorf("result.IsError = false, want true")
	}
}

// TestMetaToolParameterGuidanceSummaryEmptyItem verifies the
// parameter-guidance renderer skips entries that carry no semantic
// role, value source, or common confusions — even when the parent
// action has a non-empty ParameterGuidance map.
func TestMetaToolParameterGuidanceSummaryEmptyItem(t *testing.T) {
	routes := ActionMap{
		"do_thing": {
			ParameterGuidance: map[string]ParameterGuidance{
				"empty_param": {},
				"described":   {SemanticRole: "scope"},
			},
		},
	}
	got := metaToolParameterGuidanceSummary(routes, []string{"do_thing"})
	if !strings.Contains(got, "do_thing.described") {
		t.Errorf("summary = %q, want to contain 'do_thing.described'", got)
	}
	if strings.Contains(got, "do_thing.empty_param") {
		t.Errorf("summary = %q, must not contain 'do_thing.empty_param'", got)
	}
}

// TestEnrichWithHints_NonObjectJSONResult verifies the JSON-result
// path that requires the marshaled result to start with '{'. Result
// values that marshal to non-object JSON (e.g. arrays) are returned
// unchanged without crashing.
func TestEnrichWithHints_NonObjectJSONResult(t *testing.T) {
	callResult := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "<!-- HINTS: [\"step-1\"] -->\nSome text."},
		},
	}
	result := []string{"a", "b"}
	got := enrichWithHints(result, callResult)
	if _, ok := got.([]string); !ok {
		t.Errorf("enrichWithHints(non-object) = %T, want original []string", got)
	}
}
