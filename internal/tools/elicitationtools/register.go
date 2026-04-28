// register.go wires elicitationtools MCP tools to the MCP server.

package elicitationtools

import (
	"context"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/elicitation"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const descElicitRequired = "Requires the MCP client to support the elicitation capability."

// RegisterTools wires elicitation-powered interactive tools to the MCP server.
func RegisterTools(server *mcp.Server, client *gitlabclient.Client) {
	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_interactive_issue_create",
		Title: toolutil.TitleFromName("gitlab_interactive_issue_create"),
		Description: "Create a GitLab issue through step-by-step prompts, with explicit confirmation before calling the GitLab API. Cancellation at any prompt aborts without creating the issue.\n\n" +
			"After invocation, the tool elicits in order:\n" +
			"- title (string, required) — issue title.\n" +
			"- description (string, optional, multi-line, Markdown) — leave empty to skip.\n" +
			"- labels (string, optional) — comma-separated; trimmed and deduped server-side.\n" +
			"- confidential (boolean, optional) — yes/no confirmation; defaults to public when declined.\n" +
			"- confirm (boolean, required) — final yes/no review of the assembled summary.\n\n" +
			"When to use: human-in-the-loop issue creation. " +
			"NOT for: scripted/programmatic creation — use gitlab_issue (action='create') with all fields pre-supplied.\n\n" +
			descElicitRequired + " If unsupported, returns a structured error naming gitlab_issue (action='create') as the alternative.\n\n" +
			"Returns: JSON with the created issue (id, issue_iid, web_url, title, state).\n\nSee also: gitlab_issue.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input IssueInput) (*mcp.CallToolResult, issues.Output, error) {
		start := time.Now()
		out, err := IssueCreate(ctx, req, client, input)
		if errors.Is(err, elicitation.ErrElicitationNotSupported) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_interactive_issue_create", start, err)
			return UnsupportedResult("gitlab_interactive_issue_create"), issues.Output{}, nil
		}
		if errors.Is(err, elicitation.ErrCancelled) || errors.Is(err, elicitation.ErrDeclined) {
			// Cancellation is an expected outcome, not an error.
			toolutil.LogToolCallAll(ctx, req, "gitlab_interactive_issue_create", start, nil)
			return CancelledResult("Issue creation cancelled by user."), issues.Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_interactive_issue_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(issues.FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_interactive_mr_create",
		Title: toolutil.TitleFromName("gitlab_interactive_mr_create"),
		Description: "Create a GitLab merge request through step-by-step prompts, with explicit confirmation before calling the GitLab API. Cancellation at any prompt aborts without creating the MR.\n\n" +
			"After invocation, the tool elicits in order:\n" +
			"- source_branch (string, required) — branch with the changes to merge.\n" +
			"- target_branch (string, required) — branch to merge into (e.g. main, develop).\n" +
			"- title (string, required) — MR title.\n" +
			"- description (string, optional, multi-line, Markdown) — leave empty to skip.\n" +
			"- labels (string, optional) — comma-separated; trimmed and deduped server-side.\n" +
			"- remove_source_branch (boolean, optional) — yes/no confirmation; default unset.\n" +
			"- squash (boolean, optional) — yes/no confirmation; default unset.\n" +
			"- confirm (boolean, required) — final yes/no review of the assembled summary.\n\n" +
			"When to use: human-in-the-loop MR creation. " +
			"NOT for: scripted/programmatic creation — use gitlab_merge_request (action='create') with all fields pre-supplied.\n\n" +
			descElicitRequired + " If unsupported, returns a structured error naming gitlab_merge_request (action='create') as the alternative.\n\n" +
			"Returns: JSON with the created MR (id, mr_iid, web_url, title, source_branch, target_branch, state).\n\nSee also: gitlab_merge_request, gitlab_branch.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input MRInput) (*mcp.CallToolResult, mergerequests.Output, error) {
		start := time.Now()
		out, err := MRCreate(ctx, req, client, input)
		if errors.Is(err, elicitation.ErrElicitationNotSupported) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_interactive_mr_create", start, err)
			return UnsupportedResult("gitlab_interactive_mr_create"), mergerequests.Output{}, nil
		}
		if errors.Is(err, elicitation.ErrCancelled) || errors.Is(err, elicitation.ErrDeclined) {
			// Cancellation is an expected outcome, not an error.
			toolutil.LogToolCallAll(ctx, req, "gitlab_interactive_mr_create", start, nil)
			return CancelledResult("Merge request creation cancelled by user."), mergerequests.Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_interactive_mr_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(mergerequests.FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_interactive_release_create",
		Title: toolutil.TitleFromName("gitlab_interactive_release_create"),
		Description: "Create a GitLab release through step-by-step prompts: tag name (required, must reference an existing tag), " +
			"release name (optional, defaults to tag name), and release notes/description (optional multiline), " +
			"with explicit confirmation before calling the GitLab API. Cancellation at any prompt aborts without creating the release.\n\n" +
			"When to use: human-in-the-loop release publishing. " +
			"NOT for: CI/automated release creation — use gitlab_release (action='create') with all fields pre-supplied.\n\n" +
			descElicitRequired + " If unsupported, returns a structured error naming gitlab_release (action='create') as the alternative.\n\n" +
			"Behavior: each successful invocation publishes ONE new release after explicit user confirmation. NON-idempotent — re-running with the same tag returns 409 (release already exists). Cancellation/decline at any prompt aborts with no GitLab API call and no side effects. Side effects on success: GitLab fires release-created webhooks and may notify release subscribers.\n\n" +
			"Returns: JSON with the created release (tag_name, name, description, web_url).\n\nSee also: gitlab_release, gitlab_tag.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReleaseInput) (*mcp.CallToolResult, releases.Output, error) {
		start := time.Now()
		out, err := ReleaseCreate(ctx, req, client, input)
		if errors.Is(err, elicitation.ErrElicitationNotSupported) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_interactive_release_create", start, err)
			return UnsupportedResult("gitlab_interactive_release_create"), releases.Output{}, nil
		}
		if errors.Is(err, elicitation.ErrCancelled) || errors.Is(err, elicitation.ErrDeclined) {
			// Cancellation is an expected outcome, not an error.
			toolutil.LogToolCallAll(ctx, req, "gitlab_interactive_release_create", start, nil)
			return CancelledResult("Release creation cancelled by user."), releases.Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_interactive_release_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(releases.FormatMarkdown(out)), out, err)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:  "gitlab_interactive_project_create",
		Title: toolutil.TitleFromName("gitlab_interactive_project_create"),
		Description: "Create a GitLab project through step-by-step prompts: name (required), " +
			"description (optional), visibility (private/internal/public), initialize-with-README flag, and default branch name, " +
			"with explicit confirmation before calling the GitLab API. Cancellation at any prompt aborts without creating the project.\n\n" +
			"When to use: human-in-the-loop project creation. NOT for: scripted/programmatic creation — use gitlab_project (action='create') with all fields pre-supplied.\n\n" +
			descElicitRequired + " If unsupported, returns a structured error naming gitlab_project (action='create') as the alternative.\n\n" +
			"Returns: JSON with the created project (id, path_with_namespace, web_url, visibility, default_branch).\n\nSee also: gitlab_project, gitlab_group.",
		Annotations: toolutil.CreateAnnotations,
		Icons:       toolutil.IconConfig,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProjectInput) (*mcp.CallToolResult, projects.Output, error) {
		start := time.Now()
		out, err := ProjectCreate(ctx, req, client, input)
		if errors.Is(err, elicitation.ErrElicitationNotSupported) {
			toolutil.LogToolCallAll(ctx, req, "gitlab_interactive_project_create", start, err)
			return UnsupportedResult("gitlab_interactive_project_create"), projects.Output{}, nil
		}
		if errors.Is(err, elicitation.ErrCancelled) || errors.Is(err, elicitation.ErrDeclined) {
			// Cancellation is an expected outcome, not an error.
			toolutil.LogToolCallAll(ctx, req, "gitlab_interactive_project_create", start, nil)
			return CancelledResult("Project creation cancelled by user."), projects.Output{}, nil
		}
		toolutil.LogToolCallAll(ctx, req, "gitlab_interactive_project_create", start, err)
		return toolutil.WithHints(toolutil.ToolResultWithMarkdown(projects.FormatMarkdown(out)), out, err)
	})
}
