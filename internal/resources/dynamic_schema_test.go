package resources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

func dynamicSchemaSession(t *testing.T, catalog *actioncatalog.Catalog) *mcp.ClientSession {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "dynamic-schema-test", Version: "0.0.1"}, nil)
	RegisterDynamicSchemaResources(server, catalog)

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

func TestDynamicSchemaIndex_ListsCanonicalActionsSorted(t *testing.T) {
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

	session := dynamicSchemaSession(t, catalog)
	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://schema/dynamic/"})
	if err != nil {
		t.Fatalf("read dynamic index: %v", err)
	}

	var index DynamicSchemaIndex
	if uErr := json.Unmarshal([]byte(result.Contents[0].Text), &index); uErr != nil {
		t.Fatalf("unmarshal: %v", uErr)
	}
	if index.URITemplate != "gitlab://schema/dynamic/{action}" || index.ExecuteTool != "gitlab_execute_tool" {
		t.Fatalf("index metadata = %+v, want dynamic template and execute tool", index)
	}
	if index.ActionCount != 2 || len(index.Actions) != 2 {
		t.Fatalf("actions = %+v, want 2", index.Actions)
	}
	if index.Actions[0].ID != "widget.create" || index.Actions[1].ID != "widget.delete" {
		t.Fatalf("actions not sorted by canonical ID: %+v", index.Actions)
	}
	deleteAction := index.Actions[1]
	if deleteAction.SchemaURI != "gitlab://schema/dynamic/widget.delete" || deleteAction.MetaSchemaURI != "gitlab://schema/meta/gitlab_widget/delete" {
		t.Fatalf("delete schema URIs = %+v", deleteAction)
	}
	if !deleteAction.Destructive || len(deleteAction.RequiredParams) != 1 || deleteAction.RequiredParams[0] != "project_id" {
		t.Fatalf("delete metadata = %+v, want destructive project_id", deleteAction)
	}
}

func TestDynamicSchemaTemplate_ReturnsDynamicParamsSchema(t *testing.T) {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	session := dynamicSchemaSession(t, catalog)

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://schema/dynamic/project/delete"})
	if err == nil {
		t.Fatalf("slash-separated dynamic action URI should be invalid, got result %+v", result)
	}
	result, err = session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://schema/dynamic/project.delete"})
	if err != nil {
		t.Fatalf("read dynamic action schema: %v", err)
	}

	var schema map[string]any
	if uErr := json.Unmarshal([]byte(result.Contents[0].Text), &schema); uErr != nil {
		t.Fatalf("unmarshal: %v", uErr)
	}
	if !strings.Contains(result.Contents[0].Text, "project_id") {
		t.Fatalf("schema missing project_id: %s", result.Contents[0].Text)
	}
	properties, _ := schema["properties"].(map[string]any)
	if _, hasConfirmParam := properties["confirm"]; hasConfirmParam {
		t.Fatalf("dynamic params schema should not include meta confirm param: %+v", properties)
	}
	confirmation, ok := schema["x_confirmation"].(map[string]any)
	if !ok || confirmation["location"] != "gitlab_execute_tool.confirm" {
		t.Fatalf("x_confirmation = %+v, want top-level dynamic confirmation guidance", schema["x_confirmation"])
	}
}

func TestDynamicSchemaTemplate_NotFound(t *testing.T) {
	session := dynamicSchemaSession(t, nil)

	for _, uri := range []string{
		"gitlab://schema/dynamic/unknown.action",
		"gitlab://schema/dynamic/a/b",
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

func TestDynamicRequiredParams_IncludesAnyOfAndOneOf(t *testing.T) {
	schema := map[string]any{
		"anyOf": []any{
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
