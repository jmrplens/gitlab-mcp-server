package mergerequests

import (
	"context"
	"fmt"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/progress"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers all merge request CRUD tools on the given MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_create",
		Title:       toolutil.TitleFromName("gitlab_mr_create"),
		Description: "Create a new merge request in a GitLab project. Requires source and target branch names. IMPORTANT: if the user does not specify a target branch, retrieve the project default branch via gitlab_project_get and use its default_branch value — do NOT assume 'main'. Supports title, Markdown description, assignee IDs, reviewer IDs, labels, milestone ID, allow_collaboration, and target_project_id (for cross-project/fork MRs). The squash and remove_source_branch options are omitted by default to preserve repository-level settings; only set them when the user explicitly requests it.\n\nReturns: JSON with the created merge request details including iid, title, state, source/target branches, author, and web_url. See also: gitlab_mr_get, gitlab_project_get, gitlab_branch_create.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_create", start, err)
		result := toolutil.ToolResultAnnotated(FormatMarkdown(out), toolutil.ContentMutate)
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_get",
		Title:       toolutil.TitleFromName("gitlab_mr_get"),
		Description: "Retrieve detailed information about a GitLab merge request by its IID (project-scoped ID), including title, description, state, source/target branches, author, assignees, reviewers, labels, and pipeline status. See also: gitlab_mr_changes_get, gitlab_mr_discussion_list.\n\nReturns: JSON with merge request details including iid, title, description, state, author, assignees, reviewers, labels, pipeline status, and merge status.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_mr_get", start, nil)
			return toolutil.NotFoundResult("Merge Request", fmt.Sprintf("!%d in project %s", input.MRIID, input.ProjectID),
				"Use gitlab_mr_list with project_id to list available merge requests",
				"Verify the merge request IID is correct for this project",
				"The merge request may have been deleted",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_get", start, err)
		result := toolutil.ToolResultAnnotated(FormatMarkdown(out), toolutil.ContentDetail)
		if err == nil && out.ProjectID > 0 && out.IID > 0 {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://project/%d/mr/%d", out.ProjectID, out.IID),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_list",
		Title:       toolutil.TitleFromName("gitlab_mr_list"),
		Description: "List merge requests in a GitLab project. Supports filtering by state (opened/closed/merged/all), author, assignee, reviewer, labels, milestone, and source/target branch. Returns paginated results.\n\nReturns: JSON array of merge requests with pagination. Fields include iid, title, state, author, source/target branches, and web_url. See also: gitlab_mr_get, gitlab_mr_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		tracker := progress.FromRequest(req)
		tracker.Step(ctx, 1, 2, "Fetching merge requests...")
		out, err := List(ctx, client, input)
		tracker.Step(ctx, 2, 2, toolutil.StepFormattingResponse)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_update",
		Title:       toolutil.TitleFromName("gitlab_mr_update"),
		Description: "Update a GitLab merge request's title, description, target branch, assignees, reviewers, labels (replace, add, or remove), milestone, discussion_locked, allow_collaboration, or state event (close/reopen). The squash and remove_source_branch options are omitted by default to preserve repository-level settings; only set them when explicitly requested. Only specified fields are changed.\n\nReturns: JSON with the updated merge request details. See also: gitlab_mr_get, gitlab_mr_merge.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_merge",
		Title:       toolutil.TitleFromName("gitlab_mr_merge"),
		Description: "Merge an accepted GitLab merge request into its target branch. Supports optional squash commits, custom merge commit message, and automatic source branch deletion after merge. The server automatically detects enforced project settings (squash_on_merge, force_remove_source_branch) and applies them — you do NOT need to set squash or should_remove_source_branch unless the user explicitly requests a specific value.\n\nReturns: JSON with the merged merge request details including final state and merge commit SHA. See also: gitlab_mr_get, gitlab_pipeline_list.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MergeInput) (*mcp.CallToolResult, Output, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Merge MR !%d in project %q? This action is irreversible.", input.MRIID, input.ProjectID)); r != nil {
			return r, Output{}, nil
		}
		start := time.Now()
		out, err := Merge(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_merge", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_approve",
		Title:       toolutil.TitleFromName("gitlab_mr_approve"),
		Description: "Approve a GitLab merge request. Adds the authenticated user's approval to the merge request's approval list.\n\nReturns: JSON with approval details including approved_by list. See also: gitlab_mr_get, gitlab_mr_approval_rules.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ApproveInput) (*mcp.CallToolResult, ApproveOutput, error) {
		start := time.Now()
		out, err := Approve(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_approve", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatApproveMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_unapprove",
		Title:       toolutil.TitleFromName("gitlab_mr_unapprove"),
		Description: "Remove the authenticated user's approval from a GitLab merge request.\n\nReturns: confirmation message. See also: gitlab_mr_get, gitlab_mr_approve.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ApproveInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Unapprove(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_unapprove", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("approval from MR !%d in project %s", input.MRIID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_commits",
		Title:       toolutil.TitleFromName("gitlab_mr_commits"),
		Description: "List all commits in a GitLab merge request. Returns commit ID, title, author, date, and web URL with pagination.\n\nReturns: JSON array of commits with pagination. Fields include id, title, author_name, authored_date, and web_url. See also: gitlab_mr_get, gitlab_commit_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CommitsInput) (*mcp.CallToolResult, CommitsOutput, error) {
		start := time.Now()
		out, err := Commits(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_commits", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatCommitsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_pipelines",
		Title:       toolutil.TitleFromName("gitlab_mr_pipelines"),
		Description: "List all pipelines associated with a GitLab merge request. Returns pipeline ID, status, source, ref, SHA, and web URL.\n\nReturns: JSON array of pipelines with pagination. Fields include id, status, source, ref, sha, and web_url. See also: gitlab_mr_get, gitlab_pipeline_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PipelinesInput) (*mcp.CallToolResult, PipelinesOutput, error) {
		start := time.Now()
		out, err := Pipelines(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_pipelines", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPipelinesMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_delete",
		Title:       toolutil.TitleFromName("gitlab_mr_delete"),
		Description: "Permanently delete a GitLab merge request. This action cannot be undone. Requires at least Maintainer access level.\n\nReturns: confirmation message. See also: gitlab_mr_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Permanently delete MR !%d in project %q? This action cannot be undone.", input.MRIID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("MR !%d from project %s", input.MRIID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_rebase",
		Title:       toolutil.TitleFromName("gitlab_mr_rebase"),
		Description: "Rebase a merge request's source branch against its target branch. Optionally skip triggering CI pipeline after rebase. Returns whether the rebase is in progress.\n\nReturns: JSON with rebase_in_progress status. See also: gitlab_mr_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RebaseInput) (*mcp.CallToolResult, RebaseOutput, error) {
		start := time.Now()
		out, err := Rebase(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_rebase", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRebaseMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_list_global",
		Title:       toolutil.TitleFromName("gitlab_mr_list_global"),
		Description: "List merge requests across all projects visible to the authenticated user. Supports filtering by state (opened/closed/merged/all), author, reviewer, labels, milestone, draft status, and date ranges. Returns paginated results.\n\nReturns: JSON array of merge requests with pagination. Fields include iid, title, state, author, source/target branches, and web_url. See also: gitlab_mr_list, gitlab_mr_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListGlobalInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		tracker := progress.FromRequest(req)
		tracker.Step(ctx, 1, 2, "Fetching global merge requests...")
		out, err := ListGlobal(ctx, client, input)
		tracker.Step(ctx, 2, 2, toolutil.StepFormattingResponse)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_list_global", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_list_group",
		Title:       toolutil.TitleFromName("gitlab_mr_list_group"),
		Description: "List merge requests in a GitLab group. Supports filtering by state (opened/closed/merged/all), author, reviewer, labels, milestone, draft status, and date ranges. Returns paginated results.\n\nReturns: JSON array of merge requests with pagination. Fields include iid, title, state, author, source/target branches, and web_url. See also: gitlab_mr_list, gitlab_mr_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		tracker := progress.FromRequest(req)
		tracker.Step(ctx, 1, 2, "Fetching group merge requests...")
		out, err := ListGroup(ctx, client, input)
		tracker.Step(ctx, 2, 2, toolutil.StepFormattingResponse)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_list_group", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_participants",
		Title:       toolutil.TitleFromName("gitlab_mr_participants"),
		Description: "List all participants (users who have interacted via comments, approvals, or commits) in a GitLab merge request. This includes anyone who participated, not just assigned reviewers. For assigned reviewers with review state, use gitlab_mr_reviewers instead.\n\nReturns: JSON array of participant users with id, username, name, and avatar_url. See also: gitlab_mr_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ParticipantsInput) (*mcp.CallToolResult, ParticipantsOutput, error) {
		start := time.Now()
		out, err := Participants(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_participants", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatParticipantsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_reviewers",
		Title:       toolutil.TitleFromName("gitlab_mr_reviewers"),
		Description: "List the explicitly assigned reviewers of a GitLab merge request with their review state (approved/reviewed/unreviewed) and assignment date. For all users who interacted (comments, commits, approvals), use gitlab_mr_participants instead.\n\nReturns: JSON array of reviewers with id, username, name, state, and created_at. See also: gitlab_mr_get, gitlab_mr_approve.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ParticipantsInput) (*mcp.CallToolResult, ReviewersOutput, error) {
		start := time.Now()
		out, err := Reviewers(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_reviewers", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatReviewersMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_create_pipeline",
		Title:       toolutil.TitleFromName("gitlab_mr_create_pipeline"),
		Description: "Create a new pipeline for a GitLab merge request. Triggers a CI/CD pipeline run on the MR's source branch. Returns the created pipeline details.\n\nReturns: JSON with the created pipeline details including id, status, ref, sha, and web_url. See also: gitlab_pipeline_get, gitlab_mr_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreatePipelineInput) (*mcp.CallToolResult, pipelines.Output, error) {
		start := time.Now()
		out, err := CreatePipeline(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_create_pipeline", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatCreatePipelineMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_issues_closed",
		Title:       toolutil.TitleFromName("gitlab_mr_issues_closed"),
		Description: "List all issues that would be closed when a GitLab merge request is merged. Returns issue details including IID, title, state, author, and labels with pagination.\n\nReturns: JSON array of issues with pagination. Fields include iid, title, state, author, and labels. See also: gitlab_mr_get, gitlab_issue_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input IssuesClosedInput) (*mcp.CallToolResult, IssuesClosedOutput, error) {
		start := time.Now()
		out, err := IssuesClosed(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_issues_closed", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatIssuesClosedMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_cancel_auto_merge",
		Title:       toolutil.TitleFromName("gitlab_mr_cancel_auto_merge"),
		Description: "Cancel the 'merge when pipeline succeeds' (auto-merge) setting on a GitLab merge request. Returns the updated merge request details. Requires appropriate permissions; returns 405 if already merged/closed or 406 if auto-merge was not enabled.\n\nReturns: JSON with the updated merge request details. See also: gitlab_mr_merge.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CancelAutoMerge(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_cancel_auto_merge", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	// Subscribe / Unsubscribe
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_subscribe",
		Title:       toolutil.TitleFromName("gitlab_mr_subscribe"),
		Description: "Subscribe to a GitLab merge request to receive notifications. Returns the updated MR. Returns 304 if already subscribed.\n\nReturns: JSON with the updated merge request details. See also: gitlab_mr_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Subscribe(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_subscribe", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_unsubscribe",
		Title:       toolutil.TitleFromName("gitlab_mr_unsubscribe"),
		Description: "Unsubscribe from a GitLab merge request to stop receiving notifications. Returns the updated MR. Returns 304 if not subscribed.\n\nReturns: JSON with the updated merge request details. See also: gitlab_mr_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Unsubscribe(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_unsubscribe", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	// Time Tracking
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_set_time_estimate",
		Title:       toolutil.TitleFromName("gitlab_mr_set_time_estimate"),
		Description: "Set the time estimate for a GitLab merge request using a human-readable duration string (e.g. '3h30m', '1w2d'). Returns updated time stats.\n\nReturns: JSON with time stats including time_estimate, total_time_spent, and human-readable formats. See also: gitlab_mr_time_stats.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetTimeEstimateInput) (*mcp.CallToolResult, TimeStatsOutput, error) {
		start := time.Now()
		out, err := SetTimeEstimate(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_set_time_estimate", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTimeStatsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_reset_time_estimate",
		Title:       toolutil.TitleFromName("gitlab_mr_reset_time_estimate"),
		Description: "Reset the time estimate for a GitLab merge request to zero. Returns updated time stats.\n\nReturns: JSON with time stats including time_estimate, total_time_spent, and human-readable formats. See also: gitlab_mr_time_stats.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, TimeStatsOutput, error) {
		start := time.Now()
		out, err := ResetTimeEstimate(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_reset_time_estimate", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTimeStatsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_add_spent_time",
		Title:       toolutil.TitleFromName("gitlab_mr_add_spent_time"),
		Description: "Add spent time to a GitLab merge request. Duration uses human-readable format (e.g. '1h', '30m', '1w2d'). Optional summary describes the work done. Returns updated time stats.\n\nReturns: JSON with time stats including time_estimate, total_time_spent, and human-readable formats. See also: gitlab_mr_time_stats.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddSpentTimeInput) (*mcp.CallToolResult, TimeStatsOutput, error) {
		start := time.Now()
		out, err := AddSpentTime(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_add_spent_time", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTimeStatsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_reset_spent_time",
		Title:       toolutil.TitleFromName("gitlab_mr_reset_spent_time"),
		Description: "Reset the total spent time for a GitLab merge request to zero. Returns updated time stats.\n\nReturns: JSON with time stats including time_estimate, total_time_spent, and human-readable formats. See also: gitlab_mr_time_stats.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, TimeStatsOutput, error) {
		start := time.Now()
		out, err := ResetSpentTime(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_reset_spent_time", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTimeStatsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_time_stats",
		Title:       toolutil.TitleFromName("gitlab_mr_time_stats"),
		Description: "Get time tracking statistics for a GitLab merge request including estimated time and total time spent in both human-readable and seconds format.\n\nReturns: JSON with time stats including time_estimate, total_time_spent, and human-readable formats. See also: gitlab_mr_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, TimeStatsOutput, error) {
		start := time.Now()
		out, err := GetTimeStats(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_time_stats", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTimeStatsMarkdown(out)), out, err)
	})

	// Related Issues
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_related_issues",
		Title:       toolutil.TitleFromName("gitlab_mr_related_issues"),
		Description: "List all issues related to a GitLab merge request (mentioned or linked). Returns issue details including IID, title, state, author, and labels with pagination.\n\nReturns: JSON array of related issues with pagination. Fields include iid, title, state, author, and labels. See also: gitlab_mr_get, gitlab_issue_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RelatedIssuesInput) (*mcp.CallToolResult, RelatedIssuesOutput, error) {
		start := time.Now()
		tracker := progress.FromRequest(req)
		tracker.Step(ctx, 1, 2, "Fetching related issues...")
		out, err := RelatedIssues(ctx, client, input)
		tracker.Step(ctx, 2, 2, toolutil.StepFormattingResponse)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_related_issues", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRelatedIssuesMarkdown(out)), out, err)
	})

	// Create Todo
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_create_todo",
		Title:       toolutil.TitleFromName("gitlab_mr_create_todo"),
		Description: "Create a to-do item on a GitLab merge request for the authenticated user. Adds the MR to the user's to-do list for later follow-up.\n\nReturns: JSON with the created to-do item details including id, target type, and action name. See also: gitlab_mr_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateTodoInput) (*mcp.CallToolResult, CreateTodoOutput, error) {
		start := time.Now()
		out, err := CreateTodo(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_create_todo", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatCreateTodoMarkdown(out)), out, err)
	})

	// MR Dependencies
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_dependency_create",
		Title:       toolutil.TitleFromName("gitlab_mr_dependency_create"),
		Description: "Create a merge request dependency (blocker). The specified blocking MR must be merged before this MR can be merged. Requires Premium or Ultimate license.\n\nReturns: JSON with the dependency details including blocking merge request information. See also: gitlab_mr_dependencies_list.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DependencyInput) (*mcp.CallToolResult, DependencyOutput, error) {
		start := time.Now()
		out, err := CreateDependency(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_dependency_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDependencyMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_dependency_delete",
		Title:       toolutil.TitleFromName("gitlab_mr_dependency_delete"),
		Description: "Remove a merge request dependency (blocker). The specified blocking MR will no longer prevent this MR from being merged. Requires Premium or Ultimate license.\n\nReturns: confirmation message. See also: gitlab_mr_dependencies_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteDependencyInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := DeleteDependency(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_dependency_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("dependency on blocking MR %d from MR !%d in project %s", input.BlockingMergeRequestID, input.MRIID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_dependencies_list",
		Title:       toolutil.TitleFromName("gitlab_mr_dependencies_list"),
		Description: "List all merge request dependencies (blockers) for a GitLab merge request. Returns the list of MRs that must be merged before this MR can be merged. Requires Premium or Ultimate license.\n\nReturns: JSON array of blocking merge requests with iid, title, state, and web_url. See also: gitlab_mr_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetDependenciesInput) (*mcp.CallToolResult, DependenciesOutput, error) {
		start := time.Now()
		out, err := GetDependencies(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_dependencies_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDependenciesMarkdown(out)), out, err)
	})
}
