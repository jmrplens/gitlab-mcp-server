package protectedenvs

import (
	"context"
	"fmt"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers the five protected environment management tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlab.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_protected_environment_list",
		Title:       toolutil.TitleFromName("gitlab_protected_environment_list"),
		Description: "List protected environments in a GitLab project with their deploy access levels and approval rules.\n\nSee also: gitlab_protected_environment_protect, gitlab_environment_list\n\nReturns: JSON with array of protected environments and pagination info.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconShield,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_protected_environment_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_protected_environment_get",
		Title:       toolutil.TitleFromName("gitlab_protected_environment_get"),
		Description: "Get a single protected environment by name, including deploy access levels and approval rules.\n\nSee also: gitlab_protected_environment_list, gitlab_environment_get\n\nReturns: JSON with protected environment details (name, deploy access levels, approval rules).",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconShield,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_protected_environment_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_protected_environment_protect",
		Title:       toolutil.TitleFromName("gitlab_protected_environment_protect"),
		Description: "Protect an environment in a GitLab project. Configure deploy access levels, required approvals, and approval rules.\n\nSee also: gitlab_protected_environment_list, gitlab_environment_create\n\nReturns: JSON with the newly protected environment details.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconShield,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProtectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Protect(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_protected_environment_protect", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_protected_environment_update",
		Title:       toolutil.TitleFromName("gitlab_protected_environment_update"),
		Description: "Update a protected environment's deploy access levels, approval rules, or required approval count.\n\nSee also: gitlab_protected_environment_get, gitlab_protected_environment_protect\n\nReturns: JSON with the updated protected environment details.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconShield,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_protected_environment_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_protected_environment_unprotect",
		Title:       toolutil.TitleFromName("gitlab_protected_environment_unprotect"),
		Description: "Remove protection from an environment. This action cannot be undone.\n\nSee also: gitlab_protected_environment_list, gitlab_protected_environment_protect\n\nReturns: JSON confirmation of unprotection.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconShield,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UnprotectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Unprotect environment %q in project %s?", input.Environment, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Unprotect(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_protected_environment_unprotect", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("protected environment")
	})
}
