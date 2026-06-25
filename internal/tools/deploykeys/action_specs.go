package deploykeys

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for deploy key actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		deployKeyReadSpec("deploy_key_list_project", toolutil.RouteAction(client, ListProject), "gitlab_deploy_key_list_project"),
		deployKeyReadSpec("deploy_key_get", toolutil.RouteAction(client, Get), "gitlab_deploy_key_get"),
		deployKeyCreateSpec("deploy_key_add", toolutil.RouteAction(client, Add), "gitlab_deploy_key_add"),
		deployKeyUpdateSpec("deploy_key_update", toolutil.RouteAction(client, Update), "gitlab_deploy_key_update"),
		deployKeyDeleteSpec("deploy_key_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_deploy_key_delete"),
		deployKeyUpdateSpec("deploy_key_enable", toolutil.RouteAction(client, Enable), "gitlab_deploy_key_enable"),
		deployKeyReadSpec("deploy_key_list_all", toolutil.RouteAction(client, ListAll), "gitlab_deploy_key_list_all"),
		deployKeyCreateSpec("deploy_key_add_instance", toolutil.RouteAction(client, AddInstance), "gitlab_deploy_key_add_instance"),
		deployKeyReadSpec("deploy_key_list_user_project", toolutil.RouteAction(client, ListUserProject), "gitlab_deploy_key_list_user_project"),
	}
}

func deployKeyReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, deployKeyOptions(name, individualTool))
}

func deployKeyCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, deployKeyOptions(name, individualTool))
}

func deployKeyUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, deployKeyOptions(name, individualTool))
}

func deployKeyDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, deployKeyOptions(name, individualTool))
}

func deployKeyOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Tags:           []string{"access", "deploy_key", "ssh"},
		Usage:          "Use for SSH deploy keys that grant repository access to projects.",
		Aliases:        deployKeyAliases(actionName),
		RelatedActions: []string{"access.deploy_token_list_project", "access.token_project_list"},
		OpenWorld:      true,
		OwnerPackage:   "deploykeys",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	options.IndividualTool.Description = deployKeyDescription(actionName)
	if actionName == "deploy_key_list_project" {
		options.Usage = "Lists SSH deploy keys, not deploy tokens; use access.deploy_token_list_project when credentials/tokens are requested."
	}
	if actionName == "deploy_key_get" || actionName == "deploy_key_update" || actionName == "deploy_key_delete" || actionName == "deploy_key_enable" {
		options.Usage = "Use deploy_key_id returned by deploy key list/add/get operations. Do not use deploy_token_id; deploy tokens are a different resource."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"deploy_key_id": {
				SemanticRole:   "deploy_key",
				ValueSource:    "Deploy key ID returned by deploy_key_add, deploy_key_get, deploy_key_list_project, or deploy_key_list_all.",
				ExampleBinding: "params.deploy_key_id:1",
				CommonConfusions: []string{
					"Do not send deploy_token_id; deploy keys and deploy tokens are separate access resources.",
					"Do not use token_id for deploy key get/update/delete operations.",
				},
			},
		}
	}
	return options
}

// deployKeyDescription returns the individual-tool description for a deploy key
// action in the canonical "Returns: … See also: …" form (R-META) so models can
// see the output shape and related actions directly in tools/list.
func deployKeyDescription(actionName string) string {
	switch actionName {
	case "deploy_key_list_project":
		return "List a project's SSH deploy keys with ordering and pagination. Returns: deploy keys with id, title, fingerprint, can_push, expiry, and pagination metadata. See also: gitlab_deploy_key_get, gitlab_deploy_key_add, gitlab_deploy_key_enable."
	case "deploy_key_get":
		return "Get a single project deploy key by id. Returns: the deploy key with title, key, fingerprint, fingerprint_sha256, can_push, created_at, and expires_at. See also: gitlab_deploy_key_list_project, gitlab_deploy_key_update, gitlab_deploy_key_delete."
	case "deploy_key_add":
		return "Add a new SSH deploy key to a project. Returns: the created deploy key with id, title, fingerprint, can_push, and expiry. See also: gitlab_deploy_key_get, gitlab_deploy_key_list_project, gitlab_deploy_key_enable."
	case "deploy_key_update":
		return "Update a project deploy key's title or push permission. Returns: the updated deploy key. See also: gitlab_deploy_key_get, gitlab_deploy_key_list_project, gitlab_deploy_key_delete."
	case "deploy_key_delete":
		return "Delete a deploy key from a project (removes it from all projects where it is enabled). Returns: a success confirmation. See also: gitlab_deploy_key_get, gitlab_deploy_key_list_project."
	case "deploy_key_enable":
		return "Enable an existing deploy key for a project. Returns: the enabled deploy key with id, title, fingerprint, and can_push. See also: gitlab_deploy_key_list_all, gitlab_deploy_key_list_project, gitlab_deploy_key_get."
	case "deploy_key_list_all":
		return "List all instance-level deploy keys (admin only) with ordering and pagination. Returns: instance deploy keys with id, title, fingerprint, expiry, projects_with_write_access, projects_with_readonly_access, and pagination metadata. See also: gitlab_deploy_key_add_instance, gitlab_deploy_key_enable, gitlab_deploy_key_list_project."
	case "deploy_key_add_instance":
		return "Create an instance-level deploy key (admin only). Returns: the created instance deploy key with id, title, fingerprint, expiry, and project access arrays. See also: gitlab_deploy_key_list_all, gitlab_deploy_key_enable."
	case "deploy_key_list_user_project":
		return "List the deploy keys across a user's projects (admin only) with ordering and pagination. Returns: deploy keys with id, title, fingerprint, can_push, expiry, and pagination metadata. See also: gitlab_deploy_key_list_project, gitlab_deploy_key_get, gitlab_get_user."
	default:
		return ""
	}
}

func deployKeyAliases(actionName string) []string {
	switch actionName {
	case "deploy_key_list_project":
		return []string{"list project deploy keys", "project deploy keys"}
	case "deploy_key_get":
		return []string{"get deploy key", "fetch deploy key"}
	case "deploy_key_add":
		return []string{"add project deploy key", "create project deploy key"}
	case "deploy_key_update":
		return []string{"update deploy key", "edit deploy key"}
	case "deploy_key_delete":
		return []string{"delete deploy key", "remove deploy key"}
	case "deploy_key_enable":
		return []string{"enable deploy key", "attach deploy key to project"}
	case "deploy_key_list_all":
		return []string{"list all deploy keys", "instance deploy keys"}
	case "deploy_key_add_instance":
		return []string{"add instance deploy key", "create instance deploy key"}
	case "deploy_key_list_user_project":
		return []string{"list user project deploy keys", "user project deploy keys"}
	default:
		return nil
	}
}
