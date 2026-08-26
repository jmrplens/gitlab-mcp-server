package instancevariables

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionInstanceVariableGet    = "instancevariables.instance_get"
	actionInstanceVariableUpdate = "instancevariables.instance_update"
	actionInstanceVariableDelete = "instancevariables.instance_delete"
	actionInstanceVariableList   = "instancevariables.instance_list"
)

// ActionSpecs returns canonical specs for instance CI/CD variable actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		instanceVariableReadSpec("instance_list", toolutil.RouteAction(client, List), "gitlab_instance_variable_list"),
		instanceVariableReadSpec("instance_get", toolutil.RouteAction(client, Get), "gitlab_instance_variable_get"),
		instanceVariableCreateSpec("instance_create", toolutil.RouteAction(client, Create), "gitlab_instance_variable_create"),
		instanceVariableUpdateSpec("instance_update", toolutil.RouteAction(client, Update), "gitlab_instance_variable_update"),
		instanceVariableDeleteSpec("instance_delete", toolutil.DestructiveAction(client, deleteOutput), "gitlab_instance_variable_delete"),
	}
}

func deleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted instance CI/CD variable."}, nil
}

func instanceVariableReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, instanceVariableOptions(name, individualTool))
}

func instanceVariableCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, instanceVariableOptions(name, individualTool))
}

func instanceVariableUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, instanceVariableOptions(name, individualTool))
}

func instanceVariableDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, instanceVariableOptions(name, individualTool))
}

func instanceVariableOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute instancevariables domain action.", Tags: []string{"ci", "instance", "variable"},
		OpenWorld:      true,
		OwnerPackage:   "instancevariables",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch actionName {
	case "instance_list":
		options.Usage = "List instance-level CI/CD variables with order_by, sort, and offset or keyset pagination. Admin-only. Use to inspect the instance variable inventory before get/update/delete actions."
		options.Aliases = []string{individualTool, "list instance variables", "show instance variables", "find instance ci variables"}
		options.RelatedActions = []string{actionInstanceVariableGet, "instancevariables.instance_create", actionInstanceVariableUpdate, actionInstanceVariableDelete}
		options.IndividualTool.Description = "List instance-level CI/CD variables with order_by, sort, and offset or keyset pagination (admin-only). Returns: each variable's key, value (raw even when masked — masking redacts CI job logs, not this API), type, protected/masked/raw flags, description, and pagination metadata. See also: gitlab_instance_variable_get, gitlab_instance_variable_create, gitlab_instance_variable_update, gitlab_instance_variable_delete."
	case "instance_get":
		options.Usage = "Get one instance-level CI/CD variable by key. Admin-only. Use when exact variable settings are needed."
		options.Aliases = []string{individualTool, "get instance variable", "show instance variable details", "lookup instance variable"}
		options.RelatedActions = []string{actionInstanceVariableList, actionInstanceVariableUpdate, actionInstanceVariableDelete}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"key": {
				SemanticRole:   "ci_variable_key",
				ValueSource:    "Variable key from list output or task context.",
				ExampleBinding: `params.key:"DEPLOY_TOKEN"`,
			},
		}
		options.IndividualTool.Description = "Get a single instance-level CI/CD variable by key (admin-only). Returns: the variable's key, value (raw even when masked — masking redacts CI job logs, not this API), type, protected/masked/raw flags, and description. See also: gitlab_instance_variable_list, gitlab_instance_variable_update, gitlab_instance_variable_delete."
	case "instance_create":
		options.Usage = "Create an instance-level CI/CD variable. Admin-only. Use for instance-wide pipeline/runtime configuration that should not be stored in repository files."
		options.Aliases = []string{individualTool, "create instance variable", "add instance variable", "new instance ci variable"}
		options.RelatedActions = []string{actionInstanceVariableList, actionInstanceVariableGet, actionInstanceVariableUpdate}
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
		options.IndividualTool.Description = "Create an instance-level CI/CD variable with type and protected/masked/raw flags (admin-only). Returns: the created variable's key, value (raw even when masked — masking redacts CI job logs, not this API), type, flags, and description. See also: gitlab_instance_variable_list, gitlab_instance_variable_get, gitlab_instance_variable_update."
	case "instance_update":
		options.Usage = "Update an instance-level CI/CD variable by key. Admin-only."
		options.Aliases = []string{individualTool, "update instance variable", "edit instance variable", "change instance ci variable"}
		options.RelatedActions = []string{actionInstanceVariableGet, actionInstanceVariableList, actionInstanceVariableDelete}
		options.IndividualTool.Description = "Update an instance-level CI/CD variable by key, with type and protected/masked/raw flags (admin-only). Returns: the updated variable's key, value (raw even when masked — masking redacts CI job logs, not this API), type, protected/masked/raw flags, and description. See also: gitlab_instance_variable_get, gitlab_instance_variable_list, gitlab_instance_variable_delete."
	case "instance_delete":
		options.Usage = "Delete an instance-level CI/CD variable by key. Admin-only."
		options.Aliases = []string{individualTool, "delete instance variable", "remove instance variable", "drop instance ci variable"}
		options.RelatedActions = []string{actionInstanceVariableList, actionInstanceVariableGet}
		options.IndividualTool.Description = "Delete an instance-level CI/CD variable by key (admin-only). Returns: a success confirmation message. See also: gitlab_instance_variable_list, gitlab_instance_variable_get."
	}

	return options
}
