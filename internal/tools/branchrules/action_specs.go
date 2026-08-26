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
				Aliases: []string{
					"gitlab_list_branch_rules",
					"list branch protection rules",
					"audit branch protection",
					"show protected branch rules",
					"branch rule overview",
				},
				Tags:           []string{"branch", "rules", "graphql"},
				Usage:          "Audit a project's aggregated branch protection rules in one call: each rule's matched branch pattern, default/protected flags, matching branch count, allow-force-push and code-owner-approval settings, approval rules, and external status checks. Use this when reviewing branch protection posture across a project; for the protected-branch REST records (allowed-to-push/merge access levels) use branch.list_protected or branch.get_protected instead.",
				RelatedActions: []string{"branch.list_protected", "branch.get_protected", "project.get"},
				ParameterGuidance: map[string]toolutil.ParameterGuidance{
					"project_path": {
						SemanticRole:   "scope_project",
						ValueSource:    "Full project path used by GraphQL branch rule query.",
						ExampleBinding: `params.project_path:"group/project"`,
					},
				},
				OpenWorld:    true,
				OwnerPackage: "branchrules",
				IndividualTool: toolutil.IndividualToolSpec{
					Name:        "gitlab_list_branch_rules",
					Title:       toolutil.TitleFromName("gitlab_list_branch_rules"),
					Description: "List a project's aggregated branch protection rules by full project path. Returns: each branch rule with its matched pattern, default and protected flags, matching branch count, branch protection settings (allow force push, code-owner approval required), approval rules, external status checks, and keyset pagination metadata. See also: gitlab_protected_branches_list, gitlab_protected_branch_get, gitlab_project_get.",
				},
			}),
	}
}
