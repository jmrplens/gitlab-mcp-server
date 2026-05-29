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
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
