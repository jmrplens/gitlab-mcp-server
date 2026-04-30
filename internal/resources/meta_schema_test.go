// meta_schema_test.go covers the gitlab://schema/meta/* MCP resources:
// the index resource, the per-action template resource, URI parsing edge
// cases, and the InputSchema lookup contract from explicit route snapshots.
package resources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// metaSchemaSession spins up a dedicated MCP session with only the
// meta-schema resources registered. It passes the supplied fixture directly
// so each test sees a deterministic, isolated catalog.
func metaSchemaSession(t *testing.T, fixture map[string]toolutil.ActionMap) *mcp.ClientSession {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "meta-schema-test", Version: "0.0.1"}, nil)
	RegisterMetaSchemaResources(server, fixture)

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

// fakeRoute returns an ActionRoute with the given InputSchema for fixture use.
func fakeRoute(schema map[string]any) toolutil.ActionRoute {
	return toolutil.ActionRoute{InputSchema: schema}
}

// TestMetaSchemaIndex_ListsAllToolsSorted verifies the index resource emits
// every registered meta-tool with its actions sorted alphabetically.
func TestMetaSchemaIndex_ListsAllToolsSorted(t *testing.T) {
	session := metaSchemaSession(t, map[string]toolutil.ActionMap{
		"gitlab_widget": {
			"list":   fakeRoute(nil),
			"create": fakeRoute(nil),
			"delete": fakeRoute(nil),
		},
		"gitlab_alpha": {
			"get": fakeRoute(nil),
		},
	})

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://schema/meta/"})
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}

	var index MetaSchemaIndex
	if uErr := json.Unmarshal([]byte(result.Contents[0].Text), &index); uErr != nil {
		t.Fatalf("unmarshal: %v", uErr)
	}
	if index.URITemplate != "gitlab://schema/meta/{tool}/{action}" {
		t.Errorf("uri_template = %q", index.URITemplate)
	}
	if len(index.Tools) != 2 {
		t.Fatalf("tools = %d, want 2", len(index.Tools))
	}
	if index.Tools[0].Tool != "gitlab_alpha" || index.Tools[1].Tool != "gitlab_widget" {
		t.Errorf("tools not sorted: %+v", index.Tools)
	}
	want := []string{"create", "delete", "list"}
	for i, a := range index.Tools[1].Actions {
		if a != want[i] {
			t.Errorf("actions not sorted: got %v, want %v", index.Tools[1].Actions, want)
			break
		}
	}
}

// TestMetaSchemaTemplate_ReturnsInputSchema verifies the template resource
// returns the route's captured InputSchema as a JSON object.
func TestMetaSchemaTemplate_ReturnsInputSchema(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project_id": map[string]any{"type": "string"},
		},
		"required": []any{"project_id"},
	}
	session := metaSchemaSession(t, map[string]toolutil.ActionMap{
		"gitlab_widget": {"create": fakeRoute(schema)},
	})

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://schema/meta/gitlab_widget/create"})
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	if !strings.Contains(result.Contents[0].Text, `"project_id"`) {
		t.Errorf("schema missing project_id: %s", result.Contents[0].Text)
	}
}

// TestMetaSchemaTemplate_FallbackForMissingSchema verifies that a registered
// action without a captured InputSchema returns a permissive placeholder
// rather than null or an empty object.
func TestMetaSchemaTemplate_FallbackForMissingSchema(t *testing.T) {
	session := metaSchemaSession(t, map[string]toolutil.ActionMap{
		"gitlab_widget": {"ping": fakeRoute(nil)},
	})

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://schema/meta/gitlab_widget/ping"})
	if err != nil {
		t.Fatalf("read fallback: %v", err)
	}
	var got map[string]any
	if uErr := json.Unmarshal([]byte(result.Contents[0].Text), &got); uErr != nil {
		t.Fatalf("unmarshal: %v", uErr)
	}
	if got["type"] != "object" {
		t.Errorf("fallback should declare type=object, got %v", got)
	}
}

// TestMetaSchemaTemplate_NotFound covers the unhappy paths: unknown tool,
// unknown action, malformed URI segments.
func TestMetaSchemaTemplate_NotFound(t *testing.T) {
	session := metaSchemaSession(t, map[string]toolutil.ActionMap{
		"gitlab_widget": {"create": fakeRoute(nil)},
	})

	cases := []string{
		"gitlab://schema/meta/unknown_tool/create",
		"gitlab://schema/meta/gitlab_widget/unknown_action",
		"gitlab://schema/meta/gitlab_widget",
		"gitlab://schema/meta/gitlab_widget/",
		"gitlab://schema/meta//create",
		"gitlab://schema/meta/a/b/c",
	}
	for _, uri := range cases {
		t.Run(uri, func(t *testing.T) {
			_, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
			if err == nil {
				t.Error("expected ResourceNotFoundError")
			}
		})
	}
}

// TestParseMetaSchemaURI covers the pure URI parser logic without going
// through the MCP transport. Avoids accidental behavioral drift between
// transport-tested cases and helper-only edge cases.
func TestParseMetaSchemaURI(t *testing.T) {
	cases := []struct {
		uri        string
		wantTool   string
		wantAction string
	}{
		{"gitlab://schema/meta/foo/bar", "foo", "bar"},
		{"gitlab://schema/meta/foo/", "", ""},
		{"gitlab://schema/meta//bar", "", ""},
		{"gitlab://schema/meta/foo", "", ""},
		{"gitlab://schema/meta/foo/bar/baz", "", ""},
		{"unrelated://uri", "", ""},
	}
	for _, c := range cases {
		t.Run(c.uri, func(t *testing.T) {
			gotTool, gotAction := parseMetaSchemaURI(c.uri)
			if gotTool != c.wantTool || gotAction != c.wantAction {
				t.Errorf("parseMetaSchemaURI(%q) = (%q,%q), want (%q,%q)",
					c.uri, gotTool, gotAction, c.wantTool, c.wantAction)
			}
		})
	}
}

// TestMetaSchemaIndex_EmptyRegistry verifies the index resource still
// returns a well-formed JSON envelope when no meta-tools are registered.
func TestMetaSchemaIndex_EmptyRegistry(t *testing.T) {
	session := metaSchemaSession(t, nil)

	result, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "gitlab://schema/meta/"})
	if err != nil {
		t.Fatalf("read empty index: %v", err)
	}
	var index MetaSchemaIndex
	if uErr := json.Unmarshal([]byte(result.Contents[0].Text), &index); uErr != nil {
		t.Fatalf("unmarshal: %v", uErr)
	}
	if len(index.Tools) != 0 {
		t.Errorf("empty registry should yield zero tools, got %d", len(index.Tools))
	}
}
