package projectstatistics

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for project statistics actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewActionSpec("statistics_get", toolutil.RouteAction(client, Get), toolutil.ActionSpecOptions{
			Tags:           []string{"project", "statistics", "analytics"},
			RelatedActions: []string{"project.get"},
			ReadOnly:       true,
			Idempotent:     true,
			OpenWorld:      true,
			OwnerPackage:   "projectstatistics",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_get_project_statistics", Title: toolutil.TitleFromName("gitlab_get_project_statistics")},
		}),
	}
}
