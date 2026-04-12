// register.go wires jobtokenscope MCP tools to the MCP server.

package jobtokenscope

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all job token scope tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_job_token_access_settings",
		Title:       toolutil.TitleFromName("gitlab_get_job_token_access_settings"),
		Description: "Get the CI/CD job token access settings for a GitLab project.\n\nReturns: JSON with job token scope configuration.\n\nSee also: gitlab_patch_job_token_access_settings, gitlab_list_ci_variables",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetAccessSettingsInput) (*mcp.CallToolResult, AccessSettingsOutput, error) {
		start := time.Now()
		out, err := GetAccessSettings(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_job_token_access_settings", start, err)
		return toolutil.WithHints(FormatAccessSettingsMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_patch_job_token_access_settings",
		Title:       toolutil.TitleFromName("gitlab_patch_job_token_access_settings"),
		Description: "Update the CI/CD job token access settings for a GitLab project.\n\nReturns: confirmation message.\n\nSee also: gitlab_get_job_token_access_settings, gitlab_list_job_token_inbound_allowlist",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input PatchAccessSettingsInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		out, err := PatchAccessSettings(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_patch_job_token_access_settings", start, err)
		return toolutil.WithHints(FormatPatchResultMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_job_token_inbound_allowlist",
		Title:       toolutil.TitleFromName("gitlab_list_job_token_inbound_allowlist"),
		Description: "List projects on the CI/CD job token inbound allowlist for a GitLab project.\n\nReturns: JSON array of allowlist entries with pagination.\n\nSee also: gitlab_add_project_job_token_allowlist, gitlab_get_job_token_access_settings",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInboundAllowlistInput) (*mcp.CallToolResult, ListInboundAllowlistOutput, error) {
		start := time.Now()
		out, err := ListInboundAllowlist(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_job_token_inbound_allowlist", start, err)
		return toolutil.WithHints(FormatListInboundAllowlistMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_add_project_job_token_allowlist",
		Title:       toolutil.TitleFromName("gitlab_add_project_job_token_allowlist"),
		Description: "Add a project to the CI/CD job token inbound allowlist.\n\nReturns: JSON with the allowlist entry.\n\nSee also: gitlab_list_job_token_inbound_allowlist, gitlab_remove_project_job_token_allowlist",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddProjectAllowlistInput) (*mcp.CallToolResult, InboundAllowItemOutput, error) {
		start := time.Now()
		out, err := AddProjectAllowlist(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_project_job_token_allowlist", start, err)
		return toolutil.WithHints(FormatAddProjectAllowlistMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_remove_project_job_token_allowlist",
		Title:       toolutil.TitleFromName("gitlab_remove_project_job_token_allowlist"),
		Description: "Remove a project from the CI/CD job token inbound allowlist.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_job_token_inbound_allowlist, gitlab_add_project_job_token_allowlist",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RemoveProjectAllowlistInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Remove project %d from job token allowlist of project %q?", input.TargetProjectID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := RemoveProjectAllowlist(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_remove_project_job_token_allowlist", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		r, o, _ := toolutil.DeleteResult("project from job token allowlist")
		return r, o, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_job_token_group_allowlist",
		Title:       toolutil.TitleFromName("gitlab_list_job_token_group_allowlist"),
		Description: "List groups on the CI/CD job token allowlist for a GitLab project.\n\nReturns: JSON array of allowlist entries with pagination.\n\nSee also: gitlab_add_group_job_token_allowlist, gitlab_get_job_token_access_settings",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupAllowlistInput) (*mcp.CallToolResult, ListGroupAllowlistOutput, error) {
		start := time.Now()
		out, err := ListGroupAllowlist(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_job_token_group_allowlist", start, err)
		return toolutil.WithHints(FormatListGroupAllowlistMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_add_group_job_token_allowlist",
		Title:       toolutil.TitleFromName("gitlab_add_group_job_token_allowlist"),
		Description: "Add a group to the CI/CD job token allowlist.\n\nReturns: JSON with the allowlist entry.\n\nSee also: gitlab_list_job_token_group_allowlist, gitlab_remove_group_job_token_allowlist",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddGroupAllowlistInput) (*mcp.CallToolResult, GroupAllowlistItemOutput, error) {
		start := time.Now()
		out, err := AddGroupAllowlist(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_group_job_token_allowlist", start, err)
		return toolutil.WithHints(FormatAddGroupAllowlistMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_remove_group_job_token_allowlist",
		Title:       toolutil.TitleFromName("gitlab_remove_group_job_token_allowlist"),
		Description: "Remove a group from the CI/CD job token allowlist.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_job_token_group_allowlist, gitlab_add_group_job_token_allowlist",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconToken,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RemoveGroupAllowlistInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Remove group %d from job token allowlist of project %q?", input.TargetGroupID, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := RemoveGroupAllowlist(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_remove_group_job_token_allowlist", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		r, o, _ := toolutil.DeleteResult("group from job token allowlist")
		return r, o, nil
	})
}

// RegisterMeta registers the gitlab_job_token_scope meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := map[string]toolutil.ActionFunc{
		"get_access_settings":            toolutil.WrapAction(client, GetAccessSettings),
		"patch_access_settings":          toolutil.WrapAction(client, PatchAccessSettings),
		"list_inbound_project_allowlist": toolutil.WrapAction(client, ListInboundAllowlist),
		"add_project_allowlist":          toolutil.WrapAction(client, AddProjectAllowlist),
		"remove_project_allowlist":       toolutil.WrapVoidAction(client, RemoveProjectAllowlist),
		"list_group_allowlist":           toolutil.WrapAction(client, ListGroupAllowlist),
		"add_group_allowlist":            toolutil.WrapAction(client, AddGroupAllowlist),
		"remove_group_allowlist":         toolutil.WrapVoidAction(client, RemoveGroupAllowlist),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_job_token_scope",
		Title: toolutil.TitleFromName("gitlab_job_token_scope"),
		Description: `Manage CI/CD job token scope settings for a GitLab project. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- get_access_settings: Get job token access settings. Params: project_id (required)
- patch_access_settings: Update job token access settings. Params: project_id (required), enabled (required, bool)
- list_inbound_project_allowlist: List projects in inbound allowlist. Params: project_id (required), page, per_page
- add_project_allowlist: Add a project to inbound allowlist. Params: project_id (required), target_project_id (required, int)
- remove_project_allowlist: Remove a project from inbound allowlist. Params: project_id (required), target_project_id (required, int)
- list_group_allowlist: List groups in allowlist. Params: project_id (required), page, per_page
- add_group_allowlist: Add a group to allowlist. Params: project_id (required), target_group_id (required, int)
- remove_group_allowlist: Remove a group from allowlist. Params: project_id (required), target_group_id (required, int)`,
		Annotations: toolutil.MetaAnnotations,
		Icons:       toolutil.IconToken,
	}, toolutil.MakeMetaHandler("gitlab_job_token_scope", routes, nil))
}
