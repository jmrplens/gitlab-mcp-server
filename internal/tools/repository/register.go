// register.go wires repository MCP tools to the MCP server.

package repository

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers repository tree and compare tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_repository_tree",
		Title:       toolutil.TitleFromName("gitlab_repository_tree"),
		Description: "List the files and directories (tree) of a GitLab repository at a given path and ref. 'ref' accepts a branch name, tag name, or commit SHA (defaults to the project's default branch if omitted). Returns file name, type (blob/tree), mode, and path with pagination. Use recursive=true to list all files in subdirectories. For reading file content, use gitlab_file_get instead.\n\nReturns: JSON array of repository tree entries with pagination. See also: gitlab_file_get, gitlab_file_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input TreeInput) (*mcp.CallToolResult, TreeOutput, error) {
		start := time.Now()
		out, err := Tree(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_repository_tree", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatTreeMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_repository_compare",
		Title:       toolutil.TitleFromName("gitlab_repository_compare"),
		Description: "Compare two branches, tags, or commits in a GitLab repository. Returns the list of commits between them and the diffs (changed files) with old/new paths and diff text.\n\nReturns: JSON with commits and file diffs between the two refs. See also: gitlab_commit_diff, gitlab_branch_list.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CompareInput) (*mcp.CallToolResult, CompareOutput, error) {
		start := time.Now()
		out, err := Compare(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_repository_compare", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatCompareMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_repository_contributors",
		Title:       toolutil.TitleFromName("gitlab_repository_contributors"),
		Description: "List repository contributors with commit, addition, and deletion counts. Supports ordering by name, email, or commits and pagination.\n\nReturns: JSON array of contributors with commit statistics and pagination. See also: gitlab_project_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ContributorsInput) (*mcp.CallToolResult, ContributorsOutput, error) {
		start := time.Now()
		out, err := Contributors(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_repository_contributors", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatContributorsMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_repository_merge_base",
		Title:       toolutil.TitleFromName("gitlab_repository_merge_base"),
		Description: "Find the common ancestor (merge base) commit of two or more branches, tags, or commits.\n\nReturns: JSON with the merge base commit details.\n\nSee also: gitlab_repository_compare, gitlab_repository_tree.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MergeBaseInput) (*mcp.CallToolResult, commits.Output, error) {
		start := time.Now()
		out, err := MergeBase(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_repository_merge_base", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(commits.FormatOutputMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_repository_blob",
		Title:       toolutil.TitleFromName("gitlab_repository_blob"),
		Description: "Get a git blob by SHA from a repository. Returns the blob content as a base64-encoded string plus metadata (size, encoding). Requires a blob SHA obtained from gitlab_repository_tree. For decoded plain text, use gitlab_repository_raw_blob instead. For reading files by path, use gitlab_file_get.\n\nReturns: JSON with base64-encoded blob content and metadata. See also: gitlab_repository_tree.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input BlobInput) (*mcp.CallToolResult, BlobOutput, error) {
		start := time.Now()
		out, err := Blob(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_repository_blob", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBlobMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_repository_raw_blob",
		Title:       toolutil.TitleFromName("gitlab_repository_raw_blob"),
		Description: "Get the decoded plain-text content of a git blob by SHA. Unlike gitlab_repository_blob (which returns base64), this returns human-readable text. Requires a blob SHA from gitlab_repository_tree. For reading files by path, use gitlab_file_get instead.\n\nReturns: plain-text blob content. See also: gitlab_repository_blob.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input BlobInput) (*mcp.CallToolResult, RawBlobContentOutput, error) {
		start := time.Now()
		out, err := RawBlobContent(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_repository_raw_blob", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatRawBlobContentMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_repository_archive",
		Title:       toolutil.TitleFromName("gitlab_repository_archive"),
		Description: "Get the download URL for a repository archive. Supports tar.gz, tar.bz2, zip formats and optional SHA/branch/tag/path filters. Returns the URL (does not download binary content).\n\nReturns: archive download URL. See also: gitlab_project_get.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ArchiveInput) (*mcp.CallToolResult, ArchiveOutput, error) {
		start := time.Now()
		out, err := Archive(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_repository_archive", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatArchiveMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_repository_changelog_add",
		Title:       toolutil.TitleFromName("gitlab_repository_changelog_add"),
		Description: "Add changelog data to a changelog file by creating a commit. Requires version string. Optionally specify branch, from/to range, config file, and commit message.\n\nReturns: JSON with the commit details for the changelog update. See also: gitlab_release_create.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AddChangelogInput) (*mcp.CallToolResult, AddChangelogOutput, error) {
		start := time.Now()
		out, err := AddChangelog(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_repository_changelog_add", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatAddChangelogMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_repository_changelog_generate",
		Title:       toolutil.TitleFromName("gitlab_repository_changelog_generate"),
		Description: "Generate changelog data (notes) without committing. Returns the changelog notes as Markdown text. Requires version string.\n\nReturns: Markdown changelog notes for the specified version.\n\nSee also: gitlab_repository_changelog_data.",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GenerateChangelogInput) (*mcp.CallToolResult, ChangelogDataOutput, error) {
		start := time.Now()
		out, err := GenerateChangelogData(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_repository_changelog_generate", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatChangelogDataMarkdown(out)), out, err)
	})
}
