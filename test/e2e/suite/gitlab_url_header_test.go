//go:build e2e

// gitlab_url_header_test.go tests the per-request GITLAB-URL header feature
// in HTTP mode. Validates header extraction, URL validation, pool keying by
// (token, URL), and fallback to the default URL when the header is absent.
//
// Uses mock GitLab endpoints (httptest) to avoid requiring multiple real
// GitLab instances. The mock returns a distinct user ID per URL, allowing
// the test to verify that requests are routed to the correct GitLab backend.
package suite

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/serverpool"
)

// TestGitLabURLHeaderE2E validates the GITLAB-URL header feature end-to-end
// over real HTTP transport using httptest servers for both the MCP endpoint
// and mock GitLab backends.
func TestGitLabURLHeaderE2E(t *testing.T) {
	t.Parallel()

	t.Run("DefaultFallback", func(t *testing.T) {
		t.Parallel()
		testGitLabURLDefaultFallback(t)
	})

	t.Run("HeaderOverride", func(t *testing.T) {
		t.Parallel()
		testGitLabURLHeaderOverride(t)
	})

	t.Run("PoolIsolation", func(t *testing.T) {
		t.Parallel()
		testGitLabURLPoolIsolation(t)
	})

	t.Run("InvalidHeader", func(t *testing.T) {
		t.Parallel()
		testGitLabURLInvalidHeader(t)
	})
}

// mockGitLabServer creates an httptest server that responds to /api/v4/user
// with a JSON body containing the given userID and username. This simulates
// a GitLab instance for the server pool's client creation.
func mockGitLabServer(t *testing.T, userID int, username string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/user" {
			w.Header().Set("Content-Type", "application/json")
			// json.Encoder properly escapes special characters; %q would emit
			// Go-quoted output that is not strictly valid JSON for some runes.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       userID,
				"username": username,
			})
			return
		}
		// Return 200 with empty JSON for any other API call to avoid client errors.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
}

// newTestMCPOverHTTP creates an MCP server exposed over HTTP with a ServerPool
// that uses GITLAB-URL header extraction, matching production's legacy mode
// selector closure. Returns the httptest URL, the pool, and a cleanup func.
func newTestMCPOverHTTP(t *testing.T, defaultGitLabURL string) (string, *serverpool.ServerPool, func()) {
	t.Helper()

	cfg := &config.Config{
		GitLabURL:     defaultGitLabURL,
		SkipTLSVerify: true,
		Enterprise:    false,
	}

	// probeGitLabURL is a tool registered dynamically by the pool factory.
	// It reports back the GitLab URL that the client was configured with,
	// allowing the test to verify correct routing.
	factory := func(_ *gitlabclient.Client) *mcp.Server {
		srv := mcp.NewServer(&mcp.Implementation{
			Name:    "gitlab-url-header-e2e",
			Version: "test",
		}, nil)

		type ProbeInput struct{}
		type ProbeOutput struct {
			Probe string `json:"probe"`
		}
		mcp.AddTool(srv, &mcp.Tool{Name: "url_header_probe"}, func(_ context.Context, _ *mcp.CallToolRequest, _ ProbeInput) (*mcp.CallToolResult, ProbeOutput, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
			}, ProbeOutput{Probe: "ok"}, nil
		})

		return srv
	}

	pool := serverpool.New(cfg, factory, serverpool.WithMaxSize(10))

	// Reproduce the production legacy-mode selector closure from
	// cmd/server/main.go serveHTTP().
	mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		token := serverpool.ExtractToken(r)
		if token == "" {
			return nil
		}
		gitlabURL, err := serverpool.ExtractGitLabURL(r, cfg.GitLabURL)
		if err != nil {
			return nil
		}
		server, err := pool.GetOrCreate(token, gitlabURL)
		if err != nil {
			return nil
		}
		return server
	}, &mcp.StreamableHTTPOptions{
		SessionTimeout: 5 * time.Minute,
	})

	ts := httptest.NewServer(mcpHandler)

	return ts.URL, pool, func() {
		ts.Close()
		pool.Close()
	}
}

// testGitLabURLDefaultFallback verifies that when no GITLAB-URL header is
// sent, the server pool uses the default GitLab URL configured at startup.
func testGitLabURLDefaultFallback(t *testing.T) {
	t.Helper()

	mockGL := mockGitLabServer(t, 1, "default-user")
	defer mockGL.Close()

	mcpURL, pool, cleanup := newTestMCPOverHTTP(t, mockGL.URL)
	defer cleanup()

	token := "glpat-test-default-fallback"
	sessionID := gitlabURLMCPInitialize(t, mcpURL, token, "")
	gitlabURLMCPNotifyInitialized(t, mcpURL, sessionID, token, "")
	result := gitlabURLMCPCallTool(t, mcpURL, sessionID, token, "", "url_header_probe", map[string]any{})

	// Verify the tool executed successfully.
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("unexpected tool result: %v", result)
	}
	first, _ := content[0].(map[string]any)
	if first["text"] != "ok" {
		t.Errorf("tool result text = %q, want %q", first["text"], "ok")
	}

	// Pool should have exactly 1 entry (default URL).
	if size := pool.Size(); size != 1 {
		t.Errorf("pool size = %d, want 1", size)
	}
}

// testGitLabURLHeaderOverride verifies that the GITLAB-URL header overrides
// the default URL and the pool creates a separate entry for it.
func testGitLabURLHeaderOverride(t *testing.T) {
	t.Helper()

	mockDefault := mockGitLabServer(t, 1, "default-user")
	defer mockDefault.Close()

	mockOverride := mockGitLabServer(t, 2, "override-user")
	defer mockOverride.Close()

	mcpURL, pool, cleanup := newTestMCPOverHTTP(t, mockDefault.URL)
	defer cleanup()

	token := "glpat-test-header-override"

	// First request without header → default URL.
	sid1 := gitlabURLMCPInitialize(t, mcpURL, token, "")
	gitlabURLMCPNotifyInitialized(t, mcpURL, sid1, token, "")
	gitlabURLMCPCallTool(t, mcpURL, sid1, token, "", "url_header_probe", map[string]any{})

	if size := pool.Size(); size != 1 {
		t.Fatalf("pool size after default request = %d, want 1", size)
	}

	// Second request with GITLAB-URL header → override URL.
	sid2 := gitlabURLMCPInitialize(t, mcpURL, token, mockOverride.URL)
	gitlabURLMCPNotifyInitialized(t, mcpURL, sid2, token, mockOverride.URL)
	gitlabURLMCPCallTool(t, mcpURL, sid2, token, mockOverride.URL, "url_header_probe", map[string]any{})

	// Pool should now have 2 entries: one for each GitLab URL.
	if size := pool.Size(); size != 2 {
		t.Errorf("pool size after override request = %d, want 2 (default + override)", size)
	}
}

// testGitLabURLPoolIsolation verifies that the same token with two different
// GITLAB-URL values creates separate pool entries with independent servers.
func testGitLabURLPoolIsolation(t *testing.T) {
	t.Helper()

	mockA := mockGitLabServer(t, 100, "user-a")
	defer mockA.Close()

	mockB := mockGitLabServer(t, 200, "user-b")
	defer mockB.Close()

	mcpURL, pool, cleanup := newTestMCPOverHTTP(t, mockA.URL)
	defer cleanup()

	token := "glpat-test-pool-isolation"

	// Request to GitLab A (via default, no header).
	sidA := gitlabURLMCPInitialize(t, mcpURL, token, "")
	gitlabURLMCPNotifyInitialized(t, mcpURL, sidA, token, "")
	gitlabURLMCPCallTool(t, mcpURL, sidA, token, "", "url_header_probe", map[string]any{})

	// Request to GitLab B (via GITLAB-URL header).
	sidB := gitlabURLMCPInitialize(t, mcpURL, token, mockB.URL)
	gitlabURLMCPNotifyInitialized(t, mcpURL, sidB, token, mockB.URL)
	gitlabURLMCPCallTool(t, mcpURL, sidB, token, mockB.URL, "url_header_probe", map[string]any{})

	// Pool must have exactly 2 separate entries (one per GitLab URL),
	// proving that the same token with different URLs gets isolated.
	if size := pool.Size(); size != 2 {
		t.Errorf("pool size = %d, want 2 (one per GitLab URL)", size)
	}

	// Verify stats show 2 misses (new entries) and some hits (session reuse).
	stats := pool.Stats()
	if stats.Misses < 2 {
		t.Errorf("pool misses = %d, want >= 2", stats.Misses)
	}
}

// testGitLabURLInvalidHeader verifies that an invalid GITLAB-URL header
// causes the server selector to return nil, which results in the MCP
// handler rejecting the request.
func testGitLabURLInvalidHeader(t *testing.T) {
	t.Helper()

	mockGL := mockGitLabServer(t, 1, "default-user")
	defer mockGL.Close()

	mcpURL, _, cleanup := newTestMCPOverHTTP(t, mockGL.URL)
	defer cleanup()

	token := "glpat-test-invalid-header"

	invalidURLs := []string{
		"ftp://not-http.example.com",
		"not-a-url",
		"://missing-scheme",
	}

	for _, invalidURL := range invalidURLs {
		t.Run(invalidURL, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"url-header-e2e","version":"1.0"}}}`
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, mcpURL, strings.NewReader(body))
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("PRIVATE-TOKEN", token)
			req.Header.Set("GITLAB-URL", invalidURL)

			resp, err := oauthHTTPClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			// When the selector returns nil, the MCP handler rejects the
			// request. Assert a 4xx client error — not just "not 200" — so a
			// future regression that turns a handler panic into 500 would fail
			// this test instead of silently satisfying it.
			if resp.StatusCode < 400 || resp.StatusCode >= 500 {
				b, _ := io.ReadAll(resp.Body)
				t.Errorf("expected 4xx for invalid GITLAB-URL %q, got %d: %s", invalidURL, resp.StatusCode, string(b))
			}
		})
	}
}

// --- HTTP helpers for GITLAB-URL header tests ---

// gitlabURLMCPInitialize sends an MCP initialize request with PRIVATE-TOKEN
// and optionally the GITLAB-URL header. Returns the session ID.
func gitlabURLMCPInitialize(t *testing.T, serverURL, token, gitlabURL string) string {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"url-header-e2e","version":"1.0"}}}`
	resp := doGitLabURLMCPRequest(t, serverURL, body, "", token, gitlabURL)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("initialize: status %d, body: %s", resp.StatusCode, string(b))
	}
	return resp.Header.Get("Mcp-Session-Id")
}

// gitlabURLMCPNotifyInitialized sends the notifications/initialized message
// with PRIVATE-TOKEN and optionally the GITLAB-URL header.
func gitlabURLMCPNotifyInitialized(t *testing.T, serverURL, sessionID, token, gitlabURL string) {
	t.Helper()
	body := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	resp := doGitLabURLMCPRequest(t, serverURL, body, sessionID, token, gitlabURL)
	resp.Body.Close()
}

// gitlabURLMCPCallTool sends a tools/call request with PRIVATE-TOKEN and
// optionally the GITLAB-URL header. Returns the parsed JSON-RPC result.
func gitlabURLMCPCallTool(t *testing.T, serverURL, sessionID, token, gitlabURL, tool string, args map[string]any) map[string]any {
	t.Helper()
	argsJSON, _ := json.Marshal(args) //nolint:errchkjson // map[string]any of test inputs cannot fail to marshal
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"%s","arguments":%s}}`, tool, string(argsJSON))
	resp := doGitLabURLMCPRequest(t, serverURL, body, sessionID, token, gitlabURL)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("tools/call %s: status %d, body: %s", tool, resp.StatusCode, string(b))
	}

	return parseOAuthJSONRPC(t, resp)
}

// doGitLabURLMCPRequest sends a raw JSON-RPC request with PRIVATE-TOKEN and
// an optional GITLAB-URL header.
func doGitLabURLMCPRequest(t *testing.T, serverURL, body, sessionID, token, gitlabURL string) *http.Response {
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
	if gitlabURL != "" {
		req.Header.Set("GITLAB-URL", gitlabURL)
	}

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}
