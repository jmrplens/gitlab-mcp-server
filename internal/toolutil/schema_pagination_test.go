// schema_pagination_test.go verifies pagination constraint enrichment across
// root and nested JSON Schema nodes.
package toolutil

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestEnrichPaginationConstraints_NilServer verifies the helper is safe to
// call with a nil server.
func TestEnrichPaginationConstraints_NilServer(t *testing.T) {
	EnrichPaginationConstraints(nil)
}

// TestEnrichPaginationConstraints_AddsBounds verifies that page and per_page
// integer properties receive the documented minimum/maximum constraints when
// none are already present.
func TestEnrichPaginationConstraints_AddsBounds(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_things",
		Description: "List things.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page":     map[string]any{"type": "integer"},
				"per_page": map[string]any{"type": "integer"},
				"name":     map[string]any{"type": "string"},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})

	EnrichPaginationConstraints(server)

	got := findTool(t, listToolsViaClient(t, server), "list_things")
	schema := mustSchemaMap(t, got.InputSchema)
	props, _ := schema["properties"].(map[string]any)

	page, _ := props["page"].(map[string]any)
	if v, ok := page["minimum"]; !ok || v.(float64) != 1 {
		t.Errorf("page.minimum = %v, want 1", page["minimum"])
	}
	if _, ok := page["maximum"]; ok {
		t.Errorf("page.maximum unexpectedly set: %v", page["maximum"])
	}

	perPage, _ := props["per_page"].(map[string]any)
	if v, ok := perPage["minimum"]; !ok || v.(float64) != 1 {
		t.Errorf("per_page.minimum = %v, want 1", perPage["minimum"])
	}
	if v, ok := perPage["maximum"]; !ok || v.(float64) != 100 {
		t.Errorf("per_page.maximum = %v, want 100", perPage["maximum"])
	}

	if _, ok := props["name"].(map[string]any)["minimum"]; ok {
		t.Errorf("non-numeric property should not receive minimum")
	}
}

// TestEnrichPaginationConstraints_PreservesExisting verifies that explicit
// minimum/maximum already present on the schema are not overwritten.
func TestEnrichPaginationConstraints_PreservesExisting(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_things",
		Description: "List things.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"per_page": map[string]any{
					"type":    "integer",
					"minimum": float64(5),
					"maximum": float64(50),
				},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})

	EnrichPaginationConstraints(server)

	got := findTool(t, listToolsViaClient(t, server), "list_things")
	schema := mustSchemaMap(t, got.InputSchema)
	perPage := schema["properties"].(map[string]any)["per_page"].(map[string]any)
	if perPage["minimum"].(float64) != 5 {
		t.Errorf("per_page.minimum overwritten: %v", perPage["minimum"])
	}
	if perPage["maximum"].(float64) != 50 {
		t.Errorf("per_page.maximum overwritten: %v", perPage["maximum"])
	}
}

// TestEnrichPaginationConstraints_SkipsNonNumeric verifies the middleware
// does not mutate properties named page/per_page when their declared type
// is non-numeric (defensive behavior for custom schemas).
func TestEnrichPaginationConstraints_SkipsNonNumeric(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "weird",
		Description: "Weird tool with a string `page` parameter.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page": map[string]any{"type": "string"},
			},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})

	EnrichPaginationConstraints(server)

	got := findTool(t, listToolsViaClient(t, server), "weird")
	page := mustSchemaMap(t, got.InputSchema)["properties"].(map[string]any)["page"].(map[string]any)
	if _, ok := page["minimum"]; ok {
		t.Errorf("string page unexpectedly received minimum: %v", page["minimum"])
	}
}

// TestEnrichPaginationNode_RecursesItems verifies enrichPaginationNode
// recurses through the "items" key to reach nested page/per_page.
func TestEnrichPaginationNode_RecursesItems(t *testing.T) {
	t.Parallel()
	node := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"page": map[string]any{"type": "integer"},
			},
		},
	}
	enrichPaginationNode(node)

	items := node["items"].(map[string]any)
	page := items["properties"].(map[string]any)["page"].(map[string]any)
	if v, ok := page["minimum"]; !ok || v.(float64) != 1 {
		t.Errorf("items→page.minimum = %v, want 1", page["minimum"])
	}
}

// TestEnrichPaginationNode_RecursesAnyOfOneOfAllOf verifies enrichPaginationNode
// recurses through anyOf, oneOf, and allOf arrays to reach nested schemas.
func TestEnrichPaginationNode_RecursesAnyOfOneOfAllOf(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"anyOf", "oneOf", "allOf"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			node := map[string]any{
				key: []any{
					map[string]any{
						"type": "object",
						"properties": map[string]any{
							"per_page": map[string]any{"type": "integer"},
						},
					},
				},
			}
			enrichPaginationNode(node)

			branch := node[key].([]any)[0].(map[string]any)
			perPage := branch["properties"].(map[string]any)["per_page"].(map[string]any)
			if v, ok := perPage["minimum"]; !ok || v.(float64) != 1 {
				t.Errorf("%s→per_page.minimum = %v, want 1", key, perPage["minimum"])
			}
			if v, ok := perPage["maximum"]; !ok || v.(float64) != 100 {
				t.Errorf("%s→per_page.maximum = %v, want 100", key, perPage["maximum"])
			}
		})
	}
}

// TestIsIntegerLike_NonStringType verifies that isIntegerLike returns false
// when the "type" field exists but is not a string (e.g. an integer or array).
func TestIsIntegerLike_NonStringType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		node map[string]any
		want bool
	}{
		{"no type field (defaults to integer)", map[string]any{}, true},
		{"type=integer", map[string]any{"type": "integer"}, true},
		{"type=number", map[string]any{"type": "number"}, true},
		{"type=string", map[string]any{"type": "string"}, false},
		{"type=object", map[string]any{"type": "object"}, false},
		{"type is int (non-string)", map[string]any{"type": 42}, false},
		{"type is array (non-string)", map[string]any{"type": []any{"integer", "null"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isIntegerLike(tt.node)
			if got != tt.want {
				t.Errorf("isIntegerLike(%v) = %v, want %v", tt.node, got, tt.want)
			}
		})
	}
}
