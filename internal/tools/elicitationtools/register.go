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
		Description: "Interactively create a GitLab issue: step-by-step prompts collect title (required), " +
			"description (optional multiline), comma-separated labels, and confidentiality (boolean), " +
			"then ask for confirmation before calling the GitLab API. Cancellation at any prompt aborts without creating the issue.\n\n" +
			"When to use: human-in-the-loop issue creation with guided prompts. " +
			"NOT for: scripted/programmatic creation — use gitlab_issue (action='create') with all fields pre-supplied.\n\n" +
			descElicitRequired + " If unsupported, returns a structured error naming gitlab_issue (action='create') as the alternative.\n\n" +
			"Returns: JSON with the created issue (id, iid, web_url, title, state).\n\nSee also: gitlab_issue.",
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
		Description: "Interactively create a GitLab merge request: step-by-step prompts collect source branch (required), " +
			"target branch (required), title (required), description (optional multiline), comma-separated labels, " +
			"squash-on-merge flag, and remove-source-branch flag, then ask for confirmation before calling the GitLab API. " +
			"Cancellation at any prompt aborts without creating the MR.\n\n" +
			"When to use: human-in-the-loop MR creation where branches and metadata should be picked interactively. " +
			"NOT for: scripted/programmatic creation — use gitlab_merge_request (action='create') with all fields pre-supplied. " +
			"This tool is the interactive counterpart of gitlab_merge_request (action='create'); they share the same API outcome.\n\n" +
			descElicitRequired + " If unsupported, returns a structured error naming gitlab_merge_request (action='create') as the alternative.\n\n" +
			"Returns: JSON with the created MR (id, iid, web_url, title, source_branch, target_branch, state).\n\nSee also: gitlab_merge_request, gitlab_branch.",
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
		Description: "Interactively create a GitLab release: step-by-step prompts collect tag name (required, must reference an existing tag or one auto-created per project settings), " +
			"release name (optional, defaults to tag name), and release notes/description (optional multiline), " +
			"then ask for confirmation before calling the GitLab API. Cancellation at any prompt aborts without creating the release.\n\n" +
			"When to use: human-in-the-loop release publishing. " +
			"NOT for: CI/automated release creation — use gitlab_release (action='create') with all fields pre-supplied.\n\n" +
			descElicitRequired + " If unsupported, returns a structured error naming gitlab_release (action='create') as the alternative.\n\n" +
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
		Description: "Interactively create a GitLab project: step-by-step prompts collect name (required), " +
			"description (optional), visibility (private/internal/public), initialize-with-README flag, and default branch name, " +
			"then ask for confirmation before calling the GitLab API. Cancellation at any prompt aborts without creating the project.\n\n" +
			"When to use: human-in-the-loop project creation. " +
			"NOT for: scripted/programmatic creation — use gitlab_project (action='create') with all fields pre-supplied.\n\n" +
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
