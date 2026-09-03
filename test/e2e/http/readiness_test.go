//go:build httpe2e

// readiness_test.go drives the handshake of a credential the server has never
// seen, which is the request that builds a pool entry.
//
// Building an entry means registering the whole tool catalog, and the pool
// builds one per credential rather than one per process, so on HTTP this cost
// is paid by every user of a shared deployment and again whenever an entry is
// reclaimed by the size bound or by --pool-idle-timeout. Measured against a
// stand-in GitLab it was 1.74s on the dynamic surface and 3.64s on individual;
// registration is the whole of it, the shell being too fast to time.
//
// These tests exist because that window is invisible from inside the process.
// A unit test can call the factory and see it return, but only a client on the
// wire can show that the handshake is answered while registration is still
// running and that the catalog is nonetheless complete when it is asked for.
package httpe2e

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// handshakeCeiling is what "answered from the shell" has to mean in wall-clock
// terms for the test to be about the defect rather than about the machine.
//
// It sits between the two: the answer measured at 8ms after the change, and
// the 1.7 to 3.6 seconds before it. A CI runner under load can miss 8ms by an
// order of magnitude and still be nowhere near a second.
const handshakeCeiling = time.Second

// initializeBody is a client's opening request.
const initializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize",` +
	`"params":{"protocolVersion":"2025-11-25","capabilities":{},` +
	`"clientInfo":{"name":"readiness","version":"1"}}}`

// TestInitialize_ForACredentialTheServerHasNeverSeen_IsAnsweredFromTheShell
// verifies that the request which builds a pool entry does not wait for the
// catalog that build registers.
//
// The individual surface is the one that matters here: it registers roughly a
// thousand tools, so before the change this request took seconds. Waiting for
// it is what an operator's clients experience one at a time, each on their
// first request.
func TestInitialize_ForACredentialTheServerHasNeverSeen_IsAnsweredFromTheShell(t *testing.T) {
	t.Parallel()

	gitlab := startFakeGitLab(t, 200, `{"id":1,"username":"readiness","name":"Readiness"}`)
	srv := startServer(t, nil, "--gitlab-url", gitlab.url, "--tool-surface", "individual", "--tier", "free")

	started := time.Now()
	resp := srv.do(t, request{
		method: "POST",
		path:   "/mcp",
		body:   initializeBody,
		headers: map[string]string{
			"PRIVATE-TOKEN":        "glpat-never-seen-before",
			"MCP-Protocol-Version": "2025-11-25",
		},
	})
	elapsed := time.Since(started)

	if resp.status != 200 {
		t.Fatalf("initialize answered %d, want 200: %s", resp.status, resp.body)
	}
	if elapsed > handshakeCeiling {
		t.Errorf("the first initialize of a new credential took %v, over the %v ceiling; "+
			"it is waiting for the tool catalog again", elapsed.Round(time.Millisecond), handshakeCeiling)
	}
}

// TestToolsList_DuringTheCatalogBuild_ReturnsTheWholeCatalog verifies the half
// that makes the speed safe.
//
// Answering the handshake early is only correct if everything that needs the
// catalog waits for it. A client that receives an empty tools/list and does not
// act on notifications/tools/list_changed concludes the server has no tools,
// which is a worse failure than the wait it replaced and much harder to
// diagnose. So this sends tools/list immediately behind the handshake, inside
// the window where registration is still running, and asserts the full surface.
//
// Unlike the test above, this one passes with or without the change, and that
// is not a defect in it: before the change there was no window to answer inside
// of, so a complete catalog was never in doubt. It is here as the regression
// net for the risk the change introduces, and it is the test that would fail if
// the gate were ever loosened to let tools/list through early.
func TestToolsList_DuringTheCatalogBuild_ReturnsTheWholeCatalog(t *testing.T) {
	t.Parallel()

	gitlab := startFakeGitLab(t, 200, `{"id":1,"username":"readiness","name":"Readiness"}`)
	srv := startServer(t, nil, "--gitlab-url", gitlab.url, "--tool-surface", "individual", "--tier", "free")

	headers := map[string]string{
		"PRIVATE-TOKEN":        "glpat-catalog-during-build",
		"MCP-Protocol-Version": "2025-11-25",
	}
	if resp := srv.do(t, request{method: "POST", path: "/mcp", body: initializeBody, headers: headers}); resp.status != 200 {
		t.Fatalf("initialize answered %d, want 200: %s", resp.status, resp.body)
	}

	resp := srv.do(t, request{
		method:  "POST",
		path:    "/mcp",
		body:    `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		headers: headers,
	})
	if resp.status != 200 {
		t.Fatalf("tools/list answered %d, want 200: %s", resp.status, resp.body)
	}

	tools := toolNamesIn(t, resp.body)
	if len(tools) < 500 {
		t.Fatalf("tools/list returned %d tools during the catalog build, want the whole individual surface; "+
			"an early answer from an empty catalog is the failure this must not have", len(tools))
	}
	for _, want := range []string{"gitlab_project_get", "gitlab_issue_list"} {
		t.Run(want, func(t *testing.T) {
			if !tools[want] {
				t.Errorf("the catalog is missing %q, so it was answered before registration finished", want)
			}
		})
	}
}

// toolNamesIn pulls the tool names out of a tools/list response, which arrives
// as an SSE frame unless --json-response is set.
func toolNamesIn(t *testing.T, body string) map[string]bool {
	t.Helper()

	payload := body
	for line := range strings.SplitSeq(body, "\n") {
		if after, ok := strings.CutPrefix(strings.TrimSpace(line), "data: "); ok {
			payload = after
		}
	}

	var parsed struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		t.Fatalf("tools/list response is not JSON-RPC: %v\n%s", err, body)
	}
	names := make(map[string]bool, len(parsed.Result.Tools))
	for _, tool := range parsed.Result.Tools {
		names[tool.Name] = true
	}
	return names
}
