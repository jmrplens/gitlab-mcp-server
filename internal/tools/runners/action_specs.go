package runners

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runnercontrollers"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runnercontrollerscopes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/runnercontrollertokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for runner and runner controller actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	specs := []toolutil.ActionSpec{
		runnerReadSpec("list", toolutil.RouteAction(client, List), "gitlab_runner_list"),
		runnerReadSpec("list_all", toolutil.RouteAction(client, ListAll), "gitlab_runner_list_all"),
		runnerReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_runner_get"),
		runnerUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_runner_update"),
		runnerDeleteSpec("remove", toolutil.DestructiveVoidAction(client, Remove), "gitlab_runner_remove"),
		runnerReadSpec("jobs", toolutil.RouteAction(client, ListJobs), "gitlab_runner_jobs"),
		runnerReadSpec("list_project", toolutil.RouteAction(client, ListProject), "gitlab_runner_list_project"),
		runnerCreateSpec("enable_project", toolutil.RouteAction(client, EnableProject), "gitlab_runner_enable_project"),
		runnerDeleteSpec("disable_project", toolutil.DestructiveVoidAction(client, DisableProject), "gitlab_runner_disable_project"),
		runnerReadSpec("list_group", toolutil.RouteAction(client, ListGroup), "gitlab_runner_list_group"),
		runnerCreateSpec("register", toolutil.RouteAction(client, Register), "gitlab_runner_register"),
		runnerDeleteSpec("delete_registered", toolutil.DestructiveVoidAction(client, DeleteByID), "gitlab_runner_delete_registered"),
		runnerDeleteSpec("delete_by_token", toolutil.DestructiveVoidAction(client, DeleteByToken), "gitlab_runner_delete_by_token"),
		runnerReadSpec("verify", toolutil.RouteVoidAction(client, Verify), "gitlab_runner_verify"),
		runnerUpdateSpec("reset_token", toolutil.RouteAction(client, ResetAuthToken), "gitlab_runner_reset_token"),
		runnerUpdateSpec("reset_instance_reg_token", toolutil.RouteAction(client, ResetInstanceRegToken), "gitlab_runner_reset_instance_reg_token"),
		runnerUpdateSpec("reset_group_reg_token", toolutil.RouteAction(client, ResetGroupRegToken), "gitlab_runner_reset_group_reg_token"),
		runnerUpdateSpec("reset_project_reg_token", toolutil.RouteAction(client, ResetProjectRegToken), "gitlab_runner_reset_project_reg_token"),
		runnerReadSpec("list_managers", toolutil.RouteAction(client, ListManagers), "gitlab_runner_list_managers"),
	}
	specs = append(specs, runnercontrollers.ActionSpecs(client)...)
	specs = append(specs, runnercontrollerscopes.ActionSpecs(client)...)
	specs = append(specs, runnercontrollertokens.ActionSpecs(client)...)
	return specs
}

func runnerReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := runnerOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func runnerCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, runnerOptions(individualTool))
}

func runnerUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := runnerOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func runnerDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := runnerOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func runnerOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"runner"},
		OpenWorld:      true,
		OwnerPackage:   "runners",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
