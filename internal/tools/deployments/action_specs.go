package deployments

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for deployment actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		deploymentReadSpec("deployment_list", toolutil.RouteAction(client, List), "gitlab_deployment_list"),
		deploymentReadSpec("deployment_get", toolutil.RouteAction(client, Get), "gitlab_deployment_get"),
		deploymentCreateSpec("deployment_create", toolutil.RouteAction(client, Create), "gitlab_deployment_create"),
		deploymentUpdateSpec("deployment_update", toolutil.RouteAction(client, Update), "gitlab_deployment_update"),
		deploymentDeleteSpec("deployment_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_deployment_delete"),
		deploymentGateSpec("deployment_approve_or_reject", toolutil.RouteAction(client, ApproveOrReject), "gitlab_deployment_approve_or_reject"),
	}
}

func deploymentReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := deploymentOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func deploymentCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, deploymentOptions(individualTool))
}

func deploymentUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := deploymentOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func deploymentDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := deploymentOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func deploymentGateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, deploymentOptions(individualTool))
}

func deploymentOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"environment", "deployment"},
		RelatedActions: []string{"environment.get", "pipeline.get"},
		OpenWorld:      true,
		OwnerPackage:   "deployments",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
