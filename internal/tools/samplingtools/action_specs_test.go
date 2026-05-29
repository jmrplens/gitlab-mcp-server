// action_specs_test.go contains tests for the analyzeSpec function in action_specs.go.
package samplingtools

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestAnalyzeSpec_EmptyAliasesFallback verifies that analyzeSpec falls back to
// the individualTool name as the only alias when an empty aliases slice is provided.
func TestAnalyzeSpec_EmptyAliasesFallback(t *testing.T) {
	const individualTool = "gitlab_test_tool"
	spec := analyzeSpec("test", toolutil.ActionRoute{}, individualTool, "desc", "usage", nil)
	if !slices.Contains(spec.Aliases, individualTool) {
		t.Errorf("expected Aliases to contain %q when aliases arg is nil, got %v", individualTool, spec.Aliases)
	}
}
