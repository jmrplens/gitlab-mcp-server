package deployments

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionDeploymentUpdate = "deployment.update"
	actionDeploymentGet    = "deployment.get"
	actionDeploymentList   = "deployment.list"
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
		options.RelatedActions = []string{actionDeploymentGet, "environment.list", "pipeline.get"}
		options.IndividualTool.Description = "List deployments in a project with environment, status, and date filters plus offset or keyset pagination. Returns: matching deployments with ref, sha, status, user, environment, and deployable (CI job) objects, and pagination metadata. See also: gitlab_deployment_get, gitlab_environment_list, gitlab_pipeline_get."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("status", map[string]any{
				"enum": []any{"created", "running", "success", "failed", "canceled"},
			}),
			toolutil.SchemaPropertyOverride("order_by", map[string]any{
				"enum": []any{"id", "iid", "created_at", "updated_at", "finished_at", "ref"},
			}),
		}
	case "deployment_get":
		options.Usage = "Get one deployment by deployment_id for a project. Use when investigating a specific deployment state, environment, or actor metadata."
		options.Aliases = []string{"get deployment", "show deployment details", "lookup deployment"}
		options.RelatedActions = []string{actionDeploymentList, actionDeploymentUpdate, "deployment.approve_or_reject"}
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
		options.RelatedActions = []string{"environment.get", actionDeploymentList, actionDeploymentUpdate}
		options.IndividualTool.Description = "Create a deployment record for an environment at a given ref and sha with an initial status. Returns: the created deployment with id, iid, ref, sha, status, and nested user, environment, and deployable objects. See also: gitlab_environment_get, gitlab_deployment_list, gitlab_deployment_update."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("status", map[string]any{
				"enum": []any{"created", "running", "success", "failed", "canceled"},
			}),
		}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"tag": {
				SemanticRole:   "ref_is_tag_flag",
				ValueSource:    "Derive from the ref type: false for branch refs, true for tag refs. GitLab 19 rejects the request with 400 'tag is missing' when the field is omitted.",
				ExampleBinding: "params.tag:false",
			},
			"status": {
				SemanticRole: "initial_deployment_status",
				ValueSource:  "One of running, success, failed, or canceled. GitLab 19 rejects 'created' on create with 400 'status does not have a valid value'.",
				CommonConfusions: []string{
					"'created' is a stored deployment status but is not accepted when creating a deployment on GitLab 19",
				},
				ExampleBinding: "params.status:running",
			},
		}
	case "deployment_update":
		options.Usage = "Update an existing deployment's status (created, running, success, failed, or canceled) by deployment_id. Use to transition a deployment after a CI/CD job or manual step completes."
		options.Aliases = []string{"update deployment status", "set deployment status", "transition deployment"}
		options.RelatedActions = []string{actionDeploymentGet, actionDeploymentList, "deployment.approve_or_reject"}
		options.IndividualTool.Description = "Update a deployment's status by deployment_id within a project. Returns: the updated deployment with id, iid, ref, sha, status, and nested user, environment, and deployable objects. See also: gitlab_deployment_get, gitlab_deployment_list, gitlab_deployment_approve_or_reject."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("status", map[string]any{
				"enum": []any{"created", "running", "success", "failed", "canceled", "blocked"},
			}),
		}
	case "deployment_delete":
		options.Usage = "Permanently delete a deployment record by deployment_id. Use only to remove obsolete or erroneous deployment entries. This does not undo the underlying deployment."
		options.Aliases = []string{"delete deployment", "remove deployment", "purge deployment record"}
		options.RelatedActions = []string{actionDeploymentGet, actionDeploymentList}
		options.IndividualTool.Description = "Delete a deployment record by deployment_id within a project. Returns: a confirmation that the deployment was deleted. See also: gitlab_deployment_get, gitlab_deployment_list."
	case "deployment_approve_or_reject":
		options.Usage = "Approve or reject a blocked deployment. Use only when approval workflows require explicit deployment approvals/rejections."
		options.Aliases = []string{"approve deployment", "reject deployment", "deployment approval"}
		options.RelatedActions = []string{actionDeploymentGet, actionDeploymentUpdate}
		options.IndividualTool.Description = "Approve or reject a blocked deployment awaiting protected-environment approval, optionally with a comment and an approval rule to represent. Returns: a confirmation message naming the deployment and the applied status. See also: gitlab_deployment_get, gitlab_deployment_update."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("status", map[string]any{
				"enum": []any{"approved", "rejected"},
			}),
		}
	}

	return options
}
