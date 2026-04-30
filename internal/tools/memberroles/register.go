// register.go wires member role MCP tools to the MCP server.
package memberroles

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab member role operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_instance_member_roles",
		Title:       toolutil.TitleFromName("gitlab_list_instance_member_roles"),
		Description: "List all custom member roles at the GitLab instance level. Requires admin access.\n\nReturns: JSON with roles array. See also: gitlab_create_instance_member_role.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInstanceInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListInstance(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_instance_member_roles", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_instance_member_role",
		Title:       toolutil.TitleFromName("gitlab_create_instance_member_role"),
		Description: "Create a custom member role at the GitLab instance level. Define name, base access level, and optional permissions.\n\nReturns: JSON with created role details. See also: gitlab_list_instance_member_roles.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInstanceInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateInstance(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_instance_member_role", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_instance_member_role",
		Title:       toolutil.TitleFromName("gitlab_delete_instance_member_role"),
		Description: "Delete a custom member role at the GitLab instance level.\n\nReturns: JSON with deletion confirmation. See also: gitlab_list_instance_member_roles.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInstanceInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete instance member role %d?", input.MemberRoleID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := DeleteInstance(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_instance_member_role", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("instance member role %d", input.MemberRoleID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_group_member_roles",
		Title:       toolutil.TitleFromName("gitlab_list_group_member_roles"),
		Description: "List all custom member roles for a GitLab group.\n\nReturns: JSON with roles array. See also: gitlab_create_group_member_role.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_group_member_roles", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_group_member_role",
		Title:       toolutil.TitleFromName("gitlab_create_group_member_role"),
		Description: "Create a custom member role for a GitLab group. Define name, base access level, and optional permissions.\n\nReturns: JSON with created role details. See also: gitlab_list_group_member_roles.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateGroupInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := CreateGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_group_member_role", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_group_member_role",
		Title:       toolutil.TitleFromName("gitlab_delete_group_member_role"),
		Description: "Delete a custom member role from a GitLab group.\n\nReturns: JSON with deletion confirmation. See also: gitlab_list_group_member_roles.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteGroupInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete member role %d from group %q?", input.MemberRoleID, input.GroupID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := DeleteGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_group_member_role", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("member role %d from group %s", input.MemberRoleID, input.GroupID))
	})
}
