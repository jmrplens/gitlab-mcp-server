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
	options := deployTokenOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func deployTokenCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, deployTokenOptions(individualTool))
}

func deployTokenDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := deployTokenOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func deployTokenDeleteProjectSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	options := deployTokenOptions("gitlab_deploy_token_delete_project")
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
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec("deploy_token_delete_project", toolutil.DestructiveAction(client, DeleteProjectOutput), options)
}

func deployTokenOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"access", "deploy_token"},
		OpenWorld:      true,
		OwnerPackage:   "deploytokens",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
