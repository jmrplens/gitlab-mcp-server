// scopes_test.go contains unit tests for GitLab token scope validation
// and scope-checking helpers.

package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
