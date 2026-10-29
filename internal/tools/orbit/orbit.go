// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ResponseFormatInput is the response_format selector of orbit.dsl, the one
// Orbit action besides orbit.query and orbit.schema whose source still renders
// an llm text: the list_commands tool of GitLab's Orbit MCP endpoint. The query
// takes a third value and declares its own field ([QueryInput.ResponseFormat]).
type ResponseFormatInput struct {
	// ResponseFormat selects the answer's shape. GitLab declares two values
	// (the format property of list_commands' input schema):
	//   - "raw": structured JSON
	//   - "llm": compact text optimized for LLM consumption
	//
	// "json" is accepted here as a synonym of "raw" and sent as "raw": it was
	// offered here as a value of its own until GitLab's refusal of it was
	// measured on the REST routes this action read before.
	//
	// When empty the handler asks for "raw", the default of the REST route
	// this action read until GitLab.com removed it on 2026-10-28; the MCP tool
	// defaults to llm.
	ResponseFormat string `json:"response_format,omitempty" jsonschema:"Response format: raw or llm (json is read as raw). When omitted, raw."`
}

// StructuredFormatInput is the response_format of the two Orbit actions GitLab
// now answers through GraphQL, orbit.status and orbit.graph_status. GraphQL
// has one shape, so the parameter is accepted for the callers that send it and
// changes nothing; a value the REST routes never took is still refused, so a
// typo is not read as a choice.
type StructuredFormatInput struct {
	// ResponseFormat is raw, llm or json, all answered with the structured
	// output. It is checked and not sent: GitLab's GraphQL API has no llm
	// rendering of the Orbit status or of a namespace's index.
	ResponseFormat string `json:"response_format,omitempty" jsonschema:"Accepted for compatibility: raw, llm or json. GitLab answers this through GraphQL, which has one structured shape, so every value returns the same output."`
}

// StatusInput holds parameters for retrieving Orbit service health.
type StatusInput struct {
	StructuredFormatInput
}

// SchemaInput holds parameters for retrieving the Orbit graph schema.
type SchemaInput struct {
	// Expand lists node names whose properties should be included in the
	// response. Without it the schema lists every node kind and edge kind
	// without the properties, which is most of the answer's size.
	Expand []string `json:"expand,omitempty" jsonschema:"Node names whose properties to include, for example Project or MergeRequest. Without it, nodes are listed without properties."`
	// Format selects the schema response shape: "raw" (GitLab's GraphQL
	// schema, the default) or "llm" (the compact text the get_graph_schema
	// command of GitLab's Orbit MCP endpoint answers), with "json" read as
	// "raw". Whichever of the two input names carries it, it means the same.
	Format string `json:"format,omitempty" jsonschema:"Schema response format: raw (structured, the default) or llm (compact text; json is read as raw)."`
	// ResponseFormat is an alias for Format. Must match Format when both are
	// set.
	ResponseFormat string `json:"response_format,omitempty" jsonschema:"Alias for format. Must match format when both are set."`
}

// ToolsInput is the input for listing the Orbit MCP tool manifest. The tool
// list takes no parameters; the type exists so the handler signature is
// uniform with the other Orbit handlers.
type ToolsInput struct{}

// DSLInput holds parameters for retrieving the Orbit query language.
type DSLInput struct {
	ResponseFormatInput
}

// DSLOutput is the query language GitLab.com compiles orbit.query in, as the
// Orbit command a graph query runs as describes it.
//
// GitLab.com reads every query as GQL since it turned orbit_gql_queries on for
// everyone at 01:06 UTC on 2026-10-15 (issue 1414). What it publishes for GQL
// is the query_graph command of the Orbit command catalog: a description of
// the grammar and the JSON Schema of the command's input. The catalog was
// served at GET /api/v4/orbit/agent/commands until gitlab-org/gitlab!266009
// removed the route on 2026-10-28; it is now the list_commands tool of the
// Orbit MCP endpoint, POST /api/v4/orbit/mcp, which answers the same list.
type DSLOutput struct {
	toolutil.HintableOutput
	// ResponseFormat echoes the response_format that was asked for ("raw" for
	// the command as a JSON object, "llm" for it as compact text), or nothing
	// when the caller chose none and the handler asked for raw.
	ResponseFormat string `json:"response_format,omitempty"`
	// FormattedText is the command as compact text, which is the whole of
	// GitLab's answer when the caller selected the "llm" response format.
	FormattedText string `json:"formatted_text,omitempty"`
	// Name is the name of the Orbit command a query runs as, query_graph,
	// under the key GitLab sends it with.
	Name string `json:"name,omitempty"`
	// Description is the grammar of the query language as the command
	// describes it.
	Description string `json:"description,omitempty"`
	// Parameters is the decoded JSON Schema of the command's input, whose
	// query property is the query text orbit.query sends.
	Parameters any `json:"parameters,omitempty"`
}

// QueryInput holds parameters for executing an Orbit Knowledge Graph query.
//
// The Query field is GQL text, the read-only subset of openCypher 9 syntax the
// Knowledge Graph compiles, which is the language GitLab.com reads every query
// in since it turned orbit_gql_queries on for everyone at 01:06 UTC on
// 2026-10-15. [DSL] describes it from what GitLab.com publishes. This server
// checks only that a query was given: GitLab compiles it and refuses what it
// cannot run with a message naming the line and column of the fault, which
// reaches the caller (toolutil's Workhorse query refusal). Rules written here
// would only go stale, which is what issue 1031 was.
//
// The query is text and nothing else, as GitLab declares it (`requires :query,
// type: String` in ee/lib/api/orbit/data.rb). gitlab-org/gitlab!261947, merged
// on 2026-10-27, removed the orbit_gql_queries flag and the JSON query DSL with
// it, and GitLab.com has since refused a query sent as a JSON object with 400
// {"error":"query is invalid"} before the Knowledge Graph sees it (measured
// 2026-10-29). This input took an object until then, forwarded so it would work
// if GitLab.com turned the flag back off (issue 1414); with the flag gone the
// object is an input GitLab refuses whatever it holds, so it is no longer
// taken. A JSON query written as text is still sent, since it is text, and
// [wrapQueryErr] answers GitLab's compile_error for it with an example of GQL.
//
// The statement, as the query_graph command describes it on GitLab.com
// (read 2026-10-29):
//
//	MATCH pattern [WHERE predicates]
//	[MATCH pattern [WHERE predicates] ...]
//	RETURN projections
//	[ORDER BY key [ASC | DESC]]
//	[SKIP rows] [LIMIT rows]
//
// Every node carries a label where it first appears, and at least one is
// bounded by its id or a literal property, inline or in WHERE; LIMIT is not a
// filter. A relationship type follows a colon, -[:IN_PROJECT]->, a repeat is
// bounded (*1..3), and no path is longer than three hops. WHERE combines
// predicates on one node's properties with AND and NOT. RETURN selects
// properties or whole nodes, and count, sum, avg, min and max aggregate, every
// other item becoming a group key; an aggregate is aliased and sorted by its
// alias, with one sort key. SKIP takes 0 to 10000 and LIMIT 1 to 1000, which
// replaced PAGE and AFTER (gitlab-org/orbit/knowledge-graph!2924, issue 1527).
// CALL db.schema() and CALL db.schema('Node') answer the ontology. The query
// type of the answer is inferred from the statement.
//
// Examples, each answered 200 by GitLab.com on 2026-10-29:
//
//	MATCH (p:Project {full_path: 'jmrp-oss/orbit-fixtures/kg-fixtures'}) RETURN p.id, p.full_path, p.name
//	MATCH (j:Job)-[:IN_PROJECT]->(p:Project) WHERE p.full_path STARTS WITH 'jmrp-oss/orbit-fixtures/' RETURN p, count(j) AS job_count ORDER BY job_count DESC
//	MATCH (p:Project {full_path: 'jmrp-oss/orbit-fixtures/kg-fixtures'})--(n) RETURN n
type QueryInput struct {
	Query string `json:"query" jsonschema:"Orbit query as GQL text, the read-only openCypher 9 based language GitLab.com compiles (orbit.dsl describes it). One statement: MATCH pattern [WHERE predicates] RETURN projections [ORDER BY key [ASC or DESC]] [SKIP n] [LIMIT n]. Label every node and anchor at least one by its id or a literal property, as in MATCH (p:Project {full_path: 'gitlab-org/gitlab'}) RETURN p.id, p.full_path LIMIT 1; LIMIT is not a filter. A relationship type follows a colon, -[:IN_PROJECT]->, and a repeat is bounded, *1..3 at most. WHERE takes AND and NOT, never OR. count, sum, avg, min and max aggregate, and every other RETURN item groups; alias an aggregate and sort by the alias: RETURN p, count(mr) AS mr_count ORDER BY mr_count DESC. SKIP takes 0 to 10000 and LIMIT 1 to 1000. orbit.schema lists the entities, properties and relationship types a query names. The JSON query DSL GitLab.com read before is gone. GitLab refuses a query it cannot compile with a message naming the fault.,required"`
	// ResponseFormat selects the answer's shape. POST /orbit/query declares
	// a third value beside raw and llm, which is why this input carries its
	// own field rather than [ResponseFormatInput] (`values: %w[raw llm gql]`
	// in ee/lib/api/orbit/data.rb):
	//   - "raw": the structured envelope, sent when the field is omitted
	//   - "llm": compact text optimized for LLM consumption
	//   - "gql": the result as a graph pattern table, text like llm
	//
	// "json" is read as "raw", for the reason [ResponseFormatInput] gives.
	ResponseFormat string `json:"response_format,omitempty" jsonschema:"Response format: raw (structured JSON, the default here), llm (compact text) or gql (the result as a graph pattern table). json is read as raw."`
}

// GraphStatusInput holds parameters for retrieving Orbit graph indexing status.
type GraphStatusInput struct {
	// NamespaceID targets a group by ID. Mutually exclusive with ProjectID
	// and FullPath; exactly one of the three must be set.
	NamespaceID int64 `json:"namespace_id,omitempty" jsonschema:"Group ID to inspect. Set exactly one of namespace_id, project_id, or full_path."`
	// ProjectID targets a project by ID. Mutually exclusive with
	// NamespaceID and FullPath; exactly one of the three must be set.
	ProjectID int64 `json:"project_id,omitempty" jsonschema:"Project ID to inspect. Set exactly one of namespace_id, project_id, or full_path."`
	// FullPath targets a group or project by full path
	// (e.g. "gitlab-org/gitlab"). Mutually exclusive with NamespaceID
	// and ProjectID; exactly one of the three must be set.
	FullPath string `json:"full_path,omitempty" jsonschema:"Full path of a group or project to inspect, for example gitlab-org/gitlab. Set exactly one scope field."`
	StructuredFormatInput
}

// StatusComponent describes the health of one component of the Orbit service
// (OrbitServiceComponent in GitLab's GraphQL API).
type StatusComponent struct {
	// Name is the component, for example "clickhouse-graph" or "nats".
	Name string `json:"name,omitempty"`
	// Status is the component's health state, GitLab's
	// OrbitServiceComponent.state: HEALTHY, DEGRADED, UNHEALTHY, MIGRATING
	// or UNKNOWN.
	Status string `json:"status,omitempty"`
}

// StatusOutput is the health of the Orbit service, as GitLab's GraphQL API
// answers Query.orbit.serviceStatus.
//
// It used to be GET /api/v4/orbit/status, which gitlab-org/gitlab!265633
// removed on 2026-10-27. The fields GraphQL no longer sends are gone from the
// output: user.available (GitLab answers serviceStatus null to a user Orbit is
// not available to, which [Status] reports as an error), the nested system
// object, timestamp, formatted_text, and each component's replicas and
// metrics. The status and component status are GraphQL's state, asked under
// the output's names in the document.
type StatusOutput struct {
	toolutil.HintableOutput
	// Status is the overall health state of the Orbit service: HEALTHY,
	// DEGRADED, UNHEALTHY, MIGRATING or UNKNOWN.
	Status string `json:"status,omitempty"`
	// Version is the Orbit service version.
	Version string `json:"version,omitempty"`
	// Components are the health states of the service's components.
	Components []StatusComponent `json:"components,omitempty"`
}

// SchemaDomain is one domain of the Orbit graph schema and the node kinds in
// it, grouped from each node's domain in the order GitLab lists the nodes.
type SchemaDomain struct {
	// Name is the domain identifier (e.g. "core", "source_code").
	Name string `json:"name,omitempty"`
	// NodeNames are the node kinds in the domain.
	NodeNames []string `json:"node_names,omitempty"`
}

// SchemaEdge describes an Orbit graph edge kind.
type SchemaEdge struct {
	// Name is the edge kind, which a query uses as the relationship type
	// (e.g. "AUTHORED", "IN_PROJECT").
	Name string `json:"name,omitempty"`
	// Description is the human-readable edge summary.
	Description string `json:"description,omitempty"`
	// Variants are the pairs of node kinds the edge connects, GitLab's
	// endpoints.
	Variants []SchemaEdgeVariant `json:"variants,omitempty"`
}

// SchemaEdgeVariant describes one pair of node kinds an Orbit edge connects.
type SchemaEdgeVariant struct {
	// SourceType is the node kind at the start of the edge (e.g. "User").
	SourceType string `json:"source_type,omitempty"`
	// TargetType is the node kind at the end of the edge (e.g. "Issue").
	TargetType string `json:"target_type,omitempty"`
}

// SchemaNodeDisplay is how GitLab draws nodes of a kind.
type SchemaNodeDisplay struct {
	// Color is the hex color of the node kind.
	Color string `json:"color,omitempty"`
	// Size is the relative size of the node kind in a graph.
	Size int64 `json:"size,omitempty"`
}

// SchemaProperty is one property of a node kind: the name a query filters
// or returns, its type, and the values an enum takes.
type SchemaProperty struct {
	// Name is the property name.
	Name string `json:"name"`
	// DataType is the property's data type, for example String or Int.
	DataType string `json:"data_type,omitempty"`
	// Nullable reports whether the property can be empty.
	Nullable bool `json:"nullable"`
	// EnumValues are the values an enum property takes, empty otherwise.
	EnumValues []string `json:"enum_values,omitempty"`
}

// SchemaNode is one node kind of the Orbit graph schema.
type SchemaNode struct {
	// Name is the node kind, which a query uses as the node label.
	Name string `json:"name"`
	// Description is the human-readable node summary.
	Description string `json:"description,omitempty"`
	// Domain is the domain the node kind belongs to.
	Domain string `json:"domain,omitempty"`
	// PrimaryKey is the property that identifies a node.
	PrimaryKey string `json:"primary_key,omitempty"`
	// LabelField is the property shown as a node's label, GitLab's
	// labelProperty.
	LabelField string `json:"label_field,omitempty"`
	// Display is how GitLab draws nodes of the kind.
	Display *SchemaNodeDisplay `json:"display,omitempty"`
	// Properties are the node kind's properties, present only for the
	// nodes named in expand.
	Properties []SchemaProperty `json:"properties,omitempty"`
}

// SchemaOutput is the Orbit graph schema: GitLab's GraphQL Query.orbit.schema
// for the structured answer, and the get_graph_schema command of the Orbit MCP
// endpoint for the llm text.
//
// It used to be GET /api/v4/orbit/schema, which gitlab-org/gitlab!265633
// removed on 2026-10-27. Gone with it: schema_version, each domain's
// description, and each node's incoming_edges and outgoing_edges, which the
// edges list carries pair by pair. Added: each node's primary key, label
// property and display hints, typed where the REST answer was passed on
// undecoded.
type SchemaOutput struct {
	toolutil.HintableOutput
	// FormattedText is the schema as compact text, which is the whole of
	// GitLab's answer when the caller selected the "llm" format.
	FormattedText string `json:"formatted_text,omitempty"`
	// Domains are the domains of the node kinds, each with its nodes.
	Domains []SchemaDomain `json:"domains,omitempty"`
	// Nodes are the node kinds.
	Nodes []SchemaNode `json:"nodes,omitempty"`
	// Edges are the edge kinds.
	Edges []SchemaEdge `json:"edges,omitempty"`
}

// ToolDefinition describes one tool GitLab's Orbit MCP endpoint serves.
type ToolDefinition struct {
	// Name is the tool name as Orbit exposes it.
	Name string `json:"name,omitempty"`
	// Description is the tool's human-readable description.
	Description string `json:"description,omitempty"`
	// Parameters is the decoded JSON Schema for the tool's input, which
	// MCP's tools/list answers as inputSchema.
	Parameters any `json:"parameters,omitempty"`
	// Trusted is the trust flag GitLab sends beside each tool, absent when
	// GitLab sends none.
	Trusted *bool `json:"trusted,omitempty"`
}

// ToolsOutput is the tool list of GitLab's Orbit MCP endpoint.
type ToolsOutput struct {
	toolutil.HintableOutput
	// Tools is the list of MCP tools Orbit exposes.
	Tools []ToolDefinition `json:"tools,omitempty"`
}

// QueryOutput is the result envelope returned by Orbit query execution.
type QueryOutput struct {
	toolutil.HintableOutput
	// FormattedText is the text body returned by the server when the
	// caller selected the "llm" response format, or the graph pattern
	// table of the "gql" one.
	FormattedText string `json:"formatted_text,omitempty"`
	// Result is the decoded result envelope returned for the
	// "raw" response format. Its shape depends on the
	// query_type: traversal returns a list of row objects,
	// aggregation returns aggregation rows, neighbors returns the
	// neighbor expansion, and path_finding returns the matching paths.
	Result any `json:"result,omitempty"`
	// QueryType echoes the query_type that produced this result
	// (traversal, aggregation, neighbors, or path_finding).
	QueryType string `json:"query_type,omitempty"`
	// RawQueryStrings are the SQL strings the server executed for
	// the request; useful for debugging slow or unexpected queries.
	RawQueryStrings []string `json:"raw_query_strings,omitempty"`
	// RowCount is the number of rows in the result envelope.
	RowCount int64 `json:"row_count,omitempty"`
}

// GraphStatusProjects describes the indexing progress of the projects in the
// group or project (OrbitIndexingProjects).
type GraphStatusProjects struct {
	// Indexed is the number of indexed projects.
	Indexed int64 `json:"indexed"`
	// TotalKnown is the number of projects known to Orbit.
	TotalKnown int64 `json:"total_known"`
	// Gaps is the number of projects that are not indexed, as GitLab's
	// gaps field counts them.
	Gaps int64 `json:"gaps"`
	// NotIndexed is the number of projects that are not indexed, as
	// GitLab's notIndexed field counts them.
	NotIndexed int64 `json:"not_indexed"`
	// Total is the number of projects known to Orbit, as GitLab's total
	// field counts them.
	Total int64 `json:"total"`
}

// GraphStatusDomainItem is the number of indexed items of one entity.
type GraphStatusDomainItem struct {
	// Name is the entity (e.g. "MergeRequest").
	Name string `json:"name,omitempty"`
	// Count is the number of indexed items of the entity.
	Count int64 `json:"count"`
}

// GraphStatusDomain is the number of indexed items in one domain.
type GraphStatusDomain struct {
	// Name is the domain (e.g. "code_review").
	Name string `json:"name,omitempty"`
	// Items are the counts of each entity in the domain.
	Items []GraphStatusDomainItem `json:"items,omitempty"`
}

// GraphStatusArea is the indexing state of one area of the graph.
type GraphStatusArea struct {
	// Name is the area (e.g. "source_code").
	Name string `json:"name,omitempty"`
	// State is the area's indexing state.
	State string `json:"state,omitempty"`
}

// GraphStatusIndexing describes the indexing state of the group or project.
type GraphStatusIndexing struct {
	// State is the indexing state: READY, INDEXING, NOT_STARTED, FAILED or
	// UNKNOWN.
	State string `json:"state,omitempty"`
	// LastCompletedAt is when Orbit last completed indexing the group or
	// project, empty until the first run completes.
	LastCompletedAt string `json:"last_completed_at,omitempty"`
	// Areas are the indexing states of each area.
	Areas []GraphStatusArea `json:"areas,omitempty"`
}

// GraphStatusOutput is the Orbit indexing status of one group or project, as
// GitLab's GraphQL API answers Group.orbit and Project.orbit.
//
// It used to be GET /api/v4/orbit/graph_status, which gitlab-org/gitlab!264745
// removed on 2026-10-28 (issue 1516). Gone with it: formatted_text (GraphQL
// has no llm rendering), each count's state, and the last run's start, duration
// and error. Added: whether Orbit is on for the group and since when, the
// state of each area, and two more project counts.
type GraphStatusOutput struct {
	toolutil.HintableOutput
	// Enabled reports whether Orbit is turned on for the group's top-level
	// group; absent for a project, for which GitLab sends null.
	Enabled *bool `json:"enabled,omitempty"`
	// EnabledAt is when Orbit was turned on for the top-level group.
	EnabledAt string `json:"enabled_at,omitempty"`
	// Projects is the indexing progress of the projects, GitLab's
	// indexing.projects.
	Projects *GraphStatusProjects `json:"projects,omitempty"`
	// Domains are the indexed item counts of each domain, GitLab's
	// itemCounts.domains.
	Domains []GraphStatusDomain `json:"domains,omitempty"`
	// Indexing is the indexing state.
	Indexing *GraphStatusIndexing `json:"indexing,omitempty"`
}

// Query executes a read-only Orbit Knowledge Graph query on GitLab.com.
//
// Endpoint: POST /api/v4/orbit/query, one of the Orbit routes GitLab.com still
// serves after the GA removals. The query is forwarded as the caller wrote it,
// once it is known not to be blank: GQL text reaches GitLab as the body's
// query string, {"query": "<GQL text>"}. See [QueryInput] for the language.
// GitLab judges everything else, and a query it refuses comes back as a 400
// whose message says what to change, with [wrapQueryErr]'s hint pointing at
// the two actions that describe the language and the graph.
//
// GitLab infers the query type from the statement and answers it beside the
// result: traversal, aggregation, neighbors or path_finding.
//
// When response_format is omitted, the handler sends "raw" (structured
// JSON) explicitly rather than leaving the choice to GitLab's default,
// which is "raw" today (ee/lib/api/orbit/data.rb) and was not always.
// Structured JSON is strictly more useful for an MCP tool: the LLM can
// iterate over result nodes directly, the SDK decodes row_count, and
// downstream markdown formatters pretty-print the envelope. Callers who
// want the compact TOON text pass response_format="llm", and the graph
// pattern table "gql"; both are text GitLab answers whole, which
// client-go's QueryRaw hands back as it came.
func Query(ctx context.Context, client *gitlabclient.Client, input QueryInput) (QueryOutput, error) {
	if err := ctx.Err(); err != nil {
		return QueryOutput{}, err
	}
	if strings.TrimSpace(input.Query) == "" {
		return QueryOutput{}, toolutil.ErrFieldRequired("query")
	}
	format, err := queryResponseFormat(input.ResponseFormat)
	if err != nil {
		return QueryOutput{}, err
	}
	request := &gl.OrbitQueryRequest{Query: encodeQuery(input.Query), ResponseFormat: format}
	if *format != gl.OrbitResponseFormatRaw {
		var text bytes.Buffer
		_, err = client.GL().Orbit.QueryRaw(request, &text, gl.WithContext(ctx))
		if err != nil {
			return QueryOutput{}, wrapQueryErr(err, input.Query)
		}
		return QueryOutput{FormattedText: text.String()}, nil
	}

	result, _, err := client.GL().Orbit.Query(request, gl.WithContext(ctx))
	if err != nil {
		return QueryOutput{}, wrapQueryErr(err, input.Query)
	}
	return convertQuery(result), nil
}

// queryFormatGQL is the response format POST /orbit/query takes and no other
// Orbit route does: the result as a graph pattern table, text written by
// writeLLMResultResponse in workhorse/internal/orbit/sendquery.go like the llm
// answer. client-go declares raw and llm only, and its format type is a
// string, so the value is spelled here.
const queryFormatGQL gl.OrbitResponseFormatValue = "gql"

// errQueryFormat is what [queryResponseFormat] answers for a format the query
// route does not take.
var errQueryFormat = errors.New("invalid response_format: use raw, llm or gql")

// queryResponseFormat normalizes the response format of a query: gql, which
// the query route alone takes, and otherwise what [responseFormat] accepts,
// raw when the caller chose none (see [Query]). It never answers nil.
func queryResponseFormat(format string) (*gl.OrbitResponseFormatValue, error) {
	if strings.EqualFold(strings.TrimSpace(format), string(queryFormatGQL)) {
		value := queryFormatGQL
		return &value, nil
	}
	value, hasFormat, err := responseFormat(format, "response_format")
	if err != nil {
		return nil, errQueryFormat
	}
	if !hasFormat {
		raw := gl.OrbitResponseFormatRaw
		return &raw, nil
	}
	return value, nil
}

// encodeQuery returns the query as the JSON string the body carries, which
// GitLab.com compiles as GQL. Everything about the text is GitLab's to judge,
// and GitLab's own refusal names the fault it finds: the rules this used to
// hold a query to were written for an older shape of the JSON DSL and refused
// the one neighbors query GitLab ran (issue 1031).
func encodeQuery(query string) json.RawMessage {
	// json.Marshal of a string cannot fail: invalid UTF-8 is replaced rather
	// than refused. There is therefore no error here to report and no branch
	// worth carrying for one.
	encoded, _ := json.Marshal(query) //nolint:errchkjson // see above
	return encoded
}

// orbitQueryOp is the operation a [Query] failure is reported under.
const orbitQueryOp = "orbit_query"

// queryRefusedHint follows a query GitLab refused with 400. The refusal
// carries GitLab's account of the fault when it compiled the query against
// the language or the ontology, so the hint names where the vocabulary of a
// correction comes from rather than guessing at the fault itself. It is one
// literal: a constant joined from two has an operator gremlins mutates and
// no coverage block to place it in, so it reads as a mutant no test reaches.
const queryRefusedHint = "correct the query where GitLab's message points. orbit.dsl serves the query language and orbit.schema the entities, properties and relationship types it accepts"

// queryObjectRefusedHint follows a query written as a JSON object that GitLab
// refused for its language rather than for what it asked.
const queryObjectRefusedHint = "GitLab.com reads a query as GQL text, not as a JSON object. Send a GQL statement, such as MATCH (p:Project {full_path: 'gitlab-org/gitlab'}) RETURN p.id, p.full_path LIMIT 1. orbit.dsl serves the language and orbit.schema the entities, properties and relationship types it accepts"

// queryObjectRefusal is the refusal GitLab answers a query written as a JSON
// object with, sent as text: the Knowledge Graph parses it as GQL and answers
// compile_error, "schema violation: Orbit query syntax at line 1, column 1:
// expected Statement" (measured 2026-10-17 and again 2026-10-29). compile_error
// alone would not do: a GQL query refused for what it asked carries that code
// too, and would be told to send GQL when it was GQL.
const queryObjectRefusal = "Orbit query syntax"

// wrapQueryErr classifies a failed [Query]. A 400 is a query GitLab refused
// to run: one written as a JSON object and refused for its language is
// answered by [queryObjectRefusedHint], and any other by [queryRefusedHint].
// Anything else is classified the way every Orbit failure is, by
// [wrapOrbitErr].
func wrapQueryErr(err error, query string) error {
	if !toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
		return wrapOrbitErr(orbitQueryOp, err)
	}
	if writtenAsObject(query) && toolutil.ContainsAny(err, queryObjectRefusal) {
		return toolutil.WrapErrWithHint(orbitQueryOp, err, queryObjectRefusedHint)
	}
	return toolutil.WrapErrWithHint(orbitQueryOp, err, queryRefusedHint)
}

// writtenAsObject reports whether a query is text that opens a JSON object,
// which a client that serializes a nested object sends in place of the
// object, and a caller still writing the JSON query DSL sends as its query. No
// GQL statement opens with a brace, since one starts with MATCH or CALL.
func writtenAsObject(query string) bool {
	return strings.HasPrefix(strings.TrimSpace(query), "{")
}

// schemaResponseFormat normalizes the SchemaInput format fields. The
// input accepts both Format and ResponseFormat; each is normalized on its
// own, so "json" and "raw" agree, and the two must then name the same format
// when both are set. Nil means neither was set.
func schemaResponseFormat(input SchemaInput) (*gl.OrbitResponseFormatValue, error) {
	format, _, err := responseFormat(input.Format, "format")
	if err != nil {
		return nil, err
	}
	alias, _, err := responseFormat(input.ResponseFormat, "response_format")
	if err != nil {
		return nil, err
	}
	if format == nil {
		return alias, nil
	}
	if alias != nil && *alias != *format {
		return nil, errors.New("format and response_format must match when both are set")
	}
	return format, nil
}

// responseFormat normalizes a user-supplied format string to the value
// GitLab accepts. Returns (nil, false, nil) when the input is empty.
//
// GitLab declares "raw" and "llm" and nothing else. "json" is read as "raw"
// rather than refused, because it was offered here as a value until GitLab's
// refusal of it was measured and a caller may still send it; anything else is
// refused here, before a request is made.
func responseFormat(format, field string) (*gl.OrbitResponseFormatValue, bool, error) {
	normalized := strings.ToLower(strings.TrimSpace(format))
	if normalized == "" {
		return nil, false, nil
	}
	switch normalized {
	case string(gl.OrbitResponseFormatRaw), string(gl.OrbitResponseFormatLLM):
		value := gl.OrbitResponseFormatValue(normalized)
		return &value, true, nil
	case jsonFormatSynonym:
		value := gl.OrbitResponseFormatRaw
		return &value, true, nil
	default:
		return nil, false, errors.New("invalid " + field + ": use raw or llm")
	}
}

// jsonFormatSynonym is the one response format this server accepts that
// GitLab does not, read as raw by [responseFormat].
const jsonFormatSynonym = "json"

// responseFormatName returns the string form of a format pointer, or
// "" when the pointer is nil.
func responseFormatName(format *gl.OrbitResponseFormatValue) string {
	if format == nil {
		return ""
	}
	return string(*format)
}

// orbitUnavailableHint follows an Orbit answer that is a refusal or that
// carries nothing: GitLab answers Orbit only on GitLab.com, and since its
// general availability only for a top-level group that holds Orbit access.
const orbitUnavailableHint = "Orbit is served on GitLab.com only, to a token that can read a top-level group with Orbit access and Orbit turned on; check the group's Orbit settings and access"

// wrapOrbitErr classifies an HTTP error from the Orbit API and
// attaches a domain-specific actionable hint. 404 and 403 get the
// availability hint, since GitLab answers both to a user or namespace Orbit
// does not serve; 400 hints at the query/format/scope parameters; 429 and
// 503 hint at backoff; everything else uses the generic wrap.
func wrapOrbitErr(op string, err error) error {
	if toolutil.IsHTTPStatus(err, http.StatusNotFound) || toolutil.IsHTTPStatus(err, http.StatusForbidden) {
		return toolutil.WrapErrWithHint(op, err, orbitUnavailableHint)
	}
	if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
		return toolutil.WrapErrWithHint(op, err,
			"check the Orbit query, response_format, and graph_status scope parameters")
	}
	if toolutil.IsHTTPStatus(err, http.StatusTooManyRequests) {
		return toolutil.WrapErrWithHint(op, err,
			"Orbit request was rate-limited; retry later with a smaller query or lower request volume")
	}
	if toolutil.IsHTTPStatus(err, http.StatusServiceUnavailable) {
		return toolutil.WrapErrWithHint(op, err,
			"Orbit service is temporarily unavailable; retry later")
	}
	return toolutil.WrapErr(op, err)
}

// convertQuery projects a GitLab SDK [gl.OrbitQueryResult] into the
// MCP-tool [QueryOutput], decoding the raw JSON result envelope so
// downstream formatters can pretty-print it.
func convertQuery(result *gl.OrbitQueryResult) QueryOutput {
	if result == nil {
		return QueryOutput{}
	}
	return QueryOutput{
		Result:          decodeRaw(result.Result),
		QueryType:       result.QueryType,
		RawQueryStrings: result.RawQueryStrings,
		RowCount:        result.RowCount,
	}
}

// decodeRaw parses a [json.RawMessage] into a generic value, falling
// back to the raw bytes (as a string) when the payload is not valid
// JSON. Returns nil for empty input so callers can treat absent
// payloads as zero values.
func decodeRaw(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return value
}
