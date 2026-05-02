package repositorysubmodules

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers the repository submodule tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_repository_submodules",
		Title:       toolutil.TitleFromName("gitlab_list_repository_submodules"),
		Description: "List all Git submodules defined in a repository. Parses .gitmodules and enriches each entry with the current commit SHA pointer and resolved project path.\n\nReturns: JSON array of submodules with commit SHAs.\n\nSee also: gitlab_read_repository_submodule_file, gitlab_update_repository_submodule",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_repository_submodules", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_read_repository_submodule_file",
		Title:       toolutil.TitleFromName("gitlab_read_repository_submodule_file"),
		Description: "Read a file from inside a Git submodule transparently. Resolves the submodule's target project and pinned commit SHA, then fetches the file content. No need to manually find the submodule's project or commit.\n\nReturns: JSON with the file content.\n\nSee also: gitlab_list_repository_submodules, gitlab_file_get",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReadInput) (*mcp.CallToolResult, ReadOutput, error) {
		start := time.Now()
		out, err := Read(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_read_repository_submodule_file", start, err)
		return toolutil.WithHints(FormatReadMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_update_repository_submodule",
		Title:       toolutil.TitleFromName("gitlab_update_repository_submodule"),
		Description: "Update an existing submodule reference in a GitLab repository to point to a new commit SHA.\n\nReturns: JSON with the updated submodule reference.\n\nSee also: gitlab_list_repository_submodules",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input UpdateInput) (*mcp.CallToolResult, UpdateOutput, error) {
		start := time.Now()
		out, err := Update(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_update_repository_submodule", start, err)
		return toolutil.WithHints(FormatUpdateMarkdown(out), out, err)
	})
}
