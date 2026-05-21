package testutil

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// EmbedToggle is the signature of the toolutil.EnableEmbeddedResources
// setter, declared here so testutil can drive the toggle without importing
// toolutil (which would create an import cycle through other packages).
type EmbedToggle func(bool)

// AssertEmbeddedResource invokes the named tool with args twice: first with
// the embed toggle enabled (expecting an *mcp.EmbeddedResource block whose
// URI matches wantURI and MIME type is application/json), then with the
// toggle disabled (expecting no EmbeddedResource blocks). The toggle is
// always restored to enabled (the production default) on test exit.
//
//nolint:revive // *testing.T is conventionally the first parameter for test helpers.
func AssertEmbeddedResource(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any, wantURI string, toggle EmbedToggle) {
	t.Helper()
	t.Run("enabled by default", func(t *testing.T) {
		toggle(true)
		t.Cleanup(func() { toggle(true) })
		result := callToolSuccessfully(ctx, t, session, name, args)
		found := firstEmbeddedResource(result)
		if found == nil || found.Resource == nil {
			t.Fatalf("expected EmbeddedResource for %s, got %d blocks", name, len(result.Content))
		}
		assertEmbeddedResourcePayload(t, found.Resource, wantURI)
	})
	t.Run("disabled produces no embed", func(t *testing.T) {
		toggle(false)
		t.Cleanup(func() { toggle(true) })
		result := callToolSuccessfully(ctx, t, session, name, args)
		if firstEmbeddedResource(result) != nil {
			t.Fatalf("expected no EmbeddedResource when disabled (tool=%s)", name)
		}
	})
}

func callToolSuccessfully(ctx context.Context, t *testing.T, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	if result == nil || result.IsError {
		t.Fatalf("CallTool(%s): expected successful result, got IsError=%v", name, result != nil && result.IsError)
	}
	return result
}

func firstEmbeddedResource(result *mcp.CallToolResult) *mcp.EmbeddedResource {
	for _, content := range result.Content {
		if embedded, ok := content.(*mcp.EmbeddedResource); ok {
			return embedded
		}
	}
	return nil
}

func assertEmbeddedResourcePayload(t *testing.T, resource *mcp.ResourceContents, wantURI string) {
	t.Helper()
	if resource.URI != wantURI {
		t.Errorf("URI = %q, want %q", resource.URI, wantURI)
	}
	if resource.MIMEType != "application/json" {
		t.Errorf("MIMEType = %q, want application/json", resource.MIMEType)
	}
	if resource.Text == "" {
		t.Error("Text is empty, want JSON payload")
	}
}
