//go:build e2e && !enterprise

// http_stateless_ce_test.go verifies the compiled server binary in HTTP mode
// with --stateless --json-response: self-contained JSON-RPC POSTs succeed
// without session tracking against a real GitLab instance.
package suite

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildServerBinary compiles cmd/server into dir and returns the binary path.
func buildServerBinary(ctx context.Context, t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "gitlab-mcp-server-e2e")
	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/server")
	cmd.Dir = repoRootDir(t)
	out, err := cmd.CombinedOutput()
	requireNoError(t, err, "go build ./cmd/server: "+string(out))
	return bin
}

// repoRootDir resolves the repository root from the suite directory.
func repoRootDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	requireNoError(t, err, "getwd")
	root, err := filepath.Abs(filepath.Join(wd, "..", "..", ".."))
	requireNoError(t, err, "resolve repo root")
	return root
}

// freeLocalPort reserves and releases a TCP port on localhost.
func freeLocalPort(ctx context.Context, t *testing.T) string {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	requireNoError(t, err, "reserve port")
	addr := listener.Addr().String()
	listener.Close()
	return addr
}

// postJSONRPCStateless sends a sessionless JSON-RPC POST to the binary and
// returns the response status, headers, and fully read (closed) body.
func postJSONRPCStateless(ctx context.Context, t *testing.T, addr, token, body string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr, strings.NewReader(body))
	requireNoError(t, err, "build request")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("PRIVATE-TOKEN", token)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	requireNoError(t, err, "POST "+addr)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	requireNoError(t, err, "read response body")
	return resp.StatusCode, resp.Header, data
}

// waitForBinaryHealth polls the /health endpoint until the server binary
// answers 200 OK or the deadline passes.
func waitForBinaryHealth(ctx context.Context, t *testing.T, client *http.Client, addr string) bool {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/health", nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true
		}
	}
	return false
}

// TestHTTPStatelessBinary_FullFlow_NoSessionTracking builds the real server
// binary, starts it with --http --stateless --json-response against the E2E
// GitLab instance, and verifies: (1) tools/list succeeds without initialize
// and returns no Mcp-Session-Id; (2) tools/call gitlab_find_action returns
// catalog matches; (3) GET on the MCP endpoint yields 405 Allow: POST;
// (4) /health stays reachable.
func TestHTTPStatelessBinary_FullFlow_NoSessionTracking(t *testing.T) {
	gitlabURL := os.Getenv("GITLAB_URL")
	token := os.Getenv("GITLAB_TOKEN")
	requireTruef(t, gitlabURL != "" && token != "", "GITLAB_URL and GITLAB_TOKEN are required")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	bin := buildServerBinary(ctx, t, t.TempDir())
	addr := freeLocalPort(ctx, t)

	server := exec.CommandContext(ctx, bin, //nolint:gosec // bin is compiled into t.TempDir() by this test, not user input
		"--http", "--http-addr="+addr,
		"--gitlab-url="+gitlabURL,
		"--stateless", "--json-response")
	server.Stdout = os.Stderr
	server.Stderr = os.Stderr
	requireNoError(t, server.Start(), "start server binary")
	defer func() {
		_ = server.Process.Kill()
		_ = server.Wait()
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	requireTruef(t, waitForBinaryHealth(ctx, t, client, addr), "server did not become healthy on %s", addr)

	t.Run("tools_list_without_session", func(t *testing.T) {
		status, header, body := postJSONRPCStateless(ctx, t, addr, token,
			`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
		requireTruef(t, status == http.StatusOK, "tools/list status %d: %s", status, body)
		requireTruef(t, header.Get("Mcp-Session-Id") == "",
			"stateless response must not set Mcp-Session-Id, got %q", header.Get("Mcp-Session-Id"))
		requireTruef(t, strings.HasPrefix(header.Get("Content-Type"), "application/json"),
			"Content-Type %q, want application/json", header.Get("Content-Type"))
		var rpc struct {
			Result struct {
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			} `json:"result"`
		}
		requireNoError(t, json.Unmarshal(body, &rpc), "decode tools/list")
		found := false
		for _, tool := range rpc.Result.Tools {
			if tool.Name == "gitlab_find_action" {
				found = true
			}
		}
		requireTruef(t, found, "gitlab_find_action missing from tools/list: %s", body)
	})

	t.Run("tools_call_find_action", func(t *testing.T) {
		status, _, body := postJSONRPCStateless(ctx, t, addr, token,
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"gitlab_find_action","arguments":{"query":"list projects"}}}`)
		requireTruef(t, status == http.StatusOK, "tools/call status %d: %s", status, body)
		requireTruef(t, strings.Contains(string(body), "gitlab_execute_action") || strings.Contains(string(body), "project"),
			"find_action returned no catalog matches: %s", body)
	})

	t.Run("get_returns_405", func(t *testing.T) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/", nil)
		requireNoError(t, err, "build GET")
		req.Header.Set("PRIVATE-TOKEN", token)
		resp, err := client.Do(req)
		requireNoError(t, err, "GET /")
		defer resp.Body.Close()
		requireTruef(t, resp.StatusCode == http.StatusMethodNotAllowed, "GET status %d, want 405", resp.StatusCode)
		requireTruef(t, resp.Header.Get("Allow") == http.MethodPost, "Allow %q, want POST", resp.Header.Get("Allow"))
	})
}
