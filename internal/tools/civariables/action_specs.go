package civariables

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionCIVariableUpdate = "ci_variable.update"
	actionCIVariableGet    = "ci_variable.get"
	actionCIVariableList   = "ci_variable.list"
)

// ActionSpecs returns canonical specs for project CI/CD variable actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		ciVariableReadSpec("list", toolutil.RouteAction(client, List), "gitlab_ci_variable_list"),
		ciVariableReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_ci_variable_get"),
		ciVariableCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_ci_variable_create"),
		ciVariableUpdateSpec("update", toolutil.RouteAction(client, Update), "gitlab_ci_variable_update"),
		ciVariableDeleteSpec("delete", toolutil.DestructiveAction(client, DeleteOutput), "gitlab_ci_variable_delete"),
	}
}

func ciVariableReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, ciVariableOptionsForAction(name, individualTool))
}

func ciVariableCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, ciVariableOptionsForAction(name, individualTool))
}

func ciVariableUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, ciVariableOptionsForAction(name, individualTool))
}

func ciVariableDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, ciVariableOptionsForAction(name, individualTool))
}

func ciVariableOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute civariables domain action.", Tags: []string{"ci", "variable"},
		OpenWorld:      true,
		OwnerPackage:   "civariables",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case "list":
		options.Usage = "List project CI/CD variables with pagination and optional filters. Use to inspect variable inventory before get/update/delete actions."
		options.Aliases = []string{"list ci variables", "show project variables", "find ci variables"}
		options.RelatedActions = []string{actionCIVariableGet, actionCIVariableUpdate, "ci_variable.delete"}
		options.IndividualTool.Description = "List a project's CI/CD variables with order_by, sort, and offset or keyset pagination. Returns: each variable's key, type, protected/masked/hidden/raw flags, environment scope, description, and pagination metadata. See also: gitlab_ci_variable_get, gitlab_ci_variable_create, gitlab_ci_variable_update."
	case "get":
		options.Usage = "Get one CI/CD variable by key (and optional environment scope). Use when exact variable settings are needed."
		options.Aliases = []string{"get ci variable", "show variable details", "lookup variable"}
		options.RelatedActions = []string{actionCIVariableList, actionCIVariableUpdate}
		options.IndividualTool.Description = "Get a single CI/CD variable by key, optionally selecting an environment-scoped instance via the filter. Returns: the variable's key, value (redacted when masked or hidden), type, protected/masked/hidden/raw flags, environment scope, and description. See also: gitlab_ci_variable_list, gitlab_ci_variable_update, gitlab_ci_variable_delete."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"key": {
				SemanticRole:   "ci_variable_key",
				ValueSource:    "Variable key from list output or task context.",
				ExampleBinding: `params.key:"DB_HOST"`,
			},
			"environment_scope": {
				SemanticRole:   "environment_scope",
				ValueSource:    "Optional environment scope string. Use * for global variable scope.",
				ExampleBinding: `params.environment_scope:"*"`,
			},
		}
	case "create":
		options.Usage = "Create a CI/CD variable in a project. Use for pipeline/runtime configuration that should not be stored in repository files."
		options.Aliases = []string{"create ci variable", "add pipeline variable", "new ci variable"}
		options.RelatedActions = []string{actionCIVariableList, actionCIVariableGet, actionCIVariableUpdate}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"key": {
				SemanticRole:   "ci_variable_key",
				ValueSource:    "Variable key name to create.",
				ExampleBinding: `params.key:"API_TOKEN"`,
			},
			"value": {
				SemanticRole:   "ci_variable_value",
				ValueSource:    "Variable value text (secret or plain based on masked/protected settings).",
				ExampleBinding: `params.value:"example-value"`,
			},
		}
		options.IndividualTool.Description = "Create a CI/CD variable in a project with type, environment scope, and protected/masked/raw/masked_and_hidden flags. Returns: the created variable's key, value (redacted when masked or hidden), type, flags, environment scope, and description. See also: gitlab_ci_variable_list, gitlab_ci_variable_get, gitlab_ci_variable_update."
	case "update":
		options.Usage = "Update an existing project CI/CD variable's value or flags by key (and optional environment scope). Use to rotate secrets or change protected/masked settings without recreating the variable."
		options.Aliases = []string{"update ci variable", "edit pipeline variable", "rotate ci variable value"}
		options.RelatedActions = []string{actionCIVariableGet, actionCIVariableList, "ci_variable.delete"}
		options.IndividualTool.Description = "Update a CI/CD variable, selecting an environment-scoped instance via the filter. Returns: the updated variable's key, value (redacted when masked or hidden), type, protected/masked/hidden/raw flags, environment scope, and description. See also: gitlab_ci_variable_get, gitlab_ci_variable_list, gitlab_ci_variable_delete."
	case "delete":
		options.Usage = "Delete a project CI/CD variable by key (and optional environment scope). Use to remove obsolete or leaked pipeline configuration values."
		options.Aliases = []string{"delete ci variable", "remove pipeline variable", "drop ci variable"}
		options.RelatedActions = []string{actionCIVariableList, actionCIVariableGet}
		options.IndividualTool.Description = "Delete a CI/CD variable by key, selecting an environment-scoped instance via the filter. Returns: a success confirmation message. See also: gitlab_ci_variable_list, gitlab_ci_variable_get."
	}

	return options
}

// DeleteOutput deletes a CI/CD variable and returns the canonical success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted CI/CD variable."}, nil
}
