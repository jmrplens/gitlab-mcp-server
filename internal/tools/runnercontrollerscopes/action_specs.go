package runnercontrollerscopes

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for runner controller scope actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		runnerControllerScopeReadSpec("controller_scope_list", toolutil.RouteAction(client, List), "gitlab_runner_controller_scope_list"),
		runnerControllerScopeCreateSpec("controller_scope_add_instance", toolutil.RouteAction(client, AddInstanceScope), "gitlab_runner_controller_scope_add_instance"),
		runnerControllerScopeDeleteSpec("controller_scope_remove_instance", toolutil.DestructiveVoidAction(client, RemoveInstanceScope), "gitlab_runner_controller_scope_remove_instance"),
		runnerControllerScopeCreateSpec("controller_scope_add_runner", toolutil.RouteAction(client, AddRunnerScope), "gitlab_runner_controller_scope_add_runner"),
		runnerControllerScopeDeleteSpec("controller_scope_remove_runner", toolutil.DestructiveVoidAction(client, RemoveRunnerScope), "gitlab_runner_controller_scope_remove_runner"),
	}
}

func runnerControllerScopeReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := runnerControllerScopeOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func runnerControllerScopeCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, runnerControllerScopeOptions(individualTool))
}

func runnerControllerScopeDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := runnerControllerScopeOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func runnerControllerScopeOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"runner", "controller", "scope"},
		OpenWorld:      true,
		OwnerPackage:   "runnercontrollerscopes",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
