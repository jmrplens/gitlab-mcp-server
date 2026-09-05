// schema_lockdown_test.go verifies JSON Schema lockdown behavior for root,
// nested, and preconfigured additionalProperties values in MCP tool schemas.
package toolutil

import (
	"context"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
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

// TestSchemaMapCopy_Inputs_CopiedOrRejected verifies the copy accepts maps
// and marshalable structs, that a map input yields a map the caller owns
// rather than the input itself, and that malformed schema values are
// rejected without panicking.
func TestSchemaMapCopy_Inputs_CopiedOrRejected(t *testing.T) {
	t.Parallel()

	if got := schemaMapCopy(nil); got != nil {
		t.Fatalf("schemaMapCopy(nil) = %#v, want nil", got)
	}
	original := map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}}
	got := schemaMapCopy(original)
	if got["type"] != "object" {
		t.Fatalf("schemaMapCopy(map) = %#v", got)
	}
	got["type"] = "changed"
	got["properties"].(map[string]any)["name"].(map[string]any)["type"] = "integer"
	if original["type"] != "object" || original["properties"].(map[string]any)["name"].(map[string]any)["type"] != "string" {
		t.Fatalf("schemaMapCopy(map) shares storage with its input: original = %#v", original)
	}
	type schemaStruct struct {
		Type string `json:"type"`
	}
	if fromStruct := schemaMapCopy(schemaStruct{Type: "object"}); fromStruct["type"] != "object" {
		t.Fatalf("schemaMapCopy(struct) = %#v", fromStruct)
	}
	if fromFunc := schemaMapCopy(func() {}); fromFunc != nil {
		t.Fatalf("schemaMapCopy(func) = %#v, want nil", fromFunc)
	}
	if fromArray := schemaMapCopy([]string{"not", "an", "object"}); fromArray != nil {
		t.Fatalf("schemaMapCopy(array) = %#v, want nil", fromArray)
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

// TestLockedDownSchema_SharesDerivationsAndKeepsWhatItCannotRender verifies
// the derivation behind the middleware: nil stays nil, a value that cannot
// be rendered as a JSON object is returned as it is, a shared compiled
// schema is locked down once and served to every server, and the compiled
// schema itself is not touched.
func TestLockedDownSchema_SharesDerivationsAndKeepsWhatItCannotRender(t *testing.T) {
	t.Parallel()

	if got := lockedDownSchema(nil); got != nil {
		t.Errorf("lockedDownSchema(nil) = %#v, want nil", got)
	}
	unrenderable := func() {}
	if lockedDownSchema(unrenderable) == nil {
		t.Error("lockedDownSchema(func) = nil, want the input kept")
	}

	compiled := &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{"name": {Type: "string"}}}
	ShareSchema(compiled)
	first, _ := lockedDownSchema(compiled).(map[string]any)
	second, _ := lockedDownSchema(compiled).(map[string]any)
	if first == nil || !sameMap(first, second) {
		t.Fatalf("lockedDownSchema(shared compiled) = %#v then %#v, want one shared map", first, second)
	}
	if first["additionalProperties"] != false {
		t.Errorf("locked down schema = %#v, want additionalProperties false", first)
	}
	if compiled.AdditionalProperties != nil {
		t.Error("the shared compiled schema was changed in place")
	}
	if !SchemaShared(first) {
		t.Error("the locked down map was not registered as shared")
	}
}
