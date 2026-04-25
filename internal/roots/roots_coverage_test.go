package roots

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestClientSupportsRoots_RootsV2Capability verifies that ClientSupportsRoots
// returns true via the modern RootsV2 capability path, not just the legacy
// Roots.ListChanged fallback. The default client used by [setupInMemorySession]
// only populates the legacy Roots field, so this test explicitly opts in to
// RootsV2 to exercise the early-return branch.
func TestClientSupportsRoots_RootsV2Capability(t *testing.T) {
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v0.0.1"}, nil)
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{
			RootsV2: &mcp.RootCapabilities{ListChanged: true},
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()
	})

	if !ClientSupportsRoots(serverSession) {
		t.Error("ClientSupportsRoots() = false with RootsV2 capability set, want true")
	}
}
