package runnercontrollerscopes

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for runner controller scope actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		runnerControllerScopeReadSpec("controller_scope_list", toolutil.RouteAction(client, List), "gitlab_runner_controller_scope_list"),
		runnerControllerScopeCreateSpec("controller_scope_add_instance", toolutil.RouteAction(client, AddInstanceScope), "gitlab_runner_controller_scope_add_instance"),
		runnerControllerScopeDeleteSpec("controller_scope_remove_instance", toolutil.DestructiveAction(client, removeInstanceScopeOutput), "gitlab_runner_controller_scope_remove_instance"),
		runnerControllerScopeCreateSpec("controller_scope_add_runner", toolutil.RouteAction(client, AddRunnerScope), "gitlab_runner_controller_scope_add_runner"),
		runnerControllerScopeDeleteSpec("controller_scope_remove_runner", toolutil.DestructiveAction(client, removeRunnerScopeOutput), "gitlab_runner_controller_scope_remove_runner"),
	}
}

func removeInstanceScopeOutput(ctx context.Context, client *gitlabclient.Client, input RemoveInstanceScopeInput) (toolutil.DeleteOutput, error) {
	if err := RemoveInstanceScope(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("instance-level scope")
	return out, nil
}

func removeRunnerScopeOutput(ctx context.Context, client *gitlabclient.Client, input RemoveRunnerScopeInput) (toolutil.DeleteOutput, error) {
	if err := RemoveRunnerScope(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("runner scope")
	return out, nil
}

func runnerControllerScopeReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, runnerControllerScopeOptions(individualTool))
}

func runnerControllerScopeCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, runnerControllerScopeOptions(individualTool))
}

func runnerControllerScopeDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, runnerControllerScopeOptions(individualTool))
}

func runnerControllerScopeOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute runnercontrollerscopes domain action.", Tags: []string{"runner", "controller", "scope"},
		OpenWorld:      true,
		OwnerPackage:   "runnercontrollerscopes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
