// schema_lockdown_test.go verifies JSON Schema lockdown behavior for root,
// nested, and preconfigured additionalProperties values in MCP tool schemas.
package toolutil

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestLockdownInputSchemas_NilServer verifies the helper is safe to call
// with a nil server and does not panic.
func TestLockdownInputSchemas_NilServer(t *testing.T) {
	t.Parallel()
	LockdownInputSchemas(nil)
}

// TestLockdownInputSchemas_AddsFalseToRoot verifies that the registered
// middleware rewrites the tools/list response so a tool whose generated
// inputSchema lacks additionalProperties at the root gets it set to false.
func TestLockdownInputSchemas_AddsFalseToRoot(t *testing.T) {
	t.Parallel()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)

	type In struct {
		ProjectID string `json:"project_id" jsonschema:"required"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "test_tool",
		Description: "A test tool used for additionalProperties lockdown verification.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ In) (*mcp.CallToolResult, any, error) {
		return nil, nil, nil
	})

	LockdownInputSchemas(server)

	tools := listToolsViaClient(t, server)
	got := findTool(t, tools, "test_tool")
	schema := mustSchemaMap(t, got.InputSchema)
	if v, ok := schema["additionalProperties"].(bool); !ok || v {
		t.Fatalf("after lockdown additionalProperties = %v, want false", schema["additionalProperties"])
	}
}

// TestLockdownInputSchemas_PreservesExisting verifies that schemas already
// declaring additionalProperties (true or false) are left untouched. This
// matters for meta-tool router branches that intentionally permit unknown
// fields for forward compatibility.
func TestLockdownInputSchemas_PreservesExisting(t *testing.T) {
	t.Parallel()

	for _, value := range []bool{true, false} {
		label := "false"
		if value {
			label = "true"
		}
		t.Run("preserves_"+label, func(t *testing.T) {
			t.Parallel()
			node := map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"x": map[string]any{"type": "string"}},
				"additionalProperties": value,
			}
			lockdownSchemaNode(node)
			if got, _ := node["additionalProperties"].(bool); got != value {
				t.Fatalf("additionalProperties = %v, want %v", got, value)
			}
		})
	}
}

// TestLockdownSchemaNode_NestedObjects verifies recursion into nested object
// schemas referenced via properties, items, and anyOf.
func TestLockdownSchemaNode_NestedObjects(t *testing.T) {
	t.Parallel()

	node := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"nested": map[string]any{
				"type":       "object",
				"properties": map[string]any{"a": map[string]any{"type": "string"}},
			},
			"list": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":       "object",
					"properties": map[string]any{"b": map[string]any{"type": "string"}},
				},
			},
		},
		"anyOf": []any{
			map[string]any{
				"type":       "object",
				"properties": map[string]any{"c": map[string]any{"type": "string"}},
			},
		},
	}

	lockdownSchemaNode(node)

	if v, _ := node["additionalProperties"].(bool); v {
		t.Errorf("root additionalProperties = true, want false")
	}
	nested := node["properties"].(map[string]any)["nested"].(map[string]any)
	if v, _ := nested["additionalProperties"].(bool); v {
		t.Errorf("nested additionalProperties = true, want false")
	}
	listItems := node["properties"].(map[string]any)["list"].(map[string]any)["items"].(map[string]any)
	if v, _ := listItems["additionalProperties"].(bool); v {
		t.Errorf("array items additionalProperties = true, want false")
	}
	anyOfFirst := node["anyOf"].([]any)[0].(map[string]any)
	if v, _ := anyOfFirst["additionalProperties"].(bool); v {
		t.Errorf("anyOf[0] additionalProperties = true, want false")
	}
}

// TestIsObjectType verifies object-type detection across explicit "type"
// and properties-only inference paths.
func TestIsObjectType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		node map[string]any
		want bool
	}{
		{"explicit_object", map[string]any{"type": "object"}, true},
		{"properties_only", map[string]any{"properties": map[string]any{}}, true},
		{"string_type", map[string]any{"type": "string"}, false},
		{"empty", map[string]any{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isObjectType(tc.node); got != tc.want {
				t.Fatalf("isObjectType(%v) = %v, want %v", tc.node, got, tc.want)
			}
		})
	}
}

// listToolsViaClient connects a temporary in-memory MCP client to server,
// calls tools/list (which exercises the lockdown middleware), and returns
// the tools.
func listToolsViaClient(t *testing.T, server *mcp.Server) []*mcp.Tool {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	return res.Tools
}

// findTool returns the tool with the given name or fails the test.
func findTool(t *testing.T, tools []*mcp.Tool, name string) *mcp.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found in %d tools", name, len(tools))
	return nil
}

// mustSchemaMap asserts an InputSchema marshals to a JSON object.
func mustSchemaMap(t *testing.T, raw any) map[string]any {
	t.Helper()
	schema, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("InputSchema is %T, want map[string]any", raw)
	}
	return schema
}
