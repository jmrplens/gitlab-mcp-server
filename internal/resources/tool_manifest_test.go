// tool_manifest_test.go contains unit tests for the surface-aware
// tool manifest resources registered by [RegisterToolSurfaceResources].
// Tests build a small action catalog, register the manifest against an
// in-memory MCP server, and read entries via the URI template.
package resources

import (
	"context"
	"encoding/json"
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
	if manifest.Surface != toolSurfaceDynamic || manifest.VisibleToolCount != 2 || manifest.EntryCount != 2 {
		t.Fatalf("manifest = %+v, want dynamic with two visible tools and two entries", manifest)
	}
	if manifest.Entries[0].ID != "widget.create" || manifest.Entries[1].ID != "widget.delete" {
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
// verifies that in dynamic mode the manifest is empty when the
// gitlab_execute_action tool is not in the visible tools list, even if
// the action catalog is populated. The visible "find" tool is still
// reported in VisibleToolCount.
func TestToolManifest_DynamicSurfaceSkipsActionsWithoutExecuteTool(t *testing.T) {
	catalog := widgetCatalog(t)
	session := toolManifestSession(t, ToolSurfaceResourceOptions{
		Surface: toolSurfaceDynamic,
		Tools:   []*mcp.Tool{{Name: "gitlab_find_action", Title: "Find"}},
		Catalog: catalog,
	})

	manifest := readToolManifest(t, session, "gitlab://tools")
	if manifest.VisibleToolCount != 1 || manifest.EntryCount != 0 {
		t.Fatalf("manifest = %+v, want visible find tool and no executable action entries", manifest)
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
		"type":     "object",
		"required": []string{"project_id"},
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
	if len(detail.RequiredParams) != 1 || detail.RequiredParams[0] != "project_id" {
		t.Fatalf("required params = %v, want project_id", detail.RequiredParams)
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
	if detail.Call.ParamsLocation != "arguments" || len(detail.RequiredParams) != 1 || detail.RequiredParams[0] != "project_id" {
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
