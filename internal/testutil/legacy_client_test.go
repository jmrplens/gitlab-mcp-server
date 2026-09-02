package testutil_test

import (
	"context"
	"strings"
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

// TestConnectLegacyElicitationClient_AnswersEveryServerInitiatedRequest covers
// what the fake client sends back for requests other than the elicitation the
// test under it cares about.
//
// Both answers matter to whoever uses this helper. A ping must be acknowledged,
// or a server with keepalive enabled closes the session mid-test and the
// failure looks like the code under test; any other request must come back as
// MethodNotFound rather than as silence, because an unanswered request leaves
// the server blocked until its own timeout, which presents as a hang with no
// message.
func TestConnectLegacyElicitationClient_AnswersEveryServerInitiatedRequest(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	ss := testutil.ConnectLegacyElicitationClient(t.Context(), t, server,
		func(context.Context, *mcp.ElicitParams) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "accept"}, nil
		}, testutil.LegacyClientOptions{})

	if err := ss.Ping(t.Context(), nil); err != nil {
		t.Errorf("the fake client did not acknowledge a ping: %v", err)
	}

	// Sampling is deprecated by SEP-2577 and still the clearest server-initiated
	// request this helper does not implement, which is the case under test: what
	// it answers for a method it has no handler for.
	//nolint:staticcheck // SA1019: the deprecation is the reason this is a good stand-in
	_, err := ss.CreateMessage(t.Context(), &mcp.CreateMessageParams{
		Messages:  []*mcp.SamplingMessage{{Role: "user", Content: &mcp.TextContent{Text: "hello"}}},
		MaxTokens: 16,
	})
	if err == nil {
		t.Fatal("the fake client answered a method it does not implement")
	}
	if !strings.Contains(err.Error(), "sampling/createMessage") && !strings.Contains(err.Error(), "not supported") {
		t.Errorf("error = %v, want it to name the unsupported method", err)
	}
}
