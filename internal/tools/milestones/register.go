// register.go wires milestones MCP tools to the MCP server.

package milestones

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers milestone-related tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_milestone_list",
		Title:       toolutil.TitleFromName("gitlab_milestone_list"),
		Description: "List milestones for a GitLab project. Supports filtering by state (active, closed), exact title, search keyword, and including milestones from ancestor groups. Returns milestone title, description, state, start/due dates, web URL, and expiration status with pagination.\n\nReturns: JSON array of milestones with pagination. See also: gitlab_milestone_get, gitlab_milestone_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_milestone_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_milestone_get",
		Title:       toolutil.TitleFromName("gitlab_milestone_get"),
		Description: "Get details of a single project milestone by IID. Returns milestone title, description, state, start/due dates, web URL, and expiration status.\n\nReturns: JSON with milestone details. See also: gitlab_milestone_update, gitlab_milestone_issues.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_milestone_get", start, nil)
			return toolutil.NotFoundResult("Milestone", fmt.Sprintf("IID %d in project %s", input.MilestoneIID, input.ProjectID),
				"Use gitlab_milestone_list with project_id to list milestones",
				"Verify the milestone IID is correct for this project",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_milestone_get", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		if err == nil && out.IID > 0 && string(input.ProjectID) != "" {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://project/%s/milestone/%d", url.PathEscape(string(input.ProjectID)), out.IID),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_milestone_create",
		Title:       toolutil.TitleFromName("gitlab_milestone_create"),
		Description: "Create a new milestone in a GitLab project. Requires title; optionally set description, start_date (YYYY-MM-DD), and due_date (YYYY-MM-DD). Returns: milestone IID, title, state, start/due dates, web URL, and expiration status. See also: gitlab_milestone_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_milestone_create", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_milestone_update",
		Title:       toolutil.TitleFromName("gitlab_milestone_update"),
		Description: "Update an existing project milestone by IID. Supports changing title, description, start_date, due_date, and state_event (activate/close). Returns: updated milestone with IID, title, state, dates, web URL, and expired flag. See also: gitlab_milestone_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_milestone_update", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_milestone_delete",
		Title:       toolutil.TitleFromName("gitlab_milestone_delete"),
		Description: "Delete a project milestone by IID. This action is irreversible.\n\nReturns: confirmation message. See also: gitlab_milestone_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete milestone IID %d in project %q?", input.MilestoneIID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_milestone_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("milestone")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_milestone_issues",
		Title:       toolutil.TitleFromName("gitlab_milestone_issues"),
		Description: "List all issues assigned to a project milestone by IID. Returns issue IID, title, state, web URL, and creation date with pagination.\n\nReturns: JSON array of issues with pagination. See also: gitlab_milestone_get, gitlab_issue_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetIssuesInput) (*mcp.CallToolResult, MilestoneIssuesOutput, error) {
		start := time.Now()
		out, err := GetIssues(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_milestone_issues", start, err)
		return toolutil.WithHints(FormatIssuesMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_milestone_merge_requests",
		Title:       toolutil.TitleFromName("gitlab_milestone_merge_requests"),
		Description: "List all merge requests assigned to a project milestone by IID. Returns MR IID, title, state, source/target branches, web URL, and creation date with pagination.\n\nReturns: JSON array of merge requests with pagination. See also: gitlab_milestone_get, gitlab_mr_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetMergeRequestsInput) (*mcp.CallToolResult, MilestoneMergeRequestsOutput, error) {
		start := time.Now()
		out, err := GetMergeRequests(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_milestone_merge_requests", start, err)
		return toolutil.WithHints(FormatMergeRequestsMarkdown(out), out, err)
	})
}
