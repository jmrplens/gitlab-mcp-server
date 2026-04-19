package oauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeAuthHeader_ConvertsPrivateToken(t *testing.T) {
	var captured http.Header
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("PRIVATE-TOKEN", "glpat-abc123")

	rec := httptest.NewRecorder()
	NormalizeAuthHeader(inner).ServeHTTP(rec, req)

	if got := captured.Get("Authorization"); got != "Bearer glpat-abc123" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer glpat-abc123")
	}
}

func TestNormalizeAuthHeader_PreservesExistingBearer(t *testing.T) {
	var captured http.Header
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer oauth-token")
	req.Header.Set("PRIVATE-TOKEN", "glpat-should-be-ignored")

	rec := httptest.NewRecorder()
	NormalizeAuthHeader(inner).ServeHTTP(rec, req)

	if got := captured.Get("Authorization"); got != "Bearer oauth-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer oauth-token")
	}
}

func TestNormalizeAuthHeader_NoAuthHeaders(t *testing.T) {
	var captured http.Header
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	rec := httptest.NewRecorder()
	NormalizeAuthHeader(inner).ServeHTTP(rec, req)

	if got := captured.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want empty", got)
	}
}

func TestNormalizeAuthHeader_NonBearerAuth(t *testing.T) {
	var captured http.Header
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	req.Header.Set("PRIVATE-TOKEN", "glpat-should-be-ignored")

	rec := httptest.NewRecorder()
	NormalizeAuthHeader(inner).ServeHTTP(rec, req)

	if got := captured.Get("Authorization"); got != "Basic dXNlcjpwYXNz" {
		t.Errorf("Authorization = %q, want %q", got, "Basic dXNlcjpwYXNz")
	}
}
