package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// RegisterMetaStandaloneTools wires standalone utility tools that remain
// visible alongside the catalog-backed meta-tools. Today this is the
// set of interactive elicitation tools (gitlab_interactive_*).
func RegisterMetaStandaloneTools(server *mcp.Server, client *gitlabclient.Client) {
	registerStandaloneUtilities(server, client)
}

// registerStandaloneUtilities projects every standalone utility spec onto
// the server. The spec list comes from
// [StandaloneSurfaceToolSpecs].
func registerStandaloneUtilities(server *mcp.Server, client *gitlabclient.Client) {
	RegisterSurfaceTools(server, StandaloneSurfaceToolSpecs(client))
}
