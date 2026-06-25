package projectstatistics

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project statistics actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("statistics_get", toolutil.RouteAction(client, Get), toolutil.ActionSpecOptions{
			Aliases: []string{
				"project fetch statistics",
				"project clone and pull counts",
				"repository fetch activity",
			},
			Tags:           []string{"project", "statistics", "analytics"},
			Usage:          "Get the last 30 days of repository fetch statistics (git clone/pull counts) for a project, broken down by day. Use this when the prompt asks how often a project is being cloned or pulled, or for recent fetch activity. The caller must have at least Reporter access to the project. This returns fetch counts only — it is not the project-size/storage statistics that gitlab_project_get exposes via with_statistics.",
			RelatedActions: []string{"project.get"},
			ParameterGuidance: map[string]toolutil.ParameterGuidance{
				"project_id": {
					SemanticRole:     "scope_project",
					ValueSource:      "Project ID or URL-encoded path whose fetch statistics should be retrieved.",
					ExampleBinding:   `params.project_id:"group/project"`,
					CommonConfusions: []string{"This returns git fetch (clone/pull) counts, not storage/size statistics; for repository or storage size use gitlab_project_get with statistics enabled."},
				},
			},
			OpenWorld:    true,
			OwnerPackage: "projectstatistics",
			IndividualTool: toolutil.IndividualToolSpec{
				Name:        "gitlab_get_project_statistics",
				Title:       toolutil.TitleFromName("gitlab_get_project_statistics"),
				Description: "Get a project's last-30-days git fetch (clone/pull) statistics. Returns: total_fetches and a per-day breakdown of date and count. See also: gitlab_project_get.",
			},
		}),
	}
}
