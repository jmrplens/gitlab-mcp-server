package dynamic

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func TestStandalone_ExcludedToolName(t *testing.T) {
	if !standaloneExcluded([]string{"gitlab_search_tools"}, "gitlab_search_tools") {
		t.Fatal("standaloneExcluded() = false, want true for configured tool")
	}
	if standaloneExcluded([]string{"gitlab_search_tools"}, "gitlab_describe_tools") {
		t.Fatal("standaloneExcluded() = true, want false for different tool")
	}
}

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
