// tool_manifest_test.go contains unit tests for the surface-aware
// tool manifest resources registered by [RegisterToolSurfaceResources].
// Tests build a small action catalog, register the manifest against an
// in-memory MCP server, and read entries via the URI template.
package resources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// toolManifestSession registers the tool manifest resources against an
// in-memory MCP server and returns a connected client session. The
// session and server are torn down via [testing.T.Cleanup] callbacks.
func toolManifestSession(t *testing.T, opts ToolSurfaceResourceOptions) *mcp.ClientSession {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "tool-manifest-test", Version: "0.0.1"}, nil)
	RegisterToolSurfaceResources(server, opts)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// TestToolManifest_DynamicSurfaceUsesCanonicalActionIDs verifies that
// in dynamic mode the manifest uses the canonical "domain.action" IDs
// from the action catalog, sorted alphabetically, and that the
// per-entry detail resource exposes the dynamic call shape
// (params + top-level confirm for destructive actions). The
// x_confirmation extension is added on top of the params schema so
// LLMs know where to set the confirmation.
func TestToolManifest_DynamicSurfaceUsesCanonicalActionIDs(t *testing.T) {
	catalog := widgetCatalog(t)
	session := toolManifestSession(t, ToolSurfaceResourceOptions{
		Surface: toolSurfaceDynamic,
		Tools: []*mcp.Tool{
			{Name: "gitlab_execute_action", Title: "Execute"},
			{Name: "gitlab_find_action", Title: "Find"},
		},
		Catalog: catalog,
	})

	manifest := readToolManifest(t, session, "gitlab://tools")
	// Three entries: the two catalog actions, plus gitlab_find_action, which
	// is directly callable and belongs to no dispatcher.
	if manifest.Surface != toolSurfaceDynamic || manifest.VisibleToolCount != 2 || manifest.EntryCount != 3 {
		t.Fatalf("manifest = %+v, want dynamic with two visible tools and three entries", manifest)
	}
	if manifest.Entries[0].ID != "gitlab_find_action" {
		t.Fatalf("entries = %+v, want the directly callable find tool sorted first", manifest.Entries)
	}
	if manifest.Entries[1].ID != "widget.create" || manifest.Entries[2].ID != "widget.delete" {
		t.Fatalf("entries = %+v, want canonical dynamic IDs sorted", manifest.Entries)
	}

	detail := readToolDetail(t, session, "gitlab://tools/widget.delete")
	if detail.Kind != toolManifestKindDynamicAction || detail.Tool != "gitlab_execute_action" || detail.Action != "widget.delete" {
		t.Fatalf("detail = %+v, want dynamic execute shape", detail)
	}
	if detail.BackingTool != "gitlab_widget" || detail.BackingAction != "delete" {
		t.Fatalf("backing action = %+v, want gitlab_widget/delete", detail)
	}
	if detail.Call.ConfirmLocation != "confirm" || detail.Call.ParamsLocation != "params" {
		t.Fatalf("call = %+v, want dynamic top-level confirm", detail.Call)
	}
	schema := detail.InputSchema.(map[string]any)
	properties := schema["properties"].(map[string]any)
	if _, hasConfirm := properties["confirm"]; hasConfirm {
		t.Fatalf("dynamic detail schema includes params.confirm: %+v", properties)
	}
	confirmation := schema["x_confirmation"].(map[string]any)
	if confirmation["location"] != "gitlab_execute_action.confirm" {
		t.Fatalf("x_confirmation = %+v, want gitlab_execute_action.confirm", confirmation)
	}
}

// TestToolManifest_DynamicSurfaceSkipsActionsWithoutExecuteTool
// verifies that in dynamic mode the manifest lists no catalog actions when the
// gitlab_execute_action tool is not in the visible tools list, even if the
// action catalog is populated: without the dispatcher there is no way to run
// them. The visible "find" tool is still reported, and — being directly
// callable — keeps an entry of its own.
func TestToolManifest_DynamicSurfaceSkipsActionsWithoutExecuteTool(t *testing.T) {
	catalog := widgetCatalog(t)
	session := toolManifestSession(t, ToolSurfaceResourceOptions{
		Surface: toolSurfaceDynamic,
		Tools:   []*mcp.Tool{{Name: "gitlab_find_action", Title: "Find"}},
		Catalog: catalog,
	})

	manifest := readToolManifest(t, session, "gitlab://tools")
	if manifest.VisibleToolCount != 1 {
		t.Fatalf("manifest = %+v, want the visible find tool reported", manifest)
	}
	for _, entry := range manifest.Entries {
		if entry.Kind == toolManifestKindDynamicAction {
			t.Fatalf("entries = %+v, want no dynamic action entries without the execute tool", manifest.Entries)
		}
	}
	if manifest.EntryCount != 1 || manifest.Entries[0].ID != "gitlab_find_action" {
		t.Fatalf("entries = %+v, want only the directly callable find tool", manifest.Entries)
	}
}

// TestToolManifest_UnknownSurfaceUsesIndividualDefaults verifies that
// an unrecognized surface value falls back to individual mode and that
// nil or empty tools are silently skipped. The remaining valid tool is
// rendered as a direct individual entry with the destructive
// confirmation flag wired to arguments.confirm.
func TestToolManifest_UnknownSurfaceUsesIndividualDefaults(t *testing.T) {
	session := toolManifestSession(t, ToolSurfaceResourceOptions{
		Surface: "unknown",
		Tools: []*mcp.Tool{
			nil,
			{Name: ""},
			{Name: "gitlab_delete_project", Title: "Delete Project", InputSchema: "invalid", Annotations: &mcp.ToolAnnotations{DestructiveHint: new(true)}},
		},
	})

	manifest := readToolManifest(t, session, "gitlab://tools")
	if manifest.Surface != toolSurfaceIndividual || manifest.VisibleToolCount != 1 || manifest.EntryCount != 1 {
		t.Fatalf("manifest = %+v, want individual surface with one valid tool", manifest)
	}
	detail := readToolDetail(t, session, "gitlab://tools/gitlab_delete_project")
	if len(detail.RequiredParams) != 0 || detail.Call.ConfirmLocation != "arguments.confirm" {
		t.Fatalf("detail = %+v, want no required params and destructive argument confirmation", detail)
	}
}

// TestToolManifest_MetaSurfaceUsesToolActionIDs verifies that in
// meta mode each entry uses the "gitlab_<tool>.<action>" ID format
// and that the params.confirm field is present in the per-action
// schema. The visible meta tool itself is also exposed as a
// "visible_tool" entry with arguments-based params.
func TestToolManifest_MetaSurfaceUsesToolActionIDs(t *testing.T) {
	catalog := widgetCatalog(t)
	session := toolManifestSession(t, ToolSurfaceResourceOptions{
		Surface: toolSurfaceMeta,
		Tools: []*mcp.Tool{{
			Name:        "gitlab_widget",
			Title:       "Widget",
			InputSchema: map[string]any{"type": "object"},
		}},
		Catalog:    catalog,
		MetaRoutes: catalog.ActionMaps(),
	})

	manifest := readToolManifest(t, session, "gitlab://tools")
	if manifest.Surface != toolSurfaceMeta || manifest.VisibleToolCount != 1 || manifest.EntryCount != 2 {
		t.Fatalf("manifest = %+v, want meta surface with action entries", manifest)
	}
	if manifest.Entries[1].ID != "gitlab_widget.delete" {
		t.Fatalf("entries = %+v, want meta tool.action IDs", manifest.Entries)
	}

	detail := readToolDetail(t, session, "gitlab://tools/gitlab_widget.delete")
	if detail.Kind != toolManifestKindMetaAction || detail.Tool != "gitlab_widget" || detail.Action != "delete" {
		t.Fatalf("detail = %+v, want meta action", detail)
	}
	if detail.Call.ConfirmLocation != "params.confirm" {
		t.Fatalf("detail call/schema = %+v, want meta params confirmation", detail)
	}
	schema := detail.InputSchema.(map[string]any)
	properties := schema["properties"].(map[string]any)
	if _, hasConfirm := properties["confirm"]; !hasConfirm {
		t.Fatalf("meta detail schema missing params.confirm: %+v", properties)
	}

	visibleTool := readToolDetail(t, session, "gitlab://tools/gitlab_widget")
	if visibleTool.Kind != toolManifestKindVisibleTool || visibleTool.Call.ParamsLocation != "arguments" {
		t.Fatalf("visible tool detail = %+v, want direct meta-tool shape", visibleTool)
	}
}

// TestToolManifest_MetaSurfaceIncludesRouteOnlyActions verifies that
// actions present in the route map but not in the action catalog
// (route-only actions) are still surfaced in the meta-surface
// manifest with the right ID and required-params projection.
func TestToolManifest_MetaSurfaceIncludesRouteOnlyActions(t *testing.T) {
	catalog := widgetCatalog(t)
	routes := catalog.ActionMaps()
	routes["gitlab_widget"]["archive"] = toolutil.ActionRoute{InputSchema: map[string]any{
		"type":       "object",
		"required":   []string{"project_id"},
		"properties": map[string]any{"project_id": map[string]any{"type": "integer"}},
	}}
	session := toolManifestSession(t, ToolSurfaceResourceOptions{
		Surface:    toolSurfaceMeta,
		Tools:      []*mcp.Tool{{Name: "gitlab_widget", Title: "Widget"}},
		Catalog:    catalog,
		MetaRoutes: routes,
	})

	detail := readToolDetail(t, session, "gitlab://tools/gitlab_widget.archive")
	if detail.Kind != toolManifestKindMetaAction || detail.Tool != "gitlab_widget" || detail.Action != "archive" {
		t.Fatalf("detail = %+v, want route-only meta action", detail)
	}
	if len(detail.RequiredParams) != 1 || detail.RequiredParams[0].Name != "project_id" {
		t.Fatalf("required params = %v, want project_id", detail.RequiredParams)
	}
	if detail.RequiredParams[0].Type != "integer" {
		t.Fatalf("required param type = %q, want integer; the manifest must not be type-blind", detail.RequiredParams[0].Type)
	}
}

// TestToolManifest_MetaSurfaceSkipsCatalogActionsMissingFromRoutes
// verifies that catalog actions whose route is not present in the
// visible route map are filtered out in meta mode. The remaining
// route-visible action is exposed in the manifest.
func TestToolManifest_MetaSurfaceSkipsCatalogActionsMissingFromRoutes(t *testing.T) {
	catalog := widgetCatalog(t)
	routes := map[string]toolutil.ActionMap{
		"gitlab_widget": {
			"create": catalog.ActionMaps()["gitlab_widget"]["create"],
		},
	}
	session := toolManifestSession(t, ToolSurfaceResourceOptions{
		Surface:    toolSurfaceMeta,
		Tools:      []*mcp.Tool{{Name: "gitlab_widget", Title: "Widget"}},
		Catalog:    catalog,
		MetaRoutes: routes,
	})

	manifest := readToolManifest(t, session, "gitlab://tools")
	if manifest.EntryCount != 1 || manifest.Entries[0].ID != "gitlab_widget.create" {
		t.Fatalf("entries = %+v, want only route-visible create action", manifest.Entries)
	}
}

// TestToolManifestHelpers_DefensiveBranches verifies the defensive
// branches of [actionTitle] (no individual tool title, no tool/action
// names) and [metaRouteVisible] (nil route map).
func TestToolManifestHelpers_DefensiveBranches(t *testing.T) {
	if title := actionTitle(actioncatalog.Action{}); title != "" {
		t.Fatalf("actionTitle(empty) = %q, want empty", title)
	}
	if metaRouteVisible(nil, "gitlab_widget", "create") {
		t.Fatal("metaRouteVisible(nil) = true, want false")
	}
}

// TestToolManifest_IndividualSurfaceUsesDirectToolIDs verifies that
// in individual mode the manifest uses the bare MCP tool name as the
// entry ID, that the per-entry detail exposes arguments-based params
// and a destructive argument.confirm location, and that the tool's
// ReadOnly annotation is reflected in the detail payload.
func TestToolManifest_IndividualSurfaceUsesDirectToolIDs(t *testing.T) {
	session := toolManifestSession(t, ToolSurfaceResourceOptions{
		Surface: toolSurfaceIndividual,
		Tools: []*mcp.Tool{{
			Name:        "gitlab_get_project",
			Title:       "Get Project",
			Description: "Get one project.",
			InputSchema: map[string]any{
				"type":       "object",
				"required":   []any{"project_id"},
				"properties": map[string]any{"project_id": map[string]any{"type": "string"}},
			},
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false)},
		}},
	})

	manifest := readToolManifest(t, session, "gitlab://tools")
	if manifest.Surface != toolSurfaceIndividual || manifest.EntryCount != 1 || manifest.Entries[0].ID != "gitlab_get_project" {
		t.Fatalf("manifest = %+v, want direct individual tool entry", manifest)
	}
	detail := readToolDetail(t, session, "gitlab://tools/gitlab_get_project")
	if detail.Kind != toolManifestKindIndividualTool || detail.Tool != "gitlab_get_project" || !detail.ReadOnly {
		t.Fatalf("detail = %+v, want read-only individual tool", detail)
	}
	if detail.Call.ParamsLocation != "arguments" || len(detail.RequiredParams) != 1 || detail.RequiredParams[0].Name != "project_id" {
		t.Fatalf("detail call/required params = %+v, want direct arguments project_id", detail)
	}
}

// TestToolManifestTemplate_NotFound verifies that the
// "gitlab://tools/{id}" template resource returns a
// ResourceNotFoundError for unknown IDs, empty IDs, slash-separated
// IDs, and unrelated URI schemes.
func TestToolManifestTemplate_NotFound(t *testing.T) {
	session := toolManifestSession(t, ToolSurfaceResourceOptions{Surface: toolSurfaceIndividual})

	for _, uri := range []string{
		"gitlab://tools/unknown",
		"gitlab://tools/",
		"gitlab://tools/a/b",
		"unrelated://uri",
	} {
		t.Run(uri, func(t *testing.T) {
			_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
			if err == nil {
				t.Error("expected ResourceNotFoundError")
			}
		})
	}
}

// widgetCatalog builds a small in-memory action catalog with a
// "widget" domain that contains one destructive "delete" action and
// one "create" action. Used as test input for the tool manifest
// surface tests.
func widgetCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_widget", BaseDomain: "widget"})
	group.SetAction(actioncatalog.Action{Name: "delete", Route: toolutil.ActionRoute{
		InputSchema: map[string]any{
			"type":       "object",
			"required":   []any{"project_id"},
			"properties": map[string]any{"project_id": map[string]any{"type": "string"}},
		},
		Destructive: true,
	}, IndividualTool: toolutil.IndividualToolSpec{Title: "Delete Widget", Description: "Delete a widget."}})
	group.SetAction(actioncatalog.Action{Name: "create", Route: toolutil.ActionRoute{InputSchema: map[string]any{"type": "object"}}})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	return catalog
}

// readToolManifest reads a tool manifest resource and decodes it as a
// [ToolSurfaceManifest]. It fails the test on read or unmarshal error.
func readToolManifest(t *testing.T, session *mcp.ClientSession, uri string) ToolSurfaceManifest {
	t.Helper()
	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("read %s: %v", uri, err)
	}
	var manifest ToolSurfaceManifest
	if uErr := json.Unmarshal([]byte(result.Contents[0].Text), &manifest); uErr != nil {
		t.Fatalf("unmarshal manifest: %v", uErr)
	}
	return manifest
}

// readToolDetail reads a per-entry tool manifest detail and decodes it
// as a [ToolSurfaceDetail]. It fails the test on read or unmarshal
// error.
func readToolDetail(t *testing.T, session *mcp.ClientSession, uri string) ToolSurfaceDetail {
	t.Helper()
	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("read %s: %v", uri, err)
	}
	var detail ToolSurfaceDetail
	if uErr := json.Unmarshal([]byte(result.Contents[0].Text), &detail); uErr != nil {
		t.Fatalf("unmarshal detail: %v", uErr)
	}
	return detail
}

// TestDynamicRequiredParams_IncludesAnyOfAndOneOf verifies the
// defensive branches of [dynamicRequiredParams] and
// [dedupeDynamicStrings]: nil input, alternate-required branches in
// anyOf/oneOf, and deduplication of repeated entries.
func TestDynamicRequiredParams_IncludesAnyOfAndOneOf(t *testing.T) {
	if got := dynamicRequiredParams(nil); got != nil {
		t.Fatalf("dynamicRequiredParams(nil) = %v, want nil", got)
	}
	if got := dedupeDynamicStrings(nil); got != nil {
		t.Fatalf("dedupeDynamicStrings(nil) = %v, want nil", got)
	}
	if got := dedupeDynamicStrings([]string{"", "branch", "branch"}); strings.Join(got, ",") != "branch" {
		t.Fatalf("dedupeDynamicStrings() = %v, want branch", got)
	}
	schema := map[string]any{
		"anyOf": []any{
			"ignored",
			map[string]any{"required": []any{"project_id"}},
		},
		"oneOf": []any{
			map[string]any{"required": []any{"branch"}},
		},
	}

	got := dynamicRequiredParams(schema)
	want := []string{"branch", "project_id"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("dynamicRequiredParams() = %v, want %v", got, want)
	}
}

// TestDynamicParameterGuidanceEntry_CoversAllFields verifies the per-entry
// projection includes every populated field of toolutil.ParameterGuidance
// (semantic_role, value_source, common_confusions, example_binding) and
// omits empty ones to keep the model-facing guidance compact.
func TestDynamicParameterGuidanceEntry_CoversAllFields(t *testing.T) {
	tests := []struct {
		name           string
		item           toolutil.ParameterGuidance
		wantKeys       []string
		wantAbsentKeys []string
	}{
		{
			name: "all fields populated",
			item: toolutil.ParameterGuidance{
				SemanticRole:     "project_id",
				ValueSource:      "from URL or numeric ID",
				CommonConfusions: []string{"namespace_id", "group_id"},
				ExampleBinding:   `params.project_id:"group/project"`,
			},
			wantKeys:       []string{"semantic_role", "value_source", "common_confusions", "example_binding"},
			wantAbsentKeys: nil,
		},
		{
			name: "only semantic role",
			item: toolutil.ParameterGuidance{
				SemanticRole: "branch_name",
			},
			wantKeys:       []string{"semantic_role"},
			wantAbsentKeys: []string{"value_source", "common_confusions", "example_binding"},
		},
		{
			name: "only example binding",
			item: toolutil.ParameterGuidance{
				ExampleBinding: `params.ref:"main"`,
			},
			wantKeys:       []string{"example_binding"},
			wantAbsentKeys: []string{"semantic_role", "value_source", "common_confusions"},
		},
		{
			name:           "all fields empty produces empty entry",
			item:           toolutil.ParameterGuidance{},
			wantKeys:       nil,
			wantAbsentKeys: []string{"semantic_role", "value_source", "common_confusions", "example_binding"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := dynamicParameterGuidanceEntry(tt.item)
			if tt.wantKeys == nil {
				if len(entry) != 0 {
					t.Fatalf("entry = %+v, want empty map for all-empty input", entry)
				}
				return
			}
			for _, key := range tt.wantKeys {
				if _, ok := entry[key]; !ok {
					t.Errorf("entry missing key %q: %+v", key, entry)
				}
			}
			for _, key := range tt.wantAbsentKeys {
				if _, ok := entry[key]; ok {
					t.Errorf("entry should not include key %q: %+v", key, entry)
				}
			}
		})
	}
}

// TestDynamicParameterGuidance_FiltersEmptyEntries verifies the outer
// guidance projection drops params whose entries are entirely empty so the
// guidance map only surfaces meaningful hints to the LLM.
func TestDynamicParameterGuidance_FiltersEmptyEntries(t *testing.T) {
	action := actioncatalog.Action{
		Name: "create",
		Route: toolutil.ActionRoute{
			ParameterGuidance: map[string]toolutil.ParameterGuidance{
				"project_id": {
					SemanticRole:   "project_id",
					ExampleBinding: `params.project_id:"group/project"`,
				},
				"unused_param": {}, // all fields empty; should be dropped
				"branch": {
					SemanticRole: "branch_name",
				},
			},
		},
	}

	guidance := dynamicParameterGuidance(action)
	if _, ok := guidance["unused_param"]; ok {
		t.Errorf("guidance should drop entries with no populated fields: %+v", guidance)
	}
	for _, name := range []string{"project_id", "branch"} {
		if _, ok := guidance[name]; !ok {
			t.Errorf("guidance missing populated entry %q: %+v", name, guidance)
		}
	}
}

// TestDynamicParameterGuidance_NilWhenEmpty verifies the function returns
// nil when the action declares no guidance at all, so enrichDynamicSchema
// can skip the x_parameter_guidance key entirely.
func TestDynamicParameterGuidance_NilWhenEmpty(t *testing.T) {
	if got := dynamicParameterGuidance(actioncatalog.Action{Name: "noop", Route: toolutil.ActionRoute{}}); got != nil {
		t.Fatalf("dynamicParameterGuidance(empty) = %+v, want nil", got)
	}
}

// TestEnrichDynamicSchema_AttachesGuidanceAndDestructive verifies, through
// the served gitlab://tools/{id} detail, the schema
// returned by dynamicActionSchema carries the x_parameter_guidance extension
// when the action declares guidance, and the x_destructive / x_confirmation
// blocks when the route is destructive.
func TestEnrichDynamicSchema_AttachesGuidanceAndDestructive(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_widget", BaseDomain: "widget"})
	group.SetAction(actioncatalog.Action{
		Name: "remove",
		Route: toolutil.ActionRoute{
			Destructive: true,
			ParameterGuidance: map[string]toolutil.ParameterGuidance{
				"project_id": {
					SemanticRole:   "project_id",
					ExampleBinding: `params.project_id:"group/project"`,
				},
			},
		},
	})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	// Read through the served surface: the enriched schema must reach the
	// gitlab://tools/{id} detail, not only the enrichment function.
	session := toolManifestSession(t, ToolSurfaceResourceOptions{
		Surface: toolSurfaceDynamic,
		Tools:   []*mcp.Tool{{Name: "gitlab_execute_action", Title: "Execute"}},
		Catalog: catalog,
	})
	detail := readToolDetail(t, session, "gitlab://tools/widget.remove")
	schema, ok2 := detail.InputSchema.(map[string]any)
	if !ok2 {
		t.Fatalf("detail input_schema = %T, want an enriched schema object", detail.InputSchema)
	}

	guidance, ok := schema["x_parameter_guidance"].(map[string]any)
	if !ok {
		t.Fatalf("schema missing x_parameter_guidance: %+v", schema)
	}
	entry, ok := guidance["project_id"].(map[string]any)
	if !ok {
		t.Fatalf("x_parameter_guidance missing project_id: %+v", guidance)
	}
	if entry["semantic_role"] != "project_id" {
		t.Errorf("semantic_role = %v, want project_id", entry["semantic_role"])
	}
	if entry["example_binding"] != `params.project_id:"group/project"` {
		t.Errorf("example_binding = %v, want quoted project ID", entry["example_binding"])
	}

	if got, exists := schema["x_destructive"].(bool); !exists || !got {
		t.Errorf("x_destructive = %v, want true", schema["x_destructive"])
	}
	confirmation, ok := schema["x_confirmation"].(map[string]any)
	if !ok || confirmation["location"] != "gitlab_execute_action.confirm" {
		t.Errorf("x_confirmation = %+v, want confirm guidance", schema["x_confirmation"])
	}
}

// TestEnrichDynamicSchema_OmitsGuidanceWhenAbsent verifies the schema does
// not include x_parameter_guidance when the action has no guidance entries,
// keeping the contract explicit for downstream consumers.
func TestEnrichDynamicSchema_OmitsGuidanceWhenAbsent(t *testing.T) {
	action := actioncatalog.Action{
		Name: "noop",
		Route: toolutil.ActionRoute{
			Destructive: false,
		},
	}
	schema := map[string]any{"type": "object"}
	enriched := enrichDynamicSchema(schema, action)
	if _, ok := enriched["x_parameter_guidance"]; ok {
		t.Errorf("schema should not include x_parameter_guidance when no guidance declared: %+v", enriched)
	}
	if _, ok := enriched["x_destructive"]; ok {
		t.Errorf("schema should not include x_destructive for non-destructive action: %+v", enriched)
	}
}
