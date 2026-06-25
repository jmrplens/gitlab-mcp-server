package deploymentmergerequests

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for deployment merge request actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("deployment_merge_requests", toolutil.RouteAction(client, List), toolutil.ActionSpecOptions{
			Aliases: []string{"gitlab_list_deployment_merge_requests"}, Usage: "Use to execute deployment_merge_requests action.", Tags: []string{"environment", "deployment"},
			RelatedActions: []string{"environment.deployment_get", "pipeline.list"},
			OpenWorld:      true,
			OwnerPackage:   "deploymentmergerequests",
			IndividualTool: toolutil.IndividualToolSpec{
				Name:        "gitlab_list_deployment_merge_requests",
				Title:       toolutil.TitleFromName("gitlab_list_deployment_merge_requests"),
				Description: "List the merge requests associated with a deployment, with merge-request filtering (state, approval, author, assignee, draft, created date) and offset or keyset pagination. Returns: matching merge requests with full metadata (author, assignees, reviewers, labels, milestone, pipelines, time stats, timestamps) and pagination metadata. See also: gitlab_mr_get, gitlab_mr_list, gitlab_deployment_list.",
			},
		}),
	}
}
