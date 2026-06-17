package usergpgkeys

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for user GPG key actions exposed through gitlab_user.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		// gitlab_list_gpg_keys — list the current user's GPG keys.
		userGPGReadSpec("gpg_keys", toolutil.RouteAction(client, List), "gitlab_list_gpg_keys"),
		// gitlab_list_gpg_keys_for_user — list the GPG keys for a specific user.
		userGPGReadSpec("gpg_keys_for_user", toolutil.RouteAction(client, ListForUser), "gitlab_list_gpg_keys_for_user"),
		// gitlab_get_gpg_key — fetch a single GPG key by ID for the current user.
		userGPGReadSpec("get_gpg_key", toolutil.RouteAction(client, Get), "gitlab_get_gpg_key"),
		// gitlab_get_gpg_key_for_user — fetch a single GPG key for a specific user.
		userGPGReadSpec("get_gpg_key_for_user", toolutil.RouteAction(client, GetForUser), "gitlab_get_gpg_key_for_user"),
		// gitlab_add_gpg_key — add a GPG key to the current user's account.
		userGPGCreateSpec("add_gpg_key", toolutil.RouteAction(client, Add), "gitlab_add_gpg_key"),
		// gitlab_add_gpg_key_for_user — add a GPG key to a specific user's account (admin-only).
		userGPGCreateSpec("add_gpg_key_for_user", toolutil.RouteAction(client, AddForUser), "gitlab_add_gpg_key_for_user"),
		// gitlab_delete_gpg_key — delete a GPG key from the current user's account.
		userGPGDeleteSpec("delete_gpg_key", toolutil.DestructiveAction(client, Delete), "gitlab_delete_gpg_key"),
		// gitlab_delete_gpg_key_for_user — delete a GPG key from a specific user's account (admin-only).
		userGPGDeleteSpec("delete_gpg_key_for_user", toolutil.DestructiveAction(client, DeleteForUser), "gitlab_delete_gpg_key_for_user"),
	}
}

// userGPGReadSpec builds the canonical read-only spec for a user GPG key tool.
func userGPGReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, userGPGOptions(individualTool))
}

// userGPGCreateSpec builds the canonical create spec for a user GPG key tool.
func userGPGCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, userGPGOptions(individualTool))
}

// userGPGDeleteSpec builds the canonical destructive delete spec for a user GPG key tool.
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
