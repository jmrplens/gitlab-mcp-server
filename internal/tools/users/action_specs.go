package users

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for user account, status, SSH key, and misc actions.
func ActionSpecs(client *gitlabclient.Client, enterprise bool) []toolutil.ActionSpec {
	specs := []toolutil.ActionSpec{
		userReadSpec("current", toolutil.RouteAction(client, Current), "gitlab_user_current"),
		userReadSpec("me", toolutil.RouteAction(client, Current), "gitlab_user_current"),
		userReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_users"),
		userReadSpec("get", userGetRoute(client), "gitlab_get_user"),
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
		specs = append(
			specs,
			userEnterpriseCreateSpec("create_service_account", toolutil.RouteAction(client, CreateServiceAccount), "gitlab_create_service_account"),
			userEnterpriseReadSpec("list_service_accounts", toolutil.RouteAction(client, ListServiceAccounts), "gitlab_list_service_accounts"),
			userEnterpriseUpdateSpec("update_service_account", toolutil.RouteAction(client, UpdateInstanceServiceAccount), "gitlab_update_instance_service_account"),
		)
	}
	return specs
}

func userGetRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	route := toolutil.RouteAction(client, Get)
	baseHandler := route.Handler
	route.Handler = func(ctx context.Context, input map[string]any) (any, error) {
		result, err := baseHandler(ctx, input)
		if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return userNotFoundOutput{Identifier: fmt.Sprintf("ID %v", input["user_id"])}, nil
		}
		return result, err
	}
	return route
}

func userReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, userOptionsForAction(name, individualTool))
}

func userCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, userOptionsForAction(name, individualTool))
}

func userUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, userOptionsForAction(name, individualTool))
}

func userDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, userOptionsForAction(name, individualTool))
}

func userDestructiveUpdateIndividualSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	individualDestructive := false
	options := userOptionsForAction(name, individualTool)
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewDeleteActionSpec(name, route, options)
}

func userEnterpriseReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userOptionsForAction(name, individualTool)
	options.Edition = "premium"
	return toolutil.NewReadActionSpec(name, route, options)
}

func userEnterpriseCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userOptionsForAction(name, individualTool)
	options.Edition = "premium"
	return toolutil.NewCreateActionSpec(name, route, options)
}

func userEnterpriseUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userOptionsForAction(name, individualTool)
	options.Edition = "premium"
	return toolutil.NewUpdateActionSpec(name, route, options)
}

func userOptionsForAction(actionName, individualTool string) toolutil.ActionSpecOptions {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{individualTool}, Usage: "Use to execute users domain action.", Tags: []string{"user"},
		OpenWorld:      true,
		OwnerPackage:   "users",
		IndividualTool: toolutil.IndividualToolSpec{Name: individualTool, Title: toolutil.TitleFromName(individualTool)},
	}

	switch individualTool {
	case "gitlab_user_current":
		options.Usage = "Get the authenticated user profile for the current token. Use this when the prompt asks who the caller is, what permissions they have, or what user identity is currently active."
		if actionName == "current" {
			options.Aliases = []string{"who am i", "current user", "show my profile"}
		}
		options.RelatedActions = []string{"user.list", "user.current_user_status", "user.emails"}
		options.IndividualTool.Description = "Get the current authenticated user. Returns: account ID, username, name, state, avatar URL, and profile metadata. See also: gitlab_list_users, gitlab_current_user_status, gitlab_list_emails."
	case "gitlab_list_users":
		options.Usage = "List users visible to the authenticated caller. Use filters like search, username, active, blocked, and pagination when the task asks for matching users or account inventories."
		options.Aliases = []string{"list users", "find users", "search users"}
		options.RelatedActions = []string{"user.get", "user.current", "user.create"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"search": {
				ValueSource:      "Name, username, or email fragment from the user's request.",
				ExampleBinding:   `params.search:"alice"`,
				CommonConfusions: []string{"search narrows users globally; it is not a project/group membership filter."},
			},
		}
		options.IndividualTool.Description = "List users with filtering and pagination support. Returns: user summaries including ID, username, name, state, and profile URLs. See also: gitlab_get_user, gitlab_user_current, gitlab_create_user."
	case "gitlab_get_user":
		options.Usage = "Get a single user by numeric user_id. Use this when the prompt already references a concrete user account and needs detailed profile fields."
		options.Aliases = []string{"get user by id", "show user details", "lookup user"}
		options.RelatedActions = []string{"user.list", "user.modify", "user.delete"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"user_id": {
				SemanticRole:     "scope_user",
				ValueSource:      "Numeric GitLab user ID from prior list/get output or explicit task input.",
				ExampleBinding:   "params.user_id:42",
				CommonConfusions: []string{"Use numeric user_id; do not pass username where an ID is required."},
			},
		}
		options.IndividualTool.Description = "Get one user by ID. Returns: detailed account profile metadata and status fields. See also: gitlab_list_users, gitlab_modify_user, gitlab_delete_user."
	case "gitlab_create_user":
		options.Usage = "Create a new user account with required fields email, name, and username. Add optional admin/external flags only when explicitly requested."
		options.Aliases = []string{"create user", "provision user", "new user account"}
		options.RelatedActions = []string{"user.get", "user.modify", "user.block"}
		options.ParameterGuidance = map[string]toolutil.ParameterGuidance{
			"email": {
				SemanticRole:     "email_address",
				ValueSource:      "New account email address from task requirements.",
				ExampleBinding:   `params.email:"new.user@example.com"`,
				CommonConfusions: []string{"Provide a real email address; do not pass usernames in the email field."},
			},
			"username": {
				SemanticRole:     "username",
				ValueSource:      "GitLab username slug for the new account.",
				ExampleBinding:   `params.username:"newuser"`,
				CommonConfusions: []string{"Use username without spaces; it is different from display name."},
			},
		}
		options.IndividualTool.Description = "Create a user account. Returns: created user identity and profile summary fields. See also: gitlab_get_user, gitlab_modify_user, gitlab_block_user."
	case "gitlab_current_user_status":
		options.RelatedActions = []string{"user.current", "user.set_status", "user.get_status"}
	case "gitlab_set_user_status":
		options.RelatedActions = []string{"user.current_user_status", "user.get_status"}
	case "gitlab_update_instance_service_account":
		options.Usage = "Update an instance-level service account. Allows updating name, username, and email. Returns: updated service account with id, username, name, email, and unconfirmed_email. Requires admin token and GitLab Premium/Ultimate."
		options.Aliases = []string{"update instance service account", "modify service account", individualTool}
		options.RelatedActions = []string{"user.create_service_account", "user.list_service_accounts"}
		options.IndividualTool.Description = "Update an instance-level service account. Returns: updated service account object including email and unconfirmed_email. Requires admin token. See also: gitlab_create_service_account, gitlab_list_service_accounts."
	}

	return options
}
