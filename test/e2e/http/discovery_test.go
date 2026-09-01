//go:build httpe2e

package httpe2e

import (
	"net/http"
	"strings"
	"testing"
)

// TestProtectedResourceMetadata_BehavesLikeAnHTTPDocument pins the rules that
// apply to the discovery document because it is a document, not because it is
// OAuth.
//
// It is served on a public endpoint, and the things that reach for a public URL
// are not all MCP clients: health checks, link checkers and CDN origin probes
// send HEAD, and tooling that asks what a resource supports reads Allow. The
// SDK's handler answers everything that is not GET with a 405 carrying neither.
//
// Two MUSTs are involved. "All general-purpose servers MUST support the methods
// GET and HEAD" (RFC 9110 §9.1), and "the origin server MUST generate an Allow
// header field in a 405" (§15.5.6). Access-Control-Allow-Methods is not a
// substitute for Allow: it answers a CORS preflight, which is a different
// question from what the resource supports.
func TestProtectedResourceMetadata_BehavesLikeAnHTTPDocument(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil,
		"--gitlab-url="+gitlab.url,
		"--auth-mode=oauth",
		"--public-url=https://mcp.example.invalid/gitlab",
	)

	const path = "/.well-known/oauth-protected-resource"

	t.Run("GET returns the document and allows it to be cached", func(t *testing.T) {
		got := srv.do(t, request{method: http.MethodGet, path: path})
		if got.status != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", got.status, got.body)
		}
		if !strings.Contains(got.body, "resource") {
			t.Errorf("body does not look like protected-resource metadata: %s", got.body)
		}
		// Every client fetches this on every discovery attempt, and it changes
		// only when the operator restarts with different flags.
		if cc := got.header.Get("Cache-Control"); !strings.Contains(cc, "max-age") {
			t.Errorf("Cache-Control = %q, want a positive lifetime for a public document", cc)
		}
	})

	t.Run("HEAD is answered, not refused", func(t *testing.T) {
		got := srv.do(t, request{method: http.MethodHead, path: path})
		if got.status != http.StatusOK {
			t.Fatalf("status = %d, want 200: HEAD is not optional for a general-purpose server", got.status)
		}
		if got.body != "" {
			t.Errorf("HEAD returned a body of %d bytes", len(got.body))
		}
	})

	t.Run("a method the document does not support says which ones it does", func(t *testing.T) {
		got := srv.do(t, request{method: http.MethodPut, path: path, body: `{}`})
		if got.status != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", got.status)
		}
		allow := got.header.Get("Allow")
		if allow == "" {
			t.Fatal("the 405 carries no Allow header, so a client cannot learn what to send instead")
		}
		for _, method := range []string{"GET", "HEAD"} {
			t.Run(method, func(t *testing.T) {
				if !strings.Contains(allow, method) {
					t.Errorf("Allow = %q, want it to list %s", allow, method)
				}
			})
		}
	})

	t.Run("the path-inserted derivation behaves the same", func(t *testing.T) {
		// RFC 9728 §3 gives two derivations, and a client that computed the
		// path-carrying one must not meet different rules.
		got := srv.do(t, request{method: http.MethodHead, path: path + "/gitlab"})
		if got.status != http.StatusOK {
			t.Errorf("status = %d, want 200 for the path-inserted form", got.status)
		}
	})
}
