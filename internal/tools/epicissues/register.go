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
	specs := ActionSpecs(client)
	epicIssueTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconEpic})
	}

	mcp.AddTool(server, epicIssueTool("gitlab_epic_issue_list", "List all child issues of a GitLab group epic via the Work Items GraphQL API. Supports cursor-based pagination.\n\nReturns: JSON with issues array and pagination metadata. See also: gitlab_epic_get."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_issue_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, epicIssueTool("gitlab_epic_issue_assign", "Assign an existing issue to a GitLab group epic via the Work Items GraphQL API. The issue is identified by its project path and IID.\n\nReturns: JSON with the epic and child work item GIDs. See also: gitlab_epic_issue_list."), func(ctx context.Context, req *mcp.CallToolRequest, input AssignInput) (*mcp.CallToolResult, AssignOutput, error) {
		start := time.Now()
		out, err := Assign(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_issue_assign", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAssignMarkdown(out, "assigned")), out, err)
	})

	mcp.AddTool(server, epicIssueTool("gitlab_epic_issue_remove", "Remove an issue from a GitLab group epic via the Work Items GraphQL API. Clears the child's parent reference.\n\nReturns: JSON with the epic and child work item GIDs. See also: gitlab_epic_issue_list."), func(ctx context.Context, req *mcp.CallToolRequest, input RemoveInput) (*mcp.CallToolResult, AssignOutput, error) {
		start := time.Now()
		out, err := Remove(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_issue_remove", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAssignMarkdown(out, "removed")), out, err)
	})

	mcp.AddTool(server, epicIssueTool("gitlab_epic_issue_update", "Reorder an issue within a GitLab group epic via the Work Items GraphQL API. Moves the issue before or after a reference issue.\n\nReturns: JSON with the updated issues list. See also: gitlab_epic_issue_list."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := UpdateOrder(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_epic_issue_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})
}
