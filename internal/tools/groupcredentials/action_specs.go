package groupcredentials

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

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
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute groupcredentials domain action.", Tags: []string{"group", "credential"},
		RelatedActions: []string{"group.get"},
		Edition:        "premium",
		OpenWorld:      true,
		OwnerPackage:   "groupcredentials",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool), Description: description},
	}
}
