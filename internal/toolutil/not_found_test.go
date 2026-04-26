// not_found_test.go verifies the structured 404 result builder used by
// get-handlers across all domain sub-packages when a resource is not found.

package toolutil

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestNotFoundResult verifies the structured 404 result contains resource info
// and all supplied hints.
func TestNotFoundResult(t *testing.T) {
	result := NotFoundResult("Project", "42", "Use gitlab_project_list to search", "Check permissions")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Error("expected IsError = true")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "Project") {
		t.Errorf("expected resource name in output, got %q", text)
	}
	if !strings.Contains(text, "42") {
		t.Errorf("expected identifier in output, got %q", text)
	}
	if !strings.Contains(text, "gitlab_project_list") {
		t.Errorf("expected hint in output, got %q", text)
	}
}

// TestNotFoundResult_NoHints verifies NotFoundResult works without optional hints.
func TestNotFoundResult_NoHints(t *testing.T) {
	result := NotFoundResult("Branch", "main")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Error("expected IsError = true")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "Branch") || !strings.Contains(text, "main") {
		t.Errorf("unexpected text: %q", text)
	}
}
