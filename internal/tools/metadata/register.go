// register.go wires metadata MCP tools to the MCP server.
package metadata

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all Metadata MCP tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_metadata",
		Title:       toolutil.TitleFromName("gitlab_get_metadata"),
		Description: "Get GitLab instance metadata (version, revision, KAS info, enterprise flag).\n\nReturns: JSON with GitLab instance metadata.\n\nSee also: gitlab_server_status",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_metadata", start, err)
		if err != nil {
			return nil, out, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGetMarkdown(out)), out, nil)
	})
}
