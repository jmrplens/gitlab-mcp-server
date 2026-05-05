package toolutil

import "testing"

// TestLookupMetaActionSchema_DestructiveActionAddsConfirm verifies that
// destructive action schemas are copied and augmented with confirmation
// metadata without mutating the registered route schema.
//
// The test builds an in-memory route map for milestone_delete, looks up its
// schema, and asserts that the returned schema includes confirm and
// x_destructive while the original InputSchema remains unchanged.
func TestLookupMetaActionSchema_DestructiveActionAddsConfirm(t *testing.T) {
	routes := map[string]ActionMap{
		"gitlab_project": {
			"milestone_delete": {
				Destructive: true,
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"project_id": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	schema, ok := LookupMetaActionSchema(routes, "gitlab_project", "milestone_delete")
	if !ok {
		t.Fatal("LookupMetaActionSchema() ok = false, want true")
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties missing: %#v", schema)
	}
	if _, hasConfirm := properties["confirm"]; !hasConfirm {
		t.Fatalf("confirm property missing: %#v", properties)
	}
	if schema["x_destructive"] != true {
		t.Fatalf("x_destructive = %#v, want true", schema["x_destructive"])
	}
	originalProperties := routes["gitlab_project"]["milestone_delete"].InputSchema["properties"].(map[string]any)
	if _, originalHasConfirm := originalProperties["confirm"]; originalHasConfirm {
		t.Fatalf("original schema was mutated: %#v", originalProperties)
	}
}
