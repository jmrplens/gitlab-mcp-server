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
	specs := ActionSpecs(client)
	applicationTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconConfig})
	}

	mcp.AddTool(server, applicationTool("gitlab_list_applications", "List all OAuth2 applications (admin). Params: page, per_page.\n\nReturns: JSON array of OAuth2 applications with pagination.\n\nSee also: gitlab_create_application, gitlab_list_integrations"), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_applications", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, applicationTool("gitlab_create_application", "Create an OAuth2 application (admin). Params: name (required), redirect_uri (required), scopes (required), confidential.\n\nReturns: JSON with the created application details.\n\nSee also: gitlab_list_applications, gitlab_delete_application"), func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, CreateOutput, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_application", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatCreateMarkdown(out)), out, nil)
	})

	mcp.AddTool(server, applicationTool("gitlab_delete_application", "Delete an OAuth2 application (admin). Params: id (required).\n\nReturns: confirmation message.\n\nSee also: gitlab_list_applications, gitlab_create_application"), func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
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
