// scope_filter_test.go contains unit tests for PAT scope-based tool filtering.
package tools

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestRemoveScopeFilteredTools_NilScopes verifies that nil token scopes
// (detection unavailable) results in no tools removed.
func TestRemoveScopeFilteredTools_NilScopes(t *testing.T) {
	server := newMetaServer(t)
	removed := RemoveScopeFilteredTools(server, nil)
	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}
}

// TestRemoveScopeFilteredTools_AllScopesPresent verifies no tools are
// removed when the token has all required scopes.
func TestRemoveScopeFilteredTools_AllScopesPresent(t *testing.T) {
	server := newMetaServer(t)
	before := countTools(t, server)

	removed := RemoveScopeFilteredTools(server, []string{"api", "admin_mode", "read_api", "read_user"})
	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}

	after := countTools(t, server)
	if before != after {
		t.Errorf("tool count changed: before=%d after=%d", before, after)
	}
}

// TestRemoveScopeFilteredTools_MissingAdminMode verifies that tools
// requiring admin_mode are removed when that scope is absent.
func TestRemoveScopeFilteredTools_MissingAdminMode(t *testing.T) {
	server := newMetaServer(t)
	before := countTools(t, server)

	// Token has api but not admin_mode.
	removed := RemoveScopeFilteredTools(server, []string{"api", "read_api"})
	if removed == 0 {
		t.Fatal("expected some tools to be removed for missing admin_mode")
	}

	after := countTools(t, server)
	if after != before-removed {
		t.Errorf("tool count mismatch: before=%d removed=%d after=%d", before, removed, after)
	}
}

// TestRemoveScopeFilteredTools_ReadOnlyToken verifies that a read-only
// token causes admin_mode-requiring tools to be removed.
func TestRemoveScopeFilteredTools_ReadOnlyToken(t *testing.T) {
	server := newMetaServer(t)

	// Token with only read_api — all tools requiring "admin_mode" should be removed.
	removed := RemoveScopeFilteredTools(server, []string{"read_api"})
	if removed == 0 {
		t.Fatal("expected tools to be removed for read-only token")
	}

	// Verify the admin tool was removed.
	names := toolNames(t, server)
	for _, name := range names {
		if name == "gitlab_admin" {
			t.Error("gitlab_admin should have been removed for read-only token")
		}
	}
}

// TestRemoveScopeFilteredTools_EmptyScopes verifies that an empty scope
// list (token detected but no scopes) removes all scope-gated tools.
func TestRemoveScopeFilteredTools_EmptyScopes(t *testing.T) {
	server := newMetaServer(t)

	removed := RemoveScopeFilteredTools(server, []string{})
	if removed == 0 {
		t.Fatal("expected all scope-gated tools to be removed")
	}
}

// TestFilterScopeFilteredCatalog_MissingAdminMode verifies that catalog-level
// scope filtering removes the same admin-mode groups without mutating the source.
func TestFilterScopeFilteredCatalog_MissingAdminMode(t *testing.T) {
	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{Enterprise: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	t.Run("source contains admin", func(t *testing.T) {
		if _, ok := catalog.Group("gitlab_admin"); !ok {
			t.Fatal("source catalog missing gitlab_admin")
		}
	})

	t.Run("removes admin and preserves project", func(t *testing.T) {
		filtered, filterErr := FilterScopeFilteredCatalog(catalog, []string{"read_api"})
		if filterErr != nil {
			t.Fatalf("FilterScopeFilteredCatalog() error = %v", filterErr)
		}
		if _, ok := filtered.Group("gitlab_admin"); ok {
			t.Fatal("filtered catalog still contains gitlab_admin")
		}
		if _, ok := filtered.Group("gitlab_project"); !ok {
			t.Fatal("filtered catalog removed ungated gitlab_project")
		}
	})

	t.Run("source remains unchanged", func(t *testing.T) {
		if _, filterErr := FilterScopeFilteredCatalog(catalog, []string{"read_api"}); filterErr != nil {
			t.Fatalf("FilterScopeFilteredCatalog() error = %v", filterErr)
		}
		if _, ok := catalog.Group("gitlab_admin"); !ok {
			t.Fatal("source catalog was mutated")
		}
	})

	t.Run("nil scopes return clone", func(t *testing.T) {
		unfiltered, filterErr := FilterScopeFilteredCatalog(catalog, nil)
		if filterErr != nil {
			t.Fatalf("FilterScopeFilteredCatalog(nil) error = %v", filterErr)
		}
		if unfiltered == catalog {
			t.Fatal("nil token scopes should return a cloned catalog")
		}
		if unfiltered.CountGroups() != catalog.CountGroups() {
			t.Fatalf("nil-scope group count = %d, want %d", unfiltered.CountGroups(), catalog.CountGroups())
		}
	})
}

// TestFilterScopeFilteredCatalog_NilCatalog verifies scope filtering handles a
// nil source catalog by returning an empty catalog.
//
// The test expects no error, a non-nil result, and zero groups or actions. This
// keeps callers safe when filtering is invoked before catalog construction.
func TestFilterScopeFilteredCatalog_NilCatalog(t *testing.T) {
	filtered, err := FilterScopeFilteredCatalog(nil, []string{"read_api"})
	if err != nil {
		t.Fatalf("FilterScopeFilteredCatalog(nil) error = %v", err)
	}
	if filtered == nil {
		t.Fatal("FilterScopeFilteredCatalog(nil) returned nil catalog")
	}
	if filtered.CountGroups() != 0 || filtered.CountActions() != 0 {
		t.Fatalf("filtered counts = groups %d actions %d, want empty catalog", filtered.CountGroups(), filtered.CountActions())
	}
}

// TestAllScopesPresent_Scenarios_CorrectResult tests the allScopesPresent helper.
func TestAllScopesPresent_Scenarios_CorrectResult(t *testing.T) {
	tests := []struct {
		name     string
		scopes   map[string]struct{}
		required []string
		want     bool
	}{
		{
			name:     "empty required",
			scopes:   map[string]struct{}{"api": {}},
			required: nil,
			want:     true,
		},
		{
			name:     "all present",
			scopes:   map[string]struct{}{"api": {}, "admin_mode": {}},
			required: []string{"api", "admin_mode"},
			want:     true,
		},
		{
			name:     "one missing",
			scopes:   map[string]struct{}{"api": {}},
			required: []string{"api", "admin_mode"},
			want:     false,
		},
		{
			name:     "all missing",
			scopes:   map[string]struct{}{},
			required: []string{"api"},
			want:     false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := allScopesPresent(tc.scopes, tc.required)
			if got != tc.want {
				t.Errorf("allScopesPresent() = %v, want %v", got, tc.want)
			}
		})
	}
}

// newMetaServer creates an MCP server with all meta-tools registered
// (enterprise enabled) for testing scope filtering.
func newMetaServer(t *testing.T) *mcp.Server {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"version":"17.0.0"}`))
	})
	client := newTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, &mcp.ServerOptions{PageSize: 2000})
	if err := RegisterAllMeta(server, client, true); err != nil {
		t.Fatalf("RegisterAllMeta() error = %v", err)
	}
	return server
}

// countTools returns the number of tools registered on the server.
func countTools(t *testing.T, server *mcp.Server) int {
	t.Helper()
	names := toolNames(t, server)
	return len(names)
}

// toolNames returns the names of all tools registered on the server.
func toolNames(t *testing.T, server *mcp.Server) []string {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var names []string
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	return names
}
