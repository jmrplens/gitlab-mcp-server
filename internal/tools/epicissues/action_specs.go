package epicissues

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for epic issue hierarchy actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		epicIssueReadSpec("epic_issue_list", toolutil.RouteAction(client, List), "gitlab_epic_issue_list"),
		toolutil.NewCreateActionSpec("epic_issue_assign",
			toolutil.RouteAction(client, Assign),
			toolutil.ActionSpecOptions{
				Aliases: []string{"gitlab_epic_issue_assign"}, Tags: []string{"group", "epic", "issue"},
				Usage:          "Use to assign a project issue as a child of an epic owned by a group path. Send full_path for the epic group. Copy epic_iid from the epic_create or epic_get result and child_iid from the child issue result; do not omit full_path or epic_iid after creating the epic.",
				RelatedActions: []string{"group.epic_issue_list", "group.epic_issue_remove", "group.epic_get", "issue.get"},
				ParameterGuidance: map[string]toolutil.ParameterGuidance{
					"full_path": {
						SemanticRole:     "parent_group_path",
						ValueSource:      "Group full path that owns the epic.",
						CommonConfusions: []string{"Do not use the child project path as full_path."},
					},
					"child_project_path": {
						SemanticRole:     "child_project_path",
						ValueSource:      "Project path that owns the issue being assigned to the epic.",
						CommonConfusions: []string{"Do not use project_id or target_full_path for this parameter."},
					},
					"child_iid": {
						SemanticRole:     "child_issue_iid",
						ValueSource:      "Issue IID in child_project_path.",
						CommonConfusions: []string{"Do not use epic_iid as child_iid."},
					},
				},
				Edition:        "premium",
				OpenWorld:      true,
				OwnerPackage:   "epicissues",
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_epic_issue_assign", Title: toolutil.TitleFromName("gitlab_epic_issue_assign")},
			}),
		epicIssueDeleteSpec("epic_issue_remove", toolutil.DestructiveAction(client, Remove), "gitlab_epic_issue_remove"),
		epicIssueUpdateSpec("epic_issue_update", toolutil.RouteAction(client, UpdateOrder), "gitlab_epic_issue_update"),
	}
}

func epicIssueReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, epicIssueOptions(individualTool))
}

func epicIssueUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, epicIssueOptions(individualTool))
}

func epicIssueDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, epicIssueOptions(individualTool))
}

func epicIssueOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute epicissues domain action.", Tags: []string{"group", "epic", "issue"},
		RelatedActions: []string{"group.epic_get", "issue.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "epicissues",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if individualTool == "gitlab_epic_issue_list" {
		options.Usage = "Use to list issues assigned to an epic. Send full_path for the epic group and epic_iid from the epic_create or epic_get result."
	}
	if individualTool == "gitlab_epic_issue_remove" {
		options.Usage = "Use to unlink a child issue from an epic. Send full_path for the epic group. Copy epic_iid from the epic_create or epic_get result and child_iid from the child issue result; removal is destructive and requires confirmation."
	}
	return options
}
