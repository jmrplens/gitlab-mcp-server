package appearance

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all appearance tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	appearanceTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconConfig})
	}

	mcp.AddTool(server, appearanceTool("gitlab_get_appearance", "Get current application appearance settings. Requires admin access.\n\nReturns: JSON with appearance settings details.\n\nSee also: gitlab_update_appearance, gitlab_list_broadcast_messages"), func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_appearance", start, err)
		return toolutil.WithHints(FormatGetMarkdown(out), out, err)
	})

	mcp.AddTool(server, appearanceTool("gitlab_update_appearance", "Update application appearance (title, description, messages, PWA settings). Requires admin access.\n\nReturns: JSON with the updated appearance settings.\n\nSee also: gitlab_get_appearance, gitlab_create_broadcast_message"), func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, UpdateOutput, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_appearance", start, err)
		return toolutil.WithHints(FormatUpdateMarkdown(out), out, err)
	})
}
