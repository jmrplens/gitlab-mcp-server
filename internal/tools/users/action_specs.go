package users

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for user account, status, SSH key, and misc actions.
// When enterprise is true, instance-level service account specs are appended to the result.
func ActionSpecs(client *gitlabclient.Client, enterprise bool) []toolutil.ActionSpec {
	specs := []toolutil.ActionSpec{
		// gitlab_user_current — return the authenticated user profile for the current token.
		userReadSpec("current", toolutil.RouteAction(client, Current), "gitlab_user_current"),
		// gitlab_user_current (alias for "me") — same as gitlab_user_current, registered under a friendlier name.
		userReadSpec("me", toolutil.RouteAction(client, Current), "gitlab_user_current"),
		// gitlab_list_users — list users visible to the authenticated caller with optional filters.
		userReadSpec("list", toolutil.RouteAction(client, List), "gitlab_list_users"),
		// gitlab_get_user — fetch a single user by numeric ID (returns a structured not-found result on 404).
		userReadSpec("get", userGetRoute(client), "gitlab_get_user"),
		// gitlab_get_user_status — fetch a user's status (emoji, message, availability).
		userReadSpec("get_status", toolutil.RouteAction(client, GetStatus), "gitlab_get_user_status"),
		// gitlab_set_user_status — update the current user's status.
		userUpdateSpec("set_status", toolutil.RouteAction(client, SetStatus), "gitlab_set_user_status"),
		// gitlab_list_ssh_keys — list the current user's SSH keys.
		userReadSpec("ssh_keys", toolutil.RouteAction(client, ListSSHKeys), "gitlab_list_ssh_keys"),
		// gitlab_list_emails — list the current user's email addresses.
		userReadSpec("emails", toolutil.RouteAction(client, ListEmails), "gitlab_list_emails"),
		// gitlab_list_user_contribution_events — list the authenticated user's recent contribution events.
		userReadSpec("contribution_events", toolutil.RouteAction(client, ListContributionEvents), "gitlab_list_user_contribution_events"),
		// gitlab_get_user_associations_count — return the user's count of groups/projects owned.
		userReadSpec("associations_count", toolutil.RouteAction(client, GetAssociationsCount), "gitlab_get_user_associations_count"),
		// gitlab_block_user — block a user from signing in (destructive annotation: reversible via unblock).
		userDestructiveUpdateIndividualSpec("block", toolutil.DestructiveAction(client, BlockUser), "gitlab_block_user"),
		// gitlab_unblock_user — unblock a previously blocked user.
		userUpdateSpec("unblock", toolutil.RouteAction(client, UnblockUser), "gitlab_unblock_user"),
		// gitlab_ban_user — ban a user from a top-level group (destructive annotation: reversible via unban).
		userDestructiveUpdateIndividualSpec("ban", toolutil.DestructiveAction(client, BanUser), "gitlab_ban_user"),
		// gitlab_unban_user — unban a previously banned user.
		userUpdateSpec("unban", toolutil.RouteAction(client, UnbanUser), "gitlab_unban_user"),
		// gitlab_activate_user — activate a deactivated user account (admin-only).
		userUpdateSpec("activate", toolutil.RouteAction(client, ActivateUser), "gitlab_activate_user"),
		// gitlab_deactivate_user — deactivate an active user (destructive annotation: reversible via activate).
		userDestructiveUpdateIndividualSpec("deactivate", toolutil.DestructiveAction(client, DeactivateUser), "gitlab_deactivate_user"),
		// gitlab_approve_user — approve a pending sign-up.
		userUpdateSpec("approve", toolutil.RouteAction(client, ApproveUser), "gitlab_approve_user"),
		// gitlab_reject_user — reject a pending sign-up (destructive).
		userDeleteSpec("reject", toolutil.DestructiveAction(client, RejectUser), "gitlab_reject_user"),
		// gitlab_disable_two_factor — disable 2FA for a user (destructive annotation: reversible via reset).
		userDestructiveUpdateIndividualSpec("disable_two_factor", toolutil.DestructiveAction(client, DisableTwoFactor), "gitlab_disable_two_factor"),
		// gitlab_create_user — provision a new user account (admin-only).
		userCreateSpec("create", toolutil.RouteAction(client, Create), "gitlab_create_user"),
		// gitlab_modify_user — update an existing user's profile fields.
		userUpdateSpec("modify", toolutil.RouteAction(client, Modify), "gitlab_modify_user"),
		// gitlab_delete_user — delete a user account (destructive, admin-only).
		userDeleteSpec("delete", toolutil.DestructiveAction(client, Delete), "gitlab_delete_user"),
		// gitlab_list_ssh_keys_for_user — list SSH keys for a specific user.
		userReadSpec("ssh_keys_for_user", toolutil.RouteAction(client, ListSSHKeysForUser), "gitlab_list_ssh_keys_for_user"),
		// gitlab_get_ssh_key — fetch the current user's SSH key by ID.
		userReadSpec("get_ssh_key", toolutil.RouteAction(client, GetSSHKey), "gitlab_get_ssh_key"),
		// gitlab_get_ssh_key_for_user — fetch an SSH key for a specific user.
		userReadSpec("get_ssh_key_for_user", toolutil.RouteAction(client, GetSSHKeyForUser), "gitlab_get_ssh_key_for_user"),
		// gitlab_add_ssh_key — add an SSH key to the current user's account.
		userCreateSpec("add_ssh_key", toolutil.RouteAction(client, AddSSHKey), "gitlab_add_ssh_key"),
		// gitlab_add_ssh_key_for_user — add an SSH key to a specific user (admin-only).
		userCreateSpec("add_ssh_key_for_user", toolutil.RouteAction(client, AddSSHKeyForUser), "gitlab_add_ssh_key_for_user"),
		// gitlab_delete_ssh_key — delete the current user's SSH key (destructive).
		userDeleteSpec("delete_ssh_key", toolutil.DestructiveAction(client, DeleteSSHKey), "gitlab_delete_ssh_key"),
		// gitlab_delete_ssh_key_for_user — delete an SSH key for a specific user (admin-only).
		userDeleteSpec("delete_ssh_key_for_user", toolutil.DestructiveAction(client, DeleteSSHKeyForUser), "gitlab_delete_ssh_key_for_user"),
		// gitlab_current_user_status — fetch the authenticated user's status.
		userReadSpec("current_user_status", toolutil.RouteAction(client, CurrentUserStatus), "gitlab_current_user_status"),
		// gitlab_get_user_activities — list the authenticated user's recent activities.
		userReadSpec("activities", toolutil.RouteAction(client, GetUserActivities), "gitlab_get_user_activities"),
		// gitlab_get_user_memberships — list the authenticated user's group memberships.
		userReadSpec("memberships", toolutil.RouteAction(client, GetUserMemberships), "gitlab_get_user_memberships"),
		// gitlab_create_user_runner — register a runner scoped to the current user.
		userCreateSpec("create_runner", toolutil.RouteAction(client, CreateUserRunner), "gitlab_create_user_runner"),
		// gitlab_delete_user_identity — delete an external authentication identity (destructive).
		userDeleteSpec("delete_identity", toolutil.DestructiveAction(client, DeleteUserIdentity), "gitlab_delete_user_identity"),
		// gitlab_create_current_user_pat — create a personal access token for the authenticated user.
		userCreateSpec("create_current_user_pat", toolutil.RouteAction(client, CreateCurrentUserPAT), "gitlab_create_current_user_pat"),
	}
	if enterprise {
		specs = append(
			specs,
			// gitlab_create_service_account — create an instance-level service account (Premium/Ultimate).
			userEnterpriseCreateSpec("create_service_account", toolutil.RouteAction(client, CreateServiceAccount), "gitlab_create_service_account"),
			// gitlab_list_service_accounts — list instance-level service accounts (Premium/Ultimate).
			userEnterpriseReadSpec("list_service_accounts", toolutil.RouteAction(client, ListServiceAccounts), "gitlab_list_service_accounts"),
			// gitlab_update_instance_service_account — update an instance-level service account.
			userEnterpriseUpdateSpec("update_service_account", toolutil.RouteAction(client, UpdateInstanceServiceAccount), "gitlab_update_instance_service_account"),
		)
	}
	return specs
}

// userGetRoute wraps the canonical Get route to return a structured not-found output
// when GitLab responds with HTTP 404, mirroring the branches package behavior.
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

// userReadSpec builds the canonical read-only spec for a user tool.
func userReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewReadActionSpec(name, route, userOptionsForAction(name, individualTool))
}

// userCreateSpec builds the canonical create spec for a user tool.
func userCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewCreateActionSpec(name, route, userOptionsForAction(name, individualTool))
}

// userUpdateSpec builds the canonical update spec for a user tool.
func userUpdateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewUpdateActionSpec(name, route, userOptionsForAction(name, individualTool))
}

// userDeleteSpec builds the canonical destructive delete spec for a user tool.
func userDeleteSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	return toolutil.NewDeleteActionSpec(name, route, userOptionsForAction(name, individualTool))
}

// userDestructiveUpdateIndividualSpec builds the catalog destructive spec for actions that
// are reversible (block/ban/deactivate/disable_two_factor). It overrides the individual tool
// annotation so the model does not flag the action as terminal.
func userDestructiveUpdateIndividualSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	individualDestructive := false
	options := userOptionsForAction(name, individualTool)
	options.IndividualTool.AnnotationOverrides.Destructive = &individualDestructive
	return toolutil.NewDeleteActionSpec(name, route, options)
}

// userEnterpriseReadSpec builds the canonical read-only spec with Premium edition gating.
func userEnterpriseReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userOptionsForAction(name, individualTool)
	options.Edition = "premium"
	return toolutil.NewReadActionSpec(name, route, options)
}

// userEnterpriseCreateSpec builds the canonical create spec with Premium edition gating.
func userEnterpriseCreateSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	options := userOptionsForAction(name, individualTool)
	options.Edition = "premium"
	return toolutil.NewCreateActionSpec(name, route, options)
}

// userEnterpriseUpdateSpec builds the canonical update spec with Premium edition gating.
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
