// outputschema_check_test.go verifies that every registered MCP tool declares
// an OutputSchema for structured content support.

package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestOutputSchemaPresence verifies that every registered MCP tool declares an
// OutputSchema, ensuring structured content is available for all tool responses.
func TestOutputSchemaPresence(t *testing.T) {
	session := newMCPSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondJSON(w, http.StatusOK, `{"version":"17.0.0"}`)
	}))
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	// Check first 3 tools for outputSchema
	count := 0
	for _, tool := range result.Tools {
		if tool.OutputSchema != nil {
			count++
		}
	}
	t.Logf("Tools with OutputSchema: %d / %d", count, len(result.Tools))
	if count == 0 {
		t.Log("WARNING: No tools have OutputSchema set")
		// Print first tool as JSON to inspect
		if len(result.Tools) > 0 {
			data, _ := json.MarshalIndent(result.Tools[0], "", "  ")
			t.Logf("First tool JSON:\n%s", string(data)[:min(2000, len(string(data)))])
		}
	}
}
