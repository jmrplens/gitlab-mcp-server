package enterpriseusers

import (
	"context"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const (
	actionEnterpriseUserDelete     = "enterprise_user.delete"
	actionEnterpriseUserDisable2FA = "enterprise_user.disable_2fa"
	actionEnterpriseUserGet        = "enterprise_user.get"
	actionEnterpriseUserList       = "enterprise_user.list"
)

// ActionSpecs returns canonical specs for enterprise user actions.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		enterpriseUserReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_enterprise_users"),
		enterpriseUserReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_enterprise_user"),
		enterpriseUserDisable2FASpec(client),
		enterpriseUserDestructiveSpec("delete", toolutil.DestructiveAction(client, DeleteOutput), "gitlab_delete_enterprise_user"),
	}
}

// Disable2FAOutput disables two-factor authentication and returns the canonical success message shape.
func Disable2FAOutput(ctx context.Context, client *gitlabclient.Client, input Disable2FAInput) (toolutil.VoidOutput, error) {
	if err := Disable2FA(ctx, client, input); err != nil {
		return toolutil.VoidOutput{}, err
	}
	return toolutil.VoidOutput{Status: "success", Message: fmt.Sprintf("Disabled 2FA for enterprise user %d in group %s.", input.UserID, input.GroupID)}, nil
}

// DeleteOutput deletes an enterprise user and returns the canonical success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: fmt.Sprintf("Successfully deleted enterprise user %d from group %s.", input.UserID, input.GroupID)}, nil
}

func enterpriseUserReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := enterpriseUserOptions(individualTool)
	switch individualTool {
	case "gitlab_list_enterprise_users":
		options.Usage = "List the enterprise users managed by a top-level group's enterprise namespace. Use filters such as username, search, active, blocked, two_factor, created_after/created_before, plus order_by/sort and offset or keyset pagination."
		options.Aliases = []string{individualTool, "list enterprise users", "show enterprise users", "browse enterprise users"}
		options.RelatedActions = []string{actionEnterpriseUserGet, actionEnterpriseUserDisable2FA, actionEnterpriseUserDelete}
		options.IndividualTool.Description = "List enterprise users for a top-level group with filtering and pagination. Returns: enterprise users with full profile fields (identities, SCIM identities, custom attributes, sign-in metadata, license seat usage) and pagination metadata. See also: gitlab_get_enterprise_user, gitlab_disable_2fa_enterprise_user, gitlab_delete_enterprise_user."
	case "gitlab_get_enterprise_user":
		options.Usage = "Get one enterprise user by group_id plus user_id. Use after a list result or when the prompt already names a concrete enterprise user."
		options.Aliases = []string{individualTool, "get enterprise user", "show enterprise user details", "fetch enterprise user"}
		options.RelatedActions = []string{actionEnterpriseUserList, actionEnterpriseUserDisable2FA, actionEnterpriseUserDelete}
		options.IndividualTool.Description = "Get a single enterprise user by group_id and user_id. Returns: the full user profile (identities, SCIM identities, custom attributes, sign-in metadata, admin/auditor flags, license seat usage, and web URL). See also: gitlab_list_enterprise_users, gitlab_disable_2fa_enterprise_user, gitlab_delete_enterprise_user."
	}
	return toolutil.NewReadActionSpec(name, route, options)
}

func enterpriseUserDestructiveSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := enterpriseUserOptions(individualTool)
	options.Usage = "Delete an enterprise user from the group's enterprise namespace. Use hard_delete to permanently remove the user instead of soft-deleting. This action is irreversible."
	options.Aliases = []string{individualTool, "delete enterprise user", "remove enterprise user", "destroy enterprise user", "drop enterprise user"}
	options.RelatedActions = []string{actionEnterpriseUserGet, actionEnterpriseUserList, actionEnterpriseUserDisable2FA}
	options.IndividualTool.Description = "Delete an enterprise user, optionally with hard_delete. Returns: a success confirmation naming the user and group. See also: gitlab_get_enterprise_user, gitlab_list_enterprise_users, gitlab_disable_2fa_enterprise_user."
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func enterpriseUserDisable2FASpec(client *gitlabclient.Client) toolutil.ActionSpec {
	individualDestructive := false
	options := enterpriseUserOptions("gitlab_disable_2fa_enterprise_user")
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	options.Usage = "Disable two-factor authentication for an enterprise user by group_id plus user_id. Use when a user is locked out of 2FA and the group administrator must reset it."
	options.Aliases = []string{"gitlab_disable_2fa_enterprise_user", "disable 2fa enterprise user", "reset enterprise user two-factor", "reset enterprise user 2fa", "unlock enterprise user"}
	options.RelatedActions = []string{actionEnterpriseUserGet, actionEnterpriseUserList, actionEnterpriseUserDelete}
	options.IndividualTool.Description = "Disable two-factor authentication for an enterprise user. Returns: a success confirmation naming the user and group. See also: gitlab_get_enterprise_user, gitlab_list_enterprise_users, gitlab_delete_enterprise_user."
	return toolutil.NewDeleteActionSpec("disable_2fa", toolutil.DestructiveAction(client, Disable2FAOutput), options)
}

func enterpriseUserOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute enterpriseusers domain action.", Tags: []string{"enterprise_user"},
		OpenWorld:      true,
		Edition:        "premium",
		OwnerPackage:   "enterpriseusers",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
