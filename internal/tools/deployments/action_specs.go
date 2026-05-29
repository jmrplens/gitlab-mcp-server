package deployments

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const actionDeploymentUpdate = "deployment.update"

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
	return toolutil.NewReadActionSpec(name, route, deploymentOptionsForAction(name, individualTool))
}

func deploymentCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, deploymentOptionsForAction(name, individualTool))
}

func deploymentUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, deploymentOptionsForAction(name, individualTool))
}

func deploymentDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, deploymentOptionsForAction(name, individualTool))
}

func deploymentOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute deployments domain action.", Tags: []string{"environment", "deployment"},
		RelatedActions: []string{"environment.get", "pipeline.get"},
		OpenWorld:      true,
		OwnerPackage:   "deployments",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case "deployment_list":
		options.Usage = "Lists deployments in a project with filters and pagination. Use this to audit deployment history and locate deployment IDs for follow-up actions."
		options.Aliases = []string{"list deployments", "show deployment history", "find deployments"}
		options.RelatedActions = []string{"deployment.get", "environment.list", "pipeline.get"}
	case "deployment_get":
		options.Usage = "Get one deployment by deployment_id for a project. Use when investigating a specific deployment state, environment, or actor metadata."
		options.Aliases = []string{"get deployment", "show deployment details", "lookup deployment"}
		options.RelatedActions = []string{"deployment.list", actionDeploymentUpdate, "deployment.approve_or_reject"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"deployment_id": {
				SemanticRole:   "deployment_id",
				ValueSource:    "Deployment numeric ID from deployment list output.",
				ExampleBinding: "params.deployment_id:123",
			},
		}
	case "deployment_create":
		options.Usage = "Create a deployment for an environment/ref/sha. Use when orchestrating manual or API-driven deployment entries."
		options.Aliases = []string{"create deployment", "start deployment", "new deployment"}
		options.RelatedActions = []string{"environment.get", "deployment.list", actionDeploymentUpdate}
	case "deployment_approve_or_reject":
		options.Usage = "Approve or reject a blocked deployment. Use only when approval workflows require explicit deployment approvals/rejections."
		options.Aliases = []string{"approve deployment", "reject deployment", "deployment approval"}
		options.RelatedActions = []string{"deployment.get", actionDeploymentUpdate}
	}

	return options
}
