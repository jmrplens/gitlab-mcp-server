// register.go wires applications MCP tools to the MCP server.

package applications

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all Applications MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_applications",
		Title:       toolutil.TitleFromName("gitlab_list_applications"),
		Description: "List all OAuth2 applications (admin). Params: page, per_page.\n\nReturns: JSON array of OAuth2 applications with pagination.\n\nSee also: gitlab_create_application, gitlab_list_integrations",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_applications", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_application",
		Title:       toolutil.TitleFromName("gitlab_create_application"),
		Description: "Create an OAuth2 application (admin). Params: name (required), redirect_uri (required), scopes (required), confidential.\n\nReturns: JSON with the created application details.\n\nSee also: gitlab_list_applications, gitlab_delete_application",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, CreateOutput, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_application", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatCreateMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_application",
		Title:       toolutil.TitleFromName("gitlab_delete_application"),
		Description: "Delete an OAuth2 application (admin). Params: id (required).\n\nReturns: confirmation message.\n\nSee also: gitlab_list_applications, gitlab_create_application",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete OAuth2 application %d?", input.ID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_application", start, err)
		r, o, _ := toolutil.DeleteResult("application")
		if err != nil {
			return nil, o, err
		}
		return r, o, nil
	})
}
