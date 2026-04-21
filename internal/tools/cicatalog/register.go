// register.go wires CI/CD Catalog MCP tools to the MCP server.

package cicatalog

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers CI/CD Catalog tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_catalog_resources",
		Title:       toolutil.TitleFromName("gitlab_list_catalog_resources"),
		Description: "List CI/CD Catalog resources. Search the catalog of reusable CI/CD components. Supports filtering by search text, scope (ALL or NAMESPACED), and sorting. Returns: paginated list with name, stars, forks, and latest version.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_catalog_resources", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatListMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_catalog_resource",
		Title:       toolutil.TitleFromName("gitlab_get_catalog_resource"),
		Description: "Get a CI/CD Catalog resource by GID or project full path. Returns: full resource details including README, released versions, components with their input parameters and include paths.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconPackage,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_catalog_resource", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGetMarkdown(out)), out, err)
	})
}
