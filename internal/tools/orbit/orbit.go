package orbit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ResponseFormatInput is the shared response_format selector used by
// the status, dsl, and query Orbit endpoints.
type ResponseFormatInput struct {
	// ResponseFormat selects the Orbit response shape. Allowed values:
	//   - "raw"  — structured JSON (or TOON JSON, depending on endpoint)
	//   - "llm"  — compact text optimized for LLM consumption
	//   - "json" — explicit JSON request (accepted by /orbit/status, /orbit/schema,
	//              /orbit/tools, and /orbit/query as an alias of "raw")
	//
	// When empty, status / schema / tools / dsl fall back to the GitLab API
	// server-side default (currently "raw" for /orbit/dsl, "json" for
	// /orbit/status, /orbit/schema, and /orbit/tools). [Query] is the
	// exception: when the field is omitted, [Query] forces "raw" rather
	// than letting the server apply its "llm" default, because structured
	// JSON is strictly more useful for an MCP tool (the LLM can iterate
	// over result nodes directly, the SDK decodes row_count, and the
	// downstream markdown formatter pretty-prints the envelope). Callers
	// who want the compact TOON text can pass response_format="llm"
	// explicitly.
	ResponseFormat string `json:"response_format,omitempty" jsonschema:"Response format: raw, llm, or json. When omitted, the API server-side default is used. Note: gitlab_orbit_query forces raw when this field is empty."`
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
	// Format selects the schema response shape. Allowed values: "raw",
	// "llm", "json". Empty uses the API server-side default ("json").
	Format string `json:"format,omitempty" jsonschema:"Schema response format: raw, llm, or json. When omitted, the API server-side default is used (json)."`
	// ResponseFormat is an alias for Format, accepted for compatibility
	// with the public Orbit API documentation. Must match Format when
	// both are set; Format wins when only one is set.
	ResponseFormat string `json:"response_format,omitempty" jsonschema:"Alias for format. Must match format when both are set."`
}

// ToolsInput is the input for listing the Orbit MCP tool manifest.
// The endpoint accepts no parameters; the type exists so the handler
// signature is uniform with the other Orbit handlers.
type ToolsInput struct{}

// DSLInput holds parameters for retrieving the Orbit query DSL.
type DSLInput struct {
	ResponseFormatInput
}

// DSLOutput holds the Orbit query DSL response.
type DSLOutput struct {
	toolutil.HintableOutput
	// ResponseFormat echoes the response_format that produced Content
	// ("raw" for the JSON Schema body, "llm" for the text grammar).
	ResponseFormat string `json:"response_format,omitempty"`
	// Content is the DSL body verbatim, encoded as JSON or text
	// depending on ResponseFormat.
	Content string `json:"content,omitempty"`
}

// QueryInput holds parameters for executing an Orbit Knowledge Graph query.
//
// The Query field is the live Orbit query DSL. GitLab validates the
// shape server-side against a JSON Schema with four `oneOf`
// alternatives keyed on `query_type`:
//
//  1. traversal    — {query_type, node|nodes, relationships?, filters?, columns?, limit, cursor?, order_by?, options?}
//  2. aggregation  — {query_type, nodes, aggregations, group_by?, aggregation_sort?, limit, options?}
//  3. neighbors    — {query_type, node, neighbors: {node, direction?, rel_types?}, options?, limit}
//  4. path_finding — {query_type, nodes: [2+], path: {type, from, to, max_depth, rel_types?}, options?, limit}
//
// Top-level fields (only `query_type` is required):
//   - node|nodes:        one or more node selectors. Use `node` for a
//     single node, `nodes: [array]` for multiple.
//   - relationships:     [{type, from, to, direction?, min_hops?, max_hops?, filters?}]. Joins node selectors by alias.
//   - aggregations:      [{function: count|sum|avg|min|max, target, property?, alias}]. Required for aggregation.
//   - group_by:          [{kind: node|property, node, property?, alias?}] — buckets aggregation rows.
//   - aggregation_sort:  {column, direction: ASC|DESC} — sorts aggregation results.
//   - order_by:          {node, property, direction: ASC|DESC} — sorts traversal results.
//   - limit:             integer, 1..1000. Default 30.
//   - cursor:            {page_size, offset} — pagination. `page_size + offset` must be <= `limit`.
//   - path:              required for path_finding. {type: shortest|all_shortest|any, from, to, max_depth: 1..3, rel_types?}.
//   - neighbors:         required for neighbors. {node, direction?: outgoing|incoming|both, rel_types?}.
//   - options:           {dynamic_columns?: default|*, include_debug_sql?, skip_dedup?, ...} for performance/debug knobs.
//
// Node selector fields:
//   - id:        string, local alias. Referenced by relationships, aggregations, path, and neighbors.
//   - entity:    string, ontology node type (Project, MergeRequest, File, Vulnerability, etc.).
//   - columns:   string[] of properties to return, or "*" for all non-restricted columns.
//   - filters:   {property: value_or_op_object} — see operators below.
//   - node_ids:  int[] of exact ids. Maximum 500 per selector.
//   - id_range:  {start, end}. Span must be <= 100,000 to count as scope.
//   - id_property: optional. Property used by node_ids and id_range. Default "id".
//
// Filter operators ({op, value}):
//
//	eq, gt, gte, lt, lte, in, contains, starts_with, ends_with,
//	is_null, is_not_null, token_match, all_tokens, any_tokens.
//
// Simple equality is also accepted: {property: "value"}.
//
// Scope rules: traversal and aggregation queries MUST have at least
// one node with `node_ids`, `filters`, or `id_range` (span <= 100K).
// Otherwise the server rejects the request to avoid full edge table
// scans. Neighbors and path_finding reference nodes by their
// top-level `id` (a string), not by entity name.
//
// Examples:
//
//   - Find projects in plens1:
//     {query_type:"traversal", node:{id:"p",entity:"Project",
//     filters:{full_path:{op:"starts_with",value:"plens1/"}}}}
//
//   - Count MRs per project (group_by node):
//     {query_type:"aggregation",
//     nodes:[{id:"p",entity:"Project",filters:{...}},
//     {id:"mr",entity:"MergeRequest",columns:["id"]}],
//     relationships:[{type:"IN_PROJECT",from:"mr",to:"p"}],
//     group_by:[{kind:"node",node:"p"}],
//     aggregations:[{function:"count",target:"mr",alias:"mr_count"}],
//     aggregation_sort:{column:"mr_count",direction:"DESC"}}
//
//   - Find a path between two nodes (max depth 3):
//     {query_type:"path_finding",
//     nodes:[{id:"u",entity:"User",node_ids:[...]},
//     {id:"p",entity:"Project",node_ids:[...]}],
//     path:{type:"shortest",from:"u",to:"p",max_depth:3}}
//
// Reference: https://docs.gitlab.com/orbit/remote/queries/
type QueryInput struct {
	Query map[string]any `json:"query" jsonschema:"Orbit query DSL JSON object. The query_type must be one of: traversal, aggregation, neighbors, path_finding. See QueryInput docstring for the shape of each variant.,required"`
	ResponseFormatInput
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

// StatusOutput is the Orbit cluster health response.
type StatusOutput struct {
	toolutil.HintableOutput
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
	// FormattedText is the pre-formatted text body returned by the
	// server when the caller selected the "llm" response format.
	FormattedText string `json:"formatted_text,omitempty"`
	// Result is the decoded result envelope returned for the
	// "raw" or "json" response format. Its shape depends on the
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
// named node types with full properties; format selects raw, llm, or
// json response shapes.
func Schema(ctx context.Context, client *gitlabclient.Client, input SchemaInput) (SchemaOutput, error) {
	if err := ctx.Err(); err != nil {
		return SchemaOutput{}, err
	}
	format, hasFormat, err := schemaResponseFormat(input)
	if err != nil {
		return SchemaOutput{}, err
	}
	opts := &gl.GetOrbitSchemaOptions{}
	if hasFormat {
		opts.Format = format
	}
	if len(input.Expand) > 0 {
		expand := input.Expand
		opts.Expand = &expand
	}

	schema, _, err := client.GL().Orbit.GetSchema(opts, gl.WithContext(ctx))
	if err != nil {
		return SchemaOutput{}, wrapOrbitErr("orbit_schema", err)
	}
	return convertSchema(schema), nil
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

// DSL retrieves the Orbit query DSL grammar or LLM-friendly schema
// verbatim from GitLab.com.
//
// Endpoint: GET /api/v4/orbit/schema/dsl. With response_format="raw"
// the body is a JSON Schema document; with response_format="llm" it
// is a compact text grammar suitable for inclusion in an LLM prompt.
func DSL(ctx context.Context, client *gitlabclient.Client, input DSLInput) (DSLOutput, error) {
	if err := ctx.Err(); err != nil {
		return DSLOutput{}, err
	}
	format, hasFormat, err := responseFormat(input.ResponseFormat, "response_format")
	if err != nil {
		return DSLOutput{}, err
	}
	opts := &gl.GetOrbitDslOptions{}
	if hasFormat {
		opts.ResponseFormat = format
	}

	content, _, err := client.GL().Orbit.GetDsl(opts, gl.WithContext(ctx))
	if err != nil {
		return DSLOutput{}, wrapOrbitErr("orbit_dsl", err)
	}
	return DSLOutput{ResponseFormat: responseFormatName(format), Content: content}, nil
}

// Query executes a read-only Orbit Knowledge Graph query on GitLab.com.
//
// Endpoint: POST /api/v4/orbit/query. The query body is validated by
// [validateQuery] before being forwarded; see [QueryInput] for the full
// DSL shape.
//
// The live API exposes four `oneOf` query_type variants:
//
//   - traversal    — multi-node joins with relationships, ordering, paging.
//   - aggregation  — group-by plus count|sum|avg|min|max functions.
//   - neighbors    — one-hop or two-hop expansion from a single node.
//   - path_finding — shortest path between two top-level nodes (depth 1..3).
//
// When response_format is omitted, the handler defaults to "raw"
// (structured JSON) rather than the API server-side default of "llm"
// (compact TOON text). Structured JSON is strictly more useful for an
// MCP tool: the LLM can iterate over result nodes directly, the SDK
// decodes row_count, and downstream markdown formatters pretty-print
// the envelope. Callers who want the compact TOON text can pass
// response_format="llm" explicitly.
func Query(ctx context.Context, client *gitlabclient.Client, input QueryInput) (QueryOutput, error) {
	if err := ctx.Err(); err != nil {
		return QueryOutput{}, err
	}
	query, err := validateQuery(input.Query)
	if err != nil {
		return QueryOutput{}, err
	}
	format, hasFormat, err := responseFormat(input.ResponseFormat, "response_format")
	if err != nil {
		return QueryOutput{}, err
	}
	request := &gl.OrbitQueryRequest{
		Query: query,
	}
	// Default to "raw" when the caller did not pick a format; see the
	// Query function godoc for the LLM-friendly rationale.
	rawFormat := gl.OrbitResponseFormatRaw
	switch {
	case hasFormat:
		request.ResponseFormat = format
	case !hasFormat:
		request.ResponseFormat = &rawFormat
	}
	useRaw := hasFormat && *format == gl.OrbitResponseFormatLLM
	if useRaw {
		var raw bytes.Buffer
		_, err = client.GL().Orbit.QueryRaw(request, &raw, gl.WithContext(ctx))
		if err != nil {
			return QueryOutput{}, wrapOrbitErr("orbit_query", err)
		}
		return QueryOutput{FormattedText: raw.String(), QueryType: queryType(input.Query)}, nil
	}

	result, _, err := client.GL().Orbit.Query(request, gl.WithContext(ctx))
	if err != nil {
		return QueryOutput{}, wrapOrbitErr("orbit_query", err)
	}
	return convertQuery(result), nil
}

// GraphStatus retrieves Orbit graph indexing status for a single
// namespace, project, or full path on GitLab.com.
//
// Endpoint: GET /api/v4/orbit/graph_status. Exactly one of namespace_id,
// project_id, or full_path must be set. Useful for verifying that the
// indexer has caught up before running Query.
func GraphStatus(ctx context.Context, client *gitlabclient.Client, input GraphStatusInput) (GraphStatusOutput, error) {
	if err := ctx.Err(); err != nil {
		return GraphStatusOutput{}, err
	}
	opts, err := graphStatusOptions(input)
	if err != nil {
		return GraphStatusOutput{}, err
	}

	status, _, err := client.GL().Orbit.GetGraphStatus(opts, gl.WithContext(ctx))
	if err != nil {
		return GraphStatusOutput{}, wrapOrbitErr("orbit_graph_status", err)
	}
	return convertGraphStatus(status), nil
}

// schemaResponseFormat normalizes the SchemaInput format fields. The
// input accepts both Format (primary) and ResponseFormat (alias) for
// public-API compatibility; both must agree when set, and Format wins
// when only one is set.
func schemaResponseFormat(input SchemaInput) (*gl.OrbitResponseFormatValue, bool, error) {
	format := strings.TrimSpace(input.Format)
	responseFormatAlias := strings.TrimSpace(input.ResponseFormat)
	if format != "" && responseFormatAlias != "" && !strings.EqualFold(format, responseFormatAlias) {
		return nil, false, errors.New("format and response_format must match when both are set")
	}
	if format != "" {
		return responseFormat(format, "format")
	}
	return responseFormat(responseFormatAlias, "response_format")
}

// responseFormat normalizes a user-supplied format string to the SDK value
// the GitLab Orbit API understands. Returns (nil, false, nil) when the input
// is empty so the API applies its own server-side default (which differs per
// endpoint: "json" for status/schema/tools, "raw" for dsl/query). Pass an
// explicit value to force a specific format.
//
// Allowed values are "raw", "llm", and "json" (the latter is accepted as an
// explicit JSON request alias). The SDK's [gl.OrbitResponseFormatValue] is a
// plain string type, so unknown server-side values are rejected.
func responseFormat(format, field string) (*gl.OrbitResponseFormatValue, bool, error) {
	normalized := strings.ToLower(strings.TrimSpace(format))
	if normalized == "" {
		return nil, false, nil
	}
	switch normalized {
	case string(gl.OrbitResponseFormatRaw), string(gl.OrbitResponseFormatLLM), "json":
		value := gl.OrbitResponseFormatValue(normalized)
		return &value, true, nil
	default:
		return nil, false, errors.New("invalid " + field + ": use raw, llm, or json")
	}
}

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
	components := make([]StatusComponent, 0, len(status.Components))
	for _, component := range status.Components {
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
	return StatusOutput{
		FormattedText: status.FormattedText,
		Status:        status.Status,
		Timestamp:     status.Timestamp,
		Version:       status.Version,
		Components:    components,
	}
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
