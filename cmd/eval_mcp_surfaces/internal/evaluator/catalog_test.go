package evaluator

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestConvertTools_SkipsNilToolsAndSorts verifies MCP tools are converted into
// provider-facing tools with deterministic order and cache metadata.
func TestConvertTools_SkipsNilToolsAndSorts(t *testing.T) {
	tools := convertTools([]*mcp.Tool{
		nil,
		{Name: "gitlab_z", Description: "z", InputSchema: map[string]any{"type": "object"}},
		{Name: "gitlab_a", Description: "a"},
	})
	if len(tools) != 2 {
		t.Fatalf("convertTools() length = %d, want 2", len(tools))
	}
	if tools[0].Name != "gitlab_a" || tools[1].Name != "gitlab_z" {
		t.Fatalf("convertTools() order = %s,%s; want gitlab_a,gitlab_z", tools[0].Name, tools[1].Name)
	}
	if tools[1].CacheControl == nil || tools[1].CacheControl.Type != "ephemeral" {
		t.Fatalf("last tool cache control = %#v, want ephemeral", tools[1].CacheControl)
	}
	if schema := tools[0].InputSchema.(map[string]any); schema["type"] != "object" {
		t.Fatalf("fallback schema = %#v, want object", schema)
	}
}

// TestRoutesFromSnapshot_UsesActionEnums verifies snapshot schemas produce the
// same action route map shape used by static validation.
func TestRoutesFromSnapshot_UsesActionEnums(t *testing.T) {
	routes := routesFromSnapshot([]snapshotTool{
		{
			Name: "gitlab_project",
			InputSchema: map[string]any{"properties": map[string]any{
				"action": map[string]any{"enum": []any{"get", "list", ""}},
			}},
		},
		{Name: "gitlab_empty", InputSchema: map[string]any{}},
	})
	if _, ok := routes["gitlab_empty"]; ok {
		t.Fatalf("routesFromSnapshot() included tool without actions: %#v", routes)
	}
	if _, ok := routes["gitlab_project"]["get"]; !ok {
		t.Fatalf("routesFromSnapshot() missing get route: %#v", routes)
	}
	if _, ok := routes["gitlab_project"]["list"]; !ok {
		t.Fatalf("routesFromSnapshot() missing list route: %#v", routes)
	}
}

// TestIsNilModelToolSchema_DetectsTypedNilValues verifies the schema fallback
// catches typed nil containers before they reach provider JSON encoders.
func TestIsNilModelToolSchema_DetectsTypedNilValues(t *testing.T) {
	var schema map[string]any
	if !isNilModelToolSchema(schema) {
		t.Fatal("isNilModelToolSchema(typed nil map) = false, want true")
	}
	if isNilModelToolSchema(map[string]any{}) {
		t.Fatal("isNilModelToolSchema(empty map) = true, want false")
	}
}

// TestCatalogToolNames_ReturnsLookupSet verifies catalog tool names are exposed
// as a simple membership map for static validation.
func TestCatalogToolNames_ReturnsLookupSet(t *testing.T) {
	names := catalogToolNames([]modelTool{{Name: "gitlab_project"}, {Name: "gitlab_issue"}})
	if !names["gitlab_project"] || !names["gitlab_issue"] || names["gitlab_missing"] {
		t.Fatalf("catalogToolNames() = %#v", names)
	}
}
