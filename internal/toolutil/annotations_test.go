// annotations_test.go contains unit tests for the shared MCP annotation
// presets applied to tool results and resource declarations.
package toolutil

import (
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestResourcePresets_IncludeUserAudience verifies that the resource presets
// address both the user and the model, while the operation presets stay
// assistant-only. The split is load-bearing: the assistant-only audience keeps
// clients from rendering a tool result twice, once from Content and once from
// StructuredContent, but a resource is content a person asked for by URI, so
// hiding it from them describes it wrongly. Collapsing the two sets would
// silently reintroduce one of those two bugs.
func TestResourcePresets_IncludeUserAudience(t *testing.T) {
	resourcePresets := map[string]*mcp.Annotations{
		"ResourceList":   ResourceList,
		"ResourceDetail": ResourceDetail,
	}
	for name, preset := range resourcePresets {
		t.Run(name+" is readable by user and assistant", func(t *testing.T) {
			if !slices.Contains(preset.Audience, mcp.Role("user")) {
				t.Errorf("%s audience = %v, want it to include user", name, preset.Audience)
			}
			if !slices.Contains(preset.Audience, mcp.Role("assistant")) {
				t.Errorf("%s audience = %v, want it to include assistant", name, preset.Audience)
			}
		})
	}

	operationPresets := map[string]*mcp.Annotations{
		"ContentList":   ContentList,
		"ContentDetail": ContentDetail,
		"ContentMutate": ContentMutate,
	}
	for name, preset := range operationPresets {
		t.Run(name+" stays assistant-only", func(t *testing.T) {
			if slices.Contains(preset.Audience, mcp.Role("user")) {
				t.Errorf("%s audience = %v; tool results stay assistant-only to avoid double rendering",
					name, preset.Audience)
			}
		})
	}

	// The priorities stay paired, so the two sets differ only in audience.
	if ResourceList.Priority != ContentList.Priority {
		t.Errorf("ResourceList priority %v != ContentList %v", ResourceList.Priority, ContentList.Priority)
	}
	if ResourceDetail.Priority != ContentDetail.Priority {
		t.Errorf("ResourceDetail priority %v != ContentDetail %v", ResourceDetail.Priority, ContentDetail.Priority)
	}
}
