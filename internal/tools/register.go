package tools

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

// RegisterAll wires all catalog-backed GitLab MCP tools to the MCP server.
// Catalog actions are gated to the supplied instance tier: an action is
// registered only when its minimum required tier is at most tier (Free ⊂
// Premium ⊂ Ultimate). The function panics on catalog construction failure so
// startup fails fast rather than booting a partial MCP server.
// It returns the catalog it registered from. Callers that need to map a tool
// name back to its canonical action, which on this surface nothing else can do
// because the names are declared per ActionSpec rather than derived, would
// otherwise have to build the same catalog a second time. Discarding it is what
// left telemetry unable to record an action on the individual surface: the
// resolver degraded correctly to no attribute, on the one surface with a
// thousand tools.
func RegisterAll(server *mcp.Server, client *gitlabclient.Client, tier edition.Tier) *actioncatalog.Catalog {
	catalog, err := BuildActionCatalog(client, ActionCatalogOptions{Tier: tier, IncludeMCP: true})
	if err != nil {
		panic(fmt.Errorf("build individual action catalog: %w", err))
	}
	RegisterIndividualCatalogTools(server, catalog, IndividualCatalogRegisterOptions{
		IncludeStandaloneUtilities: true,
		SchemaCacheKey:             "individual|" + tier.String(),
	})
	RegisterMetaStandaloneTools(server, client)
	return catalog
}
