package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// RegisterMetaStandaloneTools registers the standalone utility tools, which
// the action catalog does not hold, as tools of their own beside the
// catalog-backed tools of the meta and individual surfaces: the project
// discovery helper (gitlab_discover_project) and the guided elicitation
// flows (gitlab_interactive_*). The dynamic surface adds the same specs
// ([StandaloneSurfaceToolSpecs]) to its own catalog instead.
func RegisterMetaStandaloneTools(server *mcp.Server, client *gitlabclient.Client) {
	registerStandaloneUtilities(server, client)
}

// registerStandaloneUtilities projects every standalone utility spec onto
// the server. The spec list comes from
// [StandaloneSurfaceToolSpecs].
func registerStandaloneUtilities(server *mcp.Server, client *gitlabclient.Client) {
	RegisterSurfaceTools(server, StandaloneSurfaceToolSpecs(client))
}
