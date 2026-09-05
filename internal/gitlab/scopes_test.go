// scopes_test.go contains unit tests for GitLab token scope validation
// and scope-checking helpers.
package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// TestDetectScopes_Success verifies that DetectScopes returns the scopes reported by the /personal_access_tokens/self endpoint.
func TestDetectScopes_Success(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/personal_access_tokens/self", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     1,
			"scopes": []string{"api", "read_user"},
			"active": true,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	scopes := DetectScopes(context.Background(), client.GL())
	if len(scopes) != 2 {
		t.Fatalf("expected 2 scopes, got %d: %v", len(scopes), scopes)
	}
	if scopes[0] != "api" || scopes[1] != "read_user" {
		t.Errorf("unexpected scopes: %v", scopes)
	}
}

// TestDetectScopes_EndpointNotAvailable verifies that DetectScopes returns nil when the scope endpoint responds with 404.
func TestDetectScopes_EndpointNotAvailable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/personal_access_tokens/self", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := NewClient(newTestConfig(srv.URL, testValidToken))
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	scopes := DetectScopes(context.Background(), client.GL())
	if scopes != nil {
		t.Errorf("expected nil scopes on 404, got %v", scopes)
	}
}

// TestScopeSatisfied_Scenarios_CorrectResult uses table-driven subtests to verify that ScopeSatisfied correctly reports whether the token scopes cover the required scopes across nil, empty, exact, partial, and missing combinations.
func TestScopeSatisfied_Scenarios_CorrectResult(t *testing.T) {
	tests := []struct {
		name     string
		token    []string
		required []string
		want     bool
	}{
		{"nil token scopes allows all", nil, []string{"api"}, true},
		{"empty required always satisfied", []string{"api"}, nil, true},
		{"exact match", []string{"api", "read_user"}, []string{"api"}, true},
		{"multiple required all present", []string{"api", "read_user", "sudo"}, []string{"api", "sudo"}, true},
		{"missing required scope", []string{"read_user"}, []string{"api"}, false},
		{"partial match fails", []string{"api"}, []string{"api", "sudo"}, false},
		{"both empty", []string{}, []string{}, true},
		{"empty token with requirement", []string{}, []string{"api"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScopeSatisfied(tt.token, tt.required)
			if got != tt.want {
				t.Errorf("ScopeSatisfied(%v, %v) = %v, want %v", tt.token, tt.required, got, tt.want)
			}
		})
	}
}

// TestNarrowToTokenScope_NarrowsOnlyATokenThatCannotWrite verifies the one
// decision both transports share: a token without the api scope makes the
// configuration read-only and marks the narrowing as the token's, unknown
// scopes narrow nothing, and a configuration already read-only is left as the
// operator set it.
func TestNarrowToTokenScope_NarrowsOnlyATokenThatCannotWrite(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		cfg           *config.ServerConfig
		wantNarrowed  bool
		wantReadOnly  bool
		wantFromScope bool
	}{
		{name: "read_api narrows", cfg: &config.ServerConfig{TokenScopes: []string{"read_api"}}, wantNarrowed: true, wantReadOnly: true, wantFromScope: true},
		{name: "api stays writable", cfg: &config.ServerConfig{TokenScopes: []string{"api", "read_user"}}},
		{name: "unknown scopes stay writable", cfg: &config.ServerConfig{}},
		{name: "empty scopes narrow", cfg: &config.ServerConfig{TokenScopes: []string{}}, wantNarrowed: true, wantReadOnly: true, wantFromScope: true},
		{name: "the operator's read-only is not the token's", cfg: &config.ServerConfig{ReadOnly: true, TokenScopes: []string{"read_api"}}, wantReadOnly: true},
		{name: "nil configuration", cfg: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NarrowToTokenScope(tt.cfg); got != tt.wantNarrowed {
				t.Errorf("NarrowToTokenScope() = %v, want %v", got, tt.wantNarrowed)
			}
			if tt.cfg == nil {
				return
			}
			if tt.cfg.ReadOnly != tt.wantReadOnly || tt.cfg.ReadOnlyFromTokenScope != tt.wantFromScope {
				t.Errorf("ReadOnly = %v (from scope %v), want %v (from scope %v)", tt.cfg.ReadOnly, tt.cfg.ReadOnlyFromTokenScope, tt.wantReadOnly, tt.wantFromScope)
			}
		})
	}
}
