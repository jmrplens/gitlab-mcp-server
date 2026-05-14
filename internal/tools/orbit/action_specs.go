package orbit

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for GitLab.com Orbit actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		orbitReadSpec("status", toolutil.RouteAction(client, Status), "gitlab_orbit_status", "Inspect experimental GitLab Orbit cluster health on GitLab.com."),
		orbitReadSpec("schema", toolutil.RouteAction(client, Schema), "gitlab_orbit_schema", "Inspect the experimental GitLab Orbit Knowledge Graph ontology."),
		orbitReadSpec("tools", toolutil.RouteAction(client, Tools), "gitlab_orbit_tools", "List the experimental GitLab Orbit MCP tool manifest and parameter schemas."),
		orbitReadSpec("query", toolutil.RouteAction(client, Query), "gitlab_orbit_query", "Execute a read-only experimental GitLab Orbit Knowledge Graph query."),
		orbitReadSpec("graph_status", toolutil.RouteAction(client, GraphStatus), "gitlab_orbit_graph_status", "Inspect experimental GitLab Orbit graph indexing status for one scope."),
	}
}

func orbitReadSpec(name string, route toolutil.ActionRoute, individualTool, usage string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, toolutil.ActionSpecOptions{
		Tags:             []string{"orbit", "knowledge_graph"},
		Usage:            usage,
		ReadOnly:         true,
		Idempotent:       true,
		OpenWorld:        true,
		Edition:          "premium",
		GitLabDotComOnly: true,
		OwnerPackage:     "orbit",
		IndividualTool:   toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
