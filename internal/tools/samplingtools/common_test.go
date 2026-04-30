// common_test.go contains unit tests for the samplingtools MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package samplingtools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	testMRTitle          = "feat: add login"
	testModelName        = "test-model"
	mdSectionDescription = "## Description"
	testLoginBug         = "Login bug"
	noteJSONSimple       = `{"id":100,"body":"Looks good to me","author":{"username":"alice"},"system":false,"internal":false,"created_at":"2026-01-15T10:00:00Z","updated_at":"2026-01-15T10:00:00Z"}`
)

// setupSamplingSession creates a connected MCP server+client pair where the
// client supports sampling via a mock createMessage handler. Returns the
// server, server session, and a cleanup function.
func setupSamplingSession(t *testing.T, ctx context.Context) (*mcp.Server, *mcp.ServerSession, func()) {
	t.Helper()

	impl := &mcp.Implementation{Name: "test", Version: "1.0.0"}
	server := mcp.NewServer(impl, nil)
	client := mcp.NewClient(impl, &mcp.ClientOptions{
		CreateMessageHandler: func(_ context.Context, req *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Model:   testModelName,
				Content: &mcp.TextContent{Text: "LLM mock analysis response"},
			}, nil
		},
	})

	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		ss.Close()
		t.Fatalf("client connect: %v", err)
	}

	cleanup := func() {
		cs.Close()
		ss.Close()
	}
	return server, ss, cleanup
}

// TestSamplingUnsupportedResult verifies the SamplingUnsupportedResult output
// when the client does not support sampling.
func TestSamplingUnsupportedResult(t *testing.T) {
	out := SamplingUnsupportedResult("test_tool")
	if out == nil {
		t.Fatal("expected non-nil result")
	}
	contents, ok := out.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(contents.Text, "does not support sampling") {
		t.Errorf("result text = %q, want 'does not support sampling' substring", contents.Text)
	}
	if !strings.Contains(contents.Text, "Alternatives without sampling") {
		t.Error("expected alternative tool suggestions in unsupported message")
	}
	if !out.IsError {
		t.Error("expected IsError = true")
	}
}

// setupFailingSamplingSession creates a connected MCP server+client pair where
// the client's CreateMessageHandler always returns an error, simulating an
// unavailable LLM backend.
func setupFailingSamplingSession(t *testing.T, ctx context.Context) (*mcp.Server, *mcp.ServerSession, func()) {
	t.Helper()

	impl := &mcp.Implementation{Name: "test-fail", Version: "1.0.0"}
	server := mcp.NewServer(impl, nil)
	client := mcp.NewClient(impl, &mcp.ClientOptions{
		CreateMessageHandler: func(_ context.Context, _ *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			return nil, errors.New("LLM unavailable")
		},
	})

	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		ss.Close()
		t.Fatalf("client connect: %v", err)
	}

	cleanup := func() {
		cs.Close()
		ss.Close()
	}
	return server, ss, cleanup
}
