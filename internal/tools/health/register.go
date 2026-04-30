// register.go wires health MCP tools to the MCP server.
package health

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers the gitlab_server_status diagnostic tool.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_server_status",
		Title:       toolutil.TitleFromName("gitlab_server_status"),
		Description: "Check MCP server health and GitLab connectivity. Returns server version, author, department, repository, GitLab version, authentication status, current user, and response time. Use this to diagnose connection issues.\n\nReturns: JSON with server health and connectivity information.\n\nSee also: gitlab_server_check_update, gitlab_get_metadata",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconHealth,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Check(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_server_status", start, err)
		return toolutil.WithHints(FormatMarkdown(out), out, err)
	})
}
