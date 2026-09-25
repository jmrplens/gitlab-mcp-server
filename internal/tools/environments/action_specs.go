package environments

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The spec names of this package's own actions. They are the names the specs
// are registered under, not what a caller names: the catalog qualifies each
// with the domain of the group these specs join, so every published ID goes
// through [canonicalID].
const (
	actionNameList   = "list"
	actionNameGet    = "get"
	actionNameCreate = "create"
	actionNameUpdate = "update"
	actionNameDelete = "delete"
	actionNameStop   = "stop"
)

// catalogDomain is the domain the environment specs are published under, the
// gitlab_environment group's own, and domainPrefix is it with the separator a
// canonical ID puts between the domain and the action. The deployment,
// protected environment and freeze period packages join the same group, which
// is why a deployment action's ID reads "environment.deployment_list" and
// there is no deployment domain for it to live in.
const (
	catalogDomain = "environment"
	domainPrefix  = catalogDomain + "."
)

// The canonical IDs of this package's own actions, and of the sibling
// packages' actions the cross-links name. Every one of ours is its spec name
// above with [domainPrefix] in front, so a rename moves both at once; both the
// RelatedActions below and the Markdown hints in markdown.go read this block,
// so the two cannot drift. They had, and the comment markdown.go carried
// recorded the drift rather than fixing it.
//
// A deployment belongs to the environment group, so its IDs are qualified the
// same way. A CI variable and a feature flag are domains of their own.
const (
	actionEnvironmentList   = domainPrefix + actionNameList
	actionEnvironmentGet    = domainPrefix + actionNameGet
	actionEnvironmentCreate = domainPrefix + actionNameCreate
	actionEnvironmentUpdate = domainPrefix + actionNameUpdate
	actionEnvironmentDelete = domainPrefix + actionNameDelete
	actionEnvironmentStop   = domainPrefix + actionNameStop

	actionDeploymentList   = domainPrefix + "deployment_list"
	actionDeploymentCreate = domainPrefix + "deployment_create"

	actionCIVariableList  = "ci_variable.list"
	actionFeatureFlagList = "feature_flags.feature_flag_list"
)

const paramEnvironmentID = "environment_id"

// ActionSpecs returns canonical specs for environment actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		environmentReadSpec(actionNameList, toolutil.RouteAction(client, List), "gitlab_environment_list"),
		environmentReadSpec(actionNameGet, environmentGetRoute(client), "gitlab_environment_get").
			WithEmbeddedResource("gitlab://project/{project_id}/environment/{environment_id}"),
		environmentCreateSpec(actionNameCreate, toolutil.RouteAction(client, Create), "gitlab_environment_create"),
		environmentUpdateSpec(actionNameUpdate, toolutil.RouteAction(client, Update), "gitlab_environment_update"),
		environmentDeleteSpec(actionNameDelete, toolutil.DestructiveVoidAction(client, Delete), "gitlab_environment_delete"),
		environmentStopSpec(client),
	}
}

func environmentGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	return toolutil.RouteAction(client, Get).WrapHandler(func(next toolutil.ActionFunc) toolutil.ActionFunc {
		return func(ctx context.Context, input map[string]any) (any, error) {
			result, err := next(ctx, input)
			if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
				return environmentNotFoundOutput{Identifier: fmt.Sprintf("ID %s in project %s",
					toolutil.ParamText(input[paramEnvironmentID]), toolutil.ParamText(input["project_id"]))}, nil
			}
			return result, err
		}
	})
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
		RelatedActions: []string{actionDeploymentList, actionCIVariableList, actionFeatureFlagList},
		OpenWorld:      true,
		OwnerPackage:   "environments",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case actionNameList:
		options.Usage = "List environments in one project with filters and pagination. Use this to discover environment IDs before get/update/stop/delete operations."
		options.Aliases = []string{"list environments", "show environments", "find environments"}
		options.RelatedActions = []string{actionEnvironmentGet, actionEnvironmentStop, actionDeploymentList}
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("states", map[string]any{"enum": []any{"available", "stopping", "stopped"}}),
		}
	case actionNameGet:
		options.Usage = "Get one environment by environment_id. Use when inspecting state, tier, external URL, and stop behavior of a specific environment."
		options.Aliases = []string{"get environment", "show environment details", "lookup environment"}
		options.RelatedActions = []string{actionEnvironmentList, actionEnvironmentUpdate, actionEnvironmentStop}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			paramEnvironmentID: {
				SemanticRole:   paramEnvironmentID,
				ValueSource:    "Environment numeric ID from environment list output.",
				ExampleBinding: "params.environment_id:7",
			},
		}
	case actionNameCreate:
		options.Usage = "Create an environment in a project. Use when introducing new runtime targets such as review, staging, or production environments."
		options.Aliases = []string{"create environment", "new environment", "add environment"}
		options.RelatedActions = []string{actionEnvironmentList, actionEnvironmentUpdate, actionDeploymentCreate}
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("tier", map[string]any{"enum": []any{"production", "staging", "testing", "development", "other"}}),
			toolutil.SchemaPropertyOverride("auto_stop_setting", map[string]any{"enum": []any{"always", "with_action"}}),
		}
	case actionNameUpdate:
		options.Usage = "Update an existing environment by environment_id. Use to change its name, description, external URL, tier, cluster agent, Kubernetes namespace, Flux resource path, or auto-stop setting."
		options.Aliases = []string{"update environment", "edit environment", "modify environment"}
		options.RelatedActions = []string{actionEnvironmentGet, actionEnvironmentList, actionEnvironmentStop}
		options.IndividualTool.Description = "Update an existing environment in a project. Returns: the updated environment with state, tier, external URL, cluster agent, Kubernetes namespace, Flux resource path, and auto-stop settings. See also: gitlab_environment_get, gitlab_environment_list, gitlab_environment_stop."
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaPropertyOverride("tier", map[string]any{"enum": []any{"production", "staging", "testing", "development", "other"}}),
			toolutil.SchemaPropertyOverride("auto_stop_setting", map[string]any{"enum": []any{"always", "with_action"}}),
		}
	case actionNameDelete:
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
