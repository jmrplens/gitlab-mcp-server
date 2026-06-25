package groupprotectedenvs

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for group protected environment actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupProtectedEnvReadSpec("protected_env_list", toolutil.RouteAction(client, List), "gitlab_group_protected_environment_list"),
		groupProtectedEnvReadSpec("protected_env_get", toolutil.RouteAction(client, Get), "gitlab_group_protected_environment_get"),
		groupProtectedEnvCreateSpec("protected_env_protect", toolutil.RouteAction(client, Protect), "gitlab_group_protected_environment_protect"),
		groupProtectedEnvUpdateSpec("protected_env_update", toolutil.RouteAction(client, Update), "gitlab_group_protected_environment_update"),
		groupProtectedEnvDeleteSpec("protected_env_unprotect", toolutil.DestructiveVoidAction(client, Unprotect), "gitlab_group_protected_environment_unprotect"),
	}
}

func groupProtectedEnvReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupProtectedEnvOptions(individualTool))
}

func groupProtectedEnvCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupProtectedEnvOptions(individualTool))
}

func groupProtectedEnvUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupProtectedEnvOptions(individualTool))
}

func groupProtectedEnvDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupProtectedEnvOptions(individualTool))
}

func groupProtectedEnvOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Tags: []string{"group", "protected-environment"},
		Usage:          "Use group protected environment actions for group-level deployment gates. deploy_access_levels must be an array of objects such as [{\"access_level\":40}]. To require approvals, use approval_rules with required_approvals, not top-level required_approval_count.",
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupprotectedenvs",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: groupProtectedEnvDescription(individualTool),
		},
	}
}

// groupProtectedEnvDescription returns the "Returns: … See also: …" tool
// description for each group-protected-environment action (R-META).
func groupProtectedEnvDescription(individualTool string) string {
	switch individualTool {
	case "gitlab_group_protected_environment_list":
		return "List group-level protected environments with order_by/sort and offset or keyset pagination. Returns: group protected environments with their deploy access levels, required approval count, approval rules, and pagination metadata. See also: gitlab_group_protected_environment_get, gitlab_group_protected_environment_protect, gitlab_group_get."
	case "gitlab_group_protected_environment_get":
		return "Get a single group-level protected environment by tier name. Returns: the environment with its deploy access levels (id, access level, user/group, group inheritance) and approval rules. See also: gitlab_group_protected_environment_list, gitlab_group_protected_environment_update, gitlab_group_protected_environment_unprotect."
	case "gitlab_group_protected_environment_protect":
		return "Protect a group-level environment tier with deploy access levels and approval rules; protection cascades to all subgroup projects. Returns: the newly protected environment with its deploy access levels, required approval count, and approval rules. See also: gitlab_group_protected_environment_get, gitlab_group_protected_environment_update, gitlab_group_protected_environment_unprotect."
	case "gitlab_group_protected_environment_update":
		return "Update a group-level protected environment's deploy access levels and approval rules (use _destroy to remove an entry). Returns: the updated environment with its deploy access levels, required approval count, and approval rules. See also: gitlab_group_protected_environment_get, gitlab_group_protected_environment_protect, gitlab_group_protected_environment_unprotect."
	case "gitlab_group_protected_environment_unprotect":
		return "Unprotect a group-level environment tier, removing its deployment gates from the group and its subgroup projects. Returns: a success confirmation. See also: gitlab_group_protected_environment_list, gitlab_group_protected_environment_protect."
	default:
		return ""
	}
}
