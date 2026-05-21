package branchrules

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for branch rule actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("rule_list",
			toolutil.RouteAction(client, List),
			toolutil.ActionSpecOptions{
				Tags:           []string{"branch", "rules", "graphql"},
				Usage:          "Use to audit aggregated branch rules, including protections and approval-related state, for a project path.",
				RelatedActions: []string{"branch.list_protected", "branch.get_protected", "project.get"},
				OpenWorld:      true,
				OwnerPackage:   "branchrules",
				IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_list_branch_rules", Title: toolutil.TitleFromName("gitlab_list_branch_rules")},
			}),
	}
}
