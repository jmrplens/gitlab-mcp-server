package elicitationtools

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/elicitation"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	descElicitRequired      = "Requires the MCP client to support the elicitation capability."
	descElicitSequenceIntro = "After invocation, the tool elicits in order:\n"
	descElicitConfirmPrompt = "- confirm (boolean, required) — final yes/no review of the assembled summary.\n\n"
)

type cancelledOutput struct {
	Message string
}

func (cancelledOutput) SurfaceToolTextOnly() {
	// Marker method only; surface tool projection checks interface satisfaction.
}

type unsupportedOutput struct {
	ToolName string
}

// ActionSpecs returns canonical specs for standalone interactive elicitation actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		interactiveCreateSpec("issue_create", elicitationRoute(client, "gitlab_interactive_issue_create", "Issue creation cancelled by user.", IssueCreate), "gitlab_interactive_issue_create", "Guided issue creation through MCP elicitation with explicit user confirmation.", issueCreateDescription()),
		interactiveCreateSpec("mr_create", elicitationRoute(client, "gitlab_interactive_mr_create", "Merge request creation cancelled by user.", MRCreate), "gitlab_interactive_mr_create", "Guided merge request creation through MCP elicitation with explicit user confirmation.", mrCreateDescription()),
		interactiveCreateSpec("project_create", elicitationRoute(client, "gitlab_interactive_project_create", "Project creation cancelled by user.", ProjectCreate), "gitlab_interactive_project_create", "Guided project creation through MCP elicitation with explicit user confirmation.", projectCreateDescription()),
		interactiveCreateSpec("release_create", elicitationRoute(client, "gitlab_interactive_release_create", "Release creation cancelled by user.", ReleaseCreate), "gitlab_interactive_release_create", "Guided release creation through MCP elicitation with explicit user confirmation.", releaseCreateDescription()),
	}
}

func interactiveCreateSpec(name string, route toolutil.ActionRoute, individualTool, usage, description string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"interactive", "elicitation"},
		Usage:          usage,
		OpenWorld:      true,
		OwnerPackage:   "elicitationtools",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	})
}

func elicitationRoute[T, R any](client *gitlabclient.Client, toolName, cancelMessage string, fn func(context.Context, *mcp.CallToolRequest, *gitlabclient.Client, T) (R, error)) toolutil.ActionRoute {
	route := toolutil.RouteActionWithRequest(client, fn)
	route.Handler = func(ctx context.Context, params map[string]any) (any, error) {
		input, err := toolutil.UnmarshalParams[T](params)
		if err != nil {
			var zero R
			return zero, err
		}
		out, err := fn(ctx, toolutil.RequestFromContext(ctx), client, input)
		if errors.Is(err, elicitation.ErrElicitationNotSupported) {
			return unsupportedOutput{ToolName: toolName}, nil
		}
		if errors.Is(err, elicitation.ErrCancelled) || errors.Is(err, elicitation.ErrDeclined) {
			return cancelledOutput{Message: cancelMessage}, nil
		}
		return out, err
	}
	return route
}

// FormatResult renders elicitation outputs and expected control outcomes.
func FormatResult(result any) *mcp.CallToolResult {
	switch v := result.(type) {
	case unsupportedOutput:
		return UnsupportedResult(v.ToolName)
	case cancelledOutput:
		return CancelledResult(v.Message)
	case issues.Output:
		return toolutil.ToolResultWithMarkdown(issues.FormatMarkdown(v))
	case mergerequests.Output:
		return toolutil.ToolResultWithMarkdown(mergerequests.FormatMarkdown(v))
	case projects.Output:
		return toolutil.ToolResultWithMarkdown(projects.FormatMarkdown(v))
	case releases.Output:
		return toolutil.ToolResultWithMarkdown(releases.FormatMarkdown(v))
	default:
		return toolutil.MarkdownForResult(result)
	}
}

func issueCreateDescription() string {
	return "Create a GitLab issue through step-by-step prompts, with explicit confirmation before calling the GitLab API. Cancellation at any prompt aborts without creating the issue.\n\n" +
		"Input: project_id (numeric ID or URL-encoded path) selects the target project. Prompted fields are title, description, labels, confidential, and confirm. Requires permission to create issues in that project.\n\n" +
		descElicitSequenceIntro +
		"- title (string, required) — issue title.\n" +
		"- description (string, optional, multi-line, Markdown) — leave empty to skip.\n" +
		"- labels (string, optional) — comma-separated; trimmed and deduped server-side.\n" +
		"- confidential (boolean, optional) — yes/no confirmation; defaults to public when declined.\n" +
		descElicitConfirmPrompt +
		"Behavior: cancellation/decline at any prompt aborts with no GitLab API call and no side effects. Each confirmed invocation creates ONE new issue; NON-idempotent — re-running with the same title/fields creates another issue. Side effects on success: GitLab fires issue-created webhooks and may notify issue subscribers.\n\n" +
		"When to use: human-in-the-loop issue creation. " +
		"NOT for: scripted/programmatic creation — use gitlab_issue (action='create') with all fields pre-supplied.\n\n" +
		descElicitRequired + " If unsupported, returns a structured error naming gitlab_issue (action='create') as the alternative.\n\n" +
		"Returns: JSON with the created issue (id, issue_iid, web_url, title, state); issue_iid corresponds to GitLab's iid field.\n\nSee also: gitlab_issue."
}

func mrCreateDescription() string {
	return "Create a GitLab merge request through step-by-step prompts, with explicit confirmation before calling the GitLab API. Cancellation at any prompt aborts without creating the MR.\n\n" +
		"Input: project_id (numeric ID or URL-encoded path) selects the target project. Prompted fields are source_branch, target_branch, title, description, labels, remove_source_branch, squash, and confirm. Requires permission to create merge requests in that project.\n\n" +
		descElicitSequenceIntro +
		"- source_branch (string, required) — branch with the changes to merge.\n" +
		"- target_branch (string, required) — branch to merge into (e.g. main, develop).\n" +
		"- title (string, required) — MR title.\n" +
		"- description (string, optional, multi-line, Markdown) — leave empty to skip.\n" +
		"- labels (string, optional) — comma-separated; trimmed and deduped server-side.\n" +
		"- remove_source_branch (boolean, optional) — yes/no confirmation; default unset.\n" +
		"- squash (boolean, optional) — yes/no confirmation; default unset.\n" +
		descElicitConfirmPrompt +
		"Behavior: cancellation/decline at any prompt aborts with no GitLab API call and no side effects. Each confirmed invocation creates ONE new merge request. " +
		"NON-idempotent — GitLab rejects an already-open MR for the same source_branch to target_branch in the same project as a validation failure (HTTP 422). " +
		"Retries may fail with 422 instead of returning the existing MR. Confirm branch/MR state before re-running. " +
		"For scripted idempotent workflows, use gitlab_merge_request (action='create') with all fields pre-supplied and handle 422 as the expected duplicate case.\n\n" +
		"When to use: human-in-the-loop MR creation. " +
		"NOT for: scripted/programmatic creation — use gitlab_merge_request (action='create') with all fields pre-supplied.\n\n" +
		descElicitRequired + " If unsupported, returns a structured error naming gitlab_merge_request (action='create') as the alternative.\n\n" +
		"Returns: JSON with the created MR (id, merge_request_iid, web_url, title, source_branch, target_branch, state); merge_request_iid corresponds to GitLab's iid field.\n\nSee also: gitlab_merge_request, gitlab_branch."
}

func releaseCreateDescription() string {
	return "Create a GitLab release through step-by-step prompts, with explicit confirmation before calling the GitLab API. Cancellation at any prompt aborts without creating the release.\n\n" +
		"Input: project_id (numeric ID or URL-encoded path) selects the target project. Prompted fields are tag_name, name, description, and confirm. Requires permission to create releases in that project.\n\n" +
		descElicitSequenceIntro +
		"- tag_name (string, required) — must reference an existing tag in the project; create it first via gitlab_tag (action='create').\n" +
		"- name (string, optional) — release title; defaults to tag_name when left empty.\n" +
		"- description (string, optional, multi-line, Markdown) — release notes; leave empty to skip.\n" +
		descElicitConfirmPrompt +
		"When to use: human-in-the-loop release publishing. " +
		"NOT for: CI/automated release creation — use gitlab_release (action='create') with all fields pre-supplied.\n\n" +
		descElicitRequired + " If unsupported, returns a structured error naming gitlab_release (action='create') as the alternative.\n\n" +
		"Behavior: each successful invocation publishes ONE new release after explicit user confirmation. NON-idempotent — re-running with the same tag returns 409 (release already exists). Cancellation/decline at any prompt aborts with no GitLab API call and no side effects. Side effects on success: GitLab fires release-created webhooks and may notify release subscribers.\n\n" +
		"Returns: JSON with the created release (tag_name, name, description, web_url).\n\nSee also: gitlab_release, gitlab_tag."
}

func projectCreateDescription() string {
	return "Create a GitLab project through step-by-step prompts, with explicit confirmation before calling the GitLab API. Cancellation at any prompt aborts without creating the project except initialize_with_readme, where decline/cancel continues with false.\n\n" +
		"Input: no fields; every project detail is elicited. Requires permission to create projects for the authenticated user.\n\n" +
		descElicitSequenceIntro +
		"- name (string, required) — project display name and (when path is omitted) URL slug.\n" +
		"- description (string, optional) — leave empty to skip.\n" +
		"- visibility (enum, required) — one of private, internal, public.\n" +
		"- initialize_with_readme (boolean, optional) — yes/no confirmation; explicit no, decline, or cancel continues with false.\n" +
		"- default_branch (string, optional) — leave empty to use the GitLab default ('main').\n" +
		descElicitConfirmPrompt +
		"When to use: human-in-the-loop project creation. NOT for: scripted/programmatic creation — use gitlab_project (action='create') with all fields pre-supplied.\n\n" +
		"Behavior: each successful invocation creates ONE new project after explicit user confirmation. NON-idempotent — re-running with the same project path/name can fail with 400/409. Cancellation/decline at any prompt aborts with no GitLab API call and no side effects, except initialize_with_readme where no/decline/cancel is accepted as initialize_with_readme=false. Side effects on success: GitLab may initialize a repository and notify project members.\n\n" +
		descElicitRequired + " If unsupported, returns a structured error naming gitlab_project (action='create') as the alternative.\n\n" +
		"Returns: JSON with the created project (id, path_with_namespace, web_url, visibility, default_branch).\n\nSee also: gitlab_project, gitlab_group."
}
