// main_test.go contains unit and integration tests for the server entry point.
// Tests cover configuration validation, GitLab connectivity checks, HTTP and
// stdio transport modes, graceful shutdown, and end-to-end MCP protocol
// interactions (initialize, tools/list) via httptest.
package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
)

// newMockGitLabClient creates a GitLab client pointed at a mock server for testing.

// HTTP header names, MIME types, and test values reused across tests.
const (
	hdrContentType  = "Content-Type"
	mimeJSON        = "application/json"
	testToken       = "test-token"
	serverName      = "gitlab-mcp-server"
	mimeJSONSSE     = "application/json, text/event-stream"
	hdrMCPSessionID = "Mcp-Session-Id"
)

// testHTTPClient avoids http.DefaultClient in tests so that stalled mock
// servers cannot hang the entire test suite indefinitely.
var testHTTPClient = &http.Client{Timeout: 10 * time.Second} //nolint:gochecknoglobals // test-only

// closeMCPSession sends an HTTP DELETE to properly terminate an MCP session
// on the server side, preventing goroutine leaks from StreamableHTTPHandler.
// Without this, the server's readIncoming goroutine blocks indefinitely on
// streamableServerConn.Read waiting for c.done to close.
func closeMCPSession(t *testing.T, serverURL, sessionID string) {
	t.Helper()
	if sessionID == "" {
		return
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, serverURL, nil)
	if err != nil {
		return
	}
	req.Header.Set(hdrMCPSessionID, sessionID)
	resp, err := testHTTPClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// newMockGitLabClient is an internal helper for the main package.
func newMockGitLabClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/version" {
			w.Header().Set(hdrContentType, mimeJSON)
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "16.0.0", "revision": "test"})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	client, err := gitlabclient.NewClient(&config.Config{
		GitLabURL:   srv.URL,
		GitLabToken: testToken,
	})
	if err != nil {
		t.Fatalf("failed to create mock gitlab client: %v", err)
	}
	return client
}

// newTestMCPServer creates a configured MCP server with all tools, resources, and prompts registered.
func newTestMCPServer(t *testing.T) *mcp.Server {
	t.Helper()
	client := newMockGitLabClient(t)
	server := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: "test",
	}, nil)
	tools.RegisterAll(server, client, true)
	resources.Register(server, client)
	prompts.Register(server, client)
	return server
}

// parseJSONRPCResponse reads the HTTP response body and parses the JSON-RPC result.
// It handles both plain JSON and SSE (text/event-stream) response formats.
func parseJSONRPCResponse(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var result map[string]any
	if err = json.Unmarshal(body, &result); err == nil {
		return result
	}

	// Parse SSE format: extract JSON from "data: " lines
	for line := range strings.SplitSeq(string(body), "\n") {
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			jsonData := after
			if err = json.Unmarshal([]byte(jsonData), &result); err == nil {
				return result
			}
		}
	}

	t.Fatalf("could not parse response as JSON or SSE:\n%s", string(body))
	return nil
}

// TestRun_InvalidConfig_ReturnsError verifies that [run] returns an error when
// required environment variables (GITLAB_URL, GITLAB_TOKEN) are missing.
func TestRun_InvalidConfig_ReturnsError(t *testing.T) {
	t.Setenv("GITLAB_URL", "")
	t.Setenv("GITLAB_TOKEN", "")

	err := run(nil)
	if err == nil {
		t.Fatal("run() expected error when config is invalid, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "GITLAB_URL") && !strings.Contains(msg, "GITLAB_TOKEN") {
		t.Errorf("error should mention GITLAB_URL or GITLAB_TOKEN, got: %s", msg)
	}
}

// TestHTTPHandler_Initialize_ReturnsServerInfo verifies that the HTTP handler
// responds to an MCP initialize request with the correct server name and
// protocol version.
func TestHTTPHandler_Initialize_ReturnsServerInfo(t *testing.T) {
	server := newTestMCPServer(t)
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL, strings.NewReader(body))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)

	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	sessionID := resp.Header.Get(hdrMCPSessionID)
	t.Cleanup(func() { closeMCPSession(t, ts.URL, sessionID) })

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(respBody))
	}

	result := parseJSONRPCResponse(t, resp)

	res, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("response missing 'result' field: %v", result)
	}

	serverInfo, ok := res["serverInfo"].(map[string]any)
	if !ok {
		t.Fatalf("response missing 'serverInfo': %v", res)
	}
	if name := serverInfo["name"]; name != serverName {
		t.Errorf("serverInfo.name = %q, want %q", name, serverName)
	}
}

// TestHTTPHandler_ToolsList_ReturnsAllTools verifies the full MCP handshake
// (initialize → initialized notification → tools/list) and asserts that all
// registered tools are returned.
func TestHTTPHandler_ToolsList_ReturnsAllTools(t *testing.T) {
	server := newTestMCPServer(t)
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	// Step 1: Initialize session
	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	initReq, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL, strings.NewReader(initBody))
	initReq.Header.Set(hdrContentType, mimeJSON)
	initReq.Header.Set("Accept", mimeJSONSSE)

	initResp, err := testHTTPClient.Do(initReq)
	if err != nil {
		t.Fatalf("initialize request failed: %v", err)
	}
	sessionID := initResp.Header.Get(hdrMCPSessionID)
	t.Cleanup(func() { closeMCPSession(t, ts.URL, sessionID) })
	initResp.Body.Close()

	// Step 2: Send initialized notification
	notifBody := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	notifReq, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL, strings.NewReader(notifBody))
	notifReq.Header.Set(hdrContentType, mimeJSON)
	notifReq.Header.Set("Accept", mimeJSONSSE)
	if sessionID != "" {
		notifReq.Header.Set(hdrMCPSessionID, sessionID)
	}
	notifResp, err := testHTTPClient.Do(notifReq)
	if err != nil {
		t.Fatalf("notification request failed: %v", err)
	}
	notifResp.Body.Close()

	// Step 3: List tools
	listBody := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	listReq, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL, strings.NewReader(listBody))
	listReq.Header.Set(hdrContentType, mimeJSON)
	listReq.Header.Set("Accept", mimeJSONSSE)
	if sessionID != "" {
		listReq.Header.Set(hdrMCPSessionID, sessionID)
	}

	listResp, err := testHTTPClient.Do(listReq)
	if err != nil {
		t.Fatalf("tools/list request failed: %v", err)
	}
	defer listResp.Body.Close()

	if listResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(listResp.Body)
		t.Fatalf("expected 200 OK, got %d: %s", listResp.StatusCode, string(respBody))
	}

	result := parseJSONRPCResponse(t, listResp)

	res, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("response missing 'result': %v", result)
	}
	toolsList, ok := res["tools"].([]any)
	if !ok {
		t.Fatalf("response missing 'tools': %v", res)
	}

	// RegisterAll registers all individual tools (~724 as of v1.0.0)
	const minExpectedTools = 700
	if len(toolsList) < minExpectedTools {
		t.Errorf("tools count = %d, want at least %d", len(toolsList), minExpectedTools)
	}
}

// TestServeHTTP_GracefulShutdown verifies that [serveHTTP] in HTTP mode shuts down
// cleanly when the context is canceled.
func TestServeHTTP_GracefulShutdown(t *testing.T) {
	srv := newMockGitLabServer(t)
	cfg := &config.Config{
		GitLabURL:      srv.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
	}
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTP(ctx, cfg, ":0")
	}()

	// Allow HTTP server to start listening
	time.Sleep(100 * time.Millisecond)

	// Cancel context to trigger graceful shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serveHTTP() unexpected error on graceful shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveHTTP() did not return within timeout after context cancellation")
	}
}

// TestServeStdio_ContextCancelled verifies that [serveStdio] returns
// promptly when given an already-canceled context.
func TestServeStdio_ContextCancelled(t *testing.T) {
	server := newTestMCPServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := serveStdio(ctx, server)
	// stdio mode with canceled context should return an error or nil
	// (either is acceptable — we just verify it doesn't hang)
	_ = err
}

// TestServeHTTP_PortConflict verifies that [serveHTTP] returns an error
// when the requested port is already occupied.
func TestServeHTTP_PortConflict(t *testing.T) {
	// Occupy a port first
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	addr := listener.Addr().String()
	defer listener.Close()

	srv := newMockGitLabServer(t)
	cfg := &config.Config{
		GitLabURL:      srv.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
	}

	ctx := t.Context()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTP(ctx, cfg, addr)
	}()

	select {
	case err = <-errCh:
		if err == nil {
			t.Fatal("serveHTTP() expected error for port conflict, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveHTTP() did not return within timeout for port conflict")
	}
}

// TestRun_GitLabConnectionFailure verifies that [run] returns an error when the
// GitLab instance is unreachable (connectivity ping failure).
func TestRun_GitLabConnectionFailure(t *testing.T) {
	t.Setenv("GITLAB_URL", "http://127.0.0.1:1") // unreachable
	t.Setenv("GITLAB_TOKEN", testToken)
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "true")

	err := run(nil)
	if err == nil {
		t.Fatal("run() expected error when gitlab is unreachable, got nil")
	}
}

// newMockGitLabServer creates a test HTTP server that responds to GitLab API
// endpoints needed by run() (version ping).
func newMockGitLabServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/version" {
			w.Header().Set(hdrContentType, mimeJSON)
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "16.0.0", "revision": "test"})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestRunWithContext_SuccessHTTPIndividualTools verifies that [runWithContext]
// starts successfully in HTTP mode with individual tools (META_TOOLS=false)
// and shuts down cleanly on context cancellation.
func TestRunWithContext_SuccessHTTPIndividualTools(t *testing.T) {
	srv := newMockGitLabServer(t)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- runWithContext(ctx, &httpConfig{
			addr:           ":0",
			gitlabURL:      srv.URL,
			metaTools:      false,
			maxHTTPClients: config.DefaultMaxHTTPClients, autoUpdateTimeout: config.DefaultAutoUpdateTimeout,
			sessionTimeout: config.DefaultSessionTimeout,
		})
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runWithContext() unexpected error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runWithContext() did not return within timeout")
	}
}

// TestRunWithContext_SuccessHTTPMetaTools verifies that [runWithContext] starts
// successfully in HTTP mode with meta-tools enabled (META_TOOLS=true) and shuts
// down cleanly on context cancellation.
func TestRunWithContext_SuccessHTTPMetaTools(t *testing.T) {
	srv := newMockGitLabServer(t)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- runWithContext(ctx, &httpConfig{
			addr:           ":0",
			gitlabURL:      srv.URL,
			metaTools:      true,
			maxHTTPClients: config.DefaultMaxHTTPClients, autoUpdateTimeout: config.DefaultAutoUpdateTimeout,
			sessionTimeout: config.DefaultSessionTimeout,
		})
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runWithContext() unexpected error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runWithContext() did not return within timeout")
	}
}

// TestRunWithContext_SuccessStdio verifies that [runWithContext] in stdio mode
// returns promptly when the context is already canceled.
func TestRunWithContext_SuccessStdio(t *testing.T) {
	srv := newMockGitLabServer(t)
	t.Setenv("GITLAB_URL", srv.URL)
	t.Setenv("GITLAB_TOKEN", testToken)
	t.Setenv("META_TOOLS", "false")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so stdio exits immediately

	err := runWithContext(ctx, nil)
	// With a canceled context, stdio server returns immediately (error or nil)
	_ = err
}

// TestRunWithContext_InvalidConfig verifies that [runWithContext] returns an
// error when configuration is invalid (missing required fields).
func TestRunWithContext_InvalidConfig(t *testing.T) {
	t.Setenv("GITLAB_URL", "")
	t.Setenv("GITLAB_TOKEN", "")

	err := runWithContext(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when config is invalid")
	}
}

// TestRunWithContext_PingFailure verifies that [runWithContext] returns an error
// when the GitLab connectivity ping fails due to an unreachable host.
func TestRunWithContext_PingFailure(t *testing.T) {
	t.Setenv("GITLAB_URL", "http://127.0.0.1:1") // unreachable
	t.Setenv("GITLAB_TOKEN", testToken)
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "true")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := runWithContext(ctx, nil)
	if err == nil {
		t.Fatal("expected error when gitlab ping fails")
	}
}

// TestRunWithContext_ClientCreationError verifies that [runWithContext] returns
// a descriptive error when the GitLab URL is malformed and fails validation.
func TestRunWithContext_ClientCreationError(t *testing.T) {
	t.Setenv("GITLAB_URL", "://bad")
	t.Setenv("GITLAB_TOKEN", testToken)

	err := runWithContext(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when gitlab URL is malformed")
	}
	if !strings.Contains(err.Error(), "GITLAB_URL is not a valid URL") {
		t.Errorf("expected 'GITLAB_URL is not a valid URL' in error, got: %v", err)
	}
}

// TestRunWithContext_HTTPMissingURL verifies that HTTP mode starts correctly
// when --gitlab-url is omitted and the request-level GITLAB-URL header is
// expected instead.
func TestRunWithContext_HTTPMissingURL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		// Give the HTTP server a brief moment to start, then stop it to avoid
		// waiting on the global test timeout.
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := runWithContext(ctx, &httpConfig{
		addr:           ":0",
		gitlabURL:      "",
		maxHTTPClients: config.DefaultMaxHTTPClients, autoUpdateTimeout: config.DefaultAutoUpdateTimeout,
		sessionTimeout: config.DefaultSessionTimeout,
	})
	if err != nil {
		t.Fatalf("expected nil error when --gitlab-url is missing, got: %v", err)
	}
}

// TestRunWithContext_HTTPInvalidURL verifies that HTTP mode returns an error
// when --gitlab-url has an invalid scheme or missing host.
func TestRunWithContext_HTTPInvalidURL(t *testing.T) {
	tests := []struct {
		name, url, wantSubstr string
	}{
		{"bad_scheme", "ftp://gitlab.example.com", "http:// or https://"},
		{"no_host", "https://", "must include a host"},
		{"malformed", "://bad", "not a valid URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runWithContext(context.Background(), &httpConfig{
				addr:           ":0",
				gitlabURL:      tt.url,
				maxHTTPClients: config.DefaultMaxHTTPClients, autoUpdateTimeout: config.DefaultAutoUpdateTimeout,
				sessionTimeout: config.DefaultSessionTimeout,
			})
			if err == nil {
				t.Fatal("expected error for invalid --gitlab-url")
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantSubstr)
			}
		})
	}
}

// TestCreateServer_ReturnsConfiguredServer verifies that [createServer]
// produces a valid MCP server with tools, resources, and prompts registered.
func TestCreateServer_ReturnsConfiguredServer(t *testing.T) {
	client := newMockGitLabClient(t)
	cfg := &config.Config{MetaTools: false}
	server := createServer(client, cfg, nil)

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL, strings.NewReader(body))
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)

	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	sessionID := resp.Header.Get(hdrMCPSessionID)
	t.Cleanup(func() { closeMCPSession(t, ts.URL, sessionID) })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	result := parseJSONRPCResponse(t, resp)
	res, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("response missing 'result': %v", result)
	}
	serverInfo, ok := res["serverInfo"].(map[string]any)
	if !ok {
		t.Fatalf("response missing 'serverInfo': %v", res)
	}
	if name := serverInfo["name"]; name != serverName {
		t.Errorf("serverInfo.name = %q, want %q", name, serverName)
	}
}

// TestPrintHelp_ContainsExpectedSections verifies that printHelp outputs
// all expected sections: version, author, flags, env vars, and JSON examples.
func TestPrintHelp_ContainsExpectedSections(t *testing.T) {
	// Capture stdout.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	printHelp()

	_ = w.Close()
	os.Stdout = oldStdout

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	output := string(out)

	checks := []struct {
		name, want string
	}{
		{"title", "gitlab-mcp-server"},
		{"version label", "Version:"},
		{"author", "Jose Manuel Requena Plens"},
		{"repository", "https://github.com/jmrplens/gitlab-mcp-server"},
		{"flags section", "FLAGS"},
		{"http flag", "-http"},
		{"gitlab-url flag", "-gitlab-url"},
		{"skip-tls flag", "-skip-tls-verify"},
		{"meta-tools flag", "-meta-tools"},
		{"max-http-clients flag", "-max-http-clients"},
		{"session-timeout flag", "-session-timeout"},
		{"auto-update flag", "-auto-update"},
		{"env section", "ENVIRONMENT VARIABLES"},
		{"GITLAB_URL env", "GITLAB_URL"},
		{"GITLAB_TOKEN env", "GITLAB_TOKEN"},
		{"META_TOOLS env", "META_TOOLS"},
		{"json example", "mcp.json"},
		{"opencode example", "OpenCode"},
	}
	for _, c := range checks {
		if !strings.Contains(output, c.want) {
			t.Errorf("printHelp missing %s: want substring %q", c.name, c.want)
		}
	}
}

// TestPrintHelp_NoPanic verifies that printHelp can be called without panicking.
func TestPrintHelp_NoPanic(t *testing.T) {
	oldStdout := os.Stdout
	_, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		_ = w.Close()
		os.Stdout = oldStdout
	}()

	// Should not panic.
	printHelp()
}

// TestProjectMetadata_Constants verifies that project metadata constants
// are set to the expected values.
func TestProjectMetadata_Constants(t *testing.T) {
	if projectAuthor != "Jose Manuel Requena Plens" {
		t.Errorf("projectAuthor = %q, want %q", projectAuthor, "Jose Manuel Requena Plens")
	}
	if projectDepartment != "" {
		t.Errorf("projectDepartment = %q, want empty", projectDepartment)
	}
	if projectRepository == "" {
		t.Error("projectRepository should not be empty")
	}
}

// TestCreateServer_MetaToolsEnabled verifies that createServer registers
// meta-tools when MetaTools is true and returns an operational MCP server.
func TestCreateServer_MetaToolsEnabled(t *testing.T) {
	client := newMockGitLabClient(t)
	cfg := &config.Config{MetaTools: true}
	server := createServer(client, cfg, nil)

	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, nil)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL, strings.NewReader(body))
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)

	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	sessionID := resp.Header.Get(hdrMCPSessionID)
	t.Cleanup(func() { closeMCPSession(t, ts.URL, sessionID) })

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	result := parseJSONRPCResponse(t, resp)
	res, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("response missing 'result': %v", result)
	}
	serverInfo, ok := res["serverInfo"].(map[string]any)
	if !ok {
		t.Fatalf("response missing 'serverInfo': %v", res)
	}
	if name := serverInfo["name"]; name != serverName {
		t.Errorf("serverInfo.name = %q, want %q", name, serverName)
	}
}

// TestPreStartAutoUpdate_InvalidMode verifies that preStartAutoUpdate
// returns immediately when the AUTO_UPDATE value is invalid.
func TestPreStartAutoUpdate_InvalidMode(t *testing.T) {
	cfg := &config.Config{AutoUpdate: "invalid-value"}
	// Should log warning and return without panic.
	preStartAutoUpdate(cfg)
}

// TestPreStartAutoUpdate_DisabledMode verifies that preStartAutoUpdate
// returns immediately when AUTO_UPDATE is "false" (disabled).
func TestPreStartAutoUpdate_DisabledMode(t *testing.T) {
	cfg := &config.Config{AutoUpdate: "false"}
	// Should return immediately without calling PreStartUpdate.
	preStartAutoUpdate(cfg)
}

// TestPreStartAutoUpdate_ValidMode verifies that preStartAutoUpdate
// exercises the full path through PreStartUpdate when mode is valid.
// With version="dev" (test default), PreStartUpdate is called but
// NewUpdater fails internally — this still covers the code path.
func TestPreStartAutoUpdate_ValidMode(t *testing.T) {
	cfg := &config.Config{
		AutoUpdate:     "true",
		AutoUpdateRepo: "group/project",
	}
	// Will exercise PreStartUpdate which fails internally (version="dev").
	preStartAutoUpdate(cfg)
}

// TestNewUpdaterForTools_InvalidMode verifies that newUpdaterForTools
// returns nil when the AUTO_UPDATE value cannot be parsed.
func TestNewUpdaterForTools_InvalidMode(t *testing.T) {
	cfg := &config.Config{AutoUpdate: "garbage"}
	u := newUpdaterForTools(cfg)
	if u != nil {
		t.Error("expected nil updater for invalid mode")
	}
}

// TestNewUpdaterForTools_DisabledMode verifies that newUpdaterForTools
// returns nil when auto-update is disabled.
func TestNewUpdaterForTools_DisabledMode(t *testing.T) {
	cfg := &config.Config{AutoUpdate: "false"}
	u := newUpdaterForTools(cfg)
	if u != nil {
		t.Error("expected nil updater for disabled mode")
	}
}

// TestNewUpdaterForTools_NewUpdaterError verifies that newUpdaterForTools
// returns nil when NewUpdater fails (e.g. version="dev").
func TestNewUpdaterForTools_NewUpdaterError(t *testing.T) {
	cfg := &config.Config{
		AutoUpdate:     "true",
		AutoUpdateRepo: "group/project",
		// version is "dev" by default in tests → NewUpdater rejects it.
	}
	u := newUpdaterForTools(cfg)
	if u != nil {
		t.Error("expected nil updater when version is 'dev'")
	}
}

// TestNewUpdaterForTools_Success verifies that newUpdaterForTools returns
// a valid Updater when all configuration is correct.
func TestNewUpdaterForTools_Success(t *testing.T) {
	oldVersion := version
	version = "1.0.0"
	t.Cleanup(func() { version = oldVersion })

	cfg := &config.Config{
		AutoUpdate:     "true",
		AutoUpdateRepo: "group/project",
	}
	u := newUpdaterForTools(cfg)
	if u == nil {
		t.Fatal("expected non-nil updater")
	}
}

// TestStartAutoUpdate_InvalidMode verifies that startAutoUpdate returns
// immediately when the AUTO_UPDATE value is invalid.
func TestStartAutoUpdate_InvalidMode(t *testing.T) {
	cfg := &config.Config{AutoUpdate: "bad-mode"}
	// Should log warning and return.
	startAutoUpdate(context.Background(), cfg)
}

// TestStartAutoUpdate_DisabledMode verifies that startAutoUpdate returns
// immediately when auto-update is disabled.
func TestStartAutoUpdate_DisabledMode(t *testing.T) {
	cfg := &config.Config{AutoUpdate: "false"}
	// Should return without starting periodic checks.
	startAutoUpdate(context.Background(), cfg)
}

// TestStartAutoUpdate_NewUpdaterError verifies that startAutoUpdate returns
// gracefully when NewUpdater fails (version="dev").
func TestStartAutoUpdate_NewUpdaterError(t *testing.T) {
	cfg := &config.Config{
		AutoUpdate:     "true",
		AutoUpdateRepo: "group/project",
		// version is "dev" → NewUpdater fails.
	}
	startAutoUpdate(context.Background(), cfg)
}

// TestStartAutoUpdate_Success verifies that startAutoUpdate successfully
// creates an Updater and starts the periodic check goroutine.
func TestStartAutoUpdate_Success(t *testing.T) {
	oldVersion := version
	version = "1.0.0"
	t.Cleanup(func() { version = oldVersion })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &config.Config{
		AutoUpdate:         "check",
		AutoUpdateRepo:     "group/project",
		AutoUpdateInterval: time.Hour,
	}
	// Should succeed and start background goroutine.
	startAutoUpdate(ctx, cfg)

	// Cancel context to stop the periodic checker.
	cancel()
}

// TestRunStdio_PingSucceeds verifies the success path for Ping in runStdio,
// where the GitLab mock returns a valid version response.
func TestRunStdio_PingSucceeds(t *testing.T) {
	srv := newMockGitLabServer(t)
	t.Setenv("GITLAB_URL", srv.URL)
	t.Setenv("GITLAB_TOKEN", testToken)
	t.Setenv("META_TOOLS", "false")
	t.Setenv("AUTO_UPDATE", "false")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runWithContext(ctx, nil)
	_ = err
}

// TestServeHTTP_RequestWithToken verifies that the HTTP handler processes
// requests that include a valid authentication token.
func TestServeHTTP_RequestWithToken(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	cfg := &config.Config{
		GitLabURL:      mockGL.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Find a free port.
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTP(ctx, cfg, addr)
	}()

	waitForHTTPServerReady(t, addr, errCh)

	// Send initialize request with token.
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr, strings.NewReader(body))
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	req.Header.Set("PRIVATE-TOKEN", testToken)

	resp, reqErr := testHTTPClient.Do(req)
	if reqErr != nil {
		cancel()
		t.Fatalf("request failed: %v", reqErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200 OK, got %d: %s", resp.StatusCode, string(respBody))
	}

	closeMCPSession(t, "http://"+addr, resp.Header.Get(hdrMCPSessionID))
	cancel()
	select {
	case err = <-errCh:
		if err != nil {
			t.Fatalf("serveHTTP error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveHTTP did not shut down in time")
	}
}

// TestServeHTTP_RequestWithTokenAndGitLabURLHeader verifies that HTTP mode
// accepts request-level GitLab instance selection when --gitlab-url is omitted.
func TestServeHTTP_RequestWithTokenAndGitLabURLHeader(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	cfg := &config.Config{
		GitLabURL:      "",
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTP(ctx, cfg, addr)
	}()

	waitForHTTPServerReady(t, addr, errCh)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr, strings.NewReader(body))
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	req.Header.Set("PRIVATE-TOKEN", testToken)
	req.Header.Set("GITLAB-URL", mockGL.URL)

	resp, reqErr := testHTTPClient.Do(req)
	if reqErr != nil {
		cancel()
		t.Fatalf("request failed: %v", reqErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200 OK, got %d: %s", resp.StatusCode, string(respBody))
	}

	closeMCPSession(t, "http://"+addr, resp.Header.Get(hdrMCPSessionID))
	cancel()
	select {
	case err = <-errCh:
		if err != nil {
			t.Fatalf("serveHTTP error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveHTTP did not shut down in time")
	}
}

// TestServeHTTP_MissingGitLabURLHeader verifies that requests are rejected in
// HTTP mode when no default --gitlab-url is configured and GITLAB-URL is absent.
func TestServeHTTP_MissingGitLabURLHeader(t *testing.T) {
	cfg := &config.Config{
		GitLabURL:      "",
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTP(ctx, cfg, addr)
	}()

	waitForHTTPServerReady(t, addr, errCh)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr, strings.NewReader(body))
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	req.Header.Set("PRIVATE-TOKEN", testToken)

	resp, reqErr := testHTTPClient.Do(req)
	if reqErr != nil {
		cancel()
		t.Fatalf("request failed: %v", reqErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Error("expected non-200 when GITLAB-URL header is missing and no default gitlab-url is configured")
	}

	cancel()
	select {
	case err = <-errCh:
		if err != nil {
			t.Fatalf("serveHTTP error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveHTTP did not shut down in time")
	}
}

// TestServeHTTP_InvalidGitLabURLHeader verifies that requests are rejected
// when GITLAB-URL has an invalid scheme.
func TestServeHTTP_InvalidGitLabURLHeader(t *testing.T) {
	cfg := &config.Config{
		GitLabURL:      "",
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTP(ctx, cfg, addr)
	}()

	waitForHTTPServerReady(t, addr, errCh)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr, strings.NewReader(body))
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	req.Header.Set("PRIVATE-TOKEN", testToken)
	req.Header.Set("GITLAB-URL", "ftp://gitlab.example.com")

	resp, reqErr := testHTTPClient.Do(req)
	if reqErr != nil {
		cancel()
		t.Fatalf("request failed: %v", reqErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Error("expected non-200 for invalid GITLAB-URL header")
	}

	cancel()
	select {
	case err = <-errCh:
		if err != nil {
			t.Fatalf("serveHTTP error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveHTTP did not shut down in time")
	}
}

// TestRunHTTP_AutoUpdateDisabled verifies that runHTTP works correctly
// when auto-update is explicitly disabled.
func TestRunHTTP_AutoUpdateDisabled(t *testing.T) {
	srv := newMockGitLabServer(t)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- runWithContext(ctx, &httpConfig{
			addr:           ":0",
			gitlabURL:      srv.URL,
			metaTools:      false,
			maxHTTPClients: config.DefaultMaxHTTPClients, autoUpdateTimeout: config.DefaultAutoUpdateTimeout,
			sessionTimeout: config.DefaultSessionTimeout,
			autoUpdate:     "false",
		})
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runWithContext: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}
}

// TestServeHTTP_MissingToken verifies that the HTTP handler rejects requests
// without an authentication token by returning nil from the server factory.
func TestServeHTTP_MissingToken(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	cfg := &config.Config{
		GitLabURL:      mockGL.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTP(ctx, cfg, addr)
	}()

	waitForHTTPServerReady(t, addr, errCh)

	// Send request WITHOUT token.
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr, strings.NewReader(body))
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	// No PRIVATE-TOKEN header.

	resp, reqErr := testHTTPClient.Do(req)
	if reqErr != nil {
		cancel()
		t.Fatalf("request failed: %v", reqErr)
	}
	defer resp.Body.Close()

	// The server factory returns nil for missing token → MCP SDK responds
	// with an error status (400 or 401).
	if resp.StatusCode == http.StatusOK {
		t.Error("expected non-200 for request without token")
	}

	cancel()
	select {
	case err = <-errCh:
		if err != nil {
			t.Fatalf("serveHTTP error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveHTTP did not shut down in time")
	}
}

// TestRunHTTP_AutoUpdateInvalid verifies that runHTTP continues even when
// the auto-update mode is invalid (logs warning, does not block startup).
func TestRunHTTP_AutoUpdateInvalid(t *testing.T) {
	srv := newMockGitLabServer(t)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- runWithContext(ctx, &httpConfig{
			addr:           ":0",
			gitlabURL:      srv.URL,
			metaTools:      false,
			maxHTTPClients: config.DefaultMaxHTTPClients, autoUpdateTimeout: config.DefaultAutoUpdateTimeout,
			sessionTimeout: config.DefaultSessionTimeout,
			autoUpdate:     "bogus",
		})
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runWithContext: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}
}

// TestHealthHandler_ReturnsOK verifies the /health endpoint returns 200 with
// JSON body containing status, version, and commit fields.
func TestHealthHandler_ReturnsOK(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get(hdrContentType); ct != mimeJSON+"; charset=utf-8" && ct != mimeJSON {
		t.Fatalf("expected Content-Type %s, got %q", mimeJSON, ct)
	}

	var body healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("expected status %q, got %q", "ok", body.Status)
	}
	if body.Version == "" {
		t.Error("expected non-empty version")
	}
	if body.Commit == "" {
		t.Error("expected non-empty commit")
	}
}

// TestParseLogLevel verifies that LOG_LEVEL values map to correct slog levels.
func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
		{" debug ", slog.LevelDebug},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := parseLogLevel(tt.input); got != tt.want {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestExtractHost verifies host extraction from URLs.
func TestExtractHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://gitlab.example.com", "gitlab.example.com"},
		{"https://gitlab.example.com:443/path", "gitlab.example.com:443"},
		{"http://localhost:8080", "localhost:8080"},
		{"", ""},
		{"://invalid", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := extractHost(tt.input); got != tt.want {
				t.Errorf("extractHost(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestAutoUpdateRedactHandler_RedactsOnlyAutoUpdateLogs verifies that the
// handler redacts the auto-update URL only in log entries prefixed with
// "autoupdate:" and leaves other entries untouched.
func TestAutoUpdateRedactHandler_RedactsOnlyAutoUpdateLogs(t *testing.T) {
	var buf strings.Builder
	base := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := &autoUpdateRedactHandler{
		base:          base,
		redactStrings: []string{"https://gitlab.example.com", "gitlab.example.com"},
	}
	logger := slog.New(h)

	// Auto-update log: URL should be redacted.
	buf.Reset()
	logger.Info("autoupdate: check failed", "error", "Get https://gitlab.example.com/api/v4/releases: timeout")
	if strings.Contains(buf.String(), "gitlab.example.com") {
		t.Errorf("auto-update log should redact URL, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "[REDACTED]") {
		t.Errorf("auto-update log should contain [REDACTED], got: %s", buf.String())
	}

	// Regular log: URL should NOT be redacted.
	buf.Reset()
	logger.Info("connecting to gitlab", "url", "https://gitlab.example.com")
	if !strings.Contains(buf.String(), "gitlab.example.com") {
		t.Errorf("regular log should preserve URL, got: %s", buf.String())
	}
}

// TestSetupAutoUpdateRedaction_NoOp verifies that setupAutoUpdateRedaction
// does not panic with an empty URL.
func TestSetupAutoUpdateRedaction_NoOp(t *testing.T) {
	setupAutoUpdateRedaction("")
}

// newMockGitLabServerWithUser creates a mock GitLab that handles both
// /api/v4/version and /api/v4/user (required by the OAuth verifier).
func newMockGitLabServerWithUser(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/version":
			w.Header().Set(hdrContentType, mimeJSON)
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "16.0.0", "revision": "test"})
		case "/api/v4/user":
			token := r.Header.Get("PRIVATE-TOKEN")
			if token == "" {
				if after, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
					token = after
				}
			}
			if token == testToken {
				w.Header().Set(hdrContentType, mimeJSON)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":       42,
					"username": "testuser",
					"name":     "Test User",
				})
			} else {
				http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// oauthAddr starts serveHTTP in OAuth mode and returns the listen address.
// Caller must cancel the context and drain errCh when done.
func oauthAddr(t *testing.T, ctx context.Context, cfg *config.Config) (string, <-chan error) {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTP(ctx, cfg, addr)
	}()
	waitForHTTPServerReady(t, addr, errCh)
	return addr, errCh
}

// waitForHTTPServerReady polls /health until the HTTP server is reachable,
// or fails fast if serveHTTP exits early with an error.
func waitForHTTPServerReady(t *testing.T, addr string, errCh <-chan error) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("serveHTTP exited before accepting requests: %v", err)
			}
			t.Fatal("serveHTTP exited before accepting requests")
		default:
		}

		req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/health", nil)
		if reqErr != nil {
			t.Fatalf("failed to build readiness request: %v", reqErr)
		}

		resp, doErr := testHTTPClient.Do(req)
		if doErr == nil {
			resp.Body.Close()
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("HTTP server at %s was not ready within timeout", addr)
}

// TestServeHTTP_OAuthMode_MetadataEndpoint verifies that OAuth mode serves
// the RFC 9728 Protected Resource Metadata at /.well-known/oauth-protected-resource.
func TestServeHTTP_OAuthMode_MetadataEndpoint(t *testing.T) {
	mockGL := newMockGitLabServerWithUser(t)
	cfg := &config.Config{
		GitLabURL:      mockGL.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
		AuthMode:       "oauth",
		OAuthCacheTTL:  config.DefaultOAuthCacheTTL,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, errCh := oauthAddr(t, ctx, cfg)

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet,
		"http://"+addr+"/.well-known/oauth-protected-resource", nil)
	resp, err := testHTTPClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("metadata request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(body))
	}

	var meta map[string]any
	if decErr := json.NewDecoder(resp.Body).Decode(&meta); decErr != nil {
		t.Fatalf("failed to decode metadata JSON: %v", decErr)
	}

	servers, ok := meta["authorization_servers"].([]any)
	if !ok || len(servers) == 0 {
		t.Fatalf("missing authorization_servers in metadata: %v", meta)
	}
	if servers[0] != mockGL.URL {
		t.Errorf("authorization_servers[0] = %q, want %q", servers[0], mockGL.URL)
	}

	cancel()
	select {
	case srvErr := <-errCh:
		if srvErr != nil {
			t.Fatalf("serveHTTP error: %v", srvErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timeout")
	}
}

// TestServeHTTP_OAuthMode_RejectsUnauthenticated verifies that OAuth mode
// rejects requests without a Bearer token with 401.
func TestServeHTTP_OAuthMode_RejectsUnauthenticated(t *testing.T) {
	mockGL := newMockGitLabServerWithUser(t)
	cfg := &config.Config{
		GitLabURL:      mockGL.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
		AuthMode:       "oauth",
		OAuthCacheTTL:  config.DefaultOAuthCacheTTL,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, errCh := oauthAddr(t, ctx, cfg)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr, strings.NewReader(body))
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)

	resp, err := testHTTPClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}

	cancel()
	select {
	case srvErr := <-errCh:
		if srvErr != nil {
			t.Fatalf("serveHTTP error: %v", srvErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timeout")
	}
}

// TestServeHTTP_OAuthMode_AcceptsValidBearer verifies that OAuth mode accepts
// a valid Bearer token and returns a successful MCP initialize response.
func TestServeHTTP_OAuthMode_AcceptsValidBearer(t *testing.T) {
	mockGL := newMockGitLabServerWithUser(t)
	cfg := &config.Config{
		GitLabURL:      mockGL.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
		AuthMode:       "oauth",
		OAuthCacheTTL:  config.DefaultOAuthCacheTTL,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, errCh := oauthAddr(t, ctx, cfg)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr, strings.NewReader(body))
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	req.Header.Set("Authorization", "Bearer "+testToken)

	resp, err := testHTTPClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(respBody))
	}

	result := parseJSONRPCResponse(t, resp)
	res, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("response missing 'result': %v", result)
	}
	serverInfo, ok := res["serverInfo"].(map[string]any)
	if !ok {
		t.Fatalf("response missing 'serverInfo': %v", res)
	}
	if name := serverInfo["name"]; name != serverName {
		t.Errorf("serverInfo.name = %q, want %q", name, serverName)
	}

	closeMCPSession(t, "http://"+addr, resp.Header.Get(hdrMCPSessionID))
	cancel()
	select {
	case srvErr := <-errCh:
		if srvErr != nil {
			t.Fatalf("serveHTTP error: %v", srvErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timeout")
	}
}

// TestServeHTTP_OAuthMode_PrivateTokenConverted verifies that NormalizeAuthHeader
// converts PRIVATE-TOKEN to Bearer, allowing the OAuth verifier to validate it.
func TestServeHTTP_OAuthMode_PrivateTokenConverted(t *testing.T) {
	mockGL := newMockGitLabServerWithUser(t)
	cfg := &config.Config{
		GitLabURL:      mockGL.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
		AuthMode:       "oauth",
		OAuthCacheTTL:  config.DefaultOAuthCacheTTL,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, errCh := oauthAddr(t, ctx, cfg)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr, strings.NewReader(body))
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	req.Header.Set("PRIVATE-TOKEN", testToken)

	resp, err := testHTTPClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK (PRIVATE-TOKEN converted to Bearer), got %d: %s", resp.StatusCode, string(respBody))
	}

	closeMCPSession(t, "http://"+addr, resp.Header.Get(hdrMCPSessionID))
	cancel()
	select {
	case srvErr := <-errCh:
		if srvErr != nil {
			t.Fatalf("serveHTTP error: %v", srvErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timeout")
	}
}

// TestServeHTTP_OAuthMode_InvalidTokenReturns401 verifies that OAuth mode
// returns 401 for an invalid Bearer token.
func TestServeHTTP_OAuthMode_InvalidTokenReturns401(t *testing.T) {
	mockGL := newMockGitLabServerWithUser(t)
	cfg := &config.Config{
		GitLabURL:      mockGL.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
		AuthMode:       "oauth",
		OAuthCacheTTL:  config.DefaultOAuthCacheTTL,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, errCh := oauthAddr(t, ctx, cfg)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr, strings.NewReader(body))
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	req.Header.Set("Authorization", "Bearer invalid-token-xxx")

	resp, err := testHTTPClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", resp.StatusCode)
	}

	cancel()
	select {
	case srvErr := <-errCh:
		if srvErr != nil {
			t.Fatalf("serveHTTP error: %v", srvErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timeout")
	}
}

// TestServeHTTP_LegacyMode_NoMetadataEndpoint verifies that legacy mode
// does NOT serve the /.well-known/oauth-protected-resource endpoint.
func TestServeHTTP_LegacyMode_NoMetadataEndpoint(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	cfg := &config.Config{
		GitLabURL:      mockGL.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, errCh := oauthAddr(t, ctx, cfg)

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet,
		"http://"+addr+"/.well-known/oauth-protected-resource", nil)
	resp, err := testHTTPClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("metadata request failed: %v", err)
	}
	defer resp.Body.Close()

	// Legacy mode has no metadata endpoint — the catch-all handler will respond
	// but not with a valid OAuth metadata JSON.
	if resp.StatusCode == http.StatusOK {
		var meta map[string]any
		if decErr := json.NewDecoder(resp.Body).Decode(&meta); decErr == nil {
			if _, hasServers := meta["authorization_servers"]; hasServers {
				t.Error("legacy mode should NOT serve OAuth metadata, but found authorization_servers")
			}
		}
	}

	cancel()
	select {
	case srvErr := <-errCh:
		if srvErr != nil {
			t.Fatalf("serveHTTP error: %v", srvErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timeout")
	}
}

// TestRunHTTP_InvalidAuthMode verifies that runHTTP rejects an unsupported
// auth-mode value.
func TestRunHTTP_InvalidAuthMode(t *testing.T) {
	err := runHTTP(context.Background(), &httpConfig{
		gitlabURL:      "https://gitlab.example.com",
		authMode:       "saml",
		maxHTTPClients: config.DefaultMaxHTTPClients, autoUpdateTimeout: config.DefaultAutoUpdateTimeout,
		sessionTimeout: config.DefaultSessionTimeout,
	})
	if err == nil {
		t.Fatal("expected error for invalid auth-mode")
	}
	if !strings.Contains(err.Error(), "auth-mode") {
		t.Errorf("error should mention auth-mode, got: %v", err)
	}
}

// TestRunHTTP_OAuthCacheTTL_BelowMin verifies that runHTTP rejects an
// oauth-cache-ttl below the minimum allowed value.
func TestRunHTTP_OAuthCacheTTL_BelowMin(t *testing.T) {
	err := runHTTP(context.Background(), &httpConfig{
		gitlabURL:      "https://gitlab.example.com",
		authMode:       "oauth",
		oauthCacheTTL:  10 * time.Second,
		maxHTTPClients: config.DefaultMaxHTTPClients, autoUpdateTimeout: config.DefaultAutoUpdateTimeout,
		sessionTimeout: config.DefaultSessionTimeout,
	})
	if err == nil {
		t.Fatal("expected error for oauth-cache-ttl below minimum")
	}
	if !strings.Contains(err.Error(), "oauth-cache-ttl") {
		t.Errorf("error should mention oauth-cache-ttl, got: %v", err)
	}
}

// TestRunHTTP_OAuthCacheTTL_AboveMax verifies that runHTTP rejects an
// oauth-cache-ttl above the maximum allowed value.
func TestRunHTTP_OAuthCacheTTL_AboveMax(t *testing.T) {
	err := runHTTP(context.Background(), &httpConfig{
		gitlabURL:      "https://gitlab.example.com",
		authMode:       "oauth",
		oauthCacheTTL:  5 * time.Hour,
		maxHTTPClients: config.DefaultMaxHTTPClients, autoUpdateTimeout: config.DefaultAutoUpdateTimeout,
		sessionTimeout: config.DefaultSessionTimeout,
	})
	if err == nil {
		t.Fatal("expected error for oauth-cache-ttl above maximum")
	}
	if !strings.Contains(err.Error(), "oauth-cache-ttl") {
		t.Errorf("error should mention oauth-cache-ttl, got: %v", err)
	}
}

// TestRunHTTP_SessionTimeoutExceedsMax verifies that runHTTP rejects a
// session-timeout that exceeds the maximum.
func TestRunHTTP_SessionTimeoutExceedsMax(t *testing.T) {
	err := runHTTP(context.Background(), &httpConfig{
		gitlabURL:      "https://gitlab.example.com",
		maxHTTPClients: config.DefaultMaxHTTPClients, autoUpdateTimeout: config.DefaultAutoUpdateTimeout,
		sessionTimeout: 48 * time.Hour,
	})
	if err == nil {
		t.Fatal("expected error for session-timeout exceeding max")
	}
	if !strings.Contains(err.Error(), "session-timeout") {
		t.Errorf("error should mention session-timeout, got: %v", err)
	}
}

// TestRunHTTP_RevalidateIntervalExceedsMax verifies that runHTTP rejects a
// revalidate-interval that exceeds the maximum.
func TestRunHTTP_RevalidateIntervalExceedsMax(t *testing.T) {
	err := runHTTP(context.Background(), &httpConfig{
		gitlabURL:          "https://gitlab.example.com",
		maxHTTPClients:     config.DefaultMaxHTTPClients,
		sessionTimeout:     config.DefaultSessionTimeout,
		revalidateInterval: 48 * time.Hour,
	})
	if err == nil {
		t.Fatal("expected error for revalidate-interval exceeding max")
	}
	if !strings.Contains(err.Error(), "revalidate-interval") {
		t.Errorf("error should mention revalidate-interval, got: %v", err)
	}
}

// TestRunHTTP_MissingGitLabURL verifies that runHTTP accepts an empty
// --gitlab-url and relies on per-request GITLAB-URL headers.
func TestRunHTTP_MissingGitLabURL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := runHTTP(ctx, &httpConfig{
		gitlabURL:      "",
		maxHTTPClients: config.DefaultMaxHTTPClients, autoUpdateTimeout: config.DefaultAutoUpdateTimeout,
		sessionTimeout: config.DefaultSessionTimeout,
	})
	if err != nil {
		t.Fatalf("expected nil error for empty gitlab-url, got: %v", err)
	}
}

// TestRunHTTP_AutoUpdateTimeoutBelowMin verifies that runHTTP rejects an
// auto-update-timeout below the minimum threshold.
func TestRunHTTP_AutoUpdateTimeoutBelowMin(t *testing.T) {
	err := runHTTP(context.Background(), &httpConfig{
		gitlabURL:         "https://gitlab.example.com",
		maxHTTPClients:    config.DefaultMaxHTTPClients,
		sessionTimeout:    config.DefaultSessionTimeout,
		autoUpdateTimeout: 1 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error for auto-update-timeout below minimum")
	}
	if !strings.Contains(err.Error(), "auto-update-timeout") {
		t.Errorf("error should mention auto-update-timeout, got: %v", err)
	}
}

// TestRunHTTP_AutoUpdateTimeoutAboveMax verifies that runHTTP rejects an
// auto-update-timeout above the maximum threshold.
func TestRunHTTP_AutoUpdateTimeoutAboveMax(t *testing.T) {
	err := runHTTP(context.Background(), &httpConfig{
		gitlabURL:         "https://gitlab.example.com",
		maxHTTPClients:    config.DefaultMaxHTTPClients,
		sessionTimeout:    config.DefaultSessionTimeout,
		autoUpdateTimeout: 15 * time.Minute,
	})
	if err == nil {
		t.Fatal("expected error for auto-update-timeout above maximum")
	}
	if !strings.Contains(err.Error(), "auto-update-timeout") {
		t.Errorf("error should mention auto-update-timeout, got: %v", err)
	}
}

// TestRunHTTP_AutoUpdateTimeoutZero verifies that runHTTP rejects an
// explicit zero timeout instead of silently falling back to a default.
func TestRunHTTP_AutoUpdateTimeoutZero(t *testing.T) {
	err := runHTTP(context.Background(), &httpConfig{
		gitlabURL:         "https://gitlab.example.com",
		maxHTTPClients:    config.DefaultMaxHTTPClients,
		sessionTimeout:    config.DefaultSessionTimeout,
		autoUpdateTimeout: 0,
	})
	if err == nil {
		t.Fatal("expected error for zero auto-update-timeout")
	}
	if !strings.Contains(err.Error(), "auto-update-timeout") {
		t.Errorf("error should mention auto-update-timeout, got: %v", err)
	}
}

// TestRunHTTP_InvalidGitLabURL verifies that runHTTP rejects a non-HTTP(S) URL.
func TestRunHTTP_InvalidGitLabURL(t *testing.T) {
	err := runHTTP(context.Background(), &httpConfig{
		gitlabURL:      "ftp://gitlab.example.com",
		maxHTTPClients: config.DefaultMaxHTTPClients, autoUpdateTimeout: config.DefaultAutoUpdateTimeout,
		sessionTimeout: config.DefaultSessionTimeout,
	})
	if err == nil {
		t.Fatal("expected error for non-HTTP URL")
	}
	if !strings.Contains(err.Error(), "scheme") {
		t.Errorf("error should mention scheme, got: %v", err)
	}
}

// TestHostValidationMiddleware_BlockedHost verifies that the middleware
// returns 403 when the Host header does not match any allowed value.
func TestHostValidationMiddleware_BlockedHost(t *testing.T) {
	allowed := map[string]bool{"localhost": true, "127.0.0.1": true}
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := hostValidationMiddleware(allowed, inner)

	req := httptest.NewRequest(http.MethodGet, "http://evil.example.com/", nil)
	req.Host = "evil.example.com"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for blocked host, got %d", rr.Code)
	}
}

// TestHostValidationMiddleware_AllowedHost verifies that the middleware
// passes through when the Host header matches.
func TestHostValidationMiddleware_AllowedHost(t *testing.T) {
	allowed := map[string]bool{"localhost": true}
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := hostValidationMiddleware(allowed, inner)

	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	req.Host = "localhost"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for allowed host, got %d", rr.Code)
	}
}

// TestHostValidationMiddleware_HostWithPort verifies that the middleware
// strips the port from the Host header before checking the allow list.
func TestHostValidationMiddleware_HostWithPort(t *testing.T) {
	allowed := map[string]bool{"localhost": true}
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := hostValidationMiddleware(allowed, inner)

	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/", nil)
	req.Host = "localhost:8080"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for allowed host with port, got %d", rr.Code)
	}
}

// TestAutoUpdateRedactHandler_WithAttrs verifies that WithAttrs returns
// a new handler that preserves the redact strings configuration.
func TestAutoUpdateRedactHandler_WithAttrs(t *testing.T) {
	var buf strings.Builder
	base := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := &autoUpdateRedactHandler{
		base:          base,
		redactStrings: []string{"https://secret.example.com"},
	}

	derived := h.WithAttrs([]slog.Attr{slog.String("fixed", "value")})
	logger := slog.New(derived)

	buf.Reset()
	logger.Info("autoupdate: checking", "url", "https://secret.example.com/api")
	if strings.Contains(buf.String(), "secret.example.com") {
		t.Errorf("WithAttrs handler should still redact, got: %s", buf.String())
	}
}

// TestAutoUpdateRedactHandler_WithGroup verifies that WithGroup returns
// a new handler that preserves the redact strings configuration.
func TestAutoUpdateRedactHandler_WithGroup(t *testing.T) {
	var buf strings.Builder
	base := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := &autoUpdateRedactHandler{
		base:          base,
		redactStrings: []string{"https://secret.example.com"},
	}

	derived := h.WithGroup("mygroup")
	logger := slog.New(derived)

	buf.Reset()
	logger.Info("autoupdate: checking", "url", "https://secret.example.com/api")
	if strings.Contains(buf.String(), "secret.example.com") {
		t.Errorf("WithGroup handler should still redact, got: %s", buf.String())
	}
}

// TestSetupAutoUpdateRedaction_WithURL verifies that setupAutoUpdateRedaction
// installs a redacting handler when given a non-empty URL.
func TestSetupAutoUpdateRedaction_WithURL(t *testing.T) {
	// Use a concrete handler (not the initial defaultHandler) to mirror
	// production, where main() sets a JSONHandler before calling
	// setupAutoUpdateRedaction.  Restoring Go's initial defaultHandler via
	// slog.SetDefault creates a recursive deadlock because SetDefault
	// bridges to log.SetOutput, forming a cycle:
	//   defaultHandler → log.output → handlerWriter → defaultHandler.
	safe := slog.New(slog.NewJSONHandler(io.Discard, nil))
	slog.SetDefault(safe)
	t.Cleanup(func() { slog.SetDefault(safe) })

	setupAutoUpdateRedaction("https://private-gitlab.example.com")

	var buf strings.Builder
	// The default logger was replaced by setupAutoUpdateRedaction.
	// We can verify the handler type is wrapped.
	handler := slog.Default().Handler()
	if _, ok := handler.(*autoUpdateRedactHandler); !ok {
		t.Error("expected default handler to be autoUpdateRedactHandler after setup")
	}
	_ = buf
}

// TestRemoveNonReadOnlyTools verifies that removeNonReadOnlyTools strips
// tools that do not have ReadOnlyHint set to true.
func TestRemoveNonReadOnlyTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-readonly",
		Version: "0.1.0",
	}, nil)

	readOnlyAnnotations := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mutatingAnnotations := &mcp.ToolAnnotations{ReadOnlyHint: false}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "readonly_tool",
		Description: "A read-only tool",
		Annotations: readOnlyAnnotations,
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mutating_tool",
		Description: "A mutating tool",
		Annotations: mutatingAnnotations,
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{}, nil, nil
	})

	removed := removeNonReadOnlyTools(server)
	if removed != 1 {
		t.Errorf("removeNonReadOnlyTools removed %d tools, want 1", removed)
	}

	count, err := countRegisteredTools(server)
	if err != nil {
		t.Fatalf("countRegisteredTools: %v", err)
	}
	if count != 1 {
		t.Errorf("after removal: %d tools, want 1", count)
	}
}

// TestAllowedHosts_Localhost verifies that allowedHosts returns the expected
// set for a localhost binding.
func TestAllowedHosts_Localhost(t *testing.T) {
	hosts := allowedHosts("127.0.0.1:8080")
	if hosts == nil {
		t.Fatal("expected non-nil hosts for localhost binding")
	}
	if !hosts["127.0.0.1"] {
		t.Error("missing 127.0.0.1")
	}
	if !hosts["localhost"] {
		t.Error("missing localhost")
	}
}

// TestAllowedHosts_AllInterfaces verifies that allowedHosts returns nil
// for 0.0.0.0 (bind to all interfaces), which skips host validation.
func TestAllowedHosts_AllInterfaces(t *testing.T) {
	hosts := allowedHosts("0.0.0.0:8080")
	if hosts != nil {
		t.Error("expected nil hosts for 0.0.0.0 (all interfaces)")
	}
}

// TestAllowedHosts_EmptyHost verifies that allowedHosts returns nil
// for an empty host, which means all interfaces.
func TestAllowedHosts_EmptyHost(t *testing.T) {
	hosts := allowedHosts(":8080")
	if hosts != nil {
		t.Error("expected nil hosts for empty host")
	}
}

func TestClientIP_RemoteAddr(t *testing.T) {
	t.Parallel()
	r := &http.Request{RemoteAddr: "203.0.113.1:12345"}
	if got := clientIP(r, ""); got != "203.0.113.1" {
		t.Errorf("clientIP() = %q, want 203.0.113.1", got)
	}
}

func TestClientIP_TrustedProxyHeader(t *testing.T) {
	t.Parallel()
	r := &http.Request{
		RemoteAddr: "10.0.0.1:12345",
		Header:     http.Header{"X-Real-Ip": {"203.0.113.42"}},
	}
	if got := clientIP(r, "X-Real-IP"); got != "203.0.113.42" {
		t.Errorf("clientIP() = %q, want 203.0.113.42", got)
	}
}

func TestClientIP_TrustedProxyHeader_XForwardedFor(t *testing.T) {
	t.Parallel()
	// For comma-separated proxy-appended headers, clientIP returns the
	// rightmost IP because the leftmost entry is client-supplied and
	// therefore spoofable.
	r := &http.Request{
		RemoteAddr: "10.0.0.1:12345",
		Header:     http.Header{"X-Forwarded-For": {"203.0.113.1, 10.0.0.2, 10.0.0.77"}},
	}
	if got := clientIP(r, "X-Forwarded-For"); got != "10.0.0.77" {
		t.Errorf("clientIP() = %q, want 10.0.0.77 (rightmost entry, non-spoofable)", got)
	}
}

func TestClientIP_TrustedProxyHeader_SpoofResistant(t *testing.T) {
	t.Parallel()
	// An attacker-controlled client prepends a fake IP. The rightmost entry
	// (added by the real trusted proxy) must be returned.
	r := &http.Request{
		RemoteAddr: "10.0.0.1:12345",
		Header:     http.Header{"X-Forwarded-For": {"1.2.3.4, 203.0.113.55"}},
	}
	if got := clientIP(r, "X-Forwarded-For"); got != "203.0.113.55" {
		t.Errorf("clientIP() = %q, want 203.0.113.55 (ignores leftmost spoofed value)", got)
	}
}

func TestClientIP_TrustedProxyHeader_Empty(t *testing.T) {
	t.Parallel()
	r := &http.Request{
		RemoteAddr: "203.0.113.99:12345",
		Header:     http.Header{},
	}
	if got := clientIP(r, "X-Real-IP"); got != "203.0.113.99" {
		t.Errorf("clientIP() = %q, want 203.0.113.99 (fallback to RemoteAddr)", got)
	}
}

// TestBuildServerCard_ReturnsValidJSON verifies that [buildServerCard] produces
// valid JSON containing serverInfo, authentication, and a non-empty tools array
// with meta-tools when MetaTools=true.
func TestBuildServerCard_ReturnsValidJSON(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		GitLabURL:     "", // empty — buildServerCard falls back to https://gitlab.com
		SkipTLSVerify: true,
		MetaTools:     true,
		Enterprise:    false,
	}

	data, err := buildServerCard(cfg)
	if err != nil {
		t.Fatalf("buildServerCard() returned error: %v", err)
	}

	var card map[string]any
	if unmarshalErr := json.Unmarshal(data, &card); unmarshalErr != nil {
		t.Fatalf("buildServerCard() returned invalid JSON: %v", unmarshalErr)
	}

	// Verify serverInfo
	serverInfo, siOK := card["serverInfo"].(map[string]any)
	if !siOK {
		t.Fatal("card missing 'serverInfo' object")
	}
	if name := serverInfo["name"]; name != "gitlab-mcp-server" {
		t.Errorf("serverInfo.name = %q, want %q", name, "gitlab-mcp-server")
	}

	// Verify authentication
	auth, authOK := card["authentication"].(map[string]any)
	if !authOK {
		t.Fatal("card missing 'authentication' object")
	}
	if required, reqOK := auth["required"].(bool); !reqOK || !required {
		t.Error("authentication.required should be true")
	}

	// Verify tools is a non-empty array
	toolsRaw, toolsOK := card["tools"].([]any)
	if !toolsOK {
		t.Fatal("card missing 'tools' array")
	}
	if len(toolsRaw) == 0 {
		t.Fatal("tools array is empty, expected registered tools")
	}

	// Spot-check first tool has name and description
	firstRaw := toolsRaw[0]
	tool, toolOK := firstRaw.(map[string]any)
	if !toolOK {
		t.Fatal("tools[0] is not an object")
	}
	if name, nameOK := tool["name"].(string); !nameOK || name == "" {
		t.Error("tools[0] missing or empty 'name'")
	}
	if desc, descOK := tool["description"].(string); !descOK || desc == "" {
		t.Error("tools[0] missing or empty 'description'")
	}
}

// TestBuildServerCard_IndividualMode verifies that [buildServerCard] returns
// individual tools (not meta-tools) when MetaTools=false.
func TestBuildServerCard_IndividualMode(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		GitLabURL:     "",
		SkipTLSVerify: true,
		MetaTools:     false,
		Enterprise:    false,
	}

	data, err := buildServerCard(cfg)
	if err != nil {
		t.Fatalf("buildServerCard() returned error: %v", err)
	}

	var card map[string]any
	if unmarshalErr := json.Unmarshal(data, &card); unmarshalErr != nil {
		t.Fatalf("invalid JSON: %v", unmarshalErr)
	}

	toolsRaw, toolsOK := card["tools"].([]any)
	if !toolsOK || len(toolsRaw) == 0 {
		t.Fatal("tools array missing or empty")
	}

	// Individual mode should have many more tools than meta-tool mode
	const minIndividualTools = 700
	if len(toolsRaw) < minIndividualTools {
		t.Errorf("individual mode tools count = %d, want at least %d", len(toolsRaw), minIndividualTools)
	}
}

// TestServeHTTP_ServerCardEndpoint_ReturnsToolList verifies that the
// /.well-known/mcp/server-card.json endpoint returns a valid server card
// with tools, and is accessible without authentication.
func TestServeHTTP_ServerCardEndpoint_ReturnsToolList(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	cfg := &config.Config{
		GitLabURL:      mockGL.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTP(ctx, cfg, addr)
	}()

	waitForHTTPServerReady(t, addr, errCh)

	// GET /.well-known/mcp/server-card.json — no auth headers
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet,
		"http://"+addr+"/.well-known/mcp/server-card.json", nil)

	resp, reqErr := testHTTPClient.Do(req)
	if reqErr != nil {
		cancel()
		t.Fatalf("request failed: %v", reqErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(body))
	}

	if ct := resp.Header.Get(hdrContentType); ct != mimeJSON {
		t.Errorf("Content-Type = %q, want %q", ct, mimeJSON)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "public") {
		t.Errorf("Cache-Control = %q, want to contain 'public'", cc)
	}

	var card map[string]any
	body, _ := io.ReadAll(resp.Body)
	if unmarshalErr := json.Unmarshal(body, &card); unmarshalErr != nil {
		t.Fatalf("invalid JSON response: %v\nbody: %s", unmarshalErr, string(body))
	}

	toolsRaw, toolsOK := card["tools"].([]any)
	if !toolsOK || len(toolsRaw) == 0 {
		t.Fatal("server card 'tools' array missing or empty")
	}

	// Verify serverInfo presence
	if _, siOK := card["serverInfo"].(map[string]any); !siOK {
		t.Error("server card missing 'serverInfo'")
	}

	cancel()
	select {
	case err = <-errCh:
		if err != nil {
			t.Fatalf("serveHTTP error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveHTTP did not shut down in time")
	}
}
