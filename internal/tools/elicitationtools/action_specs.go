package elicitationtools

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/elicitation"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/releases"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	descElicitRequired      = "Requires the MCP client to support the elicitation capability."
	descElicitSequenceIntro = "After invocation, the tool elicits in order:\n"
	descElicitConfirmPrompt = "- confirm (boolean, required): final yes/no review of the assembled summary.\n\n"
)

// The action each flow offers in its place when the client cannot elicit, by
// catalog ID. The refusal names it and so does the flow's description, both
// from here, so the two cannot promise different things: they used to, when
// the refusal named two meta tools for every flow while each description
// promised its own. The domains write these IDs as package-local constants,
// which is why they are spelled again here, and why a test of the catalog in
// internal/tools holds each one to an action it serves.
const (
	alternativeIssueCreate   = "issue.create"
	alternativeMRCreate      = "merge_request.create"
	alternativeProjectCreate = "project.create"
	alternativeReleaseCreate = "release.create"
)

// The ID each flow is served under, which is the name its refusal leads with
// because it is the one spelling every surface resolves: the dynamic surface
// runs a flow through gitlab_execute_action and registers no tool of its own
// for it. The domain is the one internal/tools/surfaces aggregates the flows
// under, so these are spelled again here for the reason the alternatives are,
// and the same catalog test holds each to the flow it is handed to.
const (
	flowIssueCreate   = "interactive.issue_create"
	flowMRCreate      = "interactive.mr_create"
	flowProjectCreate = "interactive.project_create"
	flowReleaseCreate = "interactive.release_create"
)

type cancelledOutput struct {
	Message string
}

func (cancelledOutput) SurfaceToolTextOnly() {
	// Marker method only; surface tool projection checks interface satisfaction.
}

// unsupportedOutput is a flow refused for want of elicitation: the flow the
// client called, by ID and by the tool the meta and individual surfaces
// register for it, and the action that does the same work without prompting.
type unsupportedOutput struct {
	ActionID    string
	ToolName    string
	Alternative string
}

// ActionSpecs returns canonical specs for standalone interactive elicitation actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		interactiveCreateSpec("issue_create", elicitationRoute(client, flowIssueCreate, "gitlab_interactive_issue_create", alternativeIssueCreate, "Issue creation cancelled by user.", IssueCreate), "gitlab_interactive_issue_create", issueCreateDescription()),
		interactiveCreateSpec("mr_create", elicitationRoute(client, flowMRCreate, "gitlab_interactive_mr_create", alternativeMRCreate, "Merge request creation cancelled by user.", MRCreate), "gitlab_interactive_mr_create", mrCreateDescription()),
		interactiveCreateSpec("project_create", elicitationRoute(client, flowProjectCreate, "gitlab_interactive_project_create", alternativeProjectCreate, "Project creation cancelled by user.", ProjectCreate), "gitlab_interactive_project_create", projectCreateDescription()),
		interactiveCreateSpec("release_create", elicitationRoute(client, flowReleaseCreate, "gitlab_interactive_release_create", alternativeReleaseCreate, "Release creation cancelled by user.", ReleaseCreate), "gitlab_interactive_release_create", releaseCreateDescription()),
	}
}

// interactiveCreateSpec declares one flow, whose one text is its Usage and its
// tool's description both.
//
// A flow is a standalone surface tool: meta and individual register it as a
// tool and the dynamic surface runs it by ID, so the text is served on all
// three, which is why it names every action by canonical ID. It is written as
// the Usage because that is the line cmd/audit_action_ids holds to that
// demand whatever builds it; the served-prose rule reads a Description for
// tool names only where it is a constant, and the Description here is the
// usage handed in, which no rule reads under that name. The one-line usages
// the flows carried before were never served, since the surface projection
// replaced them with the description.
func interactiveCreateSpec(name string, route toolutil.ActionRoute, individualTool, usage string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"interactive", "elicitation"},
		Usage:          usage,
		OpenWorld:      true,
		OwnerPackage:   "elicitationtools",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: usage},
	})
}

func elicitationRoute[T, R any](client *gitlabclient.Client, actionID, toolName, alternative, cancelMessage string, fn func(context.Context, *mcp.CallToolRequest, *gitlabclient.Client, T) (R, error)) toolutil.ActionRoute {
	// The handler needs the client itself, not only the handler it replaces,
	// so it is bound through WithBoundHandler: a shared catalog then rebuilds
	// it for each credential's client instead of keeping this one.
	return toolutil.RouteActionWithRequest(client, fn).WithBoundHandler(client, func(client *gitlabclient.Client) toolutil.ActionFunc {
		return func(ctx context.Context, params map[string]any) (any, error) {
			input, err := toolutil.UnmarshalParams[T](params)
			if err != nil {
				var zero R
				return zero, err
			}
			out, err := fn(ctx, toolutil.RequestFromContext(ctx), client.For(ctx), input)
			if errors.Is(err, elicitation.ErrElicitationNotSupported) {
				return unsupportedOutput{ActionID: actionID, ToolName: toolName, Alternative: alternative}, nil
			}
			if errors.Is(err, elicitation.ErrCancelled) || errors.Is(err, elicitation.ErrDeclined) {
				return cancelledOutput{Message: cancelMessage}, nil
			}
			return out, err
		}
	})
}

// FormatResult renders elicitation outputs and expected control outcomes.
func FormatResult(result any) *mcp.CallToolResult {
	switch v := result.(type) {
	case unsupportedOutput:
		return UnsupportedResult(v.ActionID, v.ToolName, v.Alternative)
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

// descElicitUnsupported is the paragraph a flow's description states its
// requirement in, naming the action its refusal offers instead.
func descElicitUnsupported(alternative string) string {
	return descElicitRequired + " If unsupported, returns a structured error naming " + alternative + " as the alternative.\n\n"
}

func issueCreateDescription() string {
	return "Create a GitLab issue through step-by-step prompts, with explicit confirmation before calling the GitLab API. Canceling at any prompt aborts without creating the issue.\n\n" +
		"Input: project_id (numeric ID or URL-encoded path) selects the target project. Prompted fields are title, description, labels, confidential, and confirm. Requires permission to create issues in that project.\n\n" +
		descElicitSequenceIntro +
		"- title (string, required): issue title.\n" +
		"- description (string, optional, multi-line, Markdown): leave empty to skip.\n" +
		"- labels (string, optional): comma-separated. Trimmed and deduped server-side.\n" +
		"- confidential (boolean, optional): yes/no confirmation. Defaults to public when declined.\n" +
		descElicitConfirmPrompt +
		"Behavior: canceling at any prompt aborts with no GitLab API call and no side effects. Declining an optional prompt continues with that field unset. Each confirmed invocation creates ONE new issue. NON-idempotent: re-running with the same title/fields creates another issue. Side effects on success: GitLab fires issue-created webhooks and may notify issue subscribers.\n\n" +
		"When to use: human-in-the-loop issue creation. " +
		"NOT for: scripted/programmatic creation. Use issue.create with all fields pre-supplied.\n\n" +
		descElicitUnsupported(alternativeIssueCreate) +
		"Returns: JSON with the created issue (id, issue_iid, web_url, title, state). issue_iid corresponds to GitLab's iid field.\n\nSee also: issue.create, issue.get."
}

func mrCreateDescription() string {
	return "Create a GitLab merge request through step-by-step prompts, with explicit confirmation before calling the GitLab API. Canceling at any prompt aborts without creating the MR.\n\n" +
		"Input: project_id (numeric ID or URL-encoded path) selects the target project. Prompted fields are source_branch, target_branch, title, description, labels, remove_source_branch, squash, and confirm. Requires permission to create merge requests in that project.\n\n" +
		descElicitSequenceIntro +
		"- source_branch (string, required): branch with the changes to merge.\n" +
		"- target_branch (string, required): branch to merge into (e.g. main, develop).\n" +
		"- title (string, required): MR title.\n" +
		"- description (string, optional, multi-line, Markdown): leave empty to skip.\n" +
		"- labels (string, optional): comma-separated. Trimmed and deduped server-side.\n" +
		"- remove_source_branch (boolean, optional): yes/no confirmation, default unset.\n" +
		"- squash (boolean, optional): yes/no confirmation, default unset.\n" +
		descElicitConfirmPrompt +
		"Behavior: canceling at any prompt aborts with no GitLab API call and no side effects. Declining an optional prompt continues with that field unset. Each confirmed invocation creates ONE new merge request. " +
		"NON-idempotent: GitLab rejects an already-open MR for the same source_branch to target_branch in the same project as a validation failure (HTTP 422). " +
		"Retries may fail with 422 instead of returning the existing MR. Confirm branch/MR state before re-running. " +
		"For scripted idempotent workflows, use merge_request.create with all fields pre-supplied and handle 422 as the expected duplicate case.\n\n" +
		"When to use: human-in-the-loop MR creation. " +
		"NOT for: scripted/programmatic creation. Use merge_request.create with all fields pre-supplied.\n\n" +
		descElicitUnsupported(alternativeMRCreate) +
		"Returns: JSON with the created MR (id, merge_request_iid, web_url, title, source_branch, target_branch, state). merge_request_iid corresponds to GitLab's iid field.\n\nSee also: merge_request.create, branch.create."
}

func releaseCreateDescription() string {
	return "Create a GitLab release through step-by-step prompts, with explicit confirmation before calling the GitLab API. Canceling or declining any prompt aborts without creating the release.\n\n" +
		"Input: project_id (numeric ID or URL-encoded path) selects the target project. Prompted fields are tag_name, name, description, and confirm. Requires permission to create releases in that project.\n\n" +
		descElicitSequenceIntro +
		"- tag_name (string, required): must reference an existing tag in the project. Create it first via tag.create.\n" +
		"- name (string, optional): release title. Answer with an empty value to let GitLab name the release after the tag. Declining the prompt aborts.\n" +
		"- description (string, optional, multi-line, Markdown): release notes. Leave empty to skip.\n" +
		descElicitConfirmPrompt +
		"When to use: human-in-the-loop release publishing. " +
		"NOT for: CI/automated release creation. Use release.create with all fields pre-supplied.\n\n" +
		descElicitUnsupported(alternativeReleaseCreate) +
		"Behavior: each successful invocation publishes ONE new release after explicit user confirmation. NON-idempotent: re-running with the same tag returns 409 (release already exists). Canceling or declining any prompt aborts with no GitLab API call and no side effects. This flow's optional fields take an empty answer rather than a decline. Side effects on success: GitLab fires release-created webhooks and may notify release subscribers.\n\n" +
		"Returns: JSON with the created release (tag_name, name, description, web_url).\n\nSee also: release.create, tag.create."
}

func projectCreateDescription() string {
	return "Create a GitLab project through step-by-step prompts, with explicit confirmation before calling the GitLab API. Canceling at any prompt aborts without creating the project. Declining an optional prompt continues with that field unset, except initialize_with_readme, where a decline continues with false.\n\n" +
		"Input: no fields. Every project detail is elicited. Requires permission to create projects for the authenticated user.\n\n" +
		descElicitSequenceIntro +
		"- name (string, required): project display name and (when path is omitted) URL slug.\n" +
		"- description (string, optional): leave empty to skip.\n" +
		"- visibility (enum, required): one of private, internal, public.\n" +
		"- initialize_with_readme (boolean, optional): yes/no confirmation. An explicit no or a decline continues with false. Canceling aborts the flow.\n" +
		"- default_branch (string, optional): leave empty to use the GitLab default ('main').\n" +
		descElicitConfirmPrompt +
		"When to use: human-in-the-loop project creation. NOT for: scripted/programmatic creation. Use project.create with all fields pre-supplied.\n\n" +
		"Behavior: each successful invocation creates ONE new project after explicit user confirmation. NON-idempotent: re-running with the same project path/name can fail with 400/409. Canceling at any prompt aborts with no GitLab API call and no side effects. Declining an optional prompt continues, with initialize_with_readme taking a decline as false. Side effects on success: GitLab may initialize a repository and notify project members.\n\n" +
		descElicitUnsupported(alternativeProjectCreate) +
		"Returns: JSON with the created project (id, path_with_namespace, web_url, visibility, default_branch).\n\nSee also: project.get, group.get."
}
