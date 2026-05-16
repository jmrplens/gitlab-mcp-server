package civariables

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for project CI/CD variable actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		ciVariableReadSpec("list", toolutil.RouteAction(client, List), "gitlab_ci_variable_list"),
		ciVariableReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_ci_variable_get"),
		ciVariableCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_ci_variable_create"),
		ciVariableUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_ci_variable_update"),
		ciVariableDeleteSpec("delete", toolutil.DestructiveAction(client, DeleteOutput), "gitlab_ci_variable_delete"),
	}
}

func ciVariableReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := ciVariableOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func ciVariableCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, ciVariableOptions(individualTool))
}

func ciVariableUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := ciVariableOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func ciVariableDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := ciVariableOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func ciVariableOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"ci", "variable"},
		OpenWorld:      true,
		OwnerPackage:   "civariables",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}

// DeleteOutput deletes a CI/CD variable and returns the canonical success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted CI/CD variable."}, nil
}
