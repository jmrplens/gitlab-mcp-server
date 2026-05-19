package tools

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// RegisterAll wires all catalog-backed GitLab MCP tools to the MCP server.
// When enterprise is false, Premium/Ultimate-only catalog actions are not registered.
func RegisterAll(server *mcp.Server, client *gitlabclient.Client, enterprise bool) {
	catalog, err := BuildActionCatalog(client, ActionCatalogOptions{Enterprise: enterprise, IncludeMCP: true})
	if err != nil {
		panic(fmt.Errorf("build individual action catalog: %w", err))
	}
	RegisterIndividualCatalogTools(server, catalog, IndividualCatalogRegisterOptions{
		IncludeStandaloneUtilities: true,
	})
	RegisterMetaStandaloneTools(server, client)
}
