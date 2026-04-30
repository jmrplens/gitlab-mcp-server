// register.go wires group wiki MCP tools to the MCP server.
package groupwikis

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers group wiki tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_wiki_list",
		Title:       toolutil.TitleFromName("gitlab_group_wiki_list"),
		Description: "List all wiki pages in a GitLab group. Optionally include page content.\n\nReturns: list of wiki pages with title, slug, format, and encoding. See also: gitlab_group_wiki_get, gitlab_wiki_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconWiki,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_wiki_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_wiki_get",
		Title:       toolutil.TitleFromName("gitlab_group_wiki_get"),
		Description: "Get a single wiki page from a GitLab group by slug.\n\nReturns: wiki page with title, slug, format, content, and encoding. See also: gitlab_group_wiki_list, gitlab_group_wiki_edit.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconWiki,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_wiki_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_wiki_create",
		Title:       toolutil.TitleFromName("gitlab_group_wiki_create"),
		Description: "Create a new wiki page in a GitLab group.\n\nReturns: created wiki page with title, slug, format, and content. See also: gitlab_group_wiki_list, gitlab_group_wiki_edit.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconWiki,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_wiki_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_wiki_edit",
		Title:       toolutil.TitleFromName("gitlab_group_wiki_edit"),
		Description: "Edit an existing wiki page in a GitLab group.\n\nReturns: updated wiki page with title, slug, format, and content. See also: gitlab_group_wiki_get, gitlab_group_wiki_create.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconWiki,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EditInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Edit(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_wiki_edit", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_group_wiki_delete",
		Title:       toolutil.TitleFromName("gitlab_group_wiki_delete"),
		Description: "Delete a wiki page from a GitLab group.\n\nReturns: confirmation of deletion. See also: gitlab_group_wiki_list, gitlab_group_wiki_get.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconWiki,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_group_wiki_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult("group wiki page")
	})
}
