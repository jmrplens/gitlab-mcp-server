package issuestatistics

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all issue statistics MCP tools on the given server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_issue_statistics",
		Title:       toolutil.TitleFromName("gitlab_get_issue_statistics"),
		Description: "Get global issue statistics (counts of all/opened/closed issues).\n\nReturns: JSON with issue counts (all, opened, closed).\n\nSee also: gitlab_get_group_issue_statistics, gitlab_get_project_issue_statistics",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, StatisticsOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_issue_statistics", start, err)
		if err != nil {
			return nil, StatisticsOutput{}, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown("Global", out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_group_issue_statistics",
		Title:       toolutil.TitleFromName("gitlab_get_group_issue_statistics"),
		Description: "Get issue statistics for a group.\n\nReturns: JSON with group issue counts.\n\nSee also: gitlab_get_issue_statistics, gitlab_issue_list_group",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetGroupInput) (*mcp.CallToolResult, StatisticsOutput, error) {
		start := time.Now()
		out, err := GetGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_group_issue_statistics", start, err)
		if err != nil {
			return nil, StatisticsOutput{}, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown("Group", out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_project_issue_statistics",
		Title:       toolutil.TitleFromName("gitlab_get_project_issue_statistics"),
		Description: "Get issue statistics for a project.\n\nReturns: JSON with project issue counts.\n\nSee also: gitlab_get_issue_statistics, gitlab_issue_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetProjectInput) (*mcp.CallToolResult, StatisticsOutput, error) {
		start := time.Now()
		out, err := GetProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_issue_statistics", start, err)
		if err != nil {
			return nil, StatisticsOutput{}, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown("Project", out)), out, nil)
	})
}

// RegisterMeta registers the gitlab_issue_statistics meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"get":         toolutil.RouteAction(client, Get),
		"get_group":   toolutil.RouteAction(client, GetGroup),
		"get_project": toolutil.RouteAction(client, GetProject),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_issue_statistics",
		Title: toolutil.TitleFromName("gitlab_issue_statistics"),
		Description: `Get aggregated issue statistics from GitLab. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- get: Get global issue statistics. Params: labels, milestone, scope, author_id, assignee_id, search, confidential
- get_group: Get group issue statistics. Params: group_id (required), labels, milestone, scope, author_id, assignee_id, search, confidential
- get_project: Get project issue statistics. Params: project_id (required), labels, milestone, scope, author_id, assignee_id, search, confidential`,
		Annotations:  toolutil.DeriveAnnotations(routes),
		Icons:        toolutil.IconAnalytics,
		InputSchema:  toolutil.MetaToolSchema(routes),
		OutputSchema: toolutil.MetaToolOutputSchema(),
	}, toolutil.MakeMetaHandler("gitlab_issue_statistics", routes, nil))
}
