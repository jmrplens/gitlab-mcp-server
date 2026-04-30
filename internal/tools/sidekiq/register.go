// register.go wires sidekiq MCP tools to the MCP server.
package sidekiq

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all Sidekiq metrics MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_sidekiq_queue_metrics",
		Title:       toolutil.TitleFromName("gitlab_get_sidekiq_queue_metrics"),
		Description: "Get Sidekiq queue metrics (admin). Returns backlog and latency for all queues.\n\nReturns: JSON with queue metrics.\n\nSee also: gitlab_get_sidekiq_compound_metrics.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetQueueMetricsInput) (*mcp.CallToolResult, GetQueueMetricsOutput, error) {
		start := time.Now()
		out, err := GetQueueMetrics(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_sidekiq_queue_metrics", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatQueueMetricsMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_sidekiq_process_metrics",
		Title:       toolutil.TitleFromName("gitlab_get_sidekiq_process_metrics"),
		Description: "Get Sidekiq process metrics (admin). Returns information about running Sidekiq processes.\n\nReturns: JSON with process metrics.\n\nSee also: gitlab_get_sidekiq_compound_metrics.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetProcessMetricsInput) (*mcp.CallToolResult, GetProcessMetricsOutput, error) {
		start := time.Now()
		out, err := GetProcessMetrics(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_sidekiq_process_metrics", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProcessMetricsMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_sidekiq_job_stats",
		Title:       toolutil.TitleFromName("gitlab_get_sidekiq_job_stats"),
		Description: "Get Sidekiq job statistics (admin). Returns processed, failed, and enqueued counts.\n\nReturns: JSON with job statistics.\n\nSee also: gitlab_get_sidekiq_compound_metrics.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetJobStatsInput) (*mcp.CallToolResult, GetJobStatsOutput, error) {
		start := time.Now()
		out, err := GetJobStats(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_sidekiq_job_stats", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatJobStatsMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_sidekiq_compound_metrics",
		Title:       toolutil.TitleFromName("gitlab_get_sidekiq_compound_metrics"),
		Description: "Get all Sidekiq metrics in a single compound response (admin). Returns queue metrics, process metrics, and job statistics combined.\n\nReturns: JSON with combined queue, process, and job metrics.\n\nSee also: gitlab_get_sidekiq_queue_metrics.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetCompoundMetricsInput) (*mcp.CallToolResult, GetCompoundMetricsOutput, error) {
		start := time.Now()
		out, err := GetCompoundMetrics(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_sidekiq_compound_metrics", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatCompoundMetricsMarkdown(out)), out, nil)
	})
}
