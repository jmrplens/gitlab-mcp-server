//go:build httpe2e

// cross_origin_test.go pins the decisions the HTTP handler makes about browser
// requests, over real HTTP against the real binary.
//
// Every case here was found by hand before it was written down. The preflight
// one in particular: --trusted-origins shipped in 2.7.4 and did not work from
// an actual browser, because the OPTIONS that precedes a cross-origin POST was
// answered 401 and the real request never followed. It looked correct in every
// curl matrix, and in the one deployment that mattered a reverse proxy answered
// the preflight and hid it.
package httpe2e

import (
	"net/http"
	"strings"
	"testing"

	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
)

const trustedOrigin = "https://client.example"

// TestCrossOrigin_NonBrowserClientIsUnaffected verifies the case that must
// never break: a request carrying neither Origin nor Sec-Fetch-Site — every
// CLI, IDE and SDK client — is not a browser request and is not subject to any
// of this.
func TestCrossOrigin_NonBrowserClientIsUnaffected(t *testing.T) {
	srv := startServer(t, nil, "--gitlab-url=https://gitlab.example.com")

	got := srv.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": "glpat-whatever"}))

	if got.status == http.StatusForbidden {
		t.Fatalf("a request with no Origin was refused as cross-origin: %d %s", got.status, got.body)
	}
	if got.header.Get("Access-Control-Allow-Origin") != "" {
		t.Error("a request with no Origin should get no CORS headers")
	}
}

// TestCrossOrigin_UntrustedOriginIsRefused verifies the default: with no
// trusted origins configured, a browser request from another origin is refused
// before authentication or MCP dispatch, which is what the transport's
// DNS-rebinding requirement asks for.
func TestCrossOrigin_UntrustedOriginIsRefused(t *testing.T) {
	srv := startServer(t, nil, "--gitlab-url=https://gitlab.example.com")

	got := srv.do(t, mcpPOST(map[string]string{
		"PRIVATE-TOKEN":  "glpat-whatever",
		"Origin":         "https://evil.example",
		"Sec-Fetch-Site": "cross-site",
	}))

	if got.status != http.StatusForbidden {
		t.Errorf("status = %d, want %d for an untrusted cross-site POST", got.status, http.StatusForbidden)
	}
}

// TestCrossOrigin_TrustedOriginPasses verifies that an allowlisted origin gets
// through the protection and is answered on its merits — an explicit allowlist
// is validation, which is what the requirement asks for, not "refuse
// everything".
func TestCrossOrigin_TrustedOriginPasses(t *testing.T) {
	srv := startServer(t, nil,
		"--gitlab-url=https://gitlab.example.com",
		"--trusted-origins="+trustedOrigin,
	)

	got := srv.do(t, mcpPOST(map[string]string{
		"PRIVATE-TOKEN":  "glpat-whatever",
		"Origin":         trustedOrigin,
		"Sec-Fetch-Site": "cross-site",
	}))

	if got.status == http.StatusForbidden {
		t.Fatalf("a trusted origin was refused: %d %s", got.status, got.body)
	}
	if origin := got.header.Get("Access-Control-Allow-Origin"); origin != trustedOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, want the request origin %q", origin, trustedOrigin)
	}
	// Neither header is CORS-safelisted, so a browser cannot read the session
	// or protocol version unless they are exposed by name.
	if exposed := got.header.Get("Access-Control-Expose-Headers"); !strings.Contains(exposed, "Mcp-Session-Id") {
		t.Errorf("Access-Control-Expose-Headers = %q, want it to name Mcp-Session-Id", exposed)
	}
}

// TestCrossOrigin_ExactlyOneAllowOriginHeader verifies the property that a
// browser enforces and curl does not: more than one
// Access-Control-Allow-Origin is a CORS failure, and the response is rejected
// outright.
//
// This is the shape of a real outage. A reverse proxy in front adding its own
// `add_header Access-Control-Allow-Origin "*"` produces exactly two, and
// Chromium answers "contains multiple values ... but only one is allowed".
// The server's own response must never contribute more than one.
func TestCrossOrigin_ExactlyOneAllowOriginHeader(t *testing.T) {
	srv := startServer(t, nil,
		"--gitlab-url=https://gitlab.example.com",
		"--trusted-origins="+trustedOrigin,
	)

	cases := []struct {
		name string
		req  request
	}{
		{"actual_request", mcpPOST(map[string]string{
			"PRIVATE-TOKEN":  "glpat-whatever",
			"Origin":         trustedOrigin,
			"Sec-Fetch-Site": "cross-site",
		})},
		{"preflight", request{
			method: http.MethodOptions, path: "/mcp",
			headers: map[string]string{
				"Origin":                         trustedOrigin,
				"Access-Control-Request-Method":  http.MethodPost,
				"Access-Control-Request-Headers": "content-type",
			},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := srv.do(t, tc.req)
			if n := len(got.header.Values("Access-Control-Allow-Origin")); n > 1 {
				t.Errorf("%s: %d Access-Control-Allow-Origin headers, want at most 1 — a browser rejects the response outright", tc.req.method, n)
			}
		})
	}
}

// TestCrossOrigin_WildcardEchoesTheOrigin verifies that '*' accepts any origin
// while still echoing it rather than emitting a literal asterisk. These
// requests may carry credentials, and a browser rejects the wildcard on a
// credentialed request, so the literal form would defeat the setting.
func TestCrossOrigin_WildcardEchoesTheOrigin(t *testing.T) {
	srv := startServer(t, nil,
		"--gitlab-url=https://gitlab.example.com",
		"--trusted-origins=*",
	)

	const anyOrigin = "https://anything.example"
	got := srv.do(t, mcpPOST(map[string]string{
		"PRIVATE-TOKEN":  "glpat-whatever",
		"Origin":         anyOrigin,
		"Sec-Fetch-Site": "cross-site",
	}))

	if got.status == http.StatusForbidden {
		t.Fatalf("'*' should accept any origin, got %d", got.status)
	}
	if origin := got.header.Get("Access-Control-Allow-Origin"); origin != anyOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, want the echoed origin %q", origin, anyOrigin)
	}
}

// TestCrossOrigin_PreflightIsAnswered verifies the half that shipped broken: a
// browser sends OPTIONS before a cross-origin POST carrying Authorization or a
// JSON content type, and until that preflight is answered the real request is
// never sent.
func TestCrossOrigin_PreflightIsAnswered(t *testing.T) {
	srv := startServer(t, nil,
		"--gitlab-url=https://gitlab.example.com",
		"--trusted-origins="+trustedOrigin,
	)

	got := srv.do(t, request{
		method: http.MethodOptions, path: "/mcp",
		headers: map[string]string{
			"Origin":                         trustedOrigin,
			"Access-Control-Request-Method":  http.MethodPost,
			"Access-Control-Request-Headers": "content-type,private-token",
		},
	})

	if got.status != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d (401 here is the bug that made --trusted-origins useless in a browser)", got.status, http.StatusNoContent)
	}
	for header, want := range map[string]string{
		"Access-Control-Allow-Origin": trustedOrigin,
		"Access-Control-Max-Age":      "86400",
	} {
		t.Run(header, func(t *testing.T) {
			if got := got.header.Get(header); got != want {
				t.Errorf("%s = %q, want %q", header, got, want)
			}
		})
	}
	for _, name := range []string{"Content-Type", "PRIVATE-TOKEN", "Mcp-Session-Id"} {
		t.Run(name, func(t *testing.T) {
			if allowed := got.header.Get("Access-Control-Allow-Headers"); !strings.Contains(allowed, name) {
				t.Errorf("Access-Control-Allow-Headers = %q, want it to name %s", allowed, name)
			}
		})
	}
	if !strings.Contains(strings.Join(got.header.Values("Vary"), ","), "Origin") {
		t.Error("a response that varies by Origin must say so, or a cache serves one origin's permission to another")
	}
}

// TestCrossOrigin_UntrustedPreflightGetsNoPermission verifies that the
// preflight path never widens the trust decision: an origin that is not on the
// list gets no allow header, whatever it asks for.
func TestCrossOrigin_UntrustedPreflightGetsNoPermission(t *testing.T) {
	srv := startServer(t, nil,
		"--gitlab-url=https://gitlab.example.com",
		"--trusted-origins="+trustedOrigin,
	)

	got := srv.do(t, request{
		method: http.MethodOptions, path: "/mcp",
		headers: map[string]string{
			"Origin":                        "https://evil.example",
			"Access-Control-Request-Method": http.MethodPost,
		},
	})

	if origin := got.header.Get("Access-Control-Allow-Origin"); origin != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want none for an untrusted origin", origin)
	}
}

// TestCrossOrigin_SafeMethodsStayReachable verifies that the endpoints a
// registry, a scanner or a load balancer reads are not caught by any of this.
func TestCrossOrigin_SafeMethodsStayReachable(t *testing.T) {
	srv := startServer(t, nil, "--gitlab-url=https://gitlab.example.com")

	for _, path := range []string{"/health", "/.well-known/mcp/server-card.json"} {
		t.Run(path, func(t *testing.T) {
			got := srv.do(t, request{
				method:  http.MethodGet,
				path:    path,
				headers: map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"},
			})
			if got.status == http.StatusForbidden {
				t.Errorf("GET %s was refused as cross-origin; safe methods are exempt", path)
			}
		})
	}
}

// TestCrossOrigin_MalformedTrustedOriginFailsStartup verifies that a
// deployment cannot come up believing an origin is trusted when it is not.
// Silently dropping a bad entry is worse than refusing to boot.
func TestCrossOrigin_MalformedTrustedOriginFailsStartup(t *testing.T) {
	bin := serverBinary(t)
	port := freePort(t)

	out, err := runServerExpectingExit(t, bin,
		"--http", "--http-addr=127.0.0.1:"+itoa(port),
		"--gitlab-url=https://gitlab.example.com",
		"--trusted-origins=not a url",
	)
	if err == nil {
		t.Fatal("the server started with a malformed trusted origin")
	}
	if !strings.Contains(out, "trusted-origins") {
		t.Errorf("the startup error should name the flag; got:\n%s", out)
	}
}

// TestCrossOrigin_PreflightAuthorizesTheParameterHeader closes the gap between
// the header the server demands and the one a browser is allowed to send.
//
// TestGate_ParamHeaderMatchesTheDocumentedName sets Mcp-Param-Action directly,
// the way curl does, and so cannot see whether a browser would have been
// permitted to send it. A browser asks first: it lists the header in
// Access-Control-Request-Headers, and if the response's
// Access-Control-Allow-Headers does not cover it, the header is dropped from
// the real request — after which the server rejects the call with "header
// mismatch" for the absence of the very thing it refused to authorize.
//
// The allow-list once named three Mcp-Param-* headers no tool declares and
// omitted the only one that exists, which made every gitlab_execute_action call
// from a browser fail on the default surface while every curl-based test passed.
func TestCrossOrigin_PreflightAuthorizesTheParameterHeader(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--trusted-origins=https://claude.ai")

	got := srv.do(t, request{
		method: http.MethodOptions, path: "/mcp",
		headers: map[string]string{
			"Origin":                         "https://claude.ai",
			"Access-Control-Request-Method":  http.MethodPost,
			"Access-Control-Request-Headers": "mcp-param-action",
		},
	})

	if got.status != http.StatusNoContent && got.status != http.StatusOK {
		t.Fatalf("preflight status = %d, want 204 or 200", got.status)
	}
	// Compared token by token, not as a substring: a header list carrying
	// x-mcp-param-action contains the substring and authorizes nothing, and a
	// browser matches the header name itself.
	allowed := got.header.Get("Access-Control-Allow-Headers")
	authorized := false
	for header := range strings.SplitSeq(allowed, ",") {
		if strings.EqualFold(strings.TrimSpace(header), dynamictools.ExecuteActionHeaderName) {
			authorized = true
			break
		}
	}
	if !authorized {
		t.Errorf("Access-Control-Allow-Headers = %q; a browser would drop %s and the call would then be rejected for its absence",
			allowed, dynamictools.ExecuteActionHeaderName)
	}
}

// TestCrossOrigin_MCPEndpointRefusesUntrustedOriginOnEveryMethod pins the
// requirement that Origin is validated on all incoming connections to the MCP
// endpoint, not only on the methods a browser treats as unsafe.
//
// The process-wide guard delegates to the standard library, which exempts GET,
// HEAD and OPTIONS as safe methods. That exemption is deliberate and is what
// keeps /health and the server card publicly fetchable —
// TestCrossOrigin_SafeMethodsStayReachable pins it — but on the MCP endpoint it
// meant a cross-origin GET opened a real SSE stream against a stateful session
// instead of being refused. The two intents are separate, so they are pinned
// separately: this test asserts the endpoint refuses, that one asserts the
// public routes do not.
func TestCrossOrigin_MCPEndpointRefusesUntrustedOriginOnEveryMethod(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--stateless=false")

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			got := srv.do(t, request{
				method: method, path: "/mcp",
				headers: map[string]string{
					"Origin":         "https://evil.example",
					"Sec-Fetch-Site": "cross-site",
					"PRIVATE-TOKEN":  "glpat-whatever",
					"Accept":         "application/json, text/event-stream",
				},
			})
			if got.status != http.StatusForbidden {
				t.Errorf("%s with an untrusted Origin: status = %d, want 403", method, got.status)
			}
		})
	}

	// A preflight must still be answered rather than refused: the browser
	// strips credentials from it, and a 403 here is not something a client can
	// interpret.
	preflight := srv.do(t, request{
		method: http.MethodOptions, path: "/mcp",
		headers: map[string]string{
			"Origin":                        "https://evil.example",
			"Access-Control-Request-Method": http.MethodPost,
		},
	})
	if preflight.status == http.StatusForbidden {
		t.Error("a preflight was refused on its Origin; the browser cannot interpret that")
	}

	// And the deployment's own host is still same-origin.
	same := srv.do(t, request{
		method: http.MethodPost, path: "/mcp", body: toolsListBody,
		headers: map[string]string{
			"Origin":        srv.baseURL,
			"PRIVATE-TOKEN": "glpat-whatever",
		},
	})
	if same.status == http.StatusForbidden {
		t.Errorf("a same-origin request was refused: status = %d", same.status)
	}
}
