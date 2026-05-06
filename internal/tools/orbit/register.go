package orbit

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers Orbit MCP tools. Callers gate this package to GitLab.com and the Enterprise catalog.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_orbit_status",
		Title:       toolutil.TitleFromName("gitlab_orbit_status"),
		Description: "Get experimental GitLab Orbit Knowledge Graph cluster status. This tool is registered only when the MCP server is connected to GitLab.com with the Enterprise catalog enabled. Returns: status, version, component health, or formatted LLM text.\n\nSee also: gitlab_orbit_graph_status, gitlab_orbit_schema.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input StatusInput) (*mcp.CallToolResult, StatusOutput, error) {
		start := time.Now()
		out, err := Status(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_orbit_status", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatStatusMarkdown(out), toolutil.ContentDetail), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_orbit_schema",
		Title:       toolutil.TitleFromName("gitlab_orbit_schema"),
		Description: "Get the experimental GitLab Orbit Knowledge Graph ontology. This tool is registered only when the MCP server is connected to GitLab.com with the Enterprise catalog enabled. Returns: schema version, domains, nodes, and edges.\n\nSee also: gitlab_orbit_tools, gitlab_orbit_query.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SchemaInput) (*mcp.CallToolResult, SchemaOutput, error) {
		start := time.Now()
		out, err := Schema(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_orbit_schema", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatSchemaMarkdown(out), toolutil.ContentDetail), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_orbit_tools",
		Title:       toolutil.TitleFromName("gitlab_orbit_tools"),
		Description: "Get the experimental GitLab Orbit MCP tool manifest from GitLab.com. This tool is registered only when the MCP server is connected to GitLab.com with the Enterprise catalog enabled. Returns: Orbit tool names, descriptions, and parameter schemas.\n\nSee also: gitlab_orbit_schema, gitlab_orbit_query.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ToolsInput) (*mcp.CallToolResult, ToolsOutput, error) {
		start := time.Now()
		out, err := Tools(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_orbit_tools", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatToolsMarkdown(out), toolutil.ContentList), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_orbit_query",
		Title:       toolutil.TitleFromName("gitlab_orbit_query"),
		Description: "Execute an experimental GitLab Orbit Knowledge Graph query on GitLab.com. This read-only POST endpoint is registered only when the MCP server is connected to GitLab.com with the Enterprise catalog enabled. Query shape is provided by gitlab_orbit_tools and is passed through as JSON. Returns: raw result payload, query type, row count, and compiled query strings.\n\nSee also: gitlab_orbit_tools, gitlab_orbit_schema.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input QueryInput) (*mcp.CallToolResult, QueryOutput, error) {
		start := time.Now()
		out, err := Query(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_orbit_query", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatQueryMarkdown(out), toolutil.ContentDetail), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_orbit_graph_status",
		Title:       toolutil.TitleFromName("gitlab_orbit_graph_status"),
		Description: "Get experimental GitLab Orbit Knowledge Graph indexing status for one namespace, project, or full path on GitLab.com. This tool is registered only when the MCP server is connected to GitLab.com with the Enterprise catalog enabled. Returns: indexed project counts, domain counts, and indexing state.\n\nSee also: gitlab_orbit_status, gitlab_orbit_schema.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconAnalytics,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GraphStatusInput) (*mcp.CallToolResult, GraphStatusOutput, error) {
		start := time.Now()
		out, err := GraphStatus(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_orbit_graph_status", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatGraphStatusMarkdown(out), toolutil.ContentDetail), out, err)
	})
}

// RegisterMeta registers the gitlab_orbit meta-tool. Callers gate this package to GitLab.com and the Enterprise catalog.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"status":       toolutil.RouteAction(client, Status),
		"schema":       toolutil.RouteAction(client, Schema),
		"tools":        toolutil.RouteAction(client, Tools),
		"query":        toolutil.RouteAction(client, Query),
		"graph_status": toolutil.RouteAction(client, GraphStatus),
	}

	toolutil.AddReadOnlyMetaTool(server, "gitlab_orbit", `Experimental GitLab.com-only Orbit Knowledge Graph operations. Read-only.
When to use: inspect Orbit availability and graph indexing, discover the Orbit MCP tool/query schema, or execute a Knowledge Graph query on GitLab.com.
NOT for: GitLab self-managed instances (Orbit is GitLab.com-only), standard project/group/issue/MR CRUD (use gitlab_project, gitlab_group, gitlab_issue, gitlab_merge_request), or full-text search (use gitlab_search).

This meta-tool is registered only when the MCP server is connected to https://gitlab.com and the Enterprise/Premium catalog is enabled. Orbit itself is experimental and also depends on GitLab's knowledge_graph feature flag and namespace/project access.

Actions:
- status: response_format (raw/llm) — cluster health and component status.
- schema: expand, format (raw/llm) — graph ontology domains, nodes, and edges.
- tools: no params — Orbit MCP tool manifest and parameter schemas.
- query: query*, response_format (raw/llm) — execute an Orbit query DSL object. First call gitlab_orbit with action "tools" (or use gitlab_orbit_tools) to inspect the live query schema.
- graph_status: exactly one of namespace_id, project_id, full_path; response_format (raw/llm) — graph indexing state.

Errors: 404 means Orbit is not enabled or not available; 403 means the token lacks access to a Knowledge Graph-enabled namespace/project; 503 means Orbit is temporarily unavailable.`, routes, toolutil.IconAnalytics, toolutil.MarkdownForResult)
}
