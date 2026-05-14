package issues

import (
	"context"
	"fmt"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers CRUD tools for GitLab issues.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	issueTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconIssue})
	}

	mcp.AddTool(server, issueTool("gitlab_issue_create", "Create a new issue in a GitLab project. Supports title, description (Markdown), assignees, labels, milestone, due date, confidential flag, issue_type (issue/incident/test_case/task), weight, and epic_id. Returns the created issue with ID, IID, state, and web URL. See also: gitlab_issue_list, gitlab_issue_note_create."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_create", start, err)
		result := toolutil.ToolResultAnnotated(FormatMarkdown(out), toolutil.ContentMutate)
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_get", "Retrieve a single GitLab issue by its project-scoped IID (the '#N' number shown in URLs and UI, NOT the global numeric ID). For global IDs, use gitlab_issue_get_by_id instead. Returns title, description, state, labels, assignees, milestone, author, timestamps, and web URL. See also: gitlab_issue_note_list, gitlab_issue_mrs_closing."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_issue_get", start, nil)
			return toolutil.NotFoundResult("Issue", fmt.Sprintf("#%d in project %s", input.IssueIID, input.ProjectID),
				"Use gitlab_issue_list with project_id to list available issues",
				"Verify the issue IID is correct for this project (the '#N' number, not the global ID)",
				"For global IDs, use gitlab_issue_get_by_id instead",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_get", start, err)
		result := toolutil.ToolResultAnnotated(FormatMarkdown(out), toolutil.ContentDetail)
		if err == nil && out.ProjectID > 0 && out.IID > 0 {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://project/%d/issue/%d", out.ProjectID, out.IID),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_list", "List issues for a GitLab project with filters for state, labels, milestone, assignee, author, and search. Returns paginated results with issue details. See also: gitlab_issue_get, gitlab_issue_create."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_update", "Update a GitLab issue. Supports changing title, description, state (close/reopen), assignees, labels (replace, add, or remove), milestone, due date, confidential flag, issue_type, weight, and discussion_locked. Only specified fields are modified.\n\nReturns: JSON with the updated issue details. See also: gitlab_issue_get, gitlab_issue_note_create."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_delete", "Permanently delete a GitLab issue. This action cannot be undone. Requires at least Maintainer access level.\n\nReturns: confirmation message. See also: gitlab_issue_list."), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Permanently delete issue #%d in project %q? This action cannot be undone.", input.IssueIID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("issue #%d from project %s", input.IssueIID, input.ProjectID))
	})

	mcp.AddTool(server, issueTool("gitlab_issue_list_group", "List issues across all projects in a GitLab group. Supports filtering by state, labels, milestone, scope, time range, assignee, author, and search. Returns paginated issue details including project reference. See also: gitlab_issue_list, gitlab_issue_get."), func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupInput) (*mcp.CallToolResult, ListGroupOutput, error) {
		start := time.Now()
		out, err := ListGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_list_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListGroupMarkdown(out)), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_list_all", "List issues visible to the authenticated user across all projects (global scope). Supports filtering by state, labels, milestone, scope, search, assignee, author, time range, confidential flag, and ordering. Returns paginated results. See also: gitlab_issue_list, gitlab_issue_get."), func(ctx context.Context, req *mcp.CallToolRequest, input ListAllInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListAll(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_list_all", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListAllMarkdown(out)), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_get_by_id", "Retrieve a single GitLab issue by its global numeric ID (the 'id' field, NOT the '#N' IID shown in URLs). Use this only when you have a global ID from another API response. For the common case of looking up issue #N in a project, use gitlab_issue_get with the IID instead.\n\nReturns: JSON with issue details. See also: gitlab_issue_get."), func(ctx context.Context, req *mcp.CallToolRequest, input GetByIDInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetByID(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_issue_get_by_id", start, nil)
			return toolutil.NotFoundResult("Issue", fmt.Sprintf("global ID %d", input.IssueID),
				"Use gitlab_issue_list with project_id to find issues",
				"Verify the global issue ID is correct (not the '#N' IID)",
				"For project-scoped IIDs, use gitlab_issue_get instead",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_get_by_id", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_reorder", "Reorder an issue by specifying the issue to position it before or after. Use move_after_id and/or move_before_id to set the relative position.\n\nReturns: JSON with the reordered issue details. See also: gitlab_issue_list."), func(ctx context.Context, req *mcp.CallToolRequest, input ReorderInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Reorder(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_reorder", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_move", "Move an issue from one project to another. Requires at least Reporter access on both the source and target projects.\n\nReturns: JSON with the moved issue details. See also: gitlab_issue_get, gitlab_project_list."), func(ctx context.Context, req *mcp.CallToolRequest, input MoveInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Move(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_move", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_subscribe", "Subscribe the authenticated user to an issue to receive notifications on updates.\n\nReturns: JSON with the issue details. See also: gitlab_issue_get."), func(ctx context.Context, req *mcp.CallToolRequest, input SubscribeInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Subscribe(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_subscribe", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_unsubscribe", "Unsubscribe the authenticated user from an issue to stop receiving notifications.\n\nReturns: JSON with the issue details. See also: gitlab_issue_get."), func(ctx context.Context, req *mcp.CallToolRequest, input UnsubscribeInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Unsubscribe(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_unsubscribe", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_create_todo", "Create a to-do item for the authenticated user on the specified issue. The to-do will appear in the user's GitLab to-do list.\n\nReturns: JSON with the created to-do item.\n\nSee also: gitlab_issue_get, gitlab_todo_list."), func(ctx context.Context, req *mcp.CallToolRequest, input CreateTodoInput) (*mcp.CallToolResult, TodoOutput, error) {
		start := time.Now()
		out, err := CreateTodo(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_create_todo", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTodoMarkdown(out)), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_time_estimate_set", "Set the time estimate for an issue using a human-readable duration (e.g. 3h30m, 1w2d).\n\nReturns: JSON with updated time tracking statistics. See also: gitlab_issue_get."), func(ctx context.Context, req *mcp.CallToolRequest, input SetTimeEstimateInput) (*mcp.CallToolResult, TimeStatsOutput, error) {
		start := time.Now()
		out, err := SetTimeEstimate(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_time_estimate_set", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTimeStatsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_time_estimate_reset", "Reset the time estimate for an issue back to zero.\n\nReturns: JSON with reset time tracking statistics. See also: gitlab_issue_get."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, TimeStatsOutput, error) {
		start := time.Now()
		out, err := ResetTimeEstimate(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_time_estimate_reset", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTimeStatsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_spent_time_add", "Add spent time to an issue using a human-readable duration (e.g. 1h, 30m) with an optional summary.\n\nReturns: JSON with updated time tracking statistics. See also: gitlab_issue_get."), func(ctx context.Context, req *mcp.CallToolRequest, input AddSpentTimeInput) (*mcp.CallToolResult, TimeStatsOutput, error) {
		start := time.Now()
		out, err := AddSpentTime(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_spent_time_add", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTimeStatsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_spent_time_reset", "Reset the total spent time for an issue to zero.\n\nReturns: JSON with reset time tracking statistics. See also: gitlab_issue_get."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, TimeStatsOutput, error) {
		start := time.Now()
		out, err := ResetSpentTime(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_spent_time_reset", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTimeStatsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_time_stats_get", "Get time tracking statistics for an issue (estimate and spent time).\n\nReturns: JSON with time tracking statistics including estimate and spent time. See also: gitlab_issue_get."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, TimeStatsOutput, error) {
		start := time.Now()
		out, err := GetTimeStats(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_time_stats_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTimeStatsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_participants", "List all participants (users who engaged) in an issue. Returns usernames, names, and profile URLs. See also: gitlab_issue_get, gitlab_project_members_list."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, ParticipantsOutput, error) {
		start := time.Now()
		out, err := GetParticipants(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_participants", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatParticipantsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_mrs_closing", "List merge requests that will close this issue when merged. Returns MR details including source/target branches. See also: gitlab_mr_get, gitlab_issue_mrs_related."), func(ctx context.Context, req *mcp.CallToolRequest, input ListMRsClosingInput) (*mcp.CallToolResult, RelatedMRsOutput, error) {
		start := time.Now()
		out, err := ListMRsClosing(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_mrs_closing", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRelatedMRsMarkdown(out, "MRs Closing Issue")), out, err)
	})

	mcp.AddTool(server, issueTool("gitlab_issue_mrs_related", "List merge requests related to this issue. Returns MR details including source/target branches. See also: gitlab_issue_get, gitlab_mr_list."), func(ctx context.Context, req *mcp.CallToolRequest, input ListMRsRelatedInput) (*mcp.CallToolResult, RelatedMRsOutput, error) {
		start := time.Now()
		out, err := ListMRsRelated(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_issue_mrs_related", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRelatedMRsMarkdown(out, "Related MRs")), out, err)
	})
}
