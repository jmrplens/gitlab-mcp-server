package groupcredentials

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const actionGroupGet = "group.get"

// ActionSpecs returns canonical specs for group credential inventory actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		groupCredentialReadSpec("credential_list_pats", toolutil.RouteAction(client, ListPATs), "gitlab_list_group_personal_access_tokens",
			"List a group's enterprise personal access tokens. Returns: tokens with name, scopes, owner user ID, active/revoked flags, created and last-used dates, expiry, and pagination metadata. See also: gitlab_revoke_group_personal_access_token, gitlab_list_group_ssh_keys, gitlab_group_get."),
		groupCredentialReadSpec("credential_list_ssh_keys", toolutil.RouteAction(client, ListSSHKeys), "gitlab_list_group_ssh_keys",
			"List a group's enterprise SSH keys. Returns: keys with title, owner user ID, usage type, created/expiry/last-used dates, and pagination metadata. See also: gitlab_delete_group_ssh_key, gitlab_list_group_personal_access_tokens, gitlab_group_get."),
		groupCredentialDeleteSpec("credential_revoke_pat", toolutil.DestructiveAction(client, revokePATOutput), "gitlab_revoke_group_personal_access_token",
			"Revoke an enterprise personal access token in a group. Returns: a success confirmation naming the token and group. See also: gitlab_list_group_personal_access_tokens, gitlab_group_get."),
		groupCredentialDeleteSpec("credential_delete_ssh_key", toolutil.DestructiveAction(client, deleteSSHKeyOutput), "gitlab_delete_group_ssh_key",
			"Delete an enterprise SSH key in a group. Returns: a success confirmation naming the key and group. See also: gitlab_list_group_ssh_keys, gitlab_group_get."),
	}
}

func revokePATOutput(ctx context.Context, client *gitlabclient.Client, input RevokePATInput) (toolutil.DeleteOutput, error) {
	if err := RevokePAT(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{
		Status:  "success",
		Message: fmt.Sprintf("Successfully deleted personal access token %d from group %s.", input.TokenID, input.GroupID),
	}, nil
}

func deleteSSHKeyOutput(ctx context.Context, client *gitlabclient.Client, input DeleteSSHKeyInput) (toolutil.DeleteOutput, error) {
	if err := DeleteSSHKey(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{
		Status:  "success",
		Message: fmt.Sprintf("Successfully deleted SSH key %d from group %s.", input.KeyID, input.GroupID),
	}, nil
}

func groupCredentialReadSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, groupCredentialOptions(individualTool, description))
}

func groupCredentialDeleteSpec(name string, route toolutil.ActionRoute, individualTool, description string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, groupCredentialOptions(individualTool, description))
}

func groupCredentialOptions(individualTool, description string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupcredentials domain action.", Tags: []string{"group", "credential"},
		RelatedActions: []string{actionGroupGet},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupcredentials",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
	decorateGroupCredentialMeta(&options, individualTool)
	if individualTool == "gitlab_list_group_personal_access_tokens" {
		options.InputSchemaOverrides = []toolutil.InputSchemaOverride{
			toolutil.SchemaEnumOverride("state", "active", "inactive"),
		}
	}
	return options
}

// decorateGroupCredentialMeta fills non-generic action-specific Usage,
// distinctive natural-language Aliases, and canonical RelatedActions for each
// group-credential tool from groupCredentialActionMeta. Aliases use group
// credential inventory phrasing distinct from the accesstokens and groupsshcerts
// domains, and always retain the tool name so discovery by exact name still
// works (1:1 audit R-META).
func decorateGroupCredentialMeta(options *toolutil.ActionSpecOptions, individualTool string) {
	meta, ok := groupCredentialActionMeta[individualTool]
	if !ok {
		return
	}
	if meta.usage != "" {
		options.Usage = meta.usage
	}
	if len(meta.aliases) > 0 {
		options.Aliases = append([]string{individualTool}, meta.aliases...)
	}
	if len(meta.related) > 0 {
		options.RelatedActions = append([]string(nil), meta.related...)
	}
}

// groupCredentialActionMetaEntry is the discovery metadata for one
// group-credential action.
type groupCredentialActionMetaEntry struct {
	usage   string
	aliases []string
	related []string
}

// groupCredentialActionMeta maps each individual group-credential tool to its
// discovery metadata. Aliases describe group-level credential inventory and
// revocation so they do not collide with the per-user accesstokens or
// groupsshcerts surfaces.
var groupCredentialActionMeta = map[string]groupCredentialActionMetaEntry{
	"gitlab_list_group_personal_access_tokens": {
		usage:   "Inventory the enterprise personal access tokens issued under a group. Use when auditing which group-owned PATs exist, their scopes, owners, expiry, and revocation state. Filter by search, state, revoked, created/last-used dates. Requires Ultimate and Owner or admin access.",
		aliases: []string{"audit group personal access tokens", "list enterprise group PATs", "group token inventory", "review group-owned access tokens"},
		related: []string{"group.credential_revoke_pat", "group.credential_list_ssh_keys", actionGroupGet},
	},
	"gitlab_list_group_ssh_keys": {
		usage:   "Inventory the enterprise SSH keys registered under a group. Use when auditing which group members' SSH keys exist, their titles, owners, usage type, and expiry. Filter by created/expiry dates. Requires Ultimate and Owner or admin access.",
		aliases: []string{"audit group SSH keys", "list enterprise group SSH keys", "group SSH key inventory", "review group member SSH keys"},
		related: []string{"group.credential_delete_ssh_key", "group.credential_list_pats", actionGroupGet},
	},
	"gitlab_revoke_group_personal_access_token": {
		usage:   "Revoke one enterprise personal access token belonging to a group, by token_id. Destructive and irreversible. Confirm group_id and token_id from credential_list_pats first. Requires Ultimate and Owner or admin access.",
		aliases: []string{"revoke group PAT from credentials inventory", "kill enterprise group PAT", "disable group-owned token", "revoke group member personal access token"},
		related: []string{"group.credential_list_pats", actionGroupGet},
	},
	"gitlab_delete_group_ssh_key": {
		usage:   "Delete one enterprise SSH key belonging to a group, by key_id. Destructive and irreversible. Confirm group_id and key_id from credential_list_ssh_keys first. Requires Ultimate and Owner or admin access.",
		aliases: []string{"delete group SSH key", "remove enterprise group SSH key", "revoke group member SSH key", "purge group-owned SSH key"},
		related: []string{"group.credential_list_ssh_keys", actionGroupGet},
	},
}
