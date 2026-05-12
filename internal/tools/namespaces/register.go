package namespaces

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers individual namespace tools.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_namespace_list",
		Title:       toolutil.TitleFromName("gitlab_namespace_list"),
		Description: "List all namespaces visible to the authenticated user. Supports filtering by search, owned-only, top-level-only, and pagination.\n\nReturns: JSON array of namespaces with pagination.\n\nSee also: gitlab_namespace_get, gitlab_group_list",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_namespace_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_namespace_get",
		Title:       toolutil.TitleFromName("gitlab_namespace_get"),
		Description: "Get details of a single namespace by ID or path.\n\nReturns: JSON with namespace details.\n\nSee also: gitlab_namespace_list, gitlab_namespace_search",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_namespace_get", start, err)
		result := FormatMarkdown(out)
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_namespace_exists",
		Title:       toolutil.TitleFromName("gitlab_namespace_exists"),
		Description: "Check whether a namespace path exists (is taken). Returns availability and suggested alternatives if the path is taken.\n\nReturns: JSON with namespace availability status.\n\nSee also: gitlab_namespace_get, gitlab_namespace_search",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ExistsInput) (*mcp.CallToolResult, ExistsOutput, error) {
		start := time.Now()
		out, err := Exists(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_namespace_exists", start, err)
		return toolutil.WithHints(FormatExistsMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_namespace_search",
		Title:       toolutil.TitleFromName("gitlab_namespace_search"),
		Description: "Search namespaces by query string. Returns matching namespaces with pagination.\n\nReturns: JSON array of matching namespaces with pagination.\n\nSee also: gitlab_namespace_list, gitlab_namespace_exists",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := Search(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_namespace_search", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})
}

// markdownForResult dispatches namespace output types to their Markdown formatter.
func markdownForResult(result any) *mcp.CallToolResult {
	switch v := result.(type) {
	case ListOutput:
		return FormatListMarkdown(v)
	case Output:
		return FormatMarkdown(v)
	case ExistsOutput:
		return FormatExistsMarkdown(v)
	default:
		return nil
	}
}
