package terraformstates

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for Terraform state tools.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		terraformStateReadSpec("terraform_state_list", toolutil.RouteAction(client, List), "gitlab_list_terraform_states"),
		terraformStateReadSpec("terraform_state_get", toolutil.RouteAction(client, Get), "gitlab_get_terraform_state"),
		terraformStateDeleteSpec("terraform_state_delete", toolutil.DestructiveVoidAction(client, Delete), "gitlab_delete_terraform_state"),
		terraformStateDeleteSpec("terraform_version_delete", toolutil.DestructiveVoidAction(client, DeleteVersion), "gitlab_delete_terraform_state_version"),
		terraformStateUpdateSpec("terraform_state_lock", toolutil.RouteAction(client, Lock), "gitlab_lock_terraform_state"),
		terraformStateUnlockSpec(client),
	}
}

func terraformStateReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, terraformStateOptions(individualTool))
}

func terraformStateUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, terraformStateOptions(individualTool))
}

func terraformStateUnlockSpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualDestructive := false
	options := terraformStateOptions("gitlab_unlock_terraform_state")
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewDeleteActionSpec("terraform_state_unlock", toolutil.DestructiveAction(client, Unlock), options)
}

func terraformStateDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, terraformStateOptions(individualTool))
}

func terraformStateOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute terraformstates domain action.", Tags: []string{"terraform-state"},
		OpenWorld:      true,
		OwnerPackage:   "terraformstates",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
