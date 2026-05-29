package groupvariables

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	return toolutil.NewReadActionSpec(name, route, groupVariableOptions(individualTool))
}

func groupVariableCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupVariableOptions(individualTool))
}

func groupVariableUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupVariableOptions(individualTool))
}

func groupVariableDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupVariableOptions(individualTool))
}

func groupVariableOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupvariables domain action.", Tags: []string{"ci", "group", "variable"},
		OpenWorld:      true,
		OwnerPackage:   "groupvariables",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
