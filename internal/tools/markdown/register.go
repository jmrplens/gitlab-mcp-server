// register.go wires markdown MCP tools to the MCP server.

package markdown

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all markdown tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_render_markdown",
		Title:       toolutil.TitleFromName("gitlab_render_markdown"),
		Description: "Render arbitrary markdown text to HTML using the GitLab API. Supports GitLab Flavored Markdown (GFM) and project-scoped references.\n\nReturns: JSON with the rendered HTML output.\n\nSee also: gitlab_wiki_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RenderInput) (*mcp.CallToolResult, RenderOutput, error) {
		start := time.Now()
		out, err := Render(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_render_markdown", start, err)
		return toolutil.WithHints(FormatRenderMarkdown(out), out, err)
	})
}
