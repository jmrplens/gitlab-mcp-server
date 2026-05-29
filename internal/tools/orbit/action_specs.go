package orbit

import (
	"context"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns the canonical ActionSpec definitions for all GitLab.com Orbit MCP tools.
//
// Each ActionSpec describes a single public Orbit endpoint (status, schema, tools, dsl, query, graph_status)
// and is used to project both individual tools and meta-tool routes in the MCP server runtime.
//
// These specs are the single source of truth for tool registration, schema, and documentation.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		orbitReadSpec("status", orbitReadRoute(client, Status, "GitLab Orbit Status", "cluster status"), "gitlab_orbit_status", "Inspect experimental GitLab Orbit cluster health on GitLab.com."),
		orbitReadSpec("schema", orbitReadRoute(client, Schema, "GitLab Orbit Schema", "graph ontology"), "gitlab_orbit_schema", "Inspect the experimental GitLab Orbit Knowledge Graph ontology."),
		orbitReadSpec("tools", orbitReadRoute(client, Tools, "GitLab Orbit Tools", "tool manifest"), "gitlab_orbit_tools", "List the experimental GitLab Orbit MCP tool manifest and parameter schemas."),
		orbitReadSpec("dsl", orbitReadRoute(client, DSL, "GitLab Orbit DSL", "query DSL"), "gitlab_orbit_dsl", "Retrieve the experimental GitLab Orbit query DSL schema or LLM grammar."),
		orbitReadSpec("query", orbitReadRoute(client, Query, "GitLab Orbit Query", "submitted query"), "gitlab_orbit_query", "Execute a read-only experimental GitLab Orbit Knowledge Graph query."),
		orbitReadSpec("graph_status", orbitReadRoute(client, GraphStatus, "GitLab Orbit Graph Status", "requested namespace, project, or full_path"), "gitlab_orbit_graph_status", "Inspect experimental GitLab Orbit graph indexing status for one scope."),
	}
}

// orbitReadRoute wraps a handler for a read-only Orbit endpoint, providing a custom
// not-found output when the underlying API returns HTTP 404.
//
// This ensures that MCP tools for Orbit endpoints return actionable guidance when
// the feature is not enabled or the resource is missing, instead of a generic error.
func orbitReadRoute[T, R any](client *gitlabclient.Client, fn func(context.Context, *gitlabclient.Client, T) (R, error), resource, identifier string) toolutil.ActionRoute {
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

// orbitReadSpec constructs an ActionSpec for a read-only Orbit endpoint.
//
// The returned spec is tagged as "orbit" and "knowledge_graph", marked as read-only,
// and gated to GitLab.com Premium/Ultimate. Used for both meta-tool and individual tool projection.
func orbitReadSpec(name string, route toolutil.ActionRoute, individualTool, usage string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"orbit", "knowledge_graph"},
		Usage:            usage,
		OpenWorld:        true,
		Edition:          "premium",
		GitLabDotComOnly: true,
		OwnerPackage:     "orbit",
		IndividualTool:   toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
