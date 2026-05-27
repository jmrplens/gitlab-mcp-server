package groupiterations

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// IssueActionSpecs returns canonical specs for group iteration actions exposed through gitlab_issue.
func IssueActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("iteration_list_group",
			toolutil.RouteAction(client, List),
			toolutil.ActionSpecOptions{
				Aliases:        []string{"gitlab_list_group_iterations"},
				Tags:           []string{"issue", "iteration"},
				Usage:          "List iterations for a group, optionally filtering by state or search term.",
				RelatedActions: []string{"group.get", "issue.list"},
				ParameterGuidance: map[string]toolutil.ParameterGuidance{
					"group_id": {
						SemanticRole:   "scope_group",
						ValueSource:    "Group ID or full path used to query iterations.",
						ExampleBinding: `params.group_id:"my-group"`,
					},
					"state": {
						SemanticRole:   "iteration_state_filter",
						ValueSource:    "Iteration state filter accepted by GitLab (for example opened).",
						ExampleBinding: `params.state:"opened"`,
					},
					"search": {
						SemanticRole:   "search_query",
						ValueSource:    "Free-text search over iteration titles.",
						ExampleBinding: `params.search:"sprint"`,
					},
				},
				OpenWorld:      true,
				Edition:        "premium",
				OwnerPackage:   "groupiterations",
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_list_group_iterations", Title: toolutil.TitleFromName("gitlab_list_group_iterations")},
			}),
	}
}
