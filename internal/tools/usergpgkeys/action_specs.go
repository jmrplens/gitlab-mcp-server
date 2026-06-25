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

// userGPGMeta carries the non-generic discovery metadata (natural-language
// usage, aliases, related actions, and the "Returns: … See also: …" individual
// tool description) for each user GPG key action, mirroring the issues domain.
type userGPGMeta struct {
	usage       string
	aliases     []string
	related     []string
	description string
}

// userGPGActionMeta maps each individual tool name to its discovery metadata.
var userGPGActionMeta = map[string]userGPGMeta{
	"gitlab_list_gpg_keys": {
		usage:       "List every GPG key on the authenticated user's own account. Use when the prompt asks to view, audit, or pick one of your own GPG signing keys; no parameters are required.",
		aliases:     []string{"list gpg keys", "show my gpg keys", "view my gpg signing keys"},
		related:     []string{"user.get_gpg_key", "user.add_gpg_key", "user.delete_gpg_key", "user.gpg_keys_for_user"},
		description: "List the authenticated user's GPG keys. Returns: each key's ID, armored public key, and creation timestamp. See also: gitlab_get_gpg_key, gitlab_add_gpg_key, gitlab_delete_gpg_key, gitlab_list_gpg_keys_for_user.",
	},
	"gitlab_list_gpg_keys_for_user": {
		usage:       "List every GPG key registered on a specific user's account by user_id. Use after resolving a user with gitlab_get_user; viewing another user's GPG keys may require an admin token.",
		aliases:     []string{"list gpg keys for user", "show another user's gpg keys", "get user gpg keys"},
		related:     []string{"user.get_gpg_key_for_user", "user.add_gpg_key_for_user", "user.delete_gpg_key_for_user", "user.get"},
		description: "List a specific user's GPG keys. Returns: each key's ID, armored public key, and creation timestamp. See also: gitlab_get_gpg_key_for_user, gitlab_add_gpg_key_for_user, gitlab_delete_gpg_key_for_user, gitlab_get_user.",
	},
	"gitlab_get_gpg_key": {
		usage:       "Fetch a single GPG key belonging to the authenticated user by key_id. Use after listing keys when the exact key ID is known and the full armored key is needed.",
		aliases:     []string{"get gpg key", "show gpg key", "fetch my gpg key"},
		related:     []string{"user.gpg_keys", "user.add_gpg_key", "user.delete_gpg_key"},
		description: "Get one GPG key from the authenticated user's account by ID. Returns: the key's ID, armored public key, and creation timestamp. See also: gitlab_list_gpg_keys, gitlab_add_gpg_key, gitlab_delete_gpg_key.",
	},
	"gitlab_get_gpg_key_for_user": {
		usage:       "Fetch a single GPG key for a specific user by user_id and key_id. Use after listing that user's keys; admin token may be required to read another user's keys.",
		aliases:     []string{"get gpg key for user", "show another user's gpg key", "fetch user gpg key"},
		related:     []string{"user.gpg_keys_for_user", "user.add_gpg_key_for_user", "user.delete_gpg_key_for_user", "user.get"},
		description: "Get one GPG key from a specific user's account by ID. Returns: the key's ID, armored public key, and creation timestamp. See also: gitlab_list_gpg_keys_for_user, gitlab_add_gpg_key_for_user, gitlab_delete_gpg_key_for_user, gitlab_get_user.",
	},
	"gitlab_add_gpg_key": {
		usage:       "Add a GPG key to the authenticated user's own account. The key must be an ASCII-armored OpenPGP public key block whose fingerprint is unique across GitLab.",
		aliases:     []string{"add gpg key", "register gpg key", "upload my gpg key"},
		related:     []string{"user.gpg_keys", "user.get_gpg_key", "user.delete_gpg_key", "user.add_gpg_key_for_user"},
		description: "Add a GPG key to the authenticated user's account. Returns: the created key's ID, armored public key, and creation timestamp. See also: gitlab_list_gpg_keys, gitlab_get_gpg_key, gitlab_delete_gpg_key, gitlab_add_gpg_key_for_user.",
	},
	"gitlab_add_gpg_key_for_user": {
		usage:       "Add a GPG key to a specific user's account by user_id (admin only). The key must be an ASCII-armored OpenPGP public key block whose fingerprint is unique across GitLab.",
		aliases:     []string{"add gpg key for user", "register gpg key for user", "upload user gpg key"},
		related:     []string{"user.gpg_keys_for_user", "user.get_gpg_key_for_user", "user.delete_gpg_key_for_user", "user.add_gpg_key"},
		description: "Add a GPG key to a specific user's account (admin only). Returns: the created key's ID, armored public key, and creation timestamp. See also: gitlab_list_gpg_keys_for_user, gitlab_get_gpg_key_for_user, gitlab_delete_gpg_key_for_user, gitlab_add_gpg_key.",
	},
	"gitlab_delete_gpg_key": {
		usage:       "Delete a GPG key from the authenticated user's own account by key_id. Destructive and irreversible; confirm the key_id before calling.",
		aliases:     []string{"delete gpg key", "remove gpg key", "revoke my gpg key"},
		related:     []string{"user.gpg_keys", "user.get_gpg_key", "user.add_gpg_key", "user.delete_gpg_key_for_user"},
		description: "Delete a GPG key from the authenticated user's account. Returns: a confirmation naming the deleted key ID. See also: gitlab_list_gpg_keys, gitlab_get_gpg_key, gitlab_add_gpg_key, gitlab_delete_gpg_key_for_user.",
	},
	"gitlab_delete_gpg_key_for_user": {
		usage:       "Delete a GPG key from a specific user's account by user_id and key_id (admin only). Destructive and irreversible; confirm both IDs before calling.",
		aliases:     []string{"delete gpg key for user", "remove user gpg key", "revoke user gpg key"},
		related:     []string{"user.gpg_keys_for_user", "user.get_gpg_key_for_user", "user.add_gpg_key_for_user", "user.delete_gpg_key"},
		description: "Delete a GPG key from a specific user's account (admin only). Returns: a confirmation naming the deleted key ID. See also: gitlab_list_gpg_keys_for_user, gitlab_get_gpg_key_for_user, gitlab_add_gpg_key_for_user, gitlab_delete_gpg_key.",
	},
}

func userGPGOptions(individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute usergpgkeys domain action.", Tags: []string{"user", "gpg_key"},
		OpenWorld:      true,
		OwnerPackage:   "usergpgkeys",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
	if meta, ok := userGPGActionMeta[individualTool]; ok {
		if meta.usage != "" {
			options.Usage = meta.usage
		}
		if len(meta.aliases) > 0 {
			options.Aliases = meta.aliases
		}
		if len(meta.related) > 0 {
			options.RelatedActions = meta.related
		}
		if meta.description != "" {
			options.IndividualTool.Description = meta.description
		}
	}
	return options
}
