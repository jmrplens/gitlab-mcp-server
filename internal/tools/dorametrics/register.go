// register.go wires DORA metrics MCP tools to the MCP server.

package dorametrics

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers DORA metrics tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_project_dora_metrics",
		Title:       toolutil.TitleFromName("gitlab_get_project_dora_metrics"),
		Description: "Get DORA metrics for a GitLab project. Requires metric type (deployment_frequency, lead_time_for_changes, time_to_restore_service, change_failure_rate). Supports date range and interval filtering. Returns: list of date/value data points. See also: gitlab_get_group_dora_metrics, gitlab_environment_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetProjectMetrics(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_dora_metrics", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out, input.Metric)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_group_dora_metrics",
		Title:       toolutil.TitleFromName("gitlab_get_group_dora_metrics"),
		Description: "Get DORA metrics for a GitLab group. Requires metric type (deployment_frequency, lead_time_for_changes, time_to_restore_service, change_failure_rate). Supports date range and interval filtering. Returns: list of date/value data points. See also: gitlab_get_project_dora_metrics, gitlab_environment_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GroupInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetGroupMetrics(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_group_dora_metrics", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out, input.Metric)), out, err)
	})
}
