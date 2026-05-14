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
	specs := ActionSpecs(client)
	attestationTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconCompliance})
	}

	mcp.AddTool(server, attestationTool("gitlab_list_attestations", "List all build attestations for a project matching a subject digest.\n\nReturns: JSON with attestations array. See also: gitlab_download_attestation."), func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_attestations", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, attestationTool("gitlab_download_attestation", "Download a specific build attestation by IID. Returns the attestation content as base64-encoded data.\n\nReturns: JSON with attestation_iid, size, and content_base64. See also: gitlab_list_attestations."), func(ctx context.Context, req *mcp.CallToolRequest, input DownloadInput) (*mcp.CallToolResult, DownloadOutput, error) {
		start := time.Now()
		out, err := Download(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_download_attestation", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDownloadMarkdown(out)), out, err)
	})
}
