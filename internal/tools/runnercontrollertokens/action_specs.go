package runnercontrollertokens

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for runner controller token actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		runnerControllerTokenReadSpec("controller_token_list", toolutil.RouteAction(client, List), "gitlab_runner_controller_token_list"),
		runnerControllerTokenReadSpec("controller_token_get", toolutil.RouteAction(client, Get), "gitlab_runner_controller_token_get"),
		runnerControllerTokenCreateSpec("controller_token_create", toolutil.RouteAction(client, Create), "gitlab_runner_controller_token_create"),
		runnerControllerTokenUpdateSpec("controller_token_rotate", toolutil.RouteAction(client, Rotate), "gitlab_runner_controller_token_rotate"),
		runnerControllerTokenDeleteSpec("controller_token_revoke", toolutil.DestructiveAction(client, revokeOutput), "gitlab_runner_controller_token_revoke"),
	}
}

func revokeOutput(ctx context.Context, client *gitlabclient.Client, input RevokeInput) (toolutil.DeleteOutput, error) {
	if err := Revoke(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	_, out, _ := toolutil.DeleteResult("runner controller token")
	return out, nil
}

func runnerControllerTokenReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, runnerControllerTokenOptions(individualTool))
}

func runnerControllerTokenCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, runnerControllerTokenOptions(individualTool))
}

func runnerControllerTokenUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, runnerControllerTokenOptions(individualTool))
}

func runnerControllerTokenDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, runnerControllerTokenOptions(individualTool))
}

func runnerControllerTokenOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute runnercontrollertokens domain action.", Tags: []string{"runner", "controller", "token"},
		OpenWorld:      true,
		OwnerPackage:   "runnercontrollertokens",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
