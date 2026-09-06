// scope_filter_test.go contains unit tests for PAT scope-based tool filtering.
package tools

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
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
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: true})

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

// TestCatalogRelevantScopes_EqualComponentsFilterIdentically runs the filter
// over the property that makes the narrowed cache key safe: two token scope
// lists with the same canonical components produce the same filtered catalog
// and the same withheld sets, so serving both from one shared catalog serves
// neither a catalog it did not earn.
//
// It drives FilterActionCatalog rather than comparing keys, because the key
// is only correct in terms of what the filter does with a scope list: the
// ways this equivalence could be broken (a prefix match, a scope-implication
// rule, a rule keyed on absence or on the length of the list, a second
// requirements map) all leave [CatalogFilterKey] and [catalogRelevantScopes]
// looking exactly as they do now. The list of them is beside
// [catalogRelevantScopes].
//
// The last case is what keeps the rest from being vacuous: a list carrying
// none of the required scopes must filter differently from one carrying them
// all, or the filter would be removing nothing and every list would agree.
func TestCatalogRelevantScopes_EqualComponentsFilterIdentically(t *testing.T) {
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: true})
	required := requiredScopeUniverse(t)
	// Scopes GitLab issues that MetaToolScopes never asks about, so no list
	// below changes its canonical components by carrying them.
	noise := []string{"api", "read_api", "read_user", "read_repository", "write_repository", "read_registry", "write_registry", "create_runner", "manage_runner", "ai_features", "k8s_proxy", "sudo"}
	noise = slices.DeleteFunc(noise, func(scope string) bool { return slices.Contains(required, scope) })
	reversed := slices.Clone(required)
	slices.Reverse(reversed)

	cases := []struct {
		name  string
		left  []string
		right []string
	}{
		{
			name:  "the required scopes, alone and among scopes the filter never reads",
			left:  required,
			right: append(slices.Clone(noise), required...),
		},
		{
			name:  "the same scopes reordered, repeated and interleaved",
			left:  append(slices.Clone(required), required...),
			right: append(append(slices.Clone(reversed), noise...), reversed...),
		},
		{
			name:  "no required scope at all, alone and among the others",
			left:  []string{},
			right: noise,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if left, right := catalogRelevantScopes(tc.left), catalogRelevantScopes(tc.right); !slices.Equal(left, right) {
				t.Fatalf("the two lists have components %v and %v, so this case is not about the equivalence", left, right)
			}
			leftActions, leftWithheld := filterByTokenScopes(t, catalog, tc.left)
			rightActions, rightWithheld := filterByTokenScopes(t, catalog, tc.right)
			if !slices.Equal(leftActions, rightActions) {
				t.Errorf("the two lists kept %d and %d actions (%s), want one filtered catalog for both",
					len(leftActions), len(rightActions), firstDifference(leftActions, rightActions))
			}
			// Compared field by field and reported by count: the withheld
			// lists carry every alias of every removed action, and a failure
			// naming them all would be unreadable.
			if !slices.Equal(leftWithheld.ByTokenScope, rightWithheld.ByTokenScope) {
				t.Errorf("withheld by token scope = %d and %d keys (%s), want the same narrowing reported to both",
					len(leftWithheld.ByTokenScope), len(rightWithheld.ByTokenScope), firstDifference(leftWithheld.ByTokenScope, rightWithheld.ByTokenScope))
			}
			if !slices.Equal(leftWithheld.ByOperator, rightWithheld.ByOperator) || !slices.Equal(leftWithheld.ExcludedByName, rightWithheld.ExcludedByName) {
				t.Errorf("withheld by operator = %d and %d keys, excluded by name = %d and %d keys; want the same for both",
					len(leftWithheld.ByOperator), len(rightWithheld.ByOperator), len(leftWithheld.ExcludedByName), len(rightWithheld.ExcludedByName))
			}
		})
	}

	t.Run("a missing required scope filters differently", func(t *testing.T) {
		withAll, _ := filterByTokenScopes(t, catalog, required)
		withNone, withheld := filterByTokenScopes(t, catalog, []string{})
		if len(withNone) >= len(withAll) {
			t.Fatalf("a token with none of %v kept %d actions and one with all of them kept %d, want strictly fewer", required, len(withNone), len(withAll))
		}
		if len(withheld.ByTokenScope) == 0 {
			t.Error("nothing was reported withheld by token scope, so the equivalence above compares two catalogs the filter never narrowed")
		}
	})
}

// requiredScopeUniverse returns every scope MetaToolScopes requires, sorted
// and deduplicated: the whole of what a scope list's canonical components can
// be drawn from. Read from the map so a new requirement joins the test
// without an edit.
func requiredScopeUniverse(t *testing.T) []string {
	t.Helper()
	seen := map[string]struct{}{}
	for _, scopes := range MetaToolScopes {
		for _, scope := range scopes {
			seen[scope] = struct{}{}
		}
	}
	universe := slices.Sorted(maps.Keys(seen))
	if len(universe) == 0 {
		t.Fatal("MetaToolScopes requires no scope at all, so the equivalence proves nothing")
	}
	return universe
}

// firstDifference names the first position at which two lists disagree, for a
// failure message that has to stay readable over lists thousands of entries
// long.
func firstDifference(left, right []string) string {
	for i := range min(len(left), len(right)) {
		if left[i] != right[i] {
			return fmt.Sprintf("first difference at %d: %q against %q", i, left[i], right[i])
		}
	}
	return fmt.Sprintf("one is a prefix of the other, %d entries longer", max(len(left), len(right))-min(len(left), len(right)))
}

// filterByTokenScopes narrows a catalog for a token carrying scopes and
// returns the action IDs it kept, sorted, together with what the narrowing
// withheld.
func filterByTokenScopes(t *testing.T, catalog *actioncatalog.Catalog, scopes []string) ([]string, WithheldActions) {
	t.Helper()
	filtered, withheld, err := FilterActionCatalog(catalog, &config.ServerConfig{TokenScopes: scopes})
	if err != nil {
		t.Fatalf("FilterActionCatalog(%v) error = %v", scopes, err)
	}
	kept := make([]string, 0, filtered.CountActions())
	for _, action := range filtered.Actions() {
		kept = append(kept, string(action.ID))
	}
	slices.Sort(kept)
	return kept, withheld
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
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, &mcp.ServerOptions{PageSize: 2000, SchemaCache: testSchemaCache})
	if err := RegisterAllMeta(server, client, edition.Ultimate); err != nil {
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
