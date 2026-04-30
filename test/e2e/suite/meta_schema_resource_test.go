//go:build e2e

// meta_schema_resource_test.go validates that, after the full meta-tool
// registry is wired into a server, the gitlab://schema/meta/* resources
// expose the captured per-action InputSchemas for real meta-tools.
package suite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// metaSchemaResourceSession registers all meta-tools (which populates
// toolutil.MetaRoutes) plus the meta-schema resources in a single MCP
// server, then returns an in-memory client session. It pins mode=full so
// per-action InputSchemas are captured with their real structured shape.
func metaSchemaResourceSession(t *testing.T, client *gitlabclient.Client, enterprise bool) *mcp.ClientSession {
	t.Helper()
	tools.SetMetaParamSchema("full")
	t.Cleanup(func() { tools.SetMetaParamSchema("opaque") })

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	metaRoutes := toolutil.CaptureMetaRoutes(func() {
		tools.RegisterAllMeta(server, client, enterprise)
	})
	resources.RegisterMetaSchemaResources(server, metaRoutes)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// TestMetaSchemaResource_ListsTemplate verifies the per-action template URI
// is advertised via ListResourceTemplates.
func TestMetaSchemaResource_ListsTemplate(t *testing.T) {
	session := metaSchemaResourceSession(t, sess.glClient, sess.enterprise)

	result, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	var found bool
	for _, tpl := range result.ResourceTemplates {
		if tpl.URITemplate == "gitlab://schema/meta/{tool}/{action}" {
			found = true
			break
		}
	}
	if !found {
		t.Error("meta-schema template not advertised via ListResourceTemplates")
	}
}

// TestMetaSchemaResource_ReadMergeRequestCreate verifies that the captured
// schema for gitlab_merge_request/create exposes the expected structural
// fields callers actually need to construct an MR.
func TestMetaSchemaResource_ReadMergeRequestCreate(t *testing.T) {
	session := metaSchemaResourceSession(t, sess.glClient, sess.enterprise)

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitlab://schema/meta/gitlab_merge_request/create",
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
		t.Fatalf("schema is not valid JSON: %v", uErr)
	}
	if schema["type"] != "object" {
		t.Errorf("type = %v, want object", schema["type"])
	}
}

// TestMetaSchemaResource_NotFound verifies unknown tool/action pairs return
// ResourceNotFoundError when looked up via the live registry.
func TestMetaSchemaResource_NotFound(t *testing.T) {
	session := metaSchemaResourceSession(t, sess.glClient, sess.enterprise)

	_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitlab://schema/meta/gitlab_merge_request/nonexistent_action",
	})
	if err == nil {
		t.Fatal("expected ResourceNotFoundError")
	}
}

// TestMetaSchemaResource_IndexEnumeratesMetaTools verifies the index lists
// at least the canonical meta-tools wired in RegisterAllMeta.
func TestMetaSchemaResource_IndexEnumeratesMetaTools(t *testing.T) {
	session := metaSchemaResourceSession(t, sess.glClient, sess.enterprise)

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{
		URI: "gitlab://schema/meta/",
	})
	if err != nil {
		t.Fatalf("ReadResource index: %v", err)
	}
	body := result.Contents[0].Text
	for _, want := range []string{"gitlab_project", "gitlab_merge_request", "gitlab_issue"} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing meta-tool %q", want)
		}
	}
}
