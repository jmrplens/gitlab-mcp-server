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

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

type ResponseFormatInput struct {
	ResponseFormat string `json:"response_format,omitempty" jsonschema:"Response format to request: raw or llm. Defaults to raw."`
}

// StatusInput holds parameters for retrieving Orbit cluster status.
type StatusInput struct {
	ResponseFormatInput
}

// SchemaInput holds parameters for retrieving the Orbit graph schema.
type SchemaInput struct {
	Expand         []string `json:"expand,omitempty" jsonschema:"Node names to expand with full properties and relationships."`
	Format         string   `json:"format,omitempty" jsonschema:"Schema response format to request: raw or llm. Defaults to raw."`
	ResponseFormat string   `json:"response_format,omitempty" jsonschema:"Alias for format, accepted for compatibility with GitLab's public Orbit API documentation."`
}

// ToolsInput is the input for listing Orbit MCP tool manifests.
type ToolsInput struct{}

// DSLInput holds parameters for retrieving the Orbit query DSL.
type DSLInput struct {
	ResponseFormatInput
}

// DSLOutput is the raw Orbit query DSL response.
type DSLOutput struct {
	toolutil.HintableOutput
	ResponseFormat string `json:"response_format,omitempty"`
	Content        string `json:"content,omitempty"`
}

// QueryInput holds parameters for executing an Orbit Knowledge Graph query.
type QueryInput struct {
	Query map[string]any `json:"query" jsonschema:"Orbit query DSL JSON object,required"`
	ResponseFormatInput
}

// GraphStatusInput holds parameters for retrieving Orbit graph indexing status.
type GraphStatusInput struct {
	NamespaceID int64  `json:"namespace_id,omitempty" jsonschema:"Namespace/group ID to inspect. Set exactly one of namespace_id, project_id, or full_path."`
	ProjectID   int64  `json:"project_id,omitempty"   jsonschema:"Project ID to inspect. Set exactly one of namespace_id, project_id, or full_path."`
	FullPath    string `json:"full_path,omitempty"    jsonschema:"Full path of a group or project to inspect, for example gitlab-org/gitlab. Set exactly one scope field."`
	ResponseFormatInput
}

// StatusReplicas describes ready and desired replica counts for an Orbit component.
type StatusReplicas struct {
	Ready   int64 `json:"ready"`
	Desired int64 `json:"desired"`
}

// StatusComponent describes an Orbit subsystem status entry.
type StatusComponent struct {
	Name     string          `json:"name,omitempty"`
	Status   string          `json:"status,omitempty"`
	Replicas *StatusReplicas `json:"replicas,omitempty"`
	Metrics  any             `json:"metrics,omitempty"`
}

// StatusOutput is the Orbit cluster health response.
type StatusOutput struct {
	toolutil.HintableOutput
	FormattedText string            `json:"formatted_text,omitempty"`
	Status        string            `json:"status,omitempty"`
	Timestamp     string            `json:"timestamp,omitempty"`
	Version       string            `json:"version,omitempty"`
	Components    []StatusComponent `json:"components,omitempty"`
}

// SchemaDomain describes a logical grouping of Orbit graph node types.
type SchemaDomain struct {
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	NodeNames   []string `json:"node_names,omitempty"`
}

// SchemaEdge describes an Orbit graph edge type.
type SchemaEdge struct {
	Name        string              `json:"name,omitempty"`
	Description string              `json:"description,omitempty"`
	Variants    []SchemaEdgeVariant `json:"variants,omitempty"`
}

// SchemaEdgeVariant describes a valid source/target pair for an Orbit edge.
type SchemaEdgeVariant struct {
	SourceType string `json:"source_type,omitempty"`
	TargetType string `json:"target_type,omitempty"`
}

// SchemaOutput is the Orbit graph ontology response.
type SchemaOutput struct {
	toolutil.HintableOutput
	SchemaVersion string         `json:"schema_version,omitempty"`
	Domains       []SchemaDomain `json:"domains,omitempty"`
	Nodes         []any          `json:"nodes,omitempty"`
	Edges         []SchemaEdge   `json:"edges,omitempty"`
}

// ToolDefinition describes one MCP tool manifest entry served by Orbit.
type ToolDefinition struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// ToolsOutput is the Orbit MCP tool manifest response.
type ToolsOutput struct {
	toolutil.HintableOutput
	Tools []ToolDefinition `json:"tools,omitempty"`
}

// QueryOutput is the result envelope returned by Orbit query execution.
type QueryOutput struct {
	toolutil.HintableOutput
	FormattedText   string   `json:"formatted_text,omitempty"`
	Result          any      `json:"result,omitempty"`
	QueryType       string   `json:"query_type,omitempty"`
	RawQueryStrings []string `json:"raw_query_strings,omitempty"`
	RowCount        int64    `json:"row_count,omitempty"`
}

// GraphStatusProjects describes indexed and known project counts.
type GraphStatusProjects struct {
	Indexed    int64 `json:"indexed"`
	TotalKnown int64 `json:"total_known"`
}

// GraphStatusDomainItem describes a count for one Orbit graph node type.
type GraphStatusDomainItem struct {
	Name  string `json:"name,omitempty"`
	Count int64  `json:"count"`
}

// GraphStatusDomain describes indexing counts for a graph domain.
type GraphStatusDomain struct {
	Name  string                  `json:"name,omitempty"`
	Items []GraphStatusDomainItem `json:"items,omitempty"`
}

// GraphStatusIndexing describes the latest indexing pipeline state.
type GraphStatusIndexing struct {
	State           string `json:"state,omitempty"`
	LastStartedAt   string `json:"last_started_at,omitempty"`
	LastCompletedAt string `json:"last_completed_at,omitempty"`
	LastDurationMs  int64  `json:"last_duration_ms,omitempty"`
	LastError       string `json:"last_error,omitempty"`
}

// GraphStatusOutput is the Orbit graph indexing status response.
type GraphStatusOutput struct {
	toolutil.HintableOutput
	FormattedText string               `json:"formatted_text,omitempty"`
	Projects      *GraphStatusProjects `json:"projects,omitempty"`
	Domains       []GraphStatusDomain  `json:"domains,omitempty"`
	Indexing      *GraphStatusIndexing `json:"indexing,omitempty"`
}

// Status retrieves Orbit cluster health.
func Status(ctx context.Context, client *gitlabclient.Client, input StatusInput) (StatusOutput, error) {
	if err := ctx.Err(); err != nil {
		return StatusOutput{}, err
	}
	format, err := responseFormat(input.ResponseFormat, "response_format")
	if err != nil {
		return StatusOutput{}, err
	}

	status, _, err := client.GL().Orbit.GetStatus(&gl.GetOrbitStatusOptions{ResponseFormat: format}, gl.WithContext(ctx))
	if err != nil {
		return StatusOutput{}, wrapOrbitErr("orbit_status", err)
	}
	return convertStatus(status), nil
}

// Schema retrieves the Orbit graph ontology.
func Schema(ctx context.Context, client *gitlabclient.Client, input SchemaInput) (SchemaOutput, error) {
	if err := ctx.Err(); err != nil {
		return SchemaOutput{}, err
	}
	format, err := schemaResponseFormat(input)
	if err != nil {
		return SchemaOutput{}, err
	}
	opts := &gl.GetOrbitSchemaOptions{Format: format}
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

// Tools retrieves the Orbit MCP tool manifest.
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

// DSL retrieves the Orbit query DSL body verbatim.
func DSL(ctx context.Context, client *gitlabclient.Client, input DSLInput) (DSLOutput, error) {
	if err := ctx.Err(); err != nil {
		return DSLOutput{}, err
	}
	format, err := responseFormat(input.ResponseFormat, "response_format")
	if err != nil {
		return DSLOutput{}, err
	}

	content, _, err := client.GL().Orbit.GetDsl(&gl.GetOrbitDslOptions{ResponseFormat: format}, gl.WithContext(ctx))
	if err != nil {
		return DSLOutput{}, wrapOrbitErr("orbit_dsl", err)
	}
	return DSLOutput{ResponseFormat: responseFormatName(format), Content: content}, nil
}

// Query executes an Orbit Knowledge Graph query.
func Query(ctx context.Context, client *gitlabclient.Client, input QueryInput) (QueryOutput, error) {
	if err := ctx.Err(); err != nil {
		return QueryOutput{}, err
	}
	query, err := validateQuery(input.Query)
	if err != nil {
		return QueryOutput{}, err
	}
	format, err := responseFormat(input.ResponseFormat, "response_format")
	if err != nil {
		return QueryOutput{}, err
	}
	request := &gl.OrbitQueryRequest{
		Query:          query,
		ResponseFormat: format,
	}
	if format != nil && *format == gl.OrbitResponseFormatLLM {
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

// GraphStatus retrieves Orbit graph indexing status for a namespace or project.
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

func schemaResponseFormat(input SchemaInput) (*gl.OrbitResponseFormatValue, error) {
	format := strings.TrimSpace(input.Format)
	responseFormatAlias := strings.TrimSpace(input.ResponseFormat)
	if format != "" && responseFormatAlias != "" && !strings.EqualFold(format, responseFormatAlias) {
		return nil, errors.New("format and response_format must match when both are set")
	}
	if format != "" {
		return responseFormat(format, "format")
	}
	return responseFormat(responseFormatAlias, "response_format")
}

func responseFormat(format, field string) (*gl.OrbitResponseFormatValue, error) {
	if strings.TrimSpace(format) == "" {
		value := gl.OrbitResponseFormatRaw
		return &value, nil
	}
	switch normalized := strings.ToLower(strings.TrimSpace(format)); normalized {
	case string(gl.OrbitResponseFormatRaw), string(gl.OrbitResponseFormatLLM):
		value := gl.OrbitResponseFormatValue(normalized)
		return &value, nil
	default:
		return nil, errors.New("invalid " + field + ": use raw or llm")
	}
}

func responseFormatName(format *gl.OrbitResponseFormatValue) string {
	if format == nil {
		return string(gl.OrbitResponseFormatRaw)
	}
	return string(*format)
}

func validateQuery(query map[string]any) (json.RawMessage, error) {
	if query == nil {
		return nil, toolutil.ErrFieldRequired("query")
	}
	buf, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("query must be a JSON object: %w", err)
	}
	return json.RawMessage(buf), nil
}

func queryType(query map[string]any) string {
	queryTypeValue, ok := query["query_type"].(string)
	if !ok {
		return ""
	}
	return queryTypeValue
}

func graphStatusOptions(input GraphStatusInput) (*gl.GetGraphStatusOptions, error) {
	if input.NamespaceID < 0 {
		return nil, errors.New("namespace_id must not be negative")
	}
	if input.ProjectID < 0 {
		return nil, errors.New("project_id must not be negative")
	}
	format, err := responseFormat(input.ResponseFormat, "response_format")
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

	opts := &gl.GetGraphStatusOptions{ResponseFormat: format}
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
