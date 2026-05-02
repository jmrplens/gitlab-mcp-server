package mrcontextcommits

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all MR context commit tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_mr_context_commits",
		Title:       toolutil.TitleFromName("gitlab_list_mr_context_commits"),
		Description: "List context commits associated with a merge request. Context commits are additional commits relevant to the MR but not in the diff. Returns commit SHA, title, and author for each.\n\nReturns: JSON array of context commits.\n\nSee also: gitlab_mr_commits, gitlab_create_mr_context_commits",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_mr_context_commits", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_create_mr_context_commits",
		Title:       toolutil.TitleFromName("gitlab_create_mr_context_commits"),
		Description: "Add context commits to a merge request. Context commits are additional commits relevant to the MR that are not part of the source branch diff. Provide a list of commit SHAs to attach.\n\nReturns: JSON array of the added context commits.\n\nSee also: gitlab_list_mr_context_commits",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CreateInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := Create(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_create_mr_context_commits", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_mr_context_commits",
		Title:       toolutil.TitleFromName("gitlab_delete_mr_context_commits"),
		Description: "Remove context commits from a merge request. Provide the list of commit SHAs to detach from the MR context.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_mr_context_commits",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_mr_context_commits", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		r, o, _ := toolutil.DeleteResult("MR context commits")
		return r, o, nil
	})
}
