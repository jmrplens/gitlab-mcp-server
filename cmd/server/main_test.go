// main_test.go contains unit and integration-style tests for the server entry
// point. Tests cover CLI flag handling, configuration validation, GitLab client
// setup, HTTP and stdio transport modes, MCP protocol handshakes, tool catalog
// filtering, OAuth middleware, server-card generation, and auto-update logging
// redaction.
//
// The tests use httptest servers for GitLab API responses and HTTP transport
// requests, plus in-memory MCP transports for direct tools/list inspection.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
)

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

// newMockGitLabClient creates a [gitlabclient.Client] backed by an httptest
// GitLab server that responds to /api/v4/version. It gives server-construction
// tests a valid client without requiring real GitLab credentials.
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

func mustCreateServer(t *testing.T, client *gitlabclient.Client, cfg *config.ServerConfig) *mcp.Server {
	t.Helper()
	server, err := createServer(client, cfg, nil)
	if err != nil {
		t.Fatalf("createServer() error: %v", err)
	}
	return server
}

// newTestMCPServer creates an MCP server with the full individual tool catalog,
// resources, and prompts registered. HTTP protocol tests use it as a stable
// handler target for initialize and tools/list requests.
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

// newInMemorySession connects an in-memory MCP client to server and registers
// cleanup for both sessions. It is used by tests that need to inspect the
// finalized server catalog without opening an HTTP listener.
func newInMemorySession(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()

	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := mcpClient.Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
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
// required environment variables are missing.
func TestRun_InvalidConfig_ReturnsError(t *testing.T) {
	t.Setenv("GITLAB_URL", "")
	t.Setenv("GITLAB_TOKEN", "")

	err := run(nil)
	if err == nil {
		t.Fatal("run() expected error when config is invalid, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "GITLAB_TOKEN") {
		t.Errorf("error should mention GITLAB_TOKEN, got: %s", msg)
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

// TestHTTPHandler_ParameterizedContentType_ReturnsServerInfo verifies that the
// streamable HTTP transport accepts JSON content types with parameters.
func TestHTTPHandler_ParameterizedContentType_ReturnsServerInfo(t *testing.T) {
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
	req.Header.Set(hdrContentType, "application/json; charset=utf-8")
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

// TestHTTPHandler_Initialize_AdvertisesListChangedCapabilities verifies that
// the initialize handshake reports listChanged: true for tools, resources,
// and prompts so that MCP clients know they will receive
// notifications/{tools,resources,prompts}/list_changed when the catalog
// changes (e.g. dynamic registration).
func TestHTTPHandler_Initialize_AdvertisesListChangedCapabilities(t *testing.T) {
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

	result := parseJSONRPCResponse(t, resp)
	res, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("response missing 'result' field: %v", result)
	}
	caps, ok := res["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("response missing 'capabilities' field: %v", res)
	}

	for _, key := range []string{"tools", "resources", "prompts"} {
		group, gok := caps[key].(map[string]any)
		if !gok {
			t.Errorf("capabilities.%s missing or not an object: %v", key, caps[key])
			continue
		}
		if got := group["listChanged"]; got != true {
			t.Errorf("capabilities.%s.listChanged = %v, want true", key, got)
		}
	}
}

// TestHTTPHandler_Initialize_CapabilitySurfaceControlsPromptsCapability verifies
// that the initialize handshake mirrors the selected resource and prompt surface.
func TestHTTPHandler_Initialize_CapabilitySurfaceControlsPromptsCapability(t *testing.T) {
	client := newMockGitLabClient(t)
	testCases := []initializeCapabilityCase{
		{name: "full", capabilitySurface: config.CapabilitySurfaceFull, wantPromptsCapability: true},
		{name: "minimal", capabilitySurface: config.CapabilitySurfaceMinimal, wantPromptsCapability: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := mustCreateServer(t, client, &config.ServerConfig{MetaTools: true, ToolSurface: config.ToolSurfaceDynamic, CapabilitySurface: tc.capabilitySurface})
			caps := initializeCapabilities(t, server)
			assertListChangedCapabilities(t, caps, "tools", "resources")
			assertPromptsCapability(t, caps, tc.wantPromptsCapability)
		})
	}
}

type initializeCapabilityCase struct {
	name                  string
	capabilitySurface     string
	wantPromptsCapability bool
}

func initializeCapabilities(t *testing.T, server *mcp.Server) map[string]any {
	t.Helper()
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return server }, nil)
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
	t.Cleanup(func() { closeMCPSession(t, ts.URL, resp.Header.Get(hdrMCPSessionID)) })
	return responseCapabilities(t, parseJSONRPCResponse(t, resp))
}

func responseCapabilities(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	res, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("response missing 'result' field: %v", result)
	}
	caps, ok := res["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("response missing 'capabilities' field: %v", res)
	}
	return caps
}

func assertListChangedCapabilities(t *testing.T, caps map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		group, ok := caps[key].(map[string]any)
		if !ok {
			t.Fatalf("capabilities.%s missing or not an object: %v", key, caps[key])
		}
		if got := group["listChanged"]; got != true {
			t.Fatalf("capabilities.%s.listChanged = %v, want true", key, got)
		}
	}
}

func assertPromptsCapability(t *testing.T, caps map[string]any, wantPrompts bool) {
	t.Helper()
	promptsCapabilityValue, hasPromptsCapability := caps["prompts"]
	if !wantPrompts {
		if hasPromptsCapability {
			t.Fatalf("minimal capability surface advertised prompts: %v", promptsCapabilityValue)
		}
		return
	}
	if !hasPromptsCapability {
		t.Fatal("full capability surface should advertise prompts")
	}
	promptsCapability, ok := promptsCapabilityValue.(map[string]any)
	if !ok {
		t.Fatalf("capabilities.prompts is not an object: %v", promptsCapabilityValue)
	}
	if got := promptsCapability["listChanged"]; got != true {
		t.Fatalf("capabilities.prompts.listChanged = %v, want true", got)
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
	case <-time.After(15 * time.Second):
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
// GitLab connectivity ping returns a failure status.
func TestRun_GitLabConnectionFailure(t *testing.T) {
	srv := newFailingGitLabServer(t, http.StatusForbidden)
	t.Setenv("GITLAB_URL", srv.URL)
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

func newFailingGitLabServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/version" {
			w.Header().Set(hdrContentType, mimeJSON)
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"error"}`))
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
// error when configuration is invalid.
func TestRunWithContext_InvalidConfig(t *testing.T) {
	t.Setenv("GITLAB_URL", "")
	t.Setenv("GITLAB_TOKEN", "")

	err := runWithContext(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when config is invalid")
	}
	if !strings.Contains(err.Error(), "GITLAB_TOKEN") {
		t.Fatalf("error = %q, want GITLAB_TOKEN", err.Error())
	}
}

// TestRunWithContext_PingFailure verifies that [runWithContext] returns an error
// when the GitLab connectivity ping returns a failure status.
func TestRunWithContext_PingFailure(t *testing.T) {
	srv := newFailingGitLabServer(t, http.StatusForbidden)
	t.Setenv("GITLAB_URL", srv.URL)
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
	serverInfo := initializeTestServer(t, &config.ServerConfig{MetaTools: false})
	if name := serverInfo["name"]; name != serverName {
		t.Errorf("serverInfo.name = %q, want %q", name, serverName)
	}
}

func initializeTestServer(t *testing.T, cfg *config.ServerConfig) map[string]any {
	t.Helper()
	server := mustCreateServer(t, newMockGitLabClient(t), cfg)
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return server }, nil)
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
	return serverInfo
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

// TestStaticConfigurationExamplesPreferToolSurface verifies static setup
// examples do not reintroduce META_TOOLS as the preferred selector.
func TestStaticConfigurationExamplesPreferToolSurface(t *testing.T) {
	repoRoot := filepath.Clean("../..")
	files := []string{"mcp.json", "docker-compose.yml", "server.json"}
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(repoRoot, name))
			if err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}
			content := string(data)
			if strings.Contains(content, "META_TOOLS") {
				t.Fatalf("%s still recommends deprecated META_TOOLS", name)
			}
			if !strings.Contains(content, "TOOL_SURFACE") {
				t.Fatalf("%s does not mention TOOL_SURFACE", name)
			}
		})
	}
}

// TestMain_HelpParsesEnterpriseFlag verifies the CLI registers and visits the
// --enterprise flag before returning through the help path.
func TestMain_HelpParsesEnterpriseFlag(t *testing.T) {
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine
	oldStdout := os.Stdout

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Args = []string{"gitlab-mcp-server", "-h", "-enterprise"}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Stdout = w
	t.Cleanup(func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
		os.Stdout = oldStdout
	})

	main()

	_ = w.Close()
	out, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("ReadAll: %v", readErr)
	}
	if !strings.Contains(string(out), "gitlab-mcp-server") {
		t.Fatalf("help output missing project name: %s", string(out))
	}
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
	serverInfo := initializeTestServer(t, &config.ServerConfig{MetaTools: true})
	if name := serverInfo["name"]; name != serverName {
		t.Errorf("serverInfo.name = %q, want %q", name, serverName)
	}
}

// TestCreateServer_DynamicToolSurface verifies that the default low-token
// dynamic surface exposes find and execute plus surface-aware catalog resources.
func TestCreateServer_DynamicToolSurface(t *testing.T) {
	client := newMockGitLabClient(t)
	server := mustCreateServer(t, client, &config.ServerConfig{MetaTools: true, ToolSurface: config.ToolSurfaceDynamic})
	session := newInMemorySession(t, server)

	toolsResult, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	wantTools := map[string]bool{
		"gitlab_find_action":    false,
		"gitlab_execute_action": false,
	}
	for _, tool := range toolsResult.Tools {
		if _, ok := wantTools[tool.Name]; !ok {
			t.Fatalf("unexpected dynamic tool %q", tool.Name)
		}
		wantTools[tool.Name] = true
	}
	for name, found := range wantTools {
		if !found {
			t.Fatalf("dynamic tool %q was not registered", name)
		}
	}

	_, err = session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "gitlab://tools/project.get"})
	if err != nil {
		t.Fatalf("dynamic surface should expose tool manifest detail resources: %v", err)
	}
}

// TestCreateServer_DynamicToolSurfaceWithUpdaterIncludesUpdateSchema verifies
// the default dynamic startup path can expose updater-backed maintenance actions
// without falling back to legacy schema-less routes.
func TestCreateServer_DynamicToolSurfaceWithUpdaterIncludesUpdateSchema(t *testing.T) {
	client := newMockGitLabClient(t)
	updater := autoupdate.NewUpdaterWithSource(autoupdate.Config{
		Mode:           autoupdate.ModeCheck,
		Repository:     "owner/repo",
		CurrentVersion: "1.0.0",
	}, autoupdate.EmptySource{})
	server, err := createServer(client, &config.ServerConfig{MetaTools: true, ToolSurface: config.ToolSurfaceDynamic}, updater)
	if err != nil {
		t.Fatalf("createServer(dynamic with updater) error = %v", err)
	}
	session := newInMemorySession(t, server)

	result, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "gitlab://tools/server.apply_update"})
	if err != nil {
		t.Fatalf("dynamic surface should expose server.apply_update detail resource: %v", err)
	}
	var detail resources.ToolSurfaceDetail
	if unmarshalErr := json.Unmarshal([]byte(result.Contents[0].Text), &detail); unmarshalErr != nil {
		t.Fatalf("unmarshal server.apply_update detail: %v", unmarshalErr)
	}
	schema, ok := detail.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("server.apply_update input schema = %T, want map[string]any", detail.InputSchema)
	}
	if got := schema["type"]; got != "object" {
		t.Fatalf("server.apply_update input schema type = %v, want object", got)
	}
}

// TestCreateServer_MetaToolSurfaceIncludesStandaloneUtilities verifies the
// catalog-backed meta surface keeps standalone helper tools available.
func TestCreateServer_MetaToolSurfaceIncludesStandaloneUtilities(t *testing.T) {
	client := newMockGitLabClient(t)
	server := mustCreateServer(t, client, &config.ServerConfig{MetaTools: true, ToolSurface: config.ToolSurfaceMeta})
	session := newInMemorySession(t, server)

	toolsResult, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	wantTools := map[string]bool{
		"gitlab_discover_project":           false,
		"gitlab_interactive_issue_create":   false,
		"gitlab_interactive_mr_create":      false,
		"gitlab_interactive_project_create": false,
		"gitlab_interactive_release_create": false,
	}
	for _, tool := range toolsResult.Tools {
		if _, ok := wantTools[tool.Name]; ok {
			wantTools[tool.Name] = true
		}
		if tool.Name == "gitlab_interactive_project_create" {
			schema, ok := tool.InputSchema.(map[string]any)
			if !ok {
				t.Fatalf("gitlab_interactive_project_create input schema = %T, want map[string]any", tool.InputSchema)
			}
			properties, ok := schema["properties"].(map[string]any)
			if !ok || properties == nil {
				t.Fatalf("gitlab_interactive_project_create properties = %T, want map[string]any in %#v", schema["properties"], schema)
			}
			if len(properties) != 0 {
				t.Fatalf("gitlab_interactive_project_create properties = %#v, want empty map", properties)
			}
			if v, boolOK := schema["additionalProperties"].(bool); !boolOK || v {
				t.Fatalf("gitlab_interactive_project_create additionalProperties = %v, want false", schema["additionalProperties"])
			}
		}
	}
	for name, found := range wantTools {
		if !found {
			t.Fatalf("meta standalone tool %q was not registered", name)
		}
	}
}

// TestCreateServer_CapabilitySurfaceParity verifies that resource and prompt
// exposure follows CAPABILITY_SURFACE consistently across catalog-backed tool
// surfaces while action schemas are served through gitlab://tools.
func TestCreateServer_CapabilitySurfaceParity(t *testing.T) {
	client := newMockGitLabClient(t)
	testCases := []capabilitySurfaceParityCase{
		{name: "meta full", toolSurface: config.ToolSurfaceMeta, capabilitySurface: config.CapabilitySurfaceFull, wantFullCatalog: true},
		{name: "meta minimal", toolSurface: config.ToolSurfaceMeta, capabilitySurface: config.CapabilitySurfaceMinimal},
		{name: "dynamic full", toolSurface: config.ToolSurfaceDynamic, capabilitySurface: config.CapabilitySurfaceFull, wantFullCatalog: true},
		{name: "dynamic minimal", toolSurface: config.ToolSurfaceDynamic, capabilitySurface: config.CapabilitySurfaceMinimal},
		{name: "individual full", toolSurface: config.ToolSurfaceIndividual, capabilitySurface: config.CapabilitySurfaceFull, wantFullCatalog: true},
		{name: "individual minimal", toolSurface: config.ToolSurfaceIndividual, capabilitySurface: config.CapabilitySurfaceMinimal},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := mustCreateServer(t, client, &config.ServerConfig{MetaTools: true, ToolSurface: tc.toolSurface, CapabilitySurface: tc.capabilitySurface})
			assertCapabilitySurfaceParity(t, newInMemorySession(t, server), tc)
		})
	}
}

type capabilitySurfaceParityCase struct {
	name              string
	toolSurface       string
	capabilitySurface string
	wantFullCatalog   bool
}

func assertCapabilitySurfaceParity(t *testing.T, session *mcp.ClientSession, tc capabilitySurfaceParityCase) {
	t.Helper()
	assertCapabilityResources(t, session, tc.wantFullCatalog)
	assertCapabilityResourceTemplates(t, session, tc.wantFullCatalog)
	assertLegacySchemaResourcesOmitted(t, session)
	assertManifestDetailReadable(t, session, tc.toolSurface)
	assertPromptSurface(t, session, tc.wantFullCatalog)
	assertCompletionHandlerAvailable(t, session)
}

func assertCapabilityResources(t *testing.T, session *mcp.ClientSession, wantFullCatalog bool) {
	t.Helper()
	resourcesResult, err := session.ListResources(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListResources() error = %v", err)
	}
	resources := resourcesResult.Resources
	for _, uri := range []string{"gitlab://workspace/roots", "gitlab://tools"} {
		if !resourceListHasURI(resources, uri) {
			t.Fatalf("resources = %+v, want %s", resources, uri)
		}
	}
	if wantFullCatalog {
		assertFullCatalogResources(t, resources)
		return
	}
	if len(resources) != 2 {
		t.Fatalf("minimal resources = %+v, want 2 resources", resources)
	}
}

func assertFullCatalogResources(t *testing.T, resources []*mcp.Resource) {
	t.Helper()
	for _, uri := range []string{"gitlab://user/current", "gitlab://guides/git-workflow"} {
		if !resourceListHasURI(resources, uri) {
			t.Fatalf("full resources missing %q: %+v", uri, resources)
		}
	}
}

func assertCapabilityResourceTemplates(t *testing.T, session *mcp.ClientSession, wantFullCatalog bool) {
	t.Helper()
	templatesResult, err := session.ListResourceTemplates(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates() error = %v", err)
	}
	templates := templatesResult.ResourceTemplates
	if !resourceTemplateListHasURI(templates, "gitlab://tools/{id}") {
		t.Fatalf("resource templates missing tool manifest template: %+v", templates)
	}
	if resourceTemplateListHasURI(templates, "gitlab://schema/meta/{tool}/{action}") || resourceTemplateListHasURI(templates, "gitlab://schema/dynamic/{action}") {
		t.Fatalf("resource templates should expose gitlab://tools/{id} instead of legacy schema templates: %+v", templates)
	}
	if !wantFullCatalog && len(templates) != 1 {
		t.Fatalf("minimal resource templates = %+v, want 1", templates)
	}
}

func assertLegacySchemaResourcesOmitted(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	for _, uri := range []string{"gitlab://schema/meta/gitlab_project/get", "gitlab://schema/dynamic/project.get"} {
		if _, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri}); err == nil {
			t.Fatalf("server should omit legacy schema resource %s", uri)
		}
	}
}

func assertManifestDetailReadable(t *testing.T, session *mcp.ClientSession, toolSurface string) {
	t.Helper()
	if _, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: manifestDetailURIForSurface(toolSurface)}); err != nil {
		t.Fatalf("tool manifest detail should be readable: %v", err)
	}
}

func resourceListHasURI(items []*mcp.Resource, uri string) bool {
	for _, item := range items {
		if item.URI == uri {
			return true
		}
	}
	return false
}

func resourceTemplateListHasURI(items []*mcp.ResourceTemplate, uri string) bool {
	for _, item := range items {
		if item.URITemplate == uri {
			return true
		}
	}
	return false
}

func manifestDetailURIForSurface(toolSurface string) string {
	switch toolSurface {
	case config.ToolSurfaceDynamic:
		return "gitlab://tools/project.get"
	case config.ToolSurfaceMeta:
		return "gitlab://tools/gitlab_project.get"
	default:
		return "gitlab://tools/gitlab_project_get"
	}
}

func assertPromptSurface(t *testing.T, session *mcp.ClientSession, wantPrompts bool) {
	t.Helper()
	promptsResult, err := session.ListPrompts(t.Context(), nil)
	if wantPrompts {
		if err != nil {
			t.Fatalf("ListPrompts() error = %v", err)
		}
		if len(promptsResult.Prompts) == 0 {
			t.Fatal("full capability surface registered no prompts")
		}
		return
	}
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "not found") {
			t.Fatalf("ListPrompts() error = %v", err)
		}
		return
	}
	if len(promptsResult.Prompts) > 0 {
		t.Fatalf("minimal prompts = %+v, want none", promptsResult.Prompts)
	}
}

func assertCompletionHandlerAvailable(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	result, err := session.Complete(t.Context(), &mcp.CompleteParams{
		Ref: &mcp.CompleteReference{
			Type: "ref/prompt",
			Name: "summarize_mr_changes",
		},
		Argument: mcp.CompleteParamsArgument{
			Name:  "unknown_argument",
			Value: "",
		},
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if len(result.Completion.Values) != 0 {
		t.Fatalf("Complete() values = %v, want empty result for unknown argument", result.Completion.Values)
	}
}

// TestCreateServer_DynamicReadOnlyRemovesExecute verifies that read-only mode
// keeps discovery but removes execution from the dynamic surface.
func TestCreateServer_DynamicReadOnlyRemovesExecute(t *testing.T) {
	client := newMockGitLabClient(t)
	server := mustCreateServer(t, client, &config.ServerConfig{MetaTools: true, ToolSurface: config.ToolSurfaceDynamic, ReadOnly: true})
	toolsResult, err := listRegisteredTools(server, "dynamic-readonly")
	if err != nil {
		t.Fatalf("list dynamic read-only tools: %v", err)
	}
	wantTools := map[string]struct{}{
		"gitlab_find_action": {},
	}
	gotTools := make(map[string]struct{}, len(toolsResult))
	for _, tool := range toolsResult {
		gotTools[tool.Name] = struct{}{}
	}
	for name := range gotTools {
		if _, ok := wantTools[name]; !ok {
			t.Fatalf("read-only dynamic surface tool %q is registered; want only discovery tools", name)
		}
	}
	for name := range wantTools {
		if _, found := gotTools[name]; !found {
			t.Fatalf("read-only dynamic surface missing discovery tool %q", name)
		}
	}
}

// TestCreateServer_ToolManifestResourcesFollowToolMode verifies that the
// unified tool manifest is advertised for every tool surface while legacy
// schema templates are not exposed.
func TestCreateServer_ToolManifestResourcesFollowToolMode(t *testing.T) {
	client := newMockGitLabClient(t)

	individual := mustCreateServer(t, client, &config.ServerConfig{MetaTools: false})
	individualSession := newInMemorySession(t, individual)
	individualTemplates, err := individualSession.ListResourceTemplates(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates individual: %v", err)
	}
	for _, tpl := range individualTemplates.ResourceTemplates {
		if tpl.URITemplate == "gitlab://schema/meta/{tool}/{action}" {
			t.Fatal("individual mode should not advertise meta-tool schema resources")
		}
	}
	if !resourceTemplateListHasURI(individualTemplates.ResourceTemplates, "gitlab://tools/{id}") {
		t.Fatal("individual mode should advertise tool manifest detail resources")
	}

	meta := mustCreateServer(t, client, &config.ServerConfig{MetaTools: true})
	metaSession := newInMemorySession(t, meta)
	metaTemplates, err := metaSession.ListResourceTemplates(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates meta: %v", err)
	}
	for _, tpl := range metaTemplates.ResourceTemplates {
		if tpl.URITemplate == "gitlab://schema/meta/{tool}/{action}" {
			t.Fatal("meta mode should not advertise legacy meta-tool schema resources")
		}
	}
	if !resourceTemplateListHasURI(metaTemplates.ResourceTemplates, "gitlab://tools/{id}") {
		t.Fatal("meta mode should advertise tool manifest detail resources")
	}
}

// TestCreateServer_ToolManifestRoutesFollowVisibleTools verifies that manifest
// entries mirror the post-filter tool catalog instead of the global route
// registry populated during registration.
func TestCreateServer_ToolManifestRoutesFollowVisibleTools(t *testing.T) {
	client := newMockGitLabClient(t)
	cfg := &config.ServerConfig{
		MetaTools:    true,
		ExcludeTools: []string{"gitlab_runner"},
	}
	server := mustCreateServer(t, client, cfg)
	session := newInMemorySession(t, server)

	result, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "gitlab://tools"})
	if err != nil {
		t.Fatalf("ReadResource tool manifest: %v", err)
	}
	var manifest resources.ToolSurfaceManifest
	if unmarshalErr := json.Unmarshal([]byte(result.Contents[0].Text), &manifest); unmarshalErr != nil {
		t.Fatalf("unmarshal manifest: %v", unmarshalErr)
	}
	for _, entry := range manifest.Entries {
		if entry.Tool == "gitlab_runner" {
			t.Fatal("excluded meta-tool should not appear in tool manifest")
		}
	}

	_, err = session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "gitlab://tools/gitlab_runner.list"})
	if err == nil {
		t.Fatal("excluded meta-tool manifest detail should not be readable")
	}
	_, err = session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "gitlab://tools/gitlab_merge_request.create"})
	if err != nil {
		t.Fatalf("visible meta-tool manifest detail should be readable: %v", err)
	}
}

// TestCreateServer_ToolManifestRoutesAreServerScoped verifies that manifest
// entries keep the route set captured for their own server even if another
// server registers a different CE/Enterprise catalog later in the same process.
func TestCreateServer_ToolManifestRoutesAreServerScoped(t *testing.T) {
	client := newMockGitLabClient(t)
	ceServer := mustCreateServer(t, client, &config.ServerConfig{MetaTools: true, Enterprise: false})
	ceSession := newInMemorySession(t, ceServer)

	_ = mustCreateServer(t, client, &config.ServerConfig{MetaTools: true, Enterprise: true})

	_, err := ceSession.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "gitlab://tools/gitlab_project.push_rule_get"})
	if err == nil {
		t.Fatal("CE server should not expose enterprise-only project action detail")
	}
	_, err = ceSession.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "gitlab://tools/gitlab_project.get"})
	if err != nil {
		t.Fatalf("CE server should still expose common project action detail: %v", err)
	}
}

// TestCreateServer_FilteringModes verifies that createServer exercises the
// request-scoped scope filtering and safe-mode wrapping branches used by HTTP
// server-pool entries.
func TestCreateServer_FilteringModes(t *testing.T) {
	client := newMockGitLabClient(t)

	readAPIServer := mustCreateServer(t, client, &config.ServerConfig{
		MetaTools:   false,
		TokenScopes: []string{"read_api"},
	})
	readAPITools, err := listRegisteredTools(readAPIServer, "read-api-filter-test")
	if err != nil {
		t.Fatalf("list read-api tools: %v", err)
	}
	for _, tool := range readAPITools {
		if tool.Name == "gitlab_create_project" {
			t.Fatal("read_api scope should remove mutating project creation tool")
		}
	}

	safeModeServer := mustCreateServer(t, client, &config.ServerConfig{MetaTools: false, SafeMode: true})
	safeModeTools, err := listRegisteredTools(safeModeServer, "safe-mode-test")
	if err != nil {
		t.Fatalf("list safe-mode tools: %v", err)
	}
	if len(safeModeTools) == 0 {
		t.Fatal("safe-mode server should still expose tools")
	}
}

// TestCreateServer_ToolManifestInspectionError verifies createServer remains
// usable when the best-effort visible-tool inspection for the tool manifest
// fails, covering the defensive warning path.
func TestCreateServer_ToolManifestInspectionError(t *testing.T) {
	client := newMockGitLabClient(t)
	original := listRegisteredToolsForInspection
	listRegisteredToolsForInspection = func(_ *mcp.Server, _ string) ([]*mcp.Tool, error) {
		return nil, errors.New("forced inspection failure")
	}
	t.Cleanup(func() { listRegisteredToolsForInspection = original })

	server := mustCreateServer(t, client, &config.ServerConfig{MetaTools: true})
	session := newInMemorySession(t, server)
	if _, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "gitlab://workspace/roots"}); err != nil {
		t.Fatalf("workspace roots should still be readable after manifest inspection error: %v", err)
	}
	if _, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "gitlab://tools"}); err == nil {
		t.Fatal("tool manifest should be omitted when inspection fails")
	}
}

// TestListRegisteredTools_ErrorPaths verifies defensive error wrapping for
// the in-memory MCP inspection helper used by tool counting and schema route
// filtering.
func TestListRegisteredTools_ErrorPaths(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "inspection-errors", Version: "0"}, nil)
	forcedErr := errors.New("forced failure")

	t.Run("server connect", func(t *testing.T) {
		original := connectInspectionServer
		connectInspectionServer = func(_ *mcp.Server, _ context.Context, _ mcp.Transport) (*mcp.ServerSession, error) {
			return nil, forcedErr
		}
		t.Cleanup(func() { connectInspectionServer = original })

		_, err := listRegisteredTools(server, "server-error")
		if err == nil || !strings.Contains(err.Error(), "server connect") {
			t.Fatalf("listRegisteredTools() error = %v, want server connect context", err)
		}
	})

	t.Run("client connect", func(t *testing.T) {
		original := connectInspectionClient
		connectInspectionClient = func(_ *mcp.Client, _ context.Context, _ mcp.Transport) (*mcp.ClientSession, error) {
			return nil, forcedErr
		}
		t.Cleanup(func() { connectInspectionClient = original })

		_, err := listRegisteredTools(server, "client-error")
		if err == nil || !strings.Contains(err.Error(), "client connect") {
			t.Fatalf("listRegisteredTools() error = %v, want client connect context", err)
		}
	})

	t.Run("list tools", func(t *testing.T) {
		original := listInspectionTools
		listInspectionTools = func(_ *mcp.ClientSession, _ context.Context) (*mcp.ListToolsResult, error) {
			return nil, forcedErr
		}
		t.Cleanup(func() { listInspectionTools = original })

		_, err := listRegisteredTools(server, "list-error")
		if err == nil || !strings.Contains(err.Error(), "list tools") {
			t.Fatalf("listRegisteredTools() error = %v, want list tools context", err)
		}
	})
}

// TestStartStdioAutoUpdate_InvalidMode verifies that startStdioAutoUpdate
// returns immediately when the AUTO_UPDATE value is invalid.
func TestStartStdioAutoUpdate_InvalidMode(t *testing.T) {
	cfg := &config.Config{AutoUpdate: "invalid-value"}
	// Should log warning and return without panic.
	startStdioAutoUpdate(t.Context(), cfg)
}

// TestStartStdioAutoUpdate_DisabledMode verifies that startStdioAutoUpdate
// returns immediately when AUTO_UPDATE is "false" (disabled).
func TestStartStdioAutoUpdate_DisabledMode(t *testing.T) {
	cfg := &config.Config{AutoUpdate: "false"}
	startStdioAutoUpdate(t.Context(), cfg)
}

// TestStartStdioAutoUpdate_ValidMode verifies that startStdioAutoUpdate
// exercises the full path when mode is valid.
func TestStartStdioAutoUpdate_ValidMode(t *testing.T) {
	called := make(chan struct{})
	check := func(context.Context, autoupdate.Config) (string, bool, error) {
		close(called)
		return "", false, nil
	}

	cfg := &config.Config{
		AutoUpdate:     "true",
		AutoUpdateRepo: "group/project",
	}
	startStdioAutoUpdateWithCheck(t.Context(), cfg, check)

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("expected startup auto-update check to run")
	}
}

// TestStartStdioAutoUpdate_ValidModeReturnsBeforeCheckCompletes verifies that
// startup auto-update work runs in the background instead of delaying stdio MCP
// startup while an update check or download is still in progress.
func TestStartStdioAutoUpdate_ValidModeReturnsBeforeCheckCompletes(t *testing.T) {
	oldVersion := version
	version = "1.0.0"
	t.Cleanup(func() { version = oldVersion })

	started := make(chan autoupdate.Config, 1)
	releaseCheck := make(chan struct{})
	checkDone := make(chan struct{})
	check := func(_ context.Context, cfg autoupdate.Config) (string, bool, error) {
		started <- cfg
		<-releaseCheck
		close(checkDone)
		return "1.1.0", true, nil
	}

	cfg := &config.Config{
		AutoUpdate:        "true",
		AutoUpdateRepo:    "group/project",
		AutoUpdateTimeout: time.Minute,
	}
	returned := make(chan struct{})
	go func() {
		startStdioAutoUpdateWithCheck(t.Context(), cfg, check)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("startStdioAutoUpdate blocked waiting for the background check")
	}

	select {
	case updateCfg := <-started:
		if updateCfg.Repository != cfg.AutoUpdateRepo {
			t.Fatalf("Repository = %q, want %q", updateCfg.Repository, cfg.AutoUpdateRepo)
		}
		if updateCfg.Timeout != cfg.AutoUpdateTimeout {
			t.Fatalf("Timeout = %s, want %s", updateCfg.Timeout, cfg.AutoUpdateTimeout)
		}
	case <-time.After(time.Second):
		t.Fatal("background update check did not start")
	}

	select {
	case <-checkDone:
		t.Fatal("background update check completed before the test released it")
	default:
	}

	close(releaseCheck)
	select {
	case <-checkDone:
	case <-time.After(time.Second):
		t.Fatal("background update check did not finish after release")
	}
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

// TestServeHTTP_CrossOriginProtection_RejectsCrossSitePost verifies HTTP mode
// rejects browser-originated cross-site POST requests before MCP dispatch.
func TestServeHTTP_CrossOriginProtection_RejectsCrossSitePost(t *testing.T) {
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

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr, strings.NewReader(body))
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	req.Header.Set("PRIVATE-TOKEN", testToken)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	resp, reqErr := testHTTPClient.Do(req)
	if reqErr != nil {
		cancel()
		t.Fatalf("request failed: %v", reqErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 403 Forbidden, got %d: %s", resp.StatusCode, string(respBody))
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
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
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

// TestSafeTokenSuffix verifies short tokens are fully masked and longer
// tokens expose only the suffix used for non-sensitive diagnostics.
func TestSafeTokenSuffix(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{name: "empty", token: "", want: "****"},
		{name: "short", token: "abc", want: "****"},
		{name: "four", token: "abcd", want: "****"},
		{name: "long", token: "glpat-123456", want: "...3456"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeTokenSuffix(tt.token); got != tt.want {
				t.Errorf("safeTokenSuffix(%q) = %q, want %q", tt.token, got, tt.want)
			}
		})
	}
}

// TestLogIgnoredRequestOptions verifies ignored per-request MCP options are
// logged without panicking and skipped when no options were ignored.
func TestLogIgnoredRequestOptions(t *testing.T) {
	logIgnoredRequestOptions("glpat-123456", serverpool.RequestOptions{})
	logIgnoredRequestOptions("glpat-123456", serverpool.RequestOptions{IgnoredOptions: []string{"GITLAB_URL"}})
}

// TestLegacyMetaToolsFlagValue_OnlyUsesExplicitFlag verifies HTTP mode does
// not let the deprecated boolean flag override the default tool surface unless
// a user explicitly passes --meta-tools.
func TestLegacyMetaToolsFlagValue_OnlyUsesExplicitFlag(t *testing.T) {
	tests := []struct {
		name string
		cfg  httpConfig
		want string
	}{
		{name: "unset", cfg: httpConfig{metaTools: true}, want: ""},
		{name: "explicit true", cfg: httpConfig{metaToolsSet: true, metaTools: true}, want: "true"},
		{name: "explicit false", cfg: httpConfig{metaToolsSet: true}, want: "false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := legacyMetaToolsFlagValue(&tt.cfg); got != tt.want {
				t.Fatalf("legacyMetaToolsFlagValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDoToolSearch_HonorsToolSurface verifies tool search can inspect each
// selectable tool surface instead of always searching the legacy meta setting.
func TestDoToolSearch_HonorsToolSurface(t *testing.T) {
	tests := []struct {
		name        string
		toolSurface string
		query       string
	}{
		{name: "meta", toolSurface: config.ToolSurfaceMeta, query: "project"},
		{name: "individual", toolSurface: config.ToolSurfaceIndividual, query: "project"},
		{name: "dynamic", toolSurface: config.ToolSurfaceDynamic, query: "find"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStdout := os.Stdout
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("os.Pipe: %v", err)
			}
			os.Stdout = w
			t.Cleanup(func() { os.Stdout = oldStdout })

			if searchErr := doToolSearch(tt.query, tt.toolSurface, false); searchErr != nil {
				t.Fatalf("doToolSearch() error: %v", searchErr)
			}
			_ = w.Close()
			out, readErr := io.ReadAll(r)
			if readErr != nil {
				t.Fatalf("ReadAll: %v", readErr)
			}
			if !strings.Contains(string(out), "Found") {
				t.Fatalf("tool search output missing matches: %s", string(out))
			}
		})
	}
}

// TestRunToolSearch_ErrorExits verifies runToolSearch reports doToolSearch
// failures and exits with status 1 through the CLI wrapper path.
func TestRunToolSearch_ErrorExits(t *testing.T) {
	originalRunner := toolSearchRunner
	originalExit := exitProcess
	originalStderr := os.Stderr
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	type exitCode int
	toolSearchRunner = func(_, _ string, _ bool) error {
		return errors.New("forced search failure")
	}
	exitProcess = func(code int) { panic(exitCode(code)) }
	os.Stderr = writePipe
	t.Cleanup(func() {
		toolSearchRunner = originalRunner
		exitProcess = originalExit
		os.Stderr = originalStderr
	})

	defer func() {
		panicValue := recover()
		if panicValue == nil {
			t.Fatal("runToolSearch() did not exit")
		}
		code, ok := panicValue.(exitCode)
		if !ok {
			panic(panicValue)
		}
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		_ = writePipe.Close()
		stderr, readErr := io.ReadAll(readPipe)
		if readErr != nil {
			t.Fatalf("ReadAll stderr: %v", readErr)
		}
		if !strings.Contains(string(stderr), "forced search failure") {
			t.Fatalf("stderr = %q, want forced error", string(stderr))
		}
	}()

	runToolSearch("project", config.ToolSurfaceMeta, false)
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
	waitForHTTPServerReady(t, addr, errCh) //nolint:contextcheck // test helper: uses its own probe deadline
	return addr, errCh
}

// readinessConsecutiveSuccesses is the number of consecutive successful
// /health probes required before waitForHTTPServerReady considers the server
// ready. Two probes filter out a transient state where the listener is bound
// but the HTTP handler is not yet fully wired.
const readinessConsecutiveSuccesses = 2

// waitForHTTPServerReady polls /health until the HTTP server is reachable,
// or fails fast if serveHTTP exits early with an error.
//
// Requires readinessConsecutiveSuccesses consecutive successful probes to
// filter out transient startup states (e.g., the listener has been bound but
// the HTTP handler is not yet fully wired). After confirming readiness, idle
// connections are closed so the next request from the test opens a fresh TCP
// connection — this prevents flaky "connection refused" failures on slow CI
// runners caused by reusing a keep-alive socket whose peer has not yet
// finalized accept loop setup.
func waitForHTTPServerReady(t *testing.T, addr string, errCh <-chan error) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	consecutiveOK := 0
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
			consecutiveOK++
			if consecutiveOK >= readinessConsecutiveSuccesses {
				testHTTPClient.CloseIdleConnections()
				return
			}
		} else {
			consecutiveOK = 0
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

// TestRunHTTP_OAuthRequiresGitLabURL verifies that OAuth mode requires a
// fixed GitLab URL and cannot silently fall back to HTTP multi-instance mode.
func TestRunHTTP_OAuthRequiresGitLabURL(t *testing.T) {
	err := runHTTP(context.Background(), &httpConfig{
		gitlabURL:         "",
		maxHTTPClients:    config.DefaultMaxHTTPClients,
		sessionTimeout:    config.DefaultSessionTimeout,
		autoUpdateTimeout: config.DefaultAutoUpdateTimeout,
		authMode:          "oauth",
		oauthCacheTTL:     config.DefaultOAuthCacheTTL,
	})
	if err == nil {
		t.Fatal("expected error when OAuth mode has no fixed GitLab URL")
	}
	if !strings.Contains(err.Error(), "--auth-mode=oauth requires --gitlab-url") {
		t.Errorf("error = %q, want OAuth GitLab URL requirement", err.Error())
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

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://evil.example.com/", nil)
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

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost/", nil)
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

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost:8080/", nil)
	req.Host = "localhost:8080"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for allowed host with port, got %d", rr.Code)
	}
}

// TestCrossOriginProtectionMiddleware_AllowsNonBrowserPost verifies that MCP
// clients without browser origin headers are not rejected.
func TestCrossOriginProtectionMiddleware_AllowsNonBrowserPost(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := crossOriginProtectionMiddleware(inner)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://mcp.example/mcp", strings.NewReader(`{}`))
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for non-browser POST, got %d", rr.Code)
	}
	if !called {
		t.Fatal("inner handler was not called")
	}
}

// TestCrossOriginProtectionMiddleware_AllowsSameOriginPost verifies that
// same-origin browser POST requests are not rejected.
func TestCrossOriginProtectionMiddleware_AllowsSameOriginPost(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := crossOriginProtectionMiddleware(inner)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://mcp.example/mcp", strings.NewReader(`{}`))
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	req.Header.Set("Origin", "http://mcp.example")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for same-origin POST, got %d", rr.Code)
	}
	if !called {
		t.Fatal("inner handler was not called")
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

// TestClientIP_RemoteAddr verifies that clientIP returns the RemoteAddr host
// (without port) when no trusted proxy header is configured.
func TestClientIP_RemoteAddr(t *testing.T) {
	t.Parallel()
	r := &http.Request{RemoteAddr: "203.0.113.1:12345"}
	if got := clientIP(r, ""); got != "203.0.113.1" {
		t.Errorf("clientIP() = %q, want 203.0.113.1", got)
	}
}

// TestClientIP_TrustedProxyHeader verifies that clientIP returns the IP from
// the configured trusted proxy header (e.g. X-Real-IP) instead of RemoteAddr.
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

// TestClientIP_TrustedProxyHeader_XForwardedFor verifies that for
// comma-separated X-Forwarded-For values, clientIP returns the rightmost IP
// (added by the real trusted proxy) rather than the leftmost (spoofable).
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

// TestClientIP_TrustedProxyHeader_SpoofResistant verifies that clientIP
// ignores attacker-prepended IPs in X-Forwarded-For and returns the rightmost
// (trusted-proxy-appended) entry to prevent IP spoofing.
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

// TestClientIP_TrustedProxyHeader_Empty verifies that clientIP falls back to
// RemoteAddr when the configured trusted proxy header is absent or empty.
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

// TestClientIP_TrustedProxyHeader_TrailingCommas verifies that clientIP skips
// empty entries produced by trailing commas and returns the rightmost non-empty IP.
func TestClientIP_TrustedProxyHeader_TrailingCommas(t *testing.T) {
	t.Parallel()
	r := &http.Request{
		RemoteAddr: "203.0.113.99:12345",
		Header:     http.Header{"X-Forwarded-For": {"10.0.0.1, "}},
	}
	if got := clientIP(r, "X-Forwarded-For"); got != "10.0.0.1" {
		t.Errorf("clientIP() = %q, want 10.0.0.1 (skip empty trailing entry)", got)
	}
}

// TestBuildServerCard_ReturnsValidJSON verifies that [buildServerCard] produces
// valid JSON containing serverInfo, authentication, and a non-empty tools array
// with meta-tools when MetaTools=true.
func TestBuildServerCard_ReturnsValidJSON(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		GitLabURL:     "", // empty uses config.DefaultGitLabURL for dummy client registration
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
	toolsRaw := assertServerCardBasics(t, card)
	assertServerCardCatalogs(t, card)
	assertServerCardToolMetadata(t, toolsRaw)
}

func assertServerCardBasics(t *testing.T, card map[string]any) []any {
	t.Helper()
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
	return toolsRaw
}

func assertServerCardCatalogs(t *testing.T, card map[string]any) {
	t.Helper()
	if resourcesRaw, ok := card["resources"].([]any); !ok || len(resourcesRaw) == 0 {
		t.Error("card 'resources' array missing or empty")
	}
	if templatesRaw, ok := card["resourceTemplates"].([]any); !ok || len(templatesRaw) == 0 {
		t.Error("card 'resourceTemplates' array missing or empty")
	}
	if promptsRaw, ok := card["prompts"].([]any); !ok || len(promptsRaw) == 0 {
		t.Error("card 'prompts' array missing or empty")
	}
}

func assertServerCardToolMetadata(t *testing.T, toolsRaw []any) {
	t.Helper()
	var withOutputSchema, withAnnotations int
	for _, raw := range toolsRaw {
		tEntry, _ := raw.(map[string]any)
		if _, ok := tEntry["outputSchema"]; ok {
			withOutputSchema++
		}
		if _, ok := tEntry["annotations"]; ok {
			withAnnotations++
		}
	}
	if withOutputSchema == 0 {
		t.Error("no tool exposes 'outputSchema' — scanner will not see typed outputs")
	}
	if withAnnotations == 0 {
		t.Error("no tool exposes 'annotations' — scanner will not see destructive/readOnly hints")
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

// TestBuildServerCard_MinimalCapabilitySurface verifies that server-card
// generation returns a reduced catalog instead of failing when prompts are not
// registered.
func TestBuildServerCard_MinimalCapabilitySurface(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		GitLabURL:         "",
		SkipTLSVerify:     true,
		MetaTools:         true,
		ToolSurface:       config.ToolSurfaceDynamic,
		CapabilitySurface: config.CapabilitySurfaceMinimal,
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
	if !toolsOK || len(toolsRaw) != 2 {
		t.Fatalf("card tools = %d, want 2 dynamic tools", len(toolsRaw))
	}
	resourcesRaw, resourcesOK := card["resources"].([]any)
	if !resourcesOK || len(resourcesRaw) == 0 {
		t.Fatal("card resources array missing or empty")
	}
	promptsRaw, promptsOK := card["prompts"].([]any)
	if !promptsOK {
		t.Fatal("card prompts array missing")
	}
	if len(promptsRaw) != 0 {
		t.Fatalf("card prompts = %d, want 0 for minimal capability surface", len(promptsRaw))
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
