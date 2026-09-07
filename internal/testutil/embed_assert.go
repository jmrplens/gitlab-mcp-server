package testutil

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// EmbedToggle is the signature of the [toolutil.EnableEmbeddedResources]
// setter. testutil reproduces the type locally so callers can drive the
// embedded-resource global flag without importing toolutil (which would
// introduce an import cycle through the tool sub-packages).
type EmbedToggle func(bool)

// AssertEmbeddedResource verifies that an MCP tool behaves correctly under
// both states of the embedded-resource toggle. It invokes the named tool via
// session twice:
//
//  1. With toggle(true) — expects an [*mcp.EmbeddedResource] block whose
//     URI matches wantURI and whose MIME type is "application/json".
//  2. With toggle(false) — expects no EmbeddedResource blocks.
//
// The toggle is always restored to its enabled (production default) state on
// test exit via [testing.T.Cleanup]. The test fails the surrounding run if
// either subtest fails.
//
//nolint:revive // *testing.T is conventionally the first parameter for test helpers.
func AssertEmbeddedResource(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any, wantURI string, toggle EmbedToggle) {
	t.Helper()
	t.Run("enabled by default", func(t *testing.T) {
		toggle(true)
		t.Cleanup(func() { toggle(true) })
		assertResourceEmbedded(ctx, t, session, name, args, wantURI)
	})
	t.Run("disabled produces no embed", func(t *testing.T) {
		toggle(false)
		t.Cleanup(func() { toggle(true) })
		assertResourceNotEmbedded(ctx, t, session, name, args)
	})
}

// embedReporter is the part of [testing.T] these assertions use. It is an
// interface so a tool that answers wrongly can be reported to a recorder, since
// every failure below is one the test exercising it would otherwise take
// itself.
//
// Fatalf does not abort a recorder the way [testing.T.Fatalf] does, so each
// site returns as well.
type embedReporter interface {
	Helper()
	Error(args ...any)
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// assertResourceEmbedded checks that the tool answered with an embedded
// resource carrying wantURI.
func assertResourceEmbedded(ctx context.Context, t embedReporter, session *mcp.ClientSession, name string, args map[string]any, wantURI string) {
	t.Helper()
	result := callToolSuccessfully(ctx, t, session, name, args)
	if result == nil {
		return
	}
	found := firstEmbeddedResource(result)
	if found == nil || found.Resource == nil {
		t.Fatalf("expected EmbeddedResource for %s, got %d blocks", name, len(result.Content))
		return
	}
	assertEmbeddedResourcePayload(t, found.Resource, wantURI)
}

// assertResourceNotEmbedded checks that the tool answered with no embedded
// resource at all, which is what the toggle turned off promises.
func assertResourceNotEmbedded(ctx context.Context, t embedReporter, session *mcp.ClientSession, name string, args map[string]any) {
	t.Helper()
	result := callToolSuccessfully(ctx, t, session, name, args)
	if result == nil {
		return
	}
	if firstEmbeddedResource(result) != nil {
		t.Fatalf("expected no EmbeddedResource when disabled (tool=%s)", name)
	}
}

// callToolSuccessfully invokes an MCP tool and fails the test on transport
// error or IsError=true. It returns the successful [*mcp.CallToolResult], or
// nil when it reported one of those, which a recorder does not abort on.
func callToolSuccessfully(ctx context.Context, t embedReporter, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
		return nil
	}
	if result == nil || result.IsError {
		t.Fatalf("CallTool(%s): expected successful result, got IsError=%v", name, result != nil && result.IsError)
		return nil
	}
	return result
}

// firstEmbeddedResource returns the first [*mcp.EmbeddedResource] in result's
// content slice, or nil if the result contains no embedded resource blocks.
func firstEmbeddedResource(result *mcp.CallToolResult) *mcp.EmbeddedResource {
	for _, content := range result.Content {
		if embedded, ok := content.(*mcp.EmbeddedResource); ok {
			return embedded
		}
	}
	return nil
}

// assertEmbeddedResourcePayload checks that resource has wantURI, MIME type
// "application/json", and a non-empty Text payload. Mismatches are recorded
// via t.Errorf so a single subtest can report every problem at once.
func assertEmbeddedResourcePayload(t embedReporter, resource *mcp.ResourceContents, wantURI string) {
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
