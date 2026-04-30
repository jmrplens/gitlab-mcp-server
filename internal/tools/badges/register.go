// register.go wires badges MCP tools to the MCP server.
package badges

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all badge tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	// Project Badges.

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_project_badges",
		Title:       toolutil.TitleFromName("gitlab_list_project_badges"),
		Description: "List all badges of a project, including inherited group badges.\n\nSee also: gitlab_add_project_badge, gitlab_list_group_badges\n\nReturns: JSON array of badges with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListProjectInput) (*mcp.CallToolResult, ListProjectOutput, error) {
		start := time.Now()
		out, err := ListProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_project_badges", start, err)
		return toolutil.WithHints(FormatBadgeListMarkdown(out.Badges, "Project Badges", out.Pagination), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_project_badge",
		Title:       toolutil.TitleFromName("gitlab_get_project_badge"),
		Description: "Get a specific project badge by ID.\n\nSee also: gitlab_list_project_badges, gitlab_edit_project_badge\n\nReturns: JSON with badge details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetProjectInput) (*mcp.CallToolResult, GetProjectOutput, error) {
		start := time.Now()
		out, err := GetProject(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_badge", start, nil)
			return toolutil.NotFoundResult("Project Badge", fmt.Sprintf("badge %d in project %s", input.BadgeID, input.ProjectID),
				"Use gitlab_list_project_badges to list badges for this project",
				"Verify the badge_id is correct",
			), GetProjectOutput{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_badge", start, err)
		return toolutil.WithHints(FormatBadgeMarkdown(out.Badge), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_add_project_badge",
		Title:       toolutil.TitleFromName("gitlab_add_project_badge"),
		Description: "Add a new badge to a project. Badge URLs support placeholders like %{project_path}, %{default_branch}, %{commit_sha}.\n\nSee also: gitlab_list_project_badges, gitlab_preview_project_badge\n\nReturns: JSON with the badge details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddProjectInput) (*mcp.CallToolResult, AddProjectOutput, error) {
		start := time.Now()
		out, err := AddProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_project_badge", start, err)
		return toolutil.WithHints(FormatBadgeMarkdown(out.Badge), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_edit_project_badge",
		Title:       toolutil.TitleFromName("gitlab_edit_project_badge"),
		Description: "Edit an existing project badge.\n\nSee also: gitlab_get_project_badge, gitlab_list_project_badges\n\nReturns: JSON with the badge details.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EditProjectInput) (*mcp.CallToolResult, EditProjectOutput, error) {
		start := time.Now()
		out, err := EditProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_edit_project_badge", start, err)
		return toolutil.WithHints(FormatBadgeMarkdown(out.Badge), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_project_badge",
		Title:       toolutil.TitleFromName("gitlab_delete_project_badge"),
		Description: "Remove a badge from a project.\n\nSee also: gitlab_list_project_badges, gitlab_add_project_badge\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteProjectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete badge %d from project %s?", input.BadgeID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_project_badge", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		r, o, _ := toolutil.DeleteResult("project badge")
		return r, o, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_preview_project_badge",
		Title:       toolutil.TitleFromName("gitlab_preview_project_badge"),
		Description: "Preview how a project badge renders after placeholder interpolation, without creating it.\n\nSee also: gitlab_add_project_badge, gitlab_list_project_badges\n\nReturns: JSON with rendered badge URLs.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PreviewProjectInput) (*mcp.CallToolResult, PreviewProjectOutput, error) {
		start := time.Now()
		out, err := PreviewProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_preview_project_badge", start, err)
		return toolutil.WithHints(FormatBadgeMarkdown(out.Badge), out, err)
	})

	// Group Badges.

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_group_badges",
		Title:       toolutil.TitleFromName("gitlab_list_group_badges"),
		Description: "List all badges of a group.\n\nSee also: gitlab_add_group_badge, gitlab_list_project_badges\n\nReturns: JSON array of badges with pagination.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupInput) (*mcp.CallToolResult, ListGroupOutput, error) {
		start := time.Now()
		out, err := ListGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_group_badges", start, err)
		return toolutil.WithHints(FormatBadgeListMarkdown(out.Badges, "Group Badges", out.Pagination), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_group_badge",
		Title:       toolutil.TitleFromName("gitlab_get_group_badge"),
		Description: "Get a specific group badge by ID.\n\nSee also: gitlab_list_group_badges, gitlab_edit_group_badge\n\nReturns: JSON with badge details.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetGroupInput) (*mcp.CallToolResult, GetGroupOutput, error) {
		start := time.Now()
		out, err := GetGroup(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_get_group_badge", start, nil)
			return toolutil.NotFoundResult("Group Badge", fmt.Sprintf("badge %d in group %s", input.BadgeID, input.GroupID),
				"Use gitlab_list_group_badges to list badges for this group",
				"Verify the badge_id and group_id are correct",
			), GetGroupOutput{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_group_badge", start, err)
		return toolutil.WithHints(FormatBadgeMarkdown(out.Badge), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_add_group_badge",
		Title:       toolutil.TitleFromName("gitlab_add_group_badge"),
		Description: "Add a new badge to a group. Badge URLs support placeholders like %{project_path}, %{default_branch}, %{commit_sha}.\n\nSee also: gitlab_list_group_badges, gitlab_preview_group_badge\n\nReturns: JSON with the badge details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddGroupInput) (*mcp.CallToolResult, AddGroupOutput, error) {
		start := time.Now()
		out, err := AddGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_group_badge", start, err)
		return toolutil.WithHints(FormatBadgeMarkdown(out.Badge), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_edit_group_badge",
		Title:       toolutil.TitleFromName("gitlab_edit_group_badge"),
		Description: "Edit an existing group badge.\n\nSee also: gitlab_get_group_badge, gitlab_list_group_badges\n\nReturns: JSON with the badge details.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EditGroupInput) (*mcp.CallToolResult, EditGroupOutput, error) {
		start := time.Now()
		out, err := EditGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_edit_group_badge", start, err)
		return toolutil.WithHints(FormatBadgeMarkdown(out.Badge), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_group_badge",
		Title:       toolutil.TitleFromName("gitlab_delete_group_badge"),
		Description: "Remove a badge from a group.\n\nSee also: gitlab_list_group_badges, gitlab_add_group_badge\n\nReturns: confirmation message.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteGroupInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete badge %d from group %s?", input.BadgeID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := DeleteGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_group_badge", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		r, o, _ := toolutil.DeleteResult("group badge")
		return r, o, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_preview_group_badge",
		Title:       toolutil.TitleFromName("gitlab_preview_group_badge"),
		Description: "Preview how a group badge renders after placeholder interpolation, without creating it.\n\nSee also: gitlab_add_group_badge, gitlab_list_group_badges\n\nReturns: JSON with rendered badge URLs.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PreviewGroupInput) (*mcp.CallToolResult, PreviewGroupOutput, error) {
		start := time.Now()
		out, err := PreviewGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_preview_group_badge", start, err)
		return toolutil.WithHints(FormatBadgeMarkdown(out.Badge), out, err)
	})
}
