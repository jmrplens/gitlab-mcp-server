//go:build httpe2e

package httpe2e

import (
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestServerCard_TheURLItAdvertisesServesTheCard follows the card the way a
// client that found it does: read `remotes[0].url`, append the suffix the
// server-card extension reserves, and fetch that.
//
// The extension puts the card at `<streamable-http-url>/server-card`, so the
// endpoint a card advertises implies where the card itself lives. Before the
// card was mounted after /mcp as well, a deployment published as
// https://host/mcp advertised that URL and answered 404 at
// https://host/mcp/server-card, because a public URL whose path is /mcp names
// no prefix and the card was at /server-card alone. The public deployment
// answered 404 at https://mcp.jmrp.io/gitlab/mcp/server-card for the same
// reason, which is the card URL of its endpoint's /mcp form. Each row starts a
// real binary with that --public-url and sends the request with the advertised
// host in Host, since that is what arrives through the proxy the URL implies.
func TestServerCard_TheURLItAdvertisesServesTheCard(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")

	tests := []struct {
		publicURL string
		// endpointForms are the other paths the MCP endpoint answers at under
		// this URL, each of which has a card of its own by the same rule.
		endpointForms []string
	}{
		{publicURL: "https://mcp.example.invalid", endpointForms: []string{"/mcp"}},
		{publicURL: "https://mcp.example.invalid/mcp"},
		{publicURL: "https://mcp.example.invalid/gitlab", endpointForms: []string{"/gitlab/mcp"}},
	}

	for _, tt := range tests {
		t.Run(tt.publicURL, func(t *testing.T) {
			srv := startServer(t, nil, "--gitlab-url="+gitlab.url, "--public-url="+tt.publicURL)

			root := srv.do(t, request{method: http.MethodGet, path: "/server-card"})
			if root.status != http.StatusOK {
				t.Fatalf("GET /server-card = %d, want 200: %s", root.status, root.body)
			}
			advertised := advertisedRemote(t, root.body)
			if advertised.String() != tt.publicURL {
				t.Errorf("remotes[0].url = %q, want the --public-url %q", advertised, tt.publicURL)
			}

			for _, endpoint := range append([]string{advertised.Path}, tt.endpointForms...) {
				t.Run(endpoint+"/server-card", func(t *testing.T) {
					assertCardServedAt(t, srv, advertised.Host, endpoint+"/server-card", root.body)
				})
			}
		})
	}
}

// advertisedRemote reads `remotes[0].url` out of a card, failing the test when
// the card carries none: a deployment started with --public-url must say where
// it is reached.
func advertisedRemote(t *testing.T, card string) *url.URL {
	t.Helper()

	var parsed struct {
		Remotes []struct {
			URL string `json:"url"`
		} `json:"remotes"`
	}
	if err := json.Unmarshal([]byte(card), &parsed); err != nil {
		t.Fatalf("the card is not JSON: %v\n%s", err, card)
	}
	if len(parsed.Remotes) == 0 {
		t.Fatalf("a deployment with --public-url published no remotes: %s", card)
	}
	advertised, err := url.Parse(parsed.Remotes[0].URL)
	if err != nil {
		t.Fatalf("remotes[0].url %q: %v", parsed.Remotes[0].URL, err)
	}
	return advertised
}

// assertCardServedAt fetches a card URL the way a client that followed the card
// would, with the advertised host in Host and the card's media type in Accept,
// and holds the answer to the card the root path serves.
func assertCardServedAt(t *testing.T, srv *server, host, path, want string) {
	t.Helper()

	got := srv.do(t, request{
		method:  http.MethodGet,
		path:    path,
		headers: map[string]string{"Host": host, "Accept": "application/mcp-server-card+json"},
	})
	if got.status != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200: the endpoint answers there and its card does not: %s", path, got.status, got.body)
	}
	mediaType, _, err := mime.ParseMediaType(got.header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("Content-Type %q: %v", got.header.Get("Content-Type"), err)
	}
	if mediaType != "application/mcp-server-card+json" {
		t.Errorf("Content-Type = %q, want the card's own media type", mediaType)
	}
	if got.body != want {
		t.Errorf("the card at %s differs from the one at /server-card", path)
	}
}

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

	// The path RFC 9728 §3 derives from that identifier, and the only one this
	// deployment serves the document on. See
	// TestOAuth_MetadataAnswersOnlyItsOwnDerivedPath for why the bare form is
	// not this server's to answer.
	const path = "/.well-known/oauth-protected-resource/gitlab"

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

	t.Run("the CORS preflight is answered, not authenticated", func(t *testing.T) {
		// The document is mounted without a method restriction so the SDK
		// handler answers OPTIONS itself. A "GET "-restricted pattern would
		// send the preflight to the catch-all instead, locking out the
		// browser-based clients that fetch this cross-origin.
		got := srv.do(t, request{
			method:  http.MethodOptions,
			path:    path,
			headers: map[string]string{"Origin": "https://claude.ai", "Access-Control-Request-Method": "GET"},
		})
		if got.status != http.StatusNoContent && got.status != http.StatusOK {
			t.Fatalf("status = %d, want the preflight answered rather than refused", got.status)
		}
		if allowed := got.header.Get("Access-Control-Allow-Origin"); allowed == "" {
			t.Error("the preflight carries no Access-Control-Allow-Origin, so a browser drops the fetch")
		}
	})
}

// TestProtectedResourceMetadata_CarriesAValidator covers what the wrapper adds
// on top of the lifetime the document already published.
//
// max-age says how long a client may reuse a copy and gives it nothing to say
// once that runs out, so the fetch after expiry is a full one however
// unchanged the document is. The tag is what turns that into a conditional
// request the origin can answer with 304.
//
// It is a test of its own rather than one more leg of the one above because
// the question is a different shape: that one asks what a document owes any
// client on a single request, this one is about the round trip between two.
func TestProtectedResourceMetadata_CarriesAValidator(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := startServer(t, nil,
		"--gitlab-url="+gitlab.url,
		"--auth-mode=oauth",
		"--public-url=https://mcp.example.invalid/gitlab",
	)
	const path = "/.well-known/oauth-protected-resource/gitlab"

	first := srv.do(t, request{method: http.MethodGet, path: path})
	tag := first.header.Get("ETag")
	if tag == "" {
		t.Fatal("no ETag on the discovery document, so an expired copy can only be re-downloaded")
	}

	t.Run("a fetch carrying the tag is answered 304", func(t *testing.T) {
		got := srv.do(t, request{
			method:  http.MethodGet,
			path:    path,
			headers: map[string]string{"If-None-Match": tag},
		})
		if got.status != http.StatusNotModified {
			t.Fatalf("status = %d, want 304", got.status)
		}
		if got.body != "" {
			t.Errorf("304 carried %d bytes of body", len(got.body))
		}
		// RFC 9110 section 15.4.5: without the directives a 200 would have
		// carried, a client cannot tell how long the copy it was just told to
		// keep stays fresh.
		if got.header.Get("Cache-Control") != first.header.Get("Cache-Control") {
			t.Errorf("Cache-Control = %q on the 304 and %q on the 200",
				got.header.Get("Cache-Control"), first.header.Get("Cache-Control"))
		}
	})

	t.Run("a fetch carrying an unknown tag gets the document", func(t *testing.T) {
		got := srv.do(t, request{
			method:  http.MethodGet,
			path:    path,
			headers: map[string]string{"If-None-Match": `"0123456789abcdef0123456789abcdef"`},
		})
		if got.status != http.StatusOK || got.body != first.body {
			t.Errorf("status = %d, want the document back with 200", got.status)
		}
	})
}
