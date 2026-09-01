package issuelinks

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical action ids for issue link routes, referenced by RelatedActions
// metadata so cross-links stay consistent across surfaces.
const (
	actionLinkList   = "issue.link_list"
	actionLinkGet    = "issue.link_get"
	actionLinkCreate = "issue.link_create"
	actionLinkDelete = "issue.link_delete"
	actionIssueGet   = "issue.get"
)

// ActionSpecs returns canonical specs for issue link actions exposed as MCP
// tools. The list/get/create/delete routes are projected into the dynamic,
// meta, individual, and audit surfaces by the action catalog (ADR-0004).
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_issue_link_list — list all issue links for a source issue.
		toolutil.NewReadActionSpec("link_list", toolutil.RouteAction(client, List), listOptions()),
		// gitlab_issue_link_get — fetch a single issue link by its ID.
		toolutil.NewReadActionSpec("link_get", toolutil.RouteAction(client, Get), getOptions()),
		// gitlab_issue_link_create — create a link between two issues (optionally across projects).
		toolutil.NewCreateActionSpec("link_create", toolutil.RouteAction(client, Create), createOptions()),
		// gitlab_issue_link_delete — remove an existing issue link (destructive, requires confirmation).
		toolutil.NewDeleteActionSpec("link_delete", toolutil.DestructiveAction(client, deleteOutput), deleteOptions()),
	}
}

// listOptions returns the metadata for gitlab_issue_link_list.
func listOptions() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{"gitlab_issue_link_list", "list issue links", "show linked issues", "list related issues", "issue relations"},
		Usage:   "List every issue linked to a source issue (relates_to, blocks, is_blocked_by). Use after locating the source issue with issue.get or issue.list to inspect its relationships, then issue.link_get for one link's details.",
		Tags:    []string{"issue", "link"},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			fieldProjectID: {
				SemanticRole: "project",
				ValueSource:  "Project that owns the source issue.",
			},
			fieldIssueIID: {
				SemanticRole:     "source_issue",
				ValueSource:      "IID of the issue whose links are listed.",
				CommonConfusions: []string{"Use the issue IID, not its global ID."},
			},
		},
		RelatedActions: []string{actionLinkGet, actionLinkCreate, actionLinkDelete, actionIssueGet},
		OpenWorld:      true,
		OwnerPackage:   "issuelinks",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        "gitlab_issue_link_list",
			Title:       toolutil.TitleFromName("gitlab_issue_link_list"),
			Description: "List all issues linked to a source issue. Returns: each linked issue with its link_type, issue_link_id, full author/assignee/milestone objects, labels, and dates. See also: gitlab_issue_link_get, gitlab_issue_link_create, gitlab_issue_get.",
		},
	}
}

// getOptions returns the metadata for gitlab_issue_link_get.
func getOptions() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{"gitlab_issue_link_get", "get issue link", "show issue link", "fetch issue link details"},
		Usage:   "Get one issue link by its issue_link_id. Use issue.link_list first to find the issue_link_id, then this for the link's source/target issue details.",
		Tags:    []string{"issue", "link"},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			fieldProjectID: {
				SemanticRole: "project",
				ValueSource:  "Project that owns the issue holding the link.",
			},
			fieldIssueIID: {
				SemanticRole: "source_issue",
				ValueSource:  "IID of the issue the link belongs to.",
			},
			"issue_link_id": {
				SemanticRole:     "issue_link",
				ValueSource:      "issue_link_id returned by issue.link_list.",
				CommonConfusions: []string{"This is the link ID, not the linked issue's IID."},
			},
		},
		RelatedActions: []string{actionLinkList, actionLinkCreate, actionLinkDelete, actionIssueGet},
		OpenWorld:      true,
		OwnerPackage:   "issuelinks",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        "gitlab_issue_link_get",
			Title:       toolutil.TitleFromName("gitlab_issue_link_get"),
			Description: "Get one issue link by its issue_link_id. Returns: the link's id, link_type, and full source_issue/target_issue objects. See also: gitlab_issue_link_list, gitlab_issue_link_create, gitlab_issue_get.",
		},
	}
}

// createOptions returns the metadata for gitlab_issue_link_create.
func createOptions() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases:        []string{"gitlab_issue_link_create", "link issues", "create issue link", "add issue relation", "block issue", "relate issues"},
		Usage:          "Create a relationship from a source issue to a target issue, optionally across projects. Pick link_type relates_to (default), blocks, or is_blocked_by. Use issue.link_list afterward to confirm.",
		Tags:           []string{"issue", "link"},
		RelatedActions: []string{actionLinkList, actionLinkGet, actionLinkDelete, actionIssueGet},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			fieldProjectID: {
				SemanticRole:     "source_project",
				ValueSource:      "Project that owns the source issue.",
				CommonConfusions: []string{"Use target_project_id for the linked issue's project when it differs."},
			},
			fieldIssueIID: {
				SemanticRole:     "source_issue",
				ValueSource:      "IID of the source issue receiving the link.",
				CommonConfusions: []string{"Do not use the target issue IID here."},
			},
			"target_project_id": {
				SemanticRole:     "target_project",
				ValueSource:      "Project that owns the target issue.",
				CommonConfusions: []string{"For same-project links this may equal project_id. Otherwise keep it distinct."},
			},
			"target_issue_iid": {
				SemanticRole:     "target_issue",
				ValueSource:      "IID of the issue being linked to.",
				CommonConfusions: []string{"Do not use the source issue IID here."},
			},
			"link_type": {
				SemanticRole:     "link_type",
				ValueSource:      "One of relates_to (default), blocks, is_blocked_by.",
				CommonConfusions: []string{"Omit for a plain relates_to relationship."},
			},
		},
		OpenWorld:    true,
		OwnerPackage: "issuelinks",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        "gitlab_issue_link_create",
			Title:       toolutil.TitleFromName("gitlab_issue_link_create"),
			Description: "Create a link between two issues (optionally across projects). Returns: the created link with its id, link_type, and source_issue/target_issue objects. See also: gitlab_issue_link_list, gitlab_issue_link_delete, gitlab_issue_get.",
		},
	}
}

// deleteOptions returns the metadata for gitlab_issue_link_delete.
func deleteOptions() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{"gitlab_issue_link_delete", "delete issue link", "remove issue link", "unlink issues", "remove issue relation"},
		Usage:   "Remove an existing issue link by its issue_link_id. Destructive: this severs the relationship between the two issues. Use issue.link_list first to find the issue_link_id.",
		Tags:    []string{"issue", "link"},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			fieldProjectID: {
				SemanticRole: "project",
				ValueSource:  "Project that owns the issue holding the link.",
			},
			fieldIssueIID: {
				SemanticRole: "source_issue",
				ValueSource:  "IID of the issue the link belongs to.",
			},
			"issue_link_id": {
				SemanticRole:     "issue_link",
				ValueSource:      "issue_link_id returned by issue.link_list.",
				CommonConfusions: []string{"This is the link ID, not the linked issue's IID."},
			},
		},
		RelatedActions: []string{actionLinkList, actionLinkGet, actionLinkCreate, actionIssueGet},
		OpenWorld:      true,
		OwnerPackage:   "issuelinks",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        "gitlab_issue_link_delete",
			Title:       toolutil.TitleFromName("gitlab_issue_link_delete"),
			Description: "Remove an existing issue link by its issue_link_id. Returns: a deletion confirmation. See also: gitlab_issue_link_list, gitlab_issue_link_create, gitlab_issue_get.",
		},
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
