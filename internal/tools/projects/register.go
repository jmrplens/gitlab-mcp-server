package projects

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers CRUD tools for GitLab projects.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	registerCRUDTools(server, client)
	registerProjectActionTools(server, client)
	registerWebhookTools(server, client)
	registerProjectMembershipTools(server, client)
	registerUserScopedTools(server, client)
	registerWebhookCustomizationTools(server, client)
	registerForkRelationTools(server, client)
	registerAvatarTools(server, client)
	registerApprovalConfigTools(server, client)
	registerApprovalRuleTools(server, client)
	registerPullMirrorTools(server, client)
	registerMaintenanceTools(server, client)
	registerAdminProjectTools(server, client)
}

// registerCRUDTools is an internal helper for the projects package.
func registerCRUDTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_create",
		Title:       toolutil.TitleFromName("gitlab_project_create"),
		Description: "Create a new GitLab project (repository). Supports setting namespace, visibility (private/internal/public), description, default branch, optional README initialization, merge method, squash option, topics, and feature flags (issues_enabled, merge_requests_enabled, wiki_enabled, jobs_enabled, lfs_enabled, request_access_enabled). Also supports CI/CD config path, allow_merge_on_skipped_pipeline, remove_source_branch_after_merge, and autoclose_referenced_issues.\n\nReturns: JSON with the created project details including id, name, path_with_namespace, web_url, visibility, and namespace. See also: gitlab_project_get, gitlab_project_list.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_create", start, err)
		result := toolutil.ToolResultAnnotated(FormatMarkdown(out), toolutil.ContentMutate)
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_get",
		Title:       toolutil.TitleFromName("gitlab_project_get"),
		Description: "Retrieve detailed metadata for a GitLab project including name, description, visibility, web URL, default branch, and namespace. Accepts numeric project ID or URL-encoded path (e.g. 'group/subgroup/project'). The response includes default_branch which MUST be used instead of assuming 'main' when generating repository file URLs. Set statistics=true to include repository statistics, license=true to include license info, with_custom_attributes=true to include custom attributes.\n\nReturns: JSON with project details including id, name, description, visibility, web_url, default_branch, namespace, and statistics (if requested). See also: gitlab_project_update, gitlab_project_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_project_get", start, nil)
			return toolutil.NotFoundResult("Project", string(input.ProjectID),
				"Use gitlab_project_list to search for projects by name or path",
				"Verify the project ID or URL-encoded path is correct (e.g. 'group%2Fproject')",
				"The project may have been deleted or you may lack access",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_get", start, err)
		result := toolutil.ToolResultAnnotated(FormatMarkdown(out), toolutil.ContentDetail)
		if err == nil && out.ID > 0 {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://project/%d", out.ID),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_list",
		Title:       toolutil.TitleFromName("gitlab_project_list"),
		Description: "List GitLab projects accessible to the authenticated user. Supports filtering by ownership, search term, visibility, archived status, topic, minimum access level, starred, membership, last_activity_after/before dates, and feature flags (with_issues_enabled, with_merge_requests_enabled). Set include_pending_delete=true to include projects that are marked/scheduled for deletion (hidden by default). Supports ordering, sorting, simple mode, search_namespaces, and statistics.\n\nReturns: JSON array of projects with pagination. Fields include id, name, path_with_namespace, visibility, web_url, and default_branch. See also: gitlab_project_get, gitlab_project_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_delete",
		Title:       toolutil.TitleFromName("gitlab_project_delete"),
		Description: "Delete a GitLab project. On instances with delayed deletion enabled, the project is marked for deletion (scheduled) rather than removed immediately — the response includes the scheduled deletion date. Set permanently_remove=true with full_path to bypass delayed deletion and permanently remove the project immediately (requires admin on some instances). Use gitlab_project_restore to cancel a scheduled deletion. Use gitlab_project_list with include_pending_delete=true to see projects awaiting deletion.\n\nReturns: confirmation message with scheduled deletion date if delayed deletion is enabled. See also: gitlab_project_create, gitlab_project_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, DeleteOutput, error) {
		msg := fmt.Sprintf("Delete project %q?", input.ProjectID)
		if input.PermanentlyRemove {
			msg = fmt.Sprintf("Permanently delete project %q? This action cannot be undone.", input.ProjectID)
		}
		if r := toolutil.ConfirmAction(ctx, req, msg); r != nil {
			return r, DeleteOutput{}, nil
		}
		start := time.Now()
		out, err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_delete", start, err)
		if err != nil {
			return nil, DeleteOutput{}, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDeleteMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_restore",
		Title:       toolutil.TitleFromName("gitlab_project_restore"),
		Description: "Restore a GitLab project that has been marked/scheduled for deletion. Use gitlab_project_list with include_pending_delete=true to discover projects pending deletion.\n\nReturns: JSON with the restored project details including id, name, web_url, and visibility.\n\nSee also: gitlab_project_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RestoreInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Restore(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_restore", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_update",
		Title:       toolutil.TitleFromName("gitlab_project_update"),
		Description: "Update GitLab project settings such as name, description, visibility, default branch, merge method, squash option, topics, and feature flags (issues_enabled, merge_requests_enabled, wiki_enabled, jobs_enabled, lfs_enabled). Also supports CI/CD config path, merge commit/squash commit templates, merge_pipelines_enabled, merge_trains_enabled, allow_merge_on_skipped_pipeline, remove_source_branch_after_merge, autoclose_referenced_issues, resolve_outdated_diff_discussions, and approvals_before_merge. Only specified fields are modified; unset fields remain unchanged.\n\nReturns: JSON with the updated project details. See also: gitlab_project_get, gitlab_project_delete.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})
}

// registerProjectActionTools is an internal helper for the projects package.
func registerProjectActionTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_fork",
		Title:       toolutil.TitleFromName("gitlab_project_fork"),
		Description: "Fork a GitLab project into a new project. Optionally specify target namespace (namespace_id or namespace_path), name, path, description, visibility, branches to include, and whether MR default target should be the fork itself (mr_default_target_self).\n\nReturns: JSON with the forked project details including id, name, web_url, and forked_from_project. See also: gitlab_project_get, gitlab_branch_create.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ForkInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Fork(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_fork", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_star",
		Title:       toolutil.TitleFromName("gitlab_project_star"),
		Description: "Star a GitLab project for the authenticated user.\n\nReturns: JSON with updated project details including incremented star_count. See also: gitlab_project_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StarInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Star(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_star", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_unstar",
		Title:       toolutil.TitleFromName("gitlab_project_unstar"),
		Description: "Remove star from a GitLab project for the authenticated user.\n\nReturns: JSON with updated project details including decremented star_count. See also: gitlab_project_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UnstarInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Unstar(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_unstar", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_archive",
		Title:       toolutil.TitleFromName("gitlab_project_archive"),
		Description: "Archive a GitLab project, making it read-only. Archived projects are hidden from default project list.\n\nReturns: JSON with updated project details. See also: gitlab_project_unarchive, gitlab_project_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ArchiveInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Archive(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_archive", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_unarchive",
		Title:       toolutil.TitleFromName("gitlab_project_unarchive"),
		Description: "Unarchive a GitLab project, restoring it from read-only state.\n\nReturns: JSON with updated project details. See also: gitlab_project_archive, gitlab_project_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UnarchiveInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Unarchive(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_unarchive", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_transfer",
		Title:       toolutil.TitleFromName("gitlab_project_transfer"),
		Description: "Transfer a GitLab project to a different namespace. Requires the namespace (ID or path) to transfer to.\n\nReturns: JSON with updated project details including new path and namespace. See also: gitlab_project_get, gitlab_group_list.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input TransferInput) (*mcp.CallToolResult, Output, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Transfer project %q to namespace %q?", input.ProjectID, input.Namespace)); r != nil {
			return r, Output{}, nil
		}
		start := time.Now()
		out, err := Transfer(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_transfer", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_list_forks",
		Title:       toolutil.TitleFromName("gitlab_project_list_forks"),
		Description: "List forks of a GitLab project. Supports filtering by ownership, search, visibility, ordering, and pagination.\n\nReturns: JSON array of forked projects with pagination. Fields include id, name, path_with_namespace, and web_url. See also: gitlab_project_fork, gitlab_project_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListForksInput) (*mcp.CallToolResult, ListForksOutput, error) {
		start := time.Now()
		out, err := ListForks(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_list_forks", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListForksMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_languages",
		Title:       toolutil.TitleFromName("gitlab_project_languages"),
		Description: "List programming languages used in a GitLab project with their percentages.\n\nReturns: JSON object mapping language names to their percentage of the codebase. See also: gitlab_project_get, gitlab_repository_tree.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetLanguagesInput) (*mcp.CallToolResult, LanguagesOutput, error) {
		start := time.Now()
		out, err := GetLanguages(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_languages", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatLanguagesMarkdown(out)), out, err)
	})
}

// registerWebhookTools is an internal helper for the projects package.
func registerWebhookTools(server *mcp.Server, client *gitlabclient.Client) {
	// Webhook tools.

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_hook_list",
		Title:       toolutil.TitleFromName("gitlab_project_hook_list"),
		Description: "List webhooks configured for a GitLab project.\n\nReturns: JSON array of webhooks with pagination. Fields include id, url, created_at, and event trigger settings. See also: gitlab_project_hook_get, gitlab_project_hook_add.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListHooksInput) (*mcp.CallToolResult, ListHooksOutput, error) {
		start := time.Now()
		out, err := ListHooks(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_hook_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListHooksMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_hook_get",
		Title:       toolutil.TitleFromName("gitlab_project_hook_get"),
		Description: "Get details of a specific project webhook including all event trigger settings.\n\nReturns: JSON with webhook details including id, url, created_at, and all event trigger settings. See also: gitlab_project_hook_list, gitlab_project_hook_edit.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetHookInput) (*mcp.CallToolResult, HookOutput, error) {
		start := time.Now()
		out, err := GetHook(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_hook_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatHookMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_hook_add",
		Title:       toolutil.TitleFromName("gitlab_project_hook_add"),
		Description: "Add a webhook to a GitLab project. Configure the URL, secret token, SSL verification, and which events trigger the webhook (push, issues, MRs, tags, notes, jobs, pipelines, wiki, deployments, releases, emoji, etc.).\n\nReturns: JSON with the created webhook details including id, url, and event settings. See also: gitlab_project_hook_list, gitlab_project_hook_edit.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddHookInput) (*mcp.CallToolResult, HookOutput, error) {
		start := time.Now()
		out, err := AddHook(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_hook_add", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatHookMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_hook_edit",
		Title:       toolutil.TitleFromName("gitlab_project_hook_edit"),
		Description: "Edit an existing project webhook. Update the URL, events, SSL verification, secret token, or other settings.\n\nReturns: JSON with the updated webhook details. See also: gitlab_project_hook_get, gitlab_project_hook_delete.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EditHookInput) (*mcp.CallToolResult, HookOutput, error) {
		start := time.Now()
		out, err := EditHook(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_hook_edit", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatHookMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_hook_delete",
		Title:       toolutil.TitleFromName("gitlab_project_hook_delete"),
		Description: "Delete a webhook from a GitLab project.\n\nReturns: confirmation message. See also: gitlab_project_hook_list, gitlab_project_hook_add.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteHookInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete webhook %d from project %s?", input.HookID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := DeleteHook(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_hook_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("webhook %d from project %s", input.HookID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_hook_test",
		Title:       toolutil.TitleFromName("gitlab_project_hook_test"),
		Description: "Trigger a test event for a project webhook. Sends a sample payload for the specified event type (push_events, issues_events, merge_requests_events, etc.).\n\nReturns: confirmation message indicating the test event was sent. See also: gitlab_project_hook_get, gitlab_project_hook_list.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input TriggerTestHookInput) (*mcp.CallToolResult, TriggerTestHookOutput, error) {
		start := time.Now()
		out, err := TriggerTestHook(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_hook_test", start, err)
		if err != nil {
			return nil, TriggerTestHookOutput{}, err
		}
		md := fmt.Sprintf(toolutil.EmojiSuccess+" %s", out.Message)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(md), out, nil)
	})
}

// registerProjectMembershipTools is an internal helper for the projects package.
func registerProjectMembershipTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_list_user_projects",
		Title:       toolutil.TitleFromName("gitlab_project_list_user_projects"),
		Description: "List projects owned by a specific user. Accepts user ID or username. Supports filtering by search, visibility, archived status, and pagination.\n\nReturns: JSON array of projects with pagination. Fields include id, name, path_with_namespace, visibility, and web_url.\n\nSee also: gitlab_project_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListUserProjectsInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListUserProjects(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_list_user_projects", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_list_users",
		Title:       toolutil.TitleFromName("gitlab_project_list_users"),
		Description: "List users who are members of a project. Supports filtering by search (name or username) and pagination.\n\nReturns: JSON array of project users with pagination. Fields include id, username, name, and state.\n\nSee also: gitlab_project_members_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectUsersInput) (*mcp.CallToolResult, ListProjectUsersOutput, error) {
		start := time.Now()
		out, err := ListProjectUsers(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_list_users", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListProjectUsersMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_list_groups",
		Title:       toolutil.TitleFromName("gitlab_project_list_groups"),
		Description: "List ancestor groups of a project. Supports filtering by search, shared groups, minimum access level, skip_groups, and pagination.\n\nReturns: JSON array of groups with pagination. Fields include id, name, path, and visibility.\n\nSee also: gitlab_project_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectGroupsInput) (*mcp.CallToolResult, ListProjectGroupsOutput, error) {
		start := time.Now()
		out, err := ListProjectGroups(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_list_groups", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListProjectGroupsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_list_starrers",
		Title:       toolutil.TitleFromName("gitlab_project_list_starrers"),
		Description: "List users who have starred a project. Supports filtering by search (name or username) and pagination.\n\nReturns: JSON array of users who starred the project with pagination. Fields include starred_since and user details.\n\nSee also: gitlab_project_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectStarrersInput) (*mcp.CallToolResult, ListProjectStarrersOutput, error) {
		start := time.Now()
		out, err := ListProjectStarrers(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_list_starrers", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListStarrersMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_share_with_group",
		Title:       toolutil.TitleFromName("gitlab_project_share_with_group"),
		Description: "Share a project with a group, granting the specified access level. Optionally set an expiration date (YYYY-MM-DD). Access levels: 10=Guest, 20=Reporter, 30=Developer, 40=Maintainer.\n\nReturns: JSON with the group share details including group_id, group_access, and expires_at. See also: gitlab_project_delete_shared_group, gitlab_group_list.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ShareProjectInput) (*mcp.CallToolResult, ShareProjectOutput, error) {
		start := time.Now()
		out, err := ShareProjectWithGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_share_with_group", start, err)
		if err != nil {
			return nil, ShareProjectOutput{}, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatShareProjectMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_delete_shared_group",
		Title:       toolutil.TitleFromName("gitlab_project_delete_shared_group"),
		Description: "Remove a shared group from a project, revoking the group's access.\n\nReturns: confirmation message. See also: gitlab_project_share_with_group.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteSharedGroupInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Remove group %d from project %s?", input.GroupID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := DeleteSharedProjectFromGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_delete_shared_group", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("shared group %d from project %s", input.GroupID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_list_invited_groups",
		Title:       toolutil.TitleFromName("gitlab_project_list_invited_groups"),
		Description: "List groups that have been invited/shared to a project. Supports filtering by search, minimum access level, and pagination.\n\nReturns: JSON array of invited groups with pagination. Fields include id, name, path, and visibility.\n\nSee also: gitlab_project_list_groups.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInvitedGroupsInput) (*mcp.CallToolResult, ListProjectGroupsOutput, error) {
		start := time.Now()
		out, err := ListInvitedGroups(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_list_invited_groups", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListProjectGroupsMarkdown(out)), out, err)
	})
}

// registerUserScopedTools is an internal helper for user-scoped project listings.
func registerUserScopedTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_list_user_contributed",
		Title:       toolutil.TitleFromName("gitlab_project_list_user_contributed"),
		Description: "List projects that a specific user has contributed to. Supports filtering by search, visibility, archived status, and pagination.\n\nReturns: JSON array of projects with pagination. Fields include id, name, path_with_namespace, visibility, and web_url.\n\nSee also: gitlab_project_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListUserContributedProjectsInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListUserContributedProjects(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_list_user_contributed", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_list_user_starred",
		Title:       toolutil.TitleFromName("gitlab_project_list_user_starred"),
		Description: "List projects that a specific user has starred. Supports filtering by search, visibility, archived status, and pagination.\n\nReturns: JSON array of projects with pagination. Fields include id, name, path_with_namespace, visibility, and web_url.\n\nSee also: gitlab_project_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListUserStarredProjectsInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListUserStarredProjects(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_list_user_starred", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})
}

// RegisterPushRuleTools registers push rule tools (Premium/Ultimate only).
// Called separately from RegisterAll when enterprise mode is enabled.
func RegisterPushRuleTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_get_push_rules",
		Title:       toolutil.TitleFromName("gitlab_project_get_push_rules"),
		Description: "Get the push rule configuration for a project (commit message, branch name, file size restrictions, etc.).\n\nReturns: JSON with push rule configuration including commit_message_regex, branch_name_regex, max_file_size, and signing requirements.\n\nSee also: gitlab_project_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetPushRulesInput) (*mcp.CallToolResult, PushRuleOutput, error) {
		start := time.Now()
		out, err := GetPushRules(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_get_push_rules", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPushRuleMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_add_push_rule",
		Title:       toolutil.TitleFromName("gitlab_project_add_push_rule"),
		Description: "Add push rule configuration to a project. Enforce commit message format, branch naming, file size limits, secret detection, and signed commits.\n\nReturns: JSON with the created push rule configuration.\n\nSee also: gitlab_project_get_push_rules.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddPushRuleInput) (*mcp.CallToolResult, PushRuleOutput, error) {
		start := time.Now()
		out, err := AddPushRule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_add_push_rule", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPushRuleMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_edit_push_rule",
		Title:       toolutil.TitleFromName("gitlab_project_edit_push_rule"),
		Description: "Modify the push rule configuration of a project. Update commit message, branch name, file restrictions, or signing requirements.\n\nReturns: JSON with the updated push rule configuration.\n\nSee also: gitlab_project_get_push_rules.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EditPushRuleInput) (*mcp.CallToolResult, PushRuleOutput, error) {
		start := time.Now()
		out, err := EditPushRule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_edit_push_rule", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatPushRuleMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_delete_push_rule",
		Title:       toolutil.TitleFromName("gitlab_project_delete_push_rule"),
		Description: "Delete the push rule configuration from a project. This removes all push restrictions (commit format, branch naming, file size, etc.).\n\nReturns: confirmation message.\n\nSee also: gitlab_project_get_push_rules.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeletePushRuleInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete push rules for project %s?", input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := DeletePushRule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_delete_push_rule", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("push rules for project %s", input.ProjectID))
	})
}

// registerWebhookCustomizationTools registers webhook header and URL variable tools.
func registerWebhookCustomizationTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_hook_set_custom_header",
		Title:       toolutil.TitleFromName("gitlab_project_hook_set_custom_header"),
		Description: "Set a custom header on a project webhook. The header will be included in all webhook requests. If the header already exists, its value will be updated.\n\nReturns: confirmation message.\n\nSee also: gitlab_project_hook_get, gitlab_project_hook_delete_custom_header.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetCustomHeaderInput) (*mcp.CallToolResult, toolutil.VoidOutput, error) {
		start := time.Now()
		err := SetCustomHeader(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_hook_set_custom_header", start, err)
		if err != nil {
			return nil, toolutil.VoidOutput{}, err
		}
		return toolutil.VoidResult(fmt.Sprintf("Custom header %q set on webhook %d in project %s", input.Key, input.HookID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_hook_delete_custom_header",
		Title:       toolutil.TitleFromName("gitlab_project_hook_delete_custom_header"),
		Description: "Delete a custom header from a project webhook.\n\nReturns: confirmation message.\n\nSee also: gitlab_project_hook_get, gitlab_project_hook_set_custom_header.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteCustomHeaderInput) (*mcp.CallToolResult, toolutil.VoidOutput, error) {
		start := time.Now()
		err := DeleteCustomHeader(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_hook_delete_custom_header", start, err)
		if err != nil {
			return nil, toolutil.VoidOutput{}, err
		}
		return toolutil.VoidResult(fmt.Sprintf("Custom header %q deleted from webhook %d in project %s", input.Key, input.HookID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_hook_set_url_variable",
		Title:       toolutil.TitleFromName("gitlab_project_hook_set_url_variable"),
		Description: "Set a URL variable on a project webhook. URL variables can be used in the webhook URL as {variable_name}. If already exists, its value will be updated.\n\nReturns: confirmation message.\n\nSee also: gitlab_project_hook_get, gitlab_project_hook_delete_url_variable.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetWebhookURLVariableInput) (*mcp.CallToolResult, toolutil.VoidOutput, error) {
		start := time.Now()
		err := SetWebhookURLVariable(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_hook_set_url_variable", start, err)
		if err != nil {
			return nil, toolutil.VoidOutput{}, err
		}
		return toolutil.VoidResult(fmt.Sprintf("URL variable %q set on webhook %d in project %s", input.Key, input.HookID, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_hook_delete_url_variable",
		Title:       toolutil.TitleFromName("gitlab_project_hook_delete_url_variable"),
		Description: "Delete a URL variable from a project webhook.\n\nReturns: confirmation message.\n\nSee also: gitlab_project_hook_get, gitlab_project_hook_set_url_variable.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteWebhookURLVariableInput) (*mcp.CallToolResult, toolutil.VoidOutput, error) {
		start := time.Now()
		err := DeleteWebhookURLVariable(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_hook_delete_url_variable", start, err)
		if err != nil {
			return nil, toolutil.VoidOutput{}, err
		}
		return toolutil.VoidResult(fmt.Sprintf("URL variable %q deleted from webhook %d in project %s", input.Key, input.HookID, input.ProjectID))
	})
}

// registerForkRelationTools registers fork relation CRUD tools.
func registerForkRelationTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_create_fork_relation",
		Title:       toolutil.TitleFromName("gitlab_project_create_fork_relation"),
		Description: "Create a fork relation between two existing projects. This makes the project appear as forked from the specified source project.\n\nReturns: JSON with fork relation details including forked_to_project_id and forked_from_project_id.\n\nSee also: gitlab_project_fork, gitlab_project_delete_fork_relation.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateForkRelationInput) (*mcp.CallToolResult, ForkRelationOutput, error) {
		start := time.Now()
		out, err := CreateForkRelation(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_create_fork_relation", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatForkRelationMarkdown(out), toolutil.ContentMutate), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_delete_fork_relation",
		Title:       toolutil.TitleFromName("gitlab_project_delete_fork_relation"),
		Description: "Remove the fork relation from a project. The project will no longer be marked as a fork.\n\nReturns: confirmation message.\n\nSee also: gitlab_project_create_fork_relation.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteForkRelationInput) (*mcp.CallToolResult, toolutil.VoidOutput, error) {
		start := time.Now()
		err := DeleteForkRelation(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_delete_fork_relation", start, err)
		if err != nil {
			return nil, toolutil.VoidOutput{}, err
		}
		return toolutil.VoidResult(fmt.Sprintf("Fork relation removed from project %s", input.ProjectID))
	})
}

// registerAvatarTools registers project avatar upload/download tools.
func registerAvatarTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_upload_avatar",
		Title:       toolutil.TitleFromName("gitlab_project_upload_avatar"),
		Description: "Upload or replace the avatar image for a project. Provide either file_path (absolute path to a local image file) or content_base64 (base64-encoded image content), not both.\n\nReturns: JSON with updated project details including avatar_url.\n\nSee also: gitlab_project_get, gitlab_project_download_avatar.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UploadAvatarInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := UploadAvatar(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_upload_avatar", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_download_avatar",
		Title:       toolutil.TitleFromName("gitlab_project_download_avatar"),
		Description: "Download the avatar image for a project as base64-encoded data.\n\nReturns: JSON with content_base64 (base64-encoded image) and size_bytes.\n\nSee also: gitlab_project_get, gitlab_project_upload_avatar.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DownloadAvatarInput) (*mcp.CallToolResult, DownloadAvatarOutput, error) {
		start := time.Now()
		out, err := DownloadAvatar(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_download_avatar", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatDownloadAvatarMarkdown(out), toolutil.ContentDetail), out, err)
	})
}

// registerApprovalConfigTools registers approval configuration tools.
func registerApprovalConfigTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_approval_config_get",
		Title:       toolutil.TitleFromName("gitlab_project_approval_config_get"),
		Description: "Get the project-level merge request approval configuration including required approvals, author self-approval, committer approval, and password requirements.\n\nReturns: JSON with approval settings.\n\nSee also: gitlab_project_approval_config_change, gitlab_project_approval_rule_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetApprovalConfigInput) (*mcp.CallToolResult, ApprovalConfigOutput, error) {
		start := time.Now()
		out, err := GetApprovalConfig(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_approval_config_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatApprovalConfigMarkdown(out), toolutil.ContentDetail), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_approval_config_change",
		Title:       toolutil.TitleFromName("gitlab_project_approval_config_change"),
		Description: "Update the project-level merge request approval configuration. Only specified fields are modified.\n\nReturns: JSON with updated approval settings.\n\nSee also: gitlab_project_approval_config_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ChangeApprovalConfigInput) (*mcp.CallToolResult, ApprovalConfigOutput, error) {
		start := time.Now()
		out, err := ChangeApprovalConfig(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_approval_config_change", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatApprovalConfigMarkdown(out), toolutil.ContentMutate), out, err)
	})
}

// registerApprovalRuleTools registers approval rule CRUD tools.
func registerApprovalRuleTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_approval_rule_list",
		Title:       toolutil.TitleFromName("gitlab_project_approval_rule_list"),
		Description: "List project-level approval rules. Each rule specifies the number of required approvals and eligible approvers.\n\nReturns: JSON array of approval rules with pagination.\n\nSee also: gitlab_project_approval_rule_get, gitlab_project_approval_rule_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListApprovalRulesInput) (*mcp.CallToolResult, ListApprovalRulesOutput, error) {
		start := time.Now()
		out, err := ListApprovalRules(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_approval_rule_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListApprovalRulesMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_approval_rule_get",
		Title:       toolutil.TitleFromName("gitlab_project_approval_rule_get"),
		Description: "Get details of a specific project-level approval rule.\n\nReturns: JSON with approval rule details including name, approvals_required, eligible approvers, and protected branches.\n\nSee also: gitlab_project_approval_rule_list, gitlab_project_approval_rule_update.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetApprovalRuleInput) (*mcp.CallToolResult, ApprovalRuleOutput, error) {
		start := time.Now()
		out, err := GetApprovalRule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_approval_rule_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatApprovalRuleMarkdown(out), toolutil.ContentDetail), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_approval_rule_create",
		Title:       toolutil.TitleFromName("gitlab_project_approval_rule_create"),
		Description: "Create a new project-level approval rule with the specified number of required approvals and approver groups/users.\n\nReturns: JSON with the created approval rule details.\n\nSee also: gitlab_project_approval_rule_list, gitlab_project_approval_rule_update.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateApprovalRuleInput) (*mcp.CallToolResult, ApprovalRuleOutput, error) {
		start := time.Now()
		out, err := CreateApprovalRule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_approval_rule_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatApprovalRuleMarkdown(out), toolutil.ContentMutate), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_approval_rule_update",
		Title:       toolutil.TitleFromName("gitlab_project_approval_rule_update"),
		Description: "Update an existing project-level approval rule. Only specified fields are modified.\n\nReturns: JSON with the updated approval rule details.\n\nSee also: gitlab_project_approval_rule_get, gitlab_project_approval_rule_delete.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateApprovalRuleInput) (*mcp.CallToolResult, ApprovalRuleOutput, error) {
		start := time.Now()
		out, err := UpdateApprovalRule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_approval_rule_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatApprovalRuleMarkdown(out), toolutil.ContentMutate), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_approval_rule_delete",
		Title:       toolutil.TitleFromName("gitlab_project_approval_rule_delete"),
		Description: "Delete a project-level approval rule.\n\nReturns: confirmation message.\n\nSee also: gitlab_project_approval_rule_list, gitlab_project_approval_rule_create.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteApprovalRuleInput) (*mcp.CallToolResult, toolutil.VoidOutput, error) {
		start := time.Now()
		err := DeleteApprovalRule(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_approval_rule_delete", start, err)
		if err != nil {
			return nil, toolutil.VoidOutput{}, err
		}
		return toolutil.VoidResult(fmt.Sprintf("Approval rule %d deleted from project %s", input.RuleID, input.ProjectID))
	})
}

// registerPullMirrorTools registers pull mirror management tools.
func registerPullMirrorTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_pull_mirror_get",
		Title:       toolutil.TitleFromName("gitlab_project_pull_mirror_get"),
		Description: "Get pull mirror configuration for a project including URL, status, last update times, and mirror settings.\n\nReturns: JSON with pull mirror details.\n\nSee also: gitlab_project_pull_mirror_configure, gitlab_project_start_mirroring.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetPullMirrorInput) (*mcp.CallToolResult, PullMirrorOutput, error) {
		start := time.Now()
		out, err := GetPullMirror(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_pull_mirror_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatPullMirrorMarkdown(out), toolutil.ContentDetail), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_pull_mirror_configure",
		Title:       toolutil.TitleFromName("gitlab_project_pull_mirror_configure"),
		Description: "Configure or update pull mirroring for a project. Set the mirror URL, authentication, branch filtering, and trigger options.\n\nReturns: JSON with updated pull mirror configuration.\n\nSee also: gitlab_project_pull_mirror_get, gitlab_project_start_mirroring.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ConfigurePullMirrorInput) (*mcp.CallToolResult, PullMirrorOutput, error) {
		start := time.Now()
		out, err := ConfigurePullMirror(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_pull_mirror_configure", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatPullMirrorMarkdown(out), toolutil.ContentMutate), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_start_mirroring",
		Title:       toolutil.TitleFromName("gitlab_project_start_mirroring"),
		Description: "Trigger an immediate pull mirror update for a project. The project must have pull mirroring configured.\n\nReturns: confirmation message.\n\nSee also: gitlab_project_pull_mirror_get, gitlab_project_pull_mirror_configure.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StartMirroringInput) (*mcp.CallToolResult, toolutil.VoidOutput, error) {
		start := time.Now()
		err := StartMirroring(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_start_mirroring", start, err)
		if err != nil {
			return nil, toolutil.VoidOutput{}, err
		}
		return toolutil.VoidResult(fmt.Sprintf("Mirror update triggered for project %s", input.ProjectID))
	})
}

// registerMaintenanceTools registers housekeeping and storage tools.
func registerMaintenanceTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_start_housekeeping",
		Title:       toolutil.TitleFromName("gitlab_project_start_housekeeping"),
		Description: "Trigger housekeeping for a project (git gc, repack, and other repository optimization tasks).\n\nReturns: confirmation message.\n\nSee also: gitlab_project_get, gitlab_project_repository_storage_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StartHousekeepingInput) (*mcp.CallToolResult, toolutil.VoidOutput, error) {
		start := time.Now()
		err := StartHousekeeping(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_start_housekeeping", start, err)
		if err != nil {
			return nil, toolutil.VoidOutput{}, err
		}
		return toolutil.VoidResult(fmt.Sprintf("Housekeeping started for project %s", input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_repository_storage_get",
		Title:       toolutil.TitleFromName("gitlab_project_repository_storage_get"),
		Description: "Get repository storage information for a project including disk path and storage name.\n\nReturns: JSON with project_id, disk_path, repository_storage, and created_at.\n\nSee also: gitlab_project_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetRepositoryStorageInput) (*mcp.CallToolResult, RepositoryStorageOutput, error) {
		start := time.Now()
		out, err := GetRepositoryStorage(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_repository_storage_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatRepositoryStorageMarkdown(out), toolutil.ContentDetail), out, err)
	})
}

// registerAdminProjectTools registers admin-only project tools.
func registerAdminProjectTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_project_create_for_user",
		Title:       toolutil.TitleFromName("gitlab_project_create_for_user"),
		Description: "Create a new project owned by the specified user (admin operation). The project is created in the target user's personal namespace by default.\n\nReturns: JSON with the created project details.\n\nSee also: gitlab_project_create, gitlab_project_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateForUserInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateForUser(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_project_create_for_user", start, err)
		result := toolutil.ToolResultAnnotated(FormatMarkdown(out), toolutil.ContentMutate)
		return toolutil.WithHints(result, out, err)
	})
}
