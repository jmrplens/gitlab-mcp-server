// main_test.go contains unit and integration-style tests for the server entry
// point. Tests cover CLI flag handling, configuration validation, GitLab client
// setup, HTTP and stdio transport modes, MCP protocol handshakes, tool catalog
// filtering, OAuth middleware, and server-card generation.
//
// The tests use httptest servers for GitLab API responses and HTTP transport
// requests, plus in-memory MCP transports for direct tools/list inspection.
package main

import (
	"bytes"
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
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/clientcompat"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/gatewaycompat"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/oauth"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/prompts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/serverpool"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/telemetry"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// HTTP header names, MIME types, and test values reused across tests.
const (
	testToken       = "test-token"
	serverName      = "gitlab-mcp-server"
	mimeJSONSSE     = "application/json, text/event-stream"
	hdrMCPSessionID = "Mcp-Session-Id"
)

// testHTTPClient avoids http.DefaultClient in tests so that stalled mock
// servers cannot hang the entire test suite indefinitely.
// The 30-second timeout matches testHTTPLivenessTimeout's reasoning: a
// healthy server answers in milliseconds, and the budget only matters on
// the first request of a process under the race detector, where building
// the pool entry's full catalog alone can exceed ten seconds. A passing
// test is never slowed by the larger value.
var testHTTPClient = &http.Client{Timeout: 30 * time.Second} //nolint:gochecknoglobals // test-only

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
	// readOnlyFromTokenScope belongs in the key because it does not change
	// which actions are registered, only what the surface says about the ones
	// it withheld. Two configs that differ solely here build genuinely
	// different servers, so a cache that ignored it handed one test the other's
	// wording.
	readOnlyFromTokenScope bool
	safeMode               bool
	metaParamSchema        string
	rateLimitRPS           float64
	rateLimitBurst         int
	clientCompat           bool
	// descriptionSubs keeps the cache honest for tests that set the
	// substitution env var: createServer reads it at build time, so two
	// builds under different values are different servers.
	descriptionSubs string
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
		server, err := createServer(t.Context(), client, cfg)
		if err != nil {
			t.Fatalf("createServer() error: %v", err)
		}
		return server
	}
	key := createdServerKey{
		metaTools:              cfg.MetaTools,
		toolSurface:            cfg.ToolSurface,
		capabilitySurface:      cfg.CapabilitySurface,
		tier:                   cfg.Tier,
		tierExplicit:           cfg.TierExplicit,
		readOnly:               cfg.ReadOnly,
		readOnlyFromTokenScope: cfg.ReadOnlyFromTokenScope,
		safeMode:               cfg.SafeMode,
		metaParamSchema:        cfg.MetaParamSchema,
		rateLimitRPS:           cfg.RateLimitRPS,
		rateLimitBurst:         cfg.RateLimitBurst,
		clientCompat:           clientcompat.Enabled(),
		descriptionSubs:        os.Getenv(gatewaycompat.EnvVar),
	}
	createdServersMu.Lock()
	defer createdServersMu.Unlock()
	if server, ok := createdServers[key]; ok {
		return server
	}
	server, err := createServer(t.Context(), sharedCreateServerClient(t), cfg)
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
		t.Run(key, func(t *testing.T) {
			group, gok := caps[key].(map[string]any)
			if !gok {
				t.Errorf("capabilities.%s missing or not an object: %v", key, caps[key])
				return
			}
			if got := group["listChanged"]; got != true {
				t.Errorf("capabilities.%s.listChanged = %v, want true", key, got)
			}
		})
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
		// The dynamic surface, explicitly: these are transport, auth and
		// routing tests, and none of them needs the pool's first request
		// to build the full individual catalog — which, under the race
		// detector, costs longer than any sane client timeout.
		ToolSurface: config.ToolSurfaceDynamic,
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
		// The dynamic surface, explicitly: these are transport, auth and
		// routing tests, and none of them needs the pool's first request
		// to build the full individual catalog — which, under the race
		// detector, costs longer than any sane client timeout.
		ToolSurface: config.ToolSurfaceDynamic,
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
// (the server must stay usable so self-diagnostics can
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
			maxHTTPClients: config.DefaultMaxHTTPClients,
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
			maxHTTPClients: config.DefaultMaxHTTPClients,
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

// TestRunWithContext_PingFailure_StartsInDegradedMode verifies that an
// unreachable or refusing GitLab does not stop the server from starting.
//
// That is deliberate: a server that refused to start because its instance was
// briefly down would be harder to work with than one that starts and reports
// the failure per call, so runWithContext logs the failed check and enables
// lazy initialization instead.
//
// This test used to assert the opposite, and passed for a reason that had
// nothing to do with the ping. Nothing here fails, so the run reached
// serveStdio, whose transport returned an error when the test's own stdin hit
// EOF — that error was what the assertion caught. The transport now treats EOF
// as the ordinary end of a session (a client closing its pipe is not a
// failure), which is what exposed it. Ending cleanly is asserted alongside the
// startup, so the same false positive cannot come back.
func TestRunWithContext_PingFailure_StartsInDegradedMode(t *testing.T) {
	srv := newFailingGitLabServer(t, http.StatusForbidden)
	t.Setenv("GITLAB_URL", srv.URL)
	t.Setenv("GITLAB_TOKEN", testToken)
	t.Setenv("GITLAB_SKIP_TLS_VERIFY", "true")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// stdin is already at EOF here, so the session ends as soon as it starts.
	// Reaching that point at all is the assertion: the refused ping did not
	// stop the server from coming up.
	if err := runWithContext(ctx, nil); err != nil {
		t.Fatalf("a refused connectivity check stopped the server from starting: %v", err)
	}
	if ctx.Err() != nil {
		t.Error("the run did not return on its own; it was cut short by the timeout")
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

// TestRunWithContext_HTTPMissingURL verifies that HTTP mode starts only when
// the caller's free choice of instance was asked for explicitly.
//
// This used to assert the opposite, and the behavior it pinned is the finding:
// with no --gitlab-url the GITLAB-URL header selected any host, the credential
// was verified against that same host, and every tool call went wherever the
// caller pointed it.
func TestRunWithContext_HTTPMissingURL(t *testing.T) {
	for _, tc := range []struct {
		name       string
		allowAny   bool
		wantsStart bool
	}{
		{"without_the_escape_hatch", false, false},
		{"with_the_escape_hatch", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			go func() {
				// Give the HTTP server a brief moment to start, then stop it
				// to avoid waiting on the global test timeout.
				time.Sleep(50 * time.Millisecond)
				cancel()
			}()

			err := runWithContext(ctx, &httpConfig{
				addr:              "127.0.0.1:0",
				gitlabURL:         "",
				allowAnyGitLabURL: tc.allowAny,
				maxHTTPClients:    config.DefaultMaxHTTPClients,
				sessionTimeout:    config.DefaultSessionTimeout,
			})
			if tc.wantsStart {
				if err != nil {
					t.Fatalf("runWithContext with --allow-any-gitlab-url: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("HTTP mode started with no instance and no escape hatch")
			}
			if !strings.Contains(err.Error(), "--gitlab-url") {
				t.Errorf("error = %q, want it to name the missing flag", err)
			}
		})
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
				maxHTTPClients: config.DefaultMaxHTTPClients,
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

// TestNormalizeFixedGitLabURL_ReadsBothFields pins the invariant that the two
// instance fields are both INPUTS.
//
// The flag parser fills the list; a caller constructing httpConfig directly
// fills the singular field. Reading only the list discards that URL along with
// its validation — an unusable --gitlab-url stopped being rejected and the
// server started anyway, serving an instance nobody configured. That is not
// hypothetical: it hung TestRunWithContext_HTTPInvalidURL for the full test
// timeout, because the run it was supposed to reject went on serving.
func TestNormalizeFixedGitLabURL_ReadsBothFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		hcfg      httpConfig
		wantURL   string
		wantList  []string
		wantErrIn string
	}{
		{
			name:     "the singular field alone is honored",
			hcfg:     httpConfig{gitlabURL: "https://gitlab.example.com/"},
			wantURL:  "https://gitlab.example.com",
			wantList: []string{"https://gitlab.example.com"},
		},
		{
			name:     "the list alone is honored",
			hcfg:     httpConfig{gitlabURLs: repeatedFlag{"https://gitlab.com", "https://gitlab.example.com"}},
			wantURL:  "https://gitlab.com",
			wantList: []string{"https://gitlab.com", "https://gitlab.example.com"},
		},
		{
			name:     "the list wins when both are set",
			hcfg:     httpConfig{gitlabURL: "https://ignored.example.com", gitlabURLs: repeatedFlag{"https://gitlab.com"}},
			wantURL:  "https://gitlab.com",
			wantList: []string{"https://gitlab.com"},
		},
		{
			name: "neither leaves both empty",
			hcfg: httpConfig{},
		},
		{
			// The validation that stopped running, and its message must
			// name the flag rather than the GITLAB-URL header.
			name:      "an unusable singular value is still rejected",
			hcfg:      httpConfig{gitlabURL: "ftp://gitlab.example.com"},
			wantErrIn: "--gitlab-url must use http:// or https://",
		},
		{
			name:      "an unusable entry in the list is rejected",
			hcfg:      httpConfig{gitlabURLs: repeatedFlag{"https://gitlab.com", "https://"}},
			wantErrIn: "--gitlab-url must include a host",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hcfg := tt.hcfg
			err := normalizeFixedGitLabURL(&hcfg)

			if tt.wantErrIn != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrIn) {
					t.Fatalf("error = %v, want one containing %q", err, tt.wantErrIn)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeFixedGitLabURL() error = %v", err)
			}
			if hcfg.gitlabURL != tt.wantURL {
				t.Errorf("gitlabURL = %q, want %q", hcfg.gitlabURL, tt.wantURL)
			}
			if !slices.Equal([]string(hcfg.gitlabURLs), tt.wantList) {
				t.Errorf("gitlabURLs = %v, want %v", hcfg.gitlabURLs, tt.wantList)
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
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(output, c.want) {
				t.Errorf("printHelp missing %s: want substring %q", c.name, c.want)
			}
		})
	}
}

// TestPrintHelp_Transport_NamesEverySelectorAndThePrecedence verifies that
// -transport is documented in the curated help, with all three values it
// accepts and the rule that settles a run passing both selectors.
//
// The flag is discoverable through the flag package's own output, but nobody
// reads that: -h prints printHelp, and a transport flag missing from it is a
// flag an operator has to already know about to find. The precedence is worth
// stating there too, because -http is in every existing deployment and the
// answer to "what happens if I keep it" is the first question this flag raises.
func TestPrintHelp_Transport_NamesEverySelectorAndThePrecedence(t *testing.T) {
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
	// The entry is its own line plus its indented continuation lines: the
	// description is long enough to wrap, and a value named on a wrapped line
	// is still documented.
	entry := helpEntry(string(out), "-transport string")
	if entry == "" {
		t.Fatal("the help does not document -transport at all")
	}

	for _, tc := range []struct {
		name string
		want string
	}{
		{name: "stdio is named", want: transportStdio},
		{name: "http is named", want: transportHTTP},
		{name: "auto is named", want: transportAuto},
		{name: "the precedence over -http is stated", want: "defers to -http"},
		{name: "and which of the two wins when both are given", want: "given both, -transport wins"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(entry, tc.want) {
				t.Errorf("the -transport help entry does not mention %q: %q", tc.want, entry)
			}
		})
	}
}

// helpEntry returns one flag's help entry joined into a single line: the line
// naming the flag plus every indented continuation line under it.
func helpEntry(help, flagName string) string {
	lines := strings.Split(help, "\n")
	for i, candidate := range lines {
		if !strings.Contains(candidate, flagName) {
			continue
		}
		entry := []string{strings.TrimSpace(candidate)}
		for _, next := range lines[i+1:] {
			trimmed := strings.TrimSpace(next)
			if trimmed == "" || strings.HasPrefix(trimmed, "-") {
				break
			}
			entry = append(entry, trimmed)
		}
		return strings.Join(entry, " ")
	}
	return ""
}

// TestPrintHelp_RevalidateInterval_DoesNotPromiseThatZeroStopsReverification
// verifies that the help says what the pool now does.
//
// The pool rebuilds any entry whose credential was last checked more than
// [serverpool.DefaultMaxCredentialAge] ago, and that ceiling cannot be turned
// off. "0 to disable" was true before the ceiling existed; leaving it there
// tells an operator who needs re-verification stopped, for a long-lived
// stateful session that a rebuild answers 404, that a flag they set achieved
// it.
func TestPrintHelp_RevalidateInterval_DoesNotPromiseThatZeroStopsReverification(t *testing.T) {
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

	// The entry is its own line plus any indented continuation lines, since a
	// flag long enough to need this correction is long enough to wrap.
	line := helpEntry(string(out), "-revalidate-interval")
	if line == "" {
		t.Fatal("the help does not document -revalidate-interval at all")
	}
	if strings.Contains(line, "0 to disable") {
		t.Errorf("the help still promises that 0 disables re-validation: %q", line)
	}
	if !strings.Contains(line, serverpool.DefaultMaxCredentialAge.String()) {
		t.Errorf("the help does not name the credential ceiling that keeps running at 0: %q", line)
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
		t.Run(name, func(t *testing.T) {
			if !found {
				t.Fatalf("dynamic tool %q was not registered", name)
			}
		})
	}

	_, err = session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "gitlab://tools/project.get"})
	if err != nil {
		t.Fatalf("dynamic surface should expose tool manifest detail resources: %v", err)
	}
}

// TestCreateServer_DynamicSurface_ReportsWhatTheExclusionRemoved verifies that
// a working exclusion says so at startup.
//
// The dynamic surface registers two tools, neither of which is ever an
// exclusion target, so the only line an operator saw counted registered names
// and read "excluded=0" beside an exclusion that had just removed an action
// from the catalog. That is the same line a typo produces, which makes the
// startup log useless for the one question it is read for.
func TestCreateServer_DynamicSurface_ReportsWhatTheExclusionRemoved(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))

	client := newMockGitLabClient(t)
	mustCreateServer(t, client, &config.ServerConfig{
		ToolSurface:  config.ToolSurfaceDynamic,
		ExcludeTools: []string{"gitlab_issue_delete"},
	})

	if !strings.Contains(logged.String(), `"msg":"excluded catalog actions by configuration","excluded":1`) {
		t.Errorf("the startup log does not report the catalog action the exclusion removed: %s", logged.String())
	}
}

// TestCreateServer_IndividualSurface_ExcludesByEveryNameAnOperatorMayUse
// verifies that EXCLUDE_TOOLS means the same thing here as on the dynamic and
// meta surfaces.
//
// Exclusion used to run against the registered tool names alone, and on this
// surface those are the per-action names declared in the ActionSpec, so the two
// other spellings an operator legitimately uses, the meta group name (which is
// what the documented example carries) and the canonical action ID, removed
// nothing and reported nothing. A configuration reused across surfaces was
// therefore half applied on one of them, with no warning either way.
func TestCreateServer_IndividualSurface_ExcludesByEveryNameAnOperatorMayUse(t *testing.T) {
	const (
		excluded = "gitlab_issue_delete"
		control  = "gitlab_issue_list"
	)
	for _, tc := range []struct {
		name    string
		exclude []string
	}{
		{name: "the meta group name", exclude: []string{"gitlab_issue"}},
		{name: "the canonical action ID", exclude: []string{"issue.delete"}},
		{name: "the individual tool name", exclude: []string{excluded}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newMockGitLabClient(t)
			server := mustCreateServer(t, client, &config.ServerConfig{
				ToolSurface:  config.ToolSurfaceIndividual,
				ExcludeTools: tc.exclude,
			})
			session := newInMemorySession(t, server)

			listed, err := session.ListTools(t.Context(), nil)
			if err != nil {
				t.Fatalf("ListTools() error = %v", err)
			}
			var sawExcluded, sawControl bool
			for _, tool := range listed.Tools {
				switch tool.Name {
				case excluded:
					sawExcluded = true
				case control:
					sawControl = true
				}
			}
			if sawExcluded {
				t.Errorf("%q is still registered after excluding %v", excluded, tc.exclude)
			}
			// A group-name exclusion removes the whole group, so the control
			// only has to survive the two narrower spellings.
			if !sawControl && tc.exclude[0] != "gitlab_issue" {
				t.Errorf("%q disappeared as well; the exclusion removed more than it was given", control)
			}
		})
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
		t.Run(name, func(t *testing.T) {
			if !found {
				t.Fatalf("meta standalone tool %q was not registered", name)
			}
		})
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

// TestCreateServer_ToolManifestEntriesCoverEveryVisibleTool verifies the
// gitlab://tools promise that it lists "every executable entry": a client
// enumerating entries must be able to reach every tool the server exposes.
//
// A tool is reachable either through an entry of its own or as the tool an
// entry dispatches to — gitlab_execute_action and the meta dispatchers are
// named by the entries they route. Standalone utilities belong to no
// dispatcher, so they need their own entry; on the meta surface the five of
// them were advertised in visible_tools while missing from entries.
func TestCreateServer_ToolManifestEntriesCoverEveryVisibleTool(t *testing.T) {
	client := newMockGitLabClient(t)
	for _, toolSurface := range []string{config.ToolSurfaceIndividual, config.ToolSurfaceMeta, config.ToolSurfaceDynamic} {
		t.Run(toolSurface, func(t *testing.T) {
			server := mustCreateServer(t, client, &config.ServerConfig{
				MetaTools:         true,
				ToolSurface:       toolSurface,
				CapabilitySurface: config.CapabilitySurfaceFull,
			})
			assertManifestCoversVisibleTools(t, newInMemorySession(t, server))
		})
	}
}

func assertManifestCoversVisibleTools(t *testing.T, session *mcp.ClientSession) {
	t.Helper()

	result, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: "gitlab://tools"})
	if err != nil {
		t.Fatalf("read gitlab://tools: %v", err)
	}
	if len(result.Contents) == 0 {
		t.Fatal("gitlab://tools returned no contents")
	}

	var manifest struct {
		VisibleTools []struct {
			Name string `json:"name"`
		} `json:"visible_tools"`
		Entries []struct {
			ID   string `json:"id"`
			Tool string `json:"tool"`
		} `json:"entries"`
	}
	if unmarshalErr := json.Unmarshal([]byte(result.Contents[0].Text), &manifest); unmarshalErr != nil {
		t.Fatalf("unmarshal tool manifest: %v", unmarshalErr)
	}
	if len(manifest.VisibleTools) == 0 {
		t.Fatal("manifest advertises no visible tools")
	}

	reachable := make(map[string]struct{}, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		reachable[entry.Tool] = struct{}{}
		reachable[entry.ID] = struct{}{}
	}

	var unreachable []string
	for _, tool := range manifest.VisibleTools {
		if _, ok := reachable[tool.Name]; !ok {
			unreachable = append(unreachable, tool.Name)
		}
	}
	if len(unreachable) > 0 {
		slices.Sort(unreachable)
		t.Errorf("%d visible tools are absent from the manifest entries:\n%s",
			len(unreachable), strings.Join(unreachable, "\n"))
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
	server, err := createServer(t.Context(), client, &config.ServerConfig{MetaTools: true})
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

// TestRunStdio_PingSucceeds verifies the success path for Ping in runStdio,
// where the GitLab mock returns a valid version response.
func TestRunStdio_PingSucceeds(t *testing.T) {
	srv := newMockGitLabServer(t)
	t.Setenv("GITLAB_URL", srv.URL)
	t.Setenv("GITLAB_TOKEN", testToken)
	t.Setenv("META_TOOLS", "false")

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
		// The dynamic surface, explicitly: these are transport, auth and
		// routing tests, and none of them needs the pool's first request
		// to build the full individual catalog — which, under the race
		// detector, costs longer than any sane client timeout.
		ToolSurface: config.ToolSurfaceDynamic,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Find a free port.
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTPOn(ctx, cfg, addr, listener, defaultHTTPIdleTimeout)
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

// TestServeHTTP_UnknownPath_Is404NotAChallenge verifies that routing now
// happens before authentication.
//
// Every unknown path used to answer 401, because the auth gate was the
// catch-all handler. That told every directory and scanner probing
// /.well-known/oauth-authorization-server that a protected metadata document
// lives there — it does not — and left nothing able to tell "exists but needs
// a token" apart from "is not here". The MCP endpoint keeps answering on the
// root and on /mcp; everything else is 404, and unauthenticated.
func TestServeHTTP_UnknownPath_Is404NotAChallenge(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	cfg := &config.Config{
		GitLabURL:      mockGL.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		ToolSurface:    config.ToolSurfaceDynamic,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTPOn(ctx, cfg, addr, listener, defaultHTTPIdleTimeout)
	}()
	waitForHTTPServerReady(t, addr, errCh)

	tests := []struct {
		name string
		path string
		want int
	}{
		{name: "a well-known document this server does not serve", path: "/.well-known/oauth-authorization-server", want: http.StatusNotFound},
		{name: "the retired well-known mcp path", path: "/.well-known/mcp", want: http.StatusNotFound},
		{name: "an invented path", path: "/nope", want: http.StatusNotFound},
		// The endpoint itself still authenticates: 404 must not have
		// swallowed the routes that matter.
		{name: "the root is the MCP endpoint", path: "/", want: http.StatusUnauthorized},
		{name: "the named MCP endpoint", path: "/mcp", want: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
			req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr+tt.path, strings.NewReader(body))
			if reqErr != nil {
				t.Fatalf("new request: %v", reqErr)
			}
			req.Header.Set(hdrContentType, mimeJSON)
			req.Header.Set("Accept", mimeJSONSSE)

			resp, doErr := testHTTPClient.Do(req)
			if doErr != nil {
				t.Fatalf("request failed: %v", doErr)
			}
			respBody := readAndCloseBody(t, resp)
			if resp.StatusCode != tt.want {
				t.Errorf("status = %d, want %d: %s", resp.StatusCode, tt.want, respBody)
			}
			if tt.want == http.StatusNotFound && resp.Header.Get("WWW-Authenticate") != "" {
				t.Error("a 404 must not carry an authentication challenge")
			}
			// The security headers apply to a 404 like any other response.
			if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
			}
		})
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

// TestServeHTTP_CrossOriginProtection_RejectsCrossSitePost verifies HTTP mode
// rejects browser-originated cross-site POST requests before MCP dispatch.
func TestServeHTTP_CrossOriginProtection_RejectsCrossSitePost(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	cfg := &config.Config{
		GitLabURL:      mockGL.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
		// The dynamic surface, explicitly: these are transport, auth and
		// routing tests, and none of them needs the pool's first request
		// to build the full individual catalog — which, under the race
		// detector, costs longer than any sane client timeout.
		ToolSurface: config.ToolSurfaceDynamic,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTPOn(ctx, cfg, addr, listener, defaultHTTPIdleTimeout)
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
		// The dynamic surface, explicitly: these are transport, auth and
		// routing tests, and none of them needs the pool's first request
		// to build the full individual catalog — which, under the race
		// detector, costs longer than any sane client timeout.
		ToolSurface: config.ToolSurfaceDynamic,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTPOn(ctx, cfg, addr, listener, defaultHTTPIdleTimeout)
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

// TestServeHTTP_NoInstanceAndNoHeader_IsRefused verifies that a deployment
// publishing no instance refuses a request that names none either, rather than
// resolving it to the public gitlab.com.
//
// The old contract, which this test asserted under its previous name, was that
// an absent header fell back to gitlab.com. That put the caller's token on the
// wire to a host they never named, which is the defect the multi-instance
// refusal already closes, so the fallback is gone (see
// serverpool.ErrMissingGitLabURL). Such a deployment can only exist at all with
// --allow-any-gitlab-url, where the operator has said the header chooses.
//
// The refusal must also name the header rather than blame the operator's
// configuration: with no header, an unresolvable instance used to mean only a
// mistyped --gitlab-url, and that message would now send the caller looking for
// someone else over an omission of their own.
func TestServeHTTP_NoInstanceAndNoHeader_IsRefused(t *testing.T) {
	cfg := &config.Config{
		GitLabURL:      "",
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
		// The dynamic surface, explicitly: these are transport, auth and
		// routing tests, and none of them needs the pool's first request
		// to build the full individual catalog — which, under the race
		// detector, costs longer than any sane client timeout.
		ToolSurface:  config.ToolSurfaceDynamic,
		Tier:         edition.Free,
		TierExplicit: true,
		IgnoreScopes: true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTPOn(ctx, cfg, addr, listener, defaultHTTPIdleTimeout)
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

	// Refused, and refused for the caller's own reason. The request never
	// reaches GitLab, so this stays hermetic without pinning the network
	// behavior the old fallback depended on.
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d: %s", resp.StatusCode, http.StatusBadRequest, respBody)
	}
	if !strings.Contains(respBody, serverpool.RequestOptionGitLabURL) {
		t.Errorf("the refusal must name the header the caller has to send: %s", respBody)
	}
	if strings.Contains(respBody, "contact the operator") {
		t.Errorf("a caller who omitted the header is not looking at a broken deployment: %s", respBody)
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
		// The dynamic surface, explicitly: these are transport, auth and
		// routing tests, and none of them needs the pool's first request
		// to build the full individual catalog — which, under the race
		// detector, costs longer than any sane client timeout.
		ToolSurface: config.ToolSurfaceDynamic,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTPOn(ctx, cfg, addr, listener, defaultHTTPIdleTimeout)
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

// TestServeHTTP_MissingToken verifies that the HTTP handler rejects requests
// without an authentication token by returning nil from the server factory.
func TestServeHTTP_MissingToken(t *testing.T) {
	mockGL := newMockGitLabServer(t)
	cfg := &config.Config{
		GitLabURL:      mockGL.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
		// The dynamic surface, explicitly: these are transport, auth and
		// routing tests, and none of them needs the pool's first request
		// to build the full individual catalog — which, under the race
		// detector, costs longer than any sane client timeout.
		ToolSurface: config.ToolSurfaceDynamic,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTPOn(ctx, cfg, addr, listener, defaultHTTPIdleTimeout)
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTPOn(ctx, cfg, addr, listener, defaultHTTPIdleTimeout)
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
		// The dynamic surface, explicitly: these are transport, auth and
		// routing tests, and none of them needs the pool's first request
		// to build the full individual catalog — which, under the race
		// detector, costs longer than any sane client timeout.
		ToolSurface:   config.ToolSurfaceDynamic,
		AuthMode:      "oauth",
		PublicURL:     "http://localhost:8080",
		OAuthCacheTTL: config.DefaultOAuthCacheTTL,
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
		// The dynamic surface, explicitly: these are transport, auth and
		// routing tests, and none of them needs the pool's first request
		// to build the full individual catalog — which, under the race
		// detector, costs longer than any sane client timeout.
		ToolSurface:   config.ToolSurfaceDynamic,
		AuthMode:      "oauth",
		PublicURL:     "http://localhost:8080",
		OAuthCacheTTL: config.DefaultOAuthCacheTTL,
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
		// The dynamic surface, explicitly: these are transport, auth and
		// routing tests, and none of them needs the pool's first request
		// to build the full individual catalog — which, under the race
		// detector, costs longer than any sane client timeout.
		ToolSurface:   config.ToolSurfaceDynamic,
		AuthMode:      "oauth",
		PublicURL:     "http://localhost:8080",
		OAuthCacheTTL: config.DefaultOAuthCacheTTL,
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

// TestServeHTTP_OAuthMode_PrivateTokenRejected verifies oauth mode accepts
// only the RFC 6750 Bearer scheme its challenge advertises: the legacy
// PRIVATE-TOKEN alias is no longer silently rewritten into it, so a request
// carrying only that header is answered 401 with the Bearer challenge.
func TestServeHTTP_OAuthMode_PrivateTokenRejected(t *testing.T) {
	mockGL := newMockGitLabServerWithUser(t)
	cfg := &config.Config{
		GitLabURL:      mockGL.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
		// The dynamic surface, explicitly: these are transport, auth and
		// routing tests, and none of them needs the pool's first request
		// to build the full individual catalog — which, under the race
		// detector, costs longer than any sane client timeout.
		ToolSurface:   config.ToolSurfaceDynamic,
		AuthMode:      "oauth",
		PublicURL:     "http://localhost:8080",
		OAuthCacheTTL: config.DefaultOAuthCacheTTL,
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

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 (PRIVATE-TOKEN is not the advertised Bearer scheme), got %d: %s", resp.StatusCode, respBody)
	}
	if challenge := resp.Header.Get("WWW-Authenticate"); !strings.Contains(challenge, "Bearer") {
		t.Errorf("WWW-Authenticate = %q, want a Bearer challenge", challenge)
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

// TestServeHTTP_OAuthMode_InvalidTokenReturns401 verifies that OAuth mode
// returns 401 for an invalid Bearer token.
func TestServeHTTP_OAuthMode_InvalidTokenReturns401(t *testing.T) {
	mockGL := newMockGitLabServerWithUser(t)
	cfg := &config.Config{
		GitLabURL:      mockGL.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
		// The dynamic surface, explicitly: these are transport, auth and
		// routing tests, and none of them needs the pool's first request
		// to build the full individual catalog — which, under the race
		// detector, costs longer than any sane client timeout.
		ToolSurface:   config.ToolSurfaceDynamic,
		AuthMode:      "oauth",
		PublicURL:     "http://localhost:8080",
		OAuthCacheTTL: config.DefaultOAuthCacheTTL,
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
		// The dynamic surface, explicitly: these are transport, auth and
		// routing tests, and none of them needs the pool's first request
		// to build the full individual catalog — which, under the race
		// detector, costs longer than any sane client timeout.
		ToolSurface: config.ToolSurfaceDynamic,
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
		maxHTTPClients: config.DefaultMaxHTTPClients,
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
//
// --allow-any-gitlab-url is passed so the general allow-list requirement is
// satisfied and this reaches the oauth-specific check: the escape hatch that
// makes free instance selection tolerable for a local single-user deployment
// does not open it for a deployment forwarding bearer tokens.
func TestRunHTTP_OAuthRequiresGitLabURL(t *testing.T) {
	err := runHTTP(context.Background(), &httpConfig{
		gitlabURL:         "",
		allowAnyGitLabURL: true,
		maxHTTPClients:    config.DefaultMaxHTTPClients,
		sessionTimeout:    config.DefaultSessionTimeout,
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
		publicURL:      "http://localhost:8080",
		oauthCacheTTL:  10 * time.Second,
		maxHTTPClients: config.DefaultMaxHTTPClients,
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
		publicURL:      "http://localhost:8080",
		oauthCacheTTL:  5 * time.Hour,
		maxHTTPClients: config.DefaultMaxHTTPClients,
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
		maxHTTPClients: config.DefaultMaxHTTPClients,
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

// TestRunHTTP_InvalidGitLabURL verifies that runHTTP rejects a non-HTTP(S) URL.
func TestRunHTTP_InvalidGitLabURL(t *testing.T) {
	err := runHTTP(context.Background(), &httpConfig{
		gitlabURL:      "ftp://gitlab.example.com",
		maxHTTPClients: config.DefaultMaxHTTPClients,
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

// TestCorsAllowHeaders_FollowTheAuthMode pins what the preflight actually
// permits, per mode.
//
// Two failures live here. Advertising a credential header a mode ignores tells
// a browser client to send something that produces a 401 — PRIVATE-TOKEN is
// legacy-only, and GITLAB-URL means nothing when one instance is pinned.
// Omitting a header the SDK REQUIRES is worse: from protocol 2026-07-28 a POST
// without Mcp-Method is rejected before any handler runs, so leaving it out of
// the preflight made the server refuse the very headers it then demanded, and
// no browser client could speak the current protocol at all.
func TestCorsAllowHeaders_FollowTheAuthMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *config.Config
		want    []string
		notWant []string
	}{
		{
			name: "legacy with one pinned instance",
			cfg:  &config.Config{AuthMode: config.AuthModeLegacy, GitLabURL: "https://gitlab.com"},
			want: []string{"Authorization", "PRIVATE-TOKEN", "Mcp-Method", "Mcp-Protocol-Version"},
			// Pinned, so the header is logged as ignored: advertising it
			// invites a client to send something that silently does nothing.
			notWant: []string{"GITLAB-URL"},
		},
		{
			name:    "legacy with no instance pinned",
			cfg:     &config.Config{AuthMode: config.AuthModeLegacy},
			want:    []string{"PRIVATE-TOKEN", "GITLAB-URL", "Mcp-Method"},
			notWant: nil,
		},
		{
			name: "oauth pins one instance",
			cfg:  &config.Config{AuthMode: config.AuthModeOAuth, GitLabURL: "https://gitlab.com"},
			want: []string{"Authorization", "Mcp-Method", "Mcp-Name"},
			// OAuth mode reads Authorization: Bearer and nothing else.
			notWant: []string{"PRIVATE-TOKEN", "GITLAB-URL"},
		},
		{
			name: "oauth publishes several instances",
			cfg: &config.Config{
				AuthMode:   config.AuthModeOAuth,
				GitLabURL:  "https://gitlab.com",
				GitLabURLs: []string{"https://gitlab.com", "https://gitlab.example.com"},
			},
			// Several published instances make the header a real choice again.
			want:    []string{"Authorization", "GITLAB-URL", "Mcp-Method"},
			notWant: []string{"PRIVATE-TOKEN"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := corsAllowHeadersFor(tt.cfg)
			for _, header := range tt.want {
				if !strings.Contains(got, header) {
					t.Errorf("Allow-Headers = %q, want it to permit %s", got, header)
				}
			}
			for _, header := range tt.notWant {
				if strings.Contains(got, header) {
					t.Errorf("Allow-Headers = %q advertises %s, which this mode does not honor", got, header)
				}
			}
		})
	}
}

// TestSecurityHeaders_CoverRejectionsToo verifies that a response written by a
// middleware that answers instead of forwarding still carries the security
// headers.
//
// Host validation used to sit OUTSIDE securityHeadersMiddleware, so its 403
// went out bare: no nosniff, no CSP, no X-Frame-Options, no Referrer-Policy.
// A header policy with a hole in it is not a policy, and the hole was in a
// rejection — the response an unwanted caller is precisely the one to get.
func TestSecurityHeaders_CoverRejectionsToo(t *testing.T) {
	t.Parallel()

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := securityHeadersMiddleware(
		inboundLimits{maxDepth: maxInboundJSONDepth},
		hostValidationMiddleware(map[string]bool{"localhost": true}, inner),
	)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://evil.example.com/", http.NoBody)
	req.Host = "evil.example.com"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
	want := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
		"Referrer-Policy":         "no-referrer",
	}
	for name, value := range want {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := rr.Header().Get(name); got != value {
				t.Errorf("%s = %q, want %q", name, got, value)
			}
		})
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
	handler := crossOriginProtectionMiddleware(nil, inner)

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
	handler := crossOriginProtectionMiddleware(nil, inner)

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

	removed := tools.RemoveNonReadOnlyTools(t.Context(), server)
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

	data, err := buildServerCard(context.Background(), cfg)
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

	data, err := buildServerCard(context.Background(), cfg)
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

// TestServeHTTP_ShutdownDuringServerCardBuild_DrainsCleanly verifies a
// shutdown that begins while the server card is still being built neither
// waits behind the build nor reports an error, and that the caught request
// is answered with 503 rather than left hanging.
//
// The card's catalog registration is CPU work no context can interrupt, so
// the guarantee has to come from the handler: the in-flight card request
// is released with a 503 the moment shutdown starts, letting Shutdown
// drain inside its budget while the detached build finishes on its own.
//
// The builder is substituted through the buildServerCardFn seam with one
// the test gates, so cancellation provably lands mid-build instead of
// racing a sleep against the real build. Deliberately not parallel: the
// seam is a package global, and the sequential pass runs alone.
func TestServeHTTP_ShutdownDuringServerCardBuild_DrainsCleanly(t *testing.T) {
	srv := newMockGitLabServer(t)
	cfg := &config.Config{
		GitLabURL:      srv.URL,
		MaxHTTPClients: config.DefaultMaxHTTPClients,
		SessionTimeout: config.DefaultSessionTimeout,
		MetaTools:      false,
		ToolSurface:    config.ToolSurfaceDynamic, // the gated fake builds the card; the surface no longer matters
	}

	buildStarted := make(chan struct{})
	releaseBuild := make(chan struct{})
	origBuild := buildServerCardFn
	buildServerCardFn = func(context.Context, *config.Config) ([]byte, error) {
		close(buildStarted)
		<-releaseBuild
		return []byte(`{}`), nil
	}
	t.Cleanup(func() { buildServerCardFn = origBuild })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTPOn(ctx, cfg, addr, listener, defaultHTTPIdleTimeout)
	}()
	waitForHTTPServerReady(t, addr, errCh)

	// Fire the card request without waiting for it — it is the build we
	// want in flight when shutdown starts.
	cardDone := make(chan struct{})
	go func() {
		defer close(cardDone)
		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet,
			"http://"+addr+"/.well-known/mcp/server-card.json", nil)
		if reqErr != nil {
			t.Errorf("build card request: %v", reqErr)
			return
		}
		resp, doErr := testHTTPClient.Do(req)
		if doErr != nil {
			t.Errorf("card request during shutdown: %v, want a 503 response", doErr)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("card request during shutdown = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
		}
	}()

	// Only cancel once the builder is provably in flight.
	select {
	case <-buildStarted:
	case <-time.After(testHTTPLivenessTimeout):
		t.Fatal("card build never started")
	}
	cancel()

	select {
	case shutdownErr := <-errCh:
		if shutdownErr != nil {
			t.Fatalf("serveHTTPOn() = %v during card build, want a clean shutdown", shutdownErr)
		}
	case <-time.After(testHTTPLivenessTimeout):
		t.Fatal("shutdown did not complete while the server card was building")
	}
	<-cardDone
	// Release the gated builder only after the assertions: its goroutine
	// outlives serveHTTPOn by design, and must not touch the restored seam.
	close(releaseBuild)
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

	data, err := buildServerCard(context.Background(), cfg)
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTPOn(ctx, cfg, addr, listener, defaultHTTPIdleTimeout)
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
	// The live surface publishes three icons (SVG + light/dark WebP) on
	// every icon-bearing entry; the card must not show less on any of the
	// four collections that carry them.
	for _, collection := range []string{"tools", "prompts", "resources", "resourceTemplates"} {
		t.Run(collection, func(t *testing.T) {
			entries, _ := card[collection].([]any)
			if len(entries) == 0 {
				t.Errorf("card %s array missing or empty", collection)
				return
			}
			for i, raw := range entries {
				entry, _ := raw.(map[string]any)
				if icons, _ := entry["icons"].([]any); len(icons) != 3 {
					t.Errorf("card %s[%d] icons = %d entries, want 3", collection, i, len(icons))
				}
			}
		})
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

// TestServeHTTP_ServerCardEndpoint_CORSAndCapabilities pins the card
// behaviors added for the 2026-07-28 conformance pass: cross-origin reads
// (GET Allow-Origin, OPTIONS preflight with the Allow-Headers echo) and the
// capabilities key sourced from the live handshake, which must not carry
// the SEP-2577-deprecated logging capability. Deleting any of them must
// fail here, not only in a manual wire check.
func TestServeHTTP_ServerCardEndpoint_CORSAndCapabilities(t *testing.T) {
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTPOn(ctx, cfg, addr, listener, defaultHTTPIdleTimeout)
	}()
	waitForHTTPServerReady(t, addr, errCh)

	cardURL := "http://" + addr + "/.well-known/mcp/server-card.json"

	t.Run("GET carries Allow-Origin and handshake capabilities without logging", func(t *testing.T) {
		assertCardGETCORSAndCapabilities(t, cardURL)
	})
	t.Run("OPTIONS preflight answers CORS and echoes requested headers", func(t *testing.T) {
		assertCardPreflight(t, cardURL)
	})

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

// assertCardGETCORSAndCapabilities fetches the card and checks the CORS
// header, the capabilities key sourced from the live handshake, and the
// absence of the SEP-2577-deprecated logging capability.
func assertCardGETCORSAndCapabilities(t *testing.T, cardURL string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, cardURL, nil)
	resp, reqErr := testHTTPClient.Do(req)
	if reqErr != nil {
		t.Fatalf("request failed: %v", reqErr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("GET Access-Control-Allow-Origin = %q, want *", got)
	}
	var card map[string]any
	body, _ := io.ReadAll(resp.Body)
	if unmarshalErr := json.Unmarshal(body, &card); unmarshalErr != nil {
		t.Fatalf("invalid JSON response: %v", unmarshalErr)
	}
	caps, ok := card["capabilities"].(map[string]any)
	if !ok {
		t.Fatal("server card missing 'capabilities'")
	}
	if _, hasTools := caps["tools"]; !hasTools {
		t.Error("card capabilities missing 'tools' — not sourced from the live handshake?")
	}
	if _, hasLogging := caps["logging"]; hasLogging {
		t.Error("card capabilities carry 'logging', deprecated by SEP-2577 — the empty-capabilities pin regressed")
	}
}

// assertCardPreflight sends the one preflight the card route can receive —
// a fetch stamped with a custom header — and checks the response names the
// header back, without which the browser refuses the actual GET.
func assertCardPreflight(t *testing.T, cardURL string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodOptions, cardURL, nil)
	req.Header.Set("Origin", "https://scanner.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", "x-scanner-id")
	resp, reqErr := testHTTPClient.Do(req)
	if reqErr != nil {
		t.Fatalf("request failed: %v", reqErr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("OPTIONS Access-Control-Allow-Origin = %q, want *", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodGet) {
		t.Errorf("OPTIONS Access-Control-Allow-Methods = %q, want to contain GET", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); got != "x-scanner-id" {
		t.Errorf("OPTIONS Access-Control-Allow-Headers = %q, want the echoed x-scanner-id", got)
	}
	if got := resp.Header.Get("Access-Control-Max-Age"); got != "3600" {
		t.Errorf("OPTIONS Access-Control-Max-Age = %q, want 3600 so the preflight is not repeated per fetch", got)
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

// TestNewHTTPServer_HeaderCeiling_IsFarBelowTheStandardLibraryDefault verifies
// that a request's headers are bounded before anything reads them.
//
// Every header-borne carrier is written to the log before the caller is
// authenticated: the Host of a refused request, the method of a cross-origin
// one. Go's own ceiling is 1 MiB, so twenty requests were enough to write 18 MB
// of stderr and 2500 to write 157 MB, from a client that never presented a
// credential. The timeouts above bound how long a request may take; this bounds
// how much of one there can be.
func TestNewHTTPServer_HeaderCeiling_IsFarBelowTheStandardLibraryDefault(t *testing.T) {
	srv := newHTTPServer(":0", http.NewServeMux(), 0)
	if srv.MaxHeaderBytes != baseHTTPMaxHeaderBytes {
		t.Errorf("MaxHeaderBytes = %d, want %d", srv.MaxHeaderBytes, baseHTTPMaxHeaderBytes)
	}
	if srv.MaxHeaderBytes >= http.DefaultMaxHeaderBytes {
		t.Errorf("MaxHeaderBytes = %d, which is not below the standard library's %d default",
			srv.MaxHeaderBytes, http.DefaultMaxHeaderBytes)
	}
}

// TestHostValidationMiddleware_LogsABoundedHost verifies that the refusal an
// unauthenticated caller triggers cannot write an arbitrary amount to the log.
//
// The Host header is caller-supplied and was logged verbatim, so the refusal
// meant to protect against DNS rebinding was also the cheapest way to fill an
// operator's disk. The bounded prefix keeps the line diagnostic, since a
// rebinding attempt is recognizable from its first bytes, and host_len keeps
// the fact that it was truncated visible.
func TestHostValidationMiddleware_LogsABoundedHost(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))

	handler := hostValidationMiddleware(map[string]bool{"localhost": true}, http.NotFoundHandler())
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost/mcp", http.NoBody)
	req.Host = strings.Repeat("a", 32_000)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if logged.Len() > 4096 {
		t.Errorf("a single refused request wrote %d bytes of log for a %d byte Host", logged.Len(), len(req.Host))
	}
	if !strings.Contains(logged.String(), `"host_len":32000`) {
		t.Errorf("the log does not report the real Host length: %s", logged.String())
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTPOn(ctx, cfg, addr, listener, defaultHTTPIdleTimeout)
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
		// The suffix, not the full header name: the SDK prepends
		// "Mcp-Param-" to this value, so the wire header a client sends is
		// Mcp-Param-Action. Asserting the full name here is what let
		// "Mcp-Param-Mcp-Param-Action" ship.
		if got := tool.InputSchema.Properties["action"].XMCPHeader; got != "Action" {
			t.Fatalf("gitlab_execute_action action x-mcp-header = %q, want Action", got)
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

// TestValidateHTTPAuthConfig_OAuthPublicURL verifies the flag path enforces
// the RFC 9728 constraints on --public-url: without it (or with an invalid
// value) an oauth server would advertise a broken protected-resource
// identifier, so startup must refuse instead.
func TestValidateHTTPAuthConfig_OAuthPublicURL(t *testing.T) {
	tests := []struct {
		name      string
		publicURL string
		wantErr   bool
	}{
		{"missing", "", true},
		{"relative", "gitlab", true},
		{"trailing slash", "https://mcp.example.com/gitlab/", true},
		{"http non-loopback", "http://mcp.example.com", true},
		{"valid https path", "https://mcp.example.com/gitlab", false},
		{"valid http loopback", "http://localhost:8080", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				AuthMode:  "oauth",
				GitLabURL: "https://gitlab.example.com",
				PublicURL: tt.publicURL,
			}
			err := validateHTTPAuthConfig(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateHTTPAuthConfig(public-url=%q) error = %v, wantErr %v", tt.publicURL, err, tt.wantErr)
			}
		})
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
	// Withheld, not unknown: the surface names the action and the decision
	// that removed it, so a caller is not told the capability is missing.
	if !strings.Contains(write, "exists but is not available") {
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
	// Not "unknown action": the action is known, it was withheld. The old
	// wording came with read-only near misses, which reads as a capability the
	// server lacks rather than one this deployment turned off.
	if !strings.Contains(write, "exists but is not available") {
		t.Errorf("mutating action was routable with read-only + safe mode: %s", write)
	}
	if !strings.Contains(write, "Ask the operator") {
		t.Errorf("read-only imposed by the operator must name the operator as the remedy: %s", write)
	}

	read := callModeTool(t, session, "gitlab_execute_action", map[string]any{
		"action": "project.list",
		"params": map[string]any{},
	})
	if !strings.Contains(read, "projectList") {
		t.Errorf("reads must keep working with read-only + safe mode: %s", read)
	}
}

// TestValidateHTTPAuthConfig_OAuthRequiresHTTPSGitLabURL verifies oauth mode
// refuses a cleartext instance URL: bearer tokens are forwarded upstream on
// every call, so http would put a live credential on the wire (CWE-319).
// Loopback stays exempt for local development, matching --public-url.
func TestValidateHTTPAuthConfig_OAuthRequiresHTTPSGitLabURL(t *testing.T) {
	tests := []struct {
		name      string
		gitlabURL string
		wantErr   bool
	}{
		{"https accepted", "https://gitlab.com", false},
		{"https self-managed accepted", "https://gitlab.internal.example", false},
		{"http loopback accepted", "http://localhost:8929", false},
		{"http 127.0.0.1 accepted", "http://127.0.0.1:8929", false},
		{"http public host rejected", "http://gitlab.example.com", true},
		{"not a URL rejected", "gitlab.example.com", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				AuthMode:  "oauth",
				GitLabURL: tt.gitlabURL,
				PublicURL: "https://mcp.example.com",
			}
			err := validateHTTPAuthConfig(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateHTTPAuthConfig(gitlab-url=%q) error = %v, wantErr %v", tt.gitlabURL, err, tt.wantErr)
			}
		})
	}
}

// TestCrossOriginProtectionMiddleware_TrustedOriginAndWildcard exercises the
// trusted-origins list end to end: a listed origin (including a bare IP for
// local deploys) passes a cross-site browser POST, an unlisted one is still
// rejected, and the "*" wildcard disables the protection for every origin.
func TestCrossOriginProtectionMiddleware_TrustedOriginAndWildcard(t *testing.T) {
	tests := []struct {
		name     string
		trusted  []string
		origin   string
		wantPass bool
	}{
		{"listed origin passes cross-site", []string{"https://ok.example"}, "https://ok.example", true},
		{"unlisted origin rejected", []string{"https://ok.example"}, "https://evil.example", false},
		{"IP origin passes for local deploy", []string{"http://192.168.1.50:8080"}, "http://192.168.1.50:8080", true},
		{"other IP rejected", []string{"http://192.168.1.50:8080"}, "http://192.168.1.99:8080", false},
		{"wildcard accepts any origin", []string{"*"}, "https://anything.example", true},
		{"empty list rejects cross-site", nil, "https://evil.example", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})
			handler := crossOriginProtectionMiddleware(tt.trusted, inner)

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://mcp.example/mcp", strings.NewReader(`{}`))
			req.Header.Set(hdrContentType, mimeJSON)
			req.Header.Set("Origin", tt.origin)
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if tt.wantPass && rr.Code != http.StatusOK {
				t.Errorf("origin %q: status = %d, want 200 (should pass the CORS check)", tt.origin, rr.Code)
			}
			if !tt.wantPass && rr.Code != http.StatusForbidden {
				t.Errorf("origin %q: status = %d, want 403 (should be rejected)", tt.origin, rr.Code)
			}
			if called != tt.wantPass {
				t.Errorf("origin %q: inner called = %v, want %v", tt.origin, called, tt.wantPass)
			}
		})
	}
}

// TestBuildTrustedOrigins_SeedsPublicURLOrigin verifies the public-url origin
// is merged into the trusted list (deduplicated), so a same-domain browser
// client on the deployment's declared origin is trusted without extra config.
func TestBuildTrustedOrigins_SeedsPublicURLOrigin(t *testing.T) {
	tests := []struct {
		name      string
		csv       string
		publicURL string
		want      []string
	}{
		{"public-url origin seeded", "", "https://mcp.jmrp.io/gitlab", []string{"https://mcp.jmrp.io"}},
		{"explicit plus public-url deduped", "https://mcp.jmrp.io", "https://mcp.jmrp.io/gitlab", []string{"https://mcp.jmrp.io"}},
		{"explicit list preserved", "https://a.example,https://b.example", "", []string{"https://a.example", "https://b.example"}},
		{"both combined", "https://a.example", "https://mcp.jmrp.io", []string{"https://a.example", "https://mcp.jmrp.io"}},
		{"empty yields nil", "", "", nil},
		{"wildcard passes through", "*", "", []string{"*"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTrustedOrigins(tt.csv, tt.publicURL)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("buildTrustedOrigins(%q, %q) = %v, want %v", tt.csv, tt.publicURL, got, tt.want)
			}
		})
	}
}

// TestCorsMiddleware_TrustedOriginPreflight_IsAnswered verifies the fix that
// makes --trusted-origins usable from an actual browser. Allowing an origin
// past the cross-origin protection is only half of it: a browser will not
// send the POST until a preflight comes back with permission, and in oauth
// mode that preflight carries no Authorization header by definition, so the
// bearer layer answered it 401 and the real request never happened.
func TestCorsMiddleware_TrustedOriginPreflight_IsAnswered(t *testing.T) {
	t.Parallel()

	var reached atomic.Bool
	handler := corsMiddleware(&config.Config{TrustedOrigins: []string{"https://claude.ai"}}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached.Store(true)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/mcp", http.NoBody)
	req.Header.Set("Origin", "https://claude.ai")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if reached.Load() {
		t.Error("a preflight must be answered here, not forwarded to the authenticated handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	want := map[string]string{
		"Access-Control-Allow-Origin":  "https://claude.ai",
		"Access-Control-Allow-Methods": corsAllowMethods,
		// A literal, not corsAllowHeadersFor(...): comparing the header
		// against the function that produced it would pass whatever the
		// function returned. What the list must CONTAIN per mode is pinned by
		// TestCorsAllowHeaders_FollowTheAuthMode.
		"Access-Control-Allow-Headers": "Authorization, Content-Type, Accept, " +
			"Mcp-Session-Id, Mcp-Protocol-Version, Last-Event-ID, " +
			"Mcp-Method, Mcp-Name, Mcp-Param-Action, " +
			"PRIVATE-TOKEN, GITLAB-URL",
		"Access-Control-Max-Age": corsMaxAge,
	}
	for name, value := range want {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := rec.Header().Get(name); got != value {
				t.Errorf("%s = %q, want %q", name, got, value)
			}
		})
	}
	// The origin is echoed, never "*": these requests carry Authorization,
	// and a browser rejects the wildcard on a credentialed request.
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Error("a credentialed endpoint must echo the origin, not answer with *")
	}
	if !slices.Contains(rec.Header().Values("Vary"), "Origin") {
		t.Error("the response varies by Origin and must say so")
	}
}

// TestCorsMiddleware_UntrustedOrigin_IsLeftToTheProtection verifies that this
// middleware never widens the trust decision: an origin the operator did not
// list gets no permission headers, and its ordinary request is passed down to
// the cross-origin protection, which is what refuses it.
//
// Its PREFLIGHT is answered here, though, and that distinction matters. A
// preflight carries no credential by definition, so forwarding it let the
// authentication layer count it as a failed authentication: ten of them locked
// the client's IP out of the whole endpoint, and a browser emits them without
// the user doing anything wrong. An answer with no Access-Control-Allow-Origin
// is a refusal the browser already understands.
func TestCorsMiddleware_UntrustedOrigin_IsLeftToTheProtection(t *testing.T) {
	t.Parallel()

	newHandler := func(reached *atomic.Bool) http.Handler {
		return corsMiddleware(&config.Config{TrustedOrigins: []string{"https://claude.ai"}}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reached.Store(true)
			w.WriteHeader(http.StatusOK)
		}))
	}

	// An untrusted origin's preflight is passed down rather than answered
	// here, because some routes serve their own — the RFC 9728 metadata
	// document and the server card are public and answer any origin, and
	// swallowing those would make them undiscoverable from a browser. What
	// must not happen is it being CHARGED as a failed authentication; that is
	// the bearer guard's job, pinned by
	// TestBearerGuard_PreflightIsNotAnAuthenticationFailure.
	t.Run("preflight is passed down, with no permission granted", func(t *testing.T) {
		t.Parallel()
		var reached atomic.Bool
		req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/mcp", http.NoBody)
		req.Header.Set("Origin", "https://evil.example")
		req.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()
		newHandler(&reached).ServeHTTP(rec, req)

		if !reached.Load() {
			t.Error("a preflight must reach the route, which may serve its own")
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want none for an untrusted origin", got)
		}
		// The answer downstream depends on Origin, so a cache must not serve
		// it to a trusted origin.
		if vary := rec.Header().Values("Vary"); !slices.Contains(vary, "Origin") {
			t.Errorf("Vary = %v, want it to include Origin", vary)
		}
	})

	t.Run("an ordinary request is passed to the protection", func(t *testing.T) {
		t.Parallel()
		var reached atomic.Bool
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
		req.Header.Set("Origin", "https://evil.example")
		rec := httptest.NewRecorder()
		newHandler(&reached).ServeHTTP(rec, req)

		if !reached.Load() {
			t.Error("an untrusted origin's real request must be passed through, not answered here")
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want none for an untrusted origin", got)
		}
	})
}

// TestCorsMiddleware_ActualRequest_ExposesTransportHeaders verifies the
// non-preflight half: a real cross-origin request from a trusted origin must
// carry the allow header, and must name the session and protocol headers,
// which a browser cannot read otherwise because neither is CORS-safelisted.
func TestCorsMiddleware_ActualRequest_ExposesTransportHeaders(t *testing.T) {
	t.Parallel()

	var reached atomic.Bool
	handler := corsMiddleware(&config.Config{TrustedOrigins: []string{"https://claude.ai"}}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Store(true)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
	req.Header.Set("Origin", "https://claude.ai")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !reached.Load() {
		t.Error("an actual request must reach the handler")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://claude.ai" {
		t.Errorf("Access-Control-Allow-Origin = %q, want the request origin", got)
	}
	if got := rec.Header().Get("Access-Control-Expose-Headers"); got != corsExposeHeaders {
		t.Errorf("Access-Control-Expose-Headers = %q, want %q", got, corsExposeHeaders)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "" {
		t.Error("Access-Control-Allow-Methods belongs to a preflight response only")
	}
}

// TestCorsMiddleware_WildcardAndNoOrigin_BehaveAsConfigured verifies the two
// edges: '*' answers any origin, and a request with no Origin header — every
// non-browser MCP client — is untouched.
func TestCorsMiddleware_WildcardAndNoOrigin_BehaveAsConfigured(t *testing.T) {
	t.Parallel()

	t.Run("wildcard trusts any origin", func(t *testing.T) {
		t.Parallel()
		handler := corsMiddleware(&config.Config{TrustedOrigins: []string{"*"}}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/mcp", http.NoBody)
		req.Header.Set("Origin", "https://anything.example")
		req.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://anything.example" {
			t.Errorf("Access-Control-Allow-Origin = %q, want the echoed origin", got)
		}
	})

	t.Run("no origin passes through untouched", func(t *testing.T) {
		t.Parallel()
		var reached atomic.Bool
		handler := corsMiddleware(&config.Config{TrustedOrigins: []string{"https://claude.ai"}}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			reached.Store(true)
		}))
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if !reached.Load() {
			t.Error("a request with no Origin must reach the handler")
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want none when there is no Origin", got)
		}
	})

	// With no trusted origins the middleware grants nothing and answers
	// nothing: the route below may serve its own preflight, and the
	// authentication layer is what must not charge it.
	t.Run("no trusted origins grants nothing and passes the preflight down", func(t *testing.T) {
		t.Parallel()
		var reached atomic.Bool
		handler := corsMiddleware(&config.Config{}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			reached.Store(true)
		}))
		req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/mcp", http.NoBody)
		req.Header.Set("Origin", "https://claude.ai")
		req.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if !reached.Load() {
			t.Error("a preflight must reach the route, which may serve its own")
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want none when no origin is trusted", got)
		}
	})

	t.Run("no trusted origins passes an ordinary request through", func(t *testing.T) {
		t.Parallel()
		var reached atomic.Bool
		handler := corsMiddleware(&config.Config{}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			reached.Store(true)
		}))
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
		req.Header.Set("Origin", "https://claude.ai")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if !reached.Load() {
			t.Error("with no trusted origins configured an ordinary request must pass through")
		}
	})
}

// TestCorsAllowHeaders_CoversEveryDeclaredParameterHeader pins the CORS
// allow-list against the x-mcp-header annotations the server actually declares,
// rather than against a copied list of names.
//
// The Mcp-Param-* names are not a fixed family: SEP-2243 derives each one from
// an annotation on a tool parameter, and this server declares exactly one, on
// the dynamic surface's `action` property. A hand-written list drifted from it
// and named three headers no tool declares while omitting the only real one.
// The consequence is invisible to a curl-based test, which never honors a
// preflight: a browser drops the unauthorized header, and the server then
// rejects the call with "header mismatch". Since dynamic is the default
// surface, that was every tool call from a browser.
//
// Asserting containment of the exported constant means the annotation and the
// allow-list can only be changed together.
func TestCorsAllowHeaders_CoversEveryDeclaredParameterHeader(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{config.AuthModeLegacy, config.AuthModeOAuth} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			got := corsAllowHeadersFor(&config.Config{AuthMode: mode})
			if !strings.Contains(got, dynamictools.ExecuteActionHeaderName) {
				t.Errorf("Access-Control-Allow-Headers = %q, missing %q — a browser would drop the header the server then demands",
					got, dynamictools.ExecuteActionHeaderName)
			}
		})
	}
}

// TestProtocolVersions_MatchTheSDK keeps our mirrored list honest.
//
// The SDK's supportedProtocolVersions is unexported, so this server restates it
// to answer an unsupported version with the error the spec requires. Restating
// it invites drift: an SDK bump that adds a revision would leave this server
// rejecting a version the SDK is happy to serve. The SDK names its own list in
// the plain-text rejection it emits for an old unknown version, so driving that
// path is a way to read it back without importing an unexported symbol.
func TestProtocolVersions_MatchTheSDK(t *testing.T) {
	t.Parallel()

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return nil }, nil)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("MCP-Protocol-Version", "1999-01-01")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	const marker = "supported versions: "
	idx := strings.Index(rec.Body.String(), marker)
	if idx < 0 {
		t.Skipf("the SDK no longer names its versions in that rejection: %q", rec.Body.String())
	}
	named := strings.TrimSpace(strings.TrimSuffix(rec.Body.String()[idx+len(marker):], ")\n"))
	named = strings.TrimSuffix(named, ")")

	want := strings.Join(supportedProtocolVersions, ",")
	if named != want {
		t.Errorf("the SDK supports %q but this server mirrors %q — update supportedProtocolVersions", named, want)
	}
}

// TestRateLimit_DefaultsDifferByTransport pins that tool-call limiting is on by
// default in HTTP mode and off in stdio.
//
// The specification requires a server exposing tools to rate limit their
// invocation. The mechanism existed and was correct, but shipped disabled, so an
// out-of-the-box HTTP deployment registered no limiter at all — and an HTTP
// deployment is the shared one, where a looping client's volume is charged to
// the server's own egress address and lands on every other tenant.
//
// Stdio stays at zero deliberately: a single-user local process has no co-tenant
// to protect, and a limiter there only costs latency. Both keep an explicit 0 as
// the opt-out.
// TestRateLimit_HTTPModeLimitsToolCallsByDefault pins that an HTTP deployment
// bounds tool invocations without being asked.
//
// The specification requires a server exposing tools to rate limit their
// invocation. The mechanism existed and was correct, but shipped disabled, so an
// out-of-the-box HTTP deployment registered no limiter at all — and an HTTP
// deployment is the shared one, where a looping client's volume is charged to
// the server's own egress address and lands on every other tenant.
func TestRateLimit_HTTPModeLimitsToolCallsByDefault(t *testing.T) {
	t.Parallel()

	if config.DefaultHTTPRateLimitRPS <= 0 {
		t.Fatalf("DefaultHTTPRateLimitRPS = %v; an HTTP deployment must bound tool calls",
			config.DefaultHTTPRateLimitRPS)
	}

	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	var rps float64
	fs.Float64Var(&rps, "rate-limit-rps", config.DefaultHTTPRateLimitRPS, "")
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rps != config.DefaultHTTPRateLimitRPS {
		t.Errorf("flag default = %v, want %v", rps, config.DefaultHTTPRateLimitRPS)
	}
}

// TestRateLimit_StdioLeavesItOffUnlessAsked pins the other half of that
// decision. A single-user local process has no co-tenant to protect, so a
// limiter there only costs latency; the env var stays at zero.
//
// Not parallel: it sets environment variables.
func TestRateLimit_StdioLeavesItOffUnlessAsked(t *testing.T) {
	t.Setenv("GITLAB_URL", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "glpat-whatever")
	t.Setenv("RATE_LIMIT_RPS", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.RateLimitRPS != 0 {
		t.Errorf("stdio RateLimitRPS = %v, want 0 — a local process has no co-tenant to protect", cfg.RateLimitRPS)
	}
}

// TestFilterActionCatalog_ReportsWhatItWithheldAndWhy pins that narrowing the
// catalog records what was removed, split by whose decision the caller can act
// on.
//
// The dynamic surface answers an action it cannot find with "unknown action"
// plus near misses. That is correct for a typo and a misdiagnosis for a
// narrowed credential: the near misses are all real read-only actions, so the
// answer reads as "this server cannot write" rather than "this token cannot".
// Splitting the two causes is what lets the surface say "reauthorize" only when
// reauthorizing would actually help.
//
// Tools removed by name through --exclude-tools stay out of both lists on
// purpose: the operator asked for them not to exist, so naming them in an error
// would leak the configuration and contradict it.
func TestFilterActionCatalog_ReportsWhatItWithheldAndWhy(t *testing.T) {
	t.Parallel()

	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{
		Tier:       edition.Free,
		IncludeMCP: true,
	})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	t.Run("a read-only token gets the reauthorize half", func(t *testing.T) {
		t.Parallel()
		assertWithheldByTokenScope(t, catalog)
	})

	t.Run("an operator-imposed read-only mode gets the other half", func(t *testing.T) {
		t.Parallel()
		assertWithheldByOperator(t, catalog)
	})

	t.Run("an excluded tool is not withheld, it is absent", func(t *testing.T) {
		t.Parallel()
		assertExcludedToolIsNotWithheld(t, catalog)
	})

	t.Run("nothing is withheld when nothing is narrowed", func(t *testing.T) {
		t.Parallel()
		assertNothingWithheld(t, catalog)
	})
}

// withheldWriteAction and withheldReadAction are the two catalog actions the
// withheld-bookkeeping assertions below are written against: one that read-only
// mode removes and one it must keep.
const (
	withheldWriteAction = "issue.create"
	withheldReadAction  = "issue.list"
)

// mustFilterCatalog runs the catalog filter and fails on error, returning both
// the narrowed catalog and the bookkeeping under test.
func mustFilterCatalog(t *testing.T, catalog *actioncatalog.Catalog, cfg *config.ServerConfig) (*actioncatalog.Catalog, withheldActions) {
	t.Helper()
	filtered, withheld, err := filterActionCatalog(catalog, cfg)
	if err != nil {
		t.Fatalf("filterActionCatalog() error = %v", err)
	}
	return filtered, withheld
}

func assertWithheldByTokenScope(t *testing.T, catalog *actioncatalog.Catalog) {
	t.Helper()
	filtered, withheld := mustFilterCatalog(t, catalog, &config.ServerConfig{
		ReadOnly:               true,
		ReadOnlyFromTokenScope: true,
	})
	if _, ok := filtered.Action(withheldWriteAction); ok {
		t.Fatalf("filterActionCatalog() kept %q in a read-only catalog", withheldWriteAction)
	}
	if !slices.Contains(withheld.byTokenScope, withheldWriteAction) {
		t.Errorf("withheld.byTokenScope does not name %q; a narrowed credential would be reported as a missing capability", withheldWriteAction)
	}
	if slices.Contains(withheld.byOperator, withheldWriteAction) {
		t.Errorf("withheld.byOperator names %q, but the token is the cause here", withheldWriteAction)
	}
	if slices.Contains(withheld.byTokenScope, withheldReadAction) {
		t.Errorf("withheld.byTokenScope names %q, which is still reachable", withheldReadAction)
	}
}

func assertWithheldByOperator(t *testing.T, catalog *actioncatalog.Catalog) {
	t.Helper()
	_, withheld := mustFilterCatalog(t, catalog, &config.ServerConfig{ReadOnly: true})
	if !slices.Contains(withheld.byOperator, withheldWriteAction) {
		t.Errorf("withheld.byOperator does not name %q", withheldWriteAction)
	}
	if slices.Contains(withheld.byTokenScope, withheldWriteAction) {
		t.Errorf("withheld.byTokenScope names %q, but no credential narrowed this deployment", withheldWriteAction)
	}
}

func assertExcludedToolIsNotWithheld(t *testing.T, catalog *actioncatalog.Catalog) {
	t.Helper()
	_, withheld := mustFilterCatalog(t, catalog, &config.ServerConfig{
		ReadOnly:     true,
		ExcludeTools: []string{"gitlab_issue"},
	})
	for _, keys := range [][]string{withheld.byTokenScope, withheld.byOperator} {
		if slices.Contains(keys, withheldWriteAction) {
			t.Errorf("an excluded tool's action %q was reported as withheld; exclusion means it does not exist here", withheldWriteAction)
		}
	}
}

func assertNothingWithheld(t *testing.T, catalog *actioncatalog.Catalog) {
	t.Helper()
	_, withheld := mustFilterCatalog(t, catalog, &config.ServerConfig{})
	if len(withheld.byTokenScope) != 0 || len(withheld.byOperator) != 0 {
		t.Errorf("withheld = %+v, want empty for an unnarrowed catalog", withheld)
	}
}

// TestDynamicSurface_ScopeNarrowedWriteReportsTheCredential wires the whole
// path: a token that cannot write narrows the pool entry, the catalog filter
// records what that removed, and the dynamic surface says so when the action is
// asked for.
//
// The unit tests either side of this cover the filter's bookkeeping and the
// registry's wording; what only an assembled server can prove is that the two
// are actually connected — the registration call is a single line, and a
// catalog built without it produces a fluent, confident, wrong answer.
func TestDynamicSurface_ScopeNarrowedWriteReportsTheCredential(t *testing.T) {
	session := modeTestSession(t, &config.ServerConfig{
		ToolSurface:            config.ToolSurfaceDynamic,
		ReadOnly:               true,
		ReadOnlyFromTokenScope: true,
	})

	text := callModeTool(t, session, "gitlab_execute_action", map[string]any{
		"action": "issue.create",
		"params": map[string]any{"project_id": 1, "title": "irrelevant"},
	})

	if !strings.Contains(text, "exists but is not available") {
		t.Fatalf("gitlab_execute_action(issue.create) = %q, want it reported as withheld", text)
	}
	if !strings.Contains(text, "api scope") {
		t.Errorf("gitlab_execute_action(issue.create) = %q, want the remedy to name the scope to reauthorize with", text)
	}
	if strings.Contains(text, "Did you mean") {
		t.Errorf("gitlab_execute_action(issue.create) = %q, must not offer read-only near misses for an action the credential simply cannot reach", text)
	}
}

// TestCorsExposeHeaders_ARateLimitedBrowserClientCanReadRetryAfter pins that a
// header the server sets on a throttled response is one a cross-origin client
// can actually read.
//
// Retry-After is an unsafelisted response header: without naming it in
// Access-Control-Expose-Headers the browser strips it, so a client that is being
// asked to slow down cannot see for how long and falls back to its own guess.
// That turns a rate limit into a retry storm aimed at the limit that caused it.
// The 429 here is the auth-failure lockout, but the same header carries the
// tool-call limiter's backoff and GitLab's own throttle passed through as 503.
//
// The assertion runs against the real gate rather than the constant alone, so
// dropping Retry-After from either side fails.
func TestCorsExposeHeaders_ARateLimitedBrowserClientCanReadRetryAfter(t *testing.T) {
	gate := newGate(t, okFactory)
	handler := corsMiddleware(
		&config.Config{TrustedOrigins: []string{"https://claude.ai"}},
		gate.middleware(http.NotFoundHandler()),
	)

	newRequest := func() *http.Request {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader("{}"))
		req.Header.Set("Origin", "https://claude.ai")
		return req
	}
	for range authFailureLimit {
		handler.ServeHTTP(httptest.NewRecorder(), newRequest())
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newRequest())

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d after %d failures", rec.Code, http.StatusTooManyRequests, authFailureLimit)
	}
	if rec.Header().Get(headerRetryAfter) == "" {
		t.Fatal("429 without Retry-After leaves the client guessing")
	}

	exposed := strings.Split(rec.Header().Get("Access-Control-Expose-Headers"), ",")
	for i, name := range exposed {
		exposed[i] = http.CanonicalHeaderKey(strings.TrimSpace(name))
	}
	if !slices.Contains(exposed, http.CanonicalHeaderKey(headerRetryAfter)) {
		t.Errorf("Access-Control-Expose-Headers = %v, want it to name %q so the browser does not strip the backoff",
			exposed, headerRetryAfter)
	}
}

// TestTransportRejections_AreJSONRPCErrorsNotPlainText pins that the two
// remaining transport-level refusals answer in the shape every other refusal in
// this binary uses.
//
// The Streamable HTTP specification tells a client that receives a 4xx whose
// body is not a recognized JSON-RPC error to conclude the server predates
// version negotiation and downgrade. Host validation used http.Error's plain
// "forbidden", and the standard library's cross-origin protection writes its own
// plain-text refusal, so a rebinding guard and an origin guard — both of which
// fire on perfectly ordinary misconfiguration — were reporting themselves as a
// protocol generation rather than as a policy decision.
func TestTransportRejections_AreJSONRPCErrorsNotPlainText(t *testing.T) {
	t.Parallel()

	reached := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

	tests := []struct {
		name    string
		handler http.Handler
		request func(context.Context) *http.Request
	}{
		{
			name:    "host validation",
			handler: hostValidationMiddleware(map[string]bool{"localhost": true}, http.HandlerFunc(reached)),
			request: func(ctx context.Context) *http.Request {
				req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/mcp", strings.NewReader("{}"))
				req.Host = "rebound.example"
				return req
			},
		},
		{
			name:    "cross-origin protection",
			handler: crossOriginProtectionMiddleware(nil, http.HandlerFunc(reached)),
			request: func(ctx context.Context) *http.Request {
				req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/mcp", strings.NewReader("{}"))
				req.Header.Set("Sec-Fetch-Site", "cross-site")
				req.Header.Set("Origin", "https://evil.example")
				return req
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			tt.handler.ServeHTTP(rec, tt.request(t.Context()))

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			decoded := decodeJSONRPCError(t, rec.Body.String())
			if decoded.Error.Code != errCodeForbidden {
				t.Errorf("error.code = %d, want %d", decoded.Error.Code, errCodeForbidden)
			}
			if decoded.Error.Message == "" {
				t.Error("a refusal with no message tells the operator nothing about which guard fired")
			}
		})
	}
}

// TestSSEKeepAlive_IdleStreamKeepsBytesOnTheWire pins the heartbeat on
// long-lived SSE responses.
//
// Clearing the write deadline stops this end of the connection from timing out
// and does nothing about the hops in between: nginx closes an idle upstream
// response at proxy_read_timeout (60s by default), and carrier NATs are
// tighter. The two streams this transport depends on are silent by design — the
// standalone GET carrying server-initiated messages, and a POST held open for
// the length of a tool call — so they are precisely what an idle timer collects.
//
// A comment frame is the cheapest legal thing to send: a line starting with ':'
// carries no field, so a conforming reader discards it without producing an
// event.
func TestSSEKeepAlive_IdleStreamKeepsBytesOnTheWire(t *testing.T) {
	restore := sseKeepAliveInterval
	sseKeepAliveInterval = 10 * time.Millisecond
	t.Cleanup(func() { sseKeepAliveInterval = restore })

	release := make(chan struct{})
	handler := sseWriteDeadlineMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// A stream that says nothing at all, which is the ordinary state of a
		// standalone GET between server-initiated messages.
		<-release
	}))

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	// Released on every exit path, and before the cleanup that waits for the
	// handler: a defer runs ahead of t.Cleanup, so a failed assertion reports
	// itself instead of deadlocking Close.
	defer close(release)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	// A bounded request: with no heartbeat this stream is so silent that even
	// the response header never reaches the wire, so an unbounded Do would
	// report a missing feature as a stuck suite.
	client := server.Client()
	client.Timeout = 5 * time.Second
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v — an idle SSE response never flushed, so a proxy's read timeout would sever it", err)
	}
	defer func() { _ = resp.Body.Close() }()

	buf := make([]byte, len(sseKeepAliveFrame))
	if _, readErr := io.ReadFull(resp.Body, buf); readErr != nil {
		t.Fatalf("reading the idle stream: %v — an idle SSE response sent nothing", readErr)
	}

	if got := string(buf); got != string(sseKeepAliveFrame) {
		t.Errorf("idle stream wrote %q, want the keep-alive comment %q", got, sseKeepAliveFrame)
	}
}

// TestSSEKeepAlive_LeavesNonStreamingResponsesAlone verifies the heartbeat is
// scoped to SSE. Injecting a comment into a JSON body would corrupt it, and
// --json-response is a supported mode, so the Content-Type check is what keeps
// the two apart.
func TestSSEKeepAlive_LeavesNonStreamingResponsesAlone(t *testing.T) {
	restore := sseKeepAliveInterval
	sseKeepAliveInterval = 5 * time.Millisecond
	t.Cleanup(func() { sseKeepAliveInterval = restore })

	handler := sseWriteDeadlineMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		time.Sleep(25 * time.Millisecond)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody))

	if got := rec.Body.String(); got != `{"jsonrpc":"2.0","id":1,"result":{}}` {
		t.Errorf("body = %q, want the JSON response untouched by a keep-alive", got)
	}
}

// TestSSEKeepAlive_StopsBeforeTheHandlerReturns verifies the heartbeat never
// touches a ResponseWriter the handler has finished with.
//
// net/http reuses and invalidates the writer once ServeHTTP returns, so a
// ticker goroutine outliving the handler is a use-after-free in all but name.
// Run under -race, this also covers the other half: the keep-alive and the
// handler write to the same writer from two goroutines, which is only safe
// because both take the same lock.
func TestSSEKeepAlive_StopsBeforeTheHandlerReturns(t *testing.T) {
	restore := sseKeepAliveInterval
	sseKeepAliveInterval = time.Millisecond
	t.Cleanup(func() { sseKeepAliveInterval = restore })

	var writer *sseAwareWriter
	handler := sseWriteDeadlineMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writer = w.(*sseAwareWriter)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for range 20 {
			_, _ = w.Write([]byte("data: {}\n\n"))
			time.Sleep(time.Millisecond)
		}
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/mcp", http.NoBody))

	if writer == nil {
		t.Fatal("handler did not receive the wrapping writer")
	}
	writer.mu.Lock()
	stopped := writer.stopped
	writer.mu.Unlock()
	if !stopped {
		t.Error("the keep-alive was still running after ServeHTTP returned")
	}
	select {
	case <-writer.done:
	default:
		t.Error("stopKeepAlive returned without waiting for the heartbeat goroutine")
	}
}

// TestKeepAliveInterval_HTTPPoolEntriesRunWithoutTheServerPing pins that the
// SDK's server-initiated keepalive is off for every HTTP pool entry, and still
// on for stdio.
//
// The keepalive is a JSON-RPC ping request, and the SDK closes the session the
// first time one goes unanswered. On a stateless stream the transport forbids
// the request outright, which is why stateless already disabled it — but a
// stateful HTTP client that is merely between requests, or whose transport does
// not deliver server-initiated messages to it, was losing its session at the
// 30-second mark for being idle, regardless of --session-timeout.
//
// Liveness on this transport is the SSE keep-alive comment instead: it puts
// bytes on the wire without asking the client for anything.
func TestKeepAliveInterval_HTTPPoolEntriesRunWithoutTheServerPing(t *testing.T) {
	t.Parallel()

	stateful := &config.ServerConfig{Stateless: false}

	// stdio used to keep the ping, justified as doing so "where it is
	// protocol-legal". The legality is per protocol version, not per transport:
	// 2026-07-28 removes ping and limits a server-sent notifications/cancelled
	// to subscriptions/listen, and the SDK pings regardless and then emits that
	// notification for its own timed-out ping. At KeepAliveFailureThreshold 1,
	// a conformant client of that revision cannot answer, and the session dies
	// — observed as the stdio process exiting 45 seconds into an idle session.
	// The SDK starts the keepalive at session creation, before any request
	// reveals the revision, so there is no version to gate on.
	if got := keepAliveInterval(newServerSettings(nil), stateful); got != 0 {
		t.Errorf("keepAliveInterval(stateful, no override) = %v, want 0 — a server-initiated ping is not ours to send at 2026-07-28, and the SDK starts it before the revision is known", got)
	}

	poolOptions := []serverOption{withSessionTag("tag"), withKeepAlive(0)}
	if got := keepAliveInterval(newServerSettings(poolOptions), stateful); got != 0 {
		t.Errorf("keepAliveInterval(stateful, pool options) = %v, want 0 — a stateful HTTP session must not be closed for not answering a ping", got)
	}

	stateless := &config.ServerConfig{Stateless: true}
	if got := keepAliveInterval(newServerSettings(nil), stateless); got != 0 {
		t.Errorf("keepAliveInterval(stateless, no override) = %v, want 0 — the transport forbids a server-initiated request on that stream", got)
	}
}

// TestSupportedProtocolVersionsFor_StatefulDropsTheStatelessOnlyRevision pins
// that the advertised list matches what the transport can actually negotiate.
//
// The SDK's StreamableServerTransport.SupportsProtocolVersion refuses every
// revision at or above 2026-07-28 unless the transport is stateless: SEP-2575
// has no session concept to fall back on. The whole purpose of the
// UnsupportedProtocolVersion error body is to tell a client what to retry with,
// so a stateful deployment listing 2026-07-28 there hands back the one answer
// that cannot work — and the client's single retry is spent on it.
func TestSupportedProtocolVersionsFor_StatefulDropsTheStatelessOnlyRevision(t *testing.T) {
	t.Parallel()

	stateless := supportedProtocolVersionsFor(true)
	if !slices.Contains(stateless, protocolVersionStatelessOnly) {
		t.Errorf("stateless supported = %v, want it to include %q", stateless, protocolVersionStatelessOnly)
	}
	if len(stateless) != len(supportedProtocolVersions) {
		t.Errorf("stateless supported = %v, want the full list %v", stateless, supportedProtocolVersions)
	}

	stateful := supportedProtocolVersionsFor(false)
	if slices.Contains(stateful, protocolVersionStatelessOnly) {
		t.Errorf("stateful supported = %v, must not advertise %q — the SDK refuses it without stateless", stateful, protocolVersionStatelessOnly)
	}
	if len(stateful) == 0 {
		t.Fatal("stateful supported is empty; a stateful deployment still negotiates every legacy revision")
	}
	for _, version := range stateful {
		if !slices.Contains(supportedProtocolVersions, version) {
			t.Errorf("stateful supported names %q, which is not a version this server implements", version)
		}
	}
}

// TestProtocolVersionMiddleware_StatefulRefusesTheStatelessOnlyRevision checks
// the same narrowing where a client sees it: the 400 body.
func TestProtocolVersionMiddleware_StatefulRefusesTheStatelessOnlyRevision(t *testing.T) {
	t.Parallel()

	var reached atomic.Bool
	handler := protocolVersionMiddleware(false, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Store(true)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader("{}"))
	req.Header.Set("MCP-Protocol-Version", protocolVersionStatelessOnly)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if reached.Load() {
		t.Error("a stateful server must refuse the stateless-only revision before the SDK does, so the client is told what to retry with")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var body struct {
		Error struct {
			Code int `json:"code"`
			Data struct {
				Supported []string `json:"supported"`
				Requested string   `json:"requested"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v (body=%q)", err, rec.Body.String())
	}
	if body.Error.Code != codeUnsupportedProtocolVersion {
		t.Errorf("error.code = %d, want %d", body.Error.Code, codeUnsupportedProtocolVersion)
	}
	if body.Error.Data.Requested != protocolVersionStatelessOnly {
		t.Errorf("error.data.requested = %q, want %q", body.Error.Data.Requested, protocolVersionStatelessOnly)
	}
	if slices.Contains(body.Error.Data.Supported, protocolVersionStatelessOnly) {
		t.Errorf("error.data.supported = %v, must not name the version just refused", body.Error.Data.Supported)
	}
	if len(body.Error.Data.Supported) == 0 {
		t.Error("error.data.supported is empty; the client is left with nothing to retry")
	}
}

// TestRemovedActionKeys_CoverCompatibilityAliases pins that an action's older
// names are withheld alongside its canonical one.
//
// The dynamic registry resolves compatibility aliases exactly as it resolves
// declared ones, so a caller working from a name the catalog used to carry
// would otherwise be told the action is unknown — which is precisely the
// misdiagnosis the withheld path exists to prevent, arriving through the door
// left open.
func TestRemovedActionKeys_CoverCompatibilityAliases(t *testing.T) {
	t.Parallel()

	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{
		Tier:       edition.Free,
		IncludeMCP: true,
	})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	var wantAlias string
	var wantAction string
	for _, action := range catalog.Actions() {
		if action.ReadOnly || len(action.Compatibility.ActionAliases) == 0 {
			continue
		}
		wantAlias = action.Compatibility.ActionAliases[0].Alias
		wantAction = string(action.ID)
		break
	}
	if wantAlias == "" {
		t.Skip("no mutating action in the Free catalog declares a compatibility alias")
	}

	_, withheld := mustFilterCatalog(t, catalog, &config.ServerConfig{
		ReadOnly:               true,
		ReadOnlyFromTokenScope: true,
	})
	if !slices.Contains(withheld.byTokenScope, wantAlias) {
		t.Errorf("withheld.byTokenScope omits %q, the compatibility alias of the withheld action %q; a caller using it would be told the action does not exist",
			wantAlias, wantAction)
	}
}

// TestValidateHTTPAuthConfig_OAuthRefusesSkipTLSVerify pins that oauth mode will
// not forward a bearer token over a connection whose certificate is unchecked.
//
// The existing rule refuses an http instance because "bearer tokens are
// forwarded to the instance on every call, and http would transmit them in
// cleartext". An https instance with verification disabled has the same
// property with extra steps: any host that can answer for the address is handed
// a live credential, which OAuth 2.1 section 7.1.3.2 names outright ("the client
// MUST validate the TLS certificate chain when making requests to protected
// resources"). Allowing one while refusing the other is a distinction without a
// difference.
//
// Loopback keeps the exemption for the same reason cleartext does: the
// credential does not leave the host.
func TestValidateHTTPAuthConfig_OAuthRefusesSkipTLSVerify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		authMode  string
		instances []string
		skipTLS   bool
		wantErr   bool
	}{
		{
			name:      "oauth refuses a remote instance with verification off",
			authMode:  "oauth",
			instances: []string{"https://gitlab.example.com"},
			skipTLS:   true,
			wantErr:   true,
		},
		{
			name:      "one unverified instance among several is enough to refuse",
			authMode:  "oauth",
			instances: []string{"https://localhost:8443", "https://gitlab.example.com"},
			skipTLS:   true,
			wantErr:   true,
		},
		{
			name:      "loopback keeps the exemption",
			authMode:  "oauth",
			instances: []string{"https://localhost:8443"},
			skipTLS:   true,
			wantErr:   false,
		},
		{
			name:      "oauth without the flag is unaffected",
			authMode:  "oauth",
			instances: []string{"https://gitlab.example.com"},
			skipTLS:   false,
			wantErr:   false,
		},
		{
			name:      "legacy mode keeps the flag",
			authMode:  "legacy",
			instances: []string{"https://gitlab.example.com"},
			skipTLS:   true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{
				AuthMode:      tt.authMode,
				GitLabURL:     tt.instances[0],
				GitLabURLs:    tt.instances,
				SkipTLSVerify: tt.skipTLS,
				PublicURL:     "https://mcp.example.com",
			}
			err := validateHTTPAuthConfig(cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("validateHTTPAuthConfig() = nil; the deployment would forward bearer tokens to an unverified peer")
				}
				if !strings.Contains(err.Error(), "skip-tls-verify") {
					t.Errorf("error = %q, want it to name the flag responsible", err)
				}
				return
			}
			if err != nil {
				t.Errorf("validateHTTPAuthConfig() = %v, want nil", err)
			}
		})
	}
}

// TestToolsList_FitsInOnePage is the guard on a number that has to stay ahead
// of the catalog.
//
// PageSize is 2000 because one shipping client — OpenAI Codex — ignores
// nextCursor and silently uses only the first page, so a catalog that outgrew
// the page size would not fail loudly: it would hand that client a partial tool
// list and let it conclude the missing tools do not exist.
//
// The largest surface is the individual one at the highest tier, and it grows
// every time a domain is added. Nothing watched the gap between the two numbers
// until now, which is the kind of limit that is only ever noticed after it is
// crossed.
func TestToolsList_FitsInOnePage(t *testing.T) {
	server, err := createServer(t.Context(), sharedCreateServerClient(t), &config.ServerConfig{
		ToolSurface: config.ToolSurfaceIndividual,
		Tier:        edition.Ultimate,
	})
	if err != nil {
		t.Fatalf("createServer: %v", err)
	}

	session := connectInMemory(t, server)
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}

	if listed.NextCursor != "" {
		t.Fatalf("tools/list paginated at %d tools; a client that ignores nextCursor now sees a partial catalog",
			len(listed.Tools))
	}
	t.Logf("the largest surface lists %d tools in one page", len(listed.Tools))
}

// TestUploadMaxFileSize_HTTPMode_HonorsTheSetting is the regression for a flag
// that worked on one transport and silently did nothing on the other.
//
// --upload-max-file-size writes UPLOAD_MAX_FILE_SIZE into the environment,
// which config.Load reads. HTTP mode does not go through Load: it builds its
// configuration from flags, and that path hardcoded the default. So an operator
// raising the limit saw it applied over stdio and ignored over HTTP, with
// nothing anywhere saying which they had got.
func TestUploadMaxFileSize_HTTPMode_HonorsTheSetting(t *testing.T) {
	tests := []struct {
		name  string
		set   bool
		value string
		want  int64
	}{
		{name: "unset falls back to the default", want: config.DefaultMaxFileSize},
		{name: "a plain byte count", set: true, value: "1048576", want: 1048576},
		{name: "a human-friendly size", set: true, value: "50MB", want: 50 * 1024 * 1024},
		{
			name:  "an unparseable value falls back rather than failing the start",
			set:   true,
			value: "not-a-size",
			want:  config.DefaultMaxFileSize,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Cleared first, so the "unset" case tests an absent variable
			// rather than whatever the developer's shell happens to export.
			t.Setenv("UPLOAD_MAX_FILE_SIZE", "")
			if tc.set {
				t.Setenv("UPLOAD_MAX_FILE_SIZE", tc.value)
			}
			if got := uploadMaxFileSize(); got != tc.want {
				t.Errorf("uploadMaxFileSize() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestServerCard_AnnouncesTelemetryWithoutNamingTheCollector is two assertions
// that pull in opposite directions, which is why they belong in one test.
//
// The card must SAY telemetry is running. The switch is off by default for
// privacy, and a privacy default nobody can observe is worth less than one they
// can: somebody connecting to a published endpoint should be able to see that
// their calls are instrumented without having to ask the operator.
//
// The card must NOT say where the telemetry goes. The collector endpoint names
// the operator's own infrastructure, and a server card is fetched by every
// client that asks. What a caller needs is that their calls are recorded and in
// what form; where the records land is not theirs to know.
func TestServerCard_AnnouncesTelemetryWithoutNamingTheCollector(t *testing.T) {
	const collector = "https://collector.internal.example:4318"

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector)
	t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "200")

	provider, err := telemetry.Start(t.Context(), telemetry.Config{
		Enabled: true,
		Signals: telemetry.Signals{Traces: true},
	})
	if err != nil {
		t.Fatalf("starting telemetry: %v", err)
	}
	t.Cleanup(func() { _ = provider.Shutdown(boundedShutdown(t)) })

	cfg := &config.Config{SkipTLSVerify: true, MetaTools: true}
	data, err := buildServerCard(t.Context(), cfg)
	if err != nil {
		t.Fatalf("buildServerCard: %v", err)
	}

	if strings.Contains(string(data), collector) {
		t.Errorf("the server card names the collector endpoint; it identifies the operator's infrastructure and every client can read this document\n%s", data)
	}
	if strings.Contains(string(data), "collector.internal.example") {
		t.Error("the collector host reached the card in some other form")
	}

	var card map[string]any
	if unmarshalErr := json.Unmarshal(data, &card); unmarshalErr != nil {
		t.Fatalf("the card is not valid JSON: %v", unmarshalErr)
	}
	block, ok := card["telemetry"].(map[string]any)
	if !ok {
		t.Fatalf("the card carries no telemetry block while telemetry is running:\n%s", data)
	}
	if enabled, _ := block["enabled"].(bool); !enabled {
		t.Error("the telemetry block does not report itself enabled")
	}
	if signals, _ := block["signals"].([]any); len(signals) == 0 {
		t.Error("the telemetry block names no signals, so a reader cannot tell what is recorded")
	}
	for _, key := range []string{"recorded", "not_recorded"} {
		t.Run(key, func(t *testing.T) {
			if text, _ := block[key].(string); text == "" {
				t.Errorf("the telemetry block has no %q line; the point of announcing is to say what is captured", key)
			}
		})
	}
}

// TestServerCard_OmitsTelemetryWhenItIsOff covers every deployment that never
// enables it, which is the default and therefore the common case.
//
// Absent rather than "enabled": false. A consumer should not have to parse a
// negation to learn that nothing is being recorded, and a block that is always
// present invites one written by hand that says the wrong thing.
func TestServerCard_OmitsTelemetryWhenItIsOff(t *testing.T) {
	provider, err := telemetry.Start(t.Context(), telemetry.Config{Enabled: false})
	if err != nil {
		t.Fatalf("starting telemetry: %v", err)
	}
	t.Cleanup(func() { _ = provider.Shutdown(boundedShutdown(t)) })

	cfg := &config.Config{SkipTLSVerify: true, MetaTools: true}
	data, err := buildServerCard(t.Context(), cfg)
	if err != nil {
		t.Fatalf("buildServerCard: %v", err)
	}

	var card map[string]any
	if unmarshalErr := json.Unmarshal(data, &card); unmarshalErr != nil {
		t.Fatalf("the card is not valid JSON: %v", unmarshalErr)
	}
	if _, present := card["telemetry"]; present {
		t.Errorf("the card carries a telemetry block with telemetry off:\n%s", data)
	}
}

// The gatewaycompat wiring tests verify that createServer installs the
// description-substitution middleware when GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS
// is set, that an unset variable installs nothing, and that an invalid value
// fails both createServer and runHTTP's early validation instead of surfacing
// on somebody's first pool request.

// escapeSubstitutionHalf escapes one half of an old=new pair so arbitrary
// served text can be used as the pattern.
func escapeSubstitutionHalf(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ",", `\,`)
	return strings.ReplaceAll(s, "=", `\=`)
}

// firstDescribedResource returns the URI and description of the first
// resource in the server's listing that carries a description; resources are
// served from the compiled-in registry, so no GitLab backend is involved.
func firstDescribedResource(t *testing.T, session *mcp.ClientSession) (uri, description string) {
	t.Helper()
	res, err := session.ListResources(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	for _, r := range res.Resources {
		if r.Description != "" {
			return r.URI, r.Description
		}
	}
	t.Fatal("no resource with a description; the fixture surface changed")
	return "", ""
}

// TestCreateServer_GatewayCompatWiring verifies the middleware end to end on
// a server built by createServer: a substitution whose pattern is a real
// served description rewrites that description, and only servers built while
// the variable is set are affected. The pattern is read from a control
// server first, so the test holds whatever the catalog's wording becomes.
func TestCreateServer_GatewayCompatWiring(t *testing.T) {
	client := newMockGitLabClient(t)
	cfg := &config.ServerConfig{MetaTools: true, ToolSurface: config.ToolSurfaceDynamic}

	control, err := createServer(t.Context(), client, cfg)
	if err != nil {
		t.Fatalf("createServer (control): %v", err)
	}
	controlSession := connectSessionAs(t, control, &mcp.Implementation{Name: "test-client", Version: "1.0.0"})
	uri, original := firstDescribedResource(t, controlSession)

	const rewritten = "REWRITTEN BY GATEWAYCOMPAT"
	t.Setenv(gatewaycompat.EnvVar, escapeSubstitutionHalf(original)+"="+escapeSubstitutionHalf(rewritten))

	server, err := createServer(t.Context(), client, cfg)
	if err != nil {
		t.Fatalf("createServer (substituted): %v", err)
	}
	session := connectSessionAs(t, server, &mcp.Implementation{Name: "test-client", Version: "1.0.0"})
	res, err := session.ListResources(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListResources (substituted): %v", err)
	}
	for _, r := range res.Resources {
		if r.URI != uri {
			continue
		}
		if r.Description != rewritten {
			t.Errorf("resource %s description = %q, want %q", uri, r.Description, rewritten)
		}
		return
	}
	t.Fatalf("resource %s missing from the substituted server's listing", uri)
}

// TestCreateServer_GatewayCompatInvalidValue verifies that a malformed
// substitution value fails the server build with an error naming the
// variable, rather than installing a half-parsed rewrite.
func TestCreateServer_GatewayCompatInvalidValue(t *testing.T) {
	t.Setenv(gatewaycompat.EnvVar, "=x")
	client := newMockGitLabClient(t)
	_, err := createServer(t.Context(), client, &config.ServerConfig{MetaTools: true, ToolSurface: config.ToolSurfaceDynamic})
	if err == nil {
		t.Fatal("createServer accepted a malformed substitution value")
	}
	if !strings.Contains(err.Error(), gatewaycompat.EnvVar) {
		t.Errorf("error = %q, want it to name %s", err, gatewaycompat.EnvVar)
	}
}

// TestRunHTTP_InvalidDescriptionSubstitutions verifies the early validation:
// in HTTP mode the pool builds servers lazily, so the malformed value must
// fail at startup, before the listener comes up.
func TestRunHTTP_InvalidDescriptionSubstitutions(t *testing.T) {
	t.Setenv(gatewaycompat.EnvVar, "abc")
	err := runHTTP(context.Background(), &httpConfig{
		gitlabURL:      "https://gitlab.example.com",
		maxHTTPClients: config.DefaultMaxHTTPClients,
		sessionTimeout: config.DefaultSessionTimeout,
	})
	if err == nil {
		t.Fatal("runHTTP accepted a malformed substitution value")
	}
	if !strings.Contains(err.Error(), gatewaycompat.EnvVar) {
		t.Errorf("error = %q, want it to name %s", err, gatewaycompat.EnvVar)
	}
}

// The clientcompat wiring tests verify that createServer installs the
// per-client compatibility middleware on the production path: a session that
// identifies as OpenAI Codex receives resource annotations with integer
// priorities, a generic session keeps the original floats, and the
// CLIENT_COMPAT=off kill switch disables the rewrite at server build time.

// connectSessionAs attaches an in-memory client with the given identity to a
// server built by createServer.
func connectSessionAs(t *testing.T, server *mcp.Server, impl *mcp.Implementation) *mcp.ClientSession {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })
	session, err := mcp.NewClient(impl, nil).Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// resourcePriorities lists the annotation priorities of the server's
// resources/list result (resources are served from the registry, so no
// GitLab backend call is involved) split into float and integer counts.
func resourcePriorities(t *testing.T, session *mcp.ClientSession) (floats, integers int) {
	t.Helper()
	res, err := session.ListResources(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	for _, r := range res.Resources {
		if r.Annotations == nil || r.Annotations.Priority == 0 {
			continue
		}
		if r.Annotations.Priority == float64(int64(r.Annotations.Priority)) {
			integers++
		} else {
			floats++
		}
	}
	return floats, integers
}

// TestCreateServer_ClientCompatMiddlewareWiring verifies the middleware is
// active on servers built by createServer: Codex sessions see only integer
// priorities while generic sessions of the same server keep the floats.
func TestCreateServer_ClientCompatMiddlewareWiring(t *testing.T) {
	client := newMockGitLabClient(t)
	server := mustCreateServer(t, client, &config.ServerConfig{MetaTools: true, ToolSurface: config.ToolSurfaceDynamic})

	codex := connectSessionAs(t, server, &mcp.Implementation{Name: "codex-mcp-client", Title: "Codex", Version: "0.148.0"})
	floats, integers := resourcePriorities(t, codex)
	if floats != 0 {
		t.Errorf("codex session sees %d float priorities, want 0", floats)
	}
	if integers == 0 {
		t.Error("codex session sees no annotated resources; fixture lost its annotations")
	}

	generic := connectSessionAs(t, server, &mcp.Implementation{Name: "test-client", Version: "1.0.0"})
	floats, _ = resourcePriorities(t, generic)
	if floats == 0 {
		t.Error("generic session sees no float priorities; registry was sanitized")
	}
}

// TestCreateServer_ClientCompatKillSwitch verifies CLIENT_COMPAT=off skips
// the middleware install, so even Codex sessions keep the float priorities.
func TestCreateServer_ClientCompatKillSwitch(t *testing.T) {
	t.Setenv("CLIENT_COMPAT", "off")
	client := newMockGitLabClient(t)
	server := mustCreateServer(t, client, &config.ServerConfig{MetaTools: true, ToolSurface: config.ToolSurfaceDynamic})

	codex := connectSessionAs(t, server, &mcp.Implementation{Name: "codex-mcp-client", Title: "Codex", Version: "0.148.0"})
	floats, _ := resourcePriorities(t, codex)
	if floats == 0 {
		t.Error("CLIENT_COMPAT=off but codex session sees no float priorities; middleware still active")
	}
}

// TestPublicPaths_MountEveryPublicRouteUnderTheForwardedPrefix covers the
// routing a reverse proxy that forwards its own prefix produces.
//
// Such a deployment reaches this server at /prefix/health rather than /health,
// and mounting only the bare paths left it answering 404 for its own health
// check, server card and RFC 9728 document — the three things an operator and a
// scanner reach for first. The prefix is not new configuration: --public-url
// already states the path the deployment is published under.
func TestPublicPaths_MountEveryPublicRouteUnderTheForwardedPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		publicURL string
		want      []string
	}{
		{name: "no public url", publicURL: "", want: []string{"/health"}},
		{name: "an origin with no path", publicURL: "https://mcp.example.com", want: []string{"/health"}},
		{name: "a trailing slash is not a prefix", publicURL: "https://mcp.example.com/", want: []string{"/health"}},
		{name: "the mcp endpoint is not a prefix", publicURL: "https://mcp.example.com/mcp", want: []string{"/health"}},
		{name: "unparseable", publicURL: "://nonsense", want: []string{"/health"}},
		{
			name:      "a path prefix is mounted alongside the bare path",
			publicURL: "https://mcp.example.com/gitlab",
			want:      []string{"/health", "/gitlab/health"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := publicPaths(&config.Config{PublicURL: tt.publicURL}, "/health")

			if !slices.Equal(got, tt.want) {
				t.Errorf("publicPaths = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMCPEndpointPatterns_CoverTheRootTheEndpointAndThePrefix pins the routes
// the MCP handler is mounted on.
//
// Routing happens before authentication, so this list is also the list of paths
// that can answer anything at all: everything else is an unauthenticated 404,
// which is what stopped this server from telling every scanner that
// /.well-known/oauth-authorization-server was a protected document that is not
// there.
func TestMCPEndpointPatterns_CoverTheRootTheEndpointAndThePrefix(t *testing.T) {
	t.Parallel()

	bare := mcpEndpointPatterns(&config.Config{})
	if len(bare) == 0 {
		t.Fatal("no MCP endpoint patterns; the server would answer nothing")
	}
	for _, want := range []string{"/{$}", "/mcp"} {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			if !slices.Contains(bare, want) {
				t.Errorf("patterns %v are missing %q", bare, want)
			}
		})
	}

	prefixed := mcpEndpointPatterns(&config.Config{PublicURL: "https://mcp.example.com/gitlab"})
	for _, want := range []string{"/gitlab", "/gitlab/{$}", "/gitlab/mcp", "/gitlab/mcp/{$}"} {
		t.Run(want, func(t *testing.T) {
			t.Parallel()

			if !slices.Contains(prefixed, want) {
				t.Errorf("patterns %v are missing the forwarded-prefix form %q", prefixed, want)
			}
		})
	}
}

// TestServerCardMediaType_FollowsThePathTheCardWasFetchedFrom covers the one
// difference between the two mounted card paths.
//
// The .well-known location predates the extension's own recommendation and is
// kept because scanners written against the earlier draft already fetch it;
// they expect application/json there, while the current location serves the
// card's own media type. Matching on the suffix is what keeps a deployment that
// mounts the card under a forwarded prefix classified the same way.
func TestServerCardMediaType_FollowsThePathTheCardWasFetchedFrom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "the current location", path: serverCardPath, want: mimeServerCard},
		{name: "the legacy well-known location", path: serverCardLegacyPath, want: mimeJSON},
		{name: "the legacy location behind a forwarded prefix", path: "/gitlab" + serverCardLegacyPath, want: mimeJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := serverCardMediaType(tt.path); got != tt.want {
				t.Errorf("serverCardMediaType(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestSessionTagOf_ReadsTheTagAServerMinted covers the parse behind session
// ownership.
//
// A session ID this deployment never minted carries no tag, and saying so is
// what makes the gate refuse it: under the stateless transport no session IDs
// are issued at all, so anything untagged is either stale or forged. Reading a
// missing tag as an empty one would compare equal to an entry that had none and
// hand someone else's session over.
func TestSessionTagOf_ReadsTheTagAServerMinted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		wantTag string
		wantOK  bool
	}{
		{name: "a minted id", id: "tag123" + sessionTagSeparator + "abcdef", wantTag: "tag123", wantOK: true},
		{name: "no separator at all", id: "abcdef"},
		{name: "an empty tag", id: sessionTagSeparator + "abcdef"},
		{name: "nothing", id: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tag, ok := sessionTagOf(tt.id)

			if ok != tt.wantOK {
				t.Errorf("sessionTagOf(%q) ok = %v, want %v", tt.id, ok, tt.wantOK)
			}
			if tag != tt.wantTag {
				t.Errorf("sessionTagOf(%q) tag = %q, want %q", tt.id, tag, tt.wantTag)
			}
		})
	}
}

// TestOAuthCacheTTL_ANonPositiveValueFallsBackAndSaysSo covers the guard in
// front of the identity cache's lifetime.
//
// A zero TTL would mean every request re-verifies upstream, which is a load
// multiplier on the GitLab instance rather than a security setting; the flag
// validation rejects such a value, and this is the second line of defense for a
// Config assembled directly.
func TestOAuthCacheTTL_ANonPositiveValueFallsBackAndSaysSo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured time.Duration
		want       time.Duration
	}{
		{name: "a configured ttl is kept", configured: 5 * time.Minute, want: 5 * time.Minute},
		{name: "zero falls back", configured: 0, want: config.DefaultOAuthCacheTTL},
		{name: "negative falls back", configured: -time.Minute, want: config.DefaultOAuthCacheTTL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := oauthCacheTTL(tt.configured); got != tt.want {
				t.Errorf("oauthCacheTTL(%s) = %s, want %s", tt.configured, got, tt.want)
			}
		})
	}
}

// TestUploadMaxFileSize_ClampsAndFallsBackWithoutRefusingToStart covers the
// upload limit HTTP mode reads from the environment.
//
// Neither branch stops the server: this runs while the HTTP configuration is
// being assembled, where a malformed or oversized value is worth a loud line and
// a working server rather than a dead one. The ceiling is applied here because
// the HTTP path never calls config.Validate, so without it "1025GB" would parse
// cleanly and start a deployment above the documented maximum with nothing
// saying so.
func TestUploadMaxFileSize_ClampsAndFallsBackWithoutRefusingToStart(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int64
	}{
		{name: "unset uses the default", value: "", want: config.DefaultMaxFileSize},
		{name: "unparseable uses the default", value: "the-size-of-a-cat", want: config.DefaultMaxFileSize},
		{name: "above the ceiling is clamped", value: "1025GB", want: config.MaxFileSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value == "" {
				t.Setenv("UPLOAD_MAX_FILE_SIZE", "")
				os.Unsetenv("UPLOAD_MAX_FILE_SIZE")
			} else {
				t.Setenv("UPLOAD_MAX_FILE_SIZE", tt.value)
			}

			if got := uploadMaxFileSize(); got != tt.want {
				t.Errorf("uploadMaxFileSize() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestMCPOriginMiddleware_DecidesInTheDocumentedOrder covers every arm of the
// origin decision the MCP endpoint makes before authentication.
//
// The order is the substance. An operator-named origin is allowed even when the
// browser calls it cross-site, because being deliberately cross-origin is what
// --trusted-origins is for. Fetch metadata then outranks the Origin comparison,
// exactly as the standard library's own protection treats it: a desktop client
// sending an app:// Origin with Sec-Fetch-Site: none is the case that matters,
// since comparing that Origin against the listen host would refuse it forever.
// Only with no metadata at all does the host comparison decide.
func TestMCPOriginMiddleware_DecidesInTheDocumentedOrder(t *testing.T) {
	t.Parallel()

	trusted := &config.Config{TrustedOrigins: []string{"https://app.example.com"}}
	wildcard := &config.Config{TrustedOrigins: []string{"*"}}

	tests := []struct {
		name      string
		cfg       *config.Config
		origin    string
		fetchSite string
		preflight bool
		wantPass  bool
	}{
		{name: "no origin at all", cfg: trusted, wantPass: true},
		{name: "the wildcard trusts everything", cfg: wildcard, origin: "https://evil.example.com", fetchSite: "cross-site", wantPass: true},
		{name: "a preflight is answered rather than judged", cfg: trusted, origin: "https://evil.example.com", preflight: true, wantPass: true},
		{name: "a named origin passes even cross-site", cfg: trusted, origin: "https://app.example.com", fetchSite: "cross-site", wantPass: true},
		{name: "the browser says same-origin", cfg: trusted, origin: "https://mcp.example.com", fetchSite: "same-origin", wantPass: true},
		{name: "a desktop client with no initiating site", cfg: trusted, origin: "app://obsidian.md", fetchSite: "none", wantPass: true},
		{name: "the browser says cross-site", cfg: trusted, origin: "https://evil.example.com", fetchSite: "cross-site"},
		{name: "no metadata and a matching host", cfg: trusted, origin: "https://mcp.example.com", wantPass: true},
		{name: "no metadata and another host", cfg: trusted, origin: "https://evil.example.com"},
		{name: "no metadata and an unparseable origin", cfg: trusted, origin: "://nonsense"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var reached bool
			handler := mcpOriginMiddleware(tt.cfg, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reached = true
				w.WriteHeader(http.StatusOK)
			}))

			method := http.MethodPost
			if tt.preflight {
				method = http.MethodOptions
			}
			req := httptest.NewRequestWithContext(t.Context(), method, "/mcp", http.NoBody)
			req.Host = "mcp.example.com"
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.fetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tt.fetchSite)
			}
			if tt.preflight {
				req.Header.Set("Access-Control-Request-Method", http.MethodPost)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if reached != tt.wantPass {
				t.Errorf("handler reached = %v, want %v (status %d)", reached, tt.wantPass, rec.Code)
			}
			if !tt.wantPass && rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want %d for a refused origin", rec.Code, http.StatusForbidden)
			}
		})
	}
}

// TestServerCardAuthentication_OAuthModeNamesTheScopeAndTheMetadata covers what
// the public card says about getting in.
//
// The scope is the least privilege the deployment can work with rather than a
// constant, so a read-only deployment does not ask a client to authorize a write
// scope it will never use. The metadata link is named as the RFC 9728 field a
// client already knows, so a consumer that understands protected-resource
// metadata can follow it without learning a card-specific convention.
func TestServerCardAuthentication_OAuthModeNamesTheScopeAndTheMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		cfg              *config.Config
		wantRequired     bool
		wantScope        string
		wantResourceMeta bool
	}{
		{
			name:             "oauth with a public url",
			cfg:              &config.Config{AuthMode: config.AuthModeOAuth, PublicURL: "https://mcp.example.com"},
			wantRequired:     true,
			wantScope:        oauth.ScopeAPI,
			wantResourceMeta: true,
		},
		{
			name:         "oauth read-only asks for less",
			cfg:          &config.Config{AuthMode: config.AuthModeOAuth, ReadOnly: true},
			wantRequired: true,
			wantScope:    oauth.ScopeReadAPI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			info := serverCardAuthentication(tt.cfg)

			if got, _ := info["required"].(bool); got != tt.wantRequired {
				t.Errorf("required = %v, want %v", info["required"], tt.wantRequired)
			}
			scopes, _ := info["scopes"].([]string)
			if len(scopes) != 1 || scopes[0] != tt.wantScope {
				t.Errorf("scopes = %v, want [%s]", info["scopes"], tt.wantScope)
			}
			_, hasMeta := info["resourceMetadata"]
			if hasMeta != tt.wantResourceMeta {
				t.Errorf("resourceMetadata present = %v, want %v", hasMeta, tt.wantResourceMeta)
			}
		})
	}
}

// TestValidateHTTPSurfaceConfig_DefaultsThenRefusesWhatItCannotServe covers the
// two surface selectors an HTTP deployment can misspell.
//
// Each empty value takes the documented default rather than an error, because
// omitting a flag is not a mistake; each unrecognized value is refused at
// startup naming what was accepted, because the alternative is a server that
// silently serves a different surface than the operator configured.
func TestValidateHTTPSurfaceConfig_DefaultsThenRefusesWhatItCannotServe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr string
		want    func(*config.Config) bool
	}{
		{
			name: "both empty take their defaults",
			cfg:  &config.Config{},
			want: func(c *config.Config) bool {
				return c.MetaParamSchema == config.DefaultMetaParamSchema && c.CapabilitySurface == config.DefaultCapabilitySurface
			},
		},
		{
			name: "both set are kept",
			cfg:  &config.Config{MetaParamSchema: config.MetaParamSchemaFull, CapabilitySurface: config.CapabilitySurfaceMinimal},
			want: func(c *config.Config) bool {
				return c.MetaParamSchema == config.MetaParamSchemaFull && c.CapabilitySurface == config.CapabilitySurfaceMinimal
			},
		},
		{name: "an unknown param schema", cfg: &config.Config{MetaParamSchema: "verbose"}, wantErr: "meta-param-schema"},
		{name: "an unknown capability surface", cfg: &config.Config{CapabilitySurface: "some"}, wantErr: "capability-surface"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateHTTPSurfaceConfig(tt.cfg)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want it to name %s", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateHTTPSurfaceConfig: %v", err)
			}
			if !tt.want(tt.cfg) {
				t.Errorf("config = %+v after validation, want the documented defaults kept", tt.cfg)
			}
		})
	}
}

// TestValidateHTTPDurationConfig_RefusesWhatWouldNeverExpire covers the upper
// bounds on the two intervals that keep a pooled entry honest.
//
// Both exist because the failure is invisible: a revalidation interval of a
// week means a revoked token keeps working for a week, and a pool idle timeout
// past a day means a credential nobody has used since yesterday still holds a
// server and its GitLab client.
func TestValidateHTTPDurationConfig_RefusesWhatWouldNeverExpire(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr string
	}{
		{name: "inside every bound", cfg: &config.Config{SessionTimeout: time.Minute, RevalidateInterval: time.Minute, PoolIdleTimeout: time.Hour}},
		{name: "revalidation past its maximum", cfg: &config.Config{SessionTimeout: time.Minute, RevalidateInterval: config.MaxRevalidateInterval + time.Hour}, wantErr: "revalidate-interval"},
		{name: "pool idle past its maximum", cfg: &config.Config{SessionTimeout: time.Minute, PoolIdleTimeout: config.MaxPoolIdleTimeout + time.Hour}, wantErr: "pool-idle-timeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateHTTPDurationConfig(tt.cfg)

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("validateHTTPDurationConfig = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want it to name %s", err, tt.wantErr)
			}
		})
	}
}

// TestDoToolSearch_AnEmptyQuery_IsNotAnError covers the exit taken before a
// catalog is ever built.
//
// --tool-search with nothing in it has nothing to report, and refusing would be
// a failure the caller cannot act on. It also skips building a catalog of a
// thousand tools to search for nothing.
func TestDoToolSearch_AnEmptyQuery_IsNotAnError(t *testing.T) {
	t.Parallel()

	if err := doToolSearch("   ", config.ToolSurfaceDynamic, edition.Free); err != nil {
		t.Errorf("doToolSearch with an empty query = %v, want nil", err)
	}
}

// TestRemovedActionKeys_ReportsWhatAFilterTookAway covers the diff behind the
// message a caller gets when an action was withheld.
//
// Naming the cause is the whole point: a model told only that an action is
// unknown, alongside suggestions that are all real read-only actions, concludes
// the server lacks the capability rather than that the credential is narrow.
// A missing catalog on either side yields nothing rather than claiming
// everything was removed.
func TestRemovedActionKeys_ReportsWhatAFilterTookAway(t *testing.T) {
	t.Parallel()

	if got := removedActionKeys(nil, actioncatalog.NewCatalog()); got != nil {
		t.Errorf("removedActionKeys(nil, empty) = %v, want nothing claimed", got)
	}
	if got := removedActionKeys(actioncatalog.NewCatalog(), nil); got != nil {
		t.Errorf("removedActionKeys(empty, nil) = %v, want nothing claimed", got)
	}
}

// TestRemoveExcludedTools_WithNothingToExclude_TouchesNothing covers the early
// exit that keeps startup free of an in-memory MCP round trip nobody asked for.
//
// The filter works by listing the registered tools through an ephemeral
// session, which is real work on a surface with a thousand tools; a deployment
// that excludes nothing must not pay for it.
func TestRemoveExcludedTools_WithNothingToExclude_TouchesNothing(t *testing.T) {
	t.Parallel()

	if got := removeExcludedTools(t.Context(), nil, nil); got != 0 {
		t.Errorf("removeExcludedTools with no patterns = %d, want 0 (and no server call)", got)
	}
}

// TestResolveToolSurfaceForTelemetry_ReadsTheInputsEachModeReallyUses covers
// how telemetry learns which surface this process will serve.
//
// It has to be answered before anything is registered, because the auto
// tool-name policy sizes the metric label space by it, and the two modes take
// their surface from different places: HTTP from its flags with the environment
// filling in behind them, stdio from the environment alone. Reading the flags
// alone made TOOL_SURFACE exported into an HTTP deployment's environment
// invisible here while the overlay honored it, so telemetry sized itself for
// the wrong surface. An unparseable value falls back to the default rather than
// failing: the mode's own startup reports it properly a moment later.
func TestResolveToolSurfaceForTelemetry_ReadsTheInputsEachModeReallyUses(t *testing.T) {
	tests := []struct {
		name       string
		envSurface string
		envMeta    string
		hcfg       *httpConfig
		want       string
	}{
		{name: "stdio with nothing set", want: config.ToolSurfaceDynamic},
		{name: "stdio reads the environment", envSurface: config.ToolSurfaceIndividual, want: config.ToolSurfaceIndividual},
		{name: "stdio honors the deprecated selector", envMeta: "true", want: config.ToolSurfaceMeta},
		{
			name: "http takes the flag",
			hcfg: &httpConfig{toolSurface: config.ToolSurfaceMeta},
			want: config.ToolSurfaceMeta,
		},
		{
			name:       "http lets the environment fill in behind the flag",
			envSurface: config.ToolSurfaceIndividual,
			hcfg:       &httpConfig{},
			want:       config.ToolSurfaceIndividual,
		},
		{
			name: "http honors an explicitly passed --meta-tools",
			hcfg: &httpConfig{metaTools: true, metaToolsSet: true},
			want: config.ToolSurfaceMeta,
		},
		{
			name:       "an unusable value falls back to the default",
			envSurface: "telepathy",
			want:       config.ToolSurfaceDynamic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TOOL_SURFACE", tt.envSurface)
			t.Setenv("META_TOOLS", tt.envMeta)

			if got := resolveToolSurfaceForTelemetry(tt.hcfg); got != tt.want {
				t.Errorf("resolveToolSurfaceForTelemetry = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCreateServer_WithoutAClient_IsRefused covers the one argument createServer
// cannot work without.
//
// Every registration path dereferences the client, so accepting nil would move
// a nil dereference from this line into whichever tool a model happened to call
// first — a panic on the request path rather than an error at startup.
func TestCreateServer_WithoutAClient_IsRefused(t *testing.T) {
	t.Parallel()

	server, err := createServer(t.Context(), nil, &config.ServerConfig{})

	if err == nil {
		t.Fatal("createServer accepted a nil client")
	}
	if server != nil {
		t.Errorf("server = %v, want nothing built", server)
	}
	if !strings.Contains(err.Error(), "client") {
		t.Errorf("error = %q, want it to name the missing client", err)
	}
}

// TestValidateHTTPRuntimeConfig_RefusesEachUnusableSetting covers the startup
// checks an HTTP deployment gets before anything is bound.
//
// Every one of these is a setting whose failure is otherwise invisible or late:
// a TLS pair that does not load fails on the first handshake nobody watches, a
// malformed trusted origin would be silently dropped and leave a browser client
// refused forever, and a metadata link that is not a URL lands on somebody's
// consent screen. Each is refused here, naming the flag the operator typed.
func TestValidateHTTPRuntimeConfig_RefusesEachUnusableSetting(t *testing.T) {
	t.Parallel()

	base := func() *config.Config {
		return &config.Config{
			SessionTimeout:    time.Minute,
			MetaParamSchema:   config.DefaultMetaParamSchema,
			CapabilitySurface: config.DefaultCapabilitySurface,
			RateLimitRPS:      1,
			RateLimitBurst:    1,
		}
	}

	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr string
	}{
		{name: "a usable configuration", mutate: func(*config.Config) {}},
		{
			name:    "an unknown capability surface",
			mutate:  func(c *config.Config) { c.CapabilitySurface = "some" },
			wantErr: "capability-surface",
		},
		{
			name:    "a revalidation interval past its maximum",
			mutate:  func(c *config.Config) { c.RevalidateInterval = config.MaxRevalidateInterval + time.Hour },
			wantErr: "revalidate-interval",
		},
		{
			name:    "a TLS certificate without its key",
			mutate:  func(c *config.Config) { c.TLSCertFile = "/etc/ssl/mcp.crt" },
			wantErr: "tls-key",
		},
		{
			name:    "a burst that cannot serve the configured rate",
			mutate:  func(c *config.Config) { c.RateLimitRPS, c.RateLimitBurst = 10, 0 },
			wantErr: "rate-limit",
		},
		{
			name:    "a trusted origin that is not an origin",
			mutate:  func(c *config.Config) { c.TrustedOrigins = []string{"*", "https://app.example.com", "not-an-origin"} },
			wantErr: "trusted-origins",
		},
		{
			name:    "a documentation link that is not a URL",
			mutate:  func(c *config.Config) { c.ResourceDocumentation = "see the wiki" },
			wantErr: "resource-documentation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := base()
			tt.mutate(cfg)

			err := validateHTTPRuntimeConfig(cfg)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateHTTPRuntimeConfig = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateHTTPRuntimeConfig accepted %+v", cfg)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to name %s", err, tt.wantErr)
			}
		})
	}
}

// TestValidateOAuthCacheTTL_AcceptsWhatIsInsideTheDocumentedRange covers the
// bounds on how long an identity may be answered from memory.
//
// Too short and every request re-verifies upstream, which is a load multiplier
// on the GitLab instance; too long and a revoked token keeps working. The
// range is what the documentation promises, so both ends are checked rather
// than clamped silently.
func TestValidateOAuthCacheTTL_AcceptsWhatIsInsideTheDocumentedRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ttl     time.Duration
		wantErr bool
	}{
		{name: "the default", ttl: config.DefaultOAuthCacheTTL},
		{name: "the minimum", ttl: config.MinOAuthCacheTTL},
		{name: "the maximum", ttl: config.MaxOAuthCacheTTL},
		{name: "below the minimum", ttl: config.MinOAuthCacheTTL - time.Second, wantErr: true},
		{name: "above the maximum", ttl: config.MaxOAuthCacheTTL + time.Second, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateOAuthCacheTTL(tt.ttl)

			if (err != nil) != tt.wantErr {
				t.Errorf("validateOAuthCacheTTL(%s) = %v, want error=%v", tt.ttl, err, tt.wantErr)
			}
		})
	}
}

// TestRunHTTP_RefusesABadConfigurationBeforeBinding covers the settings HTTP
// mode rejects on the way in, each from a different layer of the same startup.
//
// They are worth one test together because the property under test is shared:
// none of them may reach the point where a listener exists. A server that binds
// and then discovers its socket mode is nonsense has already told the operator
// it started.
func TestRunHTTP_RefusesABadConfigurationBeforeBinding(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		hcfg    *httpConfig
		wantErr string
	}{
		{
			name:    "an environment value that does not parse",
			env:     map[string]string{"CAPABILITY_SURFACE": "some"},
			hcfg:    &httpConfig{gitlabURL: "https://gitlab.example.com"},
			wantErr: "loading environment configuration",
		},
		{
			name:    "a socket mode that is not a mode",
			hcfg:    &httpConfig{gitlabURL: "https://gitlab.example.com", socketMode: "999"},
			wantErr: "http-socket-mode",
		},
		{
			name:    "a tool surface this server does not serve",
			hcfg:    &httpConfig{gitlabURL: "https://gitlab.example.com", toolSurface: "telepathy"},
			wantErr: "tool surface",
		},
		{
			name:    "a tier this server does not know",
			hcfg:    &httpConfig{gitlabURL: "https://gitlab.example.com", tier: "platinum", tierSet: true},
			wantErr: "tier",
		},
		{
			name:    "an instance URL carrying credentials",
			hcfg:    &httpConfig{gitlabURL: "https://user:pass@gitlab.example.com"},
			wantErr: "gitlab-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			hcfg := tt.hcfg
			hcfg.maxHTTPClients = config.DefaultMaxHTTPClients
			hcfg.sessionTimeout = config.DefaultSessionTimeout

			err := runHTTP(t.Context(), hcfg)

			if err == nil {
				t.Fatal("runHTTP started with a configuration it must refuse")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to name %s", err, tt.wantErr)
			}
		})
	}
}

// TestRunWithContext_DeclaresWhichGitLabHostsAMetricMayName covers the
// per-mode decision about the http.client metric's server.address dimension.
//
// The modes disagree about who chooses the instance, and the dimension has to
// follow: a pinned deployment names its own hosts, while a deployment that lets
// every caller choose must declare none, because a caller-supplied hostname on a
// metric is a caller minting time series. The assertion is indirect — the
// declaration happens before the mode's own validation refuses the rest of the
// configuration — so what is pinned here is that the refusal still comes from
// the mode and not from the declaration.
func TestRunWithContext_DeclaresWhichGitLabHostsAMetricMayName(t *testing.T) {
	tests := []struct {
		name string
		env  string
		hcfg *httpConfig
	}{
		{
			name: "an http deployment pinned by flag",
			hcfg: &httpConfig{gitlabURLs: repeatedFlag{"https://gitlab.example.com"}, authMode: "saml"},
		},
		{
			name: "an http deployment pinned by the environment",
			env:  "https://gitlab.example.com",
			hcfg: &httpConfig{authMode: "saml"},
		},
		{
			// Free selection now has to be asked for: without
			// --allow-any-gitlab-url the deployment is refused before it
			// reaches the mode's own validation, which is a different
			// refusal from the one this test is about.
			name: "an http deployment that lets callers choose",
			hcfg: &httpConfig{authMode: "saml", allowAnyGitLabURL: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITLAB_URL", tt.env)
			tt.hcfg.maxHTTPClients = config.DefaultMaxHTTPClients
			tt.hcfg.sessionTimeout = config.DefaultSessionTimeout

			err := runWithContext(t.Context(), tt.hcfg)

			if err == nil || !strings.Contains(err.Error(), "auth-mode") {
				t.Errorf("error = %v, want the mode's own refusal of --auth-mode", err)
			}
		})
	}
}

// TestMain_VersionAndToolSearch_ExitBeforeAnythingIsStarted covers the argument
// forms that answer and return without becoming a server.
//
// Each of them runs before the configuration is read, which is the point: an
// operator asking what version this is, or which tools match a word, must get
// an answer on a machine that has no GITLAB_TOKEN and no reachable instance.
// The two failure paths exit non-zero through the seam rather than returning,
// so a script can tell "no such surface" from "no matches".
func TestMain_VersionAndToolSearch_ExitBeforeAnythingIsStarted(t *testing.T) {
	tests := []struct {
		name string
		args []string
		// wantStdout is a fragment the answer must carry, for the forms that
		// print one themselves.
		wantStdout string
		// wantSearch is the query the search runner must receive, or "" when
		// no search may run.
		wantSearch string
		wantExit   []int
	}{
		{
			name:       "--version prints and returns",
			args:       []string{"gitlab-mcp-server", "-version"},
			wantStdout: version,
		},
		{
			name:       "--tool-search runs the search",
			args:       []string{"gitlab-mcp-server", "-tool-search", "issue list"},
			wantSearch: "issue list",
		},
		{
			name:     "a surface the search cannot use exits non-zero",
			args:     []string{"gitlab-mcp-server", "-tool-search", "issue", "-tool-surface", "telepathy"},
			wantExit: []int{1},
		},
		{
			name:     "a tier the search cannot use exits non-zero",
			args:     []string{"gitlab-mcp-server", "-tool-search", "issue", "-tier", "platinum"},
			wantExit: []int{1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFreshFlagSet(t)

			var searched string
			originalRunner, originalExit := toolSearchRunner, exitProcess
			t.Cleanup(func() { toolSearchRunner, exitProcess = originalRunner, originalExit })
			toolSearchRunner = func(query, _ string, _ edition.Tier) error {
				searched = query
				return nil
			}
			var exits []int
			exitProcess = func(code int) { exits = append(exits, code) }

			originalArgs := os.Args
			os.Args = tt.args
			t.Cleanup(func() { os.Args = originalArgs })
			stdout := captureStdout(t)

			main()

			printed := stdout()
			if tt.wantStdout != "" && !strings.Contains(printed, tt.wantStdout) {
				t.Errorf("stdout = %q, want it to carry %q", printed, tt.wantStdout)
			}
			if searched != tt.wantSearch {
				t.Errorf("search query = %q, want %q", searched, tt.wantSearch)
			}
			if !slices.Equal(exits, tt.wantExit) {
				t.Errorf("exit codes = %v, want %v", exits, tt.wantExit)
			}
		})
	}
}

// TestMain_StdioMode_StartsAndStopsWithTheClient covers the ordinary path all
// the way through main: no flags, credentials from the environment, and a
// client that closes its pipe.
//
// It is the only test that runs the startup summary — the logger, the server
// info the health tool reports, and the transport selection — in the order main
// really runs them. The client's departure is the documented way this process
// ends, and it has to end by returning rather than by a non-zero exit, because
// systemd's Restart=on-failure acts on the difference.
func TestMain_StdioMode_StartsAndStopsWithTheClient(t *testing.T) {
	withFreshFlagSet(t)
	gitlab := newMockGitLabServerWithUser(t)
	t.Setenv("GITLAB_URL", gitlab.URL)
	t.Setenv("GITLAB_TOKEN", testToken)
	t.Setenv("TOOL_SURFACE", config.ToolSurfaceDynamic)
	t.Setenv("LOG_LEVEL", "error")

	originalArgs := os.Args
	os.Args = []string{"gitlab-mcp-server"}
	t.Cleanup(func() { os.Args = originalArgs })

	// A closed pipe as stdin: the SDK reads EOF immediately, which is exactly
	// what a client exiting does, and it is also what keeps this test from
	// reading the test runner's own stdin.
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	_ = writer.Close()
	originalStdin, originalLogger := os.Stdin, slog.Default()
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = originalStdin
		slog.SetDefault(originalLogger)
		_ = reader.Close()
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		main()
	}()

	select {
	case <-done:
	case <-time.After(testHTTPLivenessTimeout):
		t.Fatal("main did not return after the client closed its end of the pipe")
	}
}

// captureStdout redirects os.Stdout for the duration of the test and returns a
// function that stops the capture and yields what was written.
func captureStdout(t *testing.T) func() string {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = writer

	var once sync.Once
	var captured string
	stop := func() string {
		once.Do(func() {
			os.Stdout = original
			_ = writer.Close()
			out, readErr := io.ReadAll(reader)
			if readErr != nil {
				t.Errorf("reading captured stdout: %v", readErr)
			}
			captured = string(out)
			_ = reader.Close()
		})
		return captured
	}
	t.Cleanup(func() { stop() })
	return stop
}

// TestBuildServerCard_AnUnusableInstanceURL_IsReportedNotServed covers the one
// failure the card builder can hit before it inspects anything.
//
// The card is a public document, so a deployment whose configured instance URL
// does not parse must answer 503 rather than serve a card built against a
// client that does not exist.
func TestBuildServerCard_AnUnusableInstanceURL_IsReportedNotServed(t *testing.T) {
	t.Parallel()

	card, err := buildServerCard(t.Context(), &config.Config{GitLabURL: "://not-a-url"})

	if err == nil {
		t.Fatalf("buildServerCard returned a card for an unusable instance URL: %s", card)
	}
	if card != nil {
		t.Errorf("card = %s, want nothing published", card)
	}
}

// TestRemoveExcludedTools_RemovesOnlyTheNamedTools covers the filter an
// operator uses to take a tool off the surface.
//
// The count is what the startup line reports, and the tools left behind are the
// deployment's actual surface, so an over-broad match would silently remove
// capability an operator still expects to have.
func TestRemoveExcludedTools_RemovesOnlyTheNamedTools(t *testing.T) {
	t.Parallel()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	for _, name := range []string{"gitlab_issue", "gitlab_runner", "gitlab_project"} {
		mcp.AddTool(server, &mcp.Tool{Name: name, Description: name},
			func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{}, nil, nil
			})
	}

	removed := removeExcludedTools(t.Context(), server, []string{"gitlab_runner", "gitlab_absent"})

	if removed != 1 {
		t.Errorf("removeExcludedTools = %d, want the one tool that was actually registered", removed)
	}
	tools, err := listRegisteredTools(server, "test")
	if err != nil {
		t.Fatalf("listRegisteredTools: %v", err)
	}
	for _, tool := range tools {
		if tool.Name == "gitlab_runner" {
			t.Error("the excluded tool is still registered")
		}
	}
	if len(tools) != 2 {
		t.Errorf("registered tools = %d, want the other two left alone", len(tools))
	}
}

// TestCatalogBackedToolNames_WithoutACatalog_ExemptsNothing covers the safe-mode
// exemption set when there is no catalog to read.
//
// Exempting a tool means safe mode does not wrap it, on the grounds that the
// catalog already previews each of its actions. With no catalog that reasoning
// does not hold, so nothing may be exempt: the alternative is a mutating tool
// executing for real in a deployment that asked for previews.
func TestCatalogBackedToolNames_WithoutACatalog_ExemptsNothing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		surface string
		catalog *actioncatalog.Catalog
	}{
		{name: "the individual surface exempts nothing by design", surface: config.ToolSurfaceIndividual, catalog: actioncatalog.NewCatalog()},
		{name: "no catalog to read", surface: config.ToolSurfaceMeta, catalog: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := catalogBackedToolNames(tt.catalog, tt.surface); len(got) != 0 {
				t.Errorf("catalogBackedToolNames = %v, want nothing exempt from safe mode", got)
			}
		})
	}
}

// TestDeprecationWarnings_SayWhatToWriteInstead covers the three lines an
// operator gets for configuration this server still honors but no longer
// documents.
//
// Each has to name the replacement rather than just the deprecation, because
// the reader's next action is editing a unit file. Nothing is logged for a
// configuration that is already current, so the lines only appear when they
// apply to something.
func TestDeprecationWarnings_SayWhatToWriteInstead(t *testing.T) {
	tests := []struct {
		name     string
		log      func()
		want     []string
		wantNone bool
	}{
		{
			name: "the deprecated boolean tool selector",
			log:  func() { logLegacyMetaToolsDeprecation("", "true") },
			want: []string{"TOOL_SURFACE"},
		},
		{
			name:     "an unusable value for it says nothing here",
			log:      func() { logLegacyMetaToolsDeprecation("", "sometimes") },
			wantNone: true,
		},
		{
			name:     "a current configuration says nothing",
			log:      func() { logLegacyMetaToolsDeprecation(config.ToolSurfaceMeta, "") },
			wantNone: true,
		},
		{
			name: "the deprecated enterprise switch",
			log:  func() { logLegacyEnterpriseEnvDeprecation("", "true") },
			want: []string{"GITLAB_ENTERPRISE", "GITLAB_TIER"},
		},
		{
			name:     "the tier variable alone says nothing",
			log:      func() { logLegacyEnterpriseEnvDeprecation("ultimate", "") },
			wantNone: true,
		},
		{
			name: "request options this deployment ignores",
			log: func() {
				logIgnoredRequestOptions("glpat-0123456789", serverpool.RequestOptions{
					IgnoredOptions:    []string{"META_TOOLS"},
					DeprecatedOptions: []string{"META_TOOLS"},
				})
			},
			want: []string{"META_TOOLS", "TOOL_SURFACE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(previous) })

			tt.log()

			logged := buf.String()
			if tt.wantNone {
				if logged != "" {
					t.Errorf("logged %q for a configuration that needs no warning", logged)
				}
				return
			}
			for _, want := range tt.want {
				if !strings.Contains(logged, want) {
					t.Errorf("log %q is missing %q", logged, want)
				}
			}
		})
	}
}

// TestSSEAwareWriter_WriteKeepAlive_StopsWhenThereIsNothingToKeepAlive covers
// the three answers the heartbeat gives without emitting a frame.
//
// The keep-alive exists because the hops in between close an idle response —
// nginx at 60 seconds by default, mobile carrier NATs sooner — so a stream that
// has just written is not idle and must not be padded, a stream whose handler
// has returned must never be written to again, and a stream whose connection is
// gone must stop rather than write into a closed socket every 25 seconds.
func TestSSEAwareWriter_WriteKeepAlive_StopsWhenThereIsNothingToKeepAlive(t *testing.T) {
	t.Parallel()

	t.Run("a stream that has just written is not idle", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		writer := &sseAwareWriter{ResponseWriter: rec, lastWrite: time.Now()}

		if !writer.writeKeepAlive() {
			t.Error("the heartbeat stopped on a stream that is alive")
		}
		if rec.Body.Len() != 0 {
			t.Errorf("body = %q, want no frame written to a stream that just wrote", rec.Body.String())
		}
	})

	t.Run("a finished handler is never written to", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		writer := &sseAwareWriter{ResponseWriter: rec, stopped: true}

		if writer.writeKeepAlive() {
			t.Error("the heartbeat continued after the handler returned")
		}
		if rec.Body.Len() != 0 {
			t.Errorf("body = %q, want nothing written after the handler returned", rec.Body.String())
		}
	})

	t.Run("a broken connection stops the heartbeat", func(t *testing.T) {
		t.Parallel()

		writer := &sseAwareWriter{ResponseWriter: brokenResponseWriter{httptest.NewRecorder()}}

		if writer.writeKeepAlive() {
			t.Error("the heartbeat continued after the write failed; it would write into a closed socket forever")
		}
	})
}

// brokenResponseWriter is a ResponseWriter whose connection has gone away.
type brokenResponseWriter struct {
	*httptest.ResponseRecorder
}

func (brokenResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("connection reset by peer")
}

// TestSSEAwareWriter_ABrokenStream_EndsItsHeartbeat covers the heartbeat
// goroutine's own exit, as opposed to the decision it takes each tick.
//
// A stream whose connection has gone away must stop rather than write into a
// closed socket every twenty-five seconds for the lifetime of the process. The
// goroutine is joined here rather than merely given time, so the exit is
// observed on every run instead of raced: without the join this path is covered
// on some runs and not others, which is how a regression in it would ship.
func TestSSEAwareWriter_ABrokenStream_EndsItsHeartbeat(t *testing.T) {
	restore := sseKeepAliveInterval
	sseKeepAliveInterval = time.Millisecond
	t.Cleanup(func() { sseKeepAliveInterval = restore })

	rec := httptest.NewRecorder()
	rec.Header().Set(hdrContentType, "text/event-stream")
	writer := &sseAwareWriter{ResponseWriter: brokenResponseWriter{rec}}

	writer.WriteHeader(http.StatusOK)
	if writer.done == nil {
		t.Fatal("no heartbeat was started for a response that committed to text/event-stream")
	}

	select {
	case <-writer.done:
	case <-time.After(10 * time.Second):
		t.Fatal("the heartbeat is still running on a stream whose writes fail")
	}

	// And stopping it afterwards is still safe: the handler always defers this,
	// including for a stream that died on its own.
	writer.stopKeepAlive()
}

// TestServeStdio_TheTwoDocumentedShutdowns_ExitCleanly pins the exit status of
// the two ways an stdio session is meant to end.
//
// Neither is a failure, and saying otherwise has consequences beyond a log
// line: a nonzero status is what systemd's Restart=on-failure acts on, what
// fails a CI wrapper, and what a launcher passes outward as its own. Both used
// to arrive here as errors — a client closing its pipe returns cleanly only
// when nothing is in flight, and a signal returns ctx.Err() whether or not
// anything was open — so the only stop that exited zero was EOF on a completely
// idle server, and the everyday case did not.
//
// The two cases are distinguished by what stdin does, which is also what makes
// them deterministic: a closed pipe is EOF before anything else can happen, and
// a pipe held open cannot produce one, so the cancelled context is the only
// thing left to end the run.
func TestServeStdio_TheTwoDocumentedShutdowns_ExitCleanly(t *testing.T) {
	tests := []struct {
		name string
		// holdOpen keeps the write end of the pipe, so the reader never sees
		// EOF and the context is what ends the session.
		holdOpen bool
		// cancelFirst cancels the context before serving, standing in for a
		// signal that arrives immediately.
		cancelFirst bool
	}{
		{name: "the client closes its pipe", cancelFirst: false},
		{name: "a signal cancels the context", holdOpen: true, cancelFirst: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatalf("os.Pipe: %v", err)
			}
			if !tt.holdOpen {
				_ = writer.Close()
			} else {
				t.Cleanup(func() { _ = writer.Close() })
			}
			original := os.Stdin
			os.Stdin = reader
			t.Cleanup(func() {
				os.Stdin = original
				_ = reader.Close()
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancelFirst {
				cancel()
			}

			done := make(chan error, 1)
			go func() { done <- serveStdio(ctx, newTestMCPServer(t)) }()

			select {
			case serveErr := <-done:
				if serveErr != nil {
					t.Errorf("serveStdio() = %v, want a clean exit: this is a documented shutdown, not a failure", serveErr)
				}
			case <-time.After(testHTTPLivenessTimeout):
				t.Fatal("serveStdio did not return")
			}
		})
	}
}

// endlessBytes yields a single repeated byte forever, so a body of any size
// can be posted without allocating it.
type endlessBytes struct{ b byte }

func (e endlessBytes) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = e.b
	}
	return len(p), nil
}

// readBodyHandler drains the request body and records what the read reported,
// which is where a body cap and a depth cap both surface.
func readBodyHandler(t *testing.T, observed *error) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.Copy(io.Discard, r.Body)
		*observed = err
		w.WriteHeader(http.StatusOK)
	})
}

// TestSecurityHeaders_BodyCapFollowsConfiguration verifies that the outermost
// middleware enforces the configured --max-request-body-bytes rather than a
// constant of its own.
//
// The middleware used to wrap every body in a hardcoded 10 MiB reader, and the
// SDK's configured limiter then wrapped the already-wrapped body, so above
// 10 MiB the constant always bound: a deployment raising the cap for the
// documented inline-upload workflow was silently held to about 7.5 MiB of
// binary, and the 413 it got back named a number no operator had typed. The
// deviation failed closed, which is why it is a correctness defect rather than
// a hole, but a limit that ignores its configuration is not a limit anyone can
// reason about.
func TestSecurityHeaders_BodyCapFollowsConfiguration(t *testing.T) {
	t.Parallel()
	const mib = 1 << 20
	for _, tc := range []struct {
		name       string
		configured int64
		bodyBytes  int64
		wantTooBig bool
	}{
		{"unset_falls_back_to_the_sdk_default", 0, 5 * mib, true},
		{"unset_admits_what_fits_the_default", 0, 1 * mib, false},
		{"raised_above_the_old_constant", 12 * mib, 11 * mib, false},
		{"raised_still_refuses_beyond_itself", 12 * mib, 13 * mib, true},
		{"lowered_below_the_old_constant", 1 * mib, 2 * mib, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var observed error
			handler := securityHeadersMiddleware(
				inboundLimits{maxBytes: tc.configured, maxDepth: maxInboundJSONDepth},
				readBodyHandler(t, &observed),
			)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost/mcp",
				io.LimitReader(endlessBytes{b: 'a'}, tc.bodyBytes))
			handler.ServeHTTP(httptest.NewRecorder(), req)

			var tooBig *http.MaxBytesError
			if errors.As(observed, &tooBig) != tc.wantTooBig {
				t.Fatalf("body read error = %v, want too-big %v", observed, tc.wantTooBig)
			}
			if !tc.wantTooBig {
				return
			}
			want := tc.configured
			if want == 0 {
				want = mcp.DefaultMaxRequestBodyBytes
			}
			if tooBig.Limit != want {
				t.Errorf("refused at %d bytes, want the configured %d", tooBig.Limit, want)
			}
		})
	}
}

// TestSecurityHeaders_RefusesDeeplyNestedBody verifies that the front door
// refuses a body whose JSON nests past the ceiling, before any decoder sees
// it.
//
// This is the half of MP-01 a receiving middleware cannot reach. The SDK
// unmarshals params._meta into a map[string]any on EVERY method, ahead of any
// middleware, using a decoder with no depth guard that is quadratic in nesting
// depth — so a deeply nested _meta on a bare ping costs the same CPU as one on
// a tools/call, and the argument guard never gets a say. The scan here is
// streaming and linear, so it costs a pass over bytes the server was going to
// read anyway.
func TestSecurityHeaders_RefusesDeeplyNestedBody(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		depth     int
		wantRefus bool
	}{
		{"ordinary_request", 6, false},
		{"at_the_ceiling", maxInboundJSONDepth, false},
		{"one_past_the_ceiling", maxInboundJSONDepth + 1, true},
		{"the_amplification_payload", 20000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var observed error
			handler := securityHeadersMiddleware(
				inboundLimits{maxBytes: 8 << 20, maxDepth: maxInboundJSONDepth},
				readBodyHandler(t, &observed),
			)
			body := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"_meta":` +
				strings.Repeat("[", tc.depth-2) + strings.Repeat("]", tc.depth-2) + `}}`
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost/mcp",
				strings.NewReader(body))
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if tc.wantRefus {
				if observed == nil {
					t.Fatalf("body at depth %d was read whole, want a refusal", tc.depth)
				}
				if !strings.Contains(observed.Error(), "nests") {
					t.Errorf("refusal = %q, want it to name the nesting ceiling", observed)
				}
				return
			}
			if observed != nil {
				t.Fatalf("body at depth %d: %v", tc.depth, observed)
			}
		})
	}
}

// TestRequireInstanceAllowList verifies that HTTP mode refuses to start
// without an instance allow-list, and that the escape hatch is the only way
// past it.
//
// With no --gitlab-url the resolver treated the GITLAB-URL header as a free
// choice, and admission does not compensate: the credential probe runs against
// the host the CALLER named, and reports a rejection only on an explicit 401
// or 403. So an unauthenticated caller pointing the header at a host they
// control was admitted with any string as a token, and every tool then ran
// against that host — an SSRF pivot whose responses are reflected back into
// the caller's own result. That was the shipped default, container CMD
// included. Refusing to start is the same shape as --auth-mode=oauth refusing
// without --public-url: a deployment that has not said which GitLab it serves
// has not been configured.
func TestRequireInstanceAllowList(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		hcfg    httpConfig
		wantErr string
	}{
		{
			name:    "no_instance_is_refused",
			hcfg:    httpConfig{},
			wantErr: "--gitlab-url",
		},
		{
			name: "no_instance_with_the_escape_hatch_starts",
			hcfg: httpConfig{allowAnyGitLabURL: true},
		},
		{
			name: "one_instance",
			hcfg: httpConfig{gitlabURLs: repeatedFlag{"https://gitlab.example.com"}},
		},
		{
			name: "several_instances",
			hcfg: httpConfig{gitlabURLs: repeatedFlag{"https://a.example.com", "https://b.example.com"}},
		},
		{
			name: "the_escape_hatch_is_ignored_when_an_instance_is_named",
			hcfg: httpConfig{gitlabURLs: repeatedFlag{"https://a.example.com"}, allowAnyGitLabURL: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := requireInstanceAllowList(&tc.hcfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("requireInstanceAllowList: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("a deployment naming no instance started, want a refusal")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to name %s", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), "--allow-any-gitlab-url") {
				t.Errorf("error = %q, want it to name the escape hatch", err)
			}
		})
	}
}

// TestRunHTTP_RefusesWithoutAnInstance verifies the refusal at the entry point
// rather than only in the helper, so a deployment cannot reach a listener
// through some other path.
func TestRunHTTP_RefusesWithoutAnInstance(t *testing.T) {
	err := runHTTP(t.Context(), &httpConfig{
		addr:           "127.0.0.1:0",
		maxHTTPClients: config.DefaultMaxHTTPClients,
		sessionTimeout: config.DefaultSessionTimeout,
	})
	if err == nil {
		t.Fatal("runHTTP with no --gitlab-url started, want a refusal")
	}
	if !strings.Contains(err.Error(), "--gitlab-url") {
		t.Errorf("error = %q, want it to name the missing flag", err)
	}
}

// TestRunHTTP_AllowAnyGitLabURLStarts verifies that the escape hatch restores
// the old free-selection behavior for the single-user local deployment it
// exists for.
func TestRunHTTP_AllowAnyGitLabURLStarts(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := runHTTP(ctx, &httpConfig{
		// An explicit ephemeral port: with addr empty, net/http listens on
		// ":http" — real port 80 — and the test's outcome then depends on
		// whether anything on the machine already owns it.
		addr:              "127.0.0.1:0",
		allowAnyGitLabURL: true,
		maxHTTPClients:    config.DefaultMaxHTTPClients,
		sessionTimeout:    config.DefaultSessionTimeout,
	})
	if err != nil {
		t.Fatalf("runHTTP with --allow-any-gitlab-url: %v", err)
	}
}

// TestRequireExplicitInstance verifies that a request leaving the instance
// choice to a multi-instance deployment is refused instead of being answered
// with the first published one.
//
// The resolver returned normalizedAllowed[0] when the header was absent, on
// the reasoning that a client which does not care keeps working. That holds
// for a client which does not care, not for one that cares and failed to say
// so — a proxy stripped the header, a library cannot set custom ones — and the
// token is then transmitted in full to an instance its holder never named. In
// oauth mode it is worse: both instances are advertised in the RFC 9728
// authorization_servers array and neither RFC 9728 nor MCP gives a client an
// in-band way to say which one it used, so a conformant client selecting the
// second sends its bearer to the first with no user error at all.
func TestRequireExplicitInstance(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		instances []string
		header    string
		wantErr   bool
	}{
		{"free_selection_absent_header", nil, "", false},
		{"free_selection_present_header", nil, "https://gitlab.example.com", false},
		{"one_instance_absent_header", []string{"https://a.example.com"}, "", false},
		{"one_instance_present_header", []string{"https://a.example.com"}, "https://b.example.com", false},
		{"two_instances_absent_header", []string{"https://a.example.com", "https://b.example.com"}, "", true},
		{"two_instances_blank_header", []string{"https://a.example.com", "https://b.example.com"}, "   ", true},
		{"two_instances_present_header", []string{"https://a.example.com", "https://b.example.com"}, "https://b.example.com", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost/mcp", http.NoBody)
			if tc.header != "" {
				req.Header.Set(serverpool.RequestOptionGitLabURL, tc.header)
			}
			err := requireExplicitInstance(req, tc.instances)
			if tc.wantErr {
				if err == nil {
					t.Fatal("a request naming no instance was resolved, want a refusal")
				}
				if !errors.Is(err, errMissingGitLabURL) {
					t.Errorf("error = %v, want errMissingGitLabURL", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("requireExplicitInstance: %v", err)
			}
		})
	}
}

// TestMCPServerGate_MultiInstanceWithoutHeader_Refused verifies the wire
// rendering of that refusal: 400 rather than a silent choice, naming the
// instances only where the list is already public.
//
// The published set is served unauthenticated as RFC 9728
// authorization_servers in oauth mode, so naming it there helps a client that
// guessed wrong and gives away nothing. Legacy mode publishes no metadata
// document and this rejection is reached before the credential is judged, so
// echoing the list there would let any non-empty token enumerate the
// operator's instance hostnames.
func TestMCPServerGate_MultiInstanceWithoutHeader_Refused(t *testing.T) {
	t.Parallel()
	const other = "https://b.example.com"
	for _, tc := range []struct {
		name      string
		oauthMode bool
		wantNamed bool
	}{
		{"legacy_redacts_the_list", false, false},
		{"oauth_names_the_list", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gate := &mcpServerGate{
				gitlabURLs: []string{"https://a.example.com", other},
				oauthMode:  tc.oauthMode,
				challenge:  legacyAuthChallenge,
			}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost/mcp", http.NoBody)
			req.Header.Set("PRIVATE-TOKEN", testToken)

			_, failure := gate.resolve(req)
			if failure == nil {
				t.Fatal("the gate resolved a request naming no instance, want a refusal")
			}
			if failure.status != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", failure.status, http.StatusBadRequest)
			}
			if named := strings.Contains(failure.message, other); named != tc.wantNamed {
				t.Errorf("message = %q, names the instances = %v, want %v", failure.message, named, tc.wantNamed)
			}
			if !strings.Contains(failure.message, serverpool.RequestOptionGitLabURL) {
				t.Errorf("message = %q, want it to name the header the client must set", failure.message)
			}
		})
	}
}

// TestApplyLocalFilesystemPolicy_FollowsTheParsedFlag_NotTheArgumentScan
// verifies that the local-path policy is settled from the flag the server
// actually runs on.
//
// [toolutil] infers it once at init time by scanning os.Args, and that scanner
// stops at the first --http token while Go's flag package keeps the last one it
// parses. `--http=false --http` is the command line a launcher produces when a
// unit file carries a default and a person appends an override: the parsers
// disagree, the server listens on HTTP, and every file_path a remote caller
// sends is honored against the operator's own disk. Both rows are driven
// through the real flag parser so the divergence is measured rather than
// assumed.
func TestApplyLocalFilesystemPolicy_FollowsTheParsedFlag_NotTheArgumentScan(t *testing.T) {
	for _, tc := range []struct {
		name        string
		args        []string
		wantAllowed bool
	}{
		{name: "no flag at all is stdio", args: nil, wantAllowed: true},
		{name: "an explicit false is stdio", args: []string{"--http=false"}, wantAllowed: true},
		{name: "http refuses local paths", args: []string{"--http"}, wantAllowed: false},
		{name: "a later http beats an earlier false", args: []string{"--http=false", "--http"}, wantAllowed: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := toolutil.LocalFilesystemAccessAllowed()
			t.Cleanup(func() { toolutil.SetLocalFilesystemAccess(previous) })
			// Start from the wrong answer so a policy that never ran is
			// distinguishable from one that ran and agreed.
			toolutil.SetLocalFilesystemAccess(!tc.wantAllowed)

			withFreshFlagSet(t)
			var useHTTP bool
			flag.BoolVar(&useHTTP, "http", false, "Run MCP server in HTTP mode")
			if err := flag.CommandLine.Parse(tc.args); err != nil {
				t.Fatalf("parsing %v: %v", tc.args, err)
			}

			applyLocalFilesystemPolicy(useHTTP)

			if got := toolutil.LocalFilesystemAccessAllowed(); got != tc.wantAllowed {
				t.Errorf("local filesystem access = %v after %v, want %v", got, tc.args, tc.wantAllowed)
			}
		})
	}
}

// TestLogDeprecatedEnvNames_WarnsThroughTheConfiguredLogger verifies that the
// environment-variable migration's only operator-facing output reaches the log.
//
// Which names warn, and in what words, is covered where that logic lives in
// internal/config. What cannot be seen from there is the half this file owns:
// the warning is emitted after slog.SetDefault has installed the handler the
// rest of startup uses, so a call placed earlier would either be formatted
// differently from every other line or be lost entirely.
//
// The warning is what makes the rename a migration rather than a permanent
// shim: an old name that keeps working and says nothing is one nobody moves off
// before v3 removes it under them.
func TestLogDeprecatedEnvNames_WarnsThroughTheConfiguredLogger(t *testing.T) {
	// The prefixed name is cleared rather than assumed absent: another test in
	// this package passes -log-level, and that flag writes the prefixed
	// spelling into the process environment.
	os.Unsetenv(config.EnvPrefix + "LOG_LEVEL")
	t.Setenv("LOG_LEVEL", "warn")
	if got := config.Getenv("LOG_LEVEL"); got != "warn" {
		t.Fatalf("config.Getenv(%q) = %q, want the deprecated name to be read", "LOG_LEVEL", got)
	}

	var logged bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))

	logDeprecatedEnvNames()

	for _, want := range []string{"LOG_LEVEL", config.EnvPrefix + "LOG_LEVEL", "v3"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(logged.String(), want) {
				t.Errorf("the startup warning does not mention %q: %s", want, logged.String())
			}
		})
	}
}

// TestServeHTTPOn_RequestInFlightAtShutdown_ExhaustedDrainBudgetIsNotAFailure
// pins that a connection which has not gone idle when the process context is
// cancelled costs a forced close, not a failed shutdown.
//
// The failure this replaces is the one that made "the five second bound is too
// tight under CI load" look like a load problem. A tool call waiting on GitLab
// keeps its connection out of the idle state [http.Server.Shutdown] waits for,
// so the drain budget runs out however large it is, and until now that was
// returned as "http server shutdown: context deadline exceeded" — exit 1, a
// failed unit to a supervisor and a broken build to CI, for one straggler
// connection that process exit was about to drop anyway.
//
// The GitLab stand-in never answers the project list, which is what holds the
// handler. The pool entry is warmed first on purpose: entry construction is
// bounded by the serve context, so a request still building one would abort at
// cancellation instead of holding the drain, and the test would prove nothing.
//
// The serve context states a drain budget of its own, which is also what keeps
// the test to a few seconds rather than the full fifteen. It has to be armed
// after the warm-up rather than declared up front, because that warm-up builds
// a catalog and costs seconds on a plain run and an order of magnitude more
// under the race detector: a deadline picked before it would expire during
// setup on exactly the slow runners this has to survive.
func TestServeHTTPOn_RequestInFlightAtShutdown_ExhaustedDrainBudgetIsNotAFailure(t *testing.T) {
	var enteredOnce, releaseOnce sync.Once
	entered := make(chan struct{})
	release := make(chan struct{})

	gitlab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/version":
			w.Header().Set(hdrContentType, mimeJSON)
			_, _ = w.Write([]byte(`{"version":"16.0.0","revision":"test"}`))
		case strings.HasPrefix(r.URL.Path, "/api/v4/projects"):
			enteredOnce.Do(func() { close(entered) })
			<-release
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(gitlab.Close)
	// Registered after the server's own close so it runs before it: Close
	// waits for outstanding requests, and the handler above is one.
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })

	inner, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &armedDeadlineContext{Context: inner}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveHTTPOn(ctx, statelessTestConfig(gitlab.URL, true), addr, listener, defaultHTTPIdleTimeout)
	}()
	waitForHTTPServerReady(t, addr, errCh)

	warm := postStatelessJSONRPC(t, addr, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	warm.Body.Close()

	callDone := make(chan struct{})
	go func() {
		defer close(callDone)
		body := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"gitlab_execute_action",` +
			`"arguments":{"action":"project.list","params":{}}}}`
		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodPost,
			"http://"+addr, strings.NewReader(body))
		if reqErr != nil {
			t.Errorf("build the tool call that shutdown must find in flight: %v", reqErr)
			return
		}
		req.Header.Set(hdrContentType, mimeJSON)
		req.Header.Set("Accept", mimeJSONSSE)
		req.Header.Set("PRIVATE-TOKEN", testToken)
		if resp, doErr := testHTTPClient.Do(req); doErr == nil {
			resp.Body.Close()
		}
	}()
	t.Cleanup(func() { <-callDone })

	select {
	case <-entered:
	case <-time.After(testHTTPLivenessTimeout):
		t.Fatal("the tool call never reached the GitLab stand-in, so no request was in flight at shutdown")
	}

	const budget = 2 * time.Second
	ctx.arm(time.Now().Add(budget))
	start := time.Now()
	cancel()
	select {
	case serveErr := <-errCh:
		if serveErr != nil {
			t.Fatalf("serveHTTPOn() = %v after %s, want a clean stop: a connection that has not gone idle is not a process failure",
				serveErr, time.Since(start))
		}
	case <-time.After(testHTTPLivenessTimeout):
		t.Fatalf("serveHTTPOn() did not return within %s of cancellation", testHTTPLivenessTimeout)
	}
	// The stated budget is what bounded the drain, not httpShutdownTimeout:
	// with the constant in charge this is the full fifteen seconds.
	if elapsed := time.Since(start); elapsed >= httpShutdownTimeout {
		t.Errorf("the drain took %s with a stated budget of %s, want the caller's budget to bound it",
			elapsed, budget)
	}
}

// armedDeadlineContext is a context that gains a deadline when the test arms
// it, and never fires [context.Context.Done] because of it.
//
// It exists for the one shape in which a caller-stated drain budget is
// meaningful: a deadline still in the future when cancellation ends the
// serving. A plain [context.WithTimeout] cannot express it in a test whose
// setup cost varies by an order of magnitude, since the deadline would have to
// be picked before the setup it must outlast.
type armedDeadlineContext struct {
	context.Context //nolint:containedctx // a context decorator, which is what this is

	mu       sync.Mutex
	deadline time.Time
}

// arm sets the deadline the context reports from now on.
func (c *armedDeadlineContext) arm(at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deadline = at
}

// Deadline reports the armed deadline, and none until arm is called.
func (c *armedDeadlineContext) Deadline() (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.deadline, !c.deadline.IsZero()
}

// TestShutdownHTTPServer_CallersDeadline_BoundsTheDrain pins that the drain
// budget comes from the caller's deadline when the caller stated a tighter one,
// which is the rule [telemetry.Provider.Shutdown] follows.
//
// Both cases hold a request in flight, so the budget is genuinely spent rather
// than short-circuited by an idle server, and both must end without an error.
// The elapsed time is what says whose budget was used: [httpShutdownTimeout] is
// fifteen seconds, so a run that honors the caller finishes in a fraction of
// that, and a deadline already in the past skips the drain altogether.
func TestShutdownHTTPServer_CallersDeadline_BoundsTheDrain(t *testing.T) {
	cases := []struct {
		name      string
		remaining time.Duration
		atLeast   time.Duration
		atMost    time.Duration
	}{
		{
			name:      "tighter than the default",
			remaining: time.Second,
			atLeast:   500 * time.Millisecond,
			atMost:    8 * time.Second,
		},
		{
			name:      "already passed",
			remaining: -time.Second,
			atLeast:   0,
			atMost:    5 * time.Second,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			httpServer := blockedHTTPServer(t)

			// The serve context is always done by the time the drain starts,
			// which is exactly what makes a past deadline reachable here.
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(tc.remaining))
			cancel()

			start := time.Now()
			if err := shutdownHTTPServer(ctx, httpServer); err != nil {
				t.Fatalf("shutdownHTTPServer() = %v, want a clean stop", err)
			}
			elapsed := time.Since(start)
			if elapsed < tc.atLeast || elapsed > tc.atMost {
				t.Errorf("drain took %s, want between %s and %s: the budget did not come from the caller",
					elapsed, tc.atLeast, tc.atMost)
			}
		})
	}
}

// TestShutdownHTTPServer_NothingInFlight_ReturnsWithoutSpendingTheBudget pins
// that a clean drain costs nothing, which is why the size of the default budget
// is not a service-level objective: an idle server stops in milliseconds on any
// runner, quick or slow.
func TestShutdownHTTPServer_NothingInFlight_ReturnsWithoutSpendingTheBudget(t *testing.T) {
	httpServer := newHTTPServer("", http.NotFoundHandler(), defaultHTTPIdleTimeout)
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = httpServer.Serve(listener) }()

	start := time.Now()
	if shutdownErr := shutdownHTTPServer(t.Context(), httpServer); shutdownErr != nil {
		t.Fatalf("shutdownHTTPServer() = %v, want a clean stop", shutdownErr)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("draining an idle server took %s, want a prompt return", elapsed)
	}
}

// TestHTTPShutdownBudget_TakesTheSmallerOfTheDefaultAndTheCallersDeadline
// verifies the arithmetic on its own, with no server to drain.
//
// The non-positive case is the one worth pinning: a deadline in the past is
// honored literally rather than clamped back up to the default, because the
// drain escalates to a forced close instead of reporting a failure, so a caller
// with no time left gets exactly that.
func TestHTTPShutdownBudget_TakesTheSmallerOfTheDefaultAndTheCallersDeadline(t *testing.T) {
	cases := []struct {
		name      string
		ctx       context.Context //nolint:containedctx // the input under test
		wantExact time.Duration
		wantRange [2]time.Duration
	}{
		{name: "no deadline", ctx: context.Background(), wantExact: httpShutdownTimeout},
		{
			name:      "deadline beyond the default",
			ctx:       cancelledDeadlineContext(t, httpShutdownTimeout+time.Minute),
			wantExact: httpShutdownTimeout,
		},
		{
			name:      "deadline inside the default",
			ctx:       cancelledDeadlineContext(t, 2*time.Second),
			wantRange: [2]time.Duration{time.Second, 2 * time.Second},
		},
		{
			name:      "deadline already passed",
			ctx:       cancelledDeadlineContext(t, -time.Second),
			wantRange: [2]time.Duration{-time.Hour, 0},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := httpShutdownBudget(tc.ctx)
			if tc.wantExact != 0 {
				if got != tc.wantExact {
					t.Errorf("httpShutdownBudget() = %s, want %s", got, tc.wantExact)
				}
				return
			}
			if got < tc.wantRange[0] || got > tc.wantRange[1] {
				t.Errorf("httpShutdownBudget() = %s, want between %s and %s", got, tc.wantRange[0], tc.wantRange[1])
			}
		})
	}
}

// cancelledDeadlineContext returns a context whose deadline is the given
// distance from now, already cancelled so it matches the state the drain sees:
// the serve context is done before the budget is ever computed.
func cancelledDeadlineContext(t *testing.T, in time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(in))
	cancel()
	return ctx
}

// blockedHTTPServer starts an [http.Server] on a loopback listener with a
// handler that never returns, and comes back only once a request is provably in
// flight, so draining it is guaranteed to spend the whole budget.
func blockedHTTPServer(t *testing.T) *http.Server {
	t.Helper()

	var enteredOnce sync.Once
	entered := make(chan struct{})
	release := make(chan struct{})
	httpServer := newHTTPServer("", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		enteredOnce.Do(func() { close(entered) })
		<-release
	}), defaultHTTPIdleTimeout)

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = httpServer.Serve(listener) }()

	callDone := make(chan struct{})
	// Registered before the release so it runs after it: the handler cannot
	// return, and so the request cannot finish, until release is closed.
	t.Cleanup(func() { <-callDone })
	t.Cleanup(func() { close(release) })

	go func() {
		defer close(callDone)
		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet,
			"http://"+listener.Addr().String(), nil)
		if reqErr != nil {
			t.Errorf("build the request that must be in flight during the drain: %v", reqErr)
			return
		}
		if resp, doErr := testHTTPClient.Do(req); doErr == nil {
			resp.Body.Close()
		}
	}()

	select {
	case <-entered:
	case <-time.After(testHTTPLivenessTimeout):
		t.Fatal("the blocking handler never ran, so nothing was in flight to drain")
	}
	return httpServer
}
