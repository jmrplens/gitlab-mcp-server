package securitysettings

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab security settings operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := append(ProjectActionSpecs(client), GroupActionSpecs(client)...)
	securitySettingTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconSecurity})
	}

	mcp.AddTool(server, securitySettingTool("gitlab_get_project_security_settings", "Get security settings for a GitLab project. Returns auto-fix, vulnerability scanning, and secret push protection status.\n\nReturns: JSON with project security settings. See also: gitlab_update_project_secret_push_protection."), func(ctx context.Context, req *mcp.CallToolRequest, input GetProjectInput) (*mcp.CallToolResult, ProjectOutput, error) {
		start := time.Now()
		out, err := GetProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_security_settings", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProjectMarkdown(out)), out, err)
	})

	mcp.AddTool(server, securitySettingTool("gitlab_update_project_secret_push_protection", "Enable or disable secret push protection for a GitLab project.\n\nReturns: JSON with updated security settings. See also: gitlab_get_project_security_settings."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateProjectInput) (*mcp.CallToolResult, ProjectOutput, error) {
		start := time.Now()
		out, err := UpdateProject(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_project_secret_push_protection", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProjectMarkdown(out)), out, err)
	})

	mcp.AddTool(server, securitySettingTool("gitlab_update_group_secret_push_protection", "Enable or disable secret push protection for a GitLab group. Optionally exclude specific projects.\n\nReturns: JSON with updated group security settings. See also: gitlab_get_project_security_settings."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateGroupInput) (*mcp.CallToolResult, GroupOutput, error) {
		start := time.Now()
		out, err := UpdateGroup(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_group_secret_push_protection", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGroupMarkdown(out)), out, err)
	})
}
