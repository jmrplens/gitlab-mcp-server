// register.go wires mrchanges MCP tools to the MCP server.

package mrchanges

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers MR changes and diff version tools on the given MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_changes_get",
		Title:       toolutil.TitleFromName("gitlab_mr_changes_get"),
		Description: "Get the current file diffs (changes) for a merge request. This is the primary tool for viewing MR changes. Returns old/new paths, diff content, file status (added/deleted/renamed), and file modes. For historical diff versions use gitlab_mr_diff_versions_list. For raw git-apply format use gitlab_mr_raw_diffs.\n\nReturns: JSON with file diffs and change metadata. See also: gitlab_mr_get, gitlab_mr_discussion_create.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, Output, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_changes_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_diff_versions_list",
		Title:       toolutil.TitleFromName("gitlab_mr_diff_versions_list"),
		Description: "List all diff versions (historical snapshots) of a merge request. Returns version IDs, SHAs, state, and timestamps — NOT the actual diffs. Use gitlab_mr_diff_version_get with a version_id to retrieve diffs for a specific version. For current diffs, use gitlab_mr_changes_get instead.\n\nReturns: JSON array of diff versions with pagination. See also: gitlab_mr_changes_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DiffVersionsListInput) (*mcp.CallToolResult, DiffVersionsListOutput, error) {
		start := time.Now()
		out, err := ListDiffVersions(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_diff_versions_list", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDiffVersionsListMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_diff_version_get",
		Title:       toolutil.TitleFromName("gitlab_mr_diff_version_get"),
		Description: "Get a single merge request diff version with its commits and file diffs. Use the version_id from gitlab_mr_diff_versions_list. Optionally request unified diff format.\n\nReturns: JSON with diff version commits and file diffs. See also: gitlab_mr_diff_versions_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DiffVersionGetInput) (*mcp.CallToolResult, DiffVersionOutput, error) {
		start := time.Now()
		out, err := GetDiffVersion(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_diff_version_get", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatDiffVersionGetMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_mr_raw_diffs",
		Title:       toolutil.TitleFromName("gitlab_mr_raw_diffs"),
		Description: "Get the raw unified-diff output for a merge request in plain-text git-apply format. Use this for programmatic diff analysis or patch application. For human-readable structured diffs, use gitlab_mr_changes_get instead.\n\nReturns: plain-text unified diff output.\n\nSee also: gitlab_mr_changes_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RawDiffsInput) (*mcp.CallToolResult, RawDiffsOutput, error) {
		start := time.Now()
		out, err := RawDiffs(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_mr_raw_diffs", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRawDiffsMarkdown(out)), out, err)
	})
}
