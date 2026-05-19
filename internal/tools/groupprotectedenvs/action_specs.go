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
	options := groupProtectedEnvOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupProtectedEnvCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, groupProtectedEnvOptions(individualTool))
}

func groupProtectedEnvUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupProtectedEnvOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupProtectedEnvDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupProtectedEnvOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupProtectedEnvOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"group", "protected-environment"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupprotectedenvs",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
