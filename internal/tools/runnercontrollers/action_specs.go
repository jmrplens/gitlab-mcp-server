package runnercontrollers

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for runner controller actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		runnerControllerReadSpec("controller_list", toolutil.RouteAction(client, List), "gitlab_runner_controller_list"),
		runnerControllerReadSpec("controller_get", toolutil.RouteAction(client, Get), "gitlab_runner_controller_get"),
		runnerControllerCreateSpec("controller_create", toolutil.RouteAction(client, Create), "gitlab_runner_controller_create"),
		runnerControllerUpdateSpec("controller_update", toolutil.RouteAction(client, Update), "gitlab_runner_controller_update"),
		runnerControllerDeleteSpec("controller_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_runner_controller_delete"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("runner controller")
	return out, nil
}

func runnerControllerReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, runnerControllerOptions(individualTool))
}

func runnerControllerCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, runnerControllerOptions(individualTool))
}

func runnerControllerUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, runnerControllerOptions(individualTool))
}

func runnerControllerDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, runnerControllerOptions(individualTool))
}

func runnerControllerOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute runnercontrollers domain action.", Tags: []string{"runner", "controller"},
		OpenWorld:      true,
		OwnerPackage:   "runnercontrollers",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
