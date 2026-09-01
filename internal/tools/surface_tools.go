package tools

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
