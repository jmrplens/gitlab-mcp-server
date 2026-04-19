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

func TestNewGitLabVerifier_InvalidURL(t *testing.T) {
	t.Parallel()

	// Control character in URL makes NewRequestWithContext fail
	verifier := NewGitLabVerifier("http://invalid\x00url", false, 15*time.Minute, nil)
	_, err := verifier(context.Background(), "token", httptest.NewRequest(http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

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

// isErrInvalidToken checks if an error wraps auth.ErrInvalidToken.
func isErrInvalidToken(err error) bool {
	return errors.Is(err, auth.ErrInvalidToken)
}
