// register.go wires usagedata MCP tools to the MCP server.
package usagedata

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all Usage Data MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_service_ping",
		Title:       toolutil.TitleFromName("gitlab_get_service_ping"),
		Description: "Get service ping data (admin). Returns recorded_at, license info, and usage counts.\n\nReturns: JSON with service ping data including usage counts and license info.\n\nSee also: gitlab_get_non_sql_metrics, gitlab_get_metric_definitions",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetServicePingInput) (*mcp.CallToolResult, GetServicePingOutput, error) {
		start := time.Now()
		out, err := GetServicePing(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_service_ping", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatServicePingMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_non_sql_metrics",
		Title:       toolutil.TitleFromName("gitlab_get_non_sql_metrics"),
		Description: "Get non-SQL service ping metrics (admin). Returns instance info, license details, and settings.\n\nReturns: JSON with non-SQL metrics including instance and license details.\n\nSee also: gitlab_get_service_ping, gitlab_get_usage_queries",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetNonSQLMetricsInput) (*mcp.CallToolResult, NonSQLMetricsOutput, error) {
		start := time.Now()
		out, err := GetNonSQLMetrics(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_non_sql_metrics", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatNonSQLMetricsMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_usage_queries",
		Title:       toolutil.TitleFromName("gitlab_get_usage_queries"),
		Description: "Get service ping SQL queries (admin). Returns the raw SQL queries used for service ping collection.\n\nReturns: JSON with raw SQL queries used for service ping.\n\nSee also: gitlab_get_service_ping, gitlab_get_non_sql_metrics",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetQueriesInput) (*mcp.CallToolResult, QueriesOutput, error) {
		start := time.Now()
		out, err := GetQueries(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_usage_queries", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatQueriesMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_metric_definitions",
		Title:       toolutil.TitleFromName("gitlab_get_metric_definitions"),
		Description: "Get metric definitions as YAML (admin). Returns all metric definitions used in service ping.\n\nReturns: YAML with all metric definitions.\n\nSee also: gitlab_get_service_ping, gitlab_get_usage_queries",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetMetricDefinitionsInput) (*mcp.CallToolResult, MetricDefinitionsOutput, error) {
		start := time.Now()
		out, err := GetMetricDefinitions(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_metric_definitions", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMetricDefinitionsMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_track_event",
		Title:       toolutil.TitleFromName("gitlab_track_event"),
		Description: "Track a single usage event. Params: event (required), send_to_snowplow, namespace_id, project_id.\n\nReturns: JSON confirming the event was tracked.\n\nSee also: gitlab_track_events, gitlab_get_service_ping",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input TrackEventInput) (*mcp.CallToolResult, TrackEventOutput, error) {
		start := time.Now()
		out, err := TrackEvent(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_track_event", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTrackEventMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_track_events",
		Title:       toolutil.TitleFromName("gitlab_track_events"),
		Description: "Track multiple usage events in batch. Params: events (required, array of event objects).\n\nReturns: JSON confirming the events were tracked.\n\nSee also: gitlab_track_event, gitlab_get_service_ping",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input TrackEventsInput) (*mcp.CallToolResult, TrackEventsOutput, error) {
		start := time.Now()
		out, err := TrackEvents(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_track_events", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTrackEventsMarkdown(out)), out, nil)
	})
}
