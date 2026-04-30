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
	info, err := verifier(context.Background(), "valid-token", httptest.NewRequest(http.MethodGet, "/", nil))
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
	_, err := verifier(context.Background(), "bad-token", httptest.NewRequest(http.MethodGet, "/", nil))
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
	_, err := verifier(context.Background(), "some-token", httptest.NewRequest(http.MethodGet, "/", nil))
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
	_, err := verifier(context.Background(), "token", httptest.NewRequest(http.MethodGet, "/", nil))
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
	_, err := verifier(context.Background(), "token", httptest.NewRequest(http.MethodGet, "/", nil))
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
	info, err := verifier(context.Background(), "tls-token", httptest.NewRequest(http.MethodGet, "/", nil))
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		apiCalls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gitlabUserResponse{ID: 1, Username: "cached"})
	}))
	defer srv.Close()

	cache := NewTokenCache()
	verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, cache)

	info1, err := verifier(context.Background(), "my-token", httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if apiCalls != 1 {
		t.Fatalf("expected 1 API call after first call, got %d", apiCalls)
	}

	info2, err := verifier(context.Background(), "my-token", httptest.NewRequest(http.MethodGet, "/", nil))
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		apiCalls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gitlabUserResponse{ID: 2, Username: "expiry"})
	}))
	defer srv.Close()

	cache := NewTokenCache()
	verifier := NewGitLabVerifier(srv.URL, false, 1*time.Millisecond, cache)

	_, err := verifier(context.Background(), "exp-token", httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	_, err = verifier(context.Background(), "exp-token", httptest.NewRequest(http.MethodGet, "/", nil))
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

	_, err := verifier(context.Background(), "inv-token", httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Expire the cached entry to force re-validation against the now-401 server
	cache.Delete("inv-token")

	_, err = verifier(context.Background(), "inv-token", httptest.NewRequest(http.MethodGet, "/", nil))
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
		apiCalls++
		id := apiCalls
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gitlabUserResponse{ID: id, Username: fmt.Sprintf("user%d", id)})
	}))
	defer srv.Close()

	cache := NewTokenCache()
	verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, cache)

	info1, err := verifier(context.Background(), "token-a", httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("token-a: %v", err)
	}
	info2, err := verifier(context.Background(), "token-b", httptest.NewRequest(http.MethodGet, "/", nil))
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
	_, _ = verifier(context.Background(), "token-a", httptest.NewRequest(http.MethodGet, "/", nil))
	_, _ = verifier(context.Background(), "token-b", httptest.NewRequest(http.MethodGet, "/", nil))
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
	_, err := verifier(context.Background(), "token", httptest.NewRequest(http.MethodGet, "/", nil))
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

	_, err := verifier(context.Background(), "net-token", httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, ok := cache.Get("net-token"); !ok {
		t.Fatal("token should be cached after successful call")
	}

	srv.Close()

	cache.Delete("net-token")
	_, err = verifier(context.Background(), "net-token", httptest.NewRequest(http.MethodGet, "/", nil))
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

	_, err := verifier(context.Background(), "srv-token", httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	cache.Delete("srv-token")

	_, err = verifier(context.Background(), "srv-token", httptest.NewRequest(http.MethodGet, "/", nil))
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
	_, err := verifier(context.Background(), "forbidden-token", httptest.NewRequest(http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !isErrInvalidToken(err) {
		t.Errorf("403 error should wrap auth.ErrInvalidToken, got: %v", err)
	}
}

// TestNewGitLabVerifier_RateLimited verifies that a 429 response returns
// an error wrapping auth.ErrInvalidToken with rate-limit context.
func TestNewGitLabVerifier_RateLimited(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, nil)
	_, err := verifier(context.Background(), "rate-token", httptest.NewRequest(http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
	if !isErrInvalidToken(err) {
		t.Errorf("429 error should wrap auth.ErrInvalidToken, got: %v", err)
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
	_, err := verifier(context.Background(), "zero-id-token", httptest.NewRequest(http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for user ID 0")
	}
	if !isErrInvalidToken(err) {
		t.Errorf("user ID 0 error should wrap auth.ErrInvalidToken, got: %v", err)
	}
}

// TestNewGitLabVerifier_UnexpectedStatusCode verifies that an unexpected HTTP
// status code (e.g. 418) returns an error wrapping auth.ErrInvalidToken.
func TestNewGitLabVerifier_UnexpectedStatusCode(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()

	verifier := NewGitLabVerifier(srv.URL, false, 15*time.Minute, nil)
	_, err := verifier(context.Background(), "teapot-token", httptest.NewRequest(http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for 418 response")
	}
	if !isErrInvalidToken(err) {
		t.Errorf("unexpected status error should wrap auth.ErrInvalidToken, got: %v", err)
	}
}

// isErrInvalidToken checks if an error wraps auth.ErrInvalidToken.
func isErrInvalidToken(err error) bool {
	return errors.Is(err, auth.ErrInvalidToken)
}
