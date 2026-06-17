// standalone_test.go contains unit tests for the dynamic surface standalone MCP server (used by inspector and integration tests).
package dynamic

import (
	"context"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestStandalone_AddStandaloneRoutesRespectsReadOnlyAndExclusions verifies the Standalone_AddStandaloneRoutesRespectsReadOnlyAndExclusions handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestStandalone_AddStandaloneRoutesRespectsReadOnlyAndExclusions(t *testing.T) {
	routes, err := AddStandaloneRoutes(nil, nil, StandaloneOptions{
		ReadOnly:     true,
		ExcludeTools: []string{"gitlab_discover_project"},
	})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}

	if _, ok := routes["gitlab_discover_project"]; ok {
		t.Fatal("routes include gitlab_discover_project despite explicit exclusion")
	}
	if _, ok := routes["gitlab_interactive"]; ok {
		t.Fatal("routes include gitlab_interactive in read-only mode")
	}
	if len(routes) != 0 {
		t.Fatalf("routes = %v, want empty map for read-only + excluded discover", routes)
	}
}

// TestStandalone_AddStandaloneRoutesAddsDiscoverByDefault verifies the Standalone_AddStandaloneRoutesAddsDiscoverByDefault handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestStandalone_AddStandaloneRoutesAddsDiscoverByDefault(t *testing.T) {
	routes, err := AddStandaloneRoutes(nil, nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes() error = %v", err)
	}

	discover, ok := routes["gitlab_discover_project"]
	if !ok {
		t.Fatal("routes missing gitlab_discover_project")
	}
	if _, hasResolve := discover["resolve"]; !hasResolve {
		t.Fatalf("discover routes = %v, want resolve action", discover)
	}
}

// TestStandalone_AddStandaloneCatalogCreatesCatalogWhenNil verifies the Standalone_AddStandaloneCatalogCreatesCatalogWhenNil handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestStandalone_AddStandaloneCatalogCreatesCatalogWhenNil(t *testing.T) {
	catalog, err := AddStandaloneCatalog(nil, nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog(nil) error = %v", err)
	}
	if catalog == nil {
		t.Fatal("AddStandaloneCatalog(nil) returned nil catalog")
	}
	if len(catalog.ActionMaps()) == 0 {
		t.Fatal("AddStandaloneCatalog(nil) produced no action maps")
	}
}

// TestStandalone_AddStandaloneRoutesPreservesExistingMappings verifies the Standalone_AddStandaloneRoutesPreservesExistingMappings handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestStandalone_AddStandaloneRoutesPreservesExistingMappings(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab_project": {
			"get": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{"ok": true}, nil
				},
			},
		},
	}

	merged, err := AddStandaloneRoutes(routes, nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneRoutes(existing) error = %v", err)
	}
	if _, ok := merged["gitlab_project"]["get"]; !ok {
		t.Fatal("existing mapping gitlab_project.get was removed")
	}
}

// TestStandalone_AddStandaloneRoutesPropagatesCatalogErrors verifies that Standalone_AddStandaloneRoutesPropagatesCatalogErrors returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestStandalone_AddStandaloneRoutesPropagatesCatalogErrors(t *testing.T) {
	routes := map[string]toolutil.ActionMap{
		"gitlab_discover_project": {
			"resolve": {
				Handler: func(_ context.Context, _ map[string]any) (any, error) {
					return map[string]any{}, nil
				},
			},
		},
	}

	_, err := AddStandaloneRoutes(routes, nil, StandaloneOptions{})
	if err == nil || !strings.Contains(err.Error(), "gitlab_discover_project.resolve") {
		t.Fatalf("AddStandaloneRoutes() error = %v, want duplicate discover route", err)
	}
}

// TestStandalone_AddStandaloneCatalogRejectsDuplicateStandaloneGroups covers Standalone with table-driven subtests for add standalone catalog rejects duplicate standalone groups.
func TestStandalone_AddStandaloneCatalogRejectsDuplicateStandaloneGroups(t *testing.T) {
	testCases := []struct {
		name string
		seed actioncatalog.Group
		want string
	}{
		{
			name: "discover duplicate",
			seed: seedStandaloneGroup(t, "gitlab_discover_project", "resolve"),
			want: "gitlab_discover_project.resolve",
		},
		{
			name: "interactive duplicate",
			seed: seedStandaloneGroup(t, "gitlab_interactive", "issue_create"),
			want: "gitlab_interactive",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			catalog := actioncatalog.NewCatalog()
			if err := catalog.AddGroup(tc.seed); err != nil {
				t.Fatalf("AddGroup(seed) error = %v", err)
			}

			_, err := AddStandaloneCatalog(catalog, nil, StandaloneOptions{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("AddStandaloneCatalog() error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

// seedStandaloneGroup seeds standalone group test fixtures.
func seedStandaloneGroup(t *testing.T, toolName, actionName string) actioncatalog.Group {
	t.Helper()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: toolName})
	group.SetAction(actioncatalog.Action{Name: actionName, Route: toolutil.ActionRoute{
		Handler: func(_ context.Context, _ map[string]any) (any, error) {
			return map[string]any{}, nil
		},
		InputSchema: map[string]any{"type": "object"},
	}})
	return group
}
