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

// StandaloneSurfaceToolSpecs returns visible utility tools that remain outside
// ordinary GitLab API meta-tool dispatchers.
func StandaloneSurfaceToolSpecs(client *gitlabclient.Client) []actioncatalog.SurfaceToolSpec {
	return surfaces.StandaloneToolSpecs(client)
}

// ServerMaintenanceSurfaceToolSpecs returns updater-backed visible server
// maintenance tools.
func ServerMaintenanceSurfaceToolSpecs(updater *autoupdate.Updater) []actioncatalog.SurfaceToolSpec {
	return surfaces.ServerMaintenanceToolSpecs(updater)
}

// RegisterSurfaceTools registers visible tools from canonical surface specs.
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

// RegisterServerMaintenanceSurfaceTools registers visible updater tools when
// auto-update is enabled.
func RegisterServerMaintenanceSurfaceTools(server *mcp.Server, updater *autoupdate.Updater) {
	RegisterSurfaceTools(server, ServerMaintenanceSurfaceToolSpecs(updater))
}
