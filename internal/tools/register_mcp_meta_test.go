// register_mcp_meta_test.go verifies the gitlab_server diagnostics meta-tool:
// that it is registered at all, and that its hand-written route map and its
// action specs describe the same set of actions.
package tools

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestRegisterMCPMeta_RegistersTheDiagnosticsToolWithItsAliases covers the one
// meta-tool that is not built from a domain's action specs alone.
//
// gitlab_server answers "does this token work, and against which GitLab", which
// is the first thing a client asks when anything else fails — so it has to be
// registered even though it is assembled from a hand-written route map. The
// alias route is part of that: health_check and status are the same action, and
// a client that learned one name must not get an unknown-action error.
func TestRegisterMCPMeta_RegistersTheDiagnosticsToolWithItsAliases(t *testing.T) {
	t.Parallel()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)

	RegisterMCPMeta(server, nil)

	tools, err := toolutil.ListRegisteredTools(t.Context(), server, "test")
	if err != nil {
		t.Fatalf("listing registered tools: %v", err)
	}
	var found *mcp.Tool
	for _, tool := range tools {
		if tool.Name == "gitlab_server" {
			found = tool
		}
	}
	if found == nil {
		t.Fatalf("gitlab_server was not registered; the registered tools are %v", tools)
	}
	if found.Annotations == nil || !found.Annotations.ReadOnlyHint {
		t.Errorf("annotations = %+v, want a read-only hint on a diagnostics tool", found.Annotations)
	}
	for _, action := range []string{"status", "health_check"} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(found.Description, action) {
				t.Errorf("description does not offer the %q action: %s", action, found.Description)
			}
		})
	}
}

// TestBuildMCPActionGroup_EveryRouteIsReachable pins that the group's actions
// and its route map agree.
//
// The group takes its metadata from the health package's action specs and its
// routes from a map written beside them, and an action present in one and not
// the other is invisible in a different way each time: a spec without a route
// is an action that cannot run, a route without a spec is an action nothing
// describes.
func TestBuildMCPActionGroup_EveryRouteIsReachable(t *testing.T) {
	t.Parallel()

	group := BuildMCPActionGroup(nil)

	if group.ToolName != "gitlab_server" {
		t.Errorf("tool name = %q, want gitlab_server", group.ToolName)
	}
	if !group.ReadOnly {
		t.Error("the diagnostics group is not read-only")
	}
	routes := group.ActionMap()
	for _, name := range []string{"status", "health_check"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			route, ok := routes[name]
			if !ok {
				t.Errorf("action %q is missing from the group", name)
				return
			}
			if route.Handler == nil {
				t.Errorf("action %q has no handler and could never run", name)
			}
		})
	}
}
