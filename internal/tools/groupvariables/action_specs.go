package groupvariables

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for group CI/CD variable actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupVariableReadSpec("group_list", toolutil.RouteAction(client, List), "gitlab_group_variable_list"),
		groupVariableReadSpec("group_get", toolutil.RouteAction(client, Get), "gitlab_group_variable_get"),
		groupVariableCreateSpec("group_create", toolutil.RouteAction(client, Create), "gitlab_group_variable_create"),
		groupVariableUpdateSpec("group_update", toolutil.RouteAction(client, Update), "gitlab_group_variable_update"),
		groupVariableDeleteSpec("group_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_group_variable_delete"),
	}
}

func groupVariableReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupVariableOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupVariableCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, groupVariableOptions(individualTool))
}

func groupVariableUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupVariableOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupVariableDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := groupVariableOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func groupVariableOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"ci", "group", "variable"},
		OpenWorld:      true,
		OwnerPackage:   "groupvariables",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
