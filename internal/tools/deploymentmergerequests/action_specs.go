package deploymentmergerequests

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for deployment merge request actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewActionSpec("deployment_merge_requests", toolutil.RouteAction(client, List), toolutil.ActionSpecOptions{
			Tags:           []string{"environment", "deployment"},
			RelatedActions: []string{"environment.deployment_get", "pipeline.list"},
			ReadOnly:       true,
			Idempotent:     true,
			OpenWorld:      true,
			OwnerPackage:   "deploymentmergerequests",
			IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_list_deployment_merge_requests", Title: toolutil.TitleFromName("gitlab_list_deployment_merge_requests")},
		}),
	}
}
