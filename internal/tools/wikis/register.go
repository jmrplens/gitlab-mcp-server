// register.go wires wikis MCP tools to the MCP server.

package wikis

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers wiki-related tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_wiki_list",
		Title:       toolutil.TitleFromName("gitlab_wiki_list"),
		Description: "List all wiki pages in a GitLab project. Optionally include page content by setting with_content=true.\n\nReturns: JSON with wiki pages array including title, slug, format, and encoding. See also: gitlab_wiki_get, gitlab_wiki_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconWiki,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_wiki_list", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_wiki_get",
		Title:       toolutil.TitleFromName("gitlab_wiki_get"),
		Description: "Get a single wiki page by slug. Set render_html=true to get HTML-rendered content. Set version to a commit SHA to retrieve a specific historical version of the page. Use gitlab_wiki_list to discover available page slugs.\n\nReturns: JSON with wiki page content, title, slug, format, and encoding. See also: gitlab_wiki_update.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconWiki,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_wiki_get", start, nil)
			return toolutil.NotFoundResult("Wiki Page", fmt.Sprintf("slug %q in project %s", input.Slug, input.ProjectID),
				"Use gitlab_wiki_list with project_id to list wiki pages",
				"Wiki slugs are case-sensitive and may differ from the page title",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_wiki_get", start, err)
		result := FormatOutputMarkdown(out)
		if err == nil && out.Slug != "" && string(input.ProjectID) != "" {
			toolutil.EmbedResourceJSON(result,
				fmt.Sprintf("gitlab://project/%s/wiki/%s", url.PathEscape(string(input.ProjectID)), url.PathEscape(out.Slug)),
				out)
		}
		return toolutil.WithHints(result, out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_wiki_create",
		Title:       toolutil.TitleFromName("gitlab_wiki_create"),
		Description: "Create a new wiki page in a GitLab project. Supports Markdown (default), RDoc, AsciiDoc, and Org formats.\n\nReturns: JSON with created wiki page including title, slug, and content. See also: gitlab_wiki_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconWiki,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_wiki_create", start, err)
		return toolutil.WithHints(FormatOutputMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_wiki_update",
		Title:       toolutil.TitleFromName("gitlab_wiki_update"),
		Description: "Update an existing wiki page by slug. Can change the title, content, and format. At least one of title, content, or format must be provided.\n\nReturns: JSON with updated wiki page including title, slug, and content. See also: gitlab_wiki_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconWiki,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_wiki_update", start, err)
		return toolutil.WithHints(FormatOutputMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_wiki_delete",
		Title:       toolutil.TitleFromName("gitlab_wiki_delete"),
		Description: "Delete a wiki page by slug. This action cannot be undone. Use gitlab_wiki_list to find available page slugs.\n\nReturns: JSON with deletion confirmation. See also: gitlab_wiki_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconWiki,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete wiki page %q from project %s?", input.Slug, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_wiki_delete", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		return toolutil.DeleteResult(fmt.Sprintf("wiki page %q from project %s", input.Slug, input.ProjectID))
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_wiki_upload_attachment",
		Title:       toolutil.TitleFromName("gitlab_wiki_upload_attachment"),
		Description: "Upload a file attachment to a project wiki. Provide file content as base64 or a local file path.\n\nReturns: JSON with file path and Markdown embed snippet. See also: gitlab_wiki_create.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconWiki,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UploadAttachmentInput) (*mcp.CallToolResult, AttachmentOutput, error) {
		start := time.Now()
		out, err := UploadAttachment(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_wiki_upload_attachment", start, err)
		return toolutil.WithHints(FormatAttachmentMarkdown(out), out, err)
	})
}
