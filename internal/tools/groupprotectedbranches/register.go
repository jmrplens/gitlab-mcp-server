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
	specs := ActionSpecs(client)
	protectedBranchTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconBranch})
	}

	mcp.AddTool(server, protectedBranchTool("gitlab_group_protected_branch_list", "List all protected branches for a GitLab group.\n\nReturns: paginated list of protected branch rules with access levels. See also: gitlab_group_protected_branch_get, gitlab_branch_protect, gitlab_list_branch_rules."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_branch_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, protectedBranchTool("gitlab_group_protected_branch_get", "Get a single group-level protected branch rule by name.\n\nReturns: protected branch with access levels and settings. See also: gitlab_group_protected_branch_list, gitlab_group_protected_branch_update."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_branch_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, protectedBranchTool("gitlab_group_protected_branch_protect", "Protect a branch at the group level.\n\nReturns: created protected branch rule with access levels. See also: gitlab_group_protected_branch_list, gitlab_group_protected_branch_unprotect."), func(ctx context.Context, req *mcp.CallToolRequest, input ProtectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Protect(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_branch_protect", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, protectedBranchTool("gitlab_group_protected_branch_update", "Update a group-level protected branch rule.\n\nReturns: updated protected branch with access levels. See also: gitlab_group_protected_branch_get, gitlab_group_protected_branch_protect."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_branch_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, protectedBranchTool("gitlab_group_protected_branch_unprotect", "Remove a group-level protected branch rule.\n\nReturns: confirmation of removal. See also: gitlab_group_protected_branch_list, gitlab_group_protected_branch_protect."), func(ctx context.Context, req *mcp.CallToolRequest, input UnprotectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Unprotect(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_protected_branch_unprotect", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group protected branch")
	})
}
