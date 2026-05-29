package dynamic

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

// TestControllerSurfaceSpecs_ClassifyDynamicControllers verifies ControllerSurfaceSpecs when classify dynamic controllers.
func TestControllerSurfaceSpecs_ClassifyDynamicControllers(t *testing.T) {
	specs := ControllerSurfaceSpecs(nil)
	if len(specs) != 2 {
		t.Fatalf("ControllerSurfaceSpecs() len = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if err := spec.Validate(); err != nil {
			t.Fatalf("spec %s Validate() error = %v", spec.Name, err)
		}
		if spec.SurfaceKind != actioncatalog.SurfaceKindDynamicController || spec.GroupToolName != "gitlab_dynamic" || spec.OwnerPackage != "dynamic" {
			t.Fatalf("spec %s = %+v, want dynamic controller metadata", spec.Name, spec)
		}
	}

	execute := findDynamicSurfaceSpec(t, specs, executeActionToolName)
	if !execute.Destructive || execute.ReadOnly || execute.Route.OutputSchema == nil {
		t.Fatalf("execute spec = %+v, want potentially destructive controller with generic output schema", execute)
	}
	if execute.Description != executeActionToolDescription || !strings.Contains(execute.Description, "confirm=true") {
		t.Fatalf("execute description = %q, want shared confirmation guidance", execute.Description)
	}

	find := findDynamicSurfaceSpec(t, specs, findToolName)
	if !find.ReadOnly || find.Destructive {
		t.Fatalf("find spec = %+v, want read-only controller", find)
	}
	if find.Description != findToolDescription || !strings.Contains(find.Description, "no GitLab API call") || !strings.Contains(find.Description, "execute examples") {
		t.Fatalf("find description = %q, want shared lookup guidance", find.Description)
	}
}

// TestControllerSurfaceSpecs_RouteHandlers verifies controller specs execute
// through their wrapped dynamic registry routes.
func TestControllerSurfaceSpecs_RouteHandlers(t *testing.T) {
	specs := ControllerSurfaceSpecs(NewRegistry(testRoutes(t)))

	find := findDynamicSurfaceSpec(t, specs, findToolName)
	if _, err := find.Route.Handler(t.Context(), map[string]any{"query": "project get", "limit": 1}); err != nil {
		t.Fatalf("find route error = %v", err)
	}

	execute := findDynamicSurfaceSpec(t, specs, executeActionToolName)
	if _, err := execute.Route.Handler(t.Context(), map[string]any{"action": "missing.action", "params": map[string]any{}}); err != nil {
		t.Fatalf("execute route error = %v", err)
	}
}

// findDynamicSurfaceSpec locates dynamic surface spec fixture data for assertions.
func findDynamicSurfaceSpec(t *testing.T, specs []actioncatalog.SurfaceToolSpec, name string) actioncatalog.SurfaceToolSpec {
	t.Helper()
	for _, spec := range specs {
		if spec.Name == name {
			return spec
		}
	}
	t.Fatalf("dynamic surface spec %q not found in %+v", name, specs)
	return actioncatalog.SurfaceToolSpec{}
}
