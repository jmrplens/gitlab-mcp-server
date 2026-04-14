// register.go wires releases MCP tools to the MCP server.

package releases

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all release-related MCP tools on the given server.
// Each tool is configured with appropriate annotations indicating whether the
// operation is read-only or destructive.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_release_create",
		Title:       toolutil.TitleFromName("gitlab_release_create"),
		Description: "Create a GitLab release. If the tag_name does not exist yet, provide 'ref' (branch name or commit SHA) and GitLab will auto-create a lightweight tag — no need to call gitlab_tag_create first. Use 'tag_message' for an annotated tag. The response includes 'assets_sources' with auto-generated tar.gz and zip archive URLs — use those real URLs instead of constructing download links manually.\n\nReturns: tag_name, name, description, author, commit, created_at, released_at, assets_sources, and assets_links. See also: gitlab_tag_create, gitlab_release_link_create.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconRelease,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_release_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_release_update",
		Title:       toolutil.TitleFromName("gitlab_release_update"),
		Description: "Update an existing GitLab release's title, description, milestones, or released date. Identified by project and tag name. Only specified fields are changed. Returns: tag_name, name, description, author, dates, commit SHA, milestones, and asset links. See also: gitlab_release_get, gitlab_release_delete.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconRelease,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_release_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_release_delete",
		Title:       toolutil.TitleFromName("gitlab_release_delete"),
		Description: "Delete a GitLab release. The underlying Git tag is preserved and not deleted.\n\nReturns: deleted release details with tag_name, name, and description. See also: gitlab_release_list, gitlab_tag_delete.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconRelease,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, Output, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete release for tag %q in project %q?", input.TagName, input.ProjectID)); r != nil {
			return r, Output{}, nil
		}
		start := time.Now()
		out, err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_release_delete", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_release_get",
		Title:       toolutil.TitleFromName("gitlab_release_get"),
		Description: "Retrieve detailed information about a specific GitLab release by its tag name, including title, description, author, creation date, and associated assets. The response includes 'assets_sources' (auto-generated tar.gz/zip archive URLs) and 'assets_links' (manually added links such as package download URLs).\n\nReturns: tag_name, name, description, author, commit, created_at, released_at, assets_sources, and assets_links. See also: gitlab_release_update, gitlab_release_link_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRelease,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		if err != nil && toolutil.IsHTTPStatus(err, 404) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_release_get", start, nil)
			return toolutil.NotFoundResult("Release", fmt.Sprintf("tag %q in project %s", input.TagName, input.ProjectID),
				"Use gitlab_release_list with project_id to list releases",
				"Verify the tag_name is correct (case-sensitive)",
				"A tag may exist without a release — check with gitlab_tag_get",
			), Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_release_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultAnnotated(FormatMarkdown(out), toolutil.ContentDetail), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_release_list",
		Title:       toolutil.TitleFromName("gitlab_release_list"),
		Description: "List all releases for a GitLab project ordered by release date. Returns paginated results including each release's metadata, tag, and asset links.\n\nReturns: paginated list of releases with tag_name, name, description, author, and assets. See also: gitlab_release_get, gitlab_tag_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRelease,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_release_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_release_latest",
		Title:       toolutil.TitleFromName("gitlab_release_latest"),
		Description: "Get the latest release for a GitLab project. Returns the most recently created release without needing to know the tag name.\n\nReturns: tag_name, name, description, author, commit, created_at, released_at, and assets. See also: gitlab_release_list, gitlab_release_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconRelease,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetLatestInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := GetLatest(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_release_latest", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatMarkdown(out)), out, err)
	})
}
