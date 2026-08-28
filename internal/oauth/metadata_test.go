// metadata_test.go contains unit tests for the RFC 9728 Protected Resource
// Metadata endpoint handler.
package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNewProtectedResourceHandler_ValidResponse verifies that the handler
// returns a valid RFC 9728 Protected Resource Metadata JSON document with
// the expected resource, authorization server, bearer methods and scopes.
func TestNewProtectedResourceHandler_ValidResponse(t *testing.T) {
	handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", []string{"https://gitlab.example.com"}, []string{ScopeAPI}, "")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/.well-known/oauth-protected-resource", nil)
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

// TestNewProtectedResourceHandler_AdvertisesTheScopesItAccepts verifies that
// scopes_supported carries what the deployment was built with rather than a
// constant. A client reads this field to decide what to ask GitLab for, so a
// read-only server advertising "api" would make every user grant write access
// the server can never use — and a writing server advertising only "api"
// would leave a client that deliberately wants a read-only credential no
// documented way to ask for one.
func TestNewProtectedResourceHandler_AdvertisesTheScopesItAccepts(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{name: "writing deployment offers both", want: []string{ScopeAPI, ScopeReadAPI}},
		{name: "read-only deployment offers read_api alone", want: []string{ScopeReadAPI}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", []string{"https://gitlab.example.com"}, tt.want, "")

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/.well-known/oauth-protected-resource", http.NoBody)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			var body map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode JSON: %v", err)
			}
			scopes, ok := body["scopes_supported"].([]any)
			if !ok || len(scopes) != len(tt.want) {
				t.Fatalf("scopes_supported = %v, want %v", body["scopes_supported"], tt.want)
			}
			for i, want := range tt.want {
				if scopes[i] != want {
					t.Errorf("scopes_supported[%d] = %v, want %q", i, scopes[i], want)
				}
			}
		})
	}
}

// TestNewProtectedResourceHandler_CORSHeaders verifies that the handler
// sets Access-Control-Allow-Origin: * so browser-based clients can fetch
// the metadata document cross-origin.
func TestNewProtectedResourceHandler_CORSHeaders(t *testing.T) {
	handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", []string{"https://gitlab.example.com"}, []string{ScopeAPI}, "")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}

// TestNewProtectedResourceHandler_OptionsPreflightReturns204 verifies that
// OPTIONS preflight requests receive 204 No Content for CORS compliance.
func TestNewProtectedResourceHandler_OptionsPreflightReturns204(t *testing.T) {
	handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", []string{"https://gitlab.example.com"}, []string{ScopeAPI}, "")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

// TestNewProtectedResourceHandler_PostMethodNotAllowed verifies that POST
// and other non-GET/OPTIONS methods return 405 Method Not Allowed.
func TestNewProtectedResourceHandler_PostMethodNotAllowed(t *testing.T) {
	handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", []string{"https://gitlab.example.com"}, []string{ScopeAPI}, "")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// TestNewProtectedResourceHandler_ResourceDocumentationIsConfigurable verifies
// that an operator can point RFC 9728's resource_documentation field at their
// own page, and that an empty value falls back to this project's guide. The
// field is the only sanctioned way to lead a client to the OAuth application it
// should use, since RFC 9728 defines no field carrying a client identifier.
func TestNewProtectedResourceHandler_ResourceDocumentationIsConfigurable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure string
		want      string
	}{
		{name: "empty falls back to the project guide", configure: "", want: DefaultResourceDocumentation},
		{name: "operator page is published verbatim", configure: "https://ops.example.com/our-oauth-app", want: "https://ops.example.com/our-oauth-app"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := NewProtectedResourceHandler("https://mcp.example.com/mcp",
				[]string{"https://gitlab.example.com"}, []string{ScopeAPI}, tt.configure)

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/.well-known/oauth-protected-resource", nil))

			var metadata struct {
				ResourceDocumentation string `json:"resource_documentation"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&metadata); err != nil {
				t.Fatalf("decode metadata: %v", err)
			}
			if metadata.ResourceDocumentation != tt.want {
				t.Errorf("resource_documentation = %q, want %q", metadata.ResourceDocumentation, tt.want)
			}
		})
	}
}
