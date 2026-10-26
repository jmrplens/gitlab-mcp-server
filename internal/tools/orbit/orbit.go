package orbit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ResponseFormatInput is the shared response_format selector used by
// the status, dsl and graph_status Orbit endpoints. The query endpoint takes a
// third value and declares its own field ([QueryInput.ResponseFormat]).
type ResponseFormatInput struct {
	// ResponseFormat selects the Orbit response shape. GitLab declares
	// two values on every route that takes it (ee/lib/api/orbit/data.rb,
	// `values: %w[raw llm]`):
	//   - "raw": structured JSON
	//   - "llm": compact text optimized for LLM consumption
	//
	// "json" is accepted here as a synonym of "raw" and sent as "raw":
	// GitLab answers 400 "response_format does not have a valid value"
	// to it on every route, and it was offered here as a value of its
	// own until that was measured.
	//
	// When empty the parameter is not sent and GitLab applies its own
	// default, which is "raw" on every route.
	ResponseFormat string `json:"response_format,omitempty" jsonschema:"Response format: raw or llm (json is read as raw). When omitted, the GitLab default (raw) applies."`
}

// StatusInput holds parameters for retrieving Orbit cluster status.
type StatusInput struct {
	ResponseFormatInput
}

// SchemaInput holds parameters for retrieving the Orbit graph schema.
type SchemaInput struct {
	// Expand lists node names whose full properties and relationships
	// should be hydrated in the response.
	Expand []string `json:"expand,omitempty" jsonschema:"Node names to expand with full properties and relationships."`
	// Format selects the schema response shape: "raw" or "llm", with
	// "json" read as "raw" for the reason [ResponseFormatInput] gives.
	// Empty leaves GitLab's default ("raw"). Whichever of the two
	// input names carries it, it reaches GitLab as response_format,
	// which is the only name the route declares.
	Format string `json:"format,omitempty" jsonschema:"Schema response format: raw or llm (json is read as raw). When omitted, the GitLab default (raw) applies."`
	// ResponseFormat is an alias for Format, under the name GitLab
	// declares for the parameter. Must match Format when both are set.
	ResponseFormat string `json:"response_format,omitempty" jsonschema:"Alias for format. Must match format when both are set."`
}

// ToolsInput is the input for listing the Orbit MCP tool manifest.
// The endpoint accepts no parameters; the type exists so the handler
// signature is uniform with the other Orbit handlers.
type ToolsInput struct{}

// DSLInput holds parameters for retrieving the Orbit query language.
type DSLInput struct {
	ResponseFormatInput
}

// DSLOutput is the query language GitLab.com compiles orbit.query in, as the
// Orbit command a graph query runs as describes it.
//
// GitLab.com reads every query as GQL since it turned orbit_gql_queries on for
// everyone at 01:06 UTC on 2026-10-15, and GET /api/v4/orbit/schema/dsl, which
// served the JSON DSL this type used to carry whole, answers 404 since then
// (issue 1414). What GitLab publishes for GQL instead is the query_graph
// command of GET /api/v4/orbit/agent/commands: a description of the grammar and
// the JSON Schema of the command's input. The other two GQL routes publish no
// grammar: GET /orbit/schema/format is the JSON Schema of a query's answer, and
// GET /orbit/query/templates lists named queries written in GQL.
type DSLOutput struct {
	toolutil.HintableOutput
	// ResponseFormat echoes the response_format that was asked for ("raw" for
	// the command as a JSON object, "llm" for it as compact text), or nothing
	// when GitLab chose its default, raw.
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
// 2026-10-15 (gitlab-org/gitlab!256465 has Rails send the language the flag
// selects, and the compiler refuses the other). [DSL] describes it from what
// GitLab.com publishes. This server checks only that a query was given and
// that it is text or a JSON object: GitLab compiles it and refuses what it
// cannot run with a message naming the line and column of the fault, which
// reaches the caller (toolutil's Workhorse query refusal). Rules written here
// would only go stale, which is what issue 1031 was.
//
// A query given as a JSON object is forwarded as written rather than refused
// here, so it keeps working if GitLab.com turns the flag back off before
// gitlab-org/gitlab!261947 removes the JSON DSL. While GitLab.com reads GQL it
// answers such a query with compile_error, and [wrapQueryErr] then says that
// GitLab.com reads queries as GQL, with an example (issue 1414).
//
// The statement, as the Orbit query frontend states it
// (docs/design-documents/querying/orbit_query_frontend.md in
// gitlab-org/orbit/knowledge-graph):
//
//	MATCH pattern [WHERE predicates]
//	[MATCH pattern [WHERE predicates] ...]
//	RETURN projections
//	[ORDER BY key [ASC | DESC]]
//	[LIMIT rows | PAGE rows [AFTER 'token']]
//
// Every node carries a label where it first appears, and at least one is
// bounded by its id or a literal property, inline or in WHERE. A relationship
// type follows a colon, -[:IN_PROJECT]->, a repeat is bounded (*1..3), and no
// path is longer than three hops. WHERE combines predicates with AND and NOT.
// RETURN selects properties or whole nodes, and count, sum, avg, min and max
// aggregate, every other item becoming a group key. A neighbors query has one
// labeled, bounded node and a far endpoint with a variable and nothing else,
// and a shortest path is written MATCH path = ANY SHORTEST (a)-[...]->(b)
// RETURN path. CALL db.schema() and CALL db.schema('Node') answer the ontology.
// The query type of the answer is inferred from the statement.
//
// Examples, each answered 200 by GitLab.com on 2026-10-26:
//
//	MATCH (p:Project {full_path: 'gitlab-mcp-server-orbit-fixtures/kg-fixtures'}) RETURN p.id, p.full_path, p.name
//	MATCH (mr:MergeRequest)-[:IN_PROJECT]->(p:Project) WHERE p.full_path STARTS WITH 'gitlab-mcp-server-orbit-fixtures/' RETURN p, count(mr) AS mr_count ORDER BY mr_count DESC
//	MATCH (p:Project {full_path: 'gitlab-mcp-server-orbit-fixtures/kg-fixtures'})--(n) RETURN n
//	MATCH path = ANY SHORTEST (mr:MergeRequest {source_branch: 'feature/restock-helper'})-[:IN_PROJECT*1..2]->(p:Project {full_path: 'gitlab-mcp-server-orbit-fixtures/kg-fixtures'}) RETURN path
type QueryInput struct {
	Query any `json:"query" jsonschema:"Orbit query as GQL text, the read-only openCypher 9 based language GitLab.com compiles (orbit.dsl describes it). One statement: MATCH pattern [WHERE predicates] RETURN projections [ORDER BY key [ASC or DESC]] [LIMIT n or PAGE n [AFTER cursor]]. Label every node and anchor at least one by its id or a literal property, as in MATCH (p:Project {full_path: 'gitlab-org/gitlab'}) RETURN p.id, p.full_path LIMIT 1. A relationship type follows a colon, -[:IN_PROJECT]->, and a repeat is bounded, *1..3 at most. WHERE takes AND and NOT, never OR. count, sum, avg, min and max aggregate, and every other RETURN item groups: RETURN p, count(mr) AS mr_count ORDER BY mr_count DESC. orbit.schema lists the entities, properties and relationship types a query names. A JSON object is forwarded as written, and GitLab.com refuses it while it reads queries as GQL. GitLab refuses a query it cannot compile with a message naming the fault.,required"`
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
	NamespaceID int64 `json:"namespace_id,omitempty" jsonschema:"Namespace/group ID to inspect. Set exactly one of namespace_id, project_id, or full_path."`
	// ProjectID targets a project by ID. Mutually exclusive with
	// NamespaceID and FullPath; exactly one of the three must be set.
	ProjectID int64 `json:"project_id,omitempty" jsonschema:"Project ID to inspect. Set exactly one of namespace_id, project_id, or full_path."`
	// FullPath targets a group or project by full path
	// (e.g. "gitlab-org/gitlab"). Mutually exclusive with NamespaceID
	// and ProjectID; exactly one of the three must be set.
	FullPath string `json:"full_path,omitempty" jsonschema:"Full path of a group or project to inspect, for example gitlab-org/gitlab. Set exactly one scope field."`
	ResponseFormatInput
}

// StatusReplicas describes ready and desired replica counts for an Orbit component.
type StatusReplicas struct {
	// Ready is the current number of healthy replicas.
	Ready int64 `json:"ready"`
	// Desired is the target number of replicas.
	Desired int64 `json:"desired"`
}

// StatusComponent describes an Orbit subsystem status entry.
type StatusComponent struct {
	// Name is the subsystem identifier (e.g. "clickhouse", "api").
	Name string `json:"name,omitempty"`
	// Status is the subsystem health label (e.g. "healthy").
	Status string `json:"status,omitempty"`
	// Replicas are the ready/desired counts, or nil when the
	// subsystem is stateless.
	Replicas *StatusReplicas `json:"replicas,omitempty"`
	// Metrics is the decoded raw JSON metrics payload for the subsystem.
	Metrics any `json:"metrics,omitempty"`
}

// StatusUser mirrors the user-level access object returned in the new
// nested shape of the Orbit status response. It is present only when
// the server emits the nested shape; nil for the old flat shape.
type StatusUser struct {
	// Available reports whether the calling user has access to the
	// Knowledge Graph.
	Available bool `json:"available"`
}

// StatusSystem mirrors the cluster health object returned in the new
// nested shape of the Orbit status response. It carries the same
// health fields as the flat shape, plus an Error field emitted when
// the backend cannot reach the gRPC cluster (Status is "unknown" in
// that case). Present only for the nested shape; nil otherwise.
type StatusSystem struct {
	// FormattedText is the pre-formatted text body, when present.
	FormattedText string `json:"formatted_text,omitempty"`
	// Status is the cluster-level health label.
	Status string `json:"status,omitempty"`
	// Timestamp is the server-side snapshot time, RFC3339.
	Timestamp string `json:"timestamp,omitempty"`
	// Version is the Orbit service version.
	Version string `json:"version,omitempty"`
	// Error is the backend error message emitted when the gRPC
	// cluster is unreachable, or empty otherwise.
	Error string `json:"error,omitempty"`
	// Components are the per-subsystem status entries.
	Components []StatusComponent `json:"components,omitempty"`
}

// StatusOutput is the Orbit cluster health response.
//
// The flat fields (FormattedText, Status, Timestamp, Version,
// Components) are always populated: the SDK promotes the nested
// system object's fields into them for backward compatibility. User
// and System carry the new nested shape directly when the server
// emits it, and are nil for the old flat shape.
type StatusOutput struct {
	toolutil.HintableOutput
	// User mirrors the user-level access object from the nested
	// response shape, or nil for the flat shape.
	User *StatusUser `json:"user,omitempty"`
	// System mirrors the cluster health object from the nested
	// response shape, or nil for the flat shape.
	System *StatusSystem `json:"system,omitempty"`
	// FormattedText is the pre-formatted text body returned by the
	// server when the caller selected the "llm" response format.
	FormattedText string `json:"formatted_text,omitempty"`
	// Status is the cluster-level health label.
	Status string `json:"status,omitempty"`
	// Timestamp is the server-side snapshot time, RFC3339.
	Timestamp string `json:"timestamp,omitempty"`
	// Version is the Orbit service version.
	Version string `json:"version,omitempty"`
	// Components are the per-subsystem status entries.
	Components []StatusComponent `json:"components,omitempty"`
}

// SchemaDomain describes a logical grouping of Orbit graph node types.
type SchemaDomain struct {
	// Name is the domain identifier (e.g. "core", "sdlc").
	Name string `json:"name,omitempty"`
	// Description is the human-readable domain summary.
	Description string `json:"description,omitempty"`
	// NodeNames are the ontology node types in the domain.
	NodeNames []string `json:"node_names,omitempty"`
}

// SchemaEdge describes an Orbit graph edge type.
type SchemaEdge struct {
	// Name is the edge type identifier (e.g. "AUTHORED", "IN_PROJECT").
	Name string `json:"name,omitempty"`
	// Description is the human-readable edge summary.
	Description string `json:"description,omitempty"`
	// Variants are the valid source/target node-type pairs.
	Variants []SchemaEdgeVariant `json:"variants,omitempty"`
}

// SchemaEdgeVariant describes a valid source/target pair for an Orbit edge.
type SchemaEdgeVariant struct {
	// SourceType is the source node type (e.g. "User").
	SourceType string `json:"source_type,omitempty"`
	// TargetType is the target node type (e.g. "Issue").
	TargetType string `json:"target_type,omitempty"`
}

// SchemaOutput is the Orbit graph ontology response.
type SchemaOutput struct {
	toolutil.HintableOutput
	// FormattedText is the ontology as compact text, which is the whole
	// of GitLab's answer when the caller selected the "llm" response
	// format. client-go's OrbitSchema does not model the key, so it is
	// read from the captured response.
	FormattedText string `json:"formatted_text,omitempty"`
	// SchemaVersion is the Orbit ontology version string.
	SchemaVersion string `json:"schema_version,omitempty"`
	// Domains are the logical groupings of node types.
	Domains []SchemaDomain `json:"domains,omitempty"`
	// Nodes are the decoded node-type definitions. The Orbit API
	// evolves the node shape, so the SDK exposes the list as raw JSON
	// values rather than fixed Go types.
	Nodes []any `json:"nodes,omitempty"`
	// Edges are the graph edge types.
	Edges []SchemaEdge `json:"edges,omitempty"`
}

// ToolDefinition describes one MCP tool manifest entry served by Orbit.
type ToolDefinition struct {
	// Name is the tool name as Orbit exposes it.
	Name string `json:"name,omitempty"`
	// Description is the tool's human-readable description.
	Description string `json:"description,omitempty"`
	// Parameters is the decoded JSON Schema for the tool's input.
	Parameters any `json:"parameters,omitempty"`
}

// ToolsOutput is the Orbit MCP tool manifest response.
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

// GraphStatusProjects describes indexed and known project counts.
type GraphStatusProjects struct {
	// Indexed is the number of projects whose Knowledge Graph is
	// up to date in the index.
	Indexed int64 `json:"indexed"`
	// TotalKnown is the total number of projects eligible for indexing.
	TotalKnown int64 `json:"total_known"`
	// Gaps is the number of projects the indexer gave up on: every attempt
	// it is allowed was used and none produced an index (ProjectsStatus.gaps
	// in crates/orbit-server/proto/orbit.proto of
	// gitlab-org/orbit/knowledge-graph). client-go's OrbitGraphStatusProjects
	// does not model it, so [GraphStatus] reads it from the captured response
	// (register row 93 of docs/development/upstream-bugs.md).
	Gaps int64 `json:"gaps"`
}

// GraphStatusDomainItem describes a count for one Orbit graph node type.
type GraphStatusDomainItem struct {
	// Name is the node-type identifier (e.g. "MergeRequest").
	Name string `json:"name,omitempty"`
	// Count is the number of indexed nodes of this type.
	Count int64 `json:"count"`
}

// GraphStatusDomain describes indexing counts for a graph domain.
type GraphStatusDomain struct {
	// Name is the domain identifier (e.g. "SDLC").
	Name string `json:"name,omitempty"`
	// Items are the per-node-type counts in this domain.
	Items []GraphStatusDomainItem `json:"items,omitempty"`
}

// GraphStatusIndexing describes the latest indexing pipeline state.
type GraphStatusIndexing struct {
	// State is the indexing pipeline state (e.g. "indexed", "running", "error").
	State string `json:"state,omitempty"`
	// LastStartedAt is the UTC RFC3339 timestamp of the last indexing run start.
	LastStartedAt string `json:"last_started_at,omitempty"`
	// LastCompletedAt is the UTC RFC3339 timestamp of the last successful indexing run.
	LastCompletedAt string `json:"last_completed_at,omitempty"`
	// LastDurationMs is the duration of the last indexing run in milliseconds.
	LastDurationMs int64 `json:"last_duration_ms,omitempty"`
	// LastError is the last indexing error message, or empty when the
	// last run completed successfully.
	LastError string `json:"last_error,omitempty"`
}

// GraphStatusOutput is the Orbit graph indexing status response.
type GraphStatusOutput struct {
	toolutil.HintableOutput
	// FormattedText is the pre-formatted text body returned by the
	// server when the caller selected the "llm" response format.
	FormattedText string `json:"formatted_text,omitempty"`
	// Projects summarizes the indexed vs. known project counts.
	Projects *GraphStatusProjects `json:"projects,omitempty"`
	// Domains are the per-domain indexing counts.
	Domains []GraphStatusDomain `json:"domains,omitempty"`
	// Indexing is the latest indexing pipeline state.
	Indexing *GraphStatusIndexing `json:"indexing,omitempty"`
}

// Status retrieves the Orbit cluster health snapshot from GitLab.com.
//
// Endpoint: GET /api/v4/orbit/status. Returns subsystem components,
// replica counts, version, and timestamp. Falls back to an informative
// not-found result when the Orbit feature is not enabled.
func Status(ctx context.Context, client *gitlabclient.Client, input StatusInput) (StatusOutput, error) {
	if err := ctx.Err(); err != nil {
		return StatusOutput{}, err
	}
	format, hasFormat, err := responseFormat(input.ResponseFormat, "response_format")
	if err != nil {
		return StatusOutput{}, err
	}
	opts := &gl.GetOrbitStatusOptions{}
	if hasFormat {
		opts.ResponseFormat = format
	}

	status, _, err := client.GL().Orbit.GetStatus(opts, gl.WithContext(ctx))
	if err != nil {
		return StatusOutput{}, wrapOrbitErr("orbit_status", err)
	}
	return convertStatus(status), nil
}

// Schema retrieves the Orbit graph ontology (domains, node types, edge
// types) from GitLab.com.
//
// Endpoint: GET /api/v4/orbit/schema. The expand parameter hydrates the
// named node types with full properties; the response format selects
// the structured ontology (raw) or the compact text one (llm).
//
// The format is sent as response_format through [responseFormatQuery]
// rather than through client-go's GetOrbitSchemaOptions.Format, which
// encodes it as `format`. Grape reserves that name for the
// representation it renders, so GitLab answered 406 "The requested
// format 'llm' is not supported." to either value and the response
// format never reached the route. The option struct is left without a
// format for that reason, and the compact text GitLab then answers
// with is read from the captured response, since OrbitSchema models no
// formatted_text.
func Schema(ctx context.Context, client *gitlabclient.Client, input SchemaInput) (SchemaOutput, error) {
	if err := ctx.Err(); err != nil {
		return SchemaOutput{}, err
	}
	format, err := schemaResponseFormat(input)
	if err != nil {
		return SchemaOutput{}, err
	}
	// An empty expand is left to the encoder, which writes no parameter for
	// an empty slice behind the pointer; testing its length here would be a
	// condition with no observable outcome.
	expand := input.Expand
	opts := &gl.GetOrbitSchemaOptions{Expand: &expand}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)

	schema, _, err := client.GL().Orbit.GetSchema(opts, gl.WithContext(ctx), responseFormatQuery(format))
	if err != nil {
		return SchemaOutput{}, wrapOrbitErr("orbit_schema", err)
	}
	var text struct {
		FormattedText string `json:"formatted_text"`
	}
	if err = captured.Decode(&text); err != nil {
		return SchemaOutput{}, toolutil.WrapErr("orbit_schema", err)
	}
	out := convertSchema(schema)
	out.FormattedText = text.FormattedText
	return out, nil
}

// responseFormatQuery sets response_format on the request's query string,
// replacing any value already there, or leaves the request as it is when
// no format was chosen. It is how [Schema] sends the one parameter
// client-go spells under a name GitLab does not read.
func responseFormatQuery(format *gl.OrbitResponseFormatValue) gl.RequestOptionFunc {
	return func(req *retryablehttp.Request) error {
		if format == nil {
			return nil
		}
		query := req.URL.Query()
		query.Set("response_format", string(*format))
		req.URL.RawQuery = query.Encode()
		return nil
	}
}

// Tools retrieves the Orbit MCP tool manifest and parameter schemas.
//
// Endpoint: GET /api/v4/orbit/tools. The manifest lists the tools Orbit
// exposes alongside the JSON Schema of each tool's parameters; use it
// to learn the canonical query shapes before calling Query.
func Tools(ctx context.Context, client *gitlabclient.Client, _ ToolsInput) (ToolsOutput, error) {
	if err := ctx.Err(); err != nil {
		return ToolsOutput{}, err
	}

	tools, _, err := client.GL().Orbit.GetTools(gl.WithContext(ctx))
	if err != nil {
		return ToolsOutput{}, wrapOrbitErr("orbit_tools", err)
	}
	return convertTools(tools), nil
}

// queryCommand is the Orbit command a graph query runs as, the one whose
// description is the grammar of the query language.
const queryCommand = "query_graph"

// agentCommandsPath is the route that describes Orbit's commands, below
// /api/v4. client-go has no method for it.
const agentCommandsPath = "orbit/agent/commands"

// orbitDSLOp is the operation a [DSL] failure is reported under.
const orbitDSLOp = "orbit_dsl"

// agentCommandsOptions are the parameters of GET /api/v4/orbit/agent/commands
// (ee/lib/api/orbit/data.rb): the comma-separated commands to describe, and
// the response format, which GitLab defaults to raw when it is left out.
type agentCommandsOptions struct {
	CommandNames   string                       `url:"command_names"`
	ResponseFormat *gl.OrbitResponseFormatValue `url:"response_format,omitempty"`
}

// DSL retrieves the query language GitLab.com compiles orbit.query in, as the
// query_graph command describes it.
//
// Endpoint: GET /api/v4/orbit/agent/commands?command_names=query_graph. With
// response_format="raw" (GitLab's default) the body is a list of commands, and
// the query_graph entry gives the grammar's description and the JSON Schema of
// the command's input; with response_format="llm" it is
// {"formatted_text": ...}, the same command as compact text. Under GQL, which
// GitLab.com reads every query in since 2026-10-15, the description is the GQL
// grammar, and the route this action used to read, GET /orbit/schema/dsl,
// answers 404, so nothing here asks it (issue 1414). The Knowledge Graph
// describes only the language it reads for the caller, so the description is
// of the JSON DSL again for a caller GitLab serves that language to.
func DSL(ctx context.Context, client *gitlabclient.Client, input DSLInput) (DSLOutput, error) {
	if err := ctx.Err(); err != nil {
		return DSLOutput{}, err
	}
	format, _, err := responseFormat(input.ResponseFormat, "response_format")
	if err != nil {
		return DSLOutput{}, err
	}
	body, err := getAgentCommands(ctx, client, agentCommandsPath, agentCommandsOptions{CommandNames: queryCommand, ResponseFormat: format})
	if err != nil {
		return DSLOutput{}, wrapOrbitErr(orbitDSLOp, err)
	}
	out, err := decodeQueryCommand(body)
	if err != nil {
		return DSLOutput{}, toolutil.WrapErr(orbitDSLOp, err)
	}
	out.ResponseFormat = responseFormatName(format)
	return out, nil
}

// getAgentCommands sends the GET that describes Orbit's commands and returns
// the body as it came. The path is a parameter so a test reaches the request
// that cannot be built, which no constant path produces.
func getAgentCommands(ctx context.Context, client *gitlabclient.Client, path string, opts agentCommandsOptions) ([]byte, error) {
	req, err := client.GL().NewRequest(http.MethodGet, path, opts, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	_, err = client.GL().Do(req, &body)
	return body.Bytes(), err
}

// agentCommand is one command of the raw answer of
// GET /api/v4/orbit/agent/commands.
type agentCommand struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// decodeQueryCommand reads the query_graph command out of the answer, by its
// shape rather than by the format asked for: an object is the llm answer,
// {"formatted_text": ...}, and anything else is the raw list of commands. A
// list that holds no query_graph command answers nothing, which the card says
// rather than describing another command as the query language.
func decodeQueryCommand(body []byte) (DSLOutput, error) {
	trimmed := bytes.TrimSpace(body)
	if bytes.HasPrefix(trimmed, []byte("{")) {
		var text struct {
			FormattedText string `json:"formatted_text"`
		}
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return DSLOutput{}, fmt.Errorf("decode the query command as text: %w", err)
		}
		return DSLOutput{FormattedText: text.FormattedText}, nil
	}
	var commands []agentCommand
	if err := json.Unmarshal(trimmed, &commands); err != nil {
		return DSLOutput{}, fmt.Errorf("decode the query command: %w", err)
	}
	for _, command := range commands {
		if command.Name == queryCommand {
			return DSLOutput{Name: command.Name, Description: command.Description, Parameters: decodeRaw(command.Parameters)}, nil
		}
	}
	return DSLOutput{}, nil
}

// Query executes a read-only Orbit Knowledge Graph query on GitLab.com.
//
// Endpoint: POST /api/v4/orbit/query. The query is forwarded as the caller
// wrote it, once [encodeQuery] has checked there is one and that it is text or
// an object: GQL text reaches GitLab as the body's query string,
// {"query": "<GQL text>"}, and an object as that object. See [QueryInput] for
// the language. GitLab judges everything else, and a query it refuses comes
// back as a 400 whose message says what to change, with [wrapQueryErr]'s hint
// pointing at the two actions that describe the language and the graph.
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
	query, err := encodeQuery(input.Query)
	if err != nil {
		return QueryOutput{}, err
	}
	format, err := queryResponseFormat(input.ResponseFormat)
	if err != nil {
		return QueryOutput{}, err
	}
	request := &gl.OrbitQueryRequest{Query: query, ResponseFormat: format}
	if *format != gl.OrbitResponseFormatRaw {
		var text bytes.Buffer
		_, err = client.GL().Orbit.QueryRaw(request, &text, gl.WithContext(ctx))
		if err != nil {
			return QueryOutput{}, wrapQueryErr(err, input.Query)
		}
		return QueryOutput{FormattedText: text.String(), QueryType: queryType(input.Query)}, nil
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

// errQueryShape is what [encodeQuery] answers for a query that is neither
// text nor an object, which no language GitLab reads is written as.
var errQueryShape = errors.New("query must be GQL text or a JSON object")

// encodeQuery checks the things about a query GitLab cannot answer for
// itself, that one was given and that it is GQL text or a JSON object that
// encodes, and returns it encoded: text as a JSON string, which is the body
// GitLab.com compiles as GQL, and an object as itself. Everything else about it
// is GitLab's to judge, and GitLab's own refusal names the fault it finds: the
// rules this used to hold a query to were written for an older shape of the
// JSON DSL and refused the one neighbors query GitLab ran (issue 1031). An
// object is not refused here although GitLab.com reads queries as GQL, so a
// caller still gets the JSON DSL if GitLab.com turns GQL back off (issue 1414).
func encodeQuery(query any) (json.RawMessage, error) {
	switch typed := query.(type) {
	case nil:
		return nil, toolutil.ErrFieldRequired("query")
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, toolutil.ErrFieldRequired("query")
		}
	case map[string]any:
		// An object is encoded below as it is, like the text.
	default:
		return nil, fmt.Errorf("%w, not %T", errQueryShape, query)
	}
	// Text always encodes; an object built in Go rather than decoded from
	// JSON may hold a value that does not, which is refused here since
	// nothing could then be sent.
	buf, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errQueryShape, err)
	}
	return json.RawMessage(buf), nil
}

// queryType returns the query_type field of an Orbit query written as a JSON
// object, or "" for GQL text, a missing field or one of another type. It
// labels the llm answer, which carries no query type of its own; GitLab
// infers the type of a GQL query from the statement and says it in the raw
// answer.
func queryType(query any) string {
	object, _ := query.(map[string]any)
	queryTypeValue, _ := object["query_type"].(string)
	return queryTypeValue
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

// queryObjectRefusedHint follows a query sent as a JSON object that GitLab
// refused for its language rather than for what it asked.
const queryObjectRefusedHint = "GitLab.com reads a query as GQL text, not as a JSON object. Send the query as a string, such as MATCH (p:Project {full_path: 'gitlab-org/gitlab'}) RETURN p.id, p.full_path LIMIT 1. orbit.dsl serves the language and orbit.schema the entities, properties and relationship types it accepts"

// queryObjectRefusals are the two refusals GitLab answers a query written as
// a JSON object with when it reads no JSON. While GitLab.com reads queries as
// GQL the Knowledge Graph parses the object's text as GQL and answers
// compile_error, "schema violation: Orbit query syntax at line 1, column 1:
// expected Statement" (measured 2026-10-17); once gitlab-org/gitlab!261947
// removes the JSON DSL, Grape refuses the object as "query is invalid" before
// the Knowledge Graph sees it. compile_error alone would not do: a JSON DSL
// query GitLab compiled and refused for what it asked carries that code too,
// and would be told to send GQL to an instance that reads JSON.
var queryObjectRefusals = []string{"Orbit query syntax", "query is invalid"}

// wrapQueryErr classifies a failed [Query]. A 400 is a query GitLab refused
// to run: one written as a JSON object and refused for its language is
// answered by [queryObjectRefusedHint], and any other by [queryRefusedHint].
// Anything else is classified the way every Orbit failure is, by
// [wrapOrbitErr].
func wrapQueryErr(err error, query any) error {
	if !toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
		return wrapOrbitErr(orbitQueryOp, err)
	}
	if writtenAsObject(query) && toolutil.ContainsAny(err, queryObjectRefusals...) {
		return toolutil.WrapErrWithHint(orbitQueryOp, err, queryObjectRefusedHint)
	}
	return toolutil.WrapErrWithHint(orbitQueryOp, err, queryRefusedHint)
}

// writtenAsObject reports whether a query was written as a JSON object: an
// object, or text that opens one, which a client that serializes nested
// objects sends in place of the object and GitLab.com refuses the same way
// (measured 2026-10-17). No GQL statement opens with a brace, since one
// starts with MATCH or CALL.
func writtenAsObject(query any) bool {
	if text, isText := query.(string); isText {
		return strings.HasPrefix(strings.TrimSpace(text), "{")
	}
	_, isObject := query.(map[string]any)
	return isObject
}

// GraphStatus retrieves Orbit graph indexing status for a single
// namespace, project, or full path on GitLab.com.
//
// Endpoint: GET /api/v4/orbit/graph_status. Exactly one of namespace_id,
// project_id, or full_path must be set. Useful for verifying that the
// indexer has caught up before running Query.
//
// The count of projects the indexer gave up on, projects.gaps, is read from
// the captured response ([gitlabclient.WithResponseCapture]), since client-go
// does not model it; see [GraphStatusProjects.Gaps].
func GraphStatus(ctx context.Context, client *gitlabclient.Client, input GraphStatusInput) (GraphStatusOutput, error) {
	if err := ctx.Err(); err != nil {
		return GraphStatusOutput{}, err
	}
	opts, err := graphStatusOptions(input)
	if err != nil {
		return GraphStatusOutput{}, err
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)

	status, _, err := client.GL().Orbit.GetGraphStatus(opts, gl.WithContext(ctx))
	if err != nil {
		return GraphStatusOutput{}, wrapOrbitErr(orbitGraphStatusOp, err)
	}
	var unmodeled struct {
		Projects *struct {
			Gaps int64 `json:"gaps"`
		} `json:"projects"`
	}
	if err = captured.Decode(&unmodeled); err != nil {
		return GraphStatusOutput{}, toolutil.WrapErr(orbitGraphStatusOp, err)
	}
	out := convertGraphStatus(status)
	// The llm answer carries formatted_text alone, and the structured one
	// carries projects whenever client-go decoded them, so the two are nil
	// together and only the second needs asking.
	if unmodeled.Projects != nil {
		out.Projects.Gaps = unmodeled.Projects.Gaps
	}
	return out, nil
}

// orbitGraphStatusOp is the operation a [GraphStatus] failure is reported
// under.
const orbitGraphStatusOp = "orbit_graph_status"

// schemaResponseFormat normalizes the SchemaInput format fields. The
// input accepts both Format and ResponseFormat, the name GitLab declares;
// each is normalized on its own, so "json" and "raw" agree, and the two
// must then name the same format when both are set. Nil means neither
// was set.
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
// GitLab accepts. Returns (nil, false, nil) when the input is empty, so
// the parameter is not sent and GitLab applies its default ("raw" on
// every Orbit route).
//
// GitLab declares "raw" and "llm" and nothing else, and refuses any
// other value with 400. "json" is read as "raw" rather than refused,
// because it was offered here as a value until that refusal was
// measured and a caller may still send it; anything else is refused
// here, before a request is made.
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
// "" when the pointer is nil or the value is empty. The empty result
// signals "use the API server-side default" to callers.
func responseFormatName(format *gl.OrbitResponseFormatValue) string {
	if format == nil {
		return "" // empty = server-side default
	}
	return string(*format)
}

// graphStatusOptions validates that exactly one of namespace_id,
// project_id, or full_path is set and translates the chosen scope
// plus the optional response_format into the GitLab SDK options.
func graphStatusOptions(input GraphStatusInput) (*gl.GetGraphStatusOptions, error) {
	if input.NamespaceID < 0 {
		return nil, errors.New("namespace_id must not be negative")
	}
	if input.ProjectID < 0 {
		return nil, errors.New("project_id must not be negative")
	}
	format, hasFormat, err := responseFormat(input.ResponseFormat, "response_format")
	if err != nil {
		return nil, err
	}

	scopeCount := 0
	if input.NamespaceID > 0 {
		scopeCount++
	}
	if input.ProjectID > 0 {
		scopeCount++
	}
	fullPath := strings.TrimSpace(input.FullPath)
	if fullPath != "" {
		scopeCount++
	}
	if scopeCount != 1 {
		return nil, errors.New("set exactly one of namespace_id, project_id, or full_path")
	}

	opts := &gl.GetGraphStatusOptions{}
	if hasFormat {
		opts.ResponseFormat = format
	}
	if input.NamespaceID > 0 {
		opts.NamespaceID = &input.NamespaceID
	}
	if input.ProjectID > 0 {
		opts.ProjectID = &input.ProjectID
	}
	if fullPath != "" {
		opts.FullPath = &fullPath
	}
	return opts, nil
}

// wrapOrbitErr classifies an HTTP error from the Orbit API and
// attaches a domain-specific actionable hint. 404 becomes the
// "Orbit is experimental" hint so callers know to check feature
// gating; 403 hints at Knowledge Graph namespace access; 400 hints
// at the query/format/scope parameters; 429 and 503 hint at
// backoff; everything else uses the generic wrap.
func wrapOrbitErr(op string, err error) error {
	if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return toolutil.WrapErrWithHint(op, err,
			"Orbit is experimental and currently available only on GitLab.com with the Enterprise/Premium catalog and the knowledge_graph feature flag enabled")
	}
	if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
		return toolutil.WrapErrWithHint(op, err,
			"verify your token can access a namespace or project with GitLab Knowledge Graph enabled")
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

// convertStatus projects a GitLab SDK [gl.OrbitStatus] into the
// MCP-tool [StatusOutput]. Nil component entries are dropped; raw
// JSON metrics are decoded for friendlier Markdown rendering.
func convertStatus(status *gl.OrbitStatus) StatusOutput {
	if status == nil {
		return StatusOutput{}
	}
	out := StatusOutput{
		FormattedText: status.FormattedText,
		Status:        status.Status,
		Timestamp:     status.Timestamp,
		Version:       status.Version,
		Components:    convertStatusComponents(status.Components),
	}
	if status.User != nil {
		out.User = &StatusUser{Available: status.User.Available}
	}
	if status.System != nil {
		out.System = &StatusSystem{
			FormattedText: status.System.FormattedText,
			Status:        status.System.Status,
			Timestamp:     status.System.Timestamp,
			Version:       status.System.Version,
			Error:         status.System.Error,
			Components:    convertStatusComponents(status.System.Components),
		}
	}
	return out
}

// convertStatusComponents projects a slice of SDK status components
// into the MCP-tool [StatusComponent] mirror, dropping nil entries
// and decoding raw JSON metrics for friendlier Markdown rendering.
func convertStatusComponents(in []*gl.OrbitStatusComponent) []StatusComponent {
	components := make([]StatusComponent, 0, len(in))
	for _, component := range in {
		if component == nil {
			continue
		}
		var replicas *StatusReplicas
		if component.Replicas != nil {
			replicas = &StatusReplicas{Ready: component.Replicas.Ready, Desired: component.Replicas.Desired}
		}
		components = append(components, StatusComponent{
			Name:     component.Name,
			Status:   component.Status,
			Replicas: replicas,
			Metrics:  decodeRaw(component.Metrics),
		})
	}
	return components
}

// convertSchema projects a GitLab SDK [gl.OrbitSchema] into the
// MCP-tool [SchemaOutput]. Raw JSON node definitions are decoded so
// downstream Markdown formatters can format them; nil entries in
// domains, nodes, and edges are skipped.
func convertSchema(schema *gl.OrbitSchema) SchemaOutput {
	if schema == nil {
		return SchemaOutput{}
	}
	domains := make([]SchemaDomain, 0, len(schema.Domains))
	for _, domain := range schema.Domains {
		if domain == nil {
			continue
		}
		domains = append(domains, SchemaDomain{
			Name:        domain.Name,
			Description: domain.Description,
			NodeNames:   domain.NodeNames,
		})
	}
	edges := make([]SchemaEdge, 0, len(schema.Edges))
	for _, edge := range schema.Edges {
		if edge == nil {
			continue
		}
		variants := make([]SchemaEdgeVariant, 0, len(edge.Variants))
		for _, variant := range edge.Variants {
			if variant == nil {
				continue
			}
			variants = append(variants, SchemaEdgeVariant{SourceType: variant.SourceType, TargetType: variant.TargetType})
		}
		edges = append(edges, SchemaEdge{Name: edge.Name, Description: edge.Description, Variants: variants})
	}
	nodes := make([]any, 0, len(schema.Nodes))
	for _, node := range schema.Nodes {
		if decoded := decodeRaw(node); decoded != nil {
			nodes = append(nodes, decoded)
		}
	}
	return SchemaOutput{
		SchemaVersion: schema.SchemaVersion,
		Domains:       domains,
		Nodes:         nodes,
		Edges:         edges,
	}
}

// convertTools projects a GitLab SDK [gl.OrbitTools] into the
// MCP-tool [ToolsOutput], decoding raw JSON parameter schemas for
// downstream rendering and dropping nil entries.
func convertTools(tools *gl.OrbitTools) ToolsOutput {
	if tools == nil {
		return ToolsOutput{}
	}
	items := make([]ToolDefinition, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		if tool == nil {
			continue
		}
		items = append(items, ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  decodeRaw(tool.Parameters),
		})
	}
	return ToolsOutput{Tools: items}
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

// convertGraphStatus projects a GitLab SDK [gl.OrbitGraphStatus] into
// the MCP-tool [GraphStatusOutput], normalizing pointer-typed
// timestamps to UTC RFC3339 strings and dropping nil nested entries.
func convertGraphStatus(status *gl.OrbitGraphStatus) GraphStatusOutput {
	if status == nil {
		return GraphStatusOutput{}
	}
	var projects *GraphStatusProjects
	if status.Projects != nil {
		projects = &GraphStatusProjects{Indexed: status.Projects.Indexed, TotalKnown: status.Projects.TotalKnown}
	}
	domains := make([]GraphStatusDomain, 0, len(status.Domains))
	for _, domain := range status.Domains {
		if domain == nil {
			continue
		}
		items := make([]GraphStatusDomainItem, 0, len(domain.Items))
		for _, item := range domain.Items {
			if item == nil {
				continue
			}
			items = append(items, GraphStatusDomainItem{Name: item.Name, Count: item.Count})
		}
		domains = append(domains, GraphStatusDomain{Name: domain.Name, Items: items})
	}
	var indexing *GraphStatusIndexing
	if status.Indexing != nil {
		indexing = &GraphStatusIndexing{State: status.Indexing.State}
		if status.Indexing.LastStartedAt != nil {
			indexing.LastStartedAt = status.Indexing.LastStartedAt.UTC().Format(time.RFC3339)
		}
		if status.Indexing.LastCompletedAt != nil {
			indexing.LastCompletedAt = status.Indexing.LastCompletedAt.UTC().Format(time.RFC3339)
		}
		if status.Indexing.LastDurationMs != nil {
			indexing.LastDurationMs = *status.Indexing.LastDurationMs
		}
		if status.Indexing.LastError != nil {
			indexing.LastError = *status.Indexing.LastError
		}
	}
	return GraphStatusOutput{
		FormattedText: status.FormattedText,
		Projects:      projects,
		Domains:       domains,
		Indexing:      indexing,
	}
}
