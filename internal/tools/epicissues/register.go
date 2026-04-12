// register.go wires epic-issue MCP tools to the MCP server.

package epicissues

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab epic-issue operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_epic_issue_list",
		Title:       toolutil.TitleFromName("gitlab_epic_issue_list"),
		Description: "List all issues assigned to a GitLab group epic. Supports pagination.\n\nReturns: JSON with issues array and pagination metadata. See also: gitlab_epic_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEpic,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_issue_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_epic_issue_assign",
		Title:       toolutil.TitleFromName("gitlab_epic_issue_assign"),
		Description: "Assign an existing issue to a GitLab group epic. The issue is identified by its global ID.\n\nReturns: JSON with the epic-issue association ID. See also: gitlab_epic_issue_list.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconEpic,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AssignInput) (*mcp.CallToolResult, AssignOutput, error) {
		start := time.Now()
		out, err := Assign(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_issue_assign", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAssignMarkdown(out, "assigned")), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_epic_issue_remove",
		Title:       toolutil.TitleFromName("gitlab_epic_issue_remove"),
		Description: "Remove an issue from a GitLab group epic using the epic-issue association ID.\n\nReturns: JSON with the removed association details. See also: gitlab_epic_issue_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconEpic,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RemoveInput) (*mcp.CallToolResult, AssignOutput, error) {
		start := time.Now()
		out, err := Remove(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_issue_remove", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAssignMarkdown(out, "removed")), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_epic_issue_update",
		Title:       toolutil.TitleFromName("gitlab_epic_issue_update"),
		Description: "Reorder an issue within a GitLab group epic by moving it before or after another epic-issue.\n\nReturns: JSON with the updated issues list. See also: gitlab_epic_issue_list.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconEpic,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := UpdateOrder(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_issue_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})
}
