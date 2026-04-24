// middleware_test.go contains unit tests for the OAuth authentication
// header normalization middleware.

package oauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNormalizeAuthHeader_ConvertsPrivateToken verifies that NormalizeAuthHeader
// rewrites a PRIVATE-TOKEN header into an Authorization: Bearer header for the
// downstream handler.
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

// TestNormalizeAuthHeader_PreservesExistingBearer verifies that an existing
// Authorization: Bearer header is preserved and the PRIVATE-TOKEN header is
// ignored when both are present.
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

// TestNormalizeAuthHeader_NoAuthHeaders verifies that requests without any
// auth headers pass through unchanged with an empty Authorization header.
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

// TestNormalizeAuthHeader_NonBearerAuth verifies that a non-Bearer
// Authorization header (e.g. Basic) is preserved verbatim and PRIVATE-TOKEN
// is ignored.
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
