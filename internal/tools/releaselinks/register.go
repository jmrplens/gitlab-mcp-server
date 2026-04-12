// register.go wires releaselinks MCP tools to the MCP server.

package releaselinks

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MCP tools for GitLab release link operations.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_release_link_create",
		Title:       toolutil.TitleFromName("gitlab_release_link_create"),
		Description: "Add an asset link to a GitLab release. IMPORTANT: (1) When linking to uploaded packages, use the real 'url' value returned by gitlab_package_publish — do NOT construct package URLs manually. (2) The 'name' MUST be the exact filename (e.g. 'checksums.txt.asc'), NEVER add descriptive suffixes. Consider using gitlab_package_publish_and_link instead to upload and link in one step. Supports link types: runbook, package, image, or other. Links appear in the release's assets section.\n\nReturns: JSON with the created release link (ID, name, URL, type). See also: gitlab_release_get.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconLink,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_release_link_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_release_link_create_batch",
		Title:       toolutil.TitleFromName("gitlab_release_link_create_batch"),
		Description: "Add multiple asset links to a GitLab release in a single call. Use this instead of calling gitlab_release_link_create multiple times. Each link requires a name and url; link_type is optional (runbook, package, image, other). IMPORTANT: (1) For package links, use the real 'url' values returned by gitlab_package_publish. (2) Link names MUST be exact filenames — never add descriptive suffixes.\n\nReturns: JSON with arrays of created links and any failures. See also: gitlab_release_link_list.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconLink,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateBatchInput) (*mcp.CallToolResult, CreateBatchOutput, error) {
		start := time.Now()
		out, err := CreateBatch(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_release_link_create_batch", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBatchMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_release_link_delete",
		Title:       toolutil.TitleFromName("gitlab_release_link_delete"),
		Description: "Remove an asset link from a GitLab release by its link ID.\n\nReturns: JSON with the deleted release link details. See also: gitlab_release_link_list.",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconLink,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, Output, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete release link %d from tag %q in project %q?", input.LinkID, input.TagName, input.ProjectID)); r != nil {
			return r, Output{}, nil
		}
		start := time.Now()
		out, err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_release_link_delete", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_release_link_list",
		Title:       toolutil.TitleFromName("gitlab_release_link_list"),
		Description: "List all asset links attached to a specific GitLab release identified by tag name. Returns link names, URLs, types, and IDs.\n\nReturns: JSON with array of release links and pagination info. See also: gitlab_release_get, gitlab_release_link_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLink,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_release_link_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_release_link_get",
		Title:       toolutil.TitleFromName("gitlab_release_link_get"),
		Description: "Get details of a specific release asset link by its ID, including name, URL, type, and whether it is external.\n\nReturns: JSON with release link details (ID, name, URL, type, external flag). See also: gitlab_release_link_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconLink,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_release_link_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_release_link_update",
		Title:       toolutil.TitleFromName("gitlab_release_link_update"),
		Description: "Update an existing release asset link. Can change name, URL, filepath, direct asset path, or link type. Only specified fields are changed.\n\nReturns: JSON with the updated release link details. See also: gitlab_release_link_get.",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconLink,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_release_link_update", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})
}
