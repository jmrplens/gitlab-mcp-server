package issuelinks

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for issue link actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewActionSpec("link_create",
			toolutil.RouteAction(client, Create),
			toolutil.ActionSpecOptions{
				Tags:           []string{"issue", "link"},
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
	}
}
