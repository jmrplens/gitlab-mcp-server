package importservice

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all import tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	specs := ActionSpecs(client)
	importServiceTool := func(name, description string) *mcp.Tool {
		return toolutil.MustIndividualToolFromSpecs(specs, name, toolutil.IndividualToolProjectionOptions{Description: description, Icons: toolutil.IconImport})
	}

	mcp.AddTool(server, importServiceTool("gitlab_import_from_github", "Import a repository from GitHub into GitLab\n\nReturns: JSON with import status (project ID, import status, import source).\n\nSee also: gitlab_cancel_github_import, gitlab_start_bulk_import"), func(ctx context.Context, req *mcp.CallToolRequest, input ImportFromGitHubInput) (*mcp.CallToolResult, *GitHubImportOutput, error) {
		start := time.Now()
		out, err := ImportFromGitHub(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_import_from_github", start, err)
		if err != nil {
			return nil, nil, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGitHubImport(out)), out, nil)
	})

	mcp.AddTool(server, importServiceTool("gitlab_cancel_github_import", "Cancel an ongoing GitHub project import\n\nReturns: JSON with cancellation result.\n\nSee also: gitlab_import_from_github, gitlab_get_project_import_status"), func(ctx context.Context, req *mcp.CallToolRequest, input CancelGitHubImportInput) (*mcp.CallToolResult, *CancelledImportOutput, error) {
		start := time.Now()
		out, err := CancelGitHubImport(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_cancel_github_import", start, err)
		if err != nil {
			return nil, nil, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatCancelledImport(out)), out, nil)
	})

	mcp.AddTool(server, importServiceTool("gitlab_import_github_gists", "Import GitHub gists into GitLab snippets\n\nReturns: JSON confirmation of gists import initiation.\n\nSee also: gitlab_import_from_github, gitlab_snippet_list"), func(ctx context.Context, req *mcp.CallToolRequest, input ImportGistsInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := ImportGists(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_import_github_gists", start, err)
		r, o, _ := toolutil.DeleteResult("gists import")
		if err != nil {
			return nil, o, err
		}
		return r, o, nil
	})

	mcp.AddTool(server, importServiceTool("gitlab_import_from_bitbucket_cloud", "Import a repository from Bitbucket Cloud into GitLab\n\nReturns: JSON with import status (project ID, import status, import source).\n\nSee also: gitlab_import_from_bitbucket_server, gitlab_import_from_github"), func(ctx context.Context, req *mcp.CallToolRequest, input ImportFromBitbucketCloudInput) (*mcp.CallToolResult, *BitbucketCloudImportOutput, error) {
		start := time.Now()
		out, err := ImportFromBitbucketCloud(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_import_from_bitbucket_cloud", start, err)
		if err != nil {
			return nil, nil, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBitbucketCloudImport(out)), out, nil)
	})

	mcp.AddTool(server, importServiceTool("gitlab_import_from_bitbucket_server", "Import a repository from Bitbucket Server into GitLab\n\nReturns: JSON with import status (project ID, import status, import source).\n\nSee also: gitlab_import_from_bitbucket_cloud, gitlab_import_from_github"), func(ctx context.Context, req *mcp.CallToolRequest, input ImportFromBitbucketServerInput) (*mcp.CallToolResult, *BitbucketServerImportOutput, error) {
		start := time.Now()
		out, err := ImportFromBitbucketServer(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_import_from_bitbucket_server", start, err)
		if err != nil {
			return nil, nil, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBitbucketServerImport(out)), out, nil)
	})
}
