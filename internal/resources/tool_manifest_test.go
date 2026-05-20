package resources

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

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

func TestToolManifest_DynamicSurfaceUsesCanonicalActionIDs(t *testing.T) {
	catalog := widgetCatalog(t)
	session := toolManifestSession(t, ToolSurfaceResourceOptions{
		Surface: toolSurfaceDynamic,
		Tools: []*mcp.Tool{
			{Name: "gitlab_execute_tool", Title: "Execute"},
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
	if detail.Kind != toolManifestKindDynamicAction || detail.Tool != "gitlab_execute_tool" || detail.Action != "widget.delete" {
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
	if confirmation["location"] != "gitlab_execute_tool.confirm" {
		t.Fatalf("x_confirmation = %+v, want gitlab_execute_tool.confirm", confirmation)
	}
}

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
	}})
	group.SetAction(actioncatalog.Action{Name: "create", Route: toolutil.ActionRoute{InputSchema: map[string]any{"type": "object"}}})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	return catalog
}

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
