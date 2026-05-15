package surfaces

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
)

func TestStandaloneToolSpecs_ClassifyStandaloneUtilities(t *testing.T) {
	specs := StandaloneToolSpecs(nil)
	discover := findSurfaceSpec(t, specs, "gitlab_discover_project")
	if discover.SurfaceKind != actioncatalog.SurfaceKindRuntimeUtility || !discover.ReadOnly || discover.GroupToolName != "gitlab_discover_project" {
		t.Fatalf("discover spec = %+v, want read-only runtime utility in discover group", discover)
	}
	if !hasActionAlias(discover, "gitlab_discover_project") {
		t.Fatalf("discover compatibility aliases = %+v, want historical tool-name alias", discover.Compatibility.ActionAliases)
	}

	interactive := findSurfaceSpec(t, specs, "gitlab_interactive_issue_create")
	if interactive.SurfaceKind != actioncatalog.SurfaceKindInteractiveUtility || interactive.ReadOnly {
		t.Fatalf("interactive spec = %+v, want non-read-only interactive utility", interactive)
	}
	if len(interactive.CapabilityRequirements) != 1 || interactive.CapabilityRequirements[0] != "elicitation" {
		t.Fatalf("interactive capability requirements = %v, want elicitation", interactive.CapabilityRequirements)
	}
	if !hasActionAlias(interactive, "gitlab_interactive_issue.create") {
		t.Fatalf("interactive compatibility aliases = %+v, want provider-specific issue alias", interactive.Compatibility.ActionAliases)
	}
}

func TestToolGroupSpecs_ProjectsSurfaceMetadata(t *testing.T) {
	groups := ToolGroupSpecs(StandaloneToolSpecs(nil))
	if len(groups) != 2 {
		t.Fatalf("ToolGroupSpecs() len = %d, want 2", len(groups))
	}
	var foundInteractive bool
	for _, group := range groups {
		if group.ToolName != "gitlab_interactive" {
			continue
		}
		foundInteractive = true
		if group.SurfaceKind != actioncatalog.SurfaceKindInteractiveUtility || len(group.Actions) != 4 {
			t.Fatalf("interactive group = %+v, want four interactive utility actions", group)
		}
	}
	if !foundInteractive {
		t.Fatalf("ToolGroupSpecs() = %+v, want gitlab_interactive group", groups)
	}
}

func findSurfaceSpec(t *testing.T, specs []actioncatalog.SurfaceToolSpec, name string) actioncatalog.SurfaceToolSpec {
	t.Helper()
	for _, spec := range specs {
		if spec.Name == name {
			return spec
		}
	}
	t.Fatalf("surface spec %q not found in %+v", name, specs)
	return actioncatalog.SurfaceToolSpec{}
}

func hasActionAlias(spec actioncatalog.SurfaceToolSpec, alias string) bool {
	for _, actionAlias := range spec.Compatibility.ActionAliases {
		if actionAlias.Alias == alias {
			return true
		}
	}
	return false
}
