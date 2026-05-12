package avatar

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all avatar tools with the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_avatar",
		Title:       toolutil.TitleFromName("gitlab_get_avatar"),
		Description: "Get the avatar URL for an email address.\n\nReturns: JSON with the avatar URL.\n\nSee also: gitlab_user_current.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_avatar", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, nil)
	})
}
