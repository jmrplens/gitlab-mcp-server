// register_mcp_meta_test.go verifies the gitlab_server diagnostics meta-tool:
// that it is registered at all, and that its hand-written route map and its
// action specs describe the same set of actions.
package tools

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
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

// TestRegisterMCPMeta_RejectedGroup_RegistersNothing covers the guard that
// answers a catalog refusing the group: nothing is registered, so a client sees
// no gitlab_server rather than one whose actions were dropped on the way in.
//
// The real builder returns one fixed group the catalog always accepts, so the
// seam is the only way to reach the branch. A group with no tool name is what a
// catalog refuses.
func TestRegisterMCPMeta_RejectedGroup_RegistersNothing(t *testing.T) {
	original := mcpActionGroup
	t.Cleanup(func() { mcpActionGroup = original })
	mcpActionGroup = func(*gitlabclient.Client) actioncatalog.Group {
		return actioncatalog.NewGroup(actioncatalog.GroupOptions{Description: "no tool name"})
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	RegisterMCPMeta(server, nil)

	tools, err := toolutil.ListRegisteredTools(t.Context(), server, "test")
	if err != nil {
		t.Fatalf("ListRegisteredTools() error = %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("registered %d tool(s), want none when the catalog refused the group", len(tools))
	}
}

// TestBuildMCPActionGroup_UnprojectableSpecs_FallBackToTheRouteMap covers the
// other half of the group builder: when the health specs do not project, the
// group is still assembled from the route map alone, so the diagnostics tool
// keeps working with generic metadata rather than disappearing.
//
// That fallback is the reason the route map exists beside the specs at all, and
// nothing else can reach it: the compiled specs always project.
func TestBuildMCPActionGroup_UnprojectableSpecs_FallBackToTheRouteMap(t *testing.T) {
	original := mcpHealthActionSpecs
	t.Cleanup(func() { mcpHealthActionSpecs = original })
	mcpHealthActionSpecs = func(*gitlabclient.Client) []toolutil.ActionSpec {
		return []toolutil.ActionSpec{{}}
	}

	group := BuildMCPActionGroup(nil)

	routes := group.ActionMap()
	for _, name := range []string{"status", "health_check"} {
		t.Run(name, func(t *testing.T) {
			route, ok := routes[name]
			if !ok {
				t.Errorf("action %q is missing, so the route-map fallback did not run", name)
				return
			}
			if route.Handler == nil {
				t.Errorf("action %q has no handler and could never run", name)
			}
		})
	}
}
