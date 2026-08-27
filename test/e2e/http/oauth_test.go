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
	"net/http"
	"strings"
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

// TestOAuth_MetadataAdvertisesTheScopeItRequires verifies the RFC 9728
// document, including the least-privilege scope: a client reads
// scopes_supported to decide what to ask GitLab for, so a read-only deployment
// advertising "api" would make every user grant write access it can never use.
func TestOAuth_MetadataAdvertisesTheScopeItRequires(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")

	for _, tc := range []struct {
		name  string
		flags []string
		want  string
	}{
		{"writes possible", nil, "api"},
		{"read-only", []string{"--read-only"}, "read_api"},
		{"safe mode", []string{"--safe-mode"}, "read_api"},
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
			if len(meta.ScopesSupported) != 1 || meta.ScopesSupported[0] != tc.want {
				t.Errorf("scopes_supported = %v, want [%q]", meta.ScopesSupported, tc.want)
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
		if !strings.Contains(challenge, want) {
			t.Errorf("challenge %q is missing %s", challenge, want)
		}
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
