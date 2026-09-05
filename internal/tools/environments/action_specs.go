package environments

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionDeploymentList  = "deployment.list"
	actionEnvironmentStop = "environment.stop"
	actionEnvironmentList = "environment.list"
	actionEnvironmentGet  = "environment.get"
	actionNameStop        = "stop"
	paramEnvironmentID    = "environment_id"
)

// ActionSpecs returns canonical specs for environment actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		environmentReadSpec("list", toolutil.RouteAction(client, List), "gitlab_environment_list"),
		environmentReadSpec("get", environmentGetRoute(client), "gitlab_environment_get").
			WithEmbeddedResource("gitlab://project/{project_id}/environment/{environment_id}"),
		environmentCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_environment_create"),
		environmentUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_environment_update"),
		environmentDeleteSpec("delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_environment_delete"),
		environmentStopSpec(client),
	}
}

func environmentGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return environmentNotFoundOutput{Identifier: fmt.Sprintf("ID %v in project %v", input[paramEnvironmentID], input["project_id"])}, nil
		}
		return result, err
	}
	return route
}

func environmentReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, environmentOptionsForAction(name, individualTool))
}

func environmentCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, environmentOptionsForAction(name, individualTool))
}

func environmentUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, environmentOptionsForAction(name, individualTool))
}

func environmentDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, environmentOptionsForAction(name, individualTool))
}

func environmentStopSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualDestructive := false
	options := environmentOptionsForAction(actionNameStop, "gitlab_environment_stop")
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewDeleteActionSpec(actionNameStop, toolutil.DestructiveAction(client, Stop), options)
}

func environmentOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute environments domain action.", Tags: []string{"environment", "deployment"},
		RelatedActions: []string{actionDeploymentList, "ci_variable.list", "feature_flags.strategy_list"},
		OpenWorld:      true,
		OwnerPackage:   "environments",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case "list":
		options.Usage = "List environments in one project with filters and pagination. Use this to discover environment IDs before get/update/stop/delete operations."
		options.Aliases = []string{"list environments", "show environments", "find environments"}
		options.RelatedActions = []string{actionEnvironmentGet, actionEnvironmentStop, actionDeploymentList}
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("states", map[string]any{"enum": []any{"available", "stopping", "stopped"}}),
		}
	case "get":
		options.Usage = "Get one environment by environment_id. Use when inspecting state, tier, external URL, and stop behavior of a specific environment."
		options.Aliases = []string{"get environment", "show environment details", "lookup environment"}
		options.RelatedActions = []string{actionEnvironmentList, "environment.update", actionEnvironmentStop}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramEnvironmentID: {
				SemanticRole:   paramEnvironmentID,
				ValueSource:    "Environment numeric ID from environment list output.",
				ExampleBinding: "params.environment_id:7",
			},
		}
	case "create":
		options.Usage = "Create an environment in a project. Use when introducing new runtime targets such as review, staging, or production environments."
		options.Aliases = []string{"create environment", "new environment", "add environment"}
		options.RelatedActions = []string{actionEnvironmentList, "environment.update", "deployment.create"}
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("tier", map[string]any{"enum": []any{"production", "staging", "testing", "development", "other"}}),
			toolutil.SchemaPropertyOverride("auto_stop_setting", map[string]any{"enum": []any{"always", "with_action"}}),
		}
	case "update":
		options.Usage = "Update an existing environment by environment_id. Use to change its name, description, external URL, tier, cluster agent, Kubernetes namespace, Flux resource path, or auto-stop setting."
		options.Aliases = []string{"update environment", "edit environment", "modify environment"}
		options.RelatedActions = []string{actionEnvironmentGet, actionEnvironmentList, actionEnvironmentStop}
		options.IndividualTool.Description = "Update an existing environment in a project. Returns: the updated environment with state, tier, external URL, cluster agent, Kubernetes namespace, Flux resource path, and auto-stop settings. See also: gitlab_environment_get, gitlab_environment_list, gitlab_environment_stop."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("tier", map[string]any{"enum": []any{"production", "staging", "testing", "development", "other"}}),
			toolutil.SchemaPropertyOverride("auto_stop_setting", map[string]any{"enum": []any{"always", "with_action"}}),
		}
	case "delete":
		options.Usage = "Delete an environment by environment_id. The environment must be stopped first. Use to permanently remove a runtime target that is no longer used."
		options.Aliases = []string{"delete environment", "remove environment", "destroy environment"}
		options.RelatedActions = []string{actionEnvironmentStop, actionEnvironmentList, actionEnvironmentGet}
		options.IndividualTool.Description = "Delete a stopped environment from a project. Returns: a success confirmation naming the deleted environment. See also: gitlab_environment_stop, gitlab_environment_list, gitlab_environment_get."
	case actionNameStop:
		options.Usage = "Stop an active environment. This is modeled as a delete-style action but intentionally marked non-destructive because it changes runtime state without deleting the environment resource."
		options.Aliases = []string{"stop environment", "pause environment", "halt environment"}
		options.RelatedActions = []string{actionEnvironmentGet, actionDeploymentList}
	}

	return options
}
