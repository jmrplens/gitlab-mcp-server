package groupvariables

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const actionGroupVariableUpdate = "group_variable.group_update"

// ActionSpecs returns canonical specs for group CI/CD variable actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupVariableReadSpec("group_list", toolutil.RouteAction(client, List), "gitlab_group_variable_list"),
		groupVariableReadSpec("group_get", toolutil.RouteAction(client, Get), "gitlab_group_variable_get"),
		groupVariableCreateSpec("group_create", toolutil.RouteAction(client, Create), "gitlab_group_variable_create"),
		groupVariableUpdateSpec("group_update", toolutil.RouteAction(client, Update), "gitlab_group_variable_update"),
		groupVariableDeleteSpec("group_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_group_variable_delete"),
	}
}

func groupVariableReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupVariableOptionsForAction(name, individualTool))
}

func groupVariableCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, groupVariableOptionsForAction(name, individualTool))
}

func groupVariableUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, groupVariableOptionsForAction(name, individualTool))
}

func groupVariableDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupVariableOptionsForAction(name, individualTool))
}

func groupVariableOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupvariables domain action.", Tags: []string{"ci", "group", "variable"},
		OpenWorld:      true,
		OwnerPackage:   "groupvariables",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case "group_list":
		options.Usage = "List group CI/CD variables with pagination and optional filters. Use to inspect variable inventory before get/update/delete actions."
		options.Aliases = []string{"list group variables", "show group variables", "find group ci variables"}
		options.RelatedActions = []string{"group_variable.group_get", actionGroupVariableUpdate, "group_variable.group_delete"}
		options.IndividualTool.Description = "List a group's CI/CD variables with order_by, sort, and offset or keyset pagination. Returns: each variable's key, value, type, protected/masked/hidden/raw flags, environment scope, description, and pagination metadata. See also: gitlab_group_variable_get, gitlab_group_variable_create, gitlab_group_variable_update."
	case "group_get":
		options.Usage = "Get one group CI/CD variable by key (and optional environment scope). Use when exact variable settings are needed."
		options.Aliases = []string{"get group variable", "show group variable details", "lookup group variable"}
		options.RelatedActions = []string{"group_variable.group_list", actionGroupVariableUpdate}
		options.IndividualTool.Description = "Get a single group CI/CD variable by key, optionally selecting an environment-scoped instance via the filter. Returns: the variable's key, value, type, protected/masked/hidden/raw flags, environment scope, and description. See also: gitlab_group_variable_list, gitlab_group_variable_update, gitlab_group_variable_delete."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"key": {
				SemanticRole:   "ci_variable_key",
				ValueSource:    "Variable key from list output or task context.",
				ExampleBinding: `params.key:"DB_HOST"`,
			},
			"environment_scope": {
				SemanticRole:   "environment_scope",
				ValueSource:    "Optional environment scope string; use * for global variable scope.",
				ExampleBinding: `params.environment_scope:"*"`,
			},
		}
	case "group_create":
		options.Usage = "Create a CI/CD variable in a group. Use for pipeline/runtime configuration shared across the group's projects."
		options.Aliases = []string{"create group variable", "add group variable", "new group ci variable"}
		options.RelatedActions = []string{"group_variable.group_list", "group_variable.group_get", actionGroupVariableUpdate}
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
		options.IndividualTool.Description = "Create a CI/CD variable in a group with type, environment scope, and protected/masked/raw/masked_and_hidden flags. Returns: the created variable's key, value, type, flags, environment scope, and description. See also: gitlab_group_variable_list, gitlab_group_variable_get, gitlab_group_variable_update."
	case "group_update":
		options.Usage = "Update a group CI/CD variable, selecting an environment-scoped instance via the filter. Use to change a variable's value or flags."
		options.Aliases = []string{"update group variable", "edit group variable", "change group ci variable"}
		options.RelatedActions = []string{"group_variable.group_get", "group_variable.group_list", "group_variable.group_delete"}
		options.IndividualTool.Description = "Update a group CI/CD variable, selecting an environment-scoped instance via the filter. Returns: the updated variable's key, value, type, protected/masked/hidden/raw flags, environment scope, and description. See also: gitlab_group_variable_get, gitlab_group_variable_list, gitlab_group_variable_delete."
	case "group_delete":
		options.Usage = "Delete a group CI/CD variable by key, selecting an environment-scoped instance via the filter."
		options.Aliases = []string{"delete group variable", "remove group variable", "drop group ci variable"}
		options.RelatedActions = []string{"group_variable.group_list", "group_variable.group_get"}
		options.IndividualTool.Description = "Delete a group CI/CD variable by key, selecting an environment-scoped instance via the filter. Returns: a success confirmation message. See also: gitlab_group_variable_list, gitlab_group_variable_get."
	}

	return options
}
