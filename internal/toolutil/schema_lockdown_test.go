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
		ProjectID string `json:"project_id" jsonschema:"Project ID,required"`
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
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties = %T, want map[string]any", schema["properties"])
	}
	projectID, ok := properties["project_id"].(map[string]any)
	if !ok {
		t.Fatalf("project_id property = %T, want map[string]any", properties["project_id"])
	}
	if projectID["description"] != "Project ID" {
		t.Fatalf("project_id description = %q, want Project ID", projectID["description"])
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

// TestLockdownSchemaNode_AddsEmptyPropertiesForObject verifies object schemas
// without fields still publish an explicit empty properties object for model
// providers that require it.
func TestLockdownSchemaNode_AddsEmptyPropertiesForObject(t *testing.T) {
	t.Parallel()

	node := map[string]any{"type": "object"}

	lockdownSchemaNode(node)

	properties, ok := node["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %T, want map[string]any", node["properties"])
	}
	if len(properties) != 0 {
		t.Fatalf("properties = %#v, want empty map", properties)
	}
	if v, boolOK := node["additionalProperties"].(bool); !boolOK || v {
		t.Fatalf("additionalProperties = %v, want false", node["additionalProperties"])
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
		"oneOf": []any{
			map[string]any{"type": "object"},
		},
		"allOf": []any{
			map[string]any{"type": "object"},
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
	oneOfFirst := node["oneOf"].([]any)[0].(map[string]any)
	if v, _ := oneOfFirst["additionalProperties"].(bool); v {
		t.Errorf("oneOf[0] additionalProperties = true, want false")
	}
	allOfFirst := node["allOf"].([]any)[0].(map[string]any)
	if v, _ := allOfFirst["additionalProperties"].(bool); v {
		t.Errorf("allOf[0] additionalProperties = true, want false")
	}
}

// TestSchemaMap verifies schema normalization accepts maps and marshalable
// structs, while malformed schema values are rejected without panicking.
func TestSchemaMap(t *testing.T) {
	t.Parallel()

	if got := schemaMap(nil); got != nil {
		t.Fatalf("schemaMap(nil) = %#v, want nil", got)
	}
	original := map[string]any{"type": "object"}
	if got := schemaMap(original); got["type"] != "object" {
		t.Fatalf("schemaMap(map) = %#v", got)
	}
	type schemaStruct struct {
		Type string `json:"type"`
	}
	if got := schemaMap(schemaStruct{Type: "object"}); got["type"] != "object" {
		t.Fatalf("schemaMap(struct) = %#v", got)
	}
	if got := schemaMap(func() {}); got != nil {
		t.Fatalf("schemaMap(func) = %#v, want nil", got)
	}
	if got := schemaMap([]string{"not", "an", "object"}); got != nil {
		t.Fatalf("schemaMap(array) = %#v, want nil", got)
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
