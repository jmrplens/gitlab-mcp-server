package actioncatalog

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// surfaceSpecInput defines parameters for the surface spec operation.
type surfaceSpecInput struct {
	Value string `json:"value" jsonschema:"value to echo"`
}

// surfaceSpecOutput represents the response from the surface spec operation.
type surfaceSpecOutput struct {
	OK bool `json:"ok" jsonschema:"operation result"`
}

// surfaceSpecRoute is the typed route the surface spec tests hand their
// specs, carrying the input and output schemas Validate demands.
func surfaceSpecRoute() toolutil.ActionRoute {
	return toolutil.RouteFunc(func(context.Context, surfaceSpecInput) (surfaceSpecOutput, error) {
		return surfaceSpecOutput{OK: true}, nil
	})
}

// TestSurfaceToolSpec_ActionSpec_PreservesCatalogMetadata verifies that
// every field of a surface tool spec reaches the ActionSpec it projects to:
// the action name, the individual tool's name, title and description, the
// usage, the aliases, tags, related actions, compatibility policy and owner,
// with both route schemas in place.
//
// Each field holds a value no other field holds, so a projection reading a
// neighbor is seen. The usage is the case that needs it: it was the
// description projected a second time, which put a standalone tool's
// description on every surface as its Usage while cmd/audit_action_ids read
// the Usage line it was written as and never the description served.
func TestSurfaceToolSpec_ActionSpec_PreservesCatalogMetadata(t *testing.T) {
	spec := SurfaceToolSpec{
		Name:          "gitlab_test_surface",
		Title:         "Test Surface",
		Description:   "Test surface utility.",
		Usage:         "Use the test surface.",
		GroupToolName: "gitlab_test",
		BaseDomain:    "test",
		ActionName:    "surface",
		SurfaceKind:   SurfaceKindRuntimeUtility,
		Route:         surfaceSpecRoute(),
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
	}

	actionSpec, err := spec.ActionSpec()
	if err != nil {
		t.Fatalf("ActionSpec() error = %v", err)
	}
	if actionSpec.Name != "surface" {
		t.Errorf("ActionSpec().Name = %q, want surface", actionSpec.Name)
	}
	wantTool := toolutil.IndividualToolSpec{Name: "gitlab_test_surface", Title: "Test Surface", Description: "Test surface utility."}
	if actionSpec.IndividualTool != wantTool {
		t.Errorf("ActionSpec().IndividualTool = %+v, want %+v", actionSpec.IndividualTool, wantTool)
	}
	if actionSpec.Usage != "Use the test surface." {
		t.Errorf("ActionSpec().Usage = %q, want the spec's own usage", actionSpec.Usage)
	}
	if !slices.Equal(actionSpec.Aliases, []string{"surface_alias"}) {
		t.Errorf("ActionSpec().Aliases = %v, want [surface_alias]", actionSpec.Aliases)
	}
	if !slices.Equal(actionSpec.Tags, []string{"utility"}) {
		t.Errorf("ActionSpec().Tags = %v, want [utility]", actionSpec.Tags)
	}
	if !slices.Equal(actionSpec.RelatedActions, []string{"test.related"}) {
		t.Errorf("ActionSpec().RelatedActions = %v, want [test.related]", actionSpec.RelatedActions)
	}
	if !reflect.DeepEqual(actionSpec.Compatibility, spec.Compatibility) {
		t.Errorf("ActionSpec().Compatibility = %+v, want %+v", actionSpec.Compatibility, spec.Compatibility)
	}
	if actionSpec.OwnerPackage != "actioncatalog" {
		t.Errorf("ActionSpec().OwnerPackage = %q, want actioncatalog", actionSpec.OwnerPackage)
	}
	if !actionSpec.ReadOnly || actionSpec.Destructive || actionSpec.Idempotent || actionSpec.OpenWorld {
		t.Errorf("ActionSpec() flags read-only/destructive/idempotent/open-world = %t/%t/%t/%t, want true/false/false/false",
			actionSpec.ReadOnly, actionSpec.Destructive, actionSpec.Idempotent, actionSpec.OpenWorld)
	}
	if actionSpec.Route.InputSchema == nil || actionSpec.Route.OutputSchema == nil {
		t.Fatalf("ActionSpec().Route schemas = input:%v output:%v, want both schemas", actionSpec.Route.InputSchema, actionSpec.Route.OutputSchema)
	}
}

// TestSurfaceToolSpec_ActionSpec_ProjectsEachFlagOnItsOwn verifies that each
// boolean hint of a surface tool spec lands on the same hint of the
// ActionSpec, one flag set per case with the other three false.
func TestSurfaceToolSpec_ActionSpec_ProjectsEachFlagOnItsOwn(t *testing.T) {
	tests := []struct {
		name string
		edit func(*SurfaceToolSpec)
		want actionFlags
	}{
		{name: "read only", edit: func(spec *SurfaceToolSpec) { spec.ReadOnly = true }, want: actionFlags{ReadOnly: true}},
		{name: "destructive", edit: func(spec *SurfaceToolSpec) { spec.Destructive = true }, want: actionFlags{Destructive: true}},
		{name: "idempotent", edit: func(spec *SurfaceToolSpec) { spec.Idempotent = true }, want: actionFlags{Idempotent: true}},
		{name: "open world", edit: func(spec *SurfaceToolSpec) { spec.OpenWorld = true }, want: actionFlags{OpenWorld: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := SurfaceToolSpec{
				Name:          "gitlab_test_surface",
				Description:   "Test surface utility.",
				GroupToolName: "gitlab_test",
				BaseDomain:    "test",
				ActionName:    "surface",
				SurfaceKind:   SurfaceKindRuntimeUtility,
				Route:         surfaceSpecRoute(),
				OwnerPackage:  "actioncatalog",
			}
			tt.edit(&spec)
			actionSpec, err := spec.ActionSpec()
			if err != nil {
				t.Fatalf("ActionSpec() error = %v", err)
			}
			got := actionFlags{
				ReadOnly:    actionSpec.ReadOnly,
				Destructive: actionSpec.Destructive,
				Idempotent:  actionSpec.Idempotent,
				OpenWorld:   actionSpec.OpenWorld,
			}
			if got != tt.want {
				t.Errorf("ActionSpec() flags = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestCloneSurfaceToolSpec_TrimsEveryNameAndOwnsEverySlice verifies that the
// defensive copy trims each of the eleven string fields, drops blank and
// repeated entries from each string list, and copies the icons and the
// compatibility policy so an edit to the original reaches none of them.
//
// Eleven trims and six copies are seventeen plain statements no gate reports
// on, and a field the copy forgets is one Validate judges untrimmed.
func TestCloneSurfaceToolSpec_TrimsEveryNameAndOwnsEverySlice(t *testing.T) {
	icons := []mcp.Icon{{Source: "data:image/svg+xml;base64,test", MIMEType: "image/svg+xml", Sizes: []string{"any"}}}
	compatibility := toolutil.CompatibilityPolicy{ActionAliases: []toolutil.ActionAliasSpec{{Alias: "old_name", Target: "surface"}}}
	original := SurfaceToolSpec{
		Name:                   " name ",
		Title:                  " title ",
		Description:            " description ",
		Usage:                  " usage ",
		GroupDescription:       " group description ",
		GroupToolName:          " group tool ",
		BaseDomain:             " domain ",
		ActionName:             " action ",
		OwnerPackage:           " owner ",
		SafeModePolicy:         " safe policy ",
		ReadOnlyPolicy:         " read-only policy ",
		Aliases:                []string{" alias ", "", "alias"},
		Tags:                   []string{" tag ", "tag", " "},
		RelatedActions:         []string{" test.related ", "test.related"},
		CapabilityRequirements: []string{" capability ", "capability", ""},
		Icons:                  icons,
		Compatibility:          compatibility,
	}

	cloned := CloneSurfaceToolSpec(original)

	icons[0].Source = "changed"
	compatibility.ActionAliases[0].Alias = "changed"
	original.Aliases[0] = "changed"

	want := SurfaceToolSpec{
		Name:                   "name",
		Title:                  "title",
		Description:            "description",
		Usage:                  "usage",
		GroupDescription:       "group description",
		GroupToolName:          "group tool",
		BaseDomain:             "domain",
		ActionName:             "action",
		OwnerPackage:           "owner",
		SafeModePolicy:         "safe policy",
		ReadOnlyPolicy:         "read-only policy",
		Aliases:                []string{"alias"},
		Tags:                   []string{"tag"},
		RelatedActions:         []string{"test.related"},
		CapabilityRequirements: []string{"capability"},
		Icons:                  []mcp.Icon{{Source: "data:image/svg+xml;base64,test", MIMEType: "image/svg+xml", Sizes: []string{"any"}}},
		Compatibility:          toolutil.CompatibilityPolicy{ActionAliases: []toolutil.ActionAliasSpec{{Alias: "old_name", Target: "surface"}}},
	}
	if !reflect.DeepEqual(cloned, want) {
		t.Fatalf("CloneSurfaceToolSpec() = %+v, want %+v", cloned, want)
	}
}

// TestSurfaceToolSpec_Validate_RequiresSchemas verifies that a route with a
// handler and no schemas is refused, since the catalog projects a schema
// from every route it serves.
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

// TestSurfaceToolSpec_ValidateRejectsMissingMetadata verifies that the
// SurfaceToolSpec.Validate method rejects specs that omit any required field
// (description, route, schema, name, owner package, etc.).
//
// The test starts from a fully valid spec, strips one field at a time, and
// asserts each variant produces a validation error. This protects the
// catalog from registering incomplete surfaces that would later fail at
// runtime.
func TestSurfaceToolSpec_ValidateRejectsMissingMetadata(t *testing.T) {
	valid := SurfaceToolSpec{
		Name:          "gitlab_test_surface",
		Description:   "Test surface utility.",
		GroupToolName: "gitlab_test",
		BaseDomain:    "test",
		ActionName:    "surface",
		SurfaceKind:   SurfaceKindRuntimeUtility,
		Route:         surfaceSpecRoute(),
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

// TestSurfaceToolSpec_Validate_AcceptsEverySurfaceKind verifies that each of
// the five surface kinds passes surface tool validation, the two that no
// production surface spec declares included, since the validator is one
// list for both spec types and a kind missing from it refuses every group of
// that kind.
func TestSurfaceToolSpec_Validate_AcceptsEverySurfaceKind(t *testing.T) {
	kinds := []SurfaceKind{
		SurfaceKindGitLabAction,
		SurfaceKindMetaGroup,
		SurfaceKindDynamicController,
		SurfaceKindRuntimeUtility,
		SurfaceKindInteractiveUtility,
	}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			spec := SurfaceToolSpec{
				Name:          "gitlab_test_surface",
				Description:   "Test surface utility.",
				GroupToolName: "gitlab_test",
				BaseDomain:    "test",
				ActionName:    "surface",
				SurfaceKind:   kind,
				Route:         surfaceSpecRoute(),
				OwnerPackage:  "actioncatalog",
			}
			if err := spec.Validate(); err != nil {
				t.Fatalf("Validate() error = %v, want kind %q accepted", err, kind)
			}
		})
	}
}

// TestSurfaceToolSpec_ActionSpecRejectsInvalidSpec verifies that ActionSpec
// refuses a spec Validate refuses instead of projecting an incomplete one.
func TestSurfaceToolSpec_ActionSpecRejectsInvalidSpec(t *testing.T) {
	_, err := (SurfaceToolSpec{}).ActionSpec()
	if err == nil {
		t.Fatal("ActionSpec() error = nil, want validation error")
	}
}
