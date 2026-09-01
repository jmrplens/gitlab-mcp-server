// tool_manifest_test.go contains unit tests for the surface-aware
// tool manifest resources registered by [RegisterToolSurfaceResources].
// Tests build a small action catalog, register the manifest against an
// in-memory MCP server, and read entries via the URI template.
package resources

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
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

	// rewriteSeeAlso with a nil resolver is the individual surface's path:
	// hand-written "See also: gitlab_a, gitlab_b." clauses already use
	// individual-tool names, so the description must pass through
	// untouched rather than being rewritten or having names dropped.
	const withSeeAlso = "Get one project. See also: gitlab_get_group, gitlab_list_projects."
	if got := rewriteSeeAlso(withSeeAlso, nil); got != withSeeAlso {
		t.Fatalf("rewriteSeeAlso(nil resolver) = %q, want description unchanged: %q", got, withSeeAlso)
	}

	// newSeeAlsoIndex(nil) must not panic on a nil catalog and must report
	// "no names known" via a nil map, which is what makes rewriteSeeAlso's
	// resolvers built on top of it drop every "See also:" reference instead
	// of indexing a non-existent catalog.
	if index := newSeeAlsoIndex(nil); index != nil {
		t.Fatalf("newSeeAlsoIndex(nil) = %v, want nil", index)
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

// TestManifestRequiredParams_SplitsUnconditionalFromAlternatives verifies
// the manifest's requirement semantics table-driven: the top-level
// "required" list is published as unconditional, anyOf/oneOf branch
// requirements become alternative groups, and names already required at
// the top level are not repeated inside a group.
func TestManifestRequiredParams_SplitsUnconditionalFromAlternatives(t *testing.T) {
	tests := []struct {
		name       string
		schema     map[string]any
		wantParams string
		wantGroups string
	}{
		{"nil schema yields nothing", nil, "", ""},
		{
			"top-level required is unconditional and typed",
			map[string]any{
				"required":   []any{"project_id"},
				"properties": map[string]any{"project_id": map[string]any{"type": "integer"}},
			},
			"project_id:integer", "",
		},
		{
			"anyOf branches become groups, not requirements",
			map[string]any{
				"anyOf": []any{
					"ignored",
					map[string]any{"required": []any{"file_name", "content"}},
					map[string]any{"required": []any{"files"}},
				},
				"properties": map[string]any{"files": map[string]any{"type": "array"}},
			},
			"", "file_name,content|files:array",
		},
		{
			"oneOf branches join the groups",
			map[string]any{"oneOf": []any{map[string]any{"required": []any{"branch"}}}},
			"", "branch",
		},
		{
			"top-level names are not repeated inside groups",
			map[string]any{
				"required": []any{"project_id"},
				"anyOf":    []any{map[string]any{"required": []any{"project_id", "name"}}},
			},
			"project_id", "name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderParams(manifestRequiredParams(tt.schema)); got != tt.wantParams {
				t.Errorf("manifestRequiredParams() = %q, want %q", got, tt.wantParams)
			}
			groups := manifestAlternativeRequiredParams(tt.schema)
			rendered := make([]string, 0, len(groups))
			for _, group := range groups {
				rendered = append(rendered, renderParams(group))
			}
			if got := strings.Join(rendered, "|"); got != tt.wantGroups {
				t.Errorf("manifestAlternativeRequiredParams() = %q, want %q", got, tt.wantGroups)
			}
		})
	}

	t.Run("dedupe helper drops empties and repeats", func(t *testing.T) {
		if got := dedupeDynamicStrings(nil); got != nil {
			t.Fatalf("dedupeDynamicStrings(nil) = %v, want nil", got)
		}
		if got := dedupeDynamicStrings([]string{"", "branch", "branch"}); strings.Join(got, ",") != "branch" {
			t.Fatalf("dedupeDynamicStrings() = %v, want branch", got)
		}
	})
}

// renderParams flattens typed params to "name:type" (type omitted when
// empty), comma-joined, for compact table expectations.
func renderParams(params []ToolSurfaceRequiredParam) string {
	parts := make([]string, 0, len(params))
	for _, param := range params {
		if param.Type == "" {
			parts = append(parts, param.Name)
			continue
		}
		parts = append(parts, param.Name+":"+param.Type)
	}
	return strings.Join(parts, ",")
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

// TestDynamicParameterGuidance_ProjectsPopulatedEntries verifies the
// guidance map table-driven: populated entries survive, all-empty entries
// are dropped, and an action with no guidance yields nil so the schema
// omits the block entirely.
func TestDynamicParameterGuidance_ProjectsPopulatedEntries(t *testing.T) {
	tests := []struct {
		name     string
		guidance map[string]toolutil.ParameterGuidance
		want     []string
		absent   []string
		wantNil  bool
	}{
		{
			name: "populated entries survive, empty entries are dropped",
			guidance: map[string]toolutil.ParameterGuidance{
				"project_id":   {SemanticRole: "project_id", ExampleBinding: `params.project_id:"group/project"`},
				"unused_param": {},
				"branch":       {SemanticRole: "branch_name"},
			},
			want:   []string{"project_id", "branch"},
			absent: []string{"unused_param"},
		},
		{name: "no guidance yields nil", guidance: nil, wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := actioncatalog.Action{Name: "create", Route: toolutil.ActionRoute{ParameterGuidance: tt.guidance}}
			got := dynamicParameterGuidance(action)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("dynamicParameterGuidance() = %+v, want nil", got)
				}
				return
			}
			for _, name := range tt.want {
				if _, ok := got[name]; !ok {
					t.Errorf("guidance missing populated entry %q: %+v", name, got)
				}
			}
			for _, name := range tt.absent {
				if _, ok := got[name]; ok {
					t.Errorf("guidance should drop entry %q with no populated fields: %+v", name, got)
				}
			}
		})
	}
}

// TestEnrichDynamicSchema_GuidanceAndDestructive names both enrichment
// scenarios as subtests: the guidance/destructive/confirmation markers
// reach the served gitlab://tools/{id} detail, and an action without
// guidance or destructiveness omits every marker.
func TestEnrichDynamicSchema_GuidanceAndDestructive(t *testing.T) {
	t.Run("attaches through the served detail", func(t *testing.T) {
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
	})

	t.Run("omits markers when absent", func(t *testing.T) {
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
	})
}

// Drift guard for the "See also:" clauses in the tool manifest: every
// cross-reference an emitted description carries must resolve to an entry ID
// (or visible tool) of the surface that emitted it.

// fullSurfaceCatalog builds the Ultimate-tier catalog the way the dynamic
// surface does at startup: the canonical actions plus the standalone
// surface tools (interactive elicitation, project discovery, server
// maintenance) — the live manifest lists those too, so a guard that skips
// them is blind to their descriptions.
func fullSurfaceCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	catalog, err := gitlabtools.BuildActionCatalog(nil, gitlabtools.ActionCatalogOptions{
		Enterprise: true,
		IncludeMCP: true,
	})
	if err != nil {
		t.Fatalf("BuildActionCatalog: %v", err)
	}
	withStandalone, err := dynamictools.AddStandaloneCatalog(catalog, nil, dynamictools.StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog: %v", err)
	}
	return withStandalone
}

// TestToolManifest_SeeAlsoReferencesResolve_OnEverySurface builds the full
// Ultimate-tier catalog and, for each tool surface, asserts every "See
// also:" reference in every emitted manifest description names an entry of
// that same surface. 2,072+ references shipped with 36 stale names and no
// alarm before this guard existed.
func TestToolManifest_SeeAlsoReferencesResolve_OnEverySurface(t *testing.T) {
	catalog := fullSurfaceCatalog(t)

	surfaces := []struct {
		name string
		opts ToolSurfaceResourceOptions
	}{
		{
			name: toolSurfaceDynamic,
			opts: ToolSurfaceResourceOptions{
				Surface: toolSurfaceDynamic,
				Tools: []*mcp.Tool{
					{Name: "gitlab_execute_action", Title: "Execute"},
					{Name: "gitlab_find_action", Title: "Find"},
				},
				Catalog: catalog,
			},
		},
		{
			name: toolSurfaceMeta,
			opts: ToolSurfaceResourceOptions{
				Surface:    toolSurfaceMeta,
				Catalog:    catalog,
				MetaRoutes: catalog.ActionMaps(),
			},
		},
		{
			// A restricted meta surface (excluded tools, read-only
			// filtering) emits a subset of routes; a reference rewritten
			// to a hidden action's ID would name an entry that does not
			// exist. Dropping gitlab_project — the most referenced tool —
			// exercises exactly that path.
			name: toolSurfaceMeta + " restricted",
			opts: ToolSurfaceResourceOptions{
				Surface:    toolSurfaceMeta,
				Catalog:    catalog,
				MetaRoutes: withoutRoute(catalog.ActionMaps(), "gitlab_project"),
			},
		},
	}

	for _, surface := range surfaces {
		t.Run(surface.name, func(t *testing.T) {
			snapshot := newToolSurfaceSnapshot(surface.opts)
			valid := make(map[string]bool, len(snapshot.manifest.Entries))
			for _, entry := range snapshot.manifest.Entries {
				valid[entry.ID] = true
			}
			for _, tool := range snapshot.manifest.VisibleTools {
				valid[tool.Name] = true
			}
			assertSeeAlsoResolves(t, snapshot, valid)
		})
	}

	// The individual surface has no snapshot projection to check (its
	// descriptions pass through untouched), but its namespace is where the
	// clauses are written — so every referenced name must be a real
	// individual tool. This is the leg that catches stale hand-written
	// names at their source.
	t.Run(toolSurfaceIndividual, func(t *testing.T) {
		valid := make(map[string]bool)
		for _, action := range catalog.Actions() {
			if action.IndividualTool.Name != "" {
				valid[action.IndividualTool.Name] = true
			}
		}
		for _, action := range catalog.Actions() {
			assertSeeAlsoFormat(t, string(action.ID), action.IndividualTool.Description)
			for _, name := range seeAlsoNames(action.IndividualTool.Description) {
				if !valid[name] {
					t.Errorf("action %s references %q in its See-also clause, but no individual tool has that name — fix the spec", action.ID, name)
				}
			}
		}
	})
}

// withoutRoute returns routes with one tool's action map removed,
// modeling a restricted meta surface.
func withoutRoute(routes map[string]toolutil.ActionMap, toolName string) map[string]toolutil.ActionMap {
	delete(routes, toolName)
	return routes
}

// assertSeeAlsoResolves walks every emitted description and requires each
// See-also reference to be a member of the surface's own namespace.
func assertSeeAlsoResolves(t *testing.T, snapshot toolSurfaceSnapshot, valid map[string]bool) {
	t.Helper()
	for _, entry := range snapshot.manifest.Entries {
		assertSeeAlsoFormat(t, entry.ID, entry.Description)
		for _, name := range seeAlsoNames(entry.Description) {
			if !valid[name] {
				t.Errorf("entry %s references %q in its See-also clause, which is not an entry of this surface", entry.ID, name)
			}
		}
	}
}

// seeAlsoNames extracts the referenced names from every "See also:"
// clause in a description — some descriptions carry more than one — using
// the same pattern the projection rewrites.
func seeAlsoNames(description string) []string {
	var names []string
	for _, match := range seeAlsoClause.FindAllStringSubmatch(description, -1) {
		names = append(names, strings.Split(match[1], ", ")...)
	}
	return names
}

// assertSeeAlsoFormat requires every "See also:" occurrence to match the
// canonical clause pattern at that exact position. A clause the pattern
// cannot consume — no trailing period, parenthetical annotations, odd
// separators — is invisible to the projection AND to the resolution legs
// of this guard: it is neither rewritten nor dropped nor checked, which
// is exactly how 13 entries shipped unprojected in v2.7.2.
func assertSeeAlsoFormat(t *testing.T, owner, description string) {
	t.Helper()
	if violation, ok := seeAlsoFormatViolation(description); !ok {
		t.Errorf("%s has a See-also clause outside the canonical format (comma-separated names, trailing period): %.120q", owner, violation)
	}
}

// seeAlsoFormatViolation reports the first "See also:" occurrence the
// canonical clause pattern cannot consume, or ok=true when every clause
// conforms. Pure, so the format rules are testable independently of the
// current catalog's metadata.
func seeAlsoFormatViolation(description string) (string, bool) {
	for idx := strings.Index(description, "See also:"); idx >= 0; {
		loc := seeAlsoClause.FindStringIndex(description[idx:])
		if loc == nil || loc[0] != 0 {
			return description[idx:], false
		}
		rest := description[idx+loc[1]:]
		next := strings.Index(rest, "See also:")
		if next < 0 {
			return "", true
		}
		idx = idx + loc[1] + next
	}
	return "", true
}

// TestSeeAlsoFormatViolation_RecognizesClauseShapes pins the format rules
// themselves, independent of what the catalog currently contains: the
// shapes that shipped unprojected in v2.7.2 (missing period,
// parenthetical annotations) must be violations, and canonical shapes
// must pass.
func TestSeeAlsoFormatViolation_RecognizesClauseShapes(t *testing.T) {
	tests := []struct {
		name        string
		description string
		wantOK      bool
	}{
		{"no clause at all", "List things. Returns: items.", true},
		{"single canonical clause", "Get a thing. See also: gitlab_thing_list, gitlab_thing_delete.", true},
		{"canonical clause with dotted IDs", "Rewritten form. See also: widget.create, gitlab_widget.get.", true},
		{"multiple canonical clauses", "A. See also: gitlab_a. B. See also: gitlab_b, gitlab_c.", true},
		{"missing trailing period", "Get a thing.\n\nSee also: gitlab_thing_list, gitlab_thing_delete", false},
		{"parenthetical annotation", "Resolve. See also: gitlab_thing_get (full CRUD), gitlab_other (checks).", false},
		{"and separator", "Compare. See also: gitlab_a and gitlab_b.", false},
		{"semicolon separator", "Compare. See also: gitlab_a; gitlab_b.", false},
		{"second clause malformed", "A. See also: gitlab_a. B. See also: gitlab_b, gitlab_c", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violation, ok := seeAlsoFormatViolation(tt.description)
			if ok != tt.wantOK {
				t.Errorf("seeAlsoFormatViolation(%q) ok = %v (violation %q), want %v", tt.description, ok, violation, tt.wantOK)
			}
		})
	}
}

// TestToolManifest_AliasEntriesDeclareAliasOf verifies the three deliberate
// alias pairs in the catalog are declared as such in the dynamic manifest,
// instead of presenting two entries a client cannot tell apart.
func TestToolManifest_AliasEntriesDeclareAliasOf(t *testing.T) {
	catalog := fullSurfaceCatalog(t)
	snapshot := newToolSurfaceSnapshot(ToolSurfaceResourceOptions{
		Surface: toolSurfaceDynamic,
		Tools:   []*mcp.Tool{{Name: "gitlab_execute_action"}, {Name: "gitlab_find_action"}},
		Catalog: catalog,
	})
	aliasOf := make(map[string]string)
	for _, entry := range snapshot.manifest.Entries {
		if entry.AliasOf != "" {
			aliasOf[entry.ID] = entry.AliasOf
		}
	}
	// Exact-set comparison: a new shared individual name would add an
	// unintended alias pair, and a primary declaring alias_of would show
	// up as an extra key — both must fail here, not slip through.
	want := map[string]string{
		"user.me":                 "user.current",
		"repository.file_history": "repository.commit_list",
		"issue.list_group":        "group.issues",
	}
	if !maps.Equal(aliasOf, want) {
		t.Errorf("alias_of map = %v, want exactly %v", aliasOf, want)
	}
}
