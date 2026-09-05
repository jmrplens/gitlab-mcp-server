//go:build httpe2e

// oauth_test.go pins --auth-mode=oauth over real HTTP: what a rejection tells
// the client, what it costs upstream, and what the metadata endpoint says.
//
// Every case here corresponds to a bug that shipped. OAuth mode never
// authenticated anyone until v2.7.4 because the pooled client sent the token in
// the wrong header; the challenge carried no RFC 6750 error code, so a client
// could not tell "reauthorize" from "ask for more scope"; a throttled GitLab
// was reported as an invalid token, which makes a well-behaved client discard a
// good credential; and an invalid token was relayed upstream on every retry.
package httpe2e

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
)

const publicURL = "https://mcp.example.com"

// oauthServer starts the binary in OAuth mode against a fake GitLab.
func oauthServer(t *testing.T, gitlabURL string, extra ...string) *server {
	t.Helper()
	flags := append([]string{
		"--auth-mode=oauth",
		"--gitlab-url=" + gitlabURL,
		"--public-url=" + publicURL,
	}, extra...)
	return startServer(t, nil, flags...)
}

// TestOAuth_MetadataAdvertisesTheScopesItAccepts verifies the RFC 9728
// document, including the scopes a client may authorize with.
//
// A client reads scopes_supported to decide what to ask GitLab for. A
// read-only deployment advertising "api" would make every user grant write
// access it can never use; a writing deployment advertising only "api" would
// leave a client that deliberately wants a credential which cannot break
// anything no documented way to ask for one — and such a token IS accepted,
// served the read-only surface.
func TestOAuth_MetadataAdvertisesTheScopesItAccepts(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")

	for _, tc := range []struct {
		name  string
		flags []string
		want  []string
	}{
		{"writes possible", nil, []string{"api", "read_api"}},
		{"read-only", []string{"--read-only"}, []string{"read_api"}},
		{"safe mode", []string{"--safe-mode"}, []string{"read_api"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := oauthServer(t, gitlab.url, tc.flags...)

			got := srv.do(t, request{method: http.MethodGet, path: "/.well-known/oauth-protected-resource"})
			if got.status != http.StatusOK {
				t.Fatalf("status = %d, want %d", got.status, http.StatusOK)
			}

			var meta struct {
				Resource        string   `json:"resource"`
				AuthServers     []string `json:"authorization_servers"`
				ScopesSupported []string `json:"scopes_supported"`
				ResourceName    string   `json:"resource_name"`
			}
			if err := json.Unmarshal([]byte(got.body), &meta); err != nil {
				t.Fatalf("metadata is not JSON: %v\n%s", err, got.body)
			}
			if meta.Resource != publicURL {
				t.Errorf("resource = %q, want %q", meta.Resource, publicURL)
			}
			if len(meta.AuthServers) != 1 || meta.AuthServers[0] != gitlab.url {
				t.Errorf("authorization_servers = %v, want [%q]", meta.AuthServers, gitlab.url)
			}
			if !slices.Equal(meta.ScopesSupported, tc.want) {
				t.Errorf("scopes_supported = %v, want %v", meta.ScopesSupported, tc.want)
			}
			if meta.ResourceName == "" {
				t.Error("resource_name is RECOMMENDED by RFC 9728 and lets a consent screen name this resource")
			}
		})
	}
}

// TestOAuth_MetadataAnswersItsOwnPreflight verifies that the metadata endpoint
// is fetchable cross-origin by a browser-based client.
//
// It was once mounted GET-only, which sent the preflight to the catch-all gate
// and got it 401 — locking out exactly the clients that discover a server this
// way.
func TestOAuth_MetadataAnswersItsOwnPreflight(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := oauthServer(t, gitlab.url)

	got := srv.do(t, request{
		method: http.MethodOptions,
		path:   "/.well-known/oauth-protected-resource",
		headers: map[string]string{
			"Origin":                        "https://claude.ai",
			"Access-Control-Request-Method": http.MethodGet,
		},
	})

	if got.status == http.StatusUnauthorized {
		t.Fatal("the metadata preflight was answered 401; browser clients cannot discover this server")
	}
	if got.header.Get("Access-Control-Allow-Origin") == "" {
		t.Error("the metadata preflight carries no Access-Control-Allow-Origin")
	}
}

// TestOAuth_MissingCredentialChallengeHasNoErrorCode verifies RFC 6750 §3.1: a
// challenge answering a request that carried no credential must not name an
// error, because the client has not got anything wrong yet.
func TestOAuth_MissingCredentialChallengeHasNoErrorCode(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := oauthServer(t, gitlab.url)

	got := srv.do(t, mcpPOST(nil))

	if got.status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", got.status, http.StatusUnauthorized)
	}
	challenge := got.header.Get("WWW-Authenticate")
	if strings.Contains(challenge, "error=") {
		t.Errorf("challenge names an error for a request that carried no credential: %q", challenge)
	}
	if !strings.Contains(challenge, "resource_metadata=") {
		t.Errorf("challenge must point at the metadata URL so a client can discover the authorization server: %q", challenge)
	}
	// The MCP authorization specification says a server SHOULD name the
	// scope in the challenge, "following the principle of least privilege
	// and preventing clients from requesting excessive permissions". A
	// client that reads the header and stops there would otherwise guess,
	// and the guess that costs it — asking for every scope the
	// authorization server advertises — is answered by GitLab with
	// invalid_scope.
	if !strings.Contains(challenge, `scope="api"`) {
		t.Errorf("challenge %q does not name the scope a client should request", challenge)
	}
}

// TestOAuth_ChallengeIsReadableCrossOrigin verifies that a browser-based
// client can actually READ the challenge it is sent.
//
// WWW-Authenticate is an unsafelisted response header, so without naming it
// in Access-Control-Expose-Headers the browser hides it: the client gets the
// 401 but cannot see the resource_metadata URL, and automatic discovery — the
// entire point of the challenge — fails for exactly the audience CORS serves.
func TestOAuth_ChallengeIsReadableCrossOrigin(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := oauthServer(t, gitlab.url, "--trusted-origins=https://claude.ai")

	got := srv.do(t, mcpPOST(map[string]string{"Origin": "https://claude.ai"}))

	if got.status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", got.status, http.StatusUnauthorized)
	}
	exposed := got.header.Get("Access-Control-Expose-Headers")
	if !strings.Contains(exposed, "WWW-Authenticate") {
		t.Errorf("Access-Control-Expose-Headers = %q; a browser client cannot read the challenge without it", exposed)
	}
}

// TestOAuth_UnknownPathIs404NotAChallenge verifies that routing happens before
// authentication.
//
// Every unknown path used to answer 401, because the auth gate was the
// catch-all handler. That tells a scanner probing
// /.well-known/oauth-authorization-server that a protected metadata document
// lives there when it simply is not served, and leaves nothing able to
// distinguish "exists but needs a token" from "is not here".
func TestOAuth_UnknownPathIs404NotAChallenge(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := oauthServer(t, gitlab.url)

	for _, path := range []string{
		"/.well-known/oauth-authorization-server",
		"/.well-known/mcp",
		"/a-path-this-server-does-not-serve",
	} {
		t.Run(path, func(t *testing.T) {
			got := srv.do(t, request{method: http.MethodPost, path: path, body: toolsListBody})

			if got.status != http.StatusNotFound {
				t.Errorf("status = %d, want %d for a path this server does not serve", got.status, http.StatusNotFound)
			}
			if challenge := got.header.Get("WWW-Authenticate"); challenge != "" {
				t.Errorf("a 404 must not carry an authentication challenge, got %q", challenge)
			}
			if got.header.Get("X-Content-Type-Options") != "nosniff" {
				t.Error("the security headers apply to a 404 like any other response")
			}
		})
	}
}

// TestOAuth_ServerCardFollowsTheAuthMode verifies that the card describes THIS
// deployment rather than the binary.
//
// It declared header-token whatever --auth-mode said, so an oauth deployment
// published a card contradicting its own 401 challenge, and a directory
// rendering the card told users to send a header the endpoint refuses.
func TestOAuth_ServerCardFollowsTheAuthMode(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := oauthServer(t, gitlab.url)

	// Both locations: /server-card is what the server-card extension
	// recommends, and the .well-known path its earlier draft did.
	for _, path := range []string{"/server-card", "/.well-known/mcp/server-card.json"} {
		t.Run(path, func(t *testing.T) {
			got := srv.do(t, request{method: http.MethodGet, path: path})
			if got.status != http.StatusOK {
				t.Fatalf("status = %d, want %d", got.status, http.StatusOK)
			}

			var card struct {
				Authentication struct {
					Required         bool     `json:"required"`
					Schemes          []string `json:"schemes"`
					ResourceMetadata string   `json:"resourceMetadata"`
					Scopes           []string `json:"scopes"`
				} `json:"authentication"`
			}
			if err := json.Unmarshal([]byte(got.body), &card); err != nil {
				t.Fatalf("card is not JSON: %v\n%s", err, got.body)
			}
			if !slices.Equal(card.Authentication.Schemes, []string{"oauth2"}) {
				t.Errorf("schemes = %v, want [oauth2] in oauth mode", card.Authentication.Schemes)
			}
			if !card.Authentication.Required {
				t.Error("required = false, want true")
			}
			if card.Authentication.ResourceMetadata == "" {
				t.Error("the card should point at the RFC 9728 document in oauth mode")
			}
			if !slices.Contains(card.Authentication.Scopes, "api") {
				t.Errorf("scopes = %v, want it to name the scope this deployment recommends", card.Authentication.Scopes)
			}
		})
	}
}

// TestOAuth_RejectedTokenSaysInvalidToken verifies that a refused credential is
// described as such, so a client knows to reauthorize rather than to ask for
// more scope or simply retry.
func TestOAuth_RejectedTokenSaysInvalidToken(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := oauthServer(t, gitlab.url)

	got := srv.do(t, mcpPOST(map[string]string{"Authorization": "Bearer gloas-rejected"}))

	if got.status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", got.status, http.StatusUnauthorized)
	}
	challenge := got.header.Get("WWW-Authenticate")
	for _, want := range []string{`error="invalid_token"`, "error_description=", "resource_metadata="} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(challenge, want) {
				t.Errorf("challenge %q is missing %s", challenge, want)
			}
		})
	}
}

// TestOAuth_RejectedTokenIsNotRelayedUpstreamEveryTime verifies the
// amplification defense: a replayed bad token is answered from memory.
//
// Without it, unauthenticated traffic anyone can generate becomes load on
// someone else's API, and on gitlab.com rate-limit pressure charged to this
// server's address, where it lands on the legitimate users sharing it.
func TestOAuth_RejectedTokenIsNotRelayedUpstreamEveryTime(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := oauthServer(t, gitlab.url)

	const attempts = 5
	for range attempts {
		got := srv.do(t, mcpPOST(map[string]string{"Authorization": "Bearer gloas-same-bad-token"}))
		if got.status != http.StatusUnauthorized {
			t.Fatalf("every attempt with a rejected token should be 401, got %d", got.status)
		}
	}

	if calls := gitlab.calls(); calls >= attempts {
		t.Errorf("%d upstream verification calls for %d attempts; the rejection cache is not absorbing replays", calls, attempts)
	}
}

// TestOAuth_ThrottledUpstreamIsNotBlamedOnTheToken verifies the classification
// that keeps a GitLab outage from looking like a credential problem.
//
// Reporting a 429 as invalid_token makes a well-behaved MCP client discard a
// perfectly good credential and start a fresh authorization flow — generating
// more upstream traffic at exactly the moment the instance asked for less, and
// asking the user to re-approve an application that was never the problem.
func TestOAuth_ThrottledUpstreamIsNotBlamedOnTheToken(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusTooManyRequests, "")
	srv := oauthServer(t, gitlab.url)

	got := srv.do(t, mcpPOST(map[string]string{"Authorization": "Bearer gloas-good"}))

	if got.status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d — a throttled GitLab is not a verdict on the token", got.status, http.StatusServiceUnavailable)
	}
	if got.header.Get("Retry-After") == "" {
		t.Error("a 503 must tell the caller when to come back")
	}
	if challenge := got.header.Get("WWW-Authenticate"); challenge != "" {
		t.Errorf("an upstream failure must not challenge the client to reauthorize, got %q", challenge)
	}
	if !strings.Contains(got.body, "not been rejected") {
		t.Errorf("the message should say the token was not rejected, got %s", got.body)
	}
}

// TestOAuth_PrivateTokenHeaderIsNotAccepted verifies that OAuth mode accepts
// exactly what its challenge advertises. Preferring a PRIVATE-TOKEN a request
// also carried would execute as an identity the bearer layer never verified.
func TestOAuth_PrivateTokenHeaderIsNotAccepted(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusOK, `{"id":7,"username":"someone"}`)
	srv := oauthServer(t, gitlab.url)

	got := srv.do(t, mcpPOST(map[string]string{"PRIVATE-TOKEN": "glpat-legacy-only"}))

	if got.status != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d — oauth mode is Bearer-only", got.status, http.StatusUnauthorized)
	}
}

// TestOAuth_RequiresPublicURL verifies that the server refuses to start rather
// than run without the RFC 9728 resource identifier, which cannot be derived
// from a bind address.
func TestOAuth_RequiresPublicURL(t *testing.T) {
	bin := serverBinary(t)
	port := freePort(t)

	out, err := runServerExpectingExit(t, bin,
		"--http", "--http-addr=127.0.0.1:"+itoa(port),
		"--auth-mode=oauth",
		"--gitlab-url=https://gitlab.example.com",
	)
	if err == nil {
		t.Fatal("oauth mode started with no --public-url")
	}
	if !strings.Contains(out, "public-url") {
		t.Errorf("the startup error should name the missing flag; got:\n%s", out)
	}
}

// TestOAuth_ConfigurableFromTheEnvironmentAlone verifies that a deployment with
// no command line — a container, a compose stack — can enable OAuth.
//
// AUTH_MODE was read from the environment and PUBLIC_URL was not, so such a
// deployment started, read the mode, and then refused to run demanding a flag
// it had no way to pass. The reference documented PUBLIC_URL as the flag's
// equivalent the whole time.
func TestOAuth_ConfigurableFromTheEnvironmentAlone(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := startServer(t, map[string]string{
		"AUTH_MODE":       "oauth",
		"GITLAB_URL":      gitlab.url,
		"PUBLIC_URL":      publicURL,
		"TRUSTED_ORIGINS": trustedOrigin,
	})

	got := srv.do(t, request{method: http.MethodGet, path: "/.well-known/oauth-protected-resource"})
	if got.status != http.StatusOK {
		t.Fatalf("metadata status = %d, want %d — the server did not come up in oauth mode from the environment", got.status, http.StatusOK)
	}
	if !strings.Contains(got.body, publicURL) {
		t.Errorf("PUBLIC_URL did not reach the resource identifier: %s", got.body)
	}

	// TRUSTED_ORIGINS must reach the cross-origin decision from the
	// environment too.
	cors := srv.do(t, request{
		method: http.MethodOptions, path: "/mcp",
		headers: map[string]string{
			"Origin":                        trustedOrigin,
			"Access-Control-Request-Method": http.MethodPost,
		},
	})
	if cors.header.Get("Access-Control-Allow-Origin") != trustedOrigin {
		t.Errorf("TRUSTED_ORIGINS did not reach the cross-origin decision; preflight headers: %v", cors.header)
	}
}

// TestOAuth_ReadAPITokenIsAdmitted pins the change this branch exists for, at
// the only level that proves it: the real binary, over real HTTP.
//
// A deployment that can write used to demand `api` at the door, so a token
// carrying only read_api was refused on the FIRST call — before tools/list,
// with JSON-RPC code -40300. That is what blocked a read-only OAuth
// application outright. Admission now asks only for what every action needs.
//
// Without this test the fix could be reverted and every other case in this
// module would still pass, because none of them uses a token whose scopes
// GitLab actually reports.
func TestOAuth_ReadAPITokenIsAdmitted(t *testing.T) {
	gitlab := startScopedFakeGitLab(t, map[string][]string{
		"gloas-read-only": {"read_api"},
		"gloas-full":      {"api"},
		"gloas-no-api":    {"read_user"},
	})
	srv := oauthServer(t, gitlab.url)

	tests := []struct {
		name  string
		token string
		want  int
	}{
		{name: "read_api is admitted", token: "gloas-read-only", want: http.StatusOK},
		{name: "api is admitted", token: "gloas-full", want: http.StatusOK},
		// The only credential still refused: one carrying no GitLab API
		// scope at all. 403, not 401 — the token is genuine.
		{name: "no API scope is forbidden", token: "gloas-no-api", want: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := srv.do(t, mcpPOST(map[string]string{"Authorization": "Bearer " + tt.token}))

			if got.status != tt.want {
				t.Fatalf("status = %d, want %d: %s", got.status, tt.want, got.body)
			}
			if tt.want != http.StatusOK {
				return
			}
			// Admitted means it reached the MCP layer and got a tool list —
			// not merely that the door opened.
			if !strings.Contains(got.body, `"tools"`) {
				t.Errorf("an admitted token must reach tools/list; body: %s", got.body)
			}
		})
	}
}

// TestOAuth_ReadAPITokenGetsAReadOnlySurface is the other half: admitting the
// token is only safe because what it may DO is settled per action.
//
// The deployment runs the default dynamic surface, where the tool COUNT does
// not move — there are two tools either way — so the question is which actions
// each credential can reach through them. An api token must find a mutating
// action; a read_api token must not. If both found it, the door would have
// been opened without the office being locked.
func TestOAuth_ReadAPITokenGetsAReadOnlySurface(t *testing.T) {
	gitlab := startScopedFakeGitLab(t, map[string][]string{
		"gloas-read-only": {"read_api"},
		"gloas-full":      {"api"},
	})
	srv := oauthServer(t, gitlab.url)

	findAction := func(token, query string) string {
		t.Helper()
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitlab_find_action","arguments":{"query":"` + query + `"},"_meta":{"io.modelcontextprotocol/protocolVersion":"` + protocolVersion + `","io.modelcontextprotocol/clientCapabilities":{}}}}`
		got := srv.do(t, request{
			method:  http.MethodPost,
			path:    "/mcp",
			body:    body,
			headers: map[string]string{"Authorization": "Bearer " + token},
		})
		if got.status != http.StatusOK {
			t.Fatalf("find_action(%q) for %s = %d: %s", query, token, got.status, got.body)
		}
		return got.body
	}

	// A write action, named the way the catalog names it.
	const mutating = "issue_create"

	full := findAction("gloas-full", "create issue")
	readOnly := findAction("gloas-read-only", "create issue")

	if !strings.Contains(full, mutating) {
		t.Fatalf("an api token must be able to find %q; got: %s", mutating, truncate(full))
	}
	if strings.Contains(readOnly, mutating) {
		t.Errorf("a read_api token reached the mutating action %q — the per-action write gate is not applied: %s",
			mutating, truncate(readOnly))
	}
}

// TestOAuth_MultiInstanceAllowList pins the allow-list that makes a
// per-request instance safe in oauth mode.
//
// The server verifies the bearer token AGAINST the instance, so a free-form
// GITLAB-URL header would let a caller name a host of their own and be handed
// the token. Publishing several instances turns the header into a choice among
// them; anything else is refused rather than quietly served from the default.
func TestOAuth_MultiInstanceAllowList(t *testing.T) {
	primary := startScopedFakeGitLab(t, map[string][]string{"gloas-full": {"api"}})
	secondary := startScopedFakeGitLab(t, map[string][]string{"gloas-full": {"api"}})

	srv := startServer(t, nil,
		"--auth-mode=oauth",
		"--gitlab-url="+primary.url,
		"--gitlab-url="+secondary.url,
		"--public-url="+publicURL,
	)

	headers := func(extra map[string]string) map[string]string {
		h := map[string]string{"Authorization": "Bearer gloas-full"}
		maps.Copy(h, extra)
		return h
	}

	t.Run("no header is refused rather than served from a default", func(t *testing.T) {
		primaryBefore, secondaryBefore := primary.calls(), secondary.calls()
		got := srv.do(t, mcpPOST(headers(nil)))
		if got.status != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d: %s", got.status, http.StatusBadRequest, got.body)
		}
		if !strings.Contains(got.body, primary.url) || !strings.Contains(got.body, secondary.url) {
			t.Errorf("the refusal must name the instances this deployment publishes; got: %s", got.body)
		}
		// Both counters, not just the first instance's. Watching the default
		// alone catches nothing but a regression back to "resolve to the
		// first published"; last published, round robin, or verifying against
		// each instance in turn all put the bearer on the wire just as surely
		// while leaving that one counter still.
		if primary.calls() != primaryBefore {
			t.Error("a request naming no instance still reached the first published one: the bearer went on the wire before the caller chose where")
		}
		if secondary.calls() != secondaryBefore {
			t.Error("a request naming no instance still reached the second published one: the bearer went on the wire before the caller chose where")
		}
	})

	t.Run("the header selects another published instance", func(t *testing.T) {
		before := secondary.calls()
		got := srv.do(t, mcpPOST(headers(map[string]string{"GITLAB-URL": secondary.url})))
		if got.status != http.StatusOK {
			t.Fatalf("status = %d, want %d: %s", got.status, http.StatusOK, got.body)
		}
		if secondary.calls() == before {
			t.Error("the selected instance was never contacted; the header was ignored")
		}
	})

	t.Run("an unpublished instance is refused, not silently replaced", func(t *testing.T) {
		hostile := startScopedFakeGitLab(t, map[string][]string{"gloas-full": {"api"}})
		before := hostile.calls()

		got := srv.do(t, mcpPOST(headers(map[string]string{"GITLAB-URL": hostile.url})))
		if got.status != http.StatusForbidden {
			t.Fatalf("status = %d, want %d: %s", got.status, http.StatusForbidden, got.body)
		}
		// The point of the refusal: the bearer token must never be sent to
		// an instance the operator did not publish.
		if hostile.calls() != before {
			t.Error("the token was sent to an unpublished instance")
		}
	})

	// Both instances are advertised so a client can discover which ones it
	// may select.
	t.Run("the metadata publishes every instance", func(t *testing.T) {
		got := srv.do(t, request{method: http.MethodGet, path: "/.well-known/oauth-protected-resource"})
		if got.status != http.StatusOK {
			t.Fatalf("status = %d, want %d", got.status, http.StatusOK)
		}
		var meta struct {
			AuthServers []string `json:"authorization_servers"`
		}
		if err := json.Unmarshal([]byte(got.body), &meta); err != nil {
			t.Fatalf("metadata is not JSON: %v", err)
		}
		if !slices.Contains(meta.AuthServers, primary.url) || !slices.Contains(meta.AuthServers, secondary.url) {
			t.Errorf("authorization_servers = %v, want both published instances", meta.AuthServers)
		}
	})
}

// TestOAuth_PlaintextInstanceIsRefusedAtStartup verifies that the https-only
// rule covers EVERY published instance, not just the first.
//
// The bearer token is forwarded to whichever instance the request selected, so
// one cleartext entry in the list puts a live credential on the wire (CWE-319)
// exactly as a cleartext deployment would. Validating only the first entry
// made the check trivially bypassable by adding a second.
func TestOAuth_PlaintextInstanceIsRefusedAtStartup(t *testing.T) {
	out, err := runServerExpectingExit(t, serverBinary(t),
		"--http", "--http-addr=127.0.0.1:0",
		"--auth-mode=oauth",
		"--gitlab-url=https://gitlab.example.com",
		"--gitlab-url=http://plaintext.example.com",
		"--public-url="+publicURL,
	)

	if err == nil {
		t.Fatalf("the server started with a plaintext instance in the list; output:\n%s", out)
	}
	if !strings.Contains(out, "requires an https --gitlab-url") {
		t.Errorf("a plaintext instance anywhere in the list must stop startup; output:\n%s", out)
	}
}

// TestOAuth_SatisfiesRemoteDirectoryAuthProbe pins the contract that external
// MCP directories verify, in one place, because each half of it is easy to
// break from a different direction.
//
// A directory probes authorization twice: at the `initialize` handshake and at
// a `tools/call` naming a tool that does not exist. Both must answer 401 with a
// challenge pointing at reachable metadata. The second probe is the fragile
// one: authentication currently wraps the MCP handler, so an unknown tool never
// reaches the dispatcher. Move the gate inside it and that request starts
// answering 200 with a JSON-RPC "unknown tool" error instead — the server would
// still be secure for real tools, but every directory would grade it as
// serving unauthenticated, because the probe it uses to detect authentication
// no longer sees any.
func TestOAuth_SatisfiesRemoteDirectoryAuthProbe(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := oauthServer(t, gitlab.url)

	probes := []struct {
		name string
		body string
	}{
		{
			name: "initialize handshake",
			body: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"directory-probe","version":"0"}}}`,
		},
		{
			name: "tools/call naming a tool that does not exist",
			body: `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"definitely_not_a_tool","arguments":{}}}`,
		},
	}

	for _, probe := range probes {
		t.Run(probe.name, func(t *testing.T) {
			got := srv.do(t, request{method: http.MethodPost, path: "/mcp", body: probe.body})

			if got.status != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d — a directory reads anything else as an unauthenticated server", got.status, http.StatusUnauthorized)
			}
			challenge := got.header.Get("WWW-Authenticate")
			if !strings.HasPrefix(challenge, "Bearer") {
				t.Errorf("challenge %q does not name the Bearer scheme", challenge)
			}
			if !strings.Contains(challenge, "resource_metadata=") {
				t.Errorf("challenge %q carries no resource_metadata pointer", challenge)
			}
		})
	}
}

// TestOAuth_DirectoryReadableMetadata verifies the metadata document a remote
// directory fetches after following the challenge, for both shapes RFC 9728 §3
// derives: a server that owns its hostname publishes at the bare well-known
// path, and one mounted under a path prefix publishes at the suffixed form.
//
// The path is read out of the challenge rather than written down here, because
// a readable document is only worth anything at the URL clients are told to
// fetch. Which paths do NOT answer is
// TestOAuth_MetadataAnswersOnlyItsOwnDerivedPath.
func TestOAuth_DirectoryReadableMetadata(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")

	deployments := []struct {
		name      string
		publicURL string
		want      string
	}{
		{"a server owning its hostname", publicURL, metadataBasePath},
		{"a server under a path prefix", publicURL + "/gitlab", metadataBasePath + "/gitlab"},
	}

	for _, deployment := range deployments {
		t.Run(deployment.name, func(t *testing.T) {
			srv := startServer(t, nil,
				"--gitlab-url="+gitlab.url,
				"--auth-mode=oauth",
				"--public-url="+deployment.publicURL,
			)

			path := metadataPathFromChallenge(t, srv)
			if path != deployment.want {
				t.Fatalf("challenge points at %q, want %q", path, deployment.want)
			}

			got := srv.do(t, request{method: http.MethodGet, path: path})
			if got.status != http.StatusOK {
				t.Fatalf("status = %d, want %d", got.status, http.StatusOK)
			}
			assertDirectoryMetadata(t, got.body)
		})
	}
}

// metadataBasePath is the RFC 9728 well-known path suffix, and the whole
// metadata path of a resource that has no path of its own.
const metadataBasePath = "/.well-known/oauth-protected-resource"

// metadataPathFromChallenge returns the path component of the resource_metadata
// URL an unauthenticated request is answered with, which is where a client
// actually goes looking for the document.
func metadataPathFromChallenge(t *testing.T, srv *server) string {
	t.Helper()

	got := srv.do(t, mcpPOST(nil))
	if got.status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 to get a challenge: %s", got.status, got.body)
	}
	challenge := got.header.Get("WWW-Authenticate")
	_, after, found := strings.Cut(challenge, `resource_metadata="`)
	if !found {
		t.Fatalf("challenge %q carries no resource_metadata pointer", challenge)
	}
	raw, _, found := strings.Cut(after, `"`)
	if !found {
		t.Fatalf("challenge %q has an unterminated resource_metadata value", challenge)
	}
	metadataURL, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("resource_metadata %q is not a URL: %v", raw, err)
	}
	return metadataURL.Path
}

// TestOAuth_MetadataAnswersOnlyItsOwnDerivedPath pins that the
// protected-resource document is served on exactly the one path RFC 9728 §3
// derives from this deployment's resource identifier, and on no other.
//
// It used to be mounted twice, the second time under a "/{rest...}" wildcard,
// so every suffix returned the same body. On a host running several MCP
// servers behind one name, asking about the neighbor at
// /.well-known/oauth-protected-resource/libgen got this deployment's document
// back: a configuration demanding OAuth against this deployment's GitLab,
// describing a server that is not it. RFC 9728 §3.3 tells a client to discard a
// document whose resource value is not the identifier it derived the URL from,
// so the extra paths were worthless to a conforming client and a false
// statement to a lax one.
//
// This is on the wire rather than on the handler because what changed is a
// property of the running server's routing table. The handler was right the
// whole time, and every happy path kept working while the wildcard was there,
// which is precisely why nothing noticed.
func TestOAuth_MetadataAnswersOnlyItsOwnDerivedPath(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")

	t.Run("a server under a path prefix answers only the suffixed form", func(t *testing.T) {
		srv := startServer(t, nil,
			"--gitlab-url="+gitlab.url,
			"--auth-mode=oauth",
			"--public-url="+publicURL+"/gitlab",
		)

		assertMetadataDocumentAt(t, srv, metadataBasePath+"/gitlab", publicURL+"/gitlab")

		foreign := []struct {
			name string
			path string
		}{
			{"a neighboring MCP server on the same host", metadataBasePath + "/libgen"},
			{"a suffix nobody deployed", metadataBasePath + "/zzz-invented"},
			// The path-less form is the document of the resource that IS the
			// origin. This deployment lives under /gitlab, so answering it
			// would be claiming the host root's identity.
			{"the host root's own document", metadataBasePath},
			// A trailing-slash mount would be a ServeMux subtree, which is the
			// wildcard again under another name.
			{"anything below the derived path", metadataBasePath + "/gitlab/deeper"},
			{"the derived path with a trailing slash", metadataBasePath + "/gitlab/"},
		}
		for _, probe := range foreign {
			t.Run(probe.name, func(t *testing.T) {
				assertNoMetadataAt(t, srv, probe.path)
			})
		}
	})

	t.Run("a server owning its hostname answers only the path-less form", func(t *testing.T) {
		srv := oauthServer(t, gitlab.url)

		assertMetadataDocumentAt(t, srv, metadataBasePath, publicURL)

		for _, path := range []string{metadataBasePath + "/gitlab", metadataBasePath + "/libgen"} {
			t.Run(path, func(t *testing.T) {
				assertNoMetadataAt(t, srv, path)
			})
		}
	})
}

// assertMetadataDocumentAt checks that the document is served at path and
// claims exactly the identifier the deployment was started with. Both halves
// matter: a client that fetched it compares the two and discards a mismatch.
func assertMetadataDocumentAt(t *testing.T, srv *server, path, wantResource string) {
	t.Helper()

	got := srv.do(t, request{method: http.MethodGet, path: path})
	if got.status != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200; the derived path must serve the document: %s", path, got.status, got.body)
	}
	var metadata struct {
		Resource string `json:"resource"`
	}
	if err := json.Unmarshal([]byte(got.body), &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if metadata.Resource != wantResource {
		t.Errorf("resource = %q, want %q", metadata.Resource, wantResource)
	}
}

// assertNoMetadataAt fails when a path this deployment does not own answers at
// all, and says so loudly when it answers with the document, which is the harm
// rather than the symptom.
func assertNoMetadataAt(t *testing.T, srv *server, path string) {
	t.Helper()

	got := srv.do(t, request{method: http.MethodGet, path: path})
	if strings.Contains(got.body, "authorization_servers") {
		t.Errorf("GET %s served the protected-resource document; this deployment is speaking for a resource that is not it: %s", path, got.body)
	}
	if got.status != http.StatusNotFound {
		t.Errorf("GET %s = %d, want 404", path, got.status)
	}
}

// assertDirectoryMetadata checks the four RFC 9728 fields a directory reads.
func assertDirectoryMetadata(t *testing.T, body string) {
	t.Helper()

	var metadata struct {
		Resource               string   `json:"resource"`
		AuthorizationServers   []string `json:"authorization_servers"`
		BearerMethodsSupported []string `json:"bearer_methods_supported"`
		ScopesSupported        []string `json:"scopes_supported"`
	}
	if err := json.Unmarshal([]byte(body), &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if metadata.Resource == "" {
		t.Error("resource is empty")
	}
	if len(metadata.AuthorizationServers) == 0 {
		t.Error("authorization_servers is empty")
	}
	if !slices.Contains(metadata.BearerMethodsSupported, "header") {
		t.Errorf("bearer_methods_supported = %v, must include \"header\"", metadata.BearerMethodsSupported)
	}
	if len(metadata.ScopesSupported) == 0 {
		t.Error("scopes_supported is empty")
	}
}

// TestOAuth_UnderScopedTokenIsForbiddenNotInvalid pins the answer to a token
// GitLab accepts as genuine but rejects for lacking a scope.
//
// Reporting it as an invalid token tells the client to discard a working
// credential and re-run its authorization flow, which returns the same
// under-scoped token and loops. Worse, the invalid-token path charges the
// caller's address an authentication failure and caches the token as rejected,
// so a client retrying with its genuine credential can lock its own address out
// of the endpoint. The server already answers this exact condition correctly
// when it notices the missing scope itself; this is the same fact arriving from
// GitLab instead.
//
// The distinction is only in the body — GitLab sends no WWW-Authenticate on a
// 403 — so the fake reproduces rack-oauth2's JSON error document.
func TestOAuth_UnderScopedTokenIsForbiddenNotInvalid(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"insufficient_scope","error_description":"requires higher privileges","scope":"api"}`))
	})
	gitlab := httptest.NewServer(mux)
	defer gitlab.Close()

	srv := oauthServer(t, gitlab.URL)

	// More attempts than the authentication-failure budget allows. If any of
	// them were charged, the address would be locked out and the status would
	// turn into 429 partway through.
	for attempt := range 12 {
		got := srv.do(t, request{
			method: http.MethodPost, path: "/mcp", body: toolsListBody,
			headers: map[string]string{"Authorization": "Bearer glpat-under-scoped"},
		})

		if got.status == http.StatusTooManyRequests {
			t.Fatalf("attempt %d was rate limited: a genuine token was charged against the address", attempt+1)
		}
		if got.status != http.StatusForbidden {
			t.Fatalf("attempt %d: status = %d, want 403 — an under-scoped token is not an invalid one", attempt+1, got.status)
		}
		challenge := got.header.Get("WWW-Authenticate")
		if !strings.Contains(challenge, `error="insufficient_scope"`) {
			t.Errorf("challenge %q does not say the scope is what is missing", challenge)
		}
		if strings.Contains(challenge, `error="invalid_token"`) {
			t.Errorf("challenge %q tells the client to discard a working credential", challenge)
		}
	}
}

// TestOAuth_PathCarryingPublicURLIsSelfConsistent pins discovery for a
// deployment whose resource identifier carries a path — the shape a server
// mounted behind a prefix actually has, and the one the suite never exercised
// because every other case here uses the bare origin.
//
// RFC 9728 §3.3 tells a client to discard metadata whose `resource` value is not
// identical to the URL it used, so the three things a client sees must agree:
// the metadata URL named in the challenge, the path that URL is served on, and
// the `resource` inside the document. If they drift, the official Go SDK falls
// back to treating the MCP host itself as the authorization server and opens a
// metadata URL that does not exist — a failure that looks like a broken
// authorization server rather than a misconfigured identifier.
func TestOAuth_PathCarryingPublicURLIsSelfConsistent(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := startServer(t, nil,
		"--gitlab-url="+gitlab.url,
		"--auth-mode=oauth",
		"--public-url=https://mcp.example.com/gitlab",
	)

	got := srv.do(t, mcpPOST(nil))
	if got.status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", got.status)
	}

	challenge := got.header.Get("WWW-Authenticate")
	const wantMetadata = "https://mcp.example.com/.well-known/oauth-protected-resource/gitlab"
	if !strings.Contains(challenge, `resource_metadata="`+wantMetadata+`"`) {
		t.Fatalf("challenge %q does not point at %s — the well-known segment goes between host and path", challenge, wantMetadata)
	}

	// The document must be served on the path the challenge named, and must
	// claim exactly the identifier the deployment was started with.
	doc := srv.do(t, request{method: http.MethodGet, path: "/.well-known/oauth-protected-resource/gitlab"})
	if doc.status != http.StatusOK {
		t.Fatalf("metadata status = %d, want 200 — the challenge points at a path that is not served", doc.status)
	}

	var metadata struct {
		Resource string `json:"resource"`
	}
	if err := json.Unmarshal([]byte(doc.body), &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if metadata.Resource != "https://mcp.example.com/gitlab" {
		t.Errorf("resource = %q, want the --public-url value; a client comparing the two discards this document", metadata.Resource)
	}
}

// TestOAuth_ResourceDocumentationIsOperatorConfigurable pins that
// --resource-documentation reaches the RFC 9728 metadata document.
//
// The plumbing existed end to end — a config field, a parameter on
// NewProtectedResourceHandler, a default — with no flag registering it, so the
// field was always empty and the documented flag did not exist. Passing it
// would have killed the process with "flag provided but not defined", which is
// what makes starting the real binary the test that matters here.
//
// It exists because the default points at this project's own OAuth setup guide,
// and an operator running their own OAuth application needs to point clients at
// a page describing *their* client ID and redirect URIs.
func TestOAuth_ResourceDocumentationIsOperatorConfigurable(t *testing.T) {
	const ownGuide = "https://example.com/our-own-oauth-app"

	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")

	t.Run("the operator's page is published", func(t *testing.T) {
		srv := startServer(t, nil,
			"--gitlab-url="+gitlab.url,
			"--auth-mode=oauth",
			"--public-url="+publicURL,
			"--resource-documentation="+ownGuide,
		)
		got := srv.do(t, request{method: http.MethodGet, path: "/.well-known/oauth-protected-resource"})
		if got.status != http.StatusOK {
			t.Fatalf("metadata status = %d, want %d: %s", got.status, http.StatusOK, got.body)
		}
		if !strings.Contains(got.body, ownGuide) {
			t.Errorf("resource_documentation did not carry the operator's page: %s", got.body)
		}
	})

	t.Run("omitting it keeps this project's guide", func(t *testing.T) {
		srv := startServer(t, nil,
			"--gitlab-url="+gitlab.url,
			"--auth-mode=oauth",
			"--public-url="+publicURL,
		)
		got := srv.do(t, request{method: http.MethodGet, path: "/.well-known/oauth-protected-resource"})
		if got.status != http.StatusOK {
			t.Fatalf("metadata status = %d, want %d: %s", got.status, http.StatusOK, got.body)
		}
		if !strings.Contains(got.body, "resource_documentation") {
			t.Errorf("no resource_documentation published at all: %s", got.body)
		}
		if strings.Contains(got.body, ownGuide) {
			t.Errorf("an unset flag published the test's value: %s", got.body)
		}
	})
}

// startApplicationFakeGitLab serves a GitLab whose /oauth/token/info names the
// OAuth application each bearer was minted for, which is the only recipient
// signal the instance offers: GitLab publishes no resource_indicators_supported,
// so the "or otherwise verify that they are the intended recipient" alternative
// the specification allows is a comparison against that uid (ADR-0019).
//
// The PAT endpoint answers 404 on purpose. A PAT answer ends introspection
// before the OAuth endpoint is asked, and the OAuth endpoint is the one that
// names the application. A token the map does not know is refused everywhere.
func startApplicationFakeGitLab(t *testing.T, applicationFor map[string]string) *fakeGitLab {
	t.Helper()

	var mu sync.Mutex
	calls := 0

	bearer := func(r *http.Request) string {
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		return token
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v4/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abcdef"}`))
	})
	mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		if _, known := applicationFor[bearer(r)]; !known {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"username":"pinned"}`))
	})
	mux.HandleFunc("/api/v4/personal_access_tokens/self", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/oauth/token/info", func(w http.ResponseWriter, r *http.Request) {
		uid, known := applicationFor[bearer(r)]
		if !known {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"scope":["api"],"expires_in":7200,"application":{"uid":%q}}`, uid)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &fakeGitLab{
		url: srv.URL,
		calls: func() int {
			mu.Lock()
			defer mu.Unlock()
			return calls
		},
	}
}

// TestOAuth_RecipientRefusalNamesTheDocumentationPage pins the refusal a pinned
// deployment gives a token from another OAuth application, on the wire.
//
// It is the one refusal whose remedy is not in the protocol. A missing
// credential says authorize, an invalid token says reauthorize, a missing scope
// names the scope; this one says "obtain a token from the application the
// operator published", and which application that is lives on the page the
// metadata document publishes as resource_documentation. RFC 6750's error_uri
// is the parameter for exactly that, and the challenge now carries it, so the
// holder is not sent to fetch a document to find a page.
//
// The page in the challenge is read out of the document rather than written
// down here: the two are the same page or the challenge is a lie, and that is
// the property under test. The refusal must also stay repeatable, because a
// refusal that charged the address would let one misconfigured client lock a
// shared address out for everybody holding a credential this deployment does
// admit.
func TestOAuth_RecipientRefusalNamesTheDocumentationPage(t *testing.T) {
	const ours, theirs = "5a4f1c0e-ours", "9b8e7d6c-theirs"
	gitlab := startApplicationFakeGitLab(t, map[string]string{
		"gloas-ours":   ours,
		"gloas-theirs": theirs,
	})

	deployments := []struct {
		name string
		// page is what the operator named, or "" for the project default.
		page string
	}{
		{name: "the operator's own page", page: "https://example.com/our-oauth-app"},
		{name: "the project page when none is named", page: ""},
	}

	for _, deployment := range deployments {
		t.Run(deployment.name, func(t *testing.T) {
			flags := []string{"--oauth-client-uid=" + ours}
			if deployment.page != "" {
				flags = append(flags, "--resource-documentation="+deployment.page)
			}
			srv := oauthServer(t, gitlab.url, flags...)

			page := publishedResourceDocumentation(t, srv)
			if deployment.page != "" && page != deployment.page {
				t.Fatalf("resource_documentation = %q, want the operator's %q", page, deployment.page)
			}

			t.Run("a token from another application is refused with the page", func(t *testing.T) {
				assertRecipientRefusal(t, srv, "gloas-theirs", page)
			})
			t.Run("a token from the pinned application is admitted", func(t *testing.T) {
				assertTokenAdmitted(t, srv, "gloas-ours")
			})
			t.Run("the refusal costs the address nothing", func(t *testing.T) {
				assertRefusalIsFree(t, srv, "gloas-theirs")
			})
		})
	}
}

// publishedResourceDocumentation reads the page the metadata document names,
// which is the page a recipient refusal must name too.
func publishedResourceDocumentation(t *testing.T, srv *server) string {
	t.Helper()

	got := srv.do(t, request{method: http.MethodGet, path: metadataBasePath})
	if got.status != http.StatusOK {
		t.Fatalf("metadata status = %d, want 200: %s", got.status, got.body)
	}
	var document struct {
		ResourceDocumentation string `json:"resource_documentation"`
	}
	if err := json.Unmarshal([]byte(got.body), &document); err != nil {
		t.Fatalf("metadata is not JSON: %v", err)
	}
	if document.ResourceDocumentation == "" {
		t.Fatal("the document publishes no resource_documentation, so there is no page for the challenge to name")
	}
	return document.ResourceDocumentation
}

// assertRecipientRefusal checks the 401 a pinned deployment gives a token from
// another application: the RFC 6750 code, the error_uri naming page, and a
// message that says what is true rather than that GitLab rejected the token.
func assertRecipientRefusal(t *testing.T, srv *server, token, page string) {
	t.Helper()

	got := srv.do(t, mcpPOST(map[string]string{"Authorization": "Bearer " + token}))
	if got.status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", got.status, got.body)
	}
	challenge := got.header.Get("WWW-Authenticate")
	if !strings.Contains(challenge, `error="invalid_token"`) {
		t.Errorf("challenge %q lacks the RFC 6750 error code", challenge)
	}
	if !strings.Contains(challenge, `error_uri="`+page+`"`) {
		t.Errorf("challenge %q does not name the page the document publishes, %q", challenge, page)
	}
	if !strings.Contains(got.body, "not issued to an OAuth application") {
		t.Errorf("the refusal must say what is actually wrong; got: %s", got.body)
	}
	if strings.Contains(got.body, "expired") || strings.Contains(challenge, "expired") {
		t.Error("the refusal claims GitLab rejected the token; it did not, and reauthorizing returns the same one")
	}
}

// assertTokenAdmitted checks that a token reaches tools/list, which is what
// admission means: not merely that the door opened.
func assertTokenAdmitted(t *testing.T, srv *server, token string) {
	t.Helper()

	got := srv.do(t, mcpPOST(map[string]string{"Authorization": "Bearer " + token}))
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", got.status, got.body)
	}
	if !strings.Contains(got.body, `"tools"`) {
		t.Errorf("an admitted token must reach tools/list; body: %s", truncate(got.body))
	}
}

// assertRefusalIsFree repeats a refusal past the authentication-failure budget
// and fails if the address is ever locked out. A refusal that charged the
// budget would let one misconfigured client lock a shared address out for
// everybody holding a credential the deployment does admit.
func assertRefusalIsFree(t *testing.T, srv *server, token string) {
	t.Helper()

	for attempt := range 12 {
		got := srv.do(t, mcpPOST(map[string]string{"Authorization": "Bearer " + token}))
		if got.status == http.StatusTooManyRequests {
			t.Fatalf("attempt %d was rate limited: a genuine token from another application was charged against the address", attempt+1)
		}
		if got.status != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", attempt+1, got.status)
		}
	}
}

// TestOAuth_PublishedAuthorizationServerIsTheCanonicalIssuer pins the string
// clients read out of authorization_servers.
//
// A client builds the authorization-server metadata URL from that value and
// then, per the discovery page, checks that the document's issuer "MUST be
// identical to the issuer identifier used to construct the well-known URL",
// discarding the metadata otherwise. GitLab's issuer is the canonical form of
// the instance URL: lowercase host, no default port, no trailing slash, the
// relative root kept. Whatever spelling the operator typed, that is what has
// to be published, or every client's discovery fails against a document that
// is perfectly correct.
func TestOAuth_PublishedAuthorizationServerIsTheCanonicalIssuer(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		want       string
	}{
		{name: "case, default port and trailing slash are canonicalized", configured: "https://GitLab.Example.com:443/", want: "https://gitlab.example.com"},
		{name: "a relative URL root is kept, since it is part of the issuer", configured: "https://gitlab.example.com/gitlab/", want: "https://gitlab.example.com/gitlab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := startServer(t, nil,
				"--auth-mode=oauth",
				"--gitlab-url="+tt.configured,
				"--public-url="+publicURL,
			)

			got := srv.do(t, request{method: http.MethodGet, path: metadataBasePath})
			if got.status != http.StatusOK {
				t.Fatalf("metadata status = %d, want 200: %s", got.status, got.body)
			}
			var document struct {
				AuthorizationServers []string `json:"authorization_servers"`
			}
			if err := json.Unmarshal([]byte(got.body), &document); err != nil {
				t.Fatalf("metadata is not JSON: %v", err)
			}
			if !slices.Equal(document.AuthorizationServers, []string{tt.want}) {
				t.Errorf("authorization_servers = %v, want [%q]: a client compares this against the issuer GitLab publishes", document.AuthorizationServers, tt.want)
			}
		})
	}
}

// TestOAuth_RefusesToStartWithNoAuthorizationServerToPublish pins the
// discovery page's requirement that the metadata document "MUST include the
// authorization_servers field containing at least one authorization server".
//
// --allow-any-gitlab-url publishes no instance, which in legacy mode is a
// choice about who selects the instance. In oauth mode it would be a document
// naming nowhere to authorize, so the combination is refused before anything
// listens rather than served as an empty list.
func TestOAuth_RefusesToStartWithNoAuthorizationServerToPublish(t *testing.T) {
	out, err := runServerExpectingExit(t, serverBinary(t),
		"--http", "--http-addr=127.0.0.1:"+itoa(freePort(t)),
		"--auth-mode=oauth",
		"--allow-any-gitlab-url",
		"--public-url="+publicURL,
	)

	if err == nil {
		t.Fatalf("oauth mode started with no authorization server to publish; output:\n%s", out)
	}
	if !strings.Contains(out, "--gitlab-url") {
		t.Errorf("the startup error should say what is missing; got:\n%s", out)
	}
}

// TestOAuth_SkipTLSVerifyScope verifies where oauth mode will and will not
// forward bearer tokens over a connection whose certificate is unchecked.
//
// oauth mode already refuses an http instance, because "bearer tokens are
// forwarded to the instance on every call, and http would transmit them in
// cleartext". An https instance with verification disabled has the same
// property: any host that can answer for the address collects a live
// credential. OAuth 2.1 section 7.1.3.2 states it directly, and in the upstream
// leg this server is the client it addresses.
//
// Loopback keeps the exemption, for the same reason cleartext does: the
// credential does not leave the host. That case is here too, because a rule
// that refused it would break local development and no refusal test would
// notice.
func TestOAuth_SkipTLSVerifyScope(t *testing.T) {
	bin := serverBinary(t)

	tests := []struct {
		name       string
		gitlabURL  string
		wantRefuse bool
		wantInOut  []string
	}{
		{
			name:       "a remote instance is refused",
			gitlabURL:  "https://gitlab.example.com",
			wantRefuse: true,
			wantInOut:  []string{"skip-tls-verify", "SSL_CERT_FILE"},
		},
		{
			name:       "one unverified instance among several is enough",
			gitlabURL:  "https://localhost:8443,https://gitlab.example.com",
			wantRefuse: true,
			wantInOut:  []string{"gitlab.example.com"},
		},
		{
			name:       "a loopback instance keeps the exemption",
			gitlabURL:  "https://localhost:8443",
			wantRefuse: false,
		},
		{
			name:       "127.0.0.1 is loopback too",
			gitlabURL:  "https://127.0.0.1:8443",
			wantRefuse: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The listen address belongs to whichever harness starts the
			// process: startServer assigns and polls its own, so passing one
			// here would leave it polling a port nothing is bound to.
			authArgs := []string{
				"--auth-mode=oauth",
				"--public-url=" + publicURL,
				"--gitlab-url=" + tt.gitlabURL,
				"--skip-tls-verify",
			}

			if !tt.wantRefuse {
				// A loopback instance must get past validation. The server is
				// then started for real, so serving /health is the assertion:
				// nothing behind the URL needs to exist for that.
				srv := startServer(t, nil, authArgs...)
				got := srv.do(t, request{method: http.MethodGet, path: "/health"})
				if got.status != http.StatusOK {
					t.Fatalf("a loopback instance was refused: /health = %d %s", got.status, got.body)
				}
				return
			}

			args := append([]string{"--http", "--http-addr=127.0.0.1:" + itoa(freePort(t))}, authArgs...)
			out, err := runServerExpectingExit(t, bin, args...)
			if err == nil {
				t.Fatal("oauth mode started with certificate verification disabled")
			}
			for _, want := range tt.wantInOut {
				if !strings.Contains(out, want) {
					t.Errorf("the startup error should mention %q; got:\n%s", want, out)
				}
			}
		})
	}
}
