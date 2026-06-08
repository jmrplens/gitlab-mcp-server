package actioncatalog

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestCatalogGroupSpec_ValidateAndClone verifies CatalogGroupSpec when validate and clone.
func TestCatalogGroupSpec_ValidateAndClone(t *testing.T) {
	icons := []mcp.Icon{{Source: "data:image/svg+xml;base64,test", MIMEType: "image/svg+xml", Sizes: []string{"any"}}}
	capabilities := []string{"sampling", "", "sampling"}
	actions := []toolutil.ActionSpec{toolutil.NewActionSpec("delete", testRoute(true), toolutil.ActionSpecOptions{
		Destructive:  true,
		Idempotent:   true,
		OwnerPackage: "projects",
		Compatibility: toolutil.CompatibilityPolicy{
			ActionAliases: []toolutil.ActionAliasSpec{{Alias: "remove", Target: "delete", Source: "dynamic", Reason: "Historical Dynamic alias."}},
		},
	})}
	group := CatalogGroupSpec{
		ToolName:               " gitlab_project ",
		Title:                  " Project ",
		Description:            "Project tools.",
		ReadOnly:               false,
		Icons:                  icons,
		BaseDomain:             "project",
		CapabilityRequirements: capabilities,
		Actions:                actions,
		OwnerPackage:           "projects",
		SurfaceKind:            SurfaceKindGitLabAction,
	}

	cloned := CloneCatalogGroupSpec(group)
	icons[0].Source = "changed"
	capabilities[0] = "changed"
	actions[0] = toolutil.NewActionSpec("changed", testRoute(false), toolutil.ActionSpecOptions{OwnerPackage: "projects"})

	if err := cloned.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if cloned.ToolName != "gitlab_project" || cloned.Title != "Project" || cloned.SurfaceKind != SurfaceKindGitLabAction {
		t.Fatalf("cloned group metadata = %+v, want trimmed canonical metadata", cloned)
	}
	if cloned.Icons[0].Source != "data:image/svg+xml;base64,test" {
		t.Fatalf("cloned icon source = %q, want original", cloned.Icons[0].Source)
	}
	if len(cloned.CapabilityRequirements) != 1 || cloned.CapabilityRequirements[0] != "sampling" {
		t.Fatalf("capability requirements = %+v, want deduped sampling", cloned.CapabilityRequirements)
	}
	if cloned.Actions[0].Name != "delete" {
		t.Fatalf("cloned action = %+v, want original delete action", cloned.Actions[0])
	}

	opts := cloned.GroupOptions()
	if opts.ToolName != "gitlab_project" || opts.BaseDomain != "project" || opts.OwnerPackage != "projects" || opts.SurfaceKind != SurfaceKindGitLabAction {
		t.Fatalf("GroupOptions() = %+v, want projected group metadata", opts)
	}
}

// TestCatalogGroupSpec_CloneDefaultsSurfaceKind verifies group specs default to
// meta-group surface kind when no explicit kind is supplied.
func TestCatalogGroupSpec_CloneDefaultsSurfaceKind(t *testing.T) {
	cloned := CloneCatalogGroupSpec(CatalogGroupSpec{ToolName: "gitlab_project"})
	if cloned.SurfaceKind != SurfaceKindMetaGroup {
		t.Fatalf("SurfaceKind = %q, want %q", cloned.SurfaceKind, SurfaceKindMetaGroup)
	}
}

// TestCatalogGroupSpec_RejectsInvalidMetadata covers CatalogGroupSpec with table-driven subtests for rejects invalid metadata.
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

// TestValidateCatalogGroupAliases_EdgeCases covers alias normalization and
// cross-action parameter alias conflicts after ActionSpec-level validation.
func TestValidateCatalogGroupAliases_EdgeCases(t *testing.T) {
	spec := CatalogGroupSpec{
		ToolName: "gitlab_project",
		Actions: []toolutil.ActionSpec{
			{Name: "get", Compatibility: toolutil.CompatibilityPolicy{
				ActionAliases:    []toolutil.ActionAliasSpec{{Alias: " "}, {Alias: "show"}},
				ParameterAliases: []toolutil.ParameterAliasSpec{{Alias: " "}, {Alias: "project", Target: "project_id"}, {Alias: "project", Target: "id"}},
			}},
		},
	}
	if err := validateCatalogGroupAliases(spec); err == nil || !strings.Contains(err.Error(), "compatibility parameter alias \"project\" maps to both") {
		t.Fatalf("validateCatalogGroupAliases() error = %v, want parameter conflict", err)
	}
}

// TestGroupOptions_BaseDomainControlsActionID verifies GroupOptions when base domain controls action ID.
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

// ----- branch coverage -----

// TestCatalogGroupSpec_Validate_DeadBranches documents why the
// "action name is required" and "duplicate action id" branches inside
// CatalogGroupSpec.Validate are unreachable through the public API:
//
//   - The empty-name branch is shadowed by toolutil.ActionSpec.Validate(),
//     which is invoked at the top of every loop iteration and returns
//     "action spec name is required" for any Name whose trimmed form is
//     empty. By the time control reaches the post-trim empty check, the
//     name is already guaranteed non-empty.
//
//   - The duplicate action ID branch cannot fire because the loop also
//     guards against duplicate names (the seenActionNames check) and
//     catalogGroupActionID derives the action ID from `<domain>.<name>`.
//     Two actions with the same name therefore collide on the name
//     check first, and the ID check is never reached.
//
// The test asserts the documented contract: an empty action name and a
// duplicate action name are both rejected, but the rejection originates
// from the upstream layer (toolutil.ActionSpec.Validate and the
// seenActionNames guard) rather than from the dead branches.
func TestCatalogGroupSpec_Validate_DeadBranches(t *testing.T) {
	makeSpec := func(name string) toolutil.ActionSpec {
		return toolutil.NewActionSpec(name, testRoute(false), toolutil.ActionSpecOptions{
			ReadOnly:     true,
			Idempotent:   true,
			OwnerPackage: "projects",
		})
	}

	t.Run("empty action name rejected by upstream validation", func(t *testing.T) {
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

	t.Run("duplicate action name rejected by seenActionNames guard", func(t *testing.T) {
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
		if !strings.Contains(err.Error(), "duplicate action") {
			t.Fatalf("err = %q, want it to mention 'duplicate action'", err.Error())
		}
	})
}
