package deploykeys

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
	options := deployKeyOptions(name, individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func deployKeyCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, deployKeyOptions(name, individualTool))
}

func deployKeyUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := deployKeyOptions(name, individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func deployKeyDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := deployKeyOptions(name, individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func deployKeyOptions(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Tags:           []string{"access", "deploy_key", "ssh"},
		OpenWorld:      true,
		OwnerPackage:   "deploykeys",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if actionName == "deploy_key_get" || actionName == "deploy_key_update" || actionName == "deploy_key_delete" || actionName == "deploy_key_enable" {
		options.Usage = "Use deploy_key_id returned by deploy key list/add/get operations. Do not use deploy_token_id; deploy tokens are a different resource."
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"deploy_key_id": {
				SemanticRole: "deploy_key",
				ValueSource:  "Deploy key ID returned by deploy_key_add, deploy_key_get, deploy_key_list_project, or deploy_key_list_all.",
				CommonConfusions: []string{
					"Do not send deploy_token_id; deploy keys and deploy tokens are separate access resources.",
					"Do not use token_id for deploy key get/update/delete operations.",
				},
			},
		}
	}
	return options
}
