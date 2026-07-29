package deploymentmergerequests

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for deployment merge request actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("deployment_merge_requests", toolutil.RouteAction(client, List), toolutil.ActionSpecOptions{
			Aliases: []string{"merge requests for a deployment", "MRs included in a deployment", "which merge requests shipped in this deployment", "deployment to merge request association"}, Usage: "List the merge requests associated with a specific deployment in a project environment. Use this when the prompt asks which MRs were included, shipped, or rolled out by a known deployment_id, not to list deployments or to list merge requests directly.", Tags: []string{"environment", "deployment"},
			RelatedActions: []string{"environment.deployment_get", "mergerequest.get", "mergerequest.list"},
			OpenWorld:      true,
			OwnerPackage:   "deploymentmergerequests",
			InputSchemaOverrides: []toolutil.InputSchemaOverride{
				toolutil.SchemaApproverIDsOverride("approver_ids"),
				toolutil.SchemaApproverIDsOverride("approved_by_ids"),
			},
			IndividualTool: toolutil.IndividualToolSpec{
				Name:        "gitlab_list_deployment_merge_requests",
				Title:       toolutil.TitleFromName("gitlab_list_deployment_merge_requests"),
				Description: "List the merge requests associated with a deployment, with merge-request filtering (state, approval, author, assignee, draft, created date) and offset or keyset pagination. Returns: matching merge requests with full metadata (author, assignees, reviewers, labels, milestone, pipelines, time stats, timestamps) and pagination metadata. See also: gitlab_mr_get, gitlab_mr_list, gitlab_deployment_list.",
			},
		}),
	}
}
