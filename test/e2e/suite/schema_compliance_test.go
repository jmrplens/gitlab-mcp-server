//go:build e2e

// schema_compliance_test.go validates that MCP tool schemas are correctly
// enriched at runtime: additionalProperties lockdown, OutputSchema on
// meta-tools, and has_more pagination field in list tool responses.
package suite

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestSchema_AdditionalPropertiesFalse verifies that every tool's root
// inputSchema has additionalProperties: false set by the LockdownInputSchemas
// middleware. This prevents LLMs from silently passing unknown arguments.
func TestSchema_AdditionalPropertiesFalse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Individual session
	if sess.individual != nil {
		indResult, err := sess.individual.ListTools(ctx, nil)
		requireNoError(t, err, "ListTools individual")
		requireTrue(t, len(indResult.Tools) > 0, "expected individual tools, got 0")

		var missing int
		for _, tool := range indResult.Tools {
			schema, ok := tool.InputSchema.(map[string]any)
			if !ok {
				continue
			}
			val, present := schema["additionalProperties"]
			if !present {
				t.Errorf("[individual] tool %s: root inputSchema missing additionalProperties", tool.Name)
				missing++
				continue
			}
			if val != false {
				t.Errorf("[individual] tool %s: additionalProperties = %v, want false", tool.Name, val)
				missing++
			}
		}
		t.Logf("Checked %d individual tools for additionalProperties: false (%d violations)", len(indResult.Tools), missing)
	}

	// Meta session
	if sess.meta != nil {
		metaResult, err := sess.meta.ListTools(ctx, nil)
		requireNoError(t, err, "ListTools meta")
		requireTrue(t, len(metaResult.Tools) > 0, "expected meta-tools, got 0")

		var missing int
		for _, tool := range metaResult.Tools {
			schema, ok := tool.InputSchema.(map[string]any)
			if !ok {
				continue
			}
			val, present := schema["additionalProperties"]
			if !present {
				t.Errorf("[meta] tool %s: root inputSchema missing additionalProperties", tool.Name)
				missing++
				continue
			}
			if val != false {
				t.Errorf("[meta] tool %s: additionalProperties = %v, want false", tool.Name, val)
				missing++
			}
		}
		t.Logf("Checked %d meta-tools for additionalProperties: false (%d violations)", len(metaResult.Tools), missing)
	}
}

// TestSchema_MetaToolsHaveOutputSchema verifies that every meta-tool
// (tools with meta-tool routing pattern) has a non-nil OutputSchema
// registered in the tools/list response.
func TestSchema_MetaToolsHaveOutputSchema(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	result, err := sess.meta.ListTools(ctx, nil)
	requireNoError(t, err, "ListTools meta")
	requireTrue(t, len(result.Tools) > 0, "expected meta-tools, got 0")

	var withSchema, withoutSchema int
	for _, tool := range result.Tools {
		if tool.OutputSchema != nil {
			withSchema++
		} else {
			t.Errorf("[meta] tool %s: nil OutputSchema", tool.Name)
			withoutSchema++
		}
	}
	t.Logf("Meta-tools: %d with OutputSchema, %d without", withSchema, withoutSchema)
	requireTrue(t, withoutSchema == 0, "found %d meta-tools without OutputSchema", withoutSchema)
}

// TestSchema_PaginationHasMore verifies that paginated list tool responses
// include the has_more boolean field in their structured output. This tests
// the pagination output enrichment by calling a list tool that returns
// paginated results.
func TestSchema_PaginationHasMore(t *testing.T) {
	t.Parallel()
	if sess.individual == nil && sess.meta == nil {
		t.Skip("no sessions configured")
	}

	ctx := context.Background()

	// Call a list tool that returns paginated results.
	// gitlab_list_projects is reliable for this test as it always has results.
	var session = sess.meta
	toolName := "gitlab_project"
	input := map[string]any{
		"action": "list",
		"params": map[string]any{
			"per_page": 1,
		},
	}
	if session == nil {
		session = sess.individual
		toolName = "gitlab_list_projects"
		input = map[string]any{
			"per_page": 1,
		}
	}

	result, err := callToolWithRetry(ctx, session, toolName, input)
	requireNoError(t, err, "CallTool "+toolName)

	// Extract structured content or text content and look for has_more.
	var raw []byte
	if result.StructuredContent != nil {
		raw, err = json.Marshal(result.StructuredContent)
		requireNoError(t, err, "marshal structured content")
	} else if len(result.Content) > 0 {
		for _, c := range result.Content {
			if tc, ok := c.(interface{ GetText() string }); ok {
				text := tc.GetText()
				if strings.Contains(text, "has_more") {
					raw = []byte(text)
					break
				}
			}
		}
	}

	if raw == nil {
		t.Fatal("no extractable content from " + toolName)
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// Look for has_more at top level or nested inside a pagination object.
	if _, ok := parsed["has_more"]; ok {
		t.Logf("has_more found at top level in %s response", toolName)
		return
	}
	if pag, ok := parsed["pagination"]; ok {
		if pagMap, isMap := pag.(map[string]any); isMap {
			if _, ok := pagMap["has_more"]; ok {
				t.Logf("has_more found in pagination object in %s response", toolName)
				return
			}
		}
	}
	t.Errorf("has_more field not found in %s response: %s", toolName, string(raw))
}
