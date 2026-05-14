package groupprotectedenvs

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers group protected environment tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	protectedEnvironmentTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconEnvironment})
	}

	mcp.AddTool(server, protectedEnvironmentTool("gitlab_group_protected_environment_list", "List all protected environments for a GitLab group.\n\nReturns: paginated list of protected environments with deploy access levels and approval rules. See also: gitlab_group_protected_environment_get, gitlab_protected_environment_list, gitlab_environment_list."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_environment_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, protectedEnvironmentTool("gitlab_group_protected_environment_get", "Get a single group-level protected environment by name.\n\nReturns: protected environment with deploy access levels and approval rules. See also: gitlab_group_protected_environment_list, gitlab_group_protected_environment_update."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_environment_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, protectedEnvironmentTool("gitlab_group_protected_environment_protect", "Protect an environment at the group level.\n\nReturns: created protected environment with access levels and approval rules. See also: gitlab_group_protected_environment_list, gitlab_group_protected_environment_unprotect."), func(ctx context.Context, req *mcp.CallToolRequest, input ProtectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Protect(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_environment_protect", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, protectedEnvironmentTool("gitlab_group_protected_environment_update", "Update a group-level protected environment.\n\nReturns: updated protected environment with access levels and approval rules. See also: gitlab_group_protected_environment_get, gitlab_group_protected_environment_protect."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_environment_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, protectedEnvironmentTool("gitlab_group_protected_environment_unprotect", "Remove protection from a group-level environment.\n\nReturns: confirmation of removal. See also: gitlab_group_protected_environment_list, gitlab_group_protected_environment_protect."), func(ctx context.Context, req *mcp.CallToolRequest, input UnprotectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Unprotect(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_environment_unprotect", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group protected environment")
	})
}
