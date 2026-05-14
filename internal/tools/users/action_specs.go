package users

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ActionSpecs returns canonical specs for user account, status, SSH key, and misc actions.
func ActionSpecs(client *gitlabclient.Client, enterprise bool) []toolutil.ActionSpec {
	specs := []toolutil.ActionSpec{
		userReadSpec("current", toolutil.RouteAction(client, Current), "gitlab_user_current"),
		userReadSpec("me", toolutil.RouteAction(client, Current), "gitlab_user_current"),
		userReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_users"),
		userReadSpec("get", toolutil.RouteAction(client, Get), "gitlab_get_user"),
		userReadSpec("get_status", toolutil.RouteAction(client, GetStatus), "gitlab_get_user_status"),
		userUpdateSpec("set_status", toolutil.RouteAction(client, SetStatus), "gitlab_set_user_status"),
		userReadSpec("ssh_keys", toolutil.RouteAction(client, ListSSHKeys), "gitlab_list_ssh_keys"),
		userReadSpec("emails", toolutil.RouteAction(client, ListEmails), "gitlab_list_emails"),
		userReadSpec("contribution_events", toolutil.RouteAction(client, ListContributionEvents), "gitlab_list_user_contribution_events"),
		userReadSpec("associations_count", toolutil.RouteAction(client, GetAssociationsCount), "gitlab_get_user_associations_count"),
		userDestructiveUpdateIndividualSpec("block", toolutil.DestructiveAction(client, BlockUser), "gitlab_block_user"),
		userUpdateSpec("unblock", toolutil.RouteAction(client, UnblockUser), "gitlab_unblock_user"),
		userDestructiveUpdateIndividualSpec("ban", toolutil.DestructiveAction(client, BanUser), "gitlab_ban_user"),
		userUpdateSpec("unban", toolutil.RouteAction(client, UnbanUser), "gitlab_unban_user"),
		userUpdateSpec("activate", toolutil.RouteAction(client, ActivateUser), "gitlab_activate_user"),
		userDestructiveUpdateIndividualSpec("deactivate", toolutil.DestructiveAction(client, DeactivateUser), "gitlab_deactivate_user"),
		userUpdateSpec("approve", toolutil.RouteAction(client, ApproveUser), "gitlab_approve_user"),
		userDeleteSpec("reject", toolutil.DestructiveAction(client, RejectUser), "gitlab_reject_user"),
		userDestructiveUpdateIndividualSpec("disable_two_factor", toolutil.DestructiveAction(client, DisableTwoFactor), "gitlab_disable_two_factor"),
		userCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_create_user"),
		userUpdateSpec("modify", toolutil.RouteAction(client, Modify), "gitlab_modify_user"),
		userDeleteSpec("delete", toolutil.DestructiveAction(client, Delete), "gitlab_delete_user"),
		userReadSpec("ssh_keys_for_user", toolutil.RouteAction(client, ListSSHKeysForUser), "gitlab_list_ssh_keys_for_user"),
		userReadSpec("get_ssh_key", toolutil.RouteAction(client, GetSSHKey), "gitlab_get_ssh_key"),
		userReadSpec("get_ssh_key_for_user", toolutil.RouteAction(client, GetSSHKeyForUser), "gitlab_get_ssh_key_for_user"),
		userCreateSpec("add_ssh_key", toolutil.RouteAction(client, AddSSHKey), "gitlab_add_ssh_key"),
		userCreateSpec("add_ssh_key_for_user", toolutil.RouteAction(client, AddSSHKeyForUser), "gitlab_add_ssh_key_for_user"),
		userDeleteSpec("delete_ssh_key", toolutil.DestructiveAction(client, DeleteSSHKey), "gitlab_delete_ssh_key"),
		userDeleteSpec("delete_ssh_key_for_user", toolutil.DestructiveAction(client, DeleteSSHKeyForUser), "gitlab_delete_ssh_key_for_user"),
		userReadSpec("current_user_status", toolutil.RouteAction(client, CurrentUserStatus), "gitlab_current_user_status"),
		userReadSpec("activities", toolutil.RouteAction(client, GetUserActivities), "gitlab_get_user_activities"),
		userReadSpec("memberships", toolutil.RouteAction(client, GetUserMemberships), "gitlab_get_user_memberships"),
		userCreateSpec("create_runner", toolutil.RouteAction(client, CreateUserRunner), "gitlab_create_user_runner"),
		userDeleteSpec("delete_identity", toolutil.DestructiveAction(client, DeleteUserIdentity), "gitlab_delete_user_identity"),
		userCreateSpec("create_current_user_pat", toolutil.RouteAction(client, CreateCurrentUserPAT), "gitlab_create_current_user_pat"),
	}
	if enterprise {
		specs = append(specs,
			userEnterpriseCreateSpec("create_service_account", toolutil.RouteAction(client, CreateServiceAccount), "gitlab_create_service_account"),
			userEnterpriseReadSpec("list_service_accounts", toolutil.RouteAction(client, ListServiceAccounts), "gitlab_list_service_accounts"),
		)
	}
	return specs
}

func userReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func userCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewActionSpec(name, route, userOptions(individualTool))
}

func userUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userOptions(individualTool)
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func userDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	return toolutil.NewActionSpec(name, route, options)
}

func userDestructiveUpdateIndividualSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	individualDestructive := false
	options := userOptions(individualTool)
	options.Destructive = true
	options.Idempotent = true
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewActionSpec(name, route, options)
}

func userEnterpriseReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userOptions(individualTool)
	options.ReadOnly = true
	options.Idempotent = true
	options.Edition = "premium"
	return toolutil.NewActionSpec(name, route, options)
}

func userEnterpriseCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userOptions(individualTool)
	options.Edition = "premium"
	return toolutil.NewActionSpec(name, route, options)
}

func userOptions(individualTool string) toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Tags:           []string{"user"},
		OpenWorld:      true,
		OwnerPackage:   "users",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}
}
