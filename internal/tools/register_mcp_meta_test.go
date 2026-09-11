// register_mcp_meta_test.go verifies the gitlab_server diagnostics group: that
// its hand-written route map and its action specs describe the same set of
// actions, and that its description offers every one of them.
package tools

import (
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestBuildMCPActionGroup_EveryRouteIsReachable pins that the group's actions
// and its route map agree.
//
// The group takes its metadata from the health package's action specs and its
// routes from a map written beside them, and an action present in one and not
// the other is invisible in a different way each time: a spec without a route
// is an action that cannot run, a route without a spec is an action nothing
// describes. The alias is part of that: health_check and status are the same
// action, and a client that learned one name must not get an unknown-action
// error.
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
			if !strings.Contains(group.Description, name) {
				t.Errorf("description does not offer the %q action: %s", name, group.Description)
			}
		})
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

// TestBuildMCPActionGroup_UnprojectableSpecs_AreReported covers the other half
// of that fallback: it is silent about the group it assembles, so the failure
// that made it necessary has to be in the log.
//
// The tool keeps working on generic metadata, which is the point, and which is
// also why nothing else would ever notice: the specs describing every action
// are gone and the group still answers. The log line is the only trace, so it
// must be written when the projection failed and never when it succeeded,
// where it would report a fault in a startup that had none.
func TestBuildMCPActionGroup_UnprojectableSpecs_AreReported(t *testing.T) {
	const message = "failed to build MCP health action specs"

	t.Run("specs that do not project are reported", func(t *testing.T) {
		original := mcpHealthActionSpecs
		t.Cleanup(func() { mcpHealthActionSpecs = original })
		mcpHealthActionSpecs = func(*gitlabclient.Client) []toolutil.ActionSpec {
			return []toolutil.ActionSpec{{}}
		}
		output := captureSlogOutput(t)

		BuildMCPActionGroup(nil)

		if !strings.Contains(output.String(), message) {
			t.Errorf("startup log = %s, want the projection failure reported", output.String())
		}
	})

	t.Run("the compiled specs report nothing", func(t *testing.T) {
		output := captureSlogOutput(t)

		BuildMCPActionGroup(nil)

		if strings.Contains(output.String(), message) {
			t.Errorf("startup log = %s, want nothing reported when the specs project", output.String())
		}
	})
}
