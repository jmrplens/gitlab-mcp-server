package actioncatalog

import (
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestCatalogGroupSpec_ValidateAndClone verifies that CloneCatalogGroupSpec
// returns a copy the caller's later edits do not reach, with its names
// trimmed and its capability list deduplicated, and that the copy validates
// and projects every one of its fields into GroupOptions.
func TestCatalogGroupSpec_ValidateAndClone(t *testing.T) {
	icons := []mcp.Icon{{Source: "data:image/svg+xml;base64,test", MIMEType: "image/svg+xml", Sizes: []string{"any"}}}
	capabilities := []string{"cap-x", "", "cap-x"}
	actions := []toolutil.ActionSpec{toolutil.NewActionSpec("delete", testRoute(true), toolutil.ActionSpecOptions{
		Destructive:  true,
		Idempotent:   true,
		OwnerPackage: "projects",
		Compatibility: toolutil.CompatibilityPolicy{
			ActionAliases: []toolutil.ActionAliasSpec{{Alias: "remove", Target: "delete", Source: "dynamic", Reason: "Historical Dynamic alias."}},
		},
	})}
	formatter := func(any) *mcp.CallToolResult {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "group formatter"}}}
	}
	group := CatalogGroupSpec{
		ToolName:               " gitlab_project ",
		Title:                  " Project ",
		Description:            " Project tools. ",
		ReadOnly:               false,
		Icons:                  icons,
		BaseDomain:             " project ",
		EnterpriseOnly:         true,
		CapabilityRequirements: capabilities,
		FormatResult:           formatter,
		Actions:                actions,
		OwnerPackage:           " projects ",
		SurfaceKind:            SurfaceKindGitLabAction,
	}

	cloned := CloneCatalogGroupSpec(group)
	icons[0].Source = "changed"
	capabilities[0] = "changed"
	actions[0] = toolutil.NewActionSpec("changed", testRoute(false), toolutil.ActionSpecOptions{OwnerPackage: "projects"})

	if err := cloned.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if cloned.ToolName != "gitlab_project" || cloned.Title != "Project" || cloned.Description != "Project tools." || cloned.BaseDomain != "project" || cloned.OwnerPackage != "projects" {
		t.Fatalf("cloned group metadata = %+v, want every name trimmed", cloned)
	}
	if cloned.SurfaceKind != SurfaceKindGitLabAction {
		t.Fatalf("cloned surface kind = %q, want %q kept rather than defaulted", cloned.SurfaceKind, SurfaceKindGitLabAction)
	}
	if cloned.Icons[0].Source != "data:image/svg+xml;base64,test" {
		t.Fatalf("cloned icon source = %q, want original", cloned.Icons[0].Source)
	}
	if len(cloned.CapabilityRequirements) != 1 || cloned.CapabilityRequirements[0] != "cap-x" {
		t.Fatalf("capability requirements = %+v, want deduped cap-x", cloned.CapabilityRequirements)
	}
	if cloned.Actions[0].Name != "delete" {
		t.Fatalf("cloned action = %+v, want original delete action", cloned.Actions[0])
	}

	opts := cloned.GroupOptions()
	if opts.FormatResult == nil {
		t.Fatal("GroupOptions().FormatResult = nil, want the spec's formatter")
	}
	if result := opts.FormatResult(nil); len(result.Content) != 1 || result.Content[0].(*mcp.TextContent).Text != "group formatter" {
		t.Errorf("GroupOptions().FormatResult() = %+v, want the spec formatter's text", result)
	}
	// A func is never DeepEqual to another, so the formatter is compared
	// above and left out of the whole-value comparison here.
	opts.FormatResult = nil
	want := GroupOptions{
		ToolName:               "gitlab_project",
		Title:                  "Project",
		Description:            "Project tools.",
		Icons:                  []mcp.Icon{{Source: "data:image/svg+xml;base64,test", MIMEType: "image/svg+xml", Sizes: []string{"any"}}},
		BaseDomain:             "project",
		EnterpriseOnly:         true,
		CapabilityRequirements: []string{"cap-x"},
		OwnerPackage:           "projects",
		SurfaceKind:            SurfaceKindGitLabAction,
	}
	if !reflect.DeepEqual(opts, want) {
		t.Fatalf("GroupOptions() = %+v, want %+v", opts, want)
	}
}

// TestCatalogGroupSpec_GroupOptions_ProjectsEachFlagOnItsOwn verifies that
// each of the three boolean fields of a group spec reaches the same field of
// its GroupOptions, one flag set per case with the other two false, since a
// fixture setting them all cannot tell a swap of two apart.
func TestCatalogGroupSpec_GroupOptions_ProjectsEachFlagOnItsOwn(t *testing.T) {
	tests := []struct {
		name string
		spec CatalogGroupSpec
		want GroupOptions
	}{
		{name: "read only", spec: CatalogGroupSpec{ToolName: "gitlab_project", ReadOnly: true}, want: GroupOptions{ToolName: "gitlab_project", ReadOnly: true, SurfaceKind: SurfaceKindMetaGroup}},
		{name: "enterprise only", spec: CatalogGroupSpec{ToolName: "gitlab_project", EnterpriseOnly: true}, want: GroupOptions{ToolName: "gitlab_project", EnterpriseOnly: true, SurfaceKind: SurfaceKindMetaGroup}},
		{name: "gitlab.com only", spec: CatalogGroupSpec{ToolName: "gitlab_project", GitLabDotComOnly: true}, want: GroupOptions{ToolName: "gitlab_project", GitLabDotComOnly: true, SurfaceKind: SurfaceKindMetaGroup}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.GroupOptions(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GroupOptions() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestCatalogGroupSpec_CloneDefaultsSurfaceKind verifies that a group spec
// naming no surface kind is cloned as a meta group, the kind every ordinary
// domain group has.
func TestCatalogGroupSpec_CloneDefaultsSurfaceKind(t *testing.T) {
	cloned := CloneCatalogGroupSpec(CatalogGroupSpec{ToolName: "gitlab_project"})
	if cloned.SurfaceKind != SurfaceKindMetaGroup {
		t.Fatalf("SurfaceKind = %q, want %q", cloned.SurfaceKind, SurfaceKindMetaGroup)
	}
}

// TestCatalogGroupSpec_Validate_AcceptsEverySurfaceKind verifies that each
// of the five surface kinds passes group validation and that a kind outside
// them is refused: a new kind added to the constants and not to the
// validator would refuse every group declaring it.
func TestCatalogGroupSpec_Validate_AcceptsEverySurfaceKind(t *testing.T) {
	tests := []struct {
		name string
		kind SurfaceKind
		ok   bool
	}{
		{name: "gitlab action", kind: SurfaceKindGitLabAction, ok: true},
		{name: "meta group", kind: SurfaceKindMetaGroup, ok: true},
		{name: "dynamic controller", kind: SurfaceKindDynamicController, ok: true},
		{name: "runtime utility", kind: SurfaceKindRuntimeUtility, ok: true},
		{name: "interactive utility", kind: SurfaceKindInteractiveUtility, ok: true},
		{name: "unknown kind", kind: SurfaceKind("legacy"), ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := CatalogGroupSpec{
				ToolName:     "gitlab_project",
				OwnerPackage: "projects",
				SurfaceKind:  tt.kind,
				Actions:      []toolutil.ActionSpec{toolutil.NewActionSpec("get", testRoute(false), toolutil.ActionSpecOptions{ReadOnly: true, OwnerPackage: "projects"})},
			}
			err := spec.Validate()
			if tt.ok && err != nil {
				t.Fatalf("Validate() error = %v, want kind %q accepted", err, tt.kind)
			}
			if !tt.ok && (err == nil || !strings.Contains(err.Error(), "unsupported surface kind")) {
				t.Fatalf("Validate() error = %v, want kind %q refused as unsupported", err, tt.kind)
			}
		})
	}
}

// TestCatalogGroupSpec_RejectsInvalidMetadata verifies each refusal of
// CatalogGroupSpec.Validate by the text it reports: a blank tool name, an
// action spec that fails its own validation, a missing owner, an unknown
// surface kind, no actions, a repeated action name, an action with no owner
// or no input schema, and an action alias two actions claim.
func TestCatalogGroupSpec_RejectsInvalidMetadata(t *testing.T) {
	validAction := toolutil.NewActionSpec("get", testRoute(false), toolutil.ActionSpecOptions{ReadOnly: true, OwnerPackage: "projects"})
	testCases := []struct {
		name string
		edit func(CatalogGroupSpec) CatalogGroupSpec
		want string
	}{
		{
			name: "missing tool name",
			edit: func(spec CatalogGroupSpec) CatalogGroupSpec {
				spec.ToolName = " "
				return spec
			},
			want: errToolNameRequired,
		},
		{
			name: "invalid action spec",
			edit: func(spec CatalogGroupSpec) CatalogGroupSpec {
				spec.Actions = []toolutil.ActionSpec{toolutil.NewActionSpec("get", testRoute(false), toolutil.ActionSpecOptions{OwnerPackage: "projects", ContentKind: "legacy"})}
				return spec
			},
			want: "unsupported content kind",
		},
		{
			name: "missing owner",
			edit: func(spec CatalogGroupSpec) CatalogGroupSpec {
				spec.OwnerPackage = ""
				return spec
			},
			want: "owner package is required",
		},
		{
			name: "invalid surface kind",
			edit: func(spec CatalogGroupSpec) CatalogGroupSpec {
				spec.SurfaceKind = SurfaceKind("legacy")
				return spec
			},
			want: "unsupported surface kind",
		},
		{
			name: "no actions",
			edit: func(spec CatalogGroupSpec) CatalogGroupSpec {
				spec.Actions = nil
				return spec
			},
			want: "has no actions",
		},
		{
			name: "duplicate action",
			edit: func(spec CatalogGroupSpec) CatalogGroupSpec {
				spec.Actions = append(spec.Actions, validAction)
				return spec
			},
			want: "duplicate action \"get\"",
		},
		{
			name: "missing action owner",
			edit: func(spec CatalogGroupSpec) CatalogGroupSpec {
				spec.Actions = []toolutil.ActionSpec{toolutil.NewActionSpec("get", testRoute(false), toolutil.ActionSpecOptions{})}
				return spec
			},
			want: "action \"get\" owner package is required",
		},
		{
			name: "missing action schema",
			edit: func(spec CatalogGroupSpec) CatalogGroupSpec {
				spec.Actions = []toolutil.ActionSpec{toolutil.NewActionSpec("get", toolutil.ActionRoute{}, toolutil.ActionSpecOptions{OwnerPackage: "projects"})}
				return spec
			},
			want: "action \"get\" has nil input schema",
		},
		{
			name: "conflicting action alias",
			edit: func(spec CatalogGroupSpec) CatalogGroupSpec {
				spec.Actions = []toolutil.ActionSpec{
					toolutil.NewActionSpec("get", testRoute(false), toolutil.ActionSpecOptions{OwnerPackage: "projects", Compatibility: toolutil.CompatibilityPolicy{ActionAliases: []toolutil.ActionAliasSpec{{Alias: "show", Target: "get", Source: "dynamic", Reason: "First owner."}}}}),
					toolutil.NewActionSpec("list", testRoute(false), toolutil.ActionSpecOptions{OwnerPackage: "projects", Compatibility: toolutil.CompatibilityPolicy{ActionAliases: []toolutil.ActionAliasSpec{{Alias: "show", Target: "list", Source: "dynamic", Reason: "Second owner."}}}}),
				}
				return spec
			},
			want: "compatibility action alias \"show\" maps to both",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			group := CatalogGroupSpec{
				ToolName:     "gitlab_project",
				Actions:      []toolutil.ActionSpec{validAction},
				OwnerPackage: "projects",
				SurfaceKind:  SurfaceKindGitLabAction,
			}
			err := tc.edit(group).Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestValidateCatalogGroupAliases_ConflictNamesTheActionIDs verifies that an
// action alias two actions claim is reported with the canonical ID of each,
// and that the ID is the group's base domain joined to the action name, or
// the domain derived from the tool name when the spec names no base domain.
//
// The IDs in the message are what a reader searches the catalog for, so a
// domain read from the wrong field is a message naming actions that do not
// exist. Nothing else in the group validator publishes the ID it computes.
func TestValidateCatalogGroupAliases_ConflictNamesTheActionIDs(t *testing.T) {
	tests := []struct {
		name       string
		toolName   string
		baseDomain string
		wantIDs    string
	}{
		{name: "base domain names the IDs", toolName: "gitlab_project_alias", baseDomain: "alias", wantIDs: `"alias.get" and "alias.list"`},
		{name: "no base domain derives them from the tool name", toolName: "gitlab_project", baseDomain: "", wantIDs: `"project.get" and "project.list"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := CatalogGroupSpec{
				ToolName:   tt.toolName,
				BaseDomain: tt.baseDomain,
				Actions: []toolutil.ActionSpec{
					{Name: "get", Compatibility: toolutil.CompatibilityPolicy{ActionAliases: []toolutil.ActionAliasSpec{{Alias: "show"}}}},
					{Name: "list", Compatibility: toolutil.CompatibilityPolicy{ActionAliases: []toolutil.ActionAliasSpec{{Alias: "Show"}}}},
				},
			}
			err := validateCatalogGroupAliases(spec)
			want := "catalog group " + `"` + tt.toolName + `"` + ` compatibility action alias "show" maps to both ` + tt.wantIDs
			if err == nil || err.Error() != want {
				t.Fatalf("validateCatalogGroupAliases() error = %v, want %q", err, want)
			}
		})
	}
}

// TestValidateCatalogGroupAliases_RepeatsOnOneActionAreNotConflicts verifies
// what the alias validator accepts: an action alias listed twice on one
// action, a parameter alias listed twice with one target, one parameter
// alias on its own, and blank aliases of either kind, which are skipped.
//
// The conflict check is "seen before, and by someone else", and a validator
// that read either half alone would refuse every action listing an alias
// twice, or every second action of a group.
func TestValidateCatalogGroupAliases_RepeatsOnOneActionAreNotConflicts(t *testing.T) {
	tests := []struct {
		name          string
		compatibility toolutil.CompatibilityPolicy
	}{
		{name: "action alias twice on one action", compatibility: toolutil.CompatibilityPolicy{
			ActionAliases: []toolutil.ActionAliasSpec{{Alias: "show"}, {Alias: " Show "}},
		}},
		{name: "one parameter alias", compatibility: toolutil.CompatibilityPolicy{
			ParameterAliases: []toolutil.ParameterAliasSpec{{Alias: "project", Target: "project_id"}},
		}},
		{name: "parameter alias twice with one target", compatibility: toolutil.CompatibilityPolicy{
			ParameterAliases: []toolutil.ParameterAliasSpec{{Alias: "project", Target: "project_id"}, {Alias: "Project", Target: "project_id"}},
		}},
		{name: "blank aliases are skipped", compatibility: toolutil.CompatibilityPolicy{
			ActionAliases:    []toolutil.ActionAliasSpec{{Alias: " "}},
			ParameterAliases: []toolutil.ParameterAliasSpec{{Alias: ""}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := CatalogGroupSpec{
				ToolName: "gitlab_project",
				Actions:  []toolutil.ActionSpec{{Name: "get", Compatibility: tt.compatibility}},
			}
			if err := validateCatalogGroupAliases(spec); err != nil {
				t.Fatalf("validateCatalogGroupAliases() error = %v, want accepted", err)
			}
		})
	}
}

// TestValidateCatalogGroupAliases_ParameterAliasConflictNamesBothTargets
// verifies that one parameter alias mapped to two targets on one action is
// refused with a message naming the action and both targets, in the order
// they were declared.
func TestValidateCatalogGroupAliases_ParameterAliasConflictNamesBothTargets(t *testing.T) {
	spec := CatalogGroupSpec{
		ToolName: "gitlab_project",
		Actions: []toolutil.ActionSpec{
			{Name: "get", Compatibility: toolutil.CompatibilityPolicy{
				ParameterAliases: []toolutil.ParameterAliasSpec{{Alias: "project", Target: "project_id"}, {Alias: "project", Target: "id"}},
			}},
		},
	}
	const want = `catalog group "gitlab_project" action "get" compatibility parameter alias "project" maps to both "project_id" and "id"`
	if err := validateCatalogGroupAliases(spec); err == nil || err.Error() != want {
		t.Fatalf("validateCatalogGroupAliases() error = %v, want %q", err, want)
	}
}

// TestGroupOptions_BaseDomainControlsActionID verifies that a group's base
// domain, when set, is the domain of its action IDs, and the domain derived
// from the tool name is not.
func TestGroupOptions_BaseDomainControlsActionID(t *testing.T) {
	group := NewGroup(GroupOptions{ToolName: "gitlab_project_alias", BaseDomain: "alias"})
	group.SetAction(Action{Name: "get", Route: testRoute(false)})
	catalog := NewCatalog()
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	if _, ok := catalog.Action("alias.get"); !ok {
		t.Fatal("catalog missing alias.get action")
	}
	if _, ok := catalog.Action("project_alias.get"); ok {
		t.Fatal("catalog unexpectedly kept derived project_alias.get action")
	}
}

// TestCatalogGroupSpec_Validate_RefusalsComeFromTheFirstLayer pins the two
// properties that let CatalogGroupSpec.Validate carry no empty-name check
// and no duplicate-ID check of its own:
//
//   - toolutil.ActionSpec.Validate, called first for every action, refuses
//     a name whose trimmed form is empty, so the trimmed name is never
//     empty afterwards;
//   - an action ID in a group spec is the group's one domain joined to the
//     action name, and the name is held unique, so two actions cannot share
//     an ID without first being refused as a duplicate name.
//
// Both checks used to be kept as belt and braces. Neither could be reached,
// so each was code a reader had to reason about for nothing, and they are
// gone; this test is what holds the reason they could go.
func TestCatalogGroupSpec_Validate_RefusalsComeFromTheFirstLayer(t *testing.T) {
	makeSpec := func(name string) toolutil.ActionSpec {
		return toolutil.NewActionSpec(name, testRoute(false), toolutil.ActionSpecOptions{
			ReadOnly:     true,
			Idempotent:   true,
			OwnerPackage: "projects",
		})
	}

	t.Run("blank action name refused by the action's own validation", func(t *testing.T) {
		group := CatalogGroupSpec{
			ToolName:     "gitlab_dup",
			OwnerPackage: "projects",
			Actions:      []toolutil.ActionSpec{makeSpec("   ")},
		}
		err := group.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want rejection for whitespace-only action name")
		}
		if !strings.Contains(err.Error(), "action spec name is required") {
			t.Fatalf("err = %q, want it to come from toolutil.ActionSpec.Validate", err.Error())
		}
	})

	t.Run("duplicate action name refused before its ID could collide", func(t *testing.T) {
		group := CatalogGroupSpec{
			ToolName:     "gitlab_dup",
			OwnerPackage: "projects",
			Actions: []toolutil.ActionSpec{
				makeSpec("get"),
				makeSpec("get"),
			},
		}
		err := group.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want rejection for duplicate action name")
		}
		if !strings.Contains(err.Error(), `duplicate action "get"`) {
			t.Fatalf("err = %q, want the duplicate name refused, not a duplicate ID", err.Error())
		}
	})
}
