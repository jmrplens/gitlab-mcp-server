// register.go wires groups MCP tools to the MCP server.

package groups

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers tools for group list, get, members, and subgroups.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_list",
		Title:       toolutil.TitleFromName("gitlab_group_list"),
		Description: "List GitLab groups accessible to the authenticated user. Supports filtering by search term, ownership, and top-level only. Returns paginated results including group name, path, visibility, and web URL.\n\nReturns: paginated list of groups with id, name, path, full_path, visibility, web_url, and parent_id. See also: gitlab_group_get, gitlab_project_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_get",
		Title:       toolutil.TitleFromName("gitlab_group_get"),
		Description: "Retrieve detailed metadata for a GitLab group including name, path, full path, description, visibility, web URL, and parent group. Accepts numeric group ID or URL-encoded path (e.g. 'group/subgroup').\n\nReturns: id, name, path, full_path, description, visibility, web_url, and parent_id. See also: gitlab_group_update, gitlab_group_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_group_get", start, nil)
			return toolutil.NotFoundResult("Group", string(input.GroupID),
				"Use gitlab_group_list to list accessible groups",
				"If using a path, ensure it is URL-encoded (e.g. my%2Fgroup)",
				"Verify your token has access to this group",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_get", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out))
		if err == nil && out.ID > 0 {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://group/%d", out.ID),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_members_list",
		Title:       toolutil.TitleFromName("gitlab_group_members_list"),
		Description: "List all members of a GitLab group including inherited members. Returns user ID, username, name, state, access level (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner), and web URL. Supports filtering by name/username query.\n\nReturns: paginated list of members with user_id, username, name, state, access_level, and web_url. See also: gitlab_group_get, gitlab_group_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MembersListInput) (*mcp.CallToolResult, MemberListOutput, error) {
		start := time.Now()
		out, err := MembersList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_members_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMemberListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_subgroups_list",
		Title:       toolutil.TitleFromName("gitlab_subgroups_list"),
		Description: "List descendant subgroups of a GitLab group. Returns each subgroup's name, path, full path, description, visibility, and parent ID. Supports search filter and pagination.\n\nReturns: paginated list of subgroups with id, name, path, full_path, visibility, and parent_id. See also: gitlab_group_get, gitlab_group_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SubgroupsListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := SubgroupsList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_subgroups_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_create",
		Title:       toolutil.TitleFromName("gitlab_group_create"),
		Description: "Create a new GitLab group. Requires name; optionally set path, description, visibility, parent_id (for subgroups), request_access_enabled, lfs_enabled, and default_branch. Returns: id, name, path, full_path, visibility, web_url, parent_id. See also: gitlab_group_get, gitlab_group_list.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_create", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_update",
		Title:       toolutil.TitleFromName("gitlab_group_update"),
		Description: "Update an existing GitLab group. Supports changing name, path, description, visibility, request_access_enabled, lfs_enabled, and default_branch. Returns: id, name, path, full_path, visibility, web_url, parent_id. See also: gitlab_group_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_update", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_delete",
		Title:       toolutil.TitleFromName("gitlab_group_delete"),
		Description: "Delete a GitLab group. On instances with delayed deletion, the group is marked for deletion. Set permanently_remove=true with full_path to bypass delayed deletion.\n\nReturns: confirmation message. See also: gitlab_group_list, gitlab_group_create.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete group %s?", input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group")
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_restore",
		Title:       toolutil.TitleFromName("gitlab_group_restore"),
		Description: "Restore a GitLab group that was marked for deletion.\n\nReturns: id, name, path, full_path, visibility, web_url, and parent_id.\n\nSee also: gitlab_group_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RestoreInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Restore(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_restore", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_archive",
		Title:       toolutil.TitleFromName("gitlab_group_archive"),
		Description: "Archive a GitLab group. Requires Owner role or administrator.\n\nReturns: confirmation message. See also: gitlab_group_unarchive, gitlab_group_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ArchiveInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Archive(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_archive", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		out := toolutil.DeleteOutput{Message: fmt.Sprintf("Group %s archived successfully", input.GroupID)}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(out.Message), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_unarchive",
		Title:       toolutil.TitleFromName("gitlab_group_unarchive"),
		Description: "Unarchive a previously archived GitLab group. Requires Owner role or administrator.\n\nReturns: confirmation message. See also: gitlab_group_archive, gitlab_group_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ArchiveInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Unarchive(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_unarchive", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		out := toolutil.DeleteOutput{Message: fmt.Sprintf("Group %s unarchived successfully", input.GroupID)}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(out.Message), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_search",
		Title:       toolutil.TitleFromName("gitlab_group_search"),
		Description: "Search for GitLab groups by name. Returns matching groups with their details.\n\nReturns: paginated list of groups with id, name, path, full_path, visibility, and web_url. See also: gitlab_group_get, gitlab_group_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := Search(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_search", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_transfer_project",
		Title:       toolutil.TitleFromName("gitlab_group_transfer_project"),
		Description: "Transfer a project into a group namespace. Moves the project to become a member of the specified group.\n\nReturns: transferred project details with id, name, path, and namespace. See also: gitlab_group_get, gitlab_project_transfer.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input TransferInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := TransferProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_transfer_project", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_projects",
		Title:       toolutil.TitleFromName("gitlab_group_projects"),
		Description: "List projects belonging to a GitLab group. Supports filtering by search, archived status, visibility, and including subgroup projects. Returns project name, path, visibility, and archived status with pagination.\n\nReturns: paginated list of projects with id, name, path, visibility, and archived status. See also: gitlab_group_get, gitlab_project_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectsInput) (*mcp.CallToolResult, ListProjectsOutput, error) {
		start := time.Now()
		out, err := ListProjects(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_projects", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListProjectsMarkdown(out)), out, err)
	})

	// Group Hook tools.

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_hook_list",
		Title:       toolutil.TitleFromName("gitlab_group_hook_list"),
		Description: "List webhooks configured for a GitLab group. Returns hook URL, enabled events, SSL verification status, and creation date with pagination.\n\nReturns: paginated list of webhooks with id, url, events, and SSL status. See also: gitlab_group_hook_get, gitlab_group_hook_add.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListHooksInput) (*mcp.CallToolResult, HookListOutput, error) {
		start := time.Now()
		out, err := ListHooks(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_hook_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatHookListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_hook_get",
		Title:       toolutil.TitleFromName("gitlab_group_hook_get"),
		Description: "Get details of a specific group webhook by hook ID. Returns URL, enabled events, SSL status, and alert status.\n\nReturns: webhook id, url, enabled events, SSL status, and alert status. See also: gitlab_group_hook_list, gitlab_group_hook_edit.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetHookInput) (*mcp.CallToolResult, HookOutput, error) {
		start := time.Now()
		out, err := GetHook(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_hook_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatHookMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_hook_add",
		Title:       toolutil.TitleFromName("gitlab_group_hook_add"),
		Description: "Add a new webhook to a GitLab group. Requires URL; optionally configure event triggers, SSL verification, secret token, and branch filter.\n\nReturns: created webhook with id, url, enabled events, and SSL status. See also: gitlab_group_hook_list, gitlab_group_hook_edit.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddHookInput) (*mcp.CallToolResult, HookOutput, error) {
		start := time.Now()
		out, err := AddHook(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_hook_add", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatHookMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_hook_edit",
		Title:       toolutil.TitleFromName("gitlab_group_hook_edit"),
		Description: "Edit an existing group webhook. Supports changing URL, events, SSL verification, secret token, and branch filter.\n\nReturns: updated webhook with id, url, enabled events, and SSL status. See also: gitlab_group_hook_get, gitlab_group_hook_delete.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EditHookInput) (*mcp.CallToolResult, HookOutput, error) {
		start := time.Now()
		out, err := EditHook(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_hook_edit", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatHookMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_hook_delete",
		Title:       toolutil.TitleFromName("gitlab_group_hook_delete"),
		Description: "Delete a webhook from a GitLab group.\n\nReturns: confirmation message. See also: gitlab_group_hook_list, gitlab_group_hook_add.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteHookInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete webhook %d from group %s?", input.HookID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteHook(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_hook_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group hook")
	})
}
