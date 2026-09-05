package toolutil

import (
	"reflect"
	"testing"
)

func testMetaSchemaRoutes() map[string]ActionMap {
	return map[string]ActionMap{
		"gitlab_issue": {
			"delete": {Destructive: true, InputSchema: map[string]any{"type": "object"}},
			"get":    {InputSchema: map[string]any{"type": "object"}},
		},
		"gitlab_project": {
			"archive": {Destructive: true, InputSchema: map[string]any{"type": "object"}},
			"list":    {InputSchema: map[string]any{"type": "object"}},
		},
	}
}

// TestMetaSchemaRegistry_ClonesRoutes verifies registry snapshots are isolated
// from later caller mutations and that returned route maps cannot mutate the
// stored snapshot.
func TestMetaSchemaRegistry_ClonesRoutes(t *testing.T) {
	routes := testMetaSchemaRoutes()
	registry := NewMetaSchemaRegistry(routes)
	routes["gitlab_issue"]["create"] = ActionRoute{}

	snapshot := registry.Routes()
	if _, ok := snapshot["gitlab_issue"]["create"]; ok {
		t.Fatal("registry observed mutation made after SetRoutes")
	}
	snapshot["gitlab_issue"]["update"] = ActionRoute{}

	secondSnapshot := registry.Routes()
	if _, ok := secondSnapshot["gitlab_issue"]["update"]; ok {
		t.Fatal("registry was mutated through Routes result")
	}
	if (*MetaSchemaRegistry)(nil).Routes() != nil {
		t.Fatal("nil registry Routes() returned non-nil map")
	}
}

// TestMetaSchemaRegistry_NilSetRoutesIsNoop verifies a nil registry receiver
// tolerates SetRoutes calls so optional registries can skip setup safely.
func TestMetaSchemaRegistry_NilSetRoutesIsNoop(t *testing.T) {
	var registry *MetaSchemaRegistry
	registry.SetRoutes(testMetaSchemaRoutes())
}

// TestCloneMetaSchemaRoutes_SharesSchemasAndOwnsSlices verifies the snapshot
// contract: the two map levels are new and the string slices are copies, so
// insertions and appends do not reach the original, while the schema and
// guidance maps are the very same objects, since they are frozen and shared
// by every server in the process.
func TestCloneMetaSchemaRoutes_SharesSchemasAndOwnsSlices(t *testing.T) {
	routes := map[string]ActionMap{
		"gitlab_project": {
			"create": {
				Aliases:           []string{"project.create"},
				Tags:              []string{"project"},
				RelatedActions:    []string{"project.get"},
				InputSchema:       map[string]any{"type": "object"},
				OutputSchema:      map[string]any{"type": "object"},
				ParameterGuidance: map[string]ParameterGuidance{"project_id": {SemanticRole: "scope_project"}},
			},
		},
	}

	clone := CloneMetaSchemaRoutes(routes)
	cloneRoute := clone["gitlab_project"]["create"]
	original := routes["gitlab_project"]["create"]

	if !sameMap(cloneRoute.InputSchema, original.InputSchema) || !sameMap(cloneRoute.OutputSchema, original.OutputSchema) {
		t.Fatal("CloneMetaSchemaRoutes() copied the schema maps, want them shared")
	}
	if reflect.ValueOf(cloneRoute.ParameterGuidance).UnsafePointer() != reflect.ValueOf(original.ParameterGuidance).UnsafePointer() {
		t.Fatal("CloneMetaSchemaRoutes() copied the guidance map, want it shared")
	}

	cloneRoute.Aliases[0] = "changed"
	cloneRoute.Tags[0] = "changed"
	cloneRoute.RelatedActions[0] = "changed"
	if original.Aliases[0] != "project.create" || original.Tags[0] != "project" || original.RelatedActions[0] != "project.get" {
		t.Fatalf("original route slices = %+v, want unchanged by edits to the snapshot", original)
	}

	delete(clone["gitlab_project"], "create")
	delete(clone, "gitlab_project")
	if _, ok := routes["gitlab_project"]["create"]; !ok {
		t.Fatal("deleting from the snapshot reached the original maps")
	}
}

// sameMap reports whether two schema maps are one object.
func sameMap(left, right map[string]any) bool {
	return reflect.ValueOf(left).UnsafePointer() == reflect.ValueOf(right).UnsafePointer()
}

// TestBuildMetaSchemaIndex_SortsToolsAndActions verifies the resource index
// has deterministic tool and action ordering.
func TestBuildMetaSchemaIndex_SortsToolsAndActions(t *testing.T) {
	index := BuildMetaSchemaIndex(testMetaSchemaRoutes())
	if index.URITemplate != MetaSchemaTemplateURI {
		t.Fatalf("URITemplate = %q, want %q", index.URITemplate, MetaSchemaTemplateURI)
	}
	if len(index.Tools) != 2 {
		t.Fatalf("Tools len = %d, want 2", len(index.Tools))
	}
	if index.Tools[0].Tool != "gitlab_issue" || index.Tools[1].Tool != "gitlab_project" {
		t.Fatalf("tools not sorted: %#v", index.Tools)
	}
	wantActions := []string{"delete", "get"}
	for i, want := range wantActions {
		t.Run(want, func(t *testing.T) {
			if index.Tools[0].Actions[i] != want {
				t.Fatalf("issue action %d = %q, want %q", i, index.Tools[0].Actions[i], want)
			}
		})
	}
}

// TestBuildMetaSchemaDiscoveryIndex_IncludesSchemaURIsAndDestructiveFlags
// verifies the richer tool-call index includes counts, stable URIs, and
// destructive metadata.
func TestBuildMetaSchemaDiscoveryIndex_IncludesSchemaURIsAndDestructiveFlags(t *testing.T) {
	index := BuildMetaSchemaDiscoveryIndex(testMetaSchemaRoutes())
	if index.ToolCount != 2 || index.ActionCount != 4 {
		t.Fatalf("counts = tools %d actions %d, want 2/4", index.ToolCount, index.ActionCount)
	}
	issue := index.Tools[0]
	if issue.Tool != "gitlab_issue" || issue.ActionCount != 2 {
		t.Fatalf("first tool = %#v, want gitlab_issue with 2 actions", issue)
	}
	deleteAction := issue.Actions[0]
	if deleteAction.Action != "delete" || !deleteAction.Destructive {
		t.Fatalf("delete action metadata = %#v, want destructive delete", deleteAction)
	}
	if deleteAction.SchemaURI != MetaSchemaURI("gitlab_issue", "delete") {
		t.Fatalf("SchemaURI = %q", deleteAction.SchemaURI)
	}
}

// TestBuildMetaSchemaDiscoveryIndexForTool_KnownTool_ReturnsSingleToolIndex verifies single-tool discovery and
// the false result for unknown tools.
func TestBuildMetaSchemaDiscoveryIndexForTool_KnownTool_ReturnsSingleToolIndex(t *testing.T) {
	index, ok := BuildMetaSchemaDiscoveryIndexForTool(testMetaSchemaRoutes(), "gitlab_project")
	if !ok {
		t.Fatal("BuildMetaSchemaDiscoveryIndexForTool() ok = false, want true")
	}
	if index.ToolCount != 1 || index.ActionCount != 2 || index.Tools[0].Tool != "gitlab_project" {
		t.Fatalf("single-tool index = %#v", index)
	}
	if _, missingOK := BuildMetaSchemaDiscoveryIndexForTool(testMetaSchemaRoutes(), "gitlab_missing"); missingOK {
		t.Fatal("unknown tool ok = true, want false")
	}
}

// TestLookupMetaActionSchema_DestructiveActionAddsConfirm verifies that
// destructive action schemas are copied and augmented with confirmation
// metadata without mutating the registered route schema.
//
// The test builds an in-memory route map for milestone_delete, looks up its
// schema, and asserts that the returned schema includes confirm and
// x_destructive while the original InputSchema remains unchanged.
func TestLookupMetaActionSchema_DestructiveActionAddsConfirm(t *testing.T) {
	routes := map[string]ActionMap{
		"gitlab_project": {
			"milestone_delete": {
				Destructive: true,
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_id": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	schema, ok := LookupMetaActionSchema(routes, "gitlab_project", "milestone_delete")
	if !ok {
		t.Fatal("LookupMetaActionSchema() ok = false, want true")
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties missing: %#v", schema)
	}
	if _, hasConfirm := properties["confirm"]; !hasConfirm {
		t.Fatalf("confirm property missing: %#v", properties)
	}
	if schema["x_destructive"] != true {
		t.Fatalf("x_destructive = %#v, want true", schema["x_destructive"])
	}
	originalProperties := routes["gitlab_project"]["milestone_delete"].InputSchema["properties"].(map[string]any)
	if _, originalHasConfirm := originalProperties["confirm"]; originalHasConfirm {
		t.Fatalf("original schema was mutated: %#v", originalProperties)
	}
}

// TestLookupMetaActionSchema_DeepClonesSliceFields verifies callers cannot
// mutate slice-valued schema fields returned from the route registry.
func TestLookupMetaActionSchema_DeepClonesSliceFields(t *testing.T) {
	routes := map[string]ActionMap{
		"gitlab_project": {
			"create": {
				InputSchema: map[string]any{
					"type":     "object",
					"required": []string{"project_id"},
					"properties": map[string]any{
						"visibility": map[string]any{"enum": []any{"private", "public"}},
					},
					"oneOf": []any{map[string]any{"required": []string{"name"}}},
				},
			},
		},
	}

	schema, ok := LookupMetaActionSchema(routes, "gitlab_project", "create")
	if !ok {
		t.Fatal("LookupMetaActionSchema() ok = false, want true")
	}
	schema["required"].([]string)[0] = "changed"
	properties := schema["properties"].(map[string]any)
	visibility := properties["visibility"].(map[string]any)
	visibility["enum"].([]any)[0] = "internal"
	schema["oneOf"].([]any)[0].(map[string]any)["required"].([]string)[0] = "path"

	original := routes["gitlab_project"]["create"].InputSchema
	if got := original["required"].([]string)[0]; got != "project_id" {
		t.Fatalf("original required[0] = %q, want project_id", got)
	}
	originalProperties := original["properties"].(map[string]any)
	originalVisibility := originalProperties["visibility"].(map[string]any)
	if got := originalVisibility["enum"].([]any)[0]; got != "private" {
		t.Fatalf("original enum[0] = %q, want private", got)
	}
	originalOneOf := original["oneOf"].([]any)[0].(map[string]any)
	if got := originalOneOf["required"].([]string)[0]; got != "name" {
		t.Fatalf("original oneOf required[0] = %q, want name", got)
	}
}

// TestLookupMetaActionSchema_IncludesParameterGuidance verifies guidance is
// exposed as schema extension metadata without mutating registered routes.
func TestLookupMetaActionSchema_IncludesParameterGuidance(t *testing.T) {
	routes := map[string]ActionMap{
		"gitlab_job": {
			"token_scope_remove_project": {
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_id":        map[string]any{"type": "integer"},
						"target_project_id": map[string]any{"type": "integer"},
					},
				},
				ParameterGuidance: map[string]ParameterGuidance{
					"project_id": {
						SemanticRole:     "scope_owner_project",
						ValueSource:      "Owning project whose allowlist is being changed.",
						CommonConfusions: []string{"Do not use the project being removed as project_id."},
						ExampleBinding:   "Remove project 51 from project 1 => project_id=1.",
					},
				},
			},
		},
	}

	schema, ok := LookupMetaActionSchema(routes, "gitlab_job", "token_scope_remove_project")
	if !ok {
		t.Fatal("LookupMetaActionSchema() ok = false, want true")
	}
	extension, ok := schema["x_parameter_guidance"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing x_parameter_guidance: %#v", schema)
	}
	projectGuidance, ok := extension["project_id"].(map[string]any)
	if !ok {
		t.Fatalf("x_parameter_guidance[project_id] missing or invalid: %#v", extension["project_id"])
	}
	if projectGuidance["semantic_role"] != "scope_owner_project" {
		t.Fatalf("project_id guidance = %#v, want scope_owner_project", projectGuidance)
	}
	projectGuidance["semantic_role"] = "changed"
	if got := routes["gitlab_job"]["token_scope_remove_project"].ParameterGuidance["project_id"].SemanticRole; got != "scope_owner_project" {
		t.Fatalf("original guidance semantic role = %q, want unchanged", got)
	}
}

// TestLookupMetaActionSchema_NilInputSchemaReturnsFallback verifies routes
// without captured input schema still expose an actionable object schema and
// unknown tool/action pairs return false.
func TestLookupMetaActionSchema_NilInputSchemaReturnsFallback(t *testing.T) {
	routes := map[string]ActionMap{
		"gitlab_project": {
			"empty": {Destructive: true},
		},
	}
	schema, ok := LookupMetaActionSchema(routes, "gitlab_project", "empty")
	if !ok {
		t.Fatal("LookupMetaActionSchema() ok = false, want true")
	}
	if schema["type"] != "object" || schema["additionalProperties"] != true || schema["x_destructive"] != true {
		t.Fatalf("fallback schema = %#v", schema)
	}
	if _, hasConfirm := schema["properties"].(map[string]any)["confirm"]; !hasConfirm {
		t.Fatalf("fallback destructive schema missing confirm: %#v", schema)
	}
	if _, missingActionOK := LookupMetaActionSchema(routes, "gitlab_project", "missing"); missingActionOK {
		t.Fatal("missing action ok = true, want false")
	}
	if _, missingToolOK := LookupMetaActionSchema(routes, "gitlab_missing", "empty"); missingToolOK {
		t.Fatal("missing tool ok = true, want false")
	}
}

// TestParseMetaSchemaURI_ValidAndMalformedURIs_ReturnsParsedParts verifies valid per-action schema URIs and malformed
// variants are parsed defensively.
func TestParseMetaSchemaURI_ValidAndMalformedURIs_ReturnsParsedParts(t *testing.T) {
	tool, action := ParseMetaSchemaURI("gitlab://schema/meta/gitlab_project/milestone_delete")
	if tool != "gitlab_project" || action != "milestone_delete" {
		t.Fatalf("ParseMetaSchemaURI() = %q/%q", tool, action)
	}
	for _, uri := range []string{
		"https://example.test/gitlab_project/milestone_delete",
		"gitlab://schema/meta/gitlab_project",
		"gitlab://schema/meta/gitlab_project/milestone/delete",
		"gitlab://schema/meta//delete",
		"gitlab://schema/meta/gitlab_project/",
	} {
		t.Run(uri, func(t *testing.T) {
			if gotTool, gotAction := ParseMetaSchemaURI(uri); gotTool != "" || gotAction != "" {
				t.Fatalf("ParseMetaSchemaURI(%q) = %q/%q, want empty", uri, gotTool, gotAction)
			}
		})
	}
}

// TestMetaSchemaURI_ToolAndAction_ReturnsMetaSchemaURI verifies URI construction uses the registered schema
// namespace and preserves tool/action names exactly.
func TestMetaSchemaURI_ToolAndAction_ReturnsMetaSchemaURI(t *testing.T) {
	got := MetaSchemaURI("gitlab_project", "milestone_delete")
	want := "gitlab://schema/meta/gitlab_project/milestone_delete"
	if got != want {
		t.Fatalf("MetaSchemaURI() = %q, want %q", got, want)
	}
}

// TestMetaActionSchema_SharedRouteDerivesOnce verifies the params schema a
// meta action serves: a route without a captured schema gets a fresh
// permissive placeholder each time, a route over a private schema gets a
// private enriched copy each time, and a route over a shared schema gets one
// enriched map for every caller, with the destructive confirm property and
// the guidance in it and the route's own schema untouched.
func TestMetaActionSchema_SharedRouteDerivesOnce(t *testing.T) {
	t.Parallel()

	placeholder := MetaActionSchema(ActionRoute{Destructive: true})
	if placeholder["type"] != "object" || placeholder["x_destructive"] != true {
		t.Fatalf("MetaActionSchema(no schema) = %#v, want the destructive placeholder", placeholder)
	}
	if sameMap(placeholder, MetaActionSchema(ActionRoute{Destructive: true})) {
		t.Fatal("the placeholder was shared between calls, want a fresh one")
	}

	private := ActionRoute{InputSchema: map[string]any{"type": "object", "properties": map[string]any{}}}
	if sameMap(MetaActionSchema(private), MetaActionSchema(private)) {
		t.Fatal("MetaActionSchema(private route) shared a map nobody registered")
	}

	schema := map[string]any{"type": "object", "properties": map[string]any{"project_id": map[string]any{"type": "string"}}}
	ShareSchema(schema)
	shared := ActionRoute{
		InputSchema:       schema,
		Destructive:       true,
		ParameterGuidance: map[string]ParameterGuidance{"project_id": {SemanticRole: "scope_project"}},
	}
	first := MetaActionSchema(shared)
	if !sameMap(first, MetaActionSchema(shared)) {
		t.Fatal("MetaActionSchema(shared route) built two maps, want one")
	}
	properties, _ := first["properties"].(map[string]any)
	if _, ok := properties["confirm"]; !ok || first["x_destructive"] != true {
		t.Errorf("derived schema = %#v, want the destructive confirm property", first)
	}
	if _, ok := first["x_parameter_guidance"]; !ok {
		t.Errorf("derived schema = %#v, want the parameter guidance", first)
	}
	if _, leaked := schema["x_destructive"]; leaked || len(schema["properties"].(map[string]any)) != 1 {
		t.Fatalf("the route's own schema was mutated: %#v", schema)
	}
}
