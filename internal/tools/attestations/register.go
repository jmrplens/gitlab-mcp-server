// register.go wires attestation MCP tools to the MCP server.

package attestations

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab attestation operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_attestations",
		Title:       toolutil.TitleFromName("gitlab_list_attestations"),
		Description: "List all build attestations for a project matching a subject digest.\n\nReturns: JSON with attestations array. See also: gitlab_download_attestation.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_attestations", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_download_attestation",
		Title:       toolutil.TitleFromName("gitlab_download_attestation"),
		Description: "Download a specific build attestation by IID. Returns the attestation content as base64-encoded data.\n\nReturns: JSON with attestation_iid, size, and content_base64. See also: gitlab_list_attestations.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconSecurity,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DownloadInput) (*mcp.CallToolResult, DownloadOutput, error) {
		start := time.Now()
		out, err := Download(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_download_attestation", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDownloadMarkdown(out)), out, err)
	})
}
