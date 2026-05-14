package runnercontrollertokens

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for runner controller token actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		runnerControllerTokenReadSpec("controller_token_list", toolutil.RouteAction(client, List), "gitlab_runner_controller_token_list"),
		runnerControllerTokenReadSpec("controller_token_get", toolutil.RouteAction(client, Get), "gitlab_runner_controller_token_get"),
		runnerControllerTokenCreateSpec("controller_token_create", toolutil.RouteAction(client, Create), "gitlab_runner_controller_token_create"),
		runnerControllerTokenUpdateSpec("controller_token_rotate", toolutil.RouteAction(client, Rotate), "gitlab_runner_controller_token_rotate"),
		runnerControllerTokenUpdateSpec("controller_token_revoke", toolutil.DestructiveVoidAction(client, Revoke), "gitlab_runner_controller_token_revoke"),
	}
}

func runnerControllerTokenReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := runnerControllerTokenOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func runnerControllerTokenCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, runnerControllerTokenOptions(individualTool))
}

func runnerControllerTokenUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := runnerControllerTokenOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func runnerControllerTokenOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"runner", "controller", "token"},
		OpenWorld:      true,
		OwnerPackage:   "runnercontrollertokens",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
