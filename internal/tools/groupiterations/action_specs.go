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
				Tags:           []string{"issue", "iteration"},
				OpenWorld:      true,
				Edition:        "premium",
				OwnerPackage:   "groupiterations",
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_list_group_iterations", Title: toolutil.TitleFromName("gitlab_list_group_iterations")},
			}),
	}
}
