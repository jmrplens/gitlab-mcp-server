//go:build e2e

// mcp_schema_test.go covers the schema invariants the binary enriches every
// tool with: the input-schema lockdown that refuses unknown arguments, the
// output schema a meta dispatcher publishes, and the has_more pagination field
// a list answer carries.
//
// These are properties of the served surface a client reads and acts on, and
// nothing else in this suite checks them: the served-set check compares names,
// and the reads sweep calls lists without inspecting the pagination block. A
// tool registered without additionalProperties:false, or a list that answered
// without has_more, would pass every other gate.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestSchema_ToolsLockDownAdditionalProperties checks that every registered
// tool's root input schema sets additionalProperties:false, so an unknown
// argument is refused rather than passed silently to GitLab.
//
// Replaces: TestSchema_AdditionalPropertiesFalse
func TestSchema_ToolsLockDownAdditionalProperties(t *testing.T) {
	e := harness.New(t)

	for _, surface := range []harness.Surface{harness.SurfaceIndividual, harness.SurfaceMeta} {
		t.Run(string(surface), func(t *testing.T) {
			tools := e.On(surface).ToolDefinitions()
			if len(tools) == 0 {
				t.Fatalf("the %s surface served no tools", surface)
			}
			checked := 0
			for _, tool := range tools {
				schema, ok := tool.InputSchema.(map[string]any)
				if !ok {
					t.Errorf("%s tool %s has an input schema of type %T, not a JSON object", surface, tool.Name, tool.InputSchema)
					continue
				}
				value, present := schema["additionalProperties"]
				if !present {
					t.Errorf("%s tool %s input schema does not set additionalProperties", surface, tool.Name)
					continue
				}
				if value != false {
					t.Errorf("%s tool %s input schema has additionalProperties %v, want false", surface, tool.Name, value)
				}
				checked++
			}
			t.Logf("checked %d %s tools for the input-schema lockdown", checked, surface)
		})
	}
}

// TestSchema_MetaToolsPublishAnOutputSchema checks that every meta dispatcher
// publishes an output schema, so a client can validate what a routed action
// returns.
//
// Replaces: TestSchema_MetaToolsHaveOutputSchema
func TestSchema_MetaToolsPublishAnOutputSchema(t *testing.T) {
	e := harness.New(t)
	tools := e.On(harness.SurfaceMeta).ToolDefinitions()
	if len(tools) == 0 {
		t.Fatal("the meta surface served no tools")
	}
	for _, tool := range tools {
		if tool.OutputSchema == nil {
			t.Errorf("meta tool %s publishes no output schema", tool.Name)
		}
	}
	t.Logf("checked %d meta tools for an output schema", len(tools))
}

// TestSchema_ListAnswersCarryHasMore checks that a paginated list answers with
// a pagination block carrying has_more, the uniform pagination contract every
// list tool exposes.
//
// Replaces: TestSchema_PaginationHasMore
func TestSchema_ListAnswersCarryHasMore(t *testing.T) {
	e := harness.New(t)
	s := e.On(harness.SurfaceMeta)

	answer := harness.Do[map[string]any](s, actionProjectListB6, map[string]any{"per_page": 1})
	pagination, ok := answer["pagination"].(map[string]any)
	if !ok {
		t.Fatalf("project.list answered no pagination object: %v", keysOf(answer))
	}
	if _, present := pagination["has_more"]; !present {
		t.Errorf("the pagination block carries no has_more field: %v", keysOf(pagination))
	}
}

// keysOf lists a map's keys for a failure message.
func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
