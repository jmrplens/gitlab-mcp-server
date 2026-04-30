// register.go wires integrations MCP tools to the MCP server.
package integrations

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// RegisterTools registers all integration tools on the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_list_integrations",
		Title:       toolutil.TitleFromName("gitlab_list_integrations"),
		Description: "List all integrations (services) configured for a project, including their active status.\n\nReturns: JSON array of integrations with status.\n\nSee also: gitlab_get_integration, gitlab_set_jira_integration",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconIntegration,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
		start := time.Now()
		out, err := List(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_list_integrations", start, err)
		return toolutil.WithHints(FormatListMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_get_integration",
		Title:       toolutil.TitleFromName("gitlab_get_integration"),
		Description: "Get details of a specific project integration by slug (e.g. jira, slack, discord, mattermost, microsoft-teams, telegram, datadog, jenkins, emails-on-push, pipelines-email, external-wiki, custom-issue-tracker, drone-ci, github, harbor, matrix, redmine, youtrack, slack-slash-commands, mattermost-slash-commands).\n\nReturns: JSON with integration details.\n\nSee also: gitlab_list_integrations, gitlab_delete_integration",
		Annotations: toolutil.ReadAnnotations,
		Icons:       toolutil.IconIntegration,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetInput) (*mcp.CallToolResult, GetOutput, error) {
		start := time.Now()
		out, err := Get(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_get_integration", start, err)
		return toolutil.WithHints(FormatGetMarkdown(out), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_delete_integration",
		Title:       toolutil.TitleFromName("gitlab_delete_integration"),
		Description: "Delete (disable) a project integration by slug. Supports the same slugs as get, plus 'slack-application' for disabling the GitLab for Slack app.\n\nReturns: confirmation message.\n\nSee also: gitlab_list_integrations, gitlab_get_integration",
		Annotations: toolutil.DeleteAnnotations,
		Icons:       toolutil.IconIntegration,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DeleteInput) (*mcp.CallToolResult, toolutil.DeleteOutput, error) {
		if r := toolutil.ConfirmAction(ctx, req, fmt.Sprintf("Delete integration %s from project %s?", input.Slug, input.ProjectID)); r != nil {
			return r, toolutil.DeleteOutput{}, nil
		}
		start := time.Now()
		err := Delete(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_delete_integration", start, err)
		if err != nil {
			return nil, toolutil.DeleteOutput{}, err
		}
		r, o, _ := toolutil.DeleteResult("integration")
		return r, o, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "gitlab_set_jira_integration",
		Title:       toolutil.TitleFromName("gitlab_set_jira_integration"),
		Description: "Configure the Jira integration for a project. Sets up the connection to a Jira instance with URL, credentials, and event triggers.\n\nReturns: JSON with the configured Jira integration details.\n\nSee also: gitlab_list_integrations, gitlab_get_integration",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconIntegration,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SetJiraInput) (*mcp.CallToolResult, SetJiraOutput, error) {
		start := time.Now()
		out, err := SetJira(ctx, client, input)
		toolutil.LogToolCallAll(ctx, req, "gitlab_set_jira_integration", start, err)
		return toolutil.WithHints(FormatGetMarkdown(GetOutput(out)), out, err)
	})
}
