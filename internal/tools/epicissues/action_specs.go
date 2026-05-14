package epicissues

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for epic issue hierarchy actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewActionSpec("epic_issue_assign",
			toolutil.RouteAction(client, Assign),
			toolutil.ActionSpecOptions{
				Tags:           []string{"group", "epic", "issue"},
				Usage:          "Use to assign a project issue as a child of an epic owned by a group path.",
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
	}
}
