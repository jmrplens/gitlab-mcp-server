package usergpgkeys

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for user GPG key actions exposed through gitlab_user.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		userGPGReadSpec("gpg_keys", toolutil.RouteAction(client, List), "gitlab_list_gpg_keys"),
		userGPGReadSpec("gpg_keys_for_user", toolutil.RouteAction(client, ListForUser), "gitlab_list_gpg_keys_for_user"),
		userGPGReadSpec("get_gpg_key", toolutil.RouteAction(client, Get), "gitlab_get_gpg_key"),
		userGPGReadSpec("get_gpg_key_for_user", toolutil.RouteAction(client, GetForUser), "gitlab_get_gpg_key_for_user"),
		userGPGCreateSpec("add_gpg_key", toolutil.RouteAction(client, Add), "gitlab_add_gpg_key"),
		userGPGCreateSpec("add_gpg_key_for_user", toolutil.RouteAction(client, AddForUser), "gitlab_add_gpg_key_for_user"),
		userGPGDeleteSpec("delete_gpg_key", toolutil.DestructiveAction(client, Delete), "gitlab_delete_gpg_key"),
		userGPGDeleteSpec("delete_gpg_key_for_user", toolutil.DestructiveAction(client, DeleteForUser), "gitlab_delete_gpg_key_for_user"),
	}
}

func userGPGReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, userGPGOptions(individualTool))
}

func userGPGCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, userGPGOptions(individualTool))
}

func userGPGDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, userGPGOptions(individualTool))
}

func userGPGOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute usergpgkeys domain action.", Tags: []string{"user", "gpg_key"},
		OpenWorld:      true,
		OwnerPackage:   "usergpgkeys",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
