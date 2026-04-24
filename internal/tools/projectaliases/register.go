// register.go wires project alias MCP tools to the MCP server.

package projectaliases

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab project alias operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_project_aliases",
		Title:       toolutil.TitleFromName("gitlab_list_project_aliases"),
		Description: "List all project aliases (admin-only). Project aliases allow accessing projects via alternative names.\n\nReturns: JSON with aliases array.\n\nSee also: gitlab_get_project_alias, gitlab_create_project_alias",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_project_aliases", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_project_alias",
		Title:       toolutil.TitleFromName("gitlab_get_project_alias"),
		Description: "Get a specific project alias by name (admin-only).\n\nReturns: JSON with alias details.\n\nSee also: gitlab_list_project_aliases",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_project_alias", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_project_alias",
		Title:       toolutil.TitleFromName("gitlab_create_project_alias"),
		Description: "Create a new project alias (admin-only). Maps an alias name to a project ID.\n\nReturns: JSON with created alias details.\n\nSee also: gitlab_list_project_aliases, gitlab_delete_project_alias",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_project_alias", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_project_alias",
		Title:       toolutil.TitleFromName("gitlab_delete_project_alias"),
		Description: "Delete a project alias by name (admin-only).\n\nReturns: JSON with confirmation.\n\nSee also: gitlab_list_project_aliases, gitlab_create_project_alias",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete project alias %q?", input.Name)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_project_alias", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("project alias %q", input.Name))
	})
}
