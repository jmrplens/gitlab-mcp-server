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
				Aliases: []string{
					"gitlab_list_group_iterations",
					"list group sprints",
					"group iteration cadences",
					"group sprint schedule",
					"current group iteration",
				},
				Tags:           []string{"issue", "iteration"},
				Usage:          "List a group's iterations (sprints) with optional state, search, and ancestor filters, sorted via keyset pagination. Use state='current' to find the active sprint.",
				RelatedActions: []string{"group.get", "group.list", "issue.list"},
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
				InputSchemaOverrides: []toolutil.InputSchemaOverride{
					toolutil.SchemaEnumOverride("state", "opened", "upcoming", "current", "closed", "all"),
				},
				OpenWorld:    true,
				Edition:      "premium",
				OwnerPackage: "groupiterations",
				IndividualTool: toolutil.IndividualToolSpec{
					Name:        "gitlab_list_group_iterations",
					Title:       toolutil.TitleFromName("gitlab_list_group_iterations"),
					Description: "List a group's iterations (sprints), optionally filtered by state, search term, or ancestor groups. Returns: iterations with sequence, title, description, state, start and due dates, web URL, and pagination metadata. See also: gitlab_group_get, gitlab_issue_list_group, gitlab_list_project_iterations.",
				},
			}),
	}
}
