package protectedenvs

import (
	"context"
	"fmt"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers the five protected environment management tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	protectedEnvironmentTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconShield})
	}

	mcp.AddTool(server, protectedEnvironmentTool("gitlab_protected_environment_list", "List protected environments in a GitLab project with their deploy access levels and approval rules.\n\nSee also: gitlab_protected_environment_protect, gitlab_environment_list\n\nReturns: JSON with array of protected environments and pagination info."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_protected_environment_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, protectedEnvironmentTool("gitlab_protected_environment_get", "Get a single protected environment by name, including deploy access levels and approval rules.\n\nSee also: gitlab_protected_environment_list, gitlab_environment_get\n\nReturns: JSON with protected environment details (name, deploy access levels, approval rules)."), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_protected_environment_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, protectedEnvironmentTool("gitlab_protected_environment_protect", "Protect an environment in a GitLab project. Configure deploy access levels, required approvals, and approval rules.\n\nSee also: gitlab_protected_environment_list, gitlab_environment_create\n\nReturns: JSON with the newly protected environment details."), func(ctx context.Context, req *mcp.CallToolRequest, input ProtectInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Protect(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_protected_environment_protect", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, protectedEnvironmentTool("gitlab_protected_environment_update", "Update a protected environment's deploy access levels, approval rules, or required approval count.\n\nSee also: gitlab_protected_environment_get, gitlab_protected_environment_protect\n\nReturns: JSON with the updated protected environment details."), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_protected_environment_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, protectedEnvironmentTool("gitlab_protected_environment_unprotect", "Remove protection from an environment. This action cannot be undone.\n\nSee also: gitlab_protected_environment_list, gitlab_protected_environment_protect\n\nReturns: JSON confirmation of unprotection."), func(ctx context.Context, req *mcp.CallToolRequest, input UnprotectInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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
