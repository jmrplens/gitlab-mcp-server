// register.go wires branches MCP tools to the MCP server.

package branches

import (
	"context"
	"fmt"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers all branch tools on the given MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_branch_get",
		Title:       toolutil.TitleFromName("gitlab_branch_get"),
		Description: "Retrieve detailed information about a single branch in a GitLab project. Returns branch name, merged/protected/default status, web URL, and latest commit ID.\n\nReturns: name, merged, protected, default status, web_url, and latest commit_id. See also: gitlab_branch_create, gitlab_commit_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_branch_get", start, err)
		result := toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out))
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_branch_delete",
		Title:       toolutil.TitleFromName("gitlab_branch_delete"),
		Description: "Delete a branch from a GitLab repository. Cannot delete the default branch or protected branches.\n\nReturns: confirmation message. See also: gitlab_branch_list, gitlab_branch_create.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_branch_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("branch %q from project %s", input.BranchName, input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_branch_delete_merged",
		Title:       toolutil.TitleFromName("gitlab_branch_delete_merged"),
		Description: "Delete all branches that have been merged into the default branch. The default branch and protected branches are never deleted.\n\nReturns: confirmation message. See also: gitlab_branch_list, gitlab_mr_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteMergedInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := DeleteMerged(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_branch_delete_merged", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("merged branches from project %s", input.ProjectID))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_branch_create",
		Title:       toolutil.TitleFromName("gitlab_branch_create"),
		Description: "Create a new Git branch in a GitLab project from a ref (branch name, tag name, or commit SHA). Returns: branch name, merged/protected/default status, web URL, and latest commit ID. See also: gitlab_mr_create, gitlab_branch_list.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_branch_create", start, err)
		result := toolutil.ToolResultAnnotated(FormatOutputMarkdown(out), toolutil.ContentMutate)
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_branch_list",
		Title:       toolutil.TitleFromName("gitlab_branch_list"),
		Description: "List Git branches in a GitLab project. Supports optional name search filter. Returns paginated results including each branch's protection status and latest commit info.\n\nReturns: paginated list of branches with name, protected status, and latest commit info. See also: gitlab_branch_get, gitlab_branch_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_branch_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_branch_protect",
		Title:       toolutil.TitleFromName("gitlab_branch_protect"),
		Description: "Protect a GitLab repository branch by setting push and merge access levels (0=no access, 30=developer, 40=maintainer, 60=admin). Protected branches cannot be force-pushed or deleted. Returns: branch name, push/merge access levels, allow_force_push, and code_owner_approval_required. See also: gitlab_branch_unprotect, gitlab_protected_branches_list.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProtectInput) (*mcp.CallToolResult, ProtectedOutput, error) {
		start := time.Now()
		out, err := Protect(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_branch_protect", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProtectedMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_branch_unprotect",
		Title:       toolutil.TitleFromName("gitlab_branch_unprotect"),
		Description: "Remove all protection rules from a GitLab branch, allowing unrestricted push, merge, and force-push access.\n\nReturns: confirmation message. See also: gitlab_branch_protect, gitlab_protected_branches_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UnprotectInput) (*mcp.CallToolResult, UnprotectOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Remove protection from branch %q in project %q?", input.BranchName, input.ProjectID)); r != nil {
			return r, UnprotectOutput{}, nil
		}
		start := time.Now()
		out, err := Unprotect(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_branch_unprotect", start, err)
		if err != nil {
			return nil, UnprotectOutput{}, err
		}
		md := fmt.Sprintf(toolutil.EmojiSuccess+" Successfully removed protection from branch **%q** in project **%s**.", input.BranchName, input.ProjectID)
		if out.Status == "already_unprotected" {
			md = fmt.Sprintf(toolutil.EmojiSuccess+" Branch **%q** in project **%s** is already unprotected — no action needed.", input.BranchName, input.ProjectID)
		}
		return toolutil.ToolResultAnnotated(md, toolutil.ContentMutate), out, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_protected_branches_list",
		Title:       toolutil.TitleFromName("gitlab_protected_branches_list"),
		Description: "List all protected branches in a GitLab project with their configured push and merge access level restrictions. Returns paginated results.\n\nReturns: paginated list of protected branches with name, push/merge access levels, and allow_force_push. See also: gitlab_branch_protect, gitlab_protected_branch_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProtectedListInput) (*mcp.CallToolResult, ProtectedListOutput, error) {
		start := time.Now()
		out, err := ProtectedList(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_protected_branches_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProtectedListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_protected_branch_get",
		Title:       toolutil.TitleFromName("gitlab_protected_branch_get"),
		Description: "Get details of a single protected branch by name. Returns: branch name, push/merge access levels, allow_force_push, and code_owner_approval_required. See also: gitlab_protected_branches_list, gitlab_branch_protect.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProtectedGetInput) (*mcp.CallToolResult, ProtectedOutput, error) {
		start := time.Now()
		out, err := ProtectedGet(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_protected_branch_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProtectedMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_protected_branch_update",
		Title:       toolutil.TitleFromName("gitlab_protected_branch_update"),
		Description: "Update settings on an existing protected branch (allow_force_push, code_owner_approval_required). Use gitlab_branch_protect to initially protect a branch. Returns: updated name, push/merge access levels, allow_force_push, and code_owner_approval_required.\n\nSee also: gitlab_protected_branch_get, gitlab_protected_branches_list.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProtectedUpdateInput) (*mcp.CallToolResult, ProtectedOutput, error) {
		start := time.Now()
		out, err := ProtectedUpdate(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_protected_branch_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatProtectedMarkdown(out)), out, err)
	})
}
