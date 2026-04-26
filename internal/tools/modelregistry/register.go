// Package modelregistry register.go wires ML model registry MCP tools to the MCP server.
package modelregistry

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab ML model registry.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_download_ml_model_package",
		Title:       toolutil.TitleFromName("gitlab_download_ml_model_package"),
		Description: "Download a machine learning model package file from the GitLab model registry.\n\nReturns: JSON with base64-encoded file content, filename, and size.\n\nSee also: gitlab_package_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DownloadInput) (*mcp.CallToolResult, DownloadOutput, error) {
		start := time.Now()
		out, err := Download(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_download_ml_model_package", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDownloadMarkdown(out)), out, err)
	})
}
