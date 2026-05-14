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
	specs := ActionSpecs(client)
	jobTokenScopeTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconToken})
	}

	mcp.AddTool(server, jobTokenScopeTool("gitlab_get_job_token_access_settings", "Get the CI/CD job token access settings for a GitLab project.\n\nReturns: JSON with job token scope configuration.\n\nSee also: gitlab_patch_job_token_access_settings, gitlab_ci_variable_list"), func(ctx context.Context, req *mcp.CallToolRequest, input GetAccessSettingsInput) (*mcp.CallToolResult, AccessSettingsOutput, error) {
		start := time.Now()
		out, err := GetAccessSettings(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_job_token_access_settings", start, err)
		return toolutil.WithHints(FormatAccessSettingsMarkdown(out), out, err)
	})

	mcp.AddTool(server, jobTokenScopeTool("gitlab_patch_job_token_access_settings", "Update the CI/CD job token access settings for a GitLab project.\n\nReturns: confirmation message.\n\nSee also: gitlab_get_job_token_access_settings, gitlab_list_job_token_inbound_allowlist"), func(ctx context.Context, req *mcp.CallToolRequest, input PatchAccessSettingsInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		out, err := PatchAccessSettings(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_patch_job_token_access_settings", start, err)
		return toolutil.WithHints(FormatPatchResultMarkdown(out), out, err)
	})

	mcp.AddTool(server, jobTokenScopeTool("gitlab_list_job_token_inbound_allowlist", "List projects on the CI/CD job token inbound allowlist for a GitLab project.\n\nReturns: JSON array of allowlist entries with pagination.\n\nSee also: gitlab_add_project_job_token_allowlist, gitlab_get_job_token_access_settings"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInboundAllowlistInput) (*mcp.CallToolResult, ListInboundAllowlistOutput, error) {
		start := time.Now()
		out, err := ListInboundAllowlist(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_job_token_inbound_allowlist", start, err)
		return toolutil.WithHints(FormatListInboundAllowlistMarkdown(out), out, err)
	})

	mcp.AddTool(server, jobTokenScopeTool("gitlab_add_project_job_token_allowlist", "Add a project to the CI/CD job token inbound allowlist.\n\nReturns: JSON with the allowlist entry.\n\nSee also: gitlab_list_job_token_inbound_allowlist, gitlab_remove_project_job_token_allowlist"), func(ctx context.Context, req *mcp.CallToolRequest, input AddProjectAllowlistInput) (*mcp.CallToolResult, InboundAllowItemOutput, error) {
		start := time.Now()
		out, err := AddProjectAllowlist(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_project_job_token_allowlist", start, err)
		return toolutil.WithHints(FormatAddProjectAllowlistMarkdown(out), out, err)
	})

	mcp.AddTool(server, jobTokenScopeTool("gitlab_remove_project_job_token_allowlist", "Remove a project from the CI/CD job token inbound allowlist.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_job_token_inbound_allowlist, gitlab_add_project_job_token_allowlist"), func(ctx context.Context, req *mcp.CallToolRequest, input RemoveProjectAllowlistInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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

	mcp.AddTool(server, jobTokenScopeTool("gitlab_list_job_token_group_allowlist", "List groups on the CI/CD job token allowlist for a GitLab project.\n\nReturns: JSON array of allowlist entries with pagination.\n\nSee also: gitlab_add_group_job_token_allowlist, gitlab_get_job_token_access_settings"), func(ctx context.Context, req *mcp.CallToolRequest, input ListGroupAllowlistInput) (*mcp.CallToolResult, ListGroupAllowlistOutput, error) {
		start := time.Now()
		out, err := ListGroupAllowlist(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_job_token_group_allowlist", start, err)
		return toolutil.WithHints(FormatListGroupAllowlistMarkdown(out), out, err)
	})

	mcp.AddTool(server, jobTokenScopeTool("gitlab_add_group_job_token_allowlist", "Add a group to the CI/CD job token allowlist.\n\nReturns: JSON with the allowlist entry.\n\nSee also: gitlab_list_job_token_group_allowlist, gitlab_remove_group_job_token_allowlist"), func(ctx context.Context, req *mcp.CallToolRequest, input AddGroupAllowlistInput) (*mcp.CallToolResult, GroupAllowlistItemOutput, error) {
		start := time.Now()
		out, err := AddGroupAllowlist(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_add_group_job_token_allowlist", start, err)
		return toolutil.WithHints(FormatAddGroupAllowlistMarkdown(out), out, err)
	})

	mcp.AddTool(server, jobTokenScopeTool("gitlab_remove_group_job_token_allowlist", "Remove a group from the CI/CD job token allowlist.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_job_token_group_allowlist, gitlab_add_group_job_token_allowlist"), func(ctx context.Context, req *mcp.CallToolRequest, input RemoveGroupAllowlistInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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
