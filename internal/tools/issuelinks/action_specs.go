package issuelinks

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for issue link actions exposed as MCP
// tools. The list/get/create/delete routes are projected into the dynamic,
// meta, individual, and audit surfaces by the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_issue_link_list — list all issue links for a source issue.
		issueLinkReadSpec("link_list", toolutil.RouteAction(client, List), "gitlab_issue_link_list"),
		// gitlab_issue_link_get — fetch a single issue link by its ID.
		issueLinkReadSpec("link_get", toolutil.RouteAction(client, Get), "gitlab_issue_link_get"),
		// gitlab_issue_link_create — create a link between two issues (optionally across projects).
		toolutil.NewCreateActionSpec("link_create",
			toolutil.RouteAction(client, Create),
			toolutil.ActionSpecOptions{
				Aliases: []string{"gitlab_issue_link_create"}, Tags: []string{"issue", "link"},
				Usage:          "Use to create a relationship from a source issue to a target issue, optionally across projects.",
				RelatedActions: []string{"issue.link_list", "issue.link_get", "issue.link_delete", "issue.get"},
				ParameterGuidance: map[string]toolutil.ParameterGuidance{
					"project_id": {
						SemanticRole:     "source_project",
						ValueSource:      "Project that owns the source issue.",
						CommonConfusions: []string{"Use target_project_id for the linked issue's project when it differs."},
					},
					"issue_iid": {
						SemanticRole:     "source_issue",
						ValueSource:      "IID of the source issue receiving the link.",
						CommonConfusions: []string{"Do not use the target issue IID here."},
					},
					"target_project_id": {
						SemanticRole:     "target_project",
						ValueSource:      "Project that owns the target issue.",
						CommonConfusions: []string{"For same-project links this may equal project_id; otherwise keep it distinct."},
					},
					"target_issue_iid": {
						SemanticRole:     "target_issue",
						ValueSource:      "IID of the issue being linked to.",
						CommonConfusions: []string{"Do not use the source issue IID here."},
					},
				},
				OpenWorld:      true,
				OwnerPackage:   "issuelinks",
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_issue_link_create", Title: toolutil.TitleFromName("gitlab_issue_link_create")},
			}),
		// gitlab_issue_link_delete — remove an existing issue link (destructive, requires confirmation).
		issueLinkDeleteSpec("link_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_issue_link_delete"),
	}
}

// deleteOutput adapts the package's [Delete] handler to the
// [toolutil.DestructiveAction] contract, returning a structured success result.
func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("issue link")
	return out, nil
}

// issueLinkReadSpec builds a read-only [toolutil.ActionSpec] for an issue
// link action, applying the package's default [issueLinkOptions].
func issueLinkReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, issueLinkOptions(individualTool))
}

// issueLinkDeleteSpec builds a destructive [toolutil.ActionSpec] for an
// issue link action, applying the package's default [issueLinkOptions].
func issueLinkDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, issueLinkOptions(individualTool))
}

// issueLinkOptions returns the base [toolutil.ActionSpecOptions] shared by
// every issue link action (tags, owner, individual tool metadata).
func issueLinkOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute issuelinks domain action.", Tags: []string{"issue", "link"},
		OpenWorld:      true,
		OwnerPackage:   "issuelinks",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
