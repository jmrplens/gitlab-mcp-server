//go:build e2e

// oauth_test.go tests the full OAuth authorization flow end-to-end against a
// real GitLab instance. Validates that Bearer tokens are verified via the
// GitLab /api/v4/user endpoint, identity is propagated to tool handlers,
// and the Protected Resource Metadata endpoint returns valid RFC 9728 JSON.
//
// Requires GITLAB_URL and GITLAB_TOKEN environment variables.
package suite

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/oauth"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestOAuthE2E validates the OAuth authorization flow against a real GitLab
// instance. Each subtest exercises a different aspect of the OAuth integration.
func TestOAuthE2E(t *testing.T) {
	t.Parallel()

	cfg := loadE2EConfig(t)

	t.Run("IdentityPropagation", func(t *testing.T) {
		t.Parallel()
		testOAuthIdentityPropagation(t, cfg)
	})

	t.Run("ProtectedResourceMetadata", func(t *testing.T) {
		t.Parallel()
		testProtectedResourceMetadata(t, cfg)
	})
}

// testOAuthIdentityPropagation verifies that a valid OAuth Bearer token is
// verified against GitLab and the resulting identity is propagated to tool
// handlers via auth.TokenInfo in the request context.
func testOAuthIdentityPropagation(t *testing.T, cfg e2eOAuthConfig) {
	t.Helper()

	var capturedIdentity toolutil.UserIdentity

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "oauth-e2e",
		Version: "test",
	}, nil)

	type ProbeInput struct{}
	type ProbeOutput struct{}
	mcp.AddTool(server, &mcp.Tool{Name: "oauth_identity_probe"}, func(_ context.Context, req *mcp.CallToolRequest, _ ProbeInput) (*mcp.CallToolResult, ProbeOutput, error) {
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
	ts := httptest.NewServer(authMiddleware(handler))
	defer ts.Close()

	bearerHeader := "Bearer " + cfg.token

	sessionID := oauthMCPInitialize(t, ts.URL, bearerHeader)
	oauthMCPNotifyInitialized(t, ts.URL, sessionID, bearerHeader)
	result := oauthMCPCallTool(t, ts.URL, sessionID, bearerHeader, "oauth_identity_probe", map[string]any{})

	if !capturedIdentity.IsAuthenticated() {
		t.Fatal("expected tool handler to receive authenticated identity from OAuth Bearer flow")
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
	first, ok := content[0].(map[string]any)
	if !ok || first["text"] != "ok" {
		t.Errorf("tool result text = %v, want %q", first["text"], "ok")
	}
}

// testProtectedResourceMetadata verifies that the Protected Resource Metadata
// handler returns valid RFC 9728 JSON when requested via GET.
func testProtectedResourceMetadata(t *testing.T, cfg e2eOAuthConfig) {
	t.Helper()

	handler := oauth.NewProtectedResourceHandler("https://mcp.example.com", cfg.gitlabURL)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var meta map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}

	if resource, ok := meta["resource"].(string); !ok || resource != "https://mcp.example.com" {
		t.Errorf("resource = %v, want %q", meta["resource"], "https://mcp.example.com")
	}

	servers, ok := meta["authorization_servers"].([]any)
	if !ok || len(servers) == 0 {
		t.Fatalf("missing authorization_servers: %v", meta)
	}
	if servers[0] != cfg.gitlabURL {
		t.Errorf("authorization_servers[0] = %v, want %q", servers[0], cfg.gitlabURL)
	}

	methods, ok := meta["bearer_methods_supported"].([]any)
	if !ok || len(methods) == 0 {
		t.Fatal("missing bearer_methods_supported")
	}
	if methods[0] != "header" {
		t.Errorf("bearer_methods_supported[0] = %v, want %q", methods[0], "header")
	}
}
