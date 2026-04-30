// register.go wires dependency MCP tools to the MCP server.
package dependencies

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers dependency tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_project_dependencies",
		Title:       toolutil.TitleFromName("gitlab_list_project_dependencies"),
		Description: "List dependencies for a GitLab project. Supports filtering by package manager. Returns: paginated list with name, version, package manager, file path, vulnerabilities, and licenses. See also: gitlab_create_dependency_list_export, gitlab_list_vulnerabilities.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := ListDeps(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_project_dependencies", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_dependency_list_export",
		Title:       toolutil.TitleFromName("gitlab_create_dependency_list_export"),
		Description: "Create a dependency list export (SBOM) for a pipeline. Returns: export ID and status. Use gitlab_get_dependency_list_export to check status, then gitlab_download_dependency_list_export to download. See also: gitlab_get_dependency_list_export, gitlab_download_dependency_list_export.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateExportInput) (*mcp.CallToolResult, ExportOutput, error) {
		start := time.Now()
		out, err := CreateExport(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_dependency_list_export", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatExportMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_dependency_list_export",
		Title:       toolutil.TitleFromName("gitlab_get_dependency_list_export"),
		Description: "Check the status of a dependency list export. Returns: export ID, completion status, and download URL when ready. See also: gitlab_create_dependency_list_export, gitlab_download_dependency_list_export.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetExportInput) (*mcp.CallToolResult, ExportOutput, error) {
		start := time.Now()
		out, err := GetExport(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_dependency_list_export", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatExportMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_download_dependency_list_export",
		Title:       toolutil.TitleFromName("gitlab_download_dependency_list_export"),
		Description: "Download a completed dependency list export (CycloneDX SBOM JSON). Returns: raw SBOM content (limited to 1MB). See also: gitlab_create_dependency_list_export, gitlab_get_dependency_list_export.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DownloadExportInput) (*mcp.CallToolResult, DownloadOutput, error) {
		start := time.Now()
		out, err := DownloadExport(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_download_dependency_list_export", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDownloadMarkdown(out)), out, err)
	})
}
