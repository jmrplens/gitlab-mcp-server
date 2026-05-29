package deploytokens

import (
	"context"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for deploy token actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		deployTokenReadSpec("deploy_token_list_all", toolutil.RouteAction(client, ListAll), "gitlab_deploy_token_list_all"),
		deployTokenReadSpec("deploy_token_list_project", toolutil.RouteAction(client, ListProject), "gitlab_deploy_token_list_project"),
		deployTokenReadSpec("deploy_token_list_group", toolutil.RouteAction(client, ListGroup), "gitlab_deploy_token_list_group"),
		deployTokenReadSpec("deploy_token_get_project", toolutil.RouteAction(client, GetProject), "gitlab_deploy_token_get_project"),
		deployTokenReadSpec("deploy_token_get_group", toolutil.RouteAction(client, GetGroup), "gitlab_deploy_token_get_group"),
		deployTokenCreateSpec("deploy_token_create_project", toolutil.RouteAction(client, CreateProject), "gitlab_deploy_token_create_project"),
		deployTokenCreateSpec("deploy_token_create_group", toolutil.RouteAction(client, CreateGroup), "gitlab_deploy_token_create_group"),
		deployTokenDeleteProjectSpec(client),
		deployTokenDeleteSpec("deploy_token_delete_group", toolutil.DestructiveAction(client, DeleteGroupOutput), "gitlab_deploy_token_delete_group"),
	}
}

// DeleteProjectOutput deletes a project deploy token and returns the canonical success message shape.
func DeleteProjectOutput(ctx context.Context, client *gitlabclient.Client, input DeleteProjectInput) (toolutil.DeleteOutput, error) {
	if err := DeleteProject(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted project deploy token."}, nil
}

// DeleteGroupOutput deletes a group deploy token and returns the canonical success message shape.
func DeleteGroupOutput(ctx context.Context, client *gitlabclient.Client, input DeleteGroupInput) (toolutil.DeleteOutput, error) {
	if err := DeleteGroup(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted group deploy token."}, nil
}

func deployTokenReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, deployTokenOptions(name, individualTool))
}

func deployTokenCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, deployTokenOptions(name, individualTool))
}

func deployTokenDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, deployTokenOptions(name, individualTool))
}

func deployTokenDeleteProjectSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := deployTokenOptions("deploy_token_delete_project", "gitlab_deploy_token_delete_project")
	options.Usage = "Use to delete a deploy token owned by a project; pass the deploy token ID, not another token type."
	options.RelatedActions = []string{"access.deploy_token_list_project", "access.deploy_token_get_project", "access.deploy_token_create_project"}
	options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
		"project_id": {
			SemanticRole: "scope_owner_project",
			ValueSource:  "Project that owns the deploy token.",
		},
		"deploy_token_id": {
			SemanticRole:     "deploy_token",
			ValueSource:      "Deploy token ID, not a project, deploy key, personal token, or runner ID.",
			CommonConfusions: []string{"Do not use deploy_key_id or token_id for project deploy token deletion."},
		},
	}
	return toolutil.NewDeleteActionSpec("deploy_token_delete_project", toolutil.DestructiveAction(client, DeleteProjectOutput), options)
}

func deployTokenOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	aliases := []string{individualTool}
	usage := "Manage deploy tokens across instance, project, and group scopes."
	relatedActions := []string{"access.deploy_key_list_project", "project.get", "group.get"}
	guidance := map[string]toolutil.ParameterGuidance{}

	switch actionName {
	case "deploy_token_list_project", "deploy_token_get_project", "deploy_token_create_project", "deploy_token_delete_project":
		guidance["project_id"] = toolutil.ParameterGuidance{
			SemanticRole:   "scope_project",
			ValueSource:    "Project ID or path owning the deploy token.",
			ExampleBinding: `params.project_id:"group/project"`,
		}
		usage = "Manage project deploy tokens/credentials (list/get/create/delete)."
		relatedActions = []string{"access.deploy_token_list_project", "project.get"}
	case "deploy_token_list_group", "deploy_token_get_group", "deploy_token_create_group", "deploy_token_delete_group":
		guidance["group_id"] = toolutil.ParameterGuidance{
			SemanticRole:   "scope_group",
			ValueSource:    "Group ID or path owning the deploy token.",
			ExampleBinding: `params.group_id:"my-group"`,
		}
		usage = "Manage group deploy tokens (list/get/create/delete)."
		relatedActions = []string{"access.deploy_token_list_group", "group.get"}
	}

	if actionName == "deploy_token_get_project" || actionName == "deploy_token_get_group" || actionName == "deploy_token_delete_project" || actionName == "deploy_token_delete_group" {
		guidance["deploy_token_id"] = toolutil.ParameterGuidance{
			SemanticRole:   "deploy_token",
			ValueSource:    "Deploy token ID returned by deploy token list/get actions.",
			ExampleBinding: "params.deploy_token_id:2",
			CommonConfusions: []string{
				"Do not use deploy_key_id; deploy keys are a different access resource.",
			},
		}
	}

	return toolutil.ActionSpecOptions{
		Aliases:           aliases,
		Tags:              []string{"access", "deploy_token"},
		Usage:             usage,
		RelatedActions:    relatedActions,
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "deploytokens",
		IndividualTool:    toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
