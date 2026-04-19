package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
	if !ok || len(scopes) != 3 {
		t.Fatalf("scopes_supported = %v, want 3 elements", body["scopes_supported"])
	}
	expectedScopes := map[string]bool{"api": true, "read_user": true, "read_api": true}
	for _, s := range scopes {
		if !expectedScopes[s.(string)] {
			t.Errorf("unexpected scope %q", s)
		}
	}
}

func TestNewProtectedResourceHandler_CORSHeaders(t *testing.T) {
	handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", "https://gitlab.example.com")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}

func TestNewProtectedResourceHandler_OptionsPreflightReturns204(t *testing.T) {
	handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", "https://gitlab.example.com")

	req := httptest.NewRequest(http.MethodOptions, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestNewProtectedResourceHandler_PostMethodNotAllowed(t *testing.T) {
	handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", "https://gitlab.example.com")

	req := httptest.NewRequest(http.MethodPost, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
