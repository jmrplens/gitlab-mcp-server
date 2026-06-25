package tools

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// RegisterAll wires all catalog-backed GitLab MCP tools to the MCP server.
// Catalog actions are gated to the supplied instance tier: an action is
// registered only when its minimum required tier is at most tier (Free ⊂
// Premium ⊂ Ultimate). The function panics on catalog construction failure so
// startup fails fast rather than booting a partial MCP server.
func RegisterAll(server *mcp.Server, client *gitlabclient.Client, tier edition.Tier) {
	catalog, err := BuildActionCatalog(client, ActionCatalogOptions{Tier: tier, IncludeMCP: true})
	if err != nil {
		panic(fmt.Errorf("build individual action catalog: %w", err))
	}
	RegisterIndividualCatalogTools(server, catalog, IndividualCatalogRegisterOptions{
		IncludeStandaloneUtilities: true,
	})
	RegisterMetaStandaloneTools(server, client)
}
