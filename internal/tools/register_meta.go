package tools

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// RegisterAllMeta wires meta-tools to the MCP server.
//
// Free/CE registers 32 tools: 27 domain meta-tools, gitlab_discover_project
// and the four gitlab_interactive_* elicitation tools. Premium adds 6
// meta-tools (38), self-managed Ultimate 11 more (49), and GitLab.com
// Ultimate adds gitlab_orbit (50). The counts are pinned by the tests in
// register_test.go; gitlab_server, the diagnostics tool, is registered on
// top of every one of them and is not in these figures.
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
