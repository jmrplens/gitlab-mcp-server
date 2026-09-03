//go:build httpe2e

// clientcompat_test.go covers the ways real MCP clients differ from each other
// over HTTP.
//
// The clients that reach this server are not one implementation. They disagree
// about which protocol version to announce, which Accept types to send, whether
// to carry a session, which credential header to use, and — the one that
// surprises people — whether to send an Origin at all. An Electron-based client
// sends one, and a server that treats "has an Origin" as "is a web page" locks
// out a desktop app.
package httpe2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// TestClient_ProtocolVersions verifies that the versions clients actually
// announce are all served.
//
// A client that predates the standard headers must not be required to send
// them, and a client that announces nothing at all must still be answered:
// the transport defaults to the initialization-era version rather than
// refusing, and refusing here would break every client that has not caught up.
func TestClient_ProtocolVersions(t *testing.T) {
	// An accepted token, so the request reaches the transport instead of
	// stopping at the auth gate: a 401 would pass this test with the version
	// never having been looked at.
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	// Every revision this build negotiates, plus the no-header case. The
	// default transport is stateless, which is what makes 2026-07-28 servable.
	for _, version := range []string{"", "2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25", "2026-07-28"} {
		name := version
		if name == "" {
			name = "no version header"
		}
		t.Run(name, func(t *testing.T) {
			// An empty value deletes the header the harness sets by default,
			// which is what "no version header" has to mean here.
			headers := map[string]string{
				"PRIVATE-TOKEN":        "glpat-whatever",
				"MCP-Protocol-Version": version,
			}
			// SEP-2575's per-request _meta belongs to 2026-07-28 and carries
			// its own protocolVersion, which the transport requires the header
			// to agree with (-32020). An older revision therefore sends the
			// plain body it would really send, not the modern one with a
			// mismatched _meta.
			body := legacyToolsListBody
			if version == "2026-07-28" {
				body = toolsListBody
			}
			got := srv.do(t, request{
				method: http.MethodPost, path: "/mcp",
				body:    body,
				headers: headers,
			})
			if got.status != http.StatusOK {
				t.Errorf("protocol version %q got %d, want 200: %s", version, got.status, got.body)
			}
		})
	}
}

// TestClient_UnknownProtocolVersionIsNotFatal verifies that a version this
// build has never heard of does not take the server down or produce a 5xx.
// Clients ship ahead of servers.
func TestClient_UnknownProtocolVersionIsNotFatal(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	for _, version := range []string{"2099-01-01", "not-a-date", "0", "1.0", "2026-03-26"} {
		t.Run(version, func(t *testing.T) {
			got := srv.do(t, request{
				method: http.MethodPost, path: "/mcp", body: legacyToolsListBody,
				headers: map[string]string{
					"PRIVATE-TOKEN":        "glpat-whatever",
					"MCP-Protocol-Version": version,
				},
			})
			if got.status != http.StatusBadRequest {
				t.Errorf("version %q got %d, want %d", version, got.status, http.StatusBadRequest)
				return
			}

			// The body is what makes the 400 actionable. The specification tells a
			// client that a 400 whose body is not a recognizable JSON-RPC error
			// means it is talking to an initialization-era server, so it falls back
			// to the withdrawn HTTP+SSE transport and ends with no transport at
			// all. The supported list is the single retry that works instead.
			var decoded struct {
				Error struct {
					Code int `json:"code"`
					Data struct {
						Supported []string `json:"supported"`
						Requested string   `json:"requested"`
					} `json:"data"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(got.body), &decoded); err != nil {
				t.Errorf("version %q: body is not JSON: %v (%s)", version, err, got.body)
				return
			}
			if decoded.Error.Code != -32022 {
				t.Errorf("version %q: error.code = %d, want -32022", version, decoded.Error.Code)
			}
			if decoded.Error.Data.Requested != version {
				t.Errorf("version %q: error.data.requested = %q", version, decoded.Error.Data.Requested)
			}
			if len(decoded.Error.Data.Supported) == 0 {
				t.Errorf("version %q: error.data.supported is empty; the client is left with nothing to retry", version)
			}
		})
	}
}

// TestClient_AcceptHeaderVariants verifies the Accept types clients send.
//
// The streamable transport can answer as SSE or as JSON, and clients differ:
// some send both types, some only one, and some send nothing. None of those is
// a reason to fail.
func TestClient_AcceptHeaderVariants(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	for _, accept := range []string{
		"application/json, text/event-stream",
		"application/json",
		"text/event-stream",
		"*/*",
		"",
	} {
		name := accept
		if name == "" {
			name = "no Accept header"
		}
		t.Run(name, func(t *testing.T) {
			got := srv.do(t, request{
				method: http.MethodPost, path: "/mcp", body: toolsListBody,
				headers: map[string]string{
					"PRIVATE-TOKEN":        "glpat-whatever",
					"Accept":               accept,
					"MCP-Protocol-Version": protocolVersion,
				},
			})
			if got.status >= http.StatusInternalServerError {
				t.Errorf("Accept %q produced %d: %s", accept, got.status, got.body)
			}
			// Whatever spelling of Accept got us here, a response the server
			// actually answers as SSE must carry the anti-buffering header —
			// and, invisibly to this assertion but from the same decision, must
			// have had its write deadline cleared. Deciding that from the
			// Accept substring missed `*/*` and `text/*`, which the SDK accepts
			// and answers with a real stream: those clients got a stream an
			// nginx-class proxy may buffer and our own WriteTimeout severs
			// after 60 seconds.
			if isEventStreamResponse(got.header.Get("Content-Type")) {
				if got.header.Get("X-Accel-Buffering") != "no" {
					t.Errorf("Accept %q was answered text/event-stream without X-Accel-Buffering: no", accept)
				}
			} else if got.header.Get("X-Accel-Buffering") != "" {
				t.Errorf("Accept %q was answered %q yet carries X-Accel-Buffering", accept, got.header.Get("Content-Type"))
			}
		})
	}
}

// isEventStreamResponse reports whether a Content-Type names the SSE media
// type, ignoring parameters and case.
func isEventStreamResponse(contentType string) bool {
	base, _, _ := strings.Cut(contentType, ";")
	return strings.EqualFold(strings.TrimSpace(base), "text/event-stream")
}

// TestClient_WildcardAcceptGetsAStreamableResponse pins the case the Accept
// substring missed.
//
// The SDK treats `*/*` and `text/*` as accepting a stream and answers
// text/event-stream, but the middleware used to decide "this is SSE" by looking
// for the literal "text/event-stream" in the request's Accept header. A client
// sending curl's default therefore received a genuine SSE stream with no
// X-Accel-Buffering header — so an nginx-class proxy may buffer it — and with
// the 60-second write deadline still armed, which severs any stream outliving
// it. Both failure modes look like network flakiness rather than a server bug.
//
// This needs a GitLab that authenticates, because only a request that succeeds
// is answered with a stream at all.
func TestClient_WildcardAcceptGetsAStreamableResponse(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	for _, accept := range []string{"*/*", "text/*, application/json", "application/json, TEXT/EVENT-STREAM"} {
		t.Run(accept, func(t *testing.T) {
			got := srv.do(t, request{
				method: http.MethodPost, path: "/mcp", body: toolsListBody,
				headers: map[string]string{
					"PRIVATE-TOKEN":        "glpat-whatever",
					"Accept":               accept,
					"MCP-Protocol-Version": protocolVersion,
				},
			})

			if !isEventStreamResponse(got.header.Get("Content-Type")) {
				t.Skipf("Accept %q was answered %q, not a stream", accept, got.header.Get("Content-Type"))
			}
			if got.header.Get("X-Accel-Buffering") != "no" {
				t.Errorf("Accept %q got a text/event-stream response without X-Accel-Buffering: no — a buffering proxy would hold its events", accept)
			}
		})
	}
}

// TestClient_DesktopOriginBehaviour records how a client that sends an Origin
// without browser fetch metadata is treated, and how an operator lets one in.
//
// Electron-based clients can send an Origin like app:// or vscode-file://. The
// stdlib compares Origin against Host when Sec-Fetch-Site is absent, so such a
// request is refused by default — conservatively and, for a DNS-rebinding
// control, correctly, since nothing distinguishes it from a page. What matters
// is that the escape hatch works: an operator who runs such a client names its
// origin in --trusted-origins.
func TestClient_DesktopOriginBehaviour(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")

	const desktopOrigin = "vscode-file://vscode-app"

	t.Run("refused by default when it looks like a page", func(t *testing.T) {
		srv := startServer(t, nil, "--gitlab-url="+gitlab.url)
		got := srv.do(t, mcpPOST(map[string]string{
			"PRIVATE-TOKEN": "glpat-whatever",
			"Origin":        desktopOrigin,
		}))
		if got.status != http.StatusForbidden {
			t.Errorf("status = %d, want %d — an Origin with no fetch metadata cannot be told from a page", got.status, http.StatusForbidden)
		}
	})

	t.Run("allowed once the operator names it", func(t *testing.T) {
		srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--trusted-origins="+desktopOrigin)
		got := srv.do(t, mcpPOST(map[string]string{
			"PRIVATE-TOKEN": "glpat-whatever",
			"Origin":        desktopOrigin,
		}))
		if got.status == http.StatusForbidden {
			t.Errorf("a trusted desktop origin was still refused: %s", got.body)
		}
	})

	t.Run("fetch metadata that says it is not cross-site is honored", func(t *testing.T) {
		srv := startServer(t, nil, "--gitlab-url="+gitlab.url)
		for _, meta := range []string{"same-origin", "none"} {
			t.Run(meta, func(t *testing.T) {
				got := srv.do(t, mcpPOST(map[string]string{
					"PRIVATE-TOKEN":  "glpat-whatever",
					"Origin":         desktopOrigin,
					"Sec-Fetch-Site": meta,
				}))
				if got.status == http.StatusForbidden {
					t.Errorf("Sec-Fetch-Site: %s was refused as cross-site", meta)
				}
			})
		}
	})
}

// TestClient_CredentialHeaderVariants verifies that legacy mode accepts what it
// advertises, in the forms clients send it.
//
// A personal access token works as Authorization: Bearer as well as
// PRIVATE-TOKEN, and header names are case-insensitive on the wire, which not
// every client normalizes.
func TestClient_CredentialHeaderVariants(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	for _, headers := range []map[string]string{
		{"PRIVATE-TOKEN": "glpat-x"},
		{"private-token": "glpat-x"},
		{"Authorization": "Bearer glpat-x"},
		{"authorization": "bearer glpat-x"},
	} {
		var name string
		for k := range headers {
			name = k
		}
		t.Run(name, func(t *testing.T) {
			got := srv.do(t, mcpPOST(headers))
			// The token is rejected upstream, so 401 is right — but the
			// message must be GitLab's verdict, not "no credential sent".
			if strings.Contains(got.body, "Authentication required") {
				t.Errorf("the credential in %v was not seen at all: %s", headers, got.body)
			}
		})
	}
}

// TestClient_StatefulSessionsStillWork verifies the legacy stateful mode some
// clients still use: the server issues an Mcp-Session-Id and honors it.
func TestClient_StatefulSessionsStillWork(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--stateless=false")

	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"compat-test","version":"1"}}}`
	// Deliberately not announcing 2026-07-28: the SDK serves that version on
	// stateless servers only, and a stateful deployment is by definition
	// serving the clients that predate it.
	got := srv.do(t, request{
		method: http.MethodPost, path: "/mcp", body: initialize,
		headers: map[string]string{
			"PRIVATE-TOKEN":        "glpat-x",
			"MCP-Protocol-Version": "2025-11-25",
		},
	})

	if got.status != http.StatusOK {
		t.Fatalf("initialize = %d, want %d: %s", got.status, http.StatusOK, got.body)
	}
	if got.header.Get("Mcp-Session-Id") == "" {
		t.Error("stateful mode issued no Mcp-Session-Id")
	}
}

// TestClient_JSONResponseMode verifies --json-response: the same call answered
// as application/json rather than an SSE stream, for clients that cannot read
// one.
func TestClient_JSONResponseMode(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--json-response")

	got := srv.do(t, mcpPOST(map[string]string{
		"PRIVATE-TOKEN": "glpat-x",
		"Mcp-Method":    "tools/list",
	}))

	if got.status != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", got.status, http.StatusOK, got.body)
	}
	if ct := got.header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json in --json-response mode", ct)
	}
	if strings.HasPrefix(got.body, "event:") {
		t.Error("--json-response still returned an SSE frame")
	}
}

// TestClient_ConcurrentRequestsShareOnePoolEntry verifies that clients arriving
// at once with the same credential converge on a single pooled server rather
// than racing to build several, and that none of them fails.
func TestClient_ConcurrentRequestsShareOnePoolEntry(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	const clients = 12
	var wg sync.WaitGroup
	statuses := make([]int, clients)
	for i := range clients {
		wg.Go(func() {
			// t.Fatal is not safe off the test goroutine; the status is
			// recorded and asserted after the wait.
			statuses[i] = srv.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": "glpat-shared"})).status
		})
	}
	wg.Wait()

	for i, status := range statuses {
		if status >= http.StatusInternalServerError {
			t.Errorf("concurrent client %d got %d", i, status)
		}
	}
	// One build, however many callers raced for it: the credential probe and
	// the identity lookup each hit /user once. Before the pool collapsed
	// same-key builds this was two calls per client, all but one discarded.
	if calls := gitlab.calls(); calls > 2 {
		t.Errorf("%d upstream /user calls for %d concurrent clients sharing one credential, want at most 2; the pool is building an entry per caller", calls, clients)
	}
}

// TestClient_DistinctCredentialsGetDistinctServers verifies the other half:
// two credentials must not share a pooled server, or one client would act as
// another.
func TestClient_DistinctCredentialsGetDistinctServers(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	for i := range 3 {
		got := srv.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": fmt.Sprintf("glpat-client-%d", i)}))
		if got.status >= http.StatusInternalServerError {
			t.Fatalf("client %d got %d: %s", i, got.status, got.body)
		}
	}

	if calls := gitlab.calls(); calls < 3 {
		t.Errorf("%d upstream /user calls for 3 distinct credentials; entries are being shared across tokens", calls)
	}
}
