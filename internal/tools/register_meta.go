package tools

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// RegisterAllMeta wires meta-tools to the MCP server.
//
// Base: 33 tools = 29 meta-tools (25 inline + 3 delegated + 1 standalone)
// + 4 standalone interactive elicitation tools (gitlab_interactive_*).
// Enterprise: +14 inline meta-tools = 47 tools total; GitLab.com
// Enterprise also adds gitlab_orbit.
//
// Each meta-tool dispatches to the underlying handler based on the
// "action" parameter. This reduces token usage for LLMs while preserving
// full functionality. Interactive tools cannot be consolidated because
// they require multi-round MCP elicitation/create exchanges with the
// client.
//
// Returns an error if the action catalog cannot be built or if wiring
// tools to the MCP server fails. The tier gates Premium/Ultimate actions per
// the central catalog tier filter.
func RegisterAllMeta(server *mcp.Server, client *gitlabclient.Client, tier edition.Tier) error {
	catalog, err := BuildActionCatalog(client, ActionCatalogOptions{Tier: tier})
	if err != nil {
		return fmt.Errorf("failed to build action catalog: %w", err)
	}
	RegisterMetaCatalog(server, catalog)
	RegisterMetaStandaloneTools(server, client)
	return nil
}

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
