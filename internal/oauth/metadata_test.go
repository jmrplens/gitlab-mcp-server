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
	handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", []string{"https://gitlab.example.com"}, []string{ScopeAPI}, ResourceLinks{})

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
			handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", []string{"https://gitlab.example.com"}, tt.want, ResourceLinks{})

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
	handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", []string{"https://gitlab.example.com"}, []string{ScopeAPI}, ResourceLinks{})

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
	handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", []string{"https://gitlab.example.com"}, []string{ScopeAPI}, ResourceLinks{})

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
	handler := NewProtectedResourceHandler("https://mcp.example.com/mcp", []string{"https://gitlab.example.com"}, []string{ScopeAPI}, ResourceLinks{})

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
				[]string{"https://gitlab.example.com"}, []string{ScopeAPI},
				ResourceLinks{Documentation: tt.configure})

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

// TestProtectedResourceHandler_OptionalLinksAreOmittedUnlessNamed pins how the
// two optional RFC 9728 URL fields behave.
//
// "resource_policy_uri ... OPTIONAL. URL of a page containing human-readable
// information about the protected resource's data usage policies", and the same
// shape for resource_tos_uri. Both describe undertakings a specific deployment
// makes about the data reached with the tokens it accepts, so neither has a
// default: this project cannot make those undertakings on an operator's behalf,
// and a field pointing at a page that does not exist would put a dead link on a
// consent screen, which is worse than an absent optional field.
//
// resource_documentation is the deliberate exception and keeps its default,
// since a client that finds no guidance at all is worse off than one sent to
// generic instructions.
func TestProtectedResourceHandler_OptionalLinksAreOmittedUnlessNamed(t *testing.T) {
	read := func(t *testing.T, links ResourceLinks) map[string]any {
		t.Helper()
		handler := NewProtectedResourceHandler("https://mcp.example.com/mcp",
			[]string{"https://gitlab.example.com"}, []string{ScopeAPI}, links)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/.well-known/oauth-protected-resource", nil))

		var document map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &document); err != nil {
			t.Fatalf("metadata is not JSON: %v (%s)", err, rec.Body.String())
		}
		return document
	}

	t.Run("neither is published by default", func(t *testing.T) {
		document := read(t, ResourceLinks{})
		for _, field := range []string{"resource_policy_uri", "resource_tos_uri"} {
			t.Run(field, func(t *testing.T) {
				if value, present := document[field]; present {
					t.Errorf("%s = %v, want it omitted when no page was named", field, value)
				}
			})
		}
		// The one that does have a default is still there.
		if document["resource_documentation"] != DefaultResourceDocumentation {
			t.Errorf("resource_documentation = %v, want the project default", document["resource_documentation"])
		}
	})

	t.Run("each appears when an operator names it", func(t *testing.T) {
		document := read(t, ResourceLinks{
			Policy:         "https://example.invalid/privacy",
			TermsOfService: "https://example.invalid/terms",
		})
		if got := document["resource_policy_uri"]; got != "https://example.invalid/privacy" {
			t.Errorf("resource_policy_uri = %v, want the configured page", got)
		}
		if got := document["resource_tos_uri"]; got != "https://example.invalid/terms" {
			t.Errorf("resource_tos_uri = %v, want the configured page", got)
		}
	})

	t.Run("one can be named without the other", func(t *testing.T) {
		document := read(t, ResourceLinks{Policy: "https://example.invalid/privacy"})
		if _, present := document["resource_policy_uri"]; !present {
			t.Error("resource_policy_uri is missing though it was configured")
		}
		if value, present := document["resource_tos_uri"]; present {
			t.Errorf("resource_tos_uri = %v, want it omitted; a deployment may have one page and not the other", value)
		}
	})
}
