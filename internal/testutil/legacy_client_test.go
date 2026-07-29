package testutil_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

func TestLegacyClientHandshake(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "s", Version: "1"}, nil)
	ss := testutil.ConnectLegacyElicitationClient(t.Context(), t, server, func(_ context.Context, p *mcp.ElicitParams) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": true}}, nil
	}, testutil.LegacyClientOptions{})
	ip := ss.InitializeParams()
	if ip == nil {
		t.Fatal("InitializeParams nil")
	}
	if ip.ProtocolVersion != "2025-11-25" {
		t.Fatalf("protocol = %q", ip.ProtocolVersion)
	}
	if ip.Capabilities.Elicitation == nil {
		t.Fatal("no elicitation capability")
	}
	res, err := ss.Elicit(context.Background(), &mcp.ElicitParams{Message: "ok?", RequestedSchema: map[string]any{"type": "object"}})
	if err != nil {
		t.Fatalf("Elicit: %v", err)
	}
	if res.Action != "accept" {
		t.Fatalf("action = %q", res.Action)
	}
}
