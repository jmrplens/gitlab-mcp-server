// register.go wires issuelinks MCP tools to the MCP server.
package issuelinks

import (
	"context"
	"fmt"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers the four issue link management tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlab.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_link_list",
		Title:       toolutil.TitleFromName("gitlab_issue_link_list"),
		Description: "List issue relations (linked issues) for a given issue in a GitLab project. Returns related issues with link type (relates_to, blocks, is_blocked_by).\n\nReturns: JSON array of linked issues.\n\nSee also: gitlab_issue_link_create, gitlab_issue_get",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLink,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_link_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_link_get",
		Title:       toolutil.TitleFromName("gitlab_issue_link_get"),
		Description: "Get a specific issue link by ID, returning source and target issue details with link type.\n\nReturns: JSON with the issue link details.\n\nSee also: gitlab_issue_link_list, gitlab_issue_link_delete",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLink,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_issue_link_get", start, nil)
			return toolutil.NotFoundResult("Issue Link", fmt.Sprintf("link %d on issue #%d in project %s", input.IssueLinkID, input.IssueIID, input.ProjectID),
				"Use gitlab_issue_link_list to list links on this issue",
				"Verify the issue_link_id and issue_iid are correct",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_link_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_link_create",
		Title:       toolutil.TitleFromName("gitlab_issue_link_create"),
		Description: "Create a link between two issues. Specify source project/issue and target project/issue. Link types: relates_to (default), blocks, is_blocked_by.\n\nReturns: JSON with the created issue link details.\n\nSee also: gitlab_issue_link_list, gitlab_issue_link_delete",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconLink,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_link_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_issue_link_delete",
		Title:       toolutil.TitleFromName("gitlab_issue_link_delete"),
		Description: "Delete an issue link, removing the two-way relationship between the linked issues. This action cannot be undone.\n\nReturns: confirmation message.\n\nSee also: gitlab_issue_link_list, gitlab_issue_link_create",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconLink,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete issue link %d from issue %d in project %q?", input.IssueLinkID, input.IssueIID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_link_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("issue link")
	})
}
