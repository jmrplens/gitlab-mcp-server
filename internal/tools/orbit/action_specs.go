package orbit

import (
	"context"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical Orbit action IDs. They are referenced from the
// RelatedActions slices of every spec, so a single source of truth
// keeps the cross-links consistent and makes the chain (schema →
// dsl → query → graph_status → query) easy to audit.
const (
	orbitActionStatus      = "orbit.status"
	orbitActionSchema      = "orbit.schema"
	orbitActionTools       = "orbit.tools"
	orbitActionDSL         = "orbit.dsl"
	orbitActionQuery       = "orbit.query"
	orbitActionGraphStatus = "orbit.graph_status"
)

// ActionSpecs returns the canonical ActionSpec definitions for all GitLab.com Orbit MCP tools.
//
// Each ActionSpec describes a single public Orbit endpoint (status, schema, tools, dsl, query, graph_status)
// and is used to project both individual tools and meta-tool routes in the MCP server runtime.
//
// These specs are the single source of truth for tool registration, schema, and documentation.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		orbitReadSpec("status", orbitReadRoute(client, Status, "GitLab Orbit Status", "cluster status"), "gitlab_orbit_status",
			"Inspect GitLab Orbit (Knowledge Graph) cluster health on GitLab.com.",
			[]string{orbitActionStatus},
			[]string{"kg.status", "knowledge_graph.status", "orbit.health", "kg.health"}),
		orbitReadSpec("schema", orbitReadRoute(client, Schema, "GitLab Orbit Schema", "graph ontology"), "gitlab_orbit_schema",
			"Inspect the GitLab Orbit (Knowledge Graph) ontology: domains, node types, edge types.",
			[]string{orbitActionSchema, orbitActionDSL},
			[]string{"kg.schema", "knowledge_graph.schema", "kg.ontology", "knowledge_graph.ontology"}),
		orbitReadSpec("tools", orbitReadRoute(client, Tools, "GitLab Orbit Tools", "tool manifest"), "gitlab_orbit_tools",
			"List the GitLab Orbit (Knowledge Graph) MCP tool manifest and parameter schemas.",
			[]string{orbitActionTools},
			[]string{"kg.tools", "knowledge_graph.tools", "kg.manifest", "knowledge_graph.manifest"}),
		orbitReadSpec("dsl", orbitReadRoute(client, DSL, "GitLab Orbit DSL", "query DSL"), "gitlab_orbit_dsl",
			"Retrieve the GitLab Orbit (Knowledge Graph) query DSL schema or LLM grammar.",
			[]string{orbitActionDSL, orbitActionQuery},
			[]string{"kg.dsl", "knowledge_graph.dsl", "kg.grammar", "knowledge_graph.grammar"}),
		orbitReadSpec("query", orbitReadRoute(client, Query, "GitLab Orbit Query", "submitted query"), "gitlab_orbit_query",
			"Execute a read-only GitLab Orbit (Knowledge Graph) query (traversal, aggregation, neighbors, or path_finding).",
			[]string{orbitActionSchema, orbitActionDSL, orbitActionGraphStatus},
			[]string{"kg.query", "knowledge_graph.query", "orbit.search", "kg.search", "knowledge_graph.search"}),
		orbitReadSpec("graph_status", orbitReadRoute(client, GraphStatus, "GitLab Orbit Graph Status", "requested namespace, project, or full_path"), "gitlab_orbit_graph_status",
			"Inspect GitLab Orbit (Knowledge Graph) indexing status for one namespace, project, or full_path.",
			[]string{orbitActionQuery},
			[]string{"kg.indexing", "kg.index_status", "knowledge_graph.indexing", "knowledge_graph.index_status"}),
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
//
// The extraAliases slice is appended to the canonical `{individualTool}` alias and the
// `kg.*` / `knowledge_graph.*` shorthands so the dynamic find tool can resolve common
// natural-language queries such as "kg status" or "knowledge graph query". The
// relatedActions slice is surfaced as `RelatedActions` so the LLM can chain calls
// (e.g. schema → dsl → query) without re-discovering the catalog.
func orbitReadSpec(name string, route toolutil.ActionRoute, individualTool, usage string, relatedActions, extraAliases []string) toolutil.ActionSpec {
	aliases := make([]string, 0, 1+len(extraAliases))
	aliases = append(aliases, individualTool)
	aliases = append(aliases, extraAliases...)
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases:          aliases,
		Tags:             []string{"orbit", "knowledge_graph"},
		Usage:            usage,
		RelatedActions:   relatedActions,
		OpenWorld:        true,
		Edition:          "premium",
		GitLabDotComOnly: true,
		OwnerPackage:     "orbit",
		IndividualTool:   toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
