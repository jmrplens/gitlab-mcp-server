package orbit

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
			[]string{"kg.status", "knowledge_graph.status", "orbit.health", "kg.health"}, nil),
		orbitReadSpec("schema", orbitReadRoute(client, Schema, "GitLab Orbit Schema", "graph ontology"), "gitlab_orbit_schema",
			"Inspect the GitLab Orbit (Knowledge Graph) ontology: domains, node types, edge types.",
			[]string{orbitActionSchema, orbitActionDSL},
			[]string{"kg.schema", "knowledge_graph.schema", "kg.ontology", "knowledge_graph.ontology"}, nil),
		orbitReadSpec("tools", orbitReadRoute(client, Tools, "GitLab Orbit Tools", "tool manifest"), "gitlab_orbit_tools",
			"List the GitLab Orbit (Knowledge Graph) MCP tool manifest and parameter schemas.",
			[]string{orbitActionTools},
			[]string{"kg.tools", "knowledge_graph.tools", "kg.manifest", "knowledge_graph.manifest"}, nil),
		orbitReadSpec("dsl", orbitReadRoute(client, DSL, "GitLab Orbit Query Language", queryCommand+" command"), "gitlab_orbit_dsl",
			"Read the GitLab Orbit (Knowledge Graph) query language GitLab.com compiles queries in, GQL, "+
				"as Orbit's query command describes it: its grammar and the JSON Schema of its input, or the same as compact text.",
			[]string{orbitActionDSL, orbitActionQuery},
			[]string{"kg.dsl", "knowledge_graph.dsl", "kg.grammar", "knowledge_graph.grammar"}, nil),
		orbitReadSpec("query", orbitReadRoute(client, Query, "GitLab Orbit Query", "submitted query"), "gitlab_orbit_query",
			"Execute a read-only GitLab Orbit (Knowledge Graph) query in GQL, the language orbit.dsl describes, naming what orbit.schema lists. "+
				"A query is one MATCH ... RETURN statement sent as text, with at least one node bounded by its id or a literal property, "+
				"and a refused query is corrected from the message GitLab answers with.",
			[]string{orbitActionSchema, orbitActionDSL, orbitActionGraphStatus},
			[]string{"kg.query", "knowledge_graph.query", "orbit.search", "kg.search", "knowledge_graph.search"},
			queryGuidance(), toolutil.SchemaPropertyOverride("query", map[string]any{"type": []any{"string", "object"}, "examples": []any{queryExampleGQL}})),
		orbitReadSpec("graph_status", orbitReadRoute(client, GraphStatus, "GitLab Orbit Graph Status", "requested namespace, project, or full_path"), "gitlab_orbit_graph_status",
			"Inspect GitLab Orbit (Knowledge Graph) indexing status for one namespace, project, or full_path.",
			[]string{orbitActionQuery},
			[]string{"kg.indexing", "kg.index_status", "knowledge_graph.indexing", "knowledge_graph.index_status"}, nil),
	}
}

// queryGuidance is the parameter guidance of orbit.query. Each confusion is a
// rule of the GQL GitLab.com compiles that a model gets wrong, taken from the
// Orbit query reference (skills/orbit/references/gql.md in
// gitlab-org/orbit/knowledge-graph) and checked against GitLab.com on
// 2026-10-17: an unbounded node is refused with "add filters or node_ids to at
// least one node" whatever LIMIT says, a relationship type without its colon is
// a variable, OR and the other openCypher clauses the frontend does not take
// are refused, the undirected arrow is for neighbors alone, and the object
// that was the JSON DSL is answered compile_error while GitLab.com reads GQL
// (issue 1414).
func queryGuidance() map[string]toolutil.ParameterGuidance {
	return map[string]toolutil.ParameterGuidance{
		"query": {
			SemanticRole: "graph_query",
			ValueSource:  "GQL text, one MATCH ... RETURN statement, naming the entities, properties and relationship types orbit.schema lists. orbit.dsl describes the language as GitLab.com publishes it.",
			CommonConfusions: []string{
				"Every query bounds at least one node by its id or a literal property, inline as {id: 278964} or {full_path: 'gitlab-org/gitlab'}, or in a WHERE comparison. LIMIT bounds nothing, so MATCH (p:Project) RETURN p LIMIT 5 is refused.",
				"A relationship type follows a colon, -[:IN_PROJECT]->. Written without it, -[IN_PROJECT]-> declares a variable and is refused.",
				"WHERE combines predicates with AND and NOT. OR, XOR, DISTINCT, count(*), OPTIONAL MATCH, WITH and UNION are refused, and a list is negated as NOT p.visibility_level IN [...], never with NOT IN.",
				"Between labeled nodes the arrow is -> or <-. The undirected -- is for the neighbors of one node, whose far end has a variable and nothing else: MATCH (p:Project {id: 278964})--(n) RETURN n.",
				"A shortest path is MATCH path = ANY SHORTEST (a)-[:TYPE*1..3]->(b) RETURN path with both ends bounded, and an aggregate is aliased to be sorted by: RETURN p, count(mr) AS mr_count ORDER BY mr_count DESC.",
				"A query sent as a JSON object is the JSON DSL GitLab.com no longer reads, and is answered compile_error while it reads GQL. Send the query as a string.",
			},
			ExampleBinding: queryExampleGQL,
		},
	}
}

// queryExampleGQL is the GQL statement orbit.query offers as its example: the
// parameter guidance's example binding, and the one example its input schema
// carries, which the dynamic surface fills the example call it offers with.
// GitLab.com answered it 200 on 2026-10-17.
const queryExampleGQL = "MATCH (p:Project {full_path: 'gitlab-org/gitlab'}) RETURN p.id, p.full_path LIMIT 1"

// orbitReadRoute wraps a handler for a read-only Orbit endpoint, providing a custom
// not-found output when the underlying API returns HTTP 404.
//
// This ensures that MCP tools for Orbit endpoints return actionable guidance when
// the feature is not enabled or the resource is missing, instead of a generic error.
func orbitReadRoute[T, R any](client *gitlabclient.Client, fn func(context.Context, *gitlabclient.Client, T) (R, error), resource, identifier string) toolutil.ActionRoute {
	return toolutil.RouteAction(client, fn).WrapNotFound(func(map[string]any) any {
		return orbitNotFoundOutput{Resource: resource, Identifier: identifier}
	})
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
// (e.g. schema → dsl → query) without re-discovering the catalog. guidance is
// the action's parameter guidance, nil for an action whose parameters need
// none, and overrides patch the reflected input schema where it cannot say
// what the action takes: the query of orbit.query is GQL text or a JSON
// object, which a Go field typed any reflects as no type at all, and its one
// example is a GQL statement, which reflection has no way to offer.
func orbitReadSpec(name string, route toolutil.ActionRoute, individualTool, usage string, relatedActions, extraAliases []string, guidance map[string]toolutil.ParameterGuidance, overrides ...toolutil.InputSchemaOverride) toolutil.ActionSpec {
	aliases := make([]string, 0, 1+len(extraAliases))
	aliases = append(aliases, individualTool)
	aliases = append(aliases, extraAliases...)
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases:              aliases,
		Tags:                 []string{"orbit", "knowledge_graph"},
		Usage:                usage,
		RelatedActions:       relatedActions,
		ParameterGuidance:    guidance,
		InputSchemaOverrides: overrides,
		OpenWorld:            true,
		Edition:              "premium",
		GitLabDotComOnly:     true,
		OwnerPackage:         "orbit",
		IndividualTool:       toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
