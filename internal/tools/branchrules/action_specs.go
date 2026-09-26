package branchrules

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
				Usage:          "Audit a project's aggregated branch protection rules in one call: each rule's matched branch pattern, default/protected flags, matching branch count, squash option, who may push, merge and unprotect (roles, and on Premium and above the users, groups, deploy keys and custom roles granted), allow-force-push and code-owner-approval settings, the security-policy flags, approval rules with their eligible approvers, and external status checks. Use this when reviewing branch protection posture across a project. Pages forward only: this GitLab connection takes first and after, and rejects last and before. For one protected branch's REST record, with the numeric ids of its grants, use branch.get_protected.",
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
					Description: "List a project's aggregated branch protection rules by full project path. Returns: each branch rule with its id, matched pattern, default and protected flags, matching branch count, squash option, branch protection settings (who may push, merge and unprotect, allow force push, code-owner approval required, group level, security-policy flags), approval rules with eligible approvers, external status checks, and keyset pagination metadata. Pages forward only: this GitLab connection takes first and after, and rejects last and before. See also: gitlab_protected_branches_list, gitlab_protected_branch_get, gitlab_project_get.",
				},
			}),
	}
}
