// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical Orbit action IDs, the one form every surface resolves. They
// are referenced from the RelatedActions slices of every spec and from the
// next steps the Markdown cards end on, so a single source of truth keeps
// the cross-links consistent and makes the chain (schema → dsl → query →
// graph_status → query) easy to audit.
const (
	orbitActionStatus      = "orbit.status"
	orbitActionSchema      = "orbit.schema"
	orbitActionTools       = "orbit.tools"
	orbitActionDSL         = "orbit.dsl"
	orbitActionQuery       = "orbit.query"
	orbitActionGraphStatus = "orbit.graph_status"

	orbitActionQueryTemplates = "orbit.query_templates"
	orbitActionResponseFormat = "orbit.response_format"
	orbitActionCommandList    = "orbit.command_list"
	orbitActionSkillList      = "orbit.skill_list"
	orbitActionSkillGet       = "orbit.skill_get"
	orbitActionSkillFile      = "orbit.skill_file"
	orbitActionNamedQuery     = "orbit.named_query"
	orbitActionContext        = "orbit.context"
)

// ActionSpecs returns the canonical ActionSpec definitions for all GitLab.com Orbit MCP tools.
//
// Each ActionSpec describes one Orbit read, in the order the actions were added, answered by GitLab's
// GraphQL API, its Orbit MCP endpoint or an Orbit REST route, and is used to project both individual
// tools and meta-tool routes in the MCP server runtime.
//
// These specs are the single source of truth for tool registration, schema, and documentation.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		orbitReadSpec("status", orbitReadRoute(client, Status, "GitLab Orbit Status", "service status"), "gitlab_orbit_status",
			"Inspect the health of the GitLab Orbit (Knowledge Graph) service on GitLab.com: its state, version and the state of each component.",
			[]string{orbitActionStatus},
			[]string{"kg.status", "knowledge_graph.status", "orbit.health", "kg.health"}, orbitInput{}),
		orbitReadSpec("schema", orbitReadRoute(client, Schema, "GitLab Orbit Schema", "graph schema"), "gitlab_orbit_schema",
			"Inspect the GitLab Orbit (Knowledge Graph) schema: domains, node kinds and edge kinds, with the properties of the node kinds named in expand, or the schema as compact text.",
			[]string{orbitActionSchema, orbitActionDSL},
			[]string{"kg.schema", "knowledge_graph.schema", "kg.ontology", "knowledge_graph.ontology"}, orbitInput{}),
		orbitReadSpec("tools", orbitReadRoute(client, Tools, "GitLab Orbit Tools", "tool list"), "gitlab_orbit_tools",
			"List the tools GitLab's Orbit (Knowledge Graph) MCP endpoint serves and the input schema of each: list_commands and invoke_command, "+
				"whose commands orbit.command_list describes with the action of this server that runs each.",
			[]string{orbitActionTools, orbitActionCommandList},
			[]string{"kg.tools", "knowledge_graph.tools", "kg.manifest", "knowledge_graph.manifest"}, orbitInput{}),
		orbitReadSpec("dsl", orbitReadRoute(client, DSL, "GitLab Orbit Query Language", queryCommand+" command"), "gitlab_orbit_dsl",
			"Read the GitLab Orbit (Knowledge Graph) query language GitLab.com compiles queries in, GQL, "+
				"as Orbit's query command describes it: its grammar and the JSON Schema of its input, or the same as compact text.",
			[]string{orbitActionDSL, orbitActionQuery},
			[]string{"kg.dsl", "knowledge_graph.dsl", "kg.grammar", "knowledge_graph.grammar"}, orbitInput{}),
		orbitReadSpec("query", orbitReadRoute(client, Query, "GitLab Orbit Query", "submitted query"), "gitlab_orbit_query",
			"Execute a read-only GitLab Orbit (Knowledge Graph) query in GQL, the language orbit.dsl describes, naming what orbit.schema lists. "+
				"A query is one MATCH ... RETURN statement sent as text, with at least one node bounded by its id or a literal property, "+
				"and a refused query is corrected from the message GitLab answers with.",
			[]string{orbitActionSchema, orbitActionDSL, orbitActionGraphStatus, orbitActionQueryTemplates},
			[]string{"kg.query", "knowledge_graph.query", "orbit.search", "kg.search", "knowledge_graph.search"}, queryInput()),
		orbitReadSpec("graph_status", orbitReadRoute(client, GraphStatus, "GitLab Orbit Graph Status", "requested namespace, project, or full_path"), "gitlab_orbit_graph_status",
			"Inspect GitLab Orbit (Knowledge Graph) indexing status for one group or project, by namespace_id, project_id or full_path: whether Orbit is on, the indexing state of each area, the project counts and the indexed items of each domain, "+
				"and for a top-level group its Orbit access and trial, which decide whether GitLab.com serves the index at all.",
			[]string{orbitActionQuery},
			[]string{"kg.indexing", "kg.index_status", "knowledge_graph.indexing", "knowledge_graph.index_status"}, orbitInput{}),
		orbitReadSpec("query_templates", orbitReadRoute(client, QueryTemplates, "GitLab Orbit Query Templates", "query templates"), "gitlab_orbit_query_templates",
			"List the GitLab Orbit (Knowledge Graph) query templates GitLab.com renders for the token's user: the name, description and GQL text of each. "+
				"The text binds the user's own id where a template asks about the user, so it is that user's alone. The named queries that take parameters are not listed.",
			[]string{orbitActionNamedQuery, orbitActionQuery, orbitActionDSL},
			[]string{"kg.templates", "knowledge_graph.templates", "orbit.templates", "kg.query_templates"}, orbitInput{}),
		orbitReadSpec("response_format", orbitReadRoute(client, QueryResponseFormat, "GitLab Orbit Response Format", "response format"), "gitlab_orbit_response_format",
			"Read the JSON Schema of the raw answer a GitLab Orbit (Knowledge Graph) query returns, with its version, or the same schema as compact text.",
			[]string{orbitActionQuery, orbitActionDSL},
			[]string{"kg.response_format", "knowledge_graph.response_format", "kg.result_schema", "orbit.result_schema"}, orbitInput{}),
		orbitReadSpec("command_list", orbitReadRoute(client, CommandList, "GitLab Orbit Commands", "command list"), "gitlab_orbit_command_list",
			"List the commands GitLab's Orbit (Knowledge Graph) MCP endpoint runs, with the description and input schema of each and the action of this server that runs it: "+
				"query_graph is orbit.query, get_graph_schema is orbit.schema and get_response_format is orbit.response_format.",
			[]string{orbitActionTools, orbitActionDSL},
			[]string{"kg.commands", "knowledge_graph.commands", "orbit.commands", "kg.command_list"}, orbitInput{}),
		orbitReadSpec("skill_list", orbitReadRoute(client, SkillList, "GitLab Orbit Skills", "deployed skills"), "gitlab_orbit_skill_list",
			"List the GitLab Orbit (Knowledge Graph) skills GitLab.com has deployed: the name, version, compatibility and description of each.",
			[]string{orbitActionSkillGet},
			[]string{"kg.skills", "knowledge_graph.skills", "orbit.skills", "kg.skill_list"}, orbitInput{}),
		orbitReadSpec("skill_get", orbitSkillRoute(client, SkillGet, skillNotFound), "gitlab_orbit_skill_get",
			"Inspect one deployed GitLab Orbit (Knowledge Graph) skill by name: its version, compatibility and the files it holds, with the path, digest and size of each. File content is read one file at a time.",
			[]string{orbitActionSkillList, orbitActionSkillFile},
			[]string{"kg.skill", "knowledge_graph.skill", "orbit.skill", "kg.skill_get"}, orbitInput{}),
		orbitReadSpec("skill_file", orbitSkillRoute(client, SkillFile, skillFileNotFound), "gitlab_orbit_skill_file",
			"Read one file of a deployed GitLab Orbit (Knowledge Graph) skill, by skill name and the file's path relative to the skill root, as GitLab.com serves it.",
			[]string{orbitActionSkillGet},
			[]string{"kg.skill_file", "knowledge_graph.skill_file", "orbit.skill_content", "kg.skill_content"}, skillFileInput()),
		orbitReadSpec("named_query", orbitReadRoute(client, NamedQuery, "GitLab Orbit Named Queries", "named query service"), "gitlab_orbit_named_query",
			"Run one of GitLab Orbit's (Knowledge Graph) named queries by name: a template orbit.query_templates lists, which takes no parameters, "+
				"or one of the queries that take node ids, an entity, a file path or search text, which the description of parameters lists with what each takes. The answer is the envelope orbit.query returns.",
			[]string{orbitActionQueryTemplates, orbitActionQuery, orbitActionContext},
			[]string{"kg.named_query", "knowledge_graph.named_query", "orbit.run_template", "kg.run_template"}, namedQueryInput()),
		orbitReadSpec("context", orbitReadRoute(client, EntityContext, "GitLab Orbit Context", "entity context service"), "gitlab_orbit_context",
			"Resolve one reference to a work item, merge request, job, pipeline, branch or commit through GitLab Orbit (Knowledge Graph) and read its summary, discussions, changed files and failed jobs' logs, "+
				"and with expand the graph neighborhood of one of the first three, one billable query. "+
				"A reference it cannot resolve is found false with the reason, not_found also for an object the user or token may not read: "+
				"a fine-grained token needs, beside read_orbit, permission to read on the object's own project or group. "+
				"Summary parts it may not read are left out, and a failed expansion carries its reason.",
			[]string{orbitActionNamedQuery, orbitActionQuery},
			[]string{"kg.context", "knowledge_graph.context", "orbit.resolve", "kg.resolve"}, contextInput()),
	}
}

// orbitInput is what an Orbit action says about its parameters beyond what
// reflection reads off its input struct. guidance is the action's parameter
// guidance, nil for an action whose parameters need none, and overrides patch
// the reflected input schema where it cannot say what the action takes.
type orbitInput struct {
	guidance  map[string]toolutil.ParameterGuidance
	overrides []toolutil.InputSchemaOverride
}

// queryInput is what orbit.query says about its parameters: its guidance, and
// an override of the query property whose one example is a GQL statement,
// which reflection has no way to offer.
func queryInput() orbitInput {
	return orbitInput{
		guidance: queryGuidance(),
		overrides: []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("query", map[string]any{"examples": []any{queryExampleGQL}}),
		},
	}
}

// queryGuidance is the parameter guidance of orbit.query. Each confusion is a
// rule of the GQL GitLab.com compiles that a model gets wrong, taken from the
// Orbit query reference (skills/orbit/references/gql.md in
// gitlab-org/orbit/knowledge-graph) and checked against GitLab.com on
// 2026-10-17: an unbounded node is refused with "add filters or node_ids to at
// least one node" whatever LIMIT says, a relationship type without its colon is
// a variable, OR and the other openCypher clauses the frontend does not take
// are refused, the undirected arrow is for neighbors alone, and the JSON DSL
// GitLab.com removed on 2026-10-27 (gitlab-org/gitlab!261947) is answered
// compile_error when it is sent as text (issue 1414).
func queryGuidance() map[string]toolutil.ParameterGuidance {
	return map[string]toolutil.ParameterGuidance{
		"query": {
			SemanticRole: "graph_query",
			ValueSource:  "GQL text, one MATCH ... RETURN statement, naming the entities, properties and relationship types orbit.schema lists. orbit.dsl describes the language as GitLab.com publishes it.",
			CommonConfusions: []string{
				"Every query bounds at least one node by its id or a literal property, inline as {id: 278964} or {full_path: 'gitlab-org/gitlab'}, or in a WHERE comparison. LIMIT bounds nothing, so MATCH (p:Project) RETURN p LIMIT 5 is refused.",
				"A relationship type follows a colon, -[:IN_PROJECT]->, and several types are joined by |, -[:IN_PROJECT|CONTAINS*1..3]->. Written without the colon, -[IN_PROJECT]-> declares a variable and is refused.",
				"WHERE combines predicates on one node's properties with AND and NOT. OR, XOR, RETURN DISTINCT, count(*), OPTIONAL MATCH, WITH and UNION are refused, and a list is negated as NOT p.visibility_level IN [...], never with NOT IN. count(DISTINCT x) counts distinct values.",
				"Between labeled nodes the arrow is -> or <-. The undirected -- is for the neighbors of one node, whose far end has a variable and nothing else: MATCH (p:Project {id: 278964})--(n) RETURN n.",
				"A shortest path is MATCH path = ANY SHORTEST (a)-[:TYPE*1..3]->(b) RETURN path with both ends bounded and its relationship types named, one type or several joined by |, since a wildcard -[*1..3]-> is refused there. An aggregate is aliased to be sorted by, with one sort key: RETURN p, count(mr) AS mr_count ORDER BY mr_count DESC.",
				"A page is SKIP and LIMIT: SKIP 0 to 10000, LIMIT 1 to 1000, as in ORDER BY p.id SKIP 20 LIMIT 20. PAGE and AFTER are refused.",
				"A query written as a JSON object is the JSON DSL GitLab.com removed, and is answered compile_error at line 1, column 1. Send a GQL statement.",
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

// orbitSkillRoute is [orbitReadRoute] for a read of one skill or one of its
// files, whose 404 says one of two things: GitLab serves the user no skills,
// answered by the availability result ([answerSkillsUnavailable]), or it holds
// no such skill or file, answered by what notFound builds from the arguments.
func orbitSkillRoute[T, R any](client *gitlabclient.Client, fn func(context.Context, *gitlabclient.Client, T) (R, error), notFound func(map[string]any) any) toolutil.ActionRoute {
	return toolutil.RouteAction(client, fn).WrapHandler(answerSkillsUnavailable).WrapNotFound(notFound)
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
// (e.g. schema → dsl → query) without re-discovering the catalog. input is
// what the action says about its parameters beyond what reflection reads off
// its struct, the zero value for an action that says nothing more.
func orbitReadSpec(name string, route toolutil.ActionRoute, individualTool, usage string, relatedActions, extraAliases []string, input orbitInput) toolutil.ActionSpec {
	aliases := make([]string, 0, 1+len(extraAliases))
	aliases = append(aliases, individualTool)
	aliases = append(aliases, extraAliases...)
	return toolutil.NewReadActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases:              aliases,
		Tags:                 []string{"orbit", "knowledge_graph"},
		Usage:                usage,
		RelatedActions:       relatedActions,
		ParameterGuidance:    input.guidance,
		InputSchemaOverrides: input.overrides,
		OpenWorld:            true,
		Edition:              "premium",
		GitLabDotComOnly:     true,
		OwnerPackage:         "orbit",
		IndividualTool:       toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	})
}
