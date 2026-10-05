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
		orbitReadSpec("dsl", orbitReadRoute(client, DSL, "GitLab Orbit DSL", "query DSL"), "gitlab_orbit_dsl",
			"Retrieve the GitLab Orbit (Knowledge Graph) query DSL schema or LLM grammar.",
			[]string{orbitActionDSL, orbitActionQuery},
			[]string{"kg.dsl", "knowledge_graph.dsl", "kg.grammar", "knowledge_graph.grammar"}, nil),
		orbitReadSpec("query", orbitReadRoute(client, Query, "GitLab Orbit Query", "submitted query"), "gitlab_orbit_query",
			"Execute a read-only GitLab Orbit (Knowledge Graph) query (traversal, aggregation, neighbors, or path_finding) "+
				"in the version 12 query DSL, which lists every node in a nodes array. "+
				"Read orbit.dsl for the grammar and orbit.schema for the entities and relationship types, "+
				"and correct a refused query from the message GitLab answers with.",
			[]string{orbitActionSchema, orbitActionDSL, orbitActionGraphStatus},
			[]string{"kg.query", "knowledge_graph.query", "orbit.search", "kg.search", "knowledge_graph.search"},
			queryGuidance()),
		orbitReadSpec("graph_status", orbitReadRoute(client, GraphStatus, "GitLab Orbit Graph Status", "requested namespace, project, or full_path"), "gitlab_orbit_graph_status",
			"Inspect GitLab Orbit (Knowledge Graph) indexing status for one namespace, project, or full_path.",
			[]string{orbitActionQuery},
			[]string{"kg.indexing", "kg.index_status", "knowledge_graph.indexing", "knowledge_graph.index_status"}, nil),
	}
}

// queryGuidance is the parameter guidance of orbit.query. Each confusion is a
// rule of version 12 of the DSL, the one GitLab.com serves, that a model gets
// wrong. The first three refuse an older shape of the DSL, the one this server
// taught until issue 1031: a top-level node, an {op, value} filter and a node
// named inside neighbors. The aggregation keyed by its function is version
// 12's own. Two of the rules they state are no change of the DSL at all: the
// compiler already applied outgoing as the neighbors direction while the
// schema declared both, and the validator refused a path without rel_types
// while the schema allowed one (register rows 76 and 75).
func queryGuidance() map[string]toolutil.ParameterGuidance {
	return map[string]toolutil.ParameterGuidance{
		"query": {
			SemanticRole: "graph_query",
			ValueSource:  "A JSON object in the version 12 DSL that orbit.dsl serves, naming entities, properties and relationship types orbit.schema lists.",
			CommonConfusions: []string{
				"A top-level node is refused. Every query lists its node selectors in nodes, even a query with one node.",
				`A filter is a bare value for equality or an object keyed by operators, such as {"starts_with": "gitlab-org/"} or {"gte": 1, "lt": 9}. The older {op, value} form is refused.`,
				"A neighbors query names no node inside neighbors. Its center is its one node, and neighbors holds only direction and rel_types. direction defaults to outgoing, so pass both to get every relationship of the center.",
				`A path_finding query needs rel_types in path, ["*"] for any relationship type, and the only path type is shortest.`,
				`An aggregation is keyed by its function, {"count": "mr", "as": "mr_count"}. A group_by entry is a node id or a node id and a property joined by a dot. order_by and aggregation_sort are strings, with a leading - for descending.`,
			},
			ExampleBinding: `{"query_type":"traversal","nodes":[{"id":"p","entity":"Project","filters":{"full_path":{"starts_with":"gitlab-org/"}},"columns":["id","full_path"]}],"limit":20}`,
		},
	}
}

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
// none.
func orbitReadSpec(name string, route toolutil.ActionRoute, individualTool, usage string, relatedActions, extraAliases []string, guidance map[string]toolutil.ParameterGuidance) toolutil.ActionSpec {
	aliases := make([]string, 0, 1+len(extraAliases))
	aliases = append(aliases, individualTool)
	aliases = append(aliases, extraAliases...)
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases:           aliases,
		Tags:              []string{"orbit", "knowledge_graph"},
		Usage:             usage,
		RelatedActions:    relatedActions,
		ParameterGuidance: guidance,
		OpenWorld:         true,
		Edition:           "premium",
		GitLabDotComOnly:  true,
		OwnerPackage:      "orbit",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
