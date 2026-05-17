package deployments

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for deployment actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		deploymentReadSpec("deployment_list", toolutil.RouteAction(client, List), "gitlab_deployment_list"),
		deploymentReadSpec("deployment_get", deploymentGetRoute(client), "gitlab_deployment_get"),
		deploymentCreateSpec("deployment_create", toolutil.RouteAction(client, Create), "gitlab_deployment_create"),
		deploymentUpdateSpec("deployment_update", toolutil.RouteAction(client, Update), "gitlab_deployment_update"),
		deploymentDeleteSpec("deployment_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_deployment_delete"),
		deploymentUpdateSpec("deployment_approve_or_reject", toolutil.RouteAction(client, ApproveOrReject), "gitlab_deployment_approve_or_reject"),
	}
}

func deploymentGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return deploymentNotFoundOutput{Identifier: fmt.Sprintf("ID %v in project %v", input["deployment_id"], input["project_id"])}, nil
		}
		return result, err
	}
	return route
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

func deploymentOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"environment", "deployment"},
		RelatedActions: []string{"environment.get", "pipeline.get"},
		OpenWorld:      true,
		OwnerPackage:   "deployments",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
