package groupprotectedbranches

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers group protected branch tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_protected_branch_list",
		Title:       toolutil.TitleFromName("gitlab_group_protected_branch_list"),
		Description: "List all protected branches for a GitLab group.\n\nReturns: paginated list of protected branch rules with access levels. See also: gitlab_group_protected_branch_get, gitlab_branch_protect, gitlab_list_branch_rules.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_branch_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_protected_branch_get",
		Title:       toolutil.TitleFromName("gitlab_group_protected_branch_get"),
		Description: "Get a single group-level protected branch rule by name.\n\nReturns: protected branch with access levels and settings. See also: gitlab_group_protected_branch_list, gitlab_group_protected_branch_update.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_branch_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_protected_branch_protect",
		Title:       toolutil.TitleFromName("gitlab_group_protected_branch_protect"),
		Description: "Protect a branch at the group level.\n\nReturns: created protected branch rule with access levels. See also: gitlab_group_protected_branch_list, gitlab_group_protected_branch_unprotect.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProtectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Protect(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_branch_protect", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_protected_branch_update",
		Title:       toolutil.TitleFromName("gitlab_group_protected_branch_update"),
		Description: "Update a group-level protected branch rule.\n\nReturns: updated protected branch with access levels. See also: gitlab_group_protected_branch_get, gitlab_group_protected_branch_protect.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_branch_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_protected_branch_unprotect",
		Title:       toolutil.TitleFromName("gitlab_group_protected_branch_unprotect"),
		Description: "Remove a group-level protected branch rule.\n\nReturns: confirmation of removal. See also: gitlab_group_protected_branch_list, gitlab_group_protected_branch_protect.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UnprotectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Unprotect(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_branch_unprotect", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group protected branch")
	})
}
