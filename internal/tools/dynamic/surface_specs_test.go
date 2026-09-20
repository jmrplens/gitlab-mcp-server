// surface_specs_test.go contains unit tests for the dynamic surface metadata and the two-tools contract.
package dynamic

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestControllerSurfaceSpecs_ClassifyDynamicControllers verifies the metadata
// the two controller specs carry: both validate as dynamic controllers of the
// dynamic group and package, execute is destructive and carries an output
// schema, find is read-only, and each serves the shared description with the
// phrases a model relies on.
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

// TestControllerSurfaceSpecs_CarryTheirHintsAndIcons verifies the rest of what
// each spec declares: execute is non-idempotent and its route is itself marked
// destructive, find is idempotent, both are open-world, and each carries its
// own icon set. The route flag matters most: a surface built from this spec
// dispatches through the route, so a route marked safe would let a read-only
// or safe-mode surface run every catalog mutation through the one tool that
// reaches them all.
func TestControllerSurfaceSpecs_CarryTheirHintsAndIcons(t *testing.T) {
	specs := ControllerSurfaceSpecs(nil)
	cases := []struct {
		name             string
		spec             actioncatalog.SurfaceToolSpec
		wantIdempotent   bool
		wantRouteDestroy bool
		wantIcons        []mcp.Icon
	}{
		{name: "execute", spec: findDynamicSurfaceSpec(t, specs, executeActionToolName), wantRouteDestroy: true, wantIcons: toolutil.IconServer},
		{name: "find", spec: findDynamicSurfaceSpec(t, specs, findToolName), wantIdempotent: true, wantIcons: toolutil.IconSearch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.spec.Idempotent != tc.wantIdempotent || !tc.spec.OpenWorld {
				t.Errorf("idempotent = %t, open world = %t; want idempotent %t and open world", tc.spec.Idempotent, tc.spec.OpenWorld, tc.wantIdempotent)
			}
			if tc.spec.Route.Destructive != tc.wantRouteDestroy {
				t.Errorf("route Destructive = %t, want %t", tc.spec.Route.Destructive, tc.wantRouteDestroy)
			}
			if !reflect.DeepEqual(tc.spec.Icons, tc.wantIcons) {
				t.Errorf("icons = %+v, want %+v", tc.spec.Icons, tc.wantIcons)
			}
		})
	}
}

// TestControllerSurfaceSpecs_RouteHandlers verifies that both controller
// routes run against a populated registry without a Go error: a find for an
// action the registry holds, and an execute of one it does not. Nothing more
// is asserted about what either route answered.
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

// TestControllerSurfaceSpecs_SearchTheRegistryTheyWereGiven verifies that the
// find route searches the registry passed to ControllerSurfaceSpecs, and that
// a nil one is answered by an empty registry rather than by a dereference of
// nothing. The substitution is only for the metadata callers that never run a
// route, so replacing a registry that was supplied would leave the served find
// tool searching an empty catalog while still answering without an error.
func TestControllerSurfaceSpecs_SearchTheRegistryTheyWereGiven(t *testing.T) {
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_project"})
	group.SetAction(actioncatalog.Action{
		Name:  "get",
		Route: toolutil.Route(func(_ context.Context, _ map[string]any) (any, error) { return struct{}{}, nil }),
	})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	tests := []struct {
		name      string
		registry  *Registry
		wantFound bool
	}{
		{name: "registry with the action", registry: NewRegistryFromCatalog(catalog), wantFound: true},
		{name: "nil registry", registry: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			find := findDynamicSurfaceSpec(t, ControllerSurfaceSpecs(tt.registry), findToolName)
			result, err := find.Route.Handler(t.Context(), map[string]any{"query": "project get", "limit": 5})
			if err != nil {
				t.Fatalf("find route error = %v", err)
			}
			output, ok := result.(FindOutput)
			if !ok {
				t.Fatalf("find route returned %T, want FindOutput", result)
			}
			if (output.Count > 0) != tt.wantFound {
				t.Errorf("find returned %d results, want found = %v", output.Count, tt.wantFound)
			}
		})
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
