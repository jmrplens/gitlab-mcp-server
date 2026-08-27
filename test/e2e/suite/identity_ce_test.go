//go:build e2e && !enterprise

// identity_ce_test.go tests identity propagation for the transports that carry
// it: stdio, through a context injected at startup, and the verifier's cache
// behind the SDK's bearer middleware as HTTP OAuth mounts it.
//
// Both tests are self-contained — in-memory transport and a mock GitLab — so
// neither needs GITLAB_URL or GITLAB_TOKEN. HTTP OAuth against a real instance
// is covered by TestOAuthE2E/IdentityPropagation.
package suite

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/oauth"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestIdentityE2E validates identity propagation in the modes that have it.
//
// OAuth mode is covered by TestOAuthE2E/IdentityPropagation in oauth_test.go,
// which validates the same pipeline against real GitLab.
//
// HTTP legacy mode has no subtest here because it propagates no identity.
// There used to be one, and it passed by building a middleware chain the
// server does not use: oauth.NormalizeAuthHeader wrapping the SDK's
// RequireBearerToken. Neither is mounted in legacy mode — registerLegacyMCPHandlers
// mounts mcpServerGate alone, which reads the credential straight from the
// header — so req.Extra.TokenInfo is unset there and ResolveIdentity falls
// through to the empty value. The chain the test assembled was removed with
// the adapter; the behavior it claimed to prove was never the server's.
// Rebuilding it truthfully is not possible from this package: the real gate
// lives in package main.
func TestIdentityE2E(t *testing.T) {
	t.Parallel()

	t.Run("StdioPropagation", func(t *testing.T) {
		t.Parallel()
		testStdioIdentityPropagation(t)
	})

	t.Run("CachingVerifier", func(t *testing.T) {
		t.Parallel()
		testCachingVerifierBehavior(t)
	})
}

// testStdioIdentityPropagation verifies that stdio mode propagates identity
// through context: IdentityToContext at startup → ResolveIdentity in handler.
// Uses in-memory MCP transport to simulate stdio without a real GitLab call.
func testStdioIdentityPropagation(t *testing.T) {
	t.Helper()

	var capturedIdentity toolutil.UserIdentity

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "identity-stdio-e2e",
		Version: "test",
	}, nil)

	type ProbeInput struct{}
	type ProbeOutput struct{}
	mcp.AddTool(server, &mcp.Tool{Name: "identity_stdio_probe"}, func(ctx context.Context, req *mcp.CallToolRequest, _ ProbeInput) (*mcp.CallToolResult, ProbeOutput, error) {
		capturedIdentity = toolutil.ResolveIdentity(ctx, req)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
		}, ProbeOutput{}, nil
	})

	// Simulate stdio mode: inject identity into context before server.Run().
	identity := toolutil.UserIdentity{
		UserID:   "12345",
		Username: sess.username,
	}
	identityCtx := toolutil.IdentityToContext(context.Background(), identity)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverCtx, serverCancel := context.WithCancel(identityCtx)
	defer serverCancel()
	go func() {
		_ = server.Run(serverCtx, serverTransport)
	}()

	mcpClient := mcp.NewClient(&mcp.Implementation{
		Name:    "identity-stdio-e2e-client",
		Version: "test",
	}, nil)
	session, err := mcpClient.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	_, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "identity_stdio_probe",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}

	if !capturedIdentity.IsAuthenticated() {
		t.Fatal("expected tool handler to receive authenticated identity via context")
	}
	if capturedIdentity.UserID != "12345" {
		t.Errorf("UserID = %q, want %q", capturedIdentity.UserID, "12345")
	}
	if capturedIdentity.Username != sess.username {
		t.Errorf("Username = %q, want %q", capturedIdentity.Username, sess.username)
	}
}

// testCachingVerifierBehavior verifies that the caching verifier calls GitLab
// API only once for multiple HTTP requests with the same token. Uses a mock
// GitLab endpoint with a call counter to prove cache effectiveness.
func testCachingVerifierBehavior(t *testing.T) {
	t.Helper()

	var apiCallCount atomic.Int32

	mockGitLab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/user" {
			apiCallCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id": 42, "username": "cached-test-user"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer mockGitLab.Close()

	cache := oauth.NewTokenCache()
	verifier := oauth.NewGitLabVerifier(mockGitLab.URL, true, 15*time.Minute, cache)
	authMiddleware := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{})

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "identity-cache-e2e",
		Version: "test",
	}, nil)

	type ProbeInput struct{}
	type ProbeOutput struct{}
	mcp.AddTool(server, &mcp.Tool{Name: "identity_cache_probe"}, func(_ context.Context, req *mcp.CallToolRequest, _ ProbeInput) (*mcp.CallToolResult, ProbeOutput, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
		}, ProbeOutput{}, nil
	})

	handler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
		return server
	}, nil)
	ts := httptest.NewServer(authMiddleware(handler))
	defer ts.Close()

	bearerHeader := "Bearer test-token-for-caching"

	// Three HTTP requests (initialize + notify + callTool) all use the same
	// token. The verifier should call the mock GitLab /api/v4/user exactly
	// once — subsequent requests hit the cache.
	sessionID := oauthMCPInitialize(t, ts.URL, bearerHeader)
	oauthMCPNotifyInitialized(t, ts.URL, sessionID, bearerHeader)
	oauthMCPCallTool(t, ts.URL, sessionID, bearerHeader, "identity_cache_probe", map[string]any{})

	if callCount := apiCallCount.Load(); callCount != 1 {
		t.Errorf("GitLab API called %d times, want 1 (cache should prevent re-validation)", callCount)
	}
}
