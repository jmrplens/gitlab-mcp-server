// register.go wires group protected environment MCP tools to the MCP server.
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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_protected_environment_list",
		Title:       toolutil.TitleFromName("gitlab_group_protected_environment_list"),
		Description: "List all protected environments for a GitLab group.\n\nReturns: paginated list of protected environments with deploy access levels and approval rules. See also: gitlab_group_protected_environment_get, gitlab_protected_environment_list, gitlab_environment_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEnvironment,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_environment_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_protected_environment_get",
		Title:       toolutil.TitleFromName("gitlab_group_protected_environment_get"),
		Description: "Get a single group-level protected environment by name.\n\nReturns: protected environment with deploy access levels and approval rules. See also: gitlab_group_protected_environment_list, gitlab_group_protected_environment_update.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconEnvironment,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_environment_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_protected_environment_protect",
		Title:       toolutil.TitleFromName("gitlab_group_protected_environment_protect"),
		Description: "Protect an environment at the group level.\n\nReturns: created protected environment with access levels and approval rules. See also: gitlab_group_protected_environment_list, gitlab_group_protected_environment_unprotect.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconEnvironment,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProtectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Protect(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_environment_protect", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_protected_environment_update",
		Title:       toolutil.TitleFromName("gitlab_group_protected_environment_update"),
		Description: "Update a group-level protected environment.\n\nReturns: updated protected environment with access levels and approval rules. See also: gitlab_group_protected_environment_get, gitlab_group_protected_environment_protect.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconEnvironment,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_environment_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_protected_environment_unprotect",
		Title:       toolutil.TitleFromName("gitlab_group_protected_environment_unprotect"),
		Description: "Remove protection from a group-level environment.\n\nReturns: confirmation of removal. See also: gitlab_group_protected_environment_list, gitlab_group_protected_environment_protect.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconEnvironment,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UnprotectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Unprotect(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_environment_unprotect", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group protected environment")
	})
}
