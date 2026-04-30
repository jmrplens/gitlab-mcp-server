// register.go wires group activity analytics MCP tools to the MCP server.
package groupanalytics

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab group activity analytics.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_recently_created_issues_count",
		Title:       toolutil.TitleFromName("gitlab_get_recently_created_issues_count"),
		Description: "Get the count of recently created issues for a group (last 90 days).\n\nReturns: JSON with group path and issues count.\n\nSee also: gitlab_get_recently_created_mr_count, gitlab_get_recently_added_members_count",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input IssuesCountInput) (*mcp.CallToolResult, IssuesCountOutput, error) {
		start := time.Now()
		out, err := GetIssuesCount(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_recently_created_issues_count", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatIssuesCountMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_recently_created_mr_count",
		Title:       toolutil.TitleFromName("gitlab_get_recently_created_mr_count"),
		Description: "Get the count of recently created merge requests for a group (last 90 days).\n\nReturns: JSON with group path and merge requests count.\n\nSee also: gitlab_get_recently_created_issues_count, gitlab_get_recently_added_members_count",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MRCountInput) (*mcp.CallToolResult, MRCountOutput, error) {
		start := time.Now()
		out, err := GetMRCount(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_recently_created_mr_count", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMRCountMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_recently_added_members_count",
		Title:       toolutil.TitleFromName("gitlab_get_recently_added_members_count"),
		Description: "Get the count of recently added members in a group (last 90 days).\n\nReturns: JSON with group path and new members count.\n\nSee also: gitlab_get_recently_created_issues_count, gitlab_get_recently_created_mr_count",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MembersCountInput) (*mcp.CallToolResult, MembersCountOutput, error) {
		start := time.Now()
		out, err := GetMembersCount(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_recently_added_members_count", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMembersCountMarkdown(out)), out, err)
	})
}
