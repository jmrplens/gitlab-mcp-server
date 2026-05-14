package toolutil

import "testing"

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

// TestCloneMetaSchemaRoutes_DeepClonesSchemas verifies route snapshots do not
// share nested input or output schema maps with the original route registry.
func TestCloneMetaSchemaRoutes_DeepClonesSchemas(t *testing.T) {
	routes := map[string]ActionMap{
		"gitlab_project": {
			"create": {
				Aliases:        []string{"project.create"},
				Tags:           []string{"project"},
				RelatedActions: []string{"project.get"},
				InputSchema: map[string]any{
					"required": []string{"name"},
					"properties": map[string]any{
						"name": map[string]any{"type": "string"},
					},
				},
				OutputSchema: map[string]any{
					"required": []string{"id"},
				},
				ParameterGuidance: map[string]ParameterGuidance{
					"project_id": {CommonConfusions: []string{"do not use target_project_id"}},
				},
			},
		},
	}

	clone := CloneMetaSchemaRoutes(routes)
	cloneRoute := clone["gitlab_project"]["create"]
	cloneRoute.InputSchema["required"].([]string)[0] = "changed"
	cloneRoute.InputSchema["properties"].(map[string]any)["name"].(map[string]any)["type"] = "integer"
	cloneRoute.OutputSchema["required"].([]string)[0] = "changed"
	cloneRoute.ParameterGuidance["project_id"].CommonConfusions[0] = "changed"
	cloneRoute.Aliases[0] = "changed"
	cloneRoute.Tags[0] = "changed"
	cloneRoute.RelatedActions[0] = "changed"

	original := routes["gitlab_project"]["create"]
	if got := original.InputSchema["required"].([]string)[0]; got != "name" {
		t.Fatalf("original input required = %q, want name", got)
	}
	if got := original.InputSchema["properties"].(map[string]any)["name"].(map[string]any)["type"]; got != "string" {
		t.Fatalf("original property type = %#v, want string", got)
	}
	if got := original.OutputSchema["required"].([]string)[0]; got != "id" {
		t.Fatalf("original output required = %q, want id", got)
	}
	if got := original.ParameterGuidance["project_id"].CommonConfusions[0]; got != "do not use target_project_id" {
		t.Fatalf("original guidance confusion = %q, want unchanged", got)
	}
	if original.Aliases[0] != "project.create" || original.Tags[0] != "project" || original.RelatedActions[0] != "project.get" {
		t.Fatalf("original route metadata = %+v, want unchanged", original)
	}
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
		if index.Tools[0].Actions[i] != want {
			t.Fatalf("issue action %d = %q, want %q", i, index.Tools[0].Actions[i], want)
		}
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
		if gotTool, gotAction := ParseMetaSchemaURI(uri); gotTool != "" || gotAction != "" {
			t.Fatalf("ParseMetaSchemaURI(%q) = %q/%q, want empty", uri, gotTool, gotAction)
		}
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
