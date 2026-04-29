//go:build e2e

// identity_test.go tests universal identity propagation across all three MCP
// transport modes (stdio, HTTP legacy, HTTP OAuth). Validates that tool handlers
// always receive the authenticated user's identity via ResolveIdentity regardless
// of transport.
//
// Requires GITLAB_URL and GITLAB_TOKEN environment variables for HTTP-mode tests.
// The stdio test uses in-memory transport with a context-injected identity.
package suite

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/oauth"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestIdentityE2E validates universal identity propagation across all MCP
// transport modes. Each subtest exercises a different mode to verify that
// tool handlers always receive the authenticated user's identity.
//
// TASK-026 (OAuth mode) is covered by the existing TestOAuthE2E/IdentityPropagation
// test in oauth_test.go, which validates the same pipeline with real GitLab.
func TestIdentityE2E(t *testing.T) {
	t.Parallel()

	cfg := loadE2EConfig(t)

	t.Run("StdioPropagation", func(t *testing.T) {
		t.Parallel()
		testStdioIdentityPropagation(t)
	})

	t.Run("HTTPLegacyPropagation", func(t *testing.T) {
		t.Parallel()
		testHTTPLegacyIdentityPropagation(t, cfg)
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

// testHTTPLegacyIdentityPropagation verifies that HTTP legacy mode converts
// PRIVATE-TOKEN to Bearer, validates against real GitLab, and propagates
// identity to tool handlers via req.Extra.TokenInfo.
func testHTTPLegacyIdentityPropagation(t *testing.T, cfg e2eOAuthConfig) {
	t.Helper()

	var capturedIdentity toolutil.UserIdentity

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "identity-legacy-e2e",
		Version: "test",
	}, nil)

	type ProbeInput struct{}
	type ProbeOutput struct{}
	mcp.AddTool(server, &mcp.Tool{Name: "identity_legacy_probe"}, func(_ context.Context, req *mcp.CallToolRequest, _ ProbeInput) (*mcp.CallToolResult, ProbeOutput, error) {
		capturedIdentity = toolutil.ResolveIdentity(context.Background(), req)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
		}, ProbeOutput{}, nil
	})

	verifier := oauth.NewGitLabVerifier(cfg.gitlabURL, cfg.skipTLS, 15*time.Minute, nil)
	authMiddleware := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{})

	handler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
		return server
	}, nil)

	// Legacy mode middleware chain: NormalizeAuthHeader → RequireBearerToken → handler.
	ts := httptest.NewServer(oauth.NormalizeAuthHeader(authMiddleware(handler)))
	defer ts.Close()

	sessionID := identityMCPInitializeWithPrivateToken(t, ts.URL, cfg.token)
	identityMCPNotifyInitializedWithPrivateToken(t, ts.URL, sessionID, cfg.token)
	result := identityMCPCallToolWithPrivateToken(t, ts.URL, sessionID, cfg.token, "identity_legacy_probe", map[string]any{})

	if !capturedIdentity.IsAuthenticated() {
		t.Fatal("expected tool handler to receive authenticated identity from PRIVATE-TOKEN flow")
	}
	if capturedIdentity.UserID == "" {
		t.Error("UserID should not be empty after GitLab validation")
	}
	if capturedIdentity.Username != sess.username {
		t.Errorf("Username = %q, want %q", capturedIdentity.Username, sess.username)
	}

	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("unexpected tool result: %v", result)
	}
	first, _ := content[0].(map[string]any)
	if first["text"] != "ok" {
		t.Errorf("tool result text = %q, want %q", first["text"], "ok")
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

// --- PRIVATE-TOKEN helpers for HTTP legacy mode tests ---

// identityMCPInitializeWithPrivateToken sends an MCP initialize request using
// PRIVATE-TOKEN authentication (HTTP legacy mode) and returns the session ID.
func identityMCPInitializeWithPrivateToken(t *testing.T, serverURL, token string) string {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"identity-e2e-test","version":"1.0"}}}`
	resp := doPrivateTokenMCPRequest(t, serverURL, body, "", token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("initialize: status %d, body: %s", resp.StatusCode, string(b))
	}
	return resp.Header.Get("Mcp-Session-Id")
}

// identityMCPNotifyInitializedWithPrivateToken sends the notifications/initialized
// message using PRIVATE-TOKEN authentication.
func identityMCPNotifyInitializedWithPrivateToken(t *testing.T, serverURL, sessionID, token string) {
	t.Helper()
	body := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	resp := doPrivateTokenMCPRequest(t, serverURL, body, sessionID, token)
	resp.Body.Close()
}

// identityMCPCallToolWithPrivateToken sends a tools/call request using PRIVATE-TOKEN
// authentication and returns the parsed result.
func identityMCPCallToolWithPrivateToken(t *testing.T, serverURL, sessionID, token, tool string, args map[string]any) map[string]any {
	t.Helper()
	argsJSON, _ := json.Marshal(args) //nolint:errchkjson // map[string]any of test inputs cannot fail to marshal
	body := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"` + tool + `","arguments":` + string(argsJSON) + `}}`
	resp := doPrivateTokenMCPRequest(t, serverURL, body, sessionID, token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("tools/call %s: status %d, body: %s", tool, resp.StatusCode, string(b))
	}

	return parseOAuthJSONRPC(t, resp)
}

// doPrivateTokenMCPRequest sends a raw JSON-RPC request with the PRIVATE-TOKEN header.
func doPrivateTokenMCPRequest(t *testing.T, serverURL, body, sessionID, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, serverURL, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}
