package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestAssertEmbeddedResource_TogglesEmbeddedContent verifies AssertEmbeddedResource checks enabled and disabled embed states.
func TestAssertEmbeddedResource_TogglesEmbeddedContent(t *testing.T) {
	const resourceURI = "gitlab://test/resources/1"

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "test_embed", Description: "Returns an embedded resource."},
		func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			result := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}
			toolutil.EmbedResourceJSON(result, resourceURI, map[string]any{"id": 1})
			return result, nil, nil
		})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		_ = serverSession.Wait()
	})

	AssertEmbeddedResource(t, ctx, clientSession, "test_embed", map[string]any{}, resourceURI, toolutil.EnableEmbeddedResources)
	if !toolutil.EmbeddedResourcesEnabled() {
		t.Fatal("AssertEmbeddedResource did not restore embedded resources to enabled")
	}
}
