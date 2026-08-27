// verifier_test.go contains unit tests for the OAuth Bearer token verifier,
// covering cache hits, misses, TTL expiration, and GitLab API error handling.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// TestNewGitLabVerifier_ValidToken verifies that a successful GitLab /user
// response produces a TokenInfo with the expected UserID, username, and
// future expiration.
func TestNewGitLabVerifier_ValidToken(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/user" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer valid-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gitlabUserResponse{ID: 42, Username: "testuser"})
	}))
	defer srv.Close()

	verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, nil)
	info, err := verifier(context.Background(), "valid-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.UserID != "42" {
		t.Errorf("UserID = %q, want %q", info.UserID, "42")
	}
	if got, ok := info.Extra["username"].(string); !ok || got != "testuser" {
		t.Errorf("Extra[username] = %v, want %q", info.Extra["username"], "testuser")
	}
	if got, ok := info.Extra["token"].(string); !ok || got != "valid-token" {
		t.Errorf("Extra[token] = %v, want %q", info.Extra["token"], "valid-token")
	}
	if info.Expiration.Before(time.Now()) {
		t.Error("Expiration should be in the future")
	}
}

// TestNewGitLabVerifier_InvalidToken verifies that a 401 response from
// GitLab is translated into an error wrapping auth.ErrInvalidToken.
func TestNewGitLabVerifier_InvalidToken(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, nil)
	_, err := verifier(context.Background(), "bad-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	if !isErrInvalidToken(err) {
		t.Errorf("error should wrap auth.ErrInvalidToken, got: %v", err)
	}
}

// TestNewGitLabVerifier_ServerError verifies that a 5xx response surfaces
// a generic error and does NOT wrap auth.ErrInvalidToken.
func TestNewGitLabVerifier_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, nil)
	_, err := verifier(context.Background(), "some-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if isErrInvalidToken(err) {
		t.Error("500 error should NOT wrap auth.ErrInvalidToken")
	}
}

// TestNewGitLabVerifier_NetworkError verifies that a connection failure to
// a closed server returns a non-nil error.
func TestNewGitLabVerifier_NetworkError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Close() // close immediately to force connection error

	verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, nil)
	_, err := verifier(context.Background(), "token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for closed server")
	}
}

// TestNewGitLabVerifier_MalformedJSON verifies that a malformed JSON body
// from the GitLab /user endpoint produces a decoding error.
func TestNewGitLabVerifier_MalformedJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid-json`))
	}))
	defer srv.Close()

	verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, nil)
	_, err := verifier(context.Background(), "token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// TestNewGitLabVerifier_SkipTLSVerify verifies that skipTLS=true allows
// successful verification against an httptest TLS server with a self-signed
// certificate.
func TestNewGitLabVerifier_SkipTLSVerify(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gitlabUserResponse{ID: 7, Username: "tlsuser"})
	}))
	defer srv.Close()

	verifier := NewGitLabVerifier(srv.URL, true, 10*time.Minute, nil)
	info, err := verifier(context.Background(), "tls-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("unexpected error with skipTLS=true: %v", err)
	}
	if info.UserID != "7" {
		t.Errorf("UserID = %q, want %q", info.UserID, "7")
	}
}

// TestNewGitLabVerifier_CacheHit verifies that a second verification for
// the same token within the TTL window is served from the cache without
// calling the GitLab API again.
func TestNewGitLabVerifier_CacheHit(t *testing.T) {
	t.Parallel()

	var apiCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Scope introspection probes other paths; the cache assertion is
		// about identity verification, so only /api/v4/user is counted and
		// the probes answer 404 (introspection unavailable).
		if r.URL.Path != "/api/v4/user" {
			http.NotFound(w, r)
			return
		}
		apiCalls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gitlabUserResponse{ID: 1, Username: "cached"})
	}))
	defer srv.Close()

	cache := NewTokenCache()
	verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, cache)

	info1, err := verifier(context.Background(), "my-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if apiCalls != 1 {
		t.Fatalf("expected 1 API call after first call, got %d", apiCalls)
	}

	info2, err := verifier(context.Background(), "my-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if apiCalls != 1 {
		t.Errorf("expected still 1 API call (cache hit), got %d", apiCalls)
	}
	if info2.UserID != info1.UserID {
		t.Errorf("cached UserID %q != original %q", info2.UserID, info1.UserID)
	}
}

// TestNewGitLabVerifier_CacheExpiry verifies that once the cached entry's
// TTL elapses, the next verification triggers a fresh call to the GitLab
// API.
func TestNewGitLabVerifier_CacheExpiry(t *testing.T) {
	t.Parallel()

	var apiCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/user" {
			http.NotFound(w, r)
			return
		}
		apiCalls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gitlabUserResponse{ID: 2, Username: "expiry"})
	}))
	defer srv.Close()

	cache := NewTokenCache()
	verifier := NewGitLabVerifier(srv.URL, false, 1*time.Millisecond, cache)

	_, err := verifier(context.Background(), "exp-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	_, err = verifier(context.Background(), "exp-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if apiCalls != 2 {
		t.Errorf("expected 2 API calls after expiry, got %d", apiCalls)
	}
}

// TestNewGitLabVerifier_CacheInvalidationOnError verifies that when a
// revoked token is re-validated and returns 401, the cache entry is
// removed and the error wraps auth.ErrInvalidToken.
func TestNewGitLabVerifier_CacheInvalidationOnError(t *testing.T) {
	t.Parallel()

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(gitlabUserResponse{ID: 3, Username: "inv"})
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	cache := NewTokenCache()
	verifier := NewGitLabVerifier(srv.URL, false, 1*time.Hour, cache)

	_, err := verifier(context.Background(), "inv-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Expire the cached entry to force re-validation against the now-401 server
	cache.Delete("inv-token")

	_, err = verifier(context.Background(), "inv-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error on revoked token")
	}
	if !isErrInvalidToken(err) {
		t.Errorf("expected auth.ErrInvalidToken, got: %v", err)
	}

	if _, ok := cache.Get("inv-token"); ok {
		t.Error("cache should not contain invalidated token")
	}
}

// TestNewGitLabVerifier_CacheDifferentTokens verifies that different
// tokens are cached under separate keys and subsequent lookups hit the
// cache without additional API calls.
func TestNewGitLabVerifier_CacheDifferentTokens(t *testing.T) {
	t.Parallel()

	var apiCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/user" {
			http.NotFound(w, r)
			return
		}
		apiCalls++
		id := apiCalls
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gitlabUserResponse{ID: id, Username: fmt.Sprintf("user%d", id)})
	}))
	defer srv.Close()

	cache := NewTokenCache()
	verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, cache)

	info1, err := verifier(context.Background(), "token-a", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("token-a: %v", err)
	}
	info2, err := verifier(context.Background(), "token-b", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("token-b: %v", err)
	}
	if apiCalls != 2 {
		t.Fatalf("expected 2 API calls for different tokens, got %d", apiCalls)
	}
	if info1.UserID == info2.UserID {
		t.Error("different tokens should map to different users")
	}

	// Re-fetch both: should be cache hits
	_, _ = verifier(context.Background(), "token-a", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	_, _ = verifier(context.Background(), "token-b", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if apiCalls != 2 {
		t.Errorf("expected still 2 API calls after cache hits, got %d", apiCalls)
	}
}

// TestNewGitLabVerifier_InvalidURL verifies that a malformed base URL
// (containing a control character) causes request construction to fail.
func TestNewGitLabVerifier_InvalidURL(t *testing.T) {
	t.Parallel()

	// Control character in URL makes NewRequestWithContext fail
	verifier := NewGitLabVerifier("http://invalid\x00url", false, 15*time.Minute, nil)
	_, err := verifier(context.Background(), "token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// TestNewGitLabVerifier_NetworkErrorWithCache verifies that a network
// failure during re-validation removes the previously cached entry so a
// stale identity is not served.
func TestNewGitLabVerifier_NetworkErrorWithCache(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gitlabUserResponse{ID: 10, Username: "net"})
	}))

	cache := NewTokenCache()
	verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, cache)

	_, err := verifier(context.Background(), "net-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, ok := cache.Get("net-token"); !ok {
		t.Fatal("token should be cached after successful call")
	}

	srv.Close()

	cache.Delete("net-token")
	_, err = verifier(context.Background(), "net-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for closed server")
	}

	if _, ok := cache.Get("net-token"); ok {
		t.Error("cache entry should be deleted after network error")
	}
}

// TestNewGitLabVerifier_ServerErrorWithCache verifies that a 5xx response
// during re-validation removes the cached entry and surfaces a non-invalid
// token error.
func TestNewGitLabVerifier_ServerErrorWithCache(t *testing.T) {
	t.Parallel()

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(gitlabUserResponse{ID: 11, Username: "srv"})
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cache := NewTokenCache()
	verifier := NewGitLabVerifier(srv.URL, false, 1*time.Hour, cache)

	_, err := verifier(context.Background(), "srv-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	cache.Delete("srv-token")

	_, err = verifier(context.Background(), "srv-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if isErrInvalidToken(err) {
		t.Error("500 error should NOT wrap auth.ErrInvalidToken")
	}
	if _, ok := cache.Get("srv-token"); ok {
		t.Error("cache entry should be deleted after server error")
	}
}

// TestNewGitLabVerifier_Forbidden verifies that a 403 response returns an
// error wrapping auth.ErrInvalidToken.
func TestNewGitLabVerifier_Forbidden(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, nil)
	_, err := verifier(context.Background(), "forbidden-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !isErrInvalidToken(err) {
		t.Errorf("403 error should wrap auth.ErrInvalidToken, got: %v", err)
	}
}

// TestNewGitLabVerifier_UserIDZero verifies that a valid HTTP 200 response
// with user.ID == 0 returns an error wrapping auth.ErrInvalidToken.
func TestNewGitLabVerifier_UserIDZero(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gitlabUserResponse{ID: 0, Username: "ghost"})
	}))
	defer srv.Close()

	verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, nil)
	_, err := verifier(context.Background(), "zero-id-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for user ID 0")
	}
	if !isErrInvalidToken(err) {
		t.Errorf("user ID 0 error should wrap auth.ErrInvalidToken, got: %v", err)
	}
}

// TestNewGitLabVerifier_UnexpectedStatusCode verifies that a status GitLab
// does not use to judge a credential is classified upstream, not as an
// invalid token.
//
// Only 401 and 403 are GitLab answering the question. A 404 from a misrouted
// proxy or a 418 from something in between is a question that never got
// answered, and calling it invalid_token would cache a perfectly good token
// as rejected and charge the caller's failure budget for someone else's
// routing mistake.
func TestNewGitLabVerifier_UnexpectedStatusCode(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusTeapot, http.StatusNotFound, http.StatusRequestTimeout} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			defer srv.Close()

			verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, nil)
			_, err := verifier(t.Context(), "odd-status-token", httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody))
			if err == nil {
				t.Fatalf("expected error for %d response", status)
			}
			if isErrInvalidToken(err) {
				t.Errorf("HTTP %d must not be blamed on the credential, got: %v", status, err)
			}
			upstream, ok := errors.AsType[*UpstreamError](err)
			if !ok {
				t.Fatalf("error should be an *UpstreamError, got %T: %v", err, err)
			}
			if upstream.Status != status {
				t.Errorf("Status = %d, want %d", upstream.Status, status)
			}
		})
	}
}

// isErrInvalidToken checks if an error wraps auth.ErrInvalidToken.
func isErrInvalidToken(err error) bool {
	return errors.Is(err, auth.ErrInvalidToken)
}

// TestNewGitLabVerifier_ScopeIntrospection_ResolvesGrantedScopes verifies granted scopes come
// from real introspection — /personal_access_tokens/self for PATs,
// /oauth/token/info for OAuth tokens — with the historical api assumption
// kept only when neither endpoint answers usably, so restricted instances
// keep working while a read_user token is never stamped api-scoped.
func TestNewGitLabVerifier_ScopeIntrospection_ResolvesGrantedScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		patStatus  int
		patBody    string
		infoStatus int
		infoBody   string
		want       []string
	}{
		{
			name:      "PAT scopes are introspected",
			patStatus: http.StatusOK, patBody: `{"scopes":["read_user"]}`,
			infoStatus: http.StatusNotFound, infoBody: `{}`,
			want: []string{"read_user"},
		},
		{
			name:      "OAuth scopes come from token info",
			patStatus: http.StatusNotFound, patBody: `{}`,
			infoStatus: http.StatusOK, infoBody: `{"scope":["api"]}`,
			want: []string{"api", "read_api"},
		},
		{
			name:      "introspection unavailable assumes api",
			patStatus: http.StatusNotFound, patBody: `{}`,
			infoStatus: http.StatusNotFound, infoBody: `{}`,
			want: []string{"api", "read_api"},
		},
		{
			name:      "non-200 introspection falls back",
			patStatus: http.StatusInternalServerError, patBody: `boom`,
			infoStatus: http.StatusServiceUnavailable, infoBody: `down`,
			want: []string{"api", "read_api"},
		},
		{
			name:      "malformed introspection JSON falls back",
			patStatus: http.StatusOK, patBody: `{not json`,
			infoStatus: http.StatusOK, infoBody: `also{bad`,
			want: []string{"api", "read_api"},
		},
		{
			name:      "empty scope lists fall back",
			patStatus: http.StatusOK, patBody: `{"scopes":[]}`,
			infoStatus: http.StatusOK, infoBody: `{"scope":[]}`,
			want: []string{"api", "read_api"},
		},
		{
			name:      "non-string scope entries are ignored",
			patStatus: http.StatusOK, patBody: `{"scopes":[7]}`,
			infoStatus: http.StatusOK, infoBody: `{"scope":["read_api"]}`,
			want: []string{"read_api"},
		},
		{
			// api is a superset of read_api, but GitLab reports only the
			// granted name and the SDK's scope check is plain set
			// containment: without the implication, a read-only deployment
			// asking for read_api would 403 the more privileged token.
			name:      "api implies read_api",
			patStatus: http.StatusOK, patBody: `{"scopes":["api","sudo"]}`,
			infoStatus: http.StatusNotFound, infoBody: `{}`,
			want: []string{"api", "sudo", "read_api"},
		},
		{
			name:      "read_api alone is not widened",
			patStatus: http.StatusOK, patBody: `{"scopes":["read_api"]}`,
			infoStatus: http.StatusNotFound, infoBody: `{}`,
			want: []string{"read_api"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v4/user":
					json.NewEncoder(w).Encode(gitlabUserResponse{ID: 7, Username: "scoped"})
				case "/api/v4/personal_access_tokens/self":
					w.WriteHeader(tt.patStatus)
					fmt.Fprint(w, tt.patBody)
				case "/oauth/token/info":
					w.WriteHeader(tt.infoStatus)
					fmt.Fprint(w, tt.infoBody)
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			verifier := NewGitLabVerifier(srv.URL, false, time.Minute, nil)
			info, err := verifier(context.Background(), "tok", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
			if err != nil {
				t.Fatalf("verifier: %v", err)
			}
			if !slices.Equal(info.Scopes, tt.want) {
				t.Errorf("Scopes = %v, want %v", info.Scopes, tt.want)
			}
		})
	}
}

// TestMetadataURLFor_InsertsWellKnownBetweenHostAndPath verifies the RFC
// 9728 §3 derivation for both path-carrying and path-less resource
// identifiers.
func TestMetadataURLFor_InsertsWellKnownBetweenHostAndPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		resource string
		want     string
	}{
		{"https://mcp.example.com/gitlab", "https://mcp.example.com/.well-known/oauth-protected-resource/gitlab"},
		{"https://mcp.example.com", "https://mcp.example.com/.well-known/oauth-protected-resource"},
		{"http://localhost:8080", "http://localhost:8080/.well-known/oauth-protected-resource"},
	}
	for _, tt := range tests {
		t.Run(tt.resource, func(t *testing.T) {
			t.Parallel()
			if got := MetadataURLFor(tt.resource); got != tt.want {
				t.Errorf("MetadataURLFor(%q) = %q, want %q", tt.resource, got, tt.want)
			}
		})
	}
}

// TestRequiredScope_LeastPrivilegeForTheDeployment verifies that a server
// which cannot reach GitLab as a write asks only for read_api. Safe mode
// counts as read-only because it answers mutating calls with a preview
// instead of forwarding them, so demanding api there would make every user
// grant write access the server never exercises.
func TestRequiredScope_LeastPrivilegeForTheDeployment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		readOnly bool
		safeMode bool
		want     string
	}{
		{"writes possible", false, false, ScopeAPI},
		{"read-only", true, false, ScopeReadAPI},
		{"safe mode", false, true, ScopeReadAPI},
		{"both", true, true, ScopeReadAPI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := RequiredScope(tt.readOnly, tt.safeMode); got != tt.want {
				t.Errorf("RequiredScope(%t, %t) = %q, want %q", tt.readOnly, tt.safeMode, got, tt.want)
			}
		})
	}
}

// TestNewGitLabVerifier_UpstreamFailure_IsNotAnInvalidToken verifies the
// distinction that matters most in this file: GitLab declining to answer is
// not GitLab saying the token is bad. Reporting a throttled or unreachable
// instance as invalid_token makes a well-behaved MCP client discard a good
// credential and start a fresh authorization flow, adding upstream load at
// exactly the wrong moment.
func TestNewGitLabVerifier_UpstreamFailure_IsNotAnInvalidToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		status         int
		retryAfter     string
		wantStatus     int
		wantRetryAfter time.Duration
	}{
		{"throttled with delta-seconds", http.StatusTooManyRequests, "17", http.StatusTooManyRequests, 17 * time.Second},
		{"throttled without a hint", http.StatusTooManyRequests, "", http.StatusTooManyRequests, 0},
		{"throttled with unparseable hint", http.StatusTooManyRequests, "soon", http.StatusTooManyRequests, 0},
		{"throttled with a past date", http.StatusTooManyRequests, "Mon, 02 Jan 2006 15:04:05 GMT", http.StatusTooManyRequests, 0},
		{"server error", http.StatusBadGateway, "", http.StatusBadGateway, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			verifier := NewGitLabVerifier(srv.URL, false, time.Minute, nil)
			_, err := verifier(t.Context(), "token", httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody))
			if err == nil {
				t.Fatal("expected an error")
			}
			if isErrInvalidToken(err) {
				t.Errorf("an upstream failure must not wrap auth.ErrInvalidToken: %v", err)
			}

			var upstream *UpstreamError
			if !errors.As(err, &upstream) {
				t.Fatalf("error should be an *UpstreamError, got %T: %v", err, err)
			}
			if upstream.Status != tt.wantStatus {
				t.Errorf("Status = %d, want %d", upstream.Status, tt.wantStatus)
			}
			if upstream.RetryAfter != tt.wantRetryAfter {
				t.Errorf("RetryAfter = %v, want %v", upstream.RetryAfter, tt.wantRetryAfter)
			}
		})
	}
}

// TestNewGitLabVerifier_UnreachableInstance_ReportsNoStatus verifies that a
// transport failure — nothing was ever answered — is classified upstream with
// a zero status, so a caller can tell "GitLab said 429" from "GitLab said
// nothing at all" and still not blame the credential.
func TestNewGitLabVerifier_UnreachableInstance_ReportsNoStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Close() // closed immediately: the connection cannot be made

	verifier := NewGitLabVerifier(srv.URL, false, time.Minute, nil)
	_, err := verifier(t.Context(), "token", httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody))
	if err == nil {
		t.Fatal("expected an error")
	}
	if isErrInvalidToken(err) {
		t.Errorf("an unreachable instance must not wrap auth.ErrInvalidToken: %v", err)
	}

	var upstream *UpstreamError
	if !errors.As(err, &upstream) {
		t.Fatalf("error should be an *UpstreamError, got %T: %v", err, err)
	}
	if upstream.Status != 0 {
		t.Errorf("Status = %d, want 0 for a request that got no response", upstream.Status)
	}
	if upstream.Error() == "" {
		t.Error("UpstreamError should describe itself")
	}
}

// TestRetryAfter_HTTPDate_RoundsUp verifies that a sub-second remainder in an
// HTTP-date is rounded up rather than truncated. The delay is rendered into a
// Retry-After header with second granularity, so truncating 0.4s to 0 would
// invite a retry before the boundary GitLab asked for.
func TestRetryAfter_HTTPDate_RoundsUp(t *testing.T) {
	t.Parallel()

	// An HTTP-date carries whole seconds, so parsing one always lands on a
	// second boundary while "now" does not: the remaining duration is
	// fractional by construction. Two seconds out keeps it comfortably
	// positive however the truncation falls.
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", time.Now().Add(2*time.Second).UTC().Format(http.TimeFormat))

	got := retryAfter(resp)
	if got <= 0 {
		t.Fatalf("retryAfter() = %v, want a positive delay", got)
	}
	if got%time.Second != 0 {
		t.Errorf("retryAfter() = %v, want a whole number of seconds", got)
	}
	// Rounded up, never down: truncating would allow a retry before the
	// instant GitLab named.
	if got < time.Second {
		t.Errorf("retryAfter() = %v, want at least 1s after rounding up", got)
	}
}
