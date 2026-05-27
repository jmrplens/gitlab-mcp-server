package projectstatistics

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project statistics actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("statistics_get", toolutil.RouteAction(client, Get), toolutil.ActionSpecOptions{
			Aliases:        []string{"gitlab_get_project_statistics"},
			Tags:           []string{"project", "statistics", "analytics"},
			Usage:          "Get detailed storage and repository statistics for a project.",
			RelatedActions: []string{"project.get"},
			ParameterGuidance: map[string]toolutil.ParameterGuidance{
				"project_id": {
					SemanticRole:   "scope_project",
					ValueSource:    "Project ID or path whose statistics should be retrieved.",
					ExampleBinding: `params.project_id:"group/project"`,
				},
			},
			OpenWorld:    true,
			OwnerPackage: "projectstatistics",
			IndividualTool: toolutil.IndividualToolSpec{
				Name:  "gitlab_get_project_statistics",
				Title: toolutil.TitleFromName("gitlab_get_project_statistics"),
			},
		}),
	}
}
