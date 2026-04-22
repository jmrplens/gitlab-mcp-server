// register.go wires importservice MCP tools to the MCP server.

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
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_import_from_github",
		Title:       toolutil.TitleFromName("gitlab_import_from_github"),
		Description: "Import a repository from GitHub into GitLab\n\nReturns: JSON with import status (project ID, import status, import source).\n\nSee also: gitlab_cancel_github_import, gitlab_start_bulk_import",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ImportFromGitHubInput) (*mcp.CallToolResult, *GitHubImportOutput, error) {
		start := time.Now()
		out, err := ImportFromGitHub(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_import_from_github", start, err)
		if err != nil {
			return nil, nil, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatGitHubImport(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_cancel_github_import",
		Title:       toolutil.TitleFromName("gitlab_cancel_github_import"),
		Description: "Cancel an ongoing GitHub project import\n\nReturns: JSON with cancellation result.\n\nSee also: gitlab_import_from_github, gitlab_get_project_import_status",
		Annotations: toolutil.UpdateAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input CancelGitHubImportInput) (*mcp.CallToolResult, *CancelledImportOutput, error) {
		start := time.Now()
		out, err := CancelGitHubImport(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_cancel_github_import", start, err)
		if err != nil {
			return nil, nil, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatCancelledImport(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_import_github_gists",
		Title:       toolutil.TitleFromName("gitlab_import_github_gists"),
		Description: "Import GitHub gists into GitLab snippets\n\nReturns: JSON confirmation of gists import initiation.\n\nSee also: gitlab_import_from_github, gitlab_list_snippets",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ImportGistsInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		start := time.Now()
		err := ImportGists(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_import_github_gists", start, err)
		r, o, _ := toolutil.DeleteResult("gists import")
		if err != nil {
			return nil, o, err
		}
		return r, o, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_import_from_bitbucket_cloud",
		Title:       toolutil.TitleFromName("gitlab_import_from_bitbucket_cloud"),
		Description: "Import a repository from Bitbucket Cloud into GitLab\n\nReturns: JSON with import status (project ID, import status, import source).\n\nSee also: gitlab_import_from_bitbucket_server, gitlab_import_from_github",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ImportFromBitbucketCloudInput) (*mcp.CallToolResult, *BitbucketCloudImportOutput, error) {
		start := time.Now()
		out, err := ImportFromBitbucketCloud(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_import_from_bitbucket_cloud", start, err)
		if err != nil {
			return nil, nil, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBitbucketCloudImport(out)), out, nil)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_import_from_bitbucket_server",
		Title:       toolutil.TitleFromName("gitlab_import_from_bitbucket_server"),
		Description: "Import a repository from Bitbucket Server into GitLab\n\nReturns: JSON with import status (project ID, import status, import source).\n\nSee also: gitlab_import_from_bitbucket_cloud, gitlab_import_from_github",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconImport,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ImportFromBitbucketServerInput) (*mcp.CallToolResult, *BitbucketServerImportOutput, error) {
		start := time.Now()
		out, err := ImportFromBitbucketServer(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_import_from_bitbucket_server", start, err)
		if err != nil {
			return nil, nil, err
		}
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(FormatBitbucketServerImport(out)), out, nil)
	})
}

// RegisterMeta registers the gitlab_import meta-tool.
func RegisterMeta(server *mcp.Server, client *gitlabclient.Client) {
	routes := toolutil.ActionMap{
		"from_github":           toolutil.RouteAction(client, ImportFromGitHub),
		"cancel_github":         toolutil.DestructiveAction(client, CancelGitHubImport),
		"github_gists":          toolutil.RouteVoidAction(client, ImportGists),
		"from_bitbucket_cloud":  toolutil.RouteAction(client, ImportFromBitbucketCloud),
		"from_bitbucket_server": toolutil.RouteAction(client, ImportFromBitbucketServer),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_import",
		Title: toolutil.TitleFromName("gitlab_import"),
		Description: `Import repositories from external services into GitLab. Use 'action' to specify the operation and 'params' for action-specific parameters.

Actions:
- from_github: Import a repository from GitHub. Params: personal_access_token (required), repo_id (required, int), new_name, target_namespace (required), github_hostname, optional_stages, timeout_strategy
- cancel_github: Cancel an ongoing GitHub import. Params: project_id (required, int)
- github_gists: Import GitHub gists as GitLab snippets. Params: personal_access_token (required)
- from_bitbucket_cloud: Import from Bitbucket Cloud. Params: bitbucket_username (required), bitbucket_app_password (required), repo_path (required), target_namespace (required), new_name
- from_bitbucket_server: Import from Bitbucket Server. Params: bitbucket_server_url (required), bitbucket_server_username (required), personal_access_token (required), bitbucket_server_project (required), bitbucket_server_repo (required), new_name, new_namespace, timeout_strategy`,
		Annotations: toolutil.DeriveAnnotations(routes),
		Icons:       toolutil.IconImport,
		InputSchema: toolutil.MetaToolSchema(routes),
	}, toolutil.MakeMetaHandler("gitlab_import", routes, nil))
}
