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
		Description: "Interactively create a GitLab issue with step-by-step user prompts via MCP elicitation. " +
			"Guides the user through entering title, description, labels, and confidentiality settings with confirmation before creation. " +
			descElicitRequired + "\n\nReturns: JSON with the created issue details.\n\nSee also: gitlab_issue_create",
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
		Description: "Interactively create a GitLab merge request with step-by-step user prompts via MCP elicitation. " +
			"Guides the user through entering branches, title, description, labels, squash/remove-source options with confirmation. " +
			descElicitRequired + "\n\nReturns: JSON with the created merge request details.\n\nSee also: gitlab_mr_create",
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
		Description: "Interactively create a GitLab release with step-by-step user prompts via MCP elicitation. " +
			"Guides the user through entering tag name, release name, description with confirmation before creation. " +
			descElicitRequired + "\n\nReturns: JSON with the created release details.\n\nSee also: gitlab_release_create",
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
		Description: "Interactively create a GitLab project with step-by-step user prompts via MCP elicitation. " +
			"Guides the user through entering name, description, visibility, README initialization, and default branch with confirmation. " +
			descElicitRequired + "\n\nReturns: JSON with the created project details.\n\nSee also: gitlab_project_create",
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
