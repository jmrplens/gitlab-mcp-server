// register.go wires server update MCP tools to the MCP server.

package serverupdate

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/autoupdate"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers the gitlab_server_check_update and
// gitlab_server_apply_update tools. If updater is nil the tools are not
// registered (auto-update disabled).
func RegisterTools(server *mcp.Server, updater *autoupdate.Updater) {
	if updater == nil {
		return
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_server_check_update",
		Title:       toolutil.TitleFromName("gitlab_server_check_update"),
		Description: "Check if a newer version of the MCP server is available. Returns current version, latest version, release URL, and release notes.\n\nReturns: JSON with version comparison and release information.\n\nSee also: gitlab_server_apply_update, gitlab_server_status",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CheckInput) (*mcp.CallToolResult, CheckOutput, error) {
		start := time.Now()
		out, err := Check(ctx, updater, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_server_check_update", start, err)
		return toolutil.WithHints(FormatCheckMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_server_apply_update",
		Title:       toolutil.TitleFromName("gitlab_server_apply_update"),
		Description: "Download and apply the latest MCP server update. On Linux/macOS the binary is replaced atomically. On Windows the update is downloaded to a staging path with an update script (the running binary cannot be replaced). Use gitlab_server_check_update first to verify an update is available.\n\nReturns: JSON with update status and instructions.\n\nSee also: gitlab_server_check_update",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconServer,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ApplyInput) (*mcp.CallToolResult, ApplyOutput, error) {
		start := time.Now()
		out, err := Apply(ctx, updater, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_server_apply_update", start, err)
		return toolutil.WithHints(FormatApplyMarkdown(out), out, err)
	})
}
