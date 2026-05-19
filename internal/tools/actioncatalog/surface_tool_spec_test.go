package actioncatalog

import (
	"context"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// surfaceSpecInput defines parameters for the surface spec operation.
type surfaceSpecInput struct {
	Value string `json:"value" jsonschema:"value to echo"`
}

// surfaceSpecOutput represents the response from the surface spec operation.
type surfaceSpecOutput struct {
	OK bool `json:"ok" jsonschema:"operation result"`
}

// TestSurfaceToolSpec_ActionSpec_PreservesCatalogMetadata verifies SurfaceToolSpec preserves catalog metadata with action spec.
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

// TestSurfaceToolSpec_Validate_RequiresSchemas verifies SurfaceToolSpec requires schemas with validate.
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

// TestSurfaceToolSpec_ValidateRejectsMissingMetadata covers each required
// metadata field so catalog projection failures remain specific.
func TestSurfaceToolSpec_ValidateRejectsMissingMetadata(t *testing.T) {
	valid := SurfaceToolSpec{
		Name:          "gitlab_test_surface",
		Description:   "Test surface utility.",
		GroupToolName: "gitlab_test",
		BaseDomain:    "test",
		ActionName:    "surface",
		SurfaceKind:   SurfaceKindRuntimeUtility,
		Route:         toolutil.RouteFunc(func(context.Context, surfaceSpecInput) (surfaceSpecOutput, error) { return surfaceSpecOutput{}, nil }),
		OwnerPackage:  "actioncatalog",
	}

	tests := []struct {
		name string
		edit func(SurfaceToolSpec) SurfaceToolSpec
		want string
	}{
		{name: "missing name", edit: func(spec SurfaceToolSpec) SurfaceToolSpec { spec.Name = ""; return spec }, want: "name is required"},
		{name: "missing description", edit: func(spec SurfaceToolSpec) SurfaceToolSpec { spec.Description = ""; return spec }, want: "description is required"},
		{name: "missing group", edit: func(spec SurfaceToolSpec) SurfaceToolSpec { spec.GroupToolName = ""; return spec }, want: "group tool name is required"},
		{name: "missing domain", edit: func(spec SurfaceToolSpec) SurfaceToolSpec { spec.BaseDomain = ""; return spec }, want: "base domain is required"},
		{name: "missing action", edit: func(spec SurfaceToolSpec) SurfaceToolSpec { spec.ActionName = ""; return spec }, want: "action name is required"},
		{name: "missing owner", edit: func(spec SurfaceToolSpec) SurfaceToolSpec { spec.OwnerPackage = ""; return spec }, want: "owner package is required"},
		{name: "invalid kind", edit: func(spec SurfaceToolSpec) SurfaceToolSpec { spec.SurfaceKind = SurfaceKind("legacy"); return spec }, want: "unsupported surface kind"},
		{name: "missing handler", edit: func(spec SurfaceToolSpec) SurfaceToolSpec { spec.Route.Handler = nil; return spec }, want: "route handler is required"},
		{name: "missing input schema", edit: func(spec SurfaceToolSpec) SurfaceToolSpec { spec.Route.InputSchema = nil; return spec }, want: "input schema is required"},
		{name: "missing output schema", edit: func(spec SurfaceToolSpec) SurfaceToolSpec { spec.Route.OutputSchema = nil; return spec }, want: "output schema is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.edit(valid).Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

// TestSurfaceToolSpec_ActionSpecRejectsInvalidSpec verifies ActionSpec returns
// validation errors instead of projecting incomplete metadata.
func TestSurfaceToolSpec_ActionSpecRejectsInvalidSpec(t *testing.T) {
	_, err := (SurfaceToolSpec{}).ActionSpec()
	if err == nil {
		t.Fatal("ActionSpec() error = nil, want validation error")
	}
}
