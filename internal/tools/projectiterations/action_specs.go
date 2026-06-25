package projectiterations

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// IssueActionSpecs returns canonical specs for project iteration actions exposed through gitlab_issue.
func IssueActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		issueIterationReadSpec("iteration_list_project", toolutil.RouteAction(client, List), "gitlab_list_project_iterations", "projectiterations"),
	}
}

func issueIterationReadSpec(name string, route toolutil.ActionRoute, individualTool, ownerPackage string) toolutil.ActionSpec {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{
			"list project iterations",
			"show project sprints",
			"project iteration cadences",
		},
		Usage:          "List the iterations (time-boxed sprints) scoped to one project, including those inherited from ancestor groups when include_ancestors is set. Use this when the prompt asks for a project's sprints, current iteration, or upcoming cadences. Filter by state (opened, upcoming, current, closed, all) and search by title. Iterations are a Premium/Ultimate feature: a 404 usually means the instance or project tier lacks iteration support, not that the project is missing.",
		Tags:           []string{"issue", "iteration", "sprint"},
		RelatedActions: []string{"issue.iteration_list_group", "issue.list", "milestone.list_project"},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			"project_id": {
				SemanticRole:     "scope_project",
				ValueSource:      "Project ID or URL-encoded namespace path whose iterations should be listed.",
				ExampleBinding:   `params.project_id:"group/project"`,
				CommonConfusions: []string{"Iterations are defined at the group level; pass the project here and set include_ancestors to surface group-defined iterations."},
			},
			"state": {
				ValueSource:      "Iteration lifecycle filter requested by the user: opened, upcoming, current, closed, or all.",
				ExampleBinding:   `params.state:"current"`,
				CommonConfusions: []string{"Use current (singular) for the active iteration; there is no active or in_progress value."},
			},
			"include_ancestors": {
				ValueSource:    "Set true to include iterations defined on ancestor groups, not only those declared directly on the project.",
				ExampleBinding: "params.include_ancestors:true",
			},
		},
		OpenWorld:    true,
		Edition:      "premium",
		OwnerPackage: ownerPackage,
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: "List iterations (sprints) for a project, optionally including ancestor-group iterations. Returns: iterations with id, iid, sequence, group_id, title, description, state, start/due dates, timestamps, web URL, and pagination metadata. See also: gitlab_list_group_iterations, gitlab_issue_list, gitlab_milestone_list.",
		},
	}
	return toolutil.NewReadActionSpec(name, route, options)
}
