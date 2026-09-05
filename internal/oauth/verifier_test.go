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
	"strings"
	"sync/atomic"
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
	if info.Expiration.Before(time.Now()) {
		t.Error("Expiration should be in the future")
	}
}

// TestNewGitLabVerifier_AdmissionCarriesNoRawToken pins that the identity a
// verifier returns, and therefore the value the [TokenCache] holds for the
// TTL, contains the token nowhere.
//
// It did until this test existed: Extra["token"] carried the bearer in the
// clear "for downstream GitLab client creation", which nothing downstream ever
// read, while the package documentation promised the cache stored no raw token
// material. A digest key is worth nothing if the value beside it is the
// credential. The check walks every Extra value rather than one key, so the
// token cannot come back under another name.
func TestNewGitLabVerifier_AdmissionCarriesNoRawToken(t *testing.T) {
	t.Parallel()

	const token = "gloas-secret-material"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/user" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gitlabUserResponse{ID: 42, Username: "testuser"})
	}))
	t.Cleanup(srv.Close)

	cache := NewTokenCache()
	verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, cache)
	info, err := verifier(t.Context(), token, nil)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}

	cached, ok := cache.Get(srv.URL, token)
	if !ok {
		t.Fatal("the admission was not cached")
	}
	for name, subject := range map[string]*auth.TokenInfo{"returned": info, "cached": cached} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, present := subject.Extra["token"]; present {
				t.Error("Extra carries a \"token\" entry; the identity must not hold the credential it was verified with")
			}
			for key, value := range subject.Extra {
				if text, isString := value.(string); isString && strings.Contains(text, token) {
					t.Errorf("Extra[%q] = %q carries the raw token", key, text)
				}
			}
		})
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
	cache.Delete(srv.URL, "inv-token")

	_, err = verifier(context.Background(), "inv-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error on revoked token")
	}
	if !isErrInvalidToken(err) {
		t.Errorf("expected auth.ErrInvalidToken, got: %v", err)
	}

	if _, ok := cache.Get(srv.URL, "inv-token"); ok {
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
	if _, ok := cache.Get(srv.URL, "net-token"); !ok {
		t.Fatal("token should be cached after successful call")
	}

	srv.Close()

	cache.Delete(srv.URL, "net-token")
	_, err = verifier(context.Background(), "net-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for closed server")
	}

	if _, ok := cache.Get(srv.URL, "net-token"); ok {
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

	cache.Delete(srv.URL, "srv-token")

	_, err = verifier(context.Background(), "srv-token", httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if isErrInvalidToken(err) {
		t.Error("500 error should NOT wrap auth.ErrInvalidToken")
	}
	if _, ok := cache.Get(srv.URL, "srv-token"); ok {
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
		// An identifier with no host cannot be split into host and path, so
		// the derivation degrades to appending. Config validation rejects
		// these before they reach here; the fallback exists so a challenge
		// still carries a syntactically valid URL rather than a panic or an
		// empty resource_metadata a client would silently ignore.
		{"://not a url", "://not a url/.well-known/oauth-protected-resource"},
		{"/gitlab/", "/gitlab/.well-known/oauth-protected-resource"},
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
		// A parseable but non-positive delta is treated as no hint at all.
		// Passing it through would render "Retry-After: 0", telling the
		// client to retry immediately — which is what the throttle is for.
		{"throttled with a zero delta", http.StatusTooManyRequests, "0", http.StatusTooManyRequests, 0},
		{"throttled with a negative delta", http.StatusTooManyRequests, "-5", http.StatusTooManyRequests, 0},
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

// TestFetchScopes_UnusableEndpoint_ReportsNoAnswer verifies that neither a
// malformed endpoint nor an unreachable one is mistaken for a definitive
// "this token has no scopes".
//
// The nil return means "this endpoint did not answer for this token kind",
// which is what lets introspectScopes try the next endpoint and then fall
// back to the historical assumption. An empty non-nil slice would read as a
// real answer instead, stripping every scope from a perfectly good token and
// locking the caller out whenever introspection is merely unreachable.
func TestFetchScopes_UnusableEndpoint_ReportsNoAnswer(t *testing.T) {
	t.Parallel()

	// A server started and immediately closed yields an address nothing is
	// listening on, so the request is well-formed and the round trip fails.
	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachable := closed.URL + "/api/v4/personal_access_tokens/self"
	closed.Close()

	tests := []struct {
		name     string
		endpoint string
	}{
		{"endpoint that cannot be parsed", "://not a url"},
		{"host that refuses the connection", unreachable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var refused atomic.Bool
			if got := fetchIntrospection(t.Context(), http.DefaultClient, tt.endpoint, "glpat-x", &refused); got != nil {
				t.Errorf("fetchIntrospection(%s) = %v, want nil", tt.name, got)
			}
		})
	}
}

// TestGitLabVerifier_CacheNeverOutlivesTheToken pins that a cached admission
// expires with the credential it was taken from.
//
// The verifier caches a successful verification for the configured TTL — fifteen
// minutes by default, up to two hours. Keyed on that alone, a token that expired
// a second after being verified kept being answered 200 for the rest of the
// window, while the specification requires an expired token to receive 401. The
// cached lifetime is now the shorter of the configured TTL and the token's own
// remaining life, which introspection reports.
func TestGitLabVerifier_CacheNeverOutlivesTheToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		endpoint   string
		payload    string
		configured time.Duration
		wantAtMost time.Duration
	}{
		{
			name:       "an OAuth token expiring sooner than the TTL shortens it",
			endpoint:   "/oauth/token/info",
			payload:    `{"scope":["api"],"expires_in":30}`,
			configured: time.Hour,
			wantAtMost: 31 * time.Second,
		},
		{
			name:       "a non-expiring OAuth token keeps the configured TTL",
			endpoint:   "/oauth/token/info",
			payload:    `{"scope":["api"],"expires_in":null}`,
			configured: time.Minute,
			wantAtMost: time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mux := http.NewServeMux()
			mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":7,"username":"someone"}`))
			})
			// The PAT endpoint must not answer, or introspection stops there.
			mux.HandleFunc("/api/v4/personal_access_tokens/self", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			})
			mux.HandleFunc(tt.endpoint, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.payload))
			})
			gitlab := httptest.NewServer(mux)
			defer gitlab.Close()

			verifier := NewGitLabVerifier(gitlab.URL, false, tt.configured, nil)
			info, err := verifier(t.Context(), "glpat-whatever", nil)
			if err != nil {
				t.Fatalf("verify: %v", err)
			}

			lifetime := time.Until(info.Expiration)
			if lifetime > tt.wantAtMost {
				t.Errorf("cached for %s, want at most %s — the admission outlives the token", lifetime, tt.wantAtMost)
			}
			if lifetime <= 0 {
				t.Errorf("cached lifetime is %s; a valid token must still be admitted", lifetime)
			}
		})
	}
}

// TestExpiryFromDate covers the shape GitLab uses for personal access tokens,
// which is a calendar date rather than a timestamp. Parsing it as RFC 3339 fails
// and silently degrades to "never expires", which is the bug worth pinning.
func TestExpiryFromDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  any
		zero bool
	}{
		{name: "a GitLab date is understood", raw: "2030-01-02", zero: false},
		{name: "null means the token does not expire", raw: nil, zero: true},
		{name: "an RFC 3339 timestamp is not the shape GitLab sends", raw: "2030-01-02T03:04:05Z", zero: true},
		{name: "an empty string is not a date", raw: "", zero: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := expiryFromDate(tt.raw)
			if got.IsZero() != tt.zero {
				t.Errorf("expiryFromDate(%v) zero = %v, want %v", tt.raw, got.IsZero(), tt.zero)
			}
		})
	}

	// The instant matters, not just that one was produced. GitLab expires a
	// personal access token at 00:00:00 UTC on the stated date, so the date is
	// when the token dies and not the last day it works. Reading it as
	// "valid through" put the expiry 24 hours late, which would let a cached
	// admission outlive the credential it was taken from.
	t.Run("the date is midnight UTC on that day, not the day after", func(t *testing.T) {
		t.Parallel()
		got := expiryFromDate("2030-01-02")
		want := time.Date(2030, time.January, 2, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("expiryFromDate(\"2030-01-02\") = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
		}
	})
}

// TestNewGitLabVerifier_ForbiddenDistinguishesScope covers the two very
// different things a GitLab 403 can mean.
//
// A token that is genuine but under-scoped must be reported as such, so the
// caller is told to request the missing scope rather than to throw a working
// credential away — and so the gate does not charge the attempt against the
// caller's authentication-failure budget. Anything else 403 means the caller may
// not do this at all, which keeps being treated as an invalid credential.
//
// The distinction lives only in the body: GitLab does not send WWW-Authenticate
// on a 403, so a check reading the challenge would never fire.
func TestNewGitLabVerifier_ForbiddenDistinguishesScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		body           string
		contentType    string
		wantScopeError bool
	}{
		{
			name:           "rack-oauth2 insufficient_scope",
			body:           `{"error":"insufficient_scope","error_description":"requires higher privileges","scope":"api"}`,
			contentType:    "application/json",
			wantScopeError: true,
		},
		{
			name:           "granular personal access token scope",
			body:           `{"error":"insufficient_granular_scope"}`,
			contentType:    "application/json",
			wantScopeError: true,
		},
		{
			name:           "a Grape forbidden carries message, not error",
			body:           `{"message":"403 Forbidden - Your account has been blocked"}`,
			contentType:    "application/json",
			wantScopeError: false,
		},
		{
			name:           "a plain-text rejection is not a scope problem",
			body:           "forbidden",
			contentType:    "text/plain",
			wantScopeError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, nil)
			_, err := verifier(t.Context(), "some-token", nil)
			if err == nil {
				t.Fatal("expected an error for a 403 response")
			}

			gotScope := errors.Is(err, ErrInsufficientScope)
			if gotScope != tt.wantScopeError {
				t.Errorf("ErrInsufficientScope = %v, want %v (err: %v)", gotScope, tt.wantScopeError, err)
			}
			// Whatever the shape, it must never be reported as an upstream
			// failure: GitLab answered, and it answered about the credential.
			if !gotScope && !isErrInvalidToken(err) {
				t.Errorf("a non-scope 403 should wrap auth.ErrInvalidToken, got: %v", err)
			}
		})
	}
}

// TestAcceptedRecipient_PinRefusesEverythingItCannotVouchFor pins the one place
// in this file whose fail-open default is deliberately inverted.
//
// GitLab's authorization server publishes no `resource_indicators_supported`,
// so RFC 8707 audience restriction is unavailable and the specification's
// alternative — "or otherwise verify that they are the intended recipient" — is
// the only route left: compare the OAuth application a token was minted for
// against the ones the operator pinned.
//
// Scope introspection around it is fail-open on purpose, falling back to `api`
// with a debug log when neither endpoint answers, so that restricted instances
// keep working. Reusing that shape here would make breaking introspection the
// way around the pin, so an unanswered introspection, an absent uid and an
// unmatched uid are all refusals.
func TestAcceptedRecipient_PinRefusesEverythingItCannotVouchFor(t *testing.T) {
	t.Parallel()

	const ours = "5a4f1c0e"

	tests := []struct {
		name string
		// wantSentinel is the error the refusal must wrap, or nil for an
		// admission. The guard routes on it: ErrUnacceptedRecipient is a
		// verdict on the token and answers 401, ErrRecipientUnverifiable is a
		// failed check and answers 503 without caching.
		wantSentinel error
		pinned       []string
		result       introspection
		wantErr      bool
	}{
		{
			name:   "no pin admits a personal access token",
			result: introspection{scopes: []string{"api"}, answered: true},
		},
		{
			name:   "no pin admits another application's token",
			result: introspection{scopes: []string{"api"}, applicationUID: "somebody-else", answered: true},
		},
		{
			name:   "pinned application is admitted",
			pinned: []string{"another", ours},
			result: introspection{scopes: []string{"api"}, applicationUID: ours, answered: true},
		},
		{
			name:         "another application is refused",
			pinned:       []string{ours},
			result:       introspection{scopes: []string{"api"}, applicationUID: "somebody-else", answered: true},
			wantErr:      true,
			wantSentinel: ErrUnacceptedRecipient,
		},
		{
			name:         "a personal access token is refused under a pin",
			pinned:       []string{ours},
			result:       introspection{scopes: []string{"api"}, answered: true},
			wantErr:      true,
			wantSentinel: ErrUnacceptedRecipient,
		},
		{
			// Refused, but not judged: introspection said nothing, so the
			// token's application is unknown rather than wrong. Reporting it as
			// a verdict would tell an admissible token it belongs to somebody
			// else, and would cache that non-answer for the whole TTL.
			name:         "an unanswered introspection is refused as unverifiable",
			pinned:       []string{ours},
			result:       introspection{scopes: []string{"api"}},
			wantErr:      true,
			wantSentinel: ErrRecipientUnverifiable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := acceptedRecipient(tt.pinned, tt.result)
			if tt.wantErr {
				if err == nil {
					t.Fatal("acceptedRecipient() = nil, want a refusal")
				}
				// Never auth.ErrInvalidToken: GitLab accepted this credential,
				// and reporting it as rejected sends its holder to reauthorize
				// and return with the same token.
				if !errors.Is(err, tt.wantSentinel) {
					t.Errorf("acceptedRecipient() error = %v, want it to wrap %v", err, tt.wantSentinel)
				}
				if errors.Is(err, auth.ErrInvalidToken) {
					t.Errorf("acceptedRecipient() error = %v, must not read as GitLab's own verdict", err)
				}
				return
			}
			if err != nil {
				t.Errorf("acceptedRecipient() = %v, want admission", err)
			}
		})
	}
}

// TestApplicationUID_ReadsTheNestedObjectAndNothingElse covers the shapes
// /oauth/token/info can produce. A misread here is silent: it would look like a
// personal access token, which under a pin is a refusal of a legitimate client.
func TestApplicationUID_ReadsTheNestedObjectAndNothingElse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  any
		want string
	}{
		{name: "absent", raw: nil, want: ""},
		{name: "not an object", raw: "5a4f1c0e", want: ""},
		{name: "object without uid", raw: map[string]any{"name": "app"}, want: ""},
		{name: "uid not a string", raw: map[string]any{"uid": 42.0}, want: ""},
		{name: "uid", raw: map[string]any{"uid": "5a4f1c0e"}, want: "5a4f1c0e"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := applicationUID(tt.raw); got != tt.want {
				t.Errorf("applicationUID(%v) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

// TestGitLabVerifier_RecipientPinIsEnforcedBeforeAnythingIsCached wires the pin
// end to end: a token minted for a pinned application is admitted, one minted
// for another is refused, and the refusal leaves nothing behind in the cache.
//
// The cache order matters on its own. Caching first and checking after would
// make the second request to a refused token answer from cache — which is to
// say, admit it.
func TestGitLabVerifier_RecipientPinIsEnforcedBeforeAnythingIsCached(t *testing.T) {
	t.Parallel()

	const ours = "5a4f1c0e"

	newGitLab := func(t *testing.T, uid string) *httptest.Server {
		t.Helper()
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v4/user", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":7,"username":"someone"}`))
		})
		// A PAT answer here would end introspection before the OAuth endpoint,
		// which is the endpoint that names the application.
		mux.HandleFunc("/api/v4/personal_access_tokens/self", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		mux.HandleFunc("/oauth/token/info", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"scope":["api"],"expires_in":null,"application":{"uid":%q}}`, uid)
		})
		gitlab := httptest.NewServer(mux)
		t.Cleanup(gitlab.Close)
		return gitlab
	}

	t.Run("a token from the pinned application is admitted", func(t *testing.T) {
		t.Parallel()
		gitlab := newGitLab(t, ours)
		verifier := NewGitLabVerifier(gitlab.URL, false, time.Minute, nil, ours)
		if _, err := verifier(t.Context(), "oauth-token", nil); err != nil {
			t.Fatalf("verify: %v, want admission", err)
		}
	})

	t.Run("a token from another application is refused and not cached", func(t *testing.T) {
		t.Parallel()
		gitlab := newGitLab(t, "somebody-else")
		cache := NewTokenCache()
		verifier := NewGitLabVerifier(gitlab.URL, false, time.Minute, cache, ours)

		_, err := verifier(t.Context(), "oauth-token", nil)
		if err == nil {
			t.Fatal("verify() = nil, want a refusal for a token minted for another application")
		}
		if !errors.Is(err, ErrUnacceptedRecipient) {
			t.Errorf("verify() error = %v, want it to wrap ErrUnacceptedRecipient", err)
		}
		if got := cache.Len(); got != 0 {
			t.Errorf("cache holds %d entries after a refusal; a refused token must never become a cached identity", got)
		}
	})

	t.Run("no pin admits the same token", func(t *testing.T) {
		t.Parallel()
		gitlab := newGitLab(t, "somebody-else")
		verifier := NewGitLabVerifier(gitlab.URL, false, time.Minute, nil)
		if _, err := verifier(t.Context(), "oauth-token", nil); err != nil {
			t.Fatalf("verify: %v — an unpinned deployment must keep admitting every credential the instance accepts", err)
		}
	})
}

// TestSupportedScopes_FollowsWhatTheDeploymentCanDo pins the list published as
// RFC 9728 scopes_supported.
//
// A deployment that cannot write must not advertise the write scope: a client
// reading the metadata authorizes for what it is offered, and offering api on a
// read-only server asks the user to grant a permission the server will never
// use. A deployment that can write advertises both, so a client that
// deliberately wants a credential which cannot break anything has one to ask
// for.
func TestSupportedScopes_FollowsWhatTheDeploymentCanDo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		readOnly bool
		safeMode bool
		want     []string
	}{
		{name: "a writing deployment offers both", want: []string{ScopeAPI, ScopeReadAPI}},
		{name: "read-only offers only the read scope", readOnly: true, want: []string{ScopeReadAPI}},
		{name: "safe mode offers only the read scope", safeMode: true, want: []string{ScopeReadAPI}},
		{name: "both together still offer only the read scope", readOnly: true, safeMode: true, want: []string{ScopeReadAPI}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := SupportedScopes(tt.readOnly, tt.safeMode); !slices.Equal(got, tt.want) {
				t.Errorf("SupportedScopes(%v, %v) = %v, want %v", tt.readOnly, tt.safeMode, got, tt.want)
			}
		})
	}
}

// TestNewGitLabVerifierFor_UnresolvableInstance_IsNotATokenVerdict covers the
// resolver failing before any credential is examined.
//
// The request named an instance this deployment does not publish, which says
// nothing about the token: the resolver's own error has to travel out
// unwrapped, so the layer above answers 403 for the instance rather than 401
// for the credential and neither the limiter nor the negative cache is charged.
func TestNewGitLabVerifierFor_UnresolvableInstance_IsNotATokenVerdict(t *testing.T) {
	t.Parallel()

	unpublished := errors.New("this deployment does not serve that instance")
	verifier := NewGitLabVerifierFor(
		func(*http.Request) (string, error) { return "", unpublished },
		false, 15*time.Minute, NewTokenCache(),
	)

	info, err := verifier(context.Background(), "any-token",
		httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", http.NoBody))

	if !errors.Is(err, unpublished) {
		t.Errorf("error = %v, want the resolver's own error", err)
	}
	if errors.Is(err, auth.ErrInvalidToken) {
		t.Error("an unresolvable instance was reported as an invalid token; the client would discard a working credential")
	}
	if info != nil {
		t.Errorf("info = %+v, want nothing identified", info)
	}
}

// TestNewGitLabVerifierFor_ForeignDefaultTransport_StillVerifies covers the
// fallback taken when http.DefaultTransport is not the standard one.
//
// A process that installs its own instrumented or mocked RoundTripper as the
// default has nothing to clone, and the verifier builds a plain transport
// rather than dereferencing what it found. Nothing about verification changes,
// which is what this asserts: the clone exists to inherit proxy and timeout
// settings, not to make the request work.
func TestNewGitLabVerifierFor_ForeignDefaultTransport_StillVerifies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/user" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gitlabUserResponse{ID: 9, Username: "foreign-transport"})
	}))
	defer srv.Close()

	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return original.RoundTrip(r)
	})

	verifier := NewGitLabVerifierFor(
		func(*http.Request) (string, error) { return srv.URL, nil },
		false, 15*time.Minute, nil,
	)
	info, err := verifier(context.Background(), "valid-token",
		httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", http.NoBody))
	if err != nil {
		t.Fatalf("verification failed with a non-standard DefaultTransport: %v", err)
	}
	if info.UserID != "9" {
		t.Errorf("UserID = %q, want the verified identity", info.UserID)
	}
}

// roundTripperFunc is a RoundTripper that is deliberately not an
// *http.Transport, which is the condition the fallback above exists for.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestEffectiveCacheTTL_NeverOutlivesTheToken covers how long an identity may
// be answered from memory.
//
// The configured TTL is a ceiling, not the answer: a token that expires in two
// minutes must not be cached for fifteen, or the server keeps admitting a
// credential GitLab has already stopped accepting. A token already past its
// expiry gets the smallest useful lifetime rather than a negative one, which
// would make the entry immediately stale in the other direction.
func TestEffectiveCacheTTL_NeverOutlivesTheToken(t *testing.T) {
	t.Parallel()

	const configured = 15 * time.Minute
	tests := []struct {
		name   string
		expiry time.Time
		want   time.Duration
	}{
		{name: "no expiry keeps the configured TTL", expiry: time.Time{}, want: configured},
		{name: "an expiry already past gets a second", expiry: time.Now().Add(-time.Hour), want: time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := effectiveCacheTTL(configured, tt.expiry); got != tt.want {
				t.Errorf("effectiveCacheTTL(%s, %v) = %s, want %s", configured, tt.expiry, got, tt.want)
			}
		})
	}

	t.Run("an expiry sooner than the TTL wins", func(t *testing.T) {
		t.Parallel()

		got := effectiveCacheTTL(configured, time.Now().Add(2*time.Minute))
		if got > 2*time.Minute || got <= 0 {
			t.Errorf("effectiveCacheTTL = %s, want at most the token's own two minutes", got)
		}
	})
}

// TestNewGitLabVerifier_CrossHostRedirect_IsRefused verifies that an instance
// answering /api/v4/user with a redirect to another host cannot supply the
// caller's identity, while a redirect that stays on the instance still works.
//
// Stripping the bearer, which is what the GitLab API clients do, would not be
// enough here: the value being trusted is the *response*, and a host reached
// by a redirect can answer {"id":1,"username":"root"} with no credential at
// all — and that answer would be admitted and cached under the caller's token.
// The test asserts the identity never comes from the redirect target and that
// the target is not even asked.
func TestNewGitLabVerifier_CrossHostRedirect_IsRefused(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		crossHost     bool
		wantErr       bool
		wantUsername  string
		wantTargetHit bool
	}{
		{name: "redirect leaving the instance", crossHost: true, wantErr: true},
		{name: "redirect staying on the instance", wantUsername: "impostor", wantTargetHit: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			targetHit := false
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				targetHit = true
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(gitlabUserResponse{ID: 1, Username: "impostor"})
			}))
			defer target.Close()

			// The Location header is set by hand rather than through
			// http.Redirect so that the destination, which is this test's own
			// server, is not read as a caller-controlled redirect target.
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL+r.URL.EscapedPath())
				w.WriteHeader(http.StatusFound)
			}))
			defer origin.Close()

			instanceURL := origin.URL
			if tt.crossHost {
				instanceURL = strings.Replace(instanceURL, "127.0.0.1", "localhost", 1)
			}

			verifier := NewGitLabVerifier(instanceURL, false, 15*time.Minute, nil)
			info, err := verifier(
				context.Background(),
				"valid-token",
				httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil),
			)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("verifier() error = nil, want a refusal; got identity %+v", info)
				}
				if targetHit {
					t.Error("the redirect target was asked for an identity")
				}
				return
			}
			if err != nil {
				t.Fatalf("verifier() unexpected error: %v", err)
			}
			if info.Extra["username"] != tt.wantUsername {
				t.Errorf("username = %v, want %q", info.Extra["username"], tt.wantUsername)
			}
			if targetHit != tt.wantTargetHit {
				t.Errorf("redirect target hit = %v, want %v", targetHit, tt.wantTargetHit)
			}
		})
	}
}

// TestVerificationRedirect_Policy verifies which hops a verification request
// may follow: the instance itself and its subdomains, never another host,
// never an https-to-http downgrade, never an address literal wearing the
// instance's name, and never more than ten in a row.
//
// The address-literal rows are CVE-2023-45289. url.Hostname reduces
// "[::1%25.gitlab.example.com]" to "::1%.gitlab.example.com", which clears
// both the dot boundary and the suffix test while Go dials ::1, so a
// loopback service would answer the question "who is this caller" and its
// reply would be cached as the caller's identity. The last row is the
// regression that guard must not cause: an instance that genuinely is an
// address literal still matches itself.
func TestVerificationRedirect_Policy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		origin  string
		dest    string
		hops    int
		wantErr bool
	}{
		{name: "same host", origin: "https://gitlab.example.com/api/v4/user", dest: "https://gitlab.example.com/api/v4/user/"},
		{name: "http upgraded to https", origin: "http://gitlab.example.com/api/v4/user", dest: "https://gitlab.example.com/api/v4/user"},
		{name: "subdomain of the instance", origin: "https://gitlab.example.com/api/v4/user", dest: "https://eu.gitlab.example.com/api/v4/user"},
		{name: "another host", origin: "https://gitlab.example.com/api/v4/user", dest: "https://evil.example.net/api/v4/user", wantErr: true},
		{name: "parent domain", origin: "https://gitlab.example.com/api/v4/user", dest: "https://example.com/api/v4/user", wantErr: true},
		{name: "suffix without a dot boundary", origin: "https://gitlab.example.com/api/v4/user", dest: "https://evilgitlab.example.com/x", wantErr: true},
		{name: "https downgraded to http", origin: "https://gitlab.example.com/api/v4/user", dest: "http://gitlab.example.com/api/v4/user", wantErr: true},
		{name: "ipv6 zone literal spelled as a subdomain", origin: "https://gitlab.example.com/api/v4/user", dest: "https://[::1%25.gitlab.example.com]/api/v4/user", wantErr: true},
		{name: "ipv6 zone literal on a plain http instance", origin: "http://gitlab.internal/api/v4/user", dest: "http://[::1%25.gitlab.internal]:9102/api/v4/user", wantErr: true},
		{name: "the instance is itself an ipv6 literal", origin: "https://[2001:db8::1]/api/v4/user", dest: "https://[2001:db8::1]/api/v4/user/"},
		{name: "ten hops already made", origin: "https://gitlab.example.com/api/v4/user", dest: "https://gitlab.example.com/api/v4/user", hops: 10, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			via := make([]*http.Request, max(tt.hops, 1))
			via[0] = redirectRequest(t, tt.origin)

			err := verificationRedirect(redirectRequest(t, tt.dest), via)
			if (err != nil) != tt.wantErr {
				t.Errorf("verificationRedirect() error = %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

// redirectRequest builds the value net/http hands a CheckRedirect policy: a
// request addressed at one end of a hop. Nothing is ever sent.
func redirectRequest(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	return httptest.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, nil)
}

// TestVerificationRedirect_MissingContext verifies the policy answers safely
// when net/http hands it a chain it cannot read: the first hop, which has no
// previous request to compare against, is allowed, and a request carrying no
// URL is refused rather than panicking.
func TestVerificationRedirect_MissingContext(t *testing.T) {
	t.Parallel()

	const instance = "https://gitlab.example.com/x"

	tests := []struct {
		name      string
		emptyVia  bool
		viaNoURL  bool
		destNoURL bool
		wantErr   bool
	}{
		{name: "no previous hop", emptyVia: true},
		{name: "previous hop with no url", viaNoURL: true},
		{name: "destination with no url", destNoURL: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var via []*http.Request
			switch {
			case tt.emptyVia:
				via = nil
			case tt.viaNoURL:
				via = []*http.Request{{}}
			default:
				via = []*http.Request{redirectRequest(t, instance)}
			}
			dest := redirectRequest(t, instance)
			if tt.destNoURL {
				dest = &http.Request{}
			}

			err := verificationRedirect(dest, via)
			if (err != nil) != tt.wantErr {
				t.Errorf("verificationRedirect() error = %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

// TestStringSlice_AcceptsTheSpaceSeparatedFormTheSpecificationsDefine covers
// the scope shape this code did not read.
//
// RFC 6749 defines scope as a space-delimited string and RFC 7662 repeats it
// for introspection. GitLab's /oauth/token/info answers with a JSON array
// instead, which is why the array form works and is why reading only the array
// went unnoticed: against gitlab.com it is right. Against a conforming
// authorization server it silently means "no scopes", and the caller then
// assumes api and admits a token it should have refused.
func TestStringSlice_AcceptsTheSpaceSeparatedFormTheSpecificationsDefine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  any
		want []string
	}{
		{name: "the array GitLab returns", raw: []any{"api", "read_user"}, want: []string{"api", "read_user"}},
		{name: "the string the specifications define", raw: "api read_user", want: []string{"api", "read_user"}},
		{name: "a single scope as a string", raw: "read_api", want: []string{"read_api"}},
		{name: "extra whitespace is not a scope", raw: "  api   read_api  ", want: []string{"api", "read_api"}},
		{name: "an empty string carries nothing", raw: "", want: nil},
		{name: "only whitespace carries nothing", raw: "   ", want: nil},
		{name: "a number is not a scope list", raw: 42, want: nil},
		{name: "an empty array carries nothing", raw: []any{}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := stringSlice(tt.raw)
			if !slices.Equal(got, tt.want) {
				t.Errorf("stringSlice(%#v) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}

// TestIntrospectToken_RefusedByBothEndpoints_ReportsNoScopesRatherThanAssumingAPI
// covers the distinction the fallback did not make.
//
// Assuming api when the instance cannot be asked is deliberate: an older
// instance, an unreachable endpoint, and refusing every such token would lock
// out deployments that work. But a 401 or 403 is an answer, not a gap. A
// credential that cannot read its own scopes will not read anything else
// either, and admitting it means failing on every tool call with an error from
// GitLab instead of one refusal at the door that says what to reauthorize.
func TestIntrospectToken_RefusedByBothEndpoints_ReportsNoScopesRatherThanAssumingAPI(t *testing.T) {
	t.Parallel()

	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(refusing.Close)

	got := introspectToken(t.Context(), refusing.Client(), refusing.URL, "gloas-whatever")

	if len(got.scopes) != 0 {
		t.Errorf("introspectToken() reported scopes %v for a token both endpoints refused to describe", got.scopes)
	}
	if !got.answered {
		t.Error("introspectToken() did not record that the instance answered, so the refusal reads as an outage")
	}
	if SatisfiesMinimum(got.scopes, MinimumScope) {
		t.Error("a token whose scopes could not be read was admitted; it would fail on every call instead")
	}
}

// TestIntrospectToken_UnreachableInstance_StillAssumesAPI verifies the other
// half, which is the reason the assumption exists at all.
//
// Nothing answered, so nothing is known. Refusing here would turn an
// introspection endpoint being absent or slow into a server that admits
// nobody, which is a worse failure than admitting a token that turns out to be
// under-scoped.
func TestIntrospectToken_UnreachableInstance_StillAssumesAPI(t *testing.T) {
	t.Parallel()

	// Closed immediately, so connections are refused rather than answered.
	down := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := down.URL
	down.Close()

	got := introspectToken(t.Context(), http.DefaultClient, url, "glpat-whatever")

	if !SatisfiesMinimum(got.scopes, MinimumScope) {
		t.Errorf("introspectToken() reported %v for an unreachable instance; an outage must not lock every token out", got.scopes)
	}
	if got.answered {
		t.Error("introspectToken() recorded an answer from an instance that never replied")
	}
}
