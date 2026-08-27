//go:build httpe2e

// robustness_test.go throws malformed, oversized and hostile input at the HTTP
// surface and checks that the server refuses it without falling over.
//
// The bar is deliberately not "returns the right error". It is that the process
// survives, keeps serving, and never hangs: every case ends by asking /health
// whether the server is still there, because a panic in a handler that the
// runtime recovers still leaves a client with a broken connection and an
// operator with nothing in the logs.
package httpe2e

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// assertStillServing fails the test if the server stopped answering. A rejection
// is fine; silence is not.
func assertStillServing(t *testing.T, srv *server, after string) {
	t.Helper()
	got := srv.do(t, request{method: http.MethodGet, path: "/health"})
	if got.status != http.StatusOK {
		t.Fatalf("the server stopped serving after %s: /health = %d", after, got.status)
	}
}

// TestRobust_MalformedBodies verifies that nothing a client can put in the body
// takes the server down.
func TestRobust_MalformedBodies(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	for _, tc := range []struct{ name, body string }{
		{"empty", ""},
		{"not json", "this is not json at all"},
		{"truncated json", `{"jsonrpc":"2.0","id":1,"method":`},
		{"json null", "null"},
		{"json array", "[]"},
		{"json number", "42"},
		{"empty object", "{}"},
		{"no method", `{"jsonrpc":"2.0","id":1}`},
		{"unknown method", `{"jsonrpc":"2.0","id":1,"method":"does/not/exist"}`},
		{"wrong jsonrpc version", `{"jsonrpc":"1.0","id":1,"method":"tools/list"}`},
		{"null bytes", "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/\x00list\"}"},
		{"invalid utf8", "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"\xff\xfe\"}"},
		{"deeply nested", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":` + strings.Repeat(`{"a":`, 500) + `1` + strings.Repeat(`}`, 500) + `}`},
		{"huge string value", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + strings.Repeat("x", 100_000) + `"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := srv.do(t, request{
				method: http.MethodPost, path: "/mcp", body: tc.body,
				headers: map[string]string{"PRIVATE-TOKEN": "glpat-x"},
			})
			// A 5xx would mean the server blamed itself for something the
			// client sent.
			if got.status >= http.StatusInternalServerError {
				t.Errorf("status = %d for %s, want a client error: %s", got.status, tc.name, truncate(got.body))
			}
			assertStillServing(t, srv, tc.name)
		})
	}
}

// TestRobust_OversizedBodyIsRefusedNotBuffered verifies that a body larger than
// the limit is cut off rather than read into memory, and that the server is
// still healthy afterwards.
func TestRobust_OversizedBodyIsRefusedNotBuffered(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--max-request-body-bytes=4096")

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + strings.Repeat("A", 64*1024) + `"}}`
	got := srv.do(t, request{
		method: http.MethodPost, path: "/mcp", body: body,
		headers: map[string]string{"PRIVATE-TOKEN": "glpat-x"},
	})

	if got.status == http.StatusOK {
		t.Errorf("a %d-byte body was accepted against a 4096-byte limit", len(body))
	}
	if got.status >= http.StatusInternalServerError {
		t.Errorf("status = %d, want a client error for an oversized body", got.status)
	}
	assertStillServing(t, srv, "an oversized body")
}

// TestRobust_HostileHeaderValues verifies that header values a client controls
// cannot break the server or the responses it writes.
//
// The GITLAB-URL header is the interesting one: its rejection message is built
// from what the client sent, so a hostile value is a response-splitting
// attempt. Values Go's own client refuses to transmit are exercised over a raw
// socket instead — see TestRobust_RawSocketAttacks.
func TestRobust_HostileHeaderValues(t *testing.T) {
	// No pinned URL, so the GITLAB-URL header is the one the server reads and
	// the value under test actually reaches the parser.
	srv := startServer(t, nil)

	for _, tc := range []struct{ name, value string }{
		{"very long", "http://" + strings.Repeat("a", 8000) + ".example.com"},
		{"unicode", "http://例え.テスト/パス"},
		{"scheme only", "http://"},
		{"no scheme", "example.com"},
		{"file scheme", "file:///etc/passwd"},
		{"credentials in url", "http://user:pass@example.com"},
		{"query string", "http://example.com/?a=b"},
		{"fragment", "http://example.com/#frag"},
		{"space in host", "http://exa mple.com"},
		{"just a scheme separator", "://"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := srv.do(t, request{
				method: http.MethodPost, path: "/mcp", body: toolsListBody,
				headers: map[string]string{
					"PRIVATE-TOKEN": "glpat-x",
					"GITLAB-URL":    tc.value,
				},
			})
			if got.status >= http.StatusInternalServerError {
				t.Errorf("status = %d for %s: %s", got.status, tc.name, truncate(got.body))
			}
			// The rejection must not echo a value that could carry a
			// credential or a query parameter back into the logs or the body.
			if strings.Contains(got.body, "user:pass") {
				t.Error("the rejection echoed embedded credentials back to the caller")
			}
			assertStillServing(t, srv, tc.name)
		})
	}
}

// TestRobust_RawSocketAttacks sends what net/http refuses to construct.
//
// Go's client rejects a header value containing CR, LF or NUL, and rejects a
// URL with a control character, so none of this can be expressed through
// http.Client — which is exactly why it is worth sending by hand. The bar is
// that the server answers something and keeps serving; a header the client
// injected must never appear in the response.
func TestRobust_RawSocketAttacks(t *testing.T) {
	srv := startServer(t, nil)

	for _, tc := range []struct{ name, wire string }{
		{
			"crlf injection in a header value",
			"POST /mcp HTTP/1.1\r\nHost: localhost\r\nPRIVATE-TOKEN: glpat-x\r\nGITLAB-URL: http://example.com/\r\nX-Injected: yes\r\nContent-Length: 0\r\n\r\n",
		},
		{
			"bare LF line endings",
			"POST /mcp HTTP/1.1\nHost: localhost\nPRIVATE-TOKEN: glpat-x\nContent-Length: 0\n\n",
		},
		{
			"NUL in the path",
			"GET /health\x00 HTTP/1.1\r\nHost: localhost\r\n\r\n",
		},
		{
			"absurd request line",
			"GET " + strings.Repeat("/a", 20000) + " HTTP/1.1\r\nHost: localhost\r\n\r\n",
		},
		{
			"no method",
			" /health HTTP/1.1\r\nHost: localhost\r\n\r\n",
		},
		{
			"content-length disagrees with the body",
			"POST /mcp HTTP/1.1\r\nHost: localhost\r\nPRIVATE-TOKEN: glpat-x\r\nContent-Length: 500\r\n\r\n{}",
		},
		{
			"duplicate content-length",
			"POST /mcp HTTP/1.1\r\nHost: localhost\r\nContent-Length: 2\r\nContent-Length: 3\r\n\r\n{}",
		},
		{
			"transfer-encoding and content-length together",
			"POST /mcp HTTP/1.1\r\nHost: localhost\r\nContent-Length: 2\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n",
		},
		{
			"http 0.9 style",
			"GET /health\r\n\r\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reply := srv.raw(t, tc.wire)
			if strings.Contains(reply, "X-Injected") {
				t.Error("a header the client smuggled into a value came back in the response")
			}
			assertStillServing(t, srv, tc.name)
		})
	}
}

// TestRobust_GitLabURLHostIsNotRestricted records that the per-request
// GITLAB-URL header may name any host, including private and link-local
// addresses.
//
// This is the accepted design of a per-request instance selector — the client
// supplies both the URL and the credential — and it is what a self-hosted
// deployment needs. It is written down here rather than left implicit, because
// on a *public* deployment it is a blind server-side request forgery primitive:
// a caller can make the server connect somewhere it chooses and infer from the
// timing and status whether that address answers.
//
// Pinning --gitlab-url closes it completely: the header is then ignored, which
// the second half asserts.
func TestRobust_GitLabURLHostIsNotRestricted(t *testing.T) {
	t.Run("unpinned deployment accepts any host", func(t *testing.T) {
		srv := startServer(t, nil)

		got := srv.do(t, request{
			method: http.MethodPost, path: "/mcp", body: toolsListBody,
			headers: map[string]string{
				"PRIVATE-TOKEN": "glpat-x",
				"GITLAB-URL":    "http://169.254.169.254",
			},
		})
		// Refused because nothing there speaks the GitLab API, not because
		// the address was rejected: the distinction is the point.
		if got.status == http.StatusBadRequest {
			t.Log("note: this build rejects link-local hosts outright, which is stricter than documented")
		}
		assertStillServing(t, srv, "a link-local GITLAB-URL")
	})

	t.Run("pinned deployment ignores the header", func(t *testing.T) {
		gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
		srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

		got := srv.do(t, request{
			method: http.MethodPost, path: "/mcp", body: toolsListBody,
			headers: map[string]string{
				"PRIVATE-TOKEN": "glpat-x",
				"GITLAB-URL":    "http://169.254.169.254",
				"Mcp-Method":    "tools/list",
			},
		})
		if got.status != http.StatusOK {
			t.Errorf("status = %d, want %d — the pinned URL should be used and the header ignored: %s", got.status, http.StatusOK, truncate(got.body))
		}
		if gitlab.calls() == 0 {
			t.Error("the pinned instance was never contacted; the header may have won")
		}
	})
}

// TestRobust_UnusualMethodsAndPaths verifies that the surface answers
// everything else without falling over.
func TestRobust_UnusualMethodsAndPaths(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodHead, http.MethodTrace, "BREW"} {
		t.Run("method "+method, func(t *testing.T) {
			got := srv.do(t, request{
				method:  method,
				path:    "/mcp",
				headers: map[string]string{"PRIVATE-TOKEN": "glpat-x"},
			})
			if got.status >= http.StatusInternalServerError {
				t.Errorf("%s produced %d", method, got.status)
			}
			assertStillServing(t, srv, method)
		})
	}

	for _, path := range []string{
		"/../../etc/passwd",
		"/%2e%2e%2f%2e%2e%2fetc%2fpasswd",
		"/" + strings.Repeat("a/", 2000),
		"/mcp?" + strings.Repeat("k=v&", 2000),
	} {
		t.Run("path "+truncate(path), func(t *testing.T) {
			got := srv.do(t, request{
				method:  http.MethodGet,
				path:    path,
				headers: map[string]string{"PRIVATE-TOKEN": "glpat-x"},
			})
			if got.status >= http.StatusInternalServerError {
				t.Errorf("%s produced %d", path, got.status)
			}
			if strings.Contains(got.body, "root:") {
				t.Fatal("a path traversal returned file content")
			}
			assertStillServing(t, srv, "path "+truncate(path))
		})
	}
}

// TestRobust_ManyAndLargeHeaders verifies that header-count and header-size
// abuse is bounded by the stdlib rather than by memory.
func TestRobust_ManyAndLargeHeaders(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	many := map[string]string{"PRIVATE-TOKEN": "glpat-x"}
	for i := range 200 {
		many[fmt.Sprintf("X-Filler-%d", i)] = "value"
	}
	got := srv.do(t, request{method: http.MethodPost, path: "/mcp", body: toolsListBody, headers: many})
	if got.status >= http.StatusInternalServerError {
		t.Errorf("200 headers produced %d", got.status)
	}
	assertStillServing(t, srv, "many headers")

	big := map[string]string{
		"PRIVATE-TOKEN": "glpat-" + strings.Repeat("x", 32*1024),
		"X-Large":       strings.Repeat("y", 32*1024),
	}
	got = srv.do(t, request{method: http.MethodPost, path: "/mcp", body: toolsListBody, headers: big})
	if got.status >= http.StatusInternalServerError && got.status != http.StatusServiceUnavailable {
		t.Errorf("oversized headers produced %d", got.status)
	}
	assertStillServing(t, srv, "oversized headers")
}

// TestRobust_SustainedInvalidTrafficKeepsServing verifies that a flood of
// rejected requests from many addresses does not degrade the server for anyone
// else — the failure budget bounds the upstream cost, and the process stays up.
func TestRobust_SustainedInvalidTrafficKeepsServing(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--trusted-proxy-header=X-Real-IP")

	const addresses, perAddress = 8, 20
	var wg sync.WaitGroup
	for a := range addresses {
		wg.Go(func() {
			for i := range perAddress {
				srv.do(t, mcpPOST(map[string]string{
					"PRIVATE-TOKEN": fmt.Sprintf("glpat-flood-%d-%d", a, i),
					"X-Real-IP":     fmt.Sprintf("198.51.100.%d", a),
				}))
			}
		})
	}
	wg.Wait()

	assertStillServing(t, srv, "a flood of invalid credentials")

	// Every address should have run out of budget well before its 20th
	// attempt, so the upstream cost is bounded by addresses, not by requests.
	if calls := gitlab.calls(); calls > addresses*12 {
		t.Errorf("%d upstream calls for %d flooding addresses; the budget is not bounding the relay", calls, addresses)
	}
}

// TestRobust_SlowUpstreamDoesNotHangTheServer verifies that a GitLab which
// never answers does not tie up the server: the request is bounded by the
// probe's own timeout and the server keeps serving everyone else meanwhile.
func TestRobust_SlowUpstreamDoesNotHangTheServer(t *testing.T) {
	gitlab := startHangingGitLab(t)
	srv := startServer(t, nil, "--gitlab-url="+gitlab)

	done := make(chan int, 1)
	go func() {
		done <- srv.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": "glpat-slow"})).status
	}()

	// While that request is stuck upstream, the server must still answer
	// something that needs no GitLab at all.
	assertStillServing(t, srv, "a request stuck on a hanging upstream")

	select {
	case status := <-done:
		if status >= http.StatusInternalServerError && status != http.StatusServiceUnavailable {
			t.Errorf("a hanging upstream produced %d", status)
		}
	case <-t.Context().Done():
		t.Fatal("the request never returned; the upstream probe is unbounded")
	}
}

// truncate shortens a value for a test name or a failure message.
func truncate(s string) string {
	const limit = 60
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}
