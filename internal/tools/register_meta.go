package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// RegisterMetaStandaloneTools registers the standalone utility tools, which
// the action catalog does not hold, as tools of their own beside the
// catalog-backed tools of the meta and individual surfaces: the project
// discovery helper (gitlab_discover_project) and the guided elicitation
// flows (gitlab_interactive_*). The dynamic surface adds the same specs
// ([StandaloneSurfaceToolSpecs]) to its own catalog instead.
func RegisterMetaStandaloneTools(server *mcp.Server, client *gitlabclient.Client) {
	RegisterMetaStandaloneToolsFor(server, client, nil)
}

// RegisterMetaStandaloneToolsFor is [RegisterMetaStandaloneTools] for a surface
// whose tools are not called by canonical ID: the next steps the standalone
// tools write are resolved by calls, [MetaActionCalls] or
// [IndividualActionCalls] of the catalog the surface serves (issue 1325). A
// nil calls leaves them as they are written.
func RegisterMetaStandaloneToolsFor(server *mcp.Server, client *gitlabclient.Client, calls toolutil.ActionCallResolver) {
	RegisterSurfaceTools(server, specsResolvingNextSteps(StandaloneSurfaceToolSpecs(client), calls))
}

// specsResolvingNextSteps returns specs with each one's formatter wrapped by
// [toolutil.WithResolvedNextSteps], the shared Markdown dispatcher standing in
// for a spec that declares none, as registration would have it.
func specsResolvingNextSteps(specs []actioncatalog.SurfaceToolSpec, calls toolutil.ActionCallResolver) []actioncatalog.SurfaceToolSpec {
	for i := range specs {
		format := specs[i].FormatResult
		if format == nil {
			format = toolutil.MarkdownForResult
		}
		specs[i].FormatResult = toolutil.WithResolvedNextSteps(format, calls)
	}
	return specs
}
