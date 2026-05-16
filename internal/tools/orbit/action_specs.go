package orbit

import (
	"context"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for GitLab.com Orbit actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		orbitReadSpec("status", orbitReadRoute(client, Status, "GitLab Orbit Status", "cluster status"), "gitlab_orbit_status", "Inspect experimental GitLab Orbit cluster health on GitLab.com."),
		orbitReadSpec("schema", orbitReadRoute(client, Schema, "GitLab Orbit Schema", "graph ontology"), "gitlab_orbit_schema", "Inspect the experimental GitLab Orbit Knowledge Graph ontology."),
		orbitReadSpec("tools", orbitReadRoute(client, Tools, "GitLab Orbit Tools", "tool manifest"), "gitlab_orbit_tools", "List the experimental GitLab Orbit MCP tool manifest and parameter schemas."),
		orbitReadSpec("query", orbitReadRoute(client, Query, "GitLab Orbit Query", "submitted query"), "gitlab_orbit_query", "Execute a read-only experimental GitLab Orbit Knowledge Graph query."),
		orbitReadSpec("graph_status", orbitReadRoute(client, GraphStatus, "GitLab Orbit Graph Status", "requested namespace, project, or full_path"), "gitlab_orbit_graph_status", "Inspect experimental GitLab Orbit graph indexing status for one scope."),
	}
}

func orbitReadRoute[T any, R any](client *gitlabclient.Client, fn func(context.Context, *gitlabclient.Client, T) (R, error), resource, identifier string) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, fn)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return orbitNotFoundOutput{Resource: resource, Identifier: identifier}, nil
		}
		return result, err
	}
	return route
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
