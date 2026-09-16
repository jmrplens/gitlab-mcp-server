package tools

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
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
		SchemaCacheKey:             IndividualSchemaCacheKey(tier),
	})
	RegisterMetaStandaloneTools(server, client)
	return catalog
}

// IndividualSchemaCacheKey names the process-wide compiled-schema cache entry
// for the individual projection of the catalog built for tier (see
// [IndividualCatalogRegisterOptions.SchemaCacheKey], which toolutil's
// CompileToolSchemas caches under).
//
// The tier identifies the content because the projection reads the catalog
// action's route and nothing else: the register options choose which actions
// are registered and what description each carries, never the shape of a
// schema, and the tier is what [pruneSchemaFieldsByTier] narrows a schema by.
// The instance class is deliberately not part of it — a GitLab.com catalog
// carries actions a self-managed one does not, and the tools both carry project
// the same schemas — and neither is anything about the credential, since the
// only thing a client changes in a catalog is its handlers.
//
// It is one function rather than the three copies of the same string it
// replaces (this package, cmd/server and cmd/internal/mcpsurface) because a key
// whose whole contract is "the same content is named the same way" cannot be
// left to three places to spell alike. A catalog built with
// [ActionCatalogOptions.SpecGroups] overrides is not the tier's catalog and
// must not be registered under this key.
func IndividualSchemaCacheKey(tier edition.Tier) string {
	return "individual|" + tier.String()
}
