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
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/clientcompat"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
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

// createdServerKey identifies one cacheable createServer configuration by its
// scalar fields. The clientCompat bit captures the CLIENT_COMPAT env read at
// build time so the kill-switch test cannot collide with default-env builds
// of the same config.
type createdServerKey struct {
	metaTools         bool
	toolSurface       string
	capabilitySurface string
	tier              edition.Tier
	tierExplicit      bool
	readOnly          bool
	safeMode          bool
	metaParamSchema   string
	rateLimitRPS      float64
	rateLimitBurst    int
	clientCompat      bool
}

var (
	createdServersMu     sync.Mutex
	createdServers       = map[createdServerKey]*mcp.Server{}
	sharedServerClient   *gitlabclient.Client
	sharedServerClientMu sync.Mutex
)

// sharedCreateServerClient returns a mock GitLab client whose httptest
// backend lives for the whole test binary, so cached servers never point at
// a backend torn down by the test that first built them.
func sharedCreateServerClient(t *testing.T) *gitlabclient.Client {
	t.Helper()
	sharedServerClientMu.Lock()
	defer sharedServerClientMu.Unlock()
	if sharedServerClient != nil {
		return sharedServerClient
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/version" {
			w.Header().Set(hdrContentType, mimeJSON)
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "16.0.0", "revision": "test"})
			return
		}
		http.NotFound(w, r)
	}))
	// Process teardown reclaims the server; it must outlive every test.
	client, err := gitlabclient.NewClient(&config.Config{GitLabURL: srv.URL, GitLabToken: testToken})
	if err != nil {
		t.Fatalf("shared createServer client: %v", err)
	}
	sharedServerClient = client
	return client
}

// mustCreateServer builds a fully registered MCP server for cfg. Registration
// resolves the whole action catalog (~2s), and for configs without slices or
// per-test state the result depends only on the config (plus the
// CLIENT_COMPAT env), so those builds are shared per key against a
// binary-lifetime mock client; sessions attach per test. Callers that mutate
// the server or need their own backend must pass a cfg with
// ExcludeTools/TokenScopes set (never cached) or call createServer directly.
func mustCreateServer(t *testing.T, client *gitlabclient.Client, cfg *config.ServerConfig) *mcp.Server {
	t.Helper()
	cacheable := cfg.ExcludeTools == nil && cfg.TokenScopes == nil && cfg.GitLabURL == ""
	if !cacheable {
		server, err := createServer(client, cfg, nil)
		if err != nil {
			t.Fatalf("createServer() error: %v", err)
		}
		return server
	}
	key := createdServerKey{
		metaTools:         cfg.MetaTools,
		toolSurface:       cfg.ToolSurface,
		capabilitySurface: cfg.CapabilitySurface,
		tier:              cfg.Tier,
		tierExplicit:      cfg.TierExplicit,
		readOnly:          cfg.ReadOnly,
		safeMode:          cfg.SafeMode,
		metaParamSchema:   cfg.MetaParamSchema,
		rateLimitRPS:      cfg.RateLimitRPS,
		rateLimitBurst:    cfg.RateLimitBurst,
		clientCompat:      clientcompat.Enabled(),
	}
	createdServersMu.Lock()
	defer createdServersMu.Unlock()
	if server, ok := createdServers[key]; ok {
		return server
	}
	server, err := createServer(sharedCreateServerClient(t), cfg, nil)
	if err != nil {
		t.Fatalf("createServer() error: %v", err)
	}
	createdServers[key] = server
	return server
}

// newTestMCPServer returns a shared MCP server with the full individual tool
// catalog, resources, and prompts registered. HTTP protocol tests use it as a
// stable handler target for initialize and tools/list requests; none of them
// vary registration-time state, and the mock GitLab backend only answers
// /api/v4/version, so one registration (which resolves ~1,000 tool schemas in
// the MCP SDK) safely serves every caller. The backing httptest server lives
// for the whole test binary.
var (
	testMCPServerOnce   sync.Once
	testMCPServerShared *mcp.Server
	errTestMCPServer    error
)

func newTestMCPServer(t *testing.T) *mcp.Server {
	t.Helper()
	testMCPServerOnce.Do(func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v4/version" {
				w.Header().Set(hdrContentType, mimeJSON)
				_ = json.NewEncoder(w).Encode(map[string]string{"version": "16.0.0", "revision": "test"})
				return
			}
			http.NotFound(w, r)
		}))
		client, err := gitlabclient.NewClient(&config.Config{
			GitLabURL:   srv.URL,
			GitLabToken: testToken,
		})
		if err != nil {
			errTestMCPServer = err
			return
		}
		server := mcp.NewServer(&mcp.Implementation{
			Name:    serverName,
			Version: "test",
		}, nil)
		tools.RegisterAll(server, client, edition.Ultimate)
		resources.Register(server, client)
		prompts.Register(server, client)
		testMCPServerShared = server
	})
	if errTestMCPServer != nil {
		t.Fatalf("failed to create mock gitlab client: %v", errTestMCPServer)
	}
	return testMCPServerShared
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
		errCh <- serveHTTP(ctx, cfg, ":0", defaultHTTPIdleTimeout)
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
		errCh <- serveHTTP(ctx, cfg, addr, defaultHTTPIdleTimeout)
	}()

	select {
	case err = <-errCh:
		if err == nil {
			t.Fatal("serveHTTP() expected error for port conflict, got nil")
		}
	case <-time.After(testHTTPLivenessTimeout):
		t.Fatal("serveHTTP() did not return within timeout for port conflict")
	}
}

// TestRun_GitLabPingFailure_StartsDegraded verifies that a failing GitLab
// connectivity ping does not abort startup: [run] continues in degraded mode
// (the server must stay usable so the setup wizard and self-diagnostics can
// run). run then serves stdio until the test binary's stdin reports EOF, so
// the assertion accepts a clean exit or a transport-closed error but rejects
// a connectivity error and a hang. The old expectation (run returns an error
// on ping failure) predates degraded-mode startup and only passed through a
// test-order accident.
func TestRun_GitLabPingFailure_StartsDegraded(t *testing.T) {
	srv := newFailingGitLabServer(t, http.StatusForbidden)
	t.Setenv("GITLAB_URL", srv.URL)
	t.Setenv("GITLAB_TOKEN", testToken)
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "true")

	done := make(chan error, 1)
	go func() { done <- run(nil) }()
	select {
	case err := <-done:
		if err != nil && strings.Contains(err.Error(), "ping") {
			t.Fatalf("run() aborted on ping failure instead of starting degraded: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("run() did not return; expected stdio serve to end on closed stdin")
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
		{"meta-param-schema flag", "-meta-param-schema"},
		{"safe-mode flag", "-safe-mode"},
		{"embedded-resources flag", "-embedded-resources"},
		{"rate-limit-rps flag", "-rate-limit-rps"},
		{"rate-limit-burst flag", "-rate-limit-burst"},
		{"max-http-clients flag", "-max-http-clients"},
		{"session-timeout flag", "-session-timeout"},
		{"http-idle-timeout flag", "-http-idle-timeout"},
		{"stateless flag", "-stateless"},
		{"stateless default", "-stateless=false"},
		{"json-response flag", "-json-response"},
		{"max-request-body-bytes flag", "-max-request-body-bytes"},
		{"auto-update flag", "-auto-update"},
		{"env section", "ENVIRONMENT VARIABLES"},
		{"GITLAB_URL env", "GITLAB_URL"},
		{"GITLAB_TOKEN env", "GITLAB_TOKEN"},
		{"META_TOOLS env", "META_TOOLS"},
		{"META_PARAM_SCHEMA env", "META_PARAM_SCHEMA"},
		{"GITLAB_SAFE_MODE env", "GITLAB_SAFE_MODE"},
		{"EMBEDDED_RESOURCES env", "EMBEDDED_RESOURCES"},
		{"UPLOAD_MAX_FILE_SIZE env", "UPLOAD_MAX_FILE_SIZE"},
		{"RATE_LIMIT_RPS env", "RATE_LIMIT_RPS"},
		{"RATE_LIMIT_BURST env", "RATE_LIMIT_BURST"},
		{"YOLO_MODE env", "YOLO_MODE"},
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

// TestMain_HelpParsesTierFlag verifies the CLI registers and visits the
// --tier flag before returning through the help path.
func TestMain_HelpParsesTierFlag(t *testing.T) {
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine
	oldStdout := os.Stdout

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Args = []string{"gitlab-mcp-server", "-h", "-tier", "ultimate"}
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
	if projectWebsite == "" {
		t.Error("projectWebsite should not be empty")
	}
}

// TestResolveBuildVersion_Fallbacks verifies that an unstamped binary recovers
// its identity from the embedded module build info, and that -ldflags values
// always win. Release binaries are stamped from the VERSION file, so a build
// info value must never override one.
func TestResolveBuildVersion_Fallbacks(t *testing.T) {
	buildInfo := func(mainVersion, revision string) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) {
			info := &debug.BuildInfo{}
			info.Main.Version = mainVersion
			if revision != "" {
				info.Settings = []debug.BuildSetting{{Key: "vcs.revision", Value: revision}}
			}
			return info, true
		}
	}

	tests := []struct {
		name        string
		version     string
		commit      string
		readInfo    func() (*debug.BuildInfo, bool)
		wantVersion string
		wantCommit  string
	}{
		{
			name:        "go install records the module version",
			version:     "dev",
			commit:      "none",
			readInfo:    buildInfo("v2.6.6", "cafe1234"),
			wantVersion: "2.6.6",
			wantCommit:  "cafe1234",
		},
		{
			name:        "ldflags values are never overridden",
			version:     "2.6.6",
			commit:      "abc1234",
			readInfo:    buildInfo("v9.9.9", "deadbeef"),
			wantVersion: "2.6.6",
			wantCommit:  "abc1234",
		},
		{
			name:        "a working-tree build reports no module version",
			version:     "dev",
			commit:      "none",
			readInfo:    buildInfo("(devel)", "beef5678"),
			wantVersion: "dev",
			wantCommit:  "beef5678",
		},
		{
			name:        "build info without a revision leaves the commit alone",
			version:     "dev",
			commit:      "none",
			readInfo:    buildInfo("v2.6.6", ""),
			wantVersion: "2.6.6",
			wantCommit:  "none",
		},
		{
			name:        "unavailable build info changes nothing",
			version:     "dev",
			commit:      "none",
			readInfo:    func() (*debug.BuildInfo, bool) { return nil, false },
			wantVersion: "dev",
			wantCommit:  "none",
		},
		{
			name:        "nil build info with ok=true changes nothing",
			version:     "dev",
			commit:      "none",
			readInfo:    func() (*debug.BuildInfo, bool) { return nil, true },
			wantVersion: "dev",
			wantCommit:  "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVersion, gotCommit := resolveBuildVersion(tt.version, tt.commit, tt.readInfo)
			if gotVersion != tt.wantVersion {
				t.Errorf("version = %q, want %q", gotVersion, tt.wantVersion)
			}
			if gotCommit != tt.wantCommit {
				t.Errorf("commit = %q, want %q", gotCommit, tt.wantCommit)
			}
		})
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
	for _, uri := range []string{"gitlab://tools"} {
		if !resourceListHasURI(resources, uri) {
			t.Fatalf("resources = %+v, want %s", resources, uri)
		}
	}
	if wantFullCatalog {
		assertFullCatalogResources(t, resources)
		return
	}
	if len(resources) != 1 {
		t.Fatalf("minimal resources = %+v, want 1 resource", resources)
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

// TestCreateServer_EveryAdvertisedEntryIsFullyDescribed verifies that every
// entry the server advertises over MCP — the handshake Implementation, tools,
// resources, resource templates, prompts, and prompt arguments — carries the
// display metadata the catalog is meant to expose: a human-readable Title
// alongside the programmatic Name, and client Annotations on every resource
// and resource template.
//
// Both fields are optional in the MCP spec, so nothing in the SDK enforces
// them; external consumers that render the catalog (the server card at
// /.well-known/mcp/server-card.json, registry scanners, documentation
// generators) silently fall back to the machine name when Title is absent.
// This test walks the finalized catalog for all three tool surfaces and
// reports every incomplete entry at once, so a registration path that forgets
// to propagate the metadata cannot reach a release unnoticed.
//
// Resource Size is deliberately not asserted: it is only knowable ahead of a
// read for static content, and the API-backed resources correctly omit it.
func TestCreateServer_EveryAdvertisedEntryIsFullyDescribed(t *testing.T) {
	client := newMockGitLabClient(t)
	for _, toolSurface := range []string{config.ToolSurfaceIndividual, config.ToolSurfaceMeta, config.ToolSurfaceDynamic} {
		t.Run(toolSurface, func(t *testing.T) {
			server := mustCreateServer(t, client, &config.ServerConfig{
				MetaTools:         true,
				ToolSurface:       toolSurface,
				CapabilitySurface: config.CapabilitySurfaceFull,
			})
			assertEveryEntryIsFullyDescribed(t, newInMemorySession(t, server))
		})
	}
}

// assertEveryEntryIsFullyDescribed enumerates the full advertised catalog of
// session and fails with the complete list of entries missing display
// metadata. The paginating iterators are used rather than the single-page
// List* calls so that a catalog larger than one page is still covered end to
// end.
func assertEveryEntryIsFullyDescribed(t *testing.T, session *mcp.ClientSession) {
	t.Helper()

	gaps := &metadataGaps{t: t}
	collectServerInfoMetadataGaps(t, session, gaps)
	collectToolMetadataGaps(t, session, gaps)
	collectResourceMetadataGaps(t, session, gaps)
	collectResourceTemplateMetadataGaps(t, session, gaps)
	collectPromptMetadataGaps(t, session, gaps)

	if len(gaps.entries) > 0 {
		slices.Sort(gaps.entries)
		t.Errorf("%d advertised entries are missing display metadata:\n%s",
			len(gaps.entries), strings.Join(gaps.entries, "\n"))
	}
}

// metadataGaps accumulates the display-metadata omissions found while walking
// an advertised catalog so that a single failure can name all of them.
type metadataGaps struct {
	t       *testing.T
	entries []string
}

// require records a gap for kind/id when present is false.
func (g *metadataGaps) require(present bool, kind, id, field string) {
	g.t.Helper()
	if !present {
		g.entries = append(g.entries, kind+" "+id+": missing "+field)
	}
}

// collectServerInfoMetadataGaps checks the Implementation returned by the
// handshake. It is the first thing a client or registry renders, and unlike
// the catalog entries there is only one of it, so an omission here is easy to
// miss and expensive when it lands in a listing.
func collectServerInfoMetadataGaps(t *testing.T, session *mcp.ClientSession, gaps *metadataGaps) {
	t.Helper()
	info := session.InitializeResult().ServerInfo
	if info == nil {
		t.Fatal("handshake returned no ServerInfo")
	}
	gaps.require(info.Name != "", "server", "info", "Name")
	gaps.require(info.Title != "", "server", "info", "Title")
	gaps.require(info.Description != "", "server", "info", "Description")
	gaps.require(info.Version != "", "server", "info", "Version")
	gaps.require(info.WebsiteURL != "", "server", "info", "WebsiteURL")
	gaps.require(len(info.Icons) > 0, "server", "info", "Icons")
}

func collectToolMetadataGaps(t *testing.T, session *mcp.ClientSession, gaps *metadataGaps) {
	t.Helper()
	for tool, err := range session.Tools(t.Context(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		gaps.require(tool.Title != "", "tool", tool.Name, "Title")
		// Annotations.Title is the pre-2025-06-18 display name and is
		// superseded by the top-level Title asserted above. The meta surface
		// used to set both; keeping it unset holds all three tool surfaces on
		// the same rule instead of duplicating the string per tools/list entry.
		if tool.Annotations != nil && tool.Annotations.Title != "" {
			gaps.entries = append(gaps.entries,
				"tool "+tool.Name+": redundant Annotations.Title (top-level Title supersedes it)")
		}
	}
}

func collectResourceMetadataGaps(t *testing.T, session *mcp.ClientSession, gaps *metadataGaps) {
	t.Helper()
	for resource, err := range session.Resources(t.Context(), nil) {
		if err != nil {
			t.Fatalf("list resources: %v", err)
		}
		gaps.require(resource.Title != "", "resource", resource.URI, "Title")
		gaps.require(resource.Annotations != nil, "resource", resource.URI, "Annotations")
	}
}

func collectResourceTemplateMetadataGaps(t *testing.T, session *mcp.ClientSession, gaps *metadataGaps) {
	t.Helper()
	for template, err := range session.ResourceTemplates(t.Context(), nil) {
		if err != nil {
			t.Fatalf("list resource templates: %v", err)
		}
		gaps.require(template.Title != "", "resource template", template.URITemplate, "Title")
		gaps.require(template.Annotations != nil, "resource template", template.URITemplate, "Annotations")
	}
}

func collectPromptMetadataGaps(t *testing.T, session *mcp.ClientSession, gaps *metadataGaps) {
	t.Helper()
	for prompt, err := range session.Prompts(t.Context(), nil) {
		if err != nil {
			t.Fatalf("list prompts: %v", err)
		}
		gaps.require(prompt.Title != "", "prompt", prompt.Name, "Title")
		for _, arg := range prompt.Arguments {
			gaps.require(arg.Title != "", "prompt argument", prompt.Name+"."+arg.Name, "Title")
		}
	}
}

// TestCreateServer_DynamicReadOnlyRemovesExecute verifies that read-only mode
// keeps discovery but removes execution from the dynamic surface.
func TestCreateServer_DynamicReadOnlyKeepsExecuteForReadActions(t *testing.T) {
	client := newMockGitLabClient(t)
	server := mustCreateServer(t, client, &config.ServerConfig{MetaTools: true, ToolSurface: config.ToolSurfaceDynamic, ReadOnly: true})
	toolsResult, err := listRegisteredTools(server, "dynamic-readonly")
	if err != nil {
		t.Fatalf("list dynamic read-only tools: %v", err)
	}
	byName := make(map[string]*mcp.Tool, len(toolsResult))
	for _, tool := range toolsResult {
		byName[tool.Name] = tool
	}
	if _, found := byName["gitlab_find_action"]; !found {
		t.Fatal("read-only dynamic surface missing gitlab_find_action")
	}
	execute, found := byName["gitlab_execute_action"]
	if !found {
		t.Fatal("read-only dynamic surface dropped gitlab_execute_action: read actions would be unreachable")
	}
	if execute.Annotations == nil || !execute.Annotations.ReadOnlyHint {
		t.Error("gitlab_execute_action must advertise ReadOnlyHint in read-only mode so read-only pruning keeps it")
	}
	if execute.Annotations != nil && execute.Annotations.DestructiveHint != nil && *execute.Annotations.DestructiveHint {
		t.Error("gitlab_execute_action must not advertise DestructiveHint in read-only mode")
	}
	if len(byName) != 2 {
		t.Errorf("read-only dynamic surface registered %d tools, want exactly find+execute", len(byName))
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
	ceServer := mustCreateServer(t, client, &config.ServerConfig{MetaTools: true, Tier: edition.Free})
	ceSession := newInMemorySession(t, ceServer)

	_ = mustCreateServer(t, client, &config.ServerConfig{MetaTools: true, Tier: edition.Ultimate})

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

	// Build directly: the stubbed inspection hook must not be captured into
	// (or satisfied from) the shared mustCreateServer cache.
	server, err := createServer(client, &config.ServerConfig{MetaTools: true}, nil)
	if err != nil {
		t.Fatalf("createServer() error: %v", err)
	}
	session := newInMemorySession(t, server)
	if _, readErr := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "gitlab://tools"}); readErr == nil {
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
		errCh <- serveHTTP(ctx, cfg, addr, defaultHTTPIdleTimeout)
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
	respBody := readAndCloseBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d: %s", resp.StatusCode, respBody)
	}

	closeMCPSession(t, "http://"+addr, resp.Header.Get(hdrMCPSessionID))
	cancel()
	select {
	case err = <-errCh:
		if err != nil {
			t.Fatalf("serveHTTP error: %v", err)
		}
	case <-time.After(testHTTPLivenessTimeout):
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
		errCh <- serveHTTP(ctx, cfg, addr, defaultHTTPIdleTimeout)
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
	respBody := readAndCloseBody(t, resp)

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d: %s", resp.StatusCode, respBody)
	}

	cancel()
	select {
	case err = <-errCh:
		if err != nil {
			t.Fatalf("serveHTTP error: %v", err)
		}
	case <-time.After(testHTTPLivenessTimeout):
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
		errCh <- serveHTTP(ctx, cfg, addr, defaultHTTPIdleTimeout)
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
	respBody := readAndCloseBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d: %s", resp.StatusCode, respBody)
	}

	closeMCPSession(t, "http://"+addr, resp.Header.Get(hdrMCPSessionID))
	cancel()
	select {
	case err = <-errCh:
		if err != nil {
			t.Fatalf("serveHTTP error: %v", err)
		}
	case <-time.After(testHTTPLivenessTimeout):
		t.Fatal("serveHTTP did not shut down in time")
	}
}

// TestServeHTTP_MissingGitLabURLDefaultsToPublic verifies that in HTTP mode with
// no default --gitlab-url configured and no GITLAB-URL header, the request falls
// back to the public gitlab.com instance instead of being rejected (see
// serverpool.ResolveRequestOptions). TierExplicit and IgnoreScopes are set so the
// server-pool entry skips live tier/scope detection against gitlab.com, keeping
// the test hermetic (no outbound network) while still exercising the default-URL
// resolution path end to end.
func TestServeHTTP_MissingGitLabURLDefaultsToPublic(t *testing.T) {
	cfg := &config.Config{
		GitLabURL:      "",
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
		Tier:           edition.Free,
		TierExplicit:   true,
		IgnoreScopes:   true,
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
		errCh <- serveHTTP(ctx, cfg, addr, defaultHTTPIdleTimeout)
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
	respBody := readAndCloseBody(t, resp)

	// The assertion is that omitting GITLAB-URL is accepted rather than
	// rejected as a missing or malformed instance URL. It deliberately does
	// not assert 200.
	//
	// The default resolves to the public gitlab.com, so the outcome now
	// depends on what that instance says about the test token: with network
	// access the credential probe is refused and the gate answers 401,
	// without it the probe fails open and the handshake succeeds. Pinning
	// either one would make this test depend on the runner having internet.
	// The resolution semantics themselves are covered hermetically by
	// TestResolveRequestOptions in internal/serverpool.
	if resp.StatusCode == http.StatusBadRequest {
		t.Errorf("omitting GITLAB-URL was rejected as a bad request: %s", respBody)
	}
	if strings.Contains(respBody, "GITLAB-URL") {
		t.Errorf("response complains about the instance URL, so the default did not apply: %s", respBody)
	}
	if resp.StatusCode == http.StatusOK {
		closeMCPSession(t, "http://"+addr, resp.Header.Get(hdrMCPSessionID))
	}

	cancel()
	select {
	case err = <-errCh:
		if err != nil {
			t.Fatalf("serveHTTP error: %v", err)
		}
	case <-time.After(testHTTPLivenessTimeout):
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
		errCh <- serveHTTP(ctx, cfg, addr, defaultHTTPIdleTimeout)
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
	readAndCloseBody(t, resp)

	if resp.StatusCode == http.StatusOK {
		t.Error("expected non-200 for invalid GITLAB-URL header")
	}

	cancel()
	select {
	case err = <-errCh:
		if err != nil {
			t.Fatalf("serveHTTP error: %v", err)
		}
	case <-time.After(testHTTPLivenessTimeout):
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
		errCh <- serveHTTP(ctx, cfg, addr, defaultHTTPIdleTimeout)
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
	respBody := readAndCloseBody(t, resp)

	// The request gate rejects a credential-less POST before the MCP handler
	// runs, so the status is exactly 401 with an RFC 9110 challenge — not the
	// blanket 400 the SDK used to emit when the server factory returned nil.
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d: %s", resp.StatusCode, http.StatusUnauthorized, respBody)
	}
	if challenge := resp.Header.Get("WWW-Authenticate"); challenge == "" {
		t.Error("401 without WWW-Authenticate violates RFC 9110")
	}
	if !strings.Contains(respBody, "\"jsonrpc\"") {
		t.Errorf("body is not a JSON-RPC error: %s", respBody)
	}

	cancel()
	select {
	case err = <-errCh:
		if err != nil {
			t.Fatalf("serveHTTP error: %v", err)
		}
	case <-time.After(testHTTPLivenessTimeout):
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

// TestNewHealthResponse_UptimeAndStartedAt verifies the two liveness fields
// against controlled instants: started_at must be RFC 3339 in UTC, and
// uptime_seconds must be whole seconds since that instant.
//
// Passing both instants in is what makes this deterministic — reading the
// package-level clock would make the expected value a moving target and would
// race with any parallel test that wrote to it.
func TestNewHealthResponse_UptimeAndStartedAt(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		now        time.Time
		wantUptime int64
	}{
		{name: "same instant", now: start, wantUptime: 0},
		{name: "partial second truncates down", now: start.Add(1900 * time.Millisecond), wantUptime: 1},
		{name: "whole minute", now: start.Add(time.Minute), wantUptime: 60},
		{name: "two weeks", now: start.Add(14 * 24 * time.Hour), wantUptime: 1_209_600},
		{name: "observation before start is clamped", now: start.Add(-time.Hour), wantUptime: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := newHealthResponse(start, tc.now)
			if got.UptimeSeconds != tc.wantUptime {
				t.Errorf("uptime_seconds = %d, want %d", got.UptimeSeconds, tc.wantUptime)
			}
			if got.StartedAt != "2026-08-22T10:00:00Z" {
				t.Errorf("started_at = %q, want RFC 3339 UTC", got.StartedAt)
			}
		})
	}
}

// TestNewHealthResponse_StartedAtIsRFC3339InUTC pins the timestamp format and
// the UTC normalisation, so a non-UTC process clock cannot change the wire
// representation.
func TestNewHealthResponse_StartedAtIsRFC3339InUTC(t *testing.T) {
	t.Parallel()

	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	start := time.Date(2026, 8, 22, 12, 0, 0, 0, madrid) // 10:00 UTC

	got := newHealthResponse(start, start)
	parsed, err := time.Parse(time.RFC3339, got.StartedAt)
	if err != nil {
		t.Fatalf("started_at %q does not parse as RFC 3339: %v", got.StartedAt, err)
	}
	if !parsed.Equal(start) {
		t.Errorf("started_at = %v, want the same instant as %v", parsed, start)
	}
	if !strings.HasSuffix(got.StartedAt, "Z") {
		t.Errorf("started_at = %q, want a UTC (Z) offset regardless of process timezone", got.StartedAt)
	}
}

// TestHealthHandler_ExposesLivenessFields checks the wired endpoint: the two
// fields reach the JSON body, and they agree with each other.
func TestHealthHandler_ExposesLivenessFields(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	healthHandler(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil))

	var body healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	startedAt, err := time.Parse(time.RFC3339, body.StartedAt)
	if err != nil {
		t.Fatalf("started_at %q does not parse as RFC 3339: %v", body.StartedAt, err)
	}
	if startedAt.After(time.Now()) {
		t.Errorf("started_at %v is in the future", startedAt)
	}
	if body.UptimeSeconds < 0 {
		t.Errorf("uptime_seconds = %d, want a non-negative value", body.UptimeSeconds)
	}
	// The two fields describe the same interval, so uptime cannot outrun the
	// gap since started_at. A unit slip (milliseconds, nanoseconds) trips this.
	if elapsed := int64(time.Since(startedAt).Seconds()) + 1; body.UptimeSeconds > elapsed {
		t.Errorf("uptime_seconds = %d exceeds %d seconds elapsed since started_at", body.UptimeSeconds, elapsed)
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

// TestResolveHTTPTier verifies that the --tier flag resolves to the expected
// tier and explicit flag, including the unset (detect) and invalid cases.
func TestResolveHTTPTier(t *testing.T) {
	tests := []struct {
		name         string
		tier         string
		tierSet      bool
		wantTier     edition.Tier
		wantExplicit bool
		wantErr      bool
	}{
		{name: "unset detects", tierSet: false, wantTier: edition.Free, wantExplicit: false},
		{name: "free", tier: "free", tierSet: true, wantTier: edition.Free, wantExplicit: true},
		{name: "ce", tier: "ce", tierSet: true, wantTier: edition.Free, wantExplicit: true},
		{name: "premium", tier: "premium", tierSet: true, wantTier: edition.Premium, wantExplicit: true},
		{name: "ultimate", tier: "ultimate", tierSet: true, wantTier: edition.Ultimate, wantExplicit: true},
		{name: "invalid", tier: "platinum", tierSet: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hcfg := &httpConfig{tier: tt.tier, tierSet: tt.tierSet}
			tier, explicit, err := resolveHTTPTier(hcfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("resolveHTTPTier() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveHTTPTier() error = %v", err)
			}
			if tier != tt.wantTier {
				t.Errorf("tier = %v, want %v", tier, tt.wantTier)
			}
			if explicit != tt.wantExplicit {
				t.Errorf("explicit = %v, want %v", explicit, tt.wantExplicit)
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

			if searchErr := doToolSearch(tt.query, tt.toolSurface, edition.Free); searchErr != nil {
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
	toolSearchRunner = func(_, _ string, _ edition.Tier) error {
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

	runToolSearch("project", config.ToolSurfaceMeta, edition.Free)
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
		errCh <- serveHTTP(ctx, cfg, addr, defaultHTTPIdleTimeout)
	}()
	waitForHTTPServerReady(t, addr, errCh) //nolint:contextcheck // test helper: uses its own probe deadline
	return addr, errCh
}

// readinessConsecutiveSuccesses is the number of consecutive successful
// /health probes required before waitForHTTPServerReady considers the server
// ready. Two probes filter out a transient state where the listener is bound
// but the HTTP handler is not yet fully wired.
const readinessConsecutiveSuccesses = 2

// testHTTPLivenessTimeout bounds how long the HTTP-server tests wait for a
// liveness transition — readiness probing and graceful shutdown. It is
// deliberately generous: a healthy server reaches these states in
// milliseconds, so a larger budget never slows a passing test and only
// tolerates scheduling stalls under the race detector or on a loaded CI
// runner. Using a single shared value keeps these waits deterministic instead
// of flaking against a tight fixed 5s budget.
const testHTTPLivenessTimeout = 30 * time.Second

// readAndCloseBody consumes the response body and closes it immediately,
// returning the contents for assertions.
//
// Callers that later cancel a serveHTTP context MUST use this instead of a
// deferred Close. http.Server.Shutdown waits for connections to return to
// idle, and a connection whose response body the client has not consumed is
// not idle: a deferred close still holds it open while Shutdown is running, so
// Shutdown burns its whole budget and returns "context deadline exceeded".
// Locally the client has usually drained a small body already and the race is
// invisible; on a loaded CI runner it is not.
func readAndCloseBody(t *testing.T, resp *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil {
		t.Errorf("closing response body: %v", closeErr)
	}
	if err != nil {
		t.Errorf("reading response body: %v", err)
	}
	return string(body)
}

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

	deadline := time.Now().Add(testHTTPLivenessTimeout)
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
	case <-time.After(testHTTPLivenessTimeout):
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
	case <-time.After(testHTTPLivenessTimeout):
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
	// parseJSONRPCResponse consumes the body below, which is what returns the
	// connection to idle before the context is cancelled.
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
	case <-time.After(testHTTPLivenessTimeout):
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
	respBody := readAndCloseBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK (PRIVATE-TOKEN converted to Bearer), got %d: %s", resp.StatusCode, respBody)
	}

	closeMCPSession(t, "http://"+addr, resp.Header.Get(hdrMCPSessionID))
	cancel()
	select {
	case srvErr := <-errCh:
		if srvErr != nil {
			t.Fatalf("serveHTTP error: %v", srvErr)
		}
	case <-time.After(testHTTPLivenessTimeout):
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
	case <-time.After(testHTTPLivenessTimeout):
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
	case <-time.After(testHTTPLivenessTimeout):
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

// TestRemoveNonReadOnlyTools verifies that tools.RemoveNonReadOnlyTools strips
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

	removed := tools.RemoveNonReadOnlyTools(server)
	if removed != 1 {
		t.Errorf("RemoveNonReadOnlyTools removed %d tools, want 1", removed)
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
		errCh <- serveHTTP(ctx, cfg, addr, defaultHTTPIdleTimeout)
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
	case <-time.After(testHTTPLivenessTimeout):
		t.Fatal("serveHTTP did not shut down in time")
	}
}

// TestEffectiveIdleTimeout verifies the mapping from the raw --http-idle-timeout
// value to the duration applied to http.Server.IdleTimeout. The key case is 0,
// which must map to the disabled sentinel (not Go's ReadTimeout fallback) so that
// long-lived Streamable HTTP (SSE) connections are not severed; any positive value
// must pass through unchanged.
func TestEffectiveIdleTimeout(t *testing.T) {
	tests := []struct {
		name  string
		input time.Duration
		want  time.Duration
	}{
		{"zero disables via sentinel", 0, idleTimeoutDisabled},
		{"positive passes through", 30 * time.Second, 30 * time.Second},
		{"large value passes through", 30 * time.Minute, 30 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveIdleTimeout(tt.input); got != tt.want {
				t.Errorf("effectiveIdleTimeout(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

// TestNewHTTPServer_Timeouts verifies that newHTTPServer applies the timeout
// policy: fixed request-read guards, IdleTimeout from effectiveIdleTimeout, and a
// fixed global WriteTimeout (slow-write guard) regardless of the idle timeout. The
// long-lived SSE write deadline is disabled separately by sseWriteDeadlineMiddleware.
func TestNewHTTPServer_Timeouts(t *testing.T) {
	tests := []struct {
		name     string
		idle     time.Duration
		wantIdle time.Duration
	}{
		{"disabled (0)", 0, idleTimeoutDisabled},
		{"short idle", 30 * time.Second, 30 * time.Second},
		{"long idle", 30 * time.Minute, 30 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newHTTPServer(":0", http.NewServeMux(), tt.idle)
			if srv.IdleTimeout != tt.wantIdle {
				t.Errorf("IdleTimeout = %s, want %s", srv.IdleTimeout, tt.wantIdle)
			}
			if srv.WriteTimeout != baseHTTPWriteTimeout {
				t.Errorf("WriteTimeout = %s, want fixed %s", srv.WriteTimeout, baseHTTPWriteTimeout)
			}
			if srv.ReadHeaderTimeout != baseHTTPReadHeaderTimeout {
				t.Errorf("ReadHeaderTimeout = %s, want %s", srv.ReadHeaderTimeout, baseHTTPReadHeaderTimeout)
			}
			if srv.ReadTimeout != baseHTTPReadTimeout {
				t.Errorf("ReadTimeout = %s, want %s", srv.ReadTimeout, baseHTTPReadTimeout)
			}
		})
	}
}

// TestSSEWriteDeadlineMiddleware_PassesThrough verifies that the middleware
// forwards every request to the wrapped handler — the long-lived SSE streams
// whose write deadline it clears (the standalone GET and streamed POST responses,
// both of which negotiate text/event-stream) and ordinary requests that keep
// WriteTimeout.
func TestSSEWriteDeadlineMiddleware_PassesThrough(t *testing.T) {
	tests := []struct {
		name   string
		method string
		accept string
	}{
		{"sse get stream", http.MethodGet, "text/event-stream"},
		{"plain get", http.MethodGet, "application/json"},
		{"mcp post", http.MethodPost, "application/json, text/event-stream"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			h := sseWriteDeadlineMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequestWithContext(context.Background(), tt.method, "/mcp", nil)
			req.Header.Set("Accept", tt.accept)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if !called {
				t.Error("wrapped handler was not called")
			}
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
			}
		})
	}
}

// TestRunHTTP_NegativeIdleTimeout_Rejected verifies that runHTTP rejects a
// negative --http-idle-timeout with an actionable error.
func TestRunHTTP_NegativeIdleTimeout_Rejected(t *testing.T) {
	err := runHTTP(context.Background(), &httpConfig{
		gitlabURL:       "https://gitlab.example.com",
		maxHTTPClients:  config.DefaultMaxHTTPClients,
		sessionTimeout:  config.DefaultSessionTimeout,
		httpIdleTimeout: -1 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error for negative http-idle-timeout")
	}
	if !strings.Contains(err.Error(), "http-idle-timeout") {
		t.Errorf("error should mention http-idle-timeout, got: %v", err)
	}
}

// TestDefaultHTTPIdleTimeout_DisabledByDefault documents that the default value
// disables HTTP-layer idle closure (0), so --session-timeout governs idle session
// lifetime out of the box.
func TestDefaultHTTPIdleTimeout_DisabledByDefault(t *testing.T) {
	if defaultHTTPIdleTimeout != 0 {
		t.Errorf("defaultHTTPIdleTimeout = %s, want 0 (disabled)", defaultHTTPIdleTimeout)
	}
	if got := effectiveIdleTimeout(defaultHTTPIdleTimeout); got != idleTimeoutDisabled {
		t.Errorf("effectiveIdleTimeout(default) = %s, want %s", got, idleTimeoutDisabled)
	}
}

// TestConfigFromHTTPFlags_StatelessJSONResponse_Propagated verifies that the
// --stateless and --json-response CLI flags are carried into config.Config so
// the streamable HTTP handler options can consume them.
//
// The zero-value httpConfig case asserts struct-to-struct mapping only: the
// flag layer defaults -stateless to true, while a zero httpConfig (as built
// directly in tests) still maps to Stateless=false.
func TestConfigFromHTTPFlags_StatelessJSONResponse_Propagated(t *testing.T) {
	hcfg := &httpConfig{stateless: true, jsonResponse: true, maxRequestBodyBytes: 2048}
	cfg := configFromHTTPFlags(hcfg, "", false, edition.Free, false)
	if !cfg.Stateless {
		t.Error("configFromHTTPFlags() Stateless = false, want true")
	}
	if !cfg.JSONResponse {
		t.Error("configFromHTTPFlags() JSONResponse = false, want true")
	}
	if cfg.MaxRequestBodyBytes != 2048 {
		t.Errorf("configFromHTTPFlags() MaxRequestBodyBytes = %d, want 2048", cfg.MaxRequestBodyBytes)
	}
	defaults := configFromHTTPFlags(&httpConfig{}, "", false, edition.Free, false)
	if defaults.Stateless || defaults.JSONResponse {
		t.Errorf("configFromHTTPFlags() defaults: Stateless=%v JSONResponse=%v, want false/false",
			defaults.Stateless, defaults.JSONResponse)
	}
}

// TestStreamableHTTPOptions_MapsConfigFields verifies that the shared handler
// options builder copies session timeout, stateless mode, and JSON response
// mode from config so the legacy and OAuth paths cannot drift apart.
func TestStreamableHTTPOptions_MapsConfigFields(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *config.Config
		stateless    bool
		jsonResponse bool
	}{
		{"defaults", &config.Config{SessionTimeout: config.DefaultSessionTimeout}, false, false},
		{"stateless", &config.Config{Stateless: true}, true, false},
		{"json_response", &config.Config{JSONResponse: true}, false, true},
		{"both", &config.Config{Stateless: true, JSONResponse: true}, true, true},
		{"body_limit", &config.Config{MaxRequestBodyBytes: 1024}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := streamableHTTPOptions(tt.cfg)
			if opts.SessionTimeout != tt.cfg.SessionTimeout {
				t.Errorf("SessionTimeout = %v, want %v", opts.SessionTimeout, tt.cfg.SessionTimeout)
			}
			if opts.Stateless != tt.stateless {
				t.Errorf("Stateless = %v, want %v", opts.Stateless, tt.stateless)
			}
			if opts.JSONResponse != tt.jsonResponse {
				t.Errorf("JSONResponse = %v, want %v", opts.JSONResponse, tt.jsonResponse)
			}
			if !opts.PropagateRequestCancellation {
				t.Error("PropagateRequestCancellation = false, want always true")
			}
			if opts.MaxRequestBodyBytes != tt.cfg.MaxRequestBodyBytes {
				t.Errorf("MaxRequestBodyBytes = %d, want %d", opts.MaxRequestBodyBytes, tt.cfg.MaxRequestBodyBytes)
			}
		})
	}
}

// startStatelessServeHTTP boots serveHTTP with the given config on a free
// port and returns the address plus a shutdown func that asserts clean exit.
func startStatelessServeHTTP(t *testing.T, cfg *config.Config) (string, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTP(ctx, cfg, addr, defaultHTTPIdleTimeout)
	}()
	waitForHTTPServerReady(t, addr, errCh)

	return addr, func() {
		cancel()
		select {
		case serveErr := <-errCh:
			if serveErr != nil {
				t.Fatalf("serveHTTP error: %v", serveErr)
			}
		case <-time.After(testHTTPLivenessTimeout):
			t.Fatal("serveHTTP did not shut down in time")
		}
	}
}

// statelessTestConfig returns an HTTP-mode config in stateless mode with the
// dynamic tool surface, backed by the given mock GitLab server URL.
func statelessTestConfig(gitlabURL string, jsonResponse bool) *config.Config {
	return &config.Config{
		GitLabURL:      gitlabURL,
		ToolSurface:    config.ToolSurfaceDynamic,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		Stateless:      true,
		JSONResponse:   jsonResponse,
	}
}

// postStatelessJSONRPC sends one self-contained JSON-RPC POST (no session
// header) and returns the response. Callers own resp.Body.
func postStatelessJSONRPC(t *testing.T, addr, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set(hdrContentType, mimeJSON)
	req.Header.Set("Accept", mimeJSONSSE)
	req.Header.Set("PRIVATE-TOKEN", testToken)
	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

// TestServeHTTP_Stateless_ToolsListWithoutSession verifies that in stateless
// mode a tools/list POST succeeds without any prior initialize call, the
// response carries no Mcp-Session-Id header, and the default response body is
// an SSE stream (text/event-stream).
func TestServeHTTP_Stateless_ToolsListWithoutSession(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	addr, shutdown := startStatelessServeHTTP(t, statelessTestConfig(mockGL.URL, false))
	defer shutdown()

	resp := postStatelessJSONRPC(t, addr, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get(hdrMCPSessionID); got != "" {
		t.Errorf("stateless response must not set %s, got %q", hdrMCPSessionID, got)
	}
	if ct := resp.Header.Get(hdrContentType); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("default stateless Content-Type = %q, want text/event-stream", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "gitlab_find_action") {
		t.Errorf("tools/list body does not mention gitlab_find_action: %s", string(body))
	}
}

// TestServeHTTP_Stateless_GETMethodNotAllowed verifies that stateless mode
// rejects GET on the MCP endpoint with 405 and an Allow: POST header, per the
// sessionless streamable HTTP semantics of SEP-2567.
func TestServeHTTP_Stateless_GETMethodNotAllowed(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	addr, shutdown := startStatelessServeHTTP(t, statelessTestConfig(mockGL.URL, false))
	defer shutdown()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("PRIVATE-TOKEN", testToken)
	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want 405", resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); allow != http.MethodPost {
		t.Errorf("Allow header = %q, want POST", allow)
	}
}

// TestServeHTTP_Stateless_JSONResponseContentType verifies that combining
// --stateless with --json-response yields plain application/json bodies with a
// parseable JSON-RPC result instead of an SSE stream.
func TestServeHTTP_Stateless_JSONResponseContentType(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	addr, shutdown := startStatelessServeHTTP(t, statelessTestConfig(mockGL.URL, true))
	defer shutdown()

	resp := postStatelessJSONRPC(t, addr, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(body))
	}
	if ct := resp.Header.Get(hdrContentType); !strings.HasPrefix(ct, mimeJSON) {
		t.Errorf("Content-Type = %q, want %s", ct, mimeJSON)
	}
	var rpc struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		t.Fatalf("decode JSON-RPC response: %v", err)
	}
	names := make([]string, 0, len(rpc.Result.Tools))
	for _, tool := range rpc.Result.Tools {
		names = append(names, tool.Name)
	}
	if len(names) == 0 || !slices.Contains(names, "gitlab_find_action") {
		t.Errorf("tools/list names = %v, want gitlab_find_action present", names)
	}
}

// TestServeHTTP_Stateless_ToolCallSucceeds verifies an end-to-end tools/call
// in stateless JSON mode: gitlab_find_action resolves catalog matches without
// any session, proving the pooled server executes tools per request.
func TestServeHTTP_Stateless_ToolCallSucceeds(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	addr, shutdown := startStatelessServeHTTP(t, statelessTestConfig(mockGL.URL, true))
	defer shutdown()

	resp := postStatelessJSONRPC(t, addr,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"gitlab_find_action","arguments":{"query":"list projects"}}}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get(hdrMCPSessionID); got != "" {
		t.Errorf("stateless response must not set %s, got %q", hdrMCPSessionID, got)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "gitlab_execute_action") && !strings.Contains(string(body), "project") {
		t.Errorf("tools/call gitlab_find_action body has no catalog matches: %s", string(body))
	}
}

// TestServeHTTP_ToolsList_ExecuteActionCarriesXMCPHeader verifies that the
// SEP-2243 x-mcp-header annotation on gitlab_execute_action survives the full
// registration and serialization path: it must be visible in the raw tools/list
// payload, because that is where MCP-aware gateways read routing annotations.
func TestServeHTTP_ToolsList_ExecuteActionCarriesXMCPHeader(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	addr, shutdown := startStatelessServeHTTP(t, statelessTestConfig(mockGL.URL, true))
	defer shutdown()

	resp := postStatelessJSONRPC(t, addr, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(body))
	}
	var rpc struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				InputSchema struct {
					Properties map[string]struct {
						XMCPHeader string `json:"x-mcp-header"`
					} `json:"properties"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		t.Fatalf("decode JSON-RPC response: %v", err)
	}
	for _, tool := range rpc.Result.Tools {
		if tool.Name != "gitlab_execute_action" {
			continue
		}
		if got := tool.InputSchema.Properties["action"].XMCPHeader; got != "Mcp-Param-Action" {
			t.Fatalf("gitlab_execute_action action x-mcp-header = %q, want Mcp-Param-Action", got)
		}
		return
	}
	t.Fatal("gitlab_execute_action not present in tools/list")
}

// TestValidateHTTPRuntimeConfig_NegativeBodyLimit_Rejected verifies the CLI
// refuses negative --max-request-body-bytes: the SDK would interpret it as
// "no limit", which must be unreachable from configuration.
func TestValidateHTTPRuntimeConfig_NegativeBodyLimit_Rejected(t *testing.T) {
	cfg := &config.Config{MaxRequestBodyBytes: -1}
	if err := validateHTTPRuntimeConfig(cfg); err == nil {
		t.Fatal("expected error for negative --max-request-body-bytes")
	}
}

// TestServeHTTP_Stateless_BodyLimitReturns413 verifies that
// --max-request-body-bytes is enforced by the streamable HTTP transport: a
// JSON-RPC POST larger than the configured limit is rejected with 413 instead
// of being parsed.
func TestServeHTTP_Stateless_BodyLimitReturns413(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	cfg := statelessTestConfig(mockGL.URL, true)
	cfg.MaxRequestBodyBytes = 512
	addr, shutdown := startStatelessServeHTTP(t, cfg)
	defer shutdown()

	oversized := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitlab_find_action","arguments":{"query":"` +
		strings.Repeat("x", 1024) + `"}}}`
	resp := postStatelessJSONRPC(t, addr, oversized)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d, want 413: %s", resp.StatusCode, string(body))
	}
}

// TestServeHTTP_CacheHints_ToolsListPrivate verifies that the SEP-2549 cache
// hints applied by the receiving middleware survive the full server-creation
// and serialization path: a stateless tools/list response carries
// cacheScope=private and the 5-minute list TTL.
func TestServeHTTP_CacheHints_ToolsListPrivate(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	addr, shutdown := startStatelessServeHTTP(t, statelessTestConfig(mockGL.URL, true))
	defer shutdown()

	resp := postStatelessJSONRPC(t, addr, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(body))
	}
	var rpc struct {
		Result struct {
			TTLMs      int    `json:"ttlMs"`
			CacheScope string `json:"cacheScope"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		t.Fatalf("decode JSON-RPC response: %v", err)
	}
	if rpc.Result.CacheScope != "private" {
		t.Errorf("cacheScope = %q, want private", rpc.Result.CacheScope)
	}
	if rpc.Result.TTLMs != 300000 {
		t.Errorf("ttlMs = %d, want 300000", rpc.Result.TTLMs)
	}
}

// TestServeHTTP_StatefulOptOut_SessionHeaderPresent verifies that stateful
// mode (--stateless=false) still issues Mcp-Session-Id, so the legacy
// session-based transport remains fully functional as an opt-out.
func TestServeHTTP_StatefulOptOut_SessionHeaderPresent(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	cfg := statelessTestConfig(mockGL.URL, false)
	cfg.Stateless = false
	addr, shutdown := startStatelessServeHTTP(t, cfg)
	defer shutdown()

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	resp := postStatelessJSONRPC(t, addr, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("initialize status = %d: %s", resp.StatusCode, payload)
	}
	sessionID := resp.Header.Get(hdrMCPSessionID)
	if sessionID == "" {
		t.Fatal("stateful opt-out must set Mcp-Session-Id, got empty")
	}
	closeMCPSession(t, "http://"+addr, sessionID)
}

// headerRoundTripper injects a static header set into every outgoing request.
type headerRoundTripper struct {
	base   http.RoundTripper
	header http.Header
}

func (h headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for key, values := range h.header {
		for _, value := range values {
			clone.Header.Set(key, value)
		}
	}
	return h.base.RoundTrip(clone)
}

// TestServeHTTP_Stateless_SDKClientInterop verifies that a real MCP go-sdk
// client completes the full handshake and tool calls against a stateless
// server: Connect (initialize), ListTools, and CallTool all succeed even
// though the server never issues an Mcp-Session-Id. This guards real-client
// interop beyond the raw JSON-RPC POSTs used by the other stateless tests.
func TestServeHTTP_Stateless_SDKClientInterop(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	addr, shutdown := startStatelessServeHTTP(t, statelessTestConfig(mockGL.URL, false))
	defer shutdown()

	transport := &mcp.StreamableClientTransport{
		Endpoint: "http://" + addr,
		HTTPClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: headerRoundTripper{base: http.DefaultTransport, header: http.Header{"Private-Token": {testToken}}},
		},
		// Stateless servers reject GET (405), so the optional standalone SSE
		// stream for server-initiated messages must stay off.
		DisableStandaloneSSE: true,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "stateless-interop-test", Version: "0.0.1"}, nil)
	session, err := client.Connect(t.Context(), transport, nil)
	if err != nil {
		t.Fatalf("SDK client Connect over stateless HTTP: %v", err)
	}
	// Close sends DELETE, which stateless servers answer with 405; the error
	// is irrelevant to this test.
	defer func() { _ = session.Close() }()

	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	if !slices.Contains(names, "gitlab_find_action") {
		t.Fatalf("ListTools = %v, want gitlab_find_action", names)
	}

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "gitlab_find_action",
		Arguments: map[string]any{"query": "list projects"},
	})
	if err != nil {
		t.Fatalf("CallTool gitlab_find_action: %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool returned IsError: %+v", result.Content)
	}
}

// modeTestSession opens an in-memory MCP session against a server built with
// cfg, so mode tests exercise the real dispatch path instead of inspecting
// registration metadata.
func modeTestSession(t *testing.T, cfg *config.ServerConfig) *mcp.ClientSession {
	t.Helper()
	server := mustCreateServer(t, newMockGitLabClient(t), cfg)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connect mode test server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "mode-test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect mode test client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// callModeTool calls one tool on session and returns its concatenated text
// content, failing the test only on transport errors: a blocked or rejected
// call is a result to assert on, not a failure.
func callModeTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	var text strings.Builder
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			text.WriteString(textContent.Text)
		}
	}
	return text.String()
}

// safeModeBlocked reports whether text is a safe-mode preview naming action.
func safeModeBlocked(t *testing.T, text, action string) bool {
	t.Helper()
	var preview tools.SafeModePreview
	if err := json.Unmarshal([]byte(text), &preview); err != nil {
		return false
	}
	return preview.Status == "blocked" && preview.Mode == "safe" && preview.Tool == action
}

// TestCreateServer_SafeMode_DynamicSurfacePreviewsWritesAndRunsReads verifies
// that safe mode on the dynamic surface intercepts per action: a mutating
// action returns a preview naming that action, while a read-only action
// executes and reaches GitLab (the mock answers 404, which only a real call
// can produce). Regression test for safe mode blocking every dynamic read
// because gitlab_execute_action itself is not read-only.
func TestCreateServer_SafeMode_DynamicSurfacePreviewsWritesAndRunsReads(t *testing.T) {
	session := modeTestSession(t, &config.ServerConfig{ToolSurface: config.ToolSurfaceDynamic, SafeMode: true})

	write := callModeTool(t, session, "gitlab_execute_action", map[string]any{
		"action": "issue.create",
		"params": map[string]any{"project_id": "1", "title": "safe mode issue"},
	})
	if !safeModeBlocked(t, write, "issue.create") {
		t.Errorf("mutating dynamic action was not previewed as issue.create: %s", write)
	}

	read := callModeTool(t, session, "gitlab_execute_action", map[string]any{
		"action": "project.list",
		"params": map[string]any{},
	})
	if safeModeBlocked(t, read, "project.list") {
		t.Errorf("read-only dynamic action was blocked by safe mode: %s", read)
	}
	if !strings.Contains(read, "projectList") {
		t.Errorf("read-only dynamic action did not reach the GitLab client: %s", read)
	}
}

// TestCreateServer_SafeMode_MetaSurfacePreviewsWritesAndRunsReads verifies the
// same per-action interception through a meta-tool dispatcher, whose single
// tool covers both reads and writes of one domain.
func TestCreateServer_SafeMode_MetaSurfacePreviewsWritesAndRunsReads(t *testing.T) {
	session := modeTestSession(t, &config.ServerConfig{ToolSurface: config.ToolSurfaceMeta, SafeMode: true})

	write := callModeTool(t, session, "gitlab_issue", map[string]any{
		"action": "create",
		"params": map[string]any{"project_id": "1", "title": "safe mode issue"},
	})
	if !safeModeBlocked(t, write, "issue.create") {
		t.Errorf("mutating meta action was not previewed as issue.create: %s", write)
	}

	read := callModeTool(t, session, "gitlab_issue", map[string]any{
		"action": "list",
		"params": map[string]any{"project_id": "1"},
	})
	if safeModeBlocked(t, read, "issue.list") {
		t.Errorf("read-only meta action was blocked by safe mode: %s", read)
	}
	if !strings.Contains(read, "issueList") {
		t.Errorf("read-only meta action did not reach the GitLab client: %s", read)
	}
}

// TestCreateServer_SafeMode_IndividualSurfaceStillWrapsTools verifies that the
// individual surface keeps its tool-level interception: one tool is one action
// there, so wrapping is already action-granular.
func TestCreateServer_SafeMode_IndividualSurfaceStillWrapsTools(t *testing.T) {
	session := modeTestSession(t, &config.ServerConfig{ToolSurface: config.ToolSurfaceIndividual, SafeMode: true})

	write := callModeTool(t, session, "gitlab_issue_create", map[string]any{
		"project_id": "1",
		"title":      "safe mode issue",
	})
	if !safeModeBlocked(t, write, "gitlab_issue_create") {
		t.Errorf("mutating individual tool was not previewed: %s", write)
	}

	read := callModeTool(t, session, "gitlab_issue_list", map[string]any{"project_id": "1"})
	if !strings.Contains(read, "issueList") {
		t.Errorf("read-only individual tool did not reach the GitLab client: %s", read)
	}
}

// TestCreateServer_SafeMode_MetaSurfaceStillWrapsStandaloneTools verifies that
// tools registered outside the action catalog — the gitlab_interactive_*
// utilities on the meta surface — are still intercepted. Exempting the
// catalog-backed dispatchers from tool-level wrapping must not exempt anything
// else, or safe mode would let those utilities mutate GitLab for real.
func TestCreateServer_SafeMode_MetaSurfaceStillWrapsStandaloneTools(t *testing.T) {
	session := modeTestSession(t, &config.ServerConfig{ToolSurface: config.ToolSurfaceMeta, SafeMode: true})

	text := callModeTool(t, session, "gitlab_interactive_project_create", map[string]any{})
	if !safeModeBlocked(t, text, "gitlab_interactive_project_create") {
		t.Errorf("standalone interactive tool was not intercepted by safe mode: %s", text)
	}
}

// TestCreateServer_SafeMode_PreviewCarriesCallArguments verifies the preview
// echoes the would-be arguments, which is what makes it reviewable, and that
// destructive actions are previewed without demanding confirmation first
// (nothing executes, so there is nothing to confirm).
func TestCreateServer_SafeMode_PreviewCarriesCallArguments(t *testing.T) {
	session := modeTestSession(t, &config.ServerConfig{ToolSurface: config.ToolSurfaceDynamic, SafeMode: true})

	text := callModeTool(t, session, "gitlab_execute_action", map[string]any{
		"action": "project.delete",
		"params": map[string]any{"project_id": "42"},
	})
	var preview tools.SafeModePreview
	if err := json.Unmarshal([]byte(text), &preview); err != nil {
		t.Fatalf("destructive action did not return a preview: %s", text)
	}
	if preview.Tool != "project.delete" {
		t.Errorf("preview tool = %q, want project.delete", preview.Tool)
	}
	if !strings.Contains(string(preview.Params), `"project_id":"42"`) {
		t.Errorf("preview params = %s, want the submitted arguments", preview.Params)
	}
	if preview.Hint == "" {
		t.Error("preview hint is empty; operators need to know how to disable safe mode")
	}
}

// TestCreateServer_ReadOnly_DynamicSurfaceRunsReadsAndRejectsWrites verifies
// that read-only mode on the dynamic surface keeps read actions executable
// through gitlab_execute_action while mutating action IDs are not routable at
// all. Regression test for read-only leaving only gitlab_find_action.
func TestCreateServer_ReadOnly_DynamicSurfaceRunsReadsAndRejectsWrites(t *testing.T) {
	session := modeTestSession(t, &config.ServerConfig{ToolSurface: config.ToolSurfaceDynamic, ReadOnly: true})

	read := callModeTool(t, session, "gitlab_execute_action", map[string]any{
		"action": "project.list",
		"params": map[string]any{},
	})
	if !strings.Contains(read, "projectList") {
		t.Errorf("read-only dynamic surface could not run a read action: %s", read)
	}

	write := callModeTool(t, session, "gitlab_execute_action", map[string]any{
		"action": "issue.create",
		"params": map[string]any{"project_id": "1", "title": "should not exist"},
	})
	if !strings.Contains(write, "unknown action") {
		t.Errorf("mutating action was routable in read-only mode: %s", write)
	}
}

// TestCreateServer_ReadOnly_MetaSurfaceKeepsMixedDomainReads verifies that a
// domain mixing reads and writes survives read-only mode with its read actions
// intact, instead of being dropped whole because the domain is not read-only.
func TestCreateServer_ReadOnly_MetaSurfaceKeepsMixedDomainReads(t *testing.T) {
	session := modeTestSession(t, &config.ServerConfig{ToolSurface: config.ToolSurfaceMeta, ReadOnly: true})

	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var issueTool *mcp.Tool
	for _, tool := range listed.Tools {
		if tool.Name == "gitlab_issue" {
			issueTool = tool
		}
	}
	if issueTool == nil {
		t.Fatal("read-only meta surface dropped gitlab_issue: its read actions are unreachable")
	}
	if issueTool.Annotations == nil || !issueTool.Annotations.ReadOnlyHint {
		t.Error("surviving read-only group must advertise ReadOnlyHint")
	}

	read := callModeTool(t, session, "gitlab_issue", map[string]any{
		"action": "list",
		"params": map[string]any{"project_id": "1"},
	})
	if !strings.Contains(read, "issueList") {
		t.Errorf("read-only meta surface could not list issues: %s", read)
	}

	write := callModeTool(t, session, "gitlab_issue", map[string]any{
		"action": "create",
		"params": map[string]any{"project_id": "1", "title": "should not exist"},
	})
	if strings.Contains(write, "issueCreate") {
		t.Errorf("mutating meta action executed in read-only mode: %s", write)
	}
	// The action is not advertised in the tool's action enum, so the call is
	// rejected during schema validation before dispatch; older builds instead
	// answered "unknown action" from the dispatcher.
	rejected := strings.Contains(write, "does not equal any of") ||
		strings.Contains(strings.ToLower(write), "unknown action") ||
		strings.Contains(strings.ToLower(write), "unsupported")
	if !rejected {
		t.Errorf("mutating meta action was not rejected in read-only mode: %s", write)
	}
}

// TestCreateServer_ReadOnly_IndividualSurfaceKeepsOnlyReadTools verifies the
// individual surface exposes read tools and no mutating ones.
func TestCreateServer_ReadOnly_IndividualSurfaceKeepsOnlyReadTools(t *testing.T) {
	server := mustCreateServer(t, newMockGitLabClient(t), &config.ServerConfig{ToolSurface: config.ToolSurfaceIndividual, ReadOnly: true})
	listed, err := listRegisteredTools(server, "individual-readonly")
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var mutating []string
	readTools := 0
	for _, tool := range listed {
		if tool.Annotations != nil && tool.Annotations.ReadOnlyHint {
			readTools++
			continue
		}
		mutating = append(mutating, tool.Name)
	}
	if len(mutating) > 0 {
		t.Errorf("read-only individual surface exposes mutating tools: %v", mutating)
	}
	if readTools == 0 {
		t.Error("read-only individual surface exposes no read tools at all")
	}
}

// TestCreateServer_ReadOnly_TakesPrecedenceOverSafeMode verifies the documented
// precedence: with both modes enabled, read-only wins and mutating actions are
// absent rather than previewable.
func TestCreateServer_ReadOnly_TakesPrecedenceOverSafeMode(t *testing.T) {
	session := modeTestSession(t, &config.ServerConfig{ToolSurface: config.ToolSurfaceDynamic, ReadOnly: true, SafeMode: true})

	write := callModeTool(t, session, "gitlab_execute_action", map[string]any{
		"action": "issue.create",
		"params": map[string]any{"project_id": "1", "title": "should not exist"},
	})
	if safeModeBlocked(t, write, "issue.create") {
		t.Errorf("read-only must remove mutating actions, not preview them: %s", write)
	}
	if !strings.Contains(write, "unknown action") {
		t.Errorf("mutating action was routable with read-only + safe mode: %s", write)
	}

	read := callModeTool(t, session, "gitlab_execute_action", map[string]any{
		"action": "project.list",
		"params": map[string]any{},
	})
	if !strings.Contains(read, "projectList") {
		t.Errorf("reads must keep working with read-only + safe mode: %s", read)
	}
}
