package tools

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/autoupdate"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/surfaces"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// StandaloneSurfaceToolSpecs returns visible utility tools that remain
// outside ordinary GitLab API meta-tool dispatchers. The list currently
// includes the interactive elicitation tools (gitlab_interactive_*) and
// the project discovery helper (gitlab_discover_project).
func StandaloneSurfaceToolSpecs(client *gitlabclient.Client) []actioncatalog.SurfaceToolSpec {
	return surfaces.StandaloneToolSpecs(client)
}

// ServerMaintenanceSurfaceToolSpecs returns updater-backed visible server
// maintenance tools. When the updater is nil, the returned slice is empty
// so RegisterServerMaintenanceSurfaceTools has nothing to register.
func ServerMaintenanceSurfaceToolSpecs(updater *autoupdate.Updater) []actioncatalog.SurfaceToolSpec {
	return surfaces.ServerMaintenanceToolSpecs(updater)
}

// RegisterSurfaceTools registers visible tools from canonical surface
// specs. Each spec is projected through [toolutil.RegisterSurfaceToolFromSpec]
// and panics on projection failure because surface tools are part of the
// declared MCP surface and a malformed spec should fail startup.
func RegisterSurfaceTools(server *mcp.Server, specs []actioncatalog.SurfaceToolSpec) {
	for _, spec := range specs {
		actionSpec, err := spec.ActionSpec()
		if err != nil {
			panic(fmt.Errorf("project surface tool %s: %w", spec.Name, err))
		}
		toolutil.RegisterSurfaceToolFromSpec(server, actionSpec, toolutil.SurfaceToolRegisterOptions{
			Icons:        spec.Icons,
			FormatResult: spec.FormatResult,
		})
	}
}

// RegisterServerMaintenanceSurfaceTools registers visible updater tools
// when auto-update is enabled. When the updater is nil, no tools are
// registered.
func RegisterServerMaintenanceSurfaceTools(server *mcp.Server, updater *autoupdate.Updater) {
	RegisterSurfaceTools(server, ServerMaintenanceSurfaceToolSpecs(updater))
}
