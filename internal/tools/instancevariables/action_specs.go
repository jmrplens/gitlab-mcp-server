package instancevariables

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for instance CI/CD variable actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		instanceVariableReadSpec("instance_list", toolutil.RouteAction(client, List), "gitlab_instance_variable_list"),
		instanceVariableReadSpec("instance_get", toolutil.RouteAction(client, Get), "gitlab_instance_variable_get"),
		instanceVariableCreateSpec("instance_create", toolutil.RouteAction(client, Create), "gitlab_instance_variable_create"),
		instanceVariableUpdateSpec("instance_update", toolutil.RouteAction(client, Update), "gitlab_instance_variable_update"),
		instanceVariableDeleteSpec("instance_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_instance_variable_delete"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted instance CI/CD variable."}, nil
}

func instanceVariableReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := instanceVariableOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func instanceVariableCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, instanceVariableOptions(individualTool))
}

func instanceVariableUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := instanceVariableOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func instanceVariableDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := instanceVariableOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func instanceVariableOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"ci", "instance", "variable"},
		OpenWorld:      true,
		OwnerPackage:   "instancevariables",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
