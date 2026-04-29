//go:build e2e

package suite

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// e2eOAuthConfig holds the configuration needed by OAuth-related E2E tests.
type e2eOAuthConfig struct {
	gitlabURL string
	token     string
	skipTLS   bool
}

// oauthHTTPClient is a shared HTTP client for OAuth E2E tests. It skips TLS
// verification because some self-hosted GitLab instances use self-signed certs.
var oauthHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // E2E test client for self-signed certs
	},
}

// loadE2EConfig loads the OAuth E2E config from environment variables.
// Skips the test if GITLAB_URL or GITLAB_TOKEN are not set.
func loadE2EConfig(t *testing.T) e2eOAuthConfig {
	t.Helper()

	gitlabURL := os.Getenv("GITLAB_URL")
	token := os.Getenv("GITLAB_TOKEN")
	if gitlabURL == "" || token == "" {
		t.Skip("GITLAB_URL and GITLAB_TOKEN required for OAuth E2E tests")
	}

	return e2eOAuthConfig{
		gitlabURL: gitlabURL,
		token:     token,
		skipTLS:   strings.EqualFold(os.Getenv("GITLAB_SKIP_TLS_VERIFY"), "true"),
	}
}

// oauthMCPInitialize sends an MCP initialize request with a Bearer token
// and returns the session ID from the response header.
func oauthMCPInitialize(t *testing.T, serverURL, bearerHeader string) string {
	t.Helper()

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"oauth-e2e-test","version":"1.0"}}}`
	resp := doOAuthMCPRequest(t, serverURL, body, "", bearerHeader)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("initialize: status %d, body: %s", resp.StatusCode, string(b))
	}
	return resp.Header.Get("Mcp-Session-Id")
}

// oauthMCPNotifyInitialized sends the notifications/initialized message with
// a Bearer token.
func oauthMCPNotifyInitialized(t *testing.T, serverURL, sessionID, bearerHeader string) {
	t.Helper()

	body := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	resp := doOAuthMCPRequest(t, serverURL, body, sessionID, bearerHeader)
	resp.Body.Close()
}

// oauthMCPCallTool sends a tools/call request with a Bearer token and returns
// the parsed JSON-RPC result.
func oauthMCPCallTool(t *testing.T, serverURL, sessionID, bearerHeader, tool string, args map[string]any) map[string]any {
	t.Helper()

	argsJSON, _ := json.Marshal(args) //nolint:errchkjson // map[string]any of test inputs cannot fail to marshal
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"%s","arguments":%s}}`, tool, string(argsJSON))
	resp := doOAuthMCPRequest(t, serverURL, body, sessionID, bearerHeader)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("tools/call %s: status %d, body: %s", tool, resp.StatusCode, string(b))
	}

	return parseOAuthJSONRPC(t, resp)
}

// doOAuthMCPRequest sends a raw JSON-RPC request with a Bearer Authorization header.
func doOAuthMCPRequest(t *testing.T, serverURL, body, sessionID, bearerHeader string) *http.Response {
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
	req.Header.Set("Authorization", bearerHeader)

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

// parseOAuthJSONRPC reads and parses a JSON-RPC response, handling both plain
// JSON and SSE (text/event-stream) response formats.
func parseOAuthJSONRPC(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()

	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		return parseOAuthSSE(t, resp.Body)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	var envelope map[string]any
	if unmarshalErr := json.Unmarshal(b, &envelope); unmarshalErr != nil {
		t.Fatalf("unmarshal JSON-RPC response: %v (body: %s)", unmarshalErr, string(b))
	}

	inner, ok := envelope["result"].(map[string]any)
	if !ok {
		t.Fatalf("JSON-RPC response missing 'result' field: %v", envelope)
	}
	return inner
}

// parseOAuthSSE extracts the last JSON-RPC response from an SSE event stream.
func parseOAuthSSE(t *testing.T, r io.Reader) map[string]any {
	t.Helper()

	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read SSE stream: %v", err)
	}

	var lastJSON map[string]any
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var obj map[string]any
		if parseErr := json.Unmarshal([]byte(data), &obj); parseErr == nil {
			if inner, hasResult := obj["result"]; hasResult {
				if m, ok := inner.(map[string]any); ok {
					lastJSON = m
				}
			}
		}
	}

	if lastJSON == nil {
		t.Fatalf("no JSON-RPC result found in SSE stream: %s", string(raw))
	}
	return lastJSON
}
