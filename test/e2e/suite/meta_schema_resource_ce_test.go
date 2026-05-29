//go:build e2e && !enterprise

// meta_schema_resource_ce_test.go validates that the gitlab://tools manifest
// exposes catalog per-action InputSchemas for real meta-tools.
package suite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
)

// toolManifestResourceSession registers catalog-backed meta-tools plus the
// unified tool manifest resources in a single MCP server, then returns an in-memory
// client session. It pins mode=full so per-action InputSchemas keep their
// real structured shape.
func toolManifestResourceSession(t *testing.T, client *gitlabclient.Client, enterprise bool) *mcp.ClientSession {
	t.Helper()
	t.Cleanup(tools.SetMetaParamSchemaScoped("full"))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	catalog, err := tools.BuildActionCatalog(client, tools.ActionCatalogOptions{Enterprise: enterprise})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	tools.RegisterMetaCatalog(server, catalog)
	resources.RegisterToolSurfaceResources(server, resources.ToolSurfaceResourceOptions{
		Surface:    config.ToolSurfaceMeta,
		Tools:      listToolsForManifest(t, server),
		Catalog:    catalog,
		MetaRoutes: catalog.ActionMaps(),
	})

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, connectErr := server.Connect(ctx, st, nil)
	if connectErr != nil {
		t.Fatalf("server connect: %v", connectErr)
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

// listToolsForManifest creates a temporary in-memory client/server session to
// inspect the server tools available during resource registration.
func listToolsForManifest(t *testing.T, server *mcp.Server) []*mcp.Tool {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	var closeClientSession func() error
	var closeServerSession func() error
	closed := false
	cleanupSessions := func() {
		if closed {
			return
		}
		closed = true
		if closeClientSession != nil {
			_ = closeClientSession()
		}
		if closeServerSession != nil {
			_ = closeServerSession()
		}
	}
	t.Cleanup(cleanupSessions)
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect for tool manifest: %v", err)
	}
	closeServerSession = serverSession.Close
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect for tool manifest: %v", err)
	}
	closeClientSession = session.Close
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools for tool manifest: %v", err)
	}
	cleanupSessions()
	return result.Tools
}

// TestToolManifestResource_ListsTemplate verifies the per-action template URI
// is advertised via ListResourceTemplates.
func TestToolManifestResource_ListsTemplate(t *testing.T) {
	session := toolManifestResourceSession(t, sess.glClient, sess.enterprise)

	result, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	var found bool
	for _, tpl := range result.ResourceTemplates {
		if tpl.URITemplate == "gitlab://tools/{id}" {
			found = true
			break
		}
	}
	if !found {
		t.Error("tool manifest detail template not advertised via ListResourceTemplates")
	}
}

// TestToolManifestResource_ReadMergeRequestCreate verifies that the captured
// schema for gitlab_merge_request/create exposes the expected structural
// fields callers actually need to construct an MR.
func TestToolManifestResource_ReadMergeRequestCreate(t *testing.T) {
	session := toolManifestResourceSession(t, sess.glClient, sess.enterprise)

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitlab://tools/gitlab_merge_request.create",
	})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("contents = %d, want 1", len(result.Contents))
	}
	body := result.Contents[0].Text
	for _, want := range []string{"project_id", "source_branch", "target_branch", "title"} {
		if !strings.Contains(body, want) {
			t.Errorf("schema missing %q", want)
		}
	}

	var schema map[string]any
	if uErr := json.Unmarshal([]byte(body), &schema); uErr != nil {
		t.Fatalf("detail is not valid JSON: %v", uErr)
	}
	call, _ := schema["call"].(map[string]any)
	if call["tool"] != "gitlab_merge_request" || call["action"] != "create" {
		t.Errorf("call = %v, want gitlab_merge_request/create", call)
	}
	inputSchema, _ := schema["input_schema"].(map[string]any)
	if inputSchema["type"] != "object" {
		t.Errorf("input_schema.type = %v, want object", inputSchema["type"])
	}
}

// TestToolManifestResource_NotFound verifies unknown tool/action pairs return
// ResourceNotFoundError when looked up via the live registry.
func TestToolManifestResource_NotFound(t *testing.T) {
	session := toolManifestResourceSession(t, sess.glClient, sess.enterprise)

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitlab://tools/gitlab_merge_request.nonexistent_action",
	})
	if err == nil {
		t.Fatal("expected ResourceNotFoundError")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Fatalf("ReadResource error = %v, want not found", err)
	}
}

// TestToolManifestResource_IndexEnumeratesMetaTools verifies the manifest lists
// at least the canonical meta-tools wired in RegisterAllMeta.
func TestToolManifestResource_IndexEnumeratesMetaTools(t *testing.T) {
	session := toolManifestResourceSession(t, sess.glClient, sess.enterprise)

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitlab://tools",
	})
	if err != nil {
		t.Fatalf("ReadResource index: %v", err)
	}
	body := result.Contents[0].Text
	for _, want := range []string{"gitlab_project.get", "gitlab_merge_request.create", "gitlab_issue.get"} {
		if !strings.Contains(body, want) {
			t.Errorf("manifest missing meta-tool action %q", want)
		}
	}
}
