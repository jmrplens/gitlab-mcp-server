// metadata_test.go contains unit tests for the RFC 9728 Protected Resource
// Metadata endpoint handler.
package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNewProtectedResourceHandler_ValidResponse verifies that the handler
// returns a valid RFC 9728 Protected Resource Metadata JSON document with
// the expected resource, authorization server, bearer methods and scopes.
func TestNewProtectedResourceHandler_ValidResponse(t *testing.T) {
	handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", "https://gitlab.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if got := body["resource"]; got != "https://mcp.example.com/mcp" {
		t.Errorf("resource = %v, want %q", got, "https://mcp.example.com/mcp")
	}

	servers, ok := body["authorization_servers"].([]any)
	if !ok || len(servers) != 1 || servers[0] != "https://gitlab.example.com" {
		t.Errorf("authorization_servers = %v, want [%q]", body["authorization_servers"], "https://gitlab.example.com")
	}

	methods, ok := body["bearer_methods_supported"].([]any)
	if !ok || len(methods) != 1 || methods[0] != "header" {
		t.Errorf("bearer_methods_supported = %v, want [%q]", body["bearer_methods_supported"], "header")
	}

	scopes, ok := body["scopes_supported"].([]any)
	if !ok || len(scopes) == 0 {
		t.Fatalf("scopes_supported = %v, want at least 1 element", body["scopes_supported"])
	}
	if scopes[0] != "api" {
		t.Errorf("scopes_supported[0] = %v, want %q", scopes[0], "api")
	}
}

// TestNewProtectedResourceHandler_CORSHeaders verifies that the handler
// sets Access-Control-Allow-Origin: * so browser-based clients can fetch
// the metadata document cross-origin.
func TestNewProtectedResourceHandler_CORSHeaders(t *testing.T) {
	handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", "https://gitlab.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}

// TestNewProtectedResourceHandler_OptionsPreflightReturns204 verifies that
// OPTIONS preflight requests receive 204 No Content for CORS compliance.
func TestNewProtectedResourceHandler_OptionsPreflightReturns204(t *testing.T) {
	handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", "https://gitlab.example.com")

	req := httptest.NewRequest(http.MethodOptions, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

// TestNewProtectedResourceHandler_PostMethodNotAllowed verifies that POST
// and other non-GET/OPTIONS methods return 405 Method Not Allowed.
func TestNewProtectedResourceHandler_PostMethodNotAllowed(t *testing.T) {
	handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", "https://gitlab.example.com")

	req := httptest.NewRequest(http.MethodPost, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
