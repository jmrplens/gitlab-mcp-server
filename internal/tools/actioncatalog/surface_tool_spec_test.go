package actioncatalog

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

type surfaceSpecInput struct {
	Value string `json:"value" jsonschema:"value to echo"`
}

type surfaceSpecOutput struct {
	OK bool `json:"ok" jsonschema:"operation result"`
}

func TestSurfaceToolSpec_ActionSpec_PreservesCatalogMetadata(t *testing.T) {
	route := toolutil.RouteFunc(func(context.Context, surfaceSpecInput) (surfaceSpecOutput, error) {
		return surfaceSpecOutput{OK: true}, nil
	})
	spec := SurfaceToolSpec{
		Name:          "gitlab_test_surface",
		Title:         "Test Surface",
		Description:   "Test surface utility.",
		GroupToolName: "gitlab_test",
		BaseDomain:    "test",
		ActionName:    "surface",
		SurfaceKind:   SurfaceKindRuntimeUtility,
		Route:         route,
		Aliases:       []string{"surface_alias"},
		Tags:          []string{"utility"},
		RelatedActions: []string{
			"test.related",
		},
		Compatibility: toolutil.CompatibilityPolicy{ActionAliases: []toolutil.ActionAliasSpec{{
			Alias:      "gitlab_test_surface",
			Target:     "surface",
			Source:     "compatibility",
			Searchable: true,
			Reason:     "historical tool name",
		}}},
		OwnerPackage: "actioncatalog",
		ReadOnly:     true,
		Idempotent:   true,
		OpenWorld:    true,
	}

	actionSpec, err := spec.ActionSpec()
	if err != nil {
		t.Fatalf("ActionSpec() error = %v", err)
	}
	if actionSpec.Name != "surface" || actionSpec.IndividualTool.Name != "gitlab_test_surface" {
		t.Fatalf("ActionSpec() = %+v, want surface individual metadata", actionSpec)
	}
	if len(actionSpec.Aliases) != 1 || actionSpec.Aliases[0] != "surface_alias" {
		t.Fatalf("ActionSpec().Aliases = %v, want surface_alias", actionSpec.Aliases)
	}
	if len(actionSpec.Compatibility.ActionAliases) != 1 || actionSpec.Compatibility.ActionAliases[0].Alias != "gitlab_test_surface" {
		t.Fatalf("ActionSpec().Compatibility.ActionAliases = %+v, want historical alias", actionSpec.Compatibility.ActionAliases)
	}
	if actionSpec.Route.InputSchema == nil || actionSpec.Route.OutputSchema == nil {
		t.Fatalf("ActionSpec().Route schemas = input:%v output:%v, want both schemas", actionSpec.Route.InputSchema, actionSpec.Route.OutputSchema)
	}
}

func TestSurfaceToolSpec_Validate_RequiresSchemas(t *testing.T) {
	spec := SurfaceToolSpec{
		Name:          "gitlab_test_surface",
		Description:   "Test surface utility.",
		GroupToolName: "gitlab_test",
		BaseDomain:    "test",
		ActionName:    "surface",
		SurfaceKind:   SurfaceKindRuntimeUtility,
		Route:         toolutil.ActionRoute{Handler: func(context.Context, map[string]any) (any, error) { return surfaceSpecOutput{}, nil }},
		OwnerPackage:  "actioncatalog",
	}
	if err := spec.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing schema error")
	}
}
