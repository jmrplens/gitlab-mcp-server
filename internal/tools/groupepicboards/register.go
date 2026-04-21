// register.go wires group epic board MCP tools to the MCP server.

package groupepicboards

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab group epic board operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_epic_board_list",
		Title:       toolutil.TitleFromName("gitlab_group_epic_board_list"),
		Description: "List all epic boards in a GitLab group. Supports pagination.\n\nReturns: JSON with boards array and pagination metadata. See also: gitlab_group_epic_board_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_epic_board_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_epic_board_get",
		Title:       toolutil.TitleFromName("gitlab_group_epic_board_get"),
		Description: "Get a single epic board in a GitLab group by its ID, including board lists (columns) and labels.\n\nReturns: JSON with board details. See also: gitlab_group_epic_board_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_epic_board_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})
}
