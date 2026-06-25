package users

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for user account, status, SSH key, and
// misc actions, including the Free-tier instance service-account specs. The
// bool parameter is retained for signature compatibility but ignored: tier
// gating is handled centrally by the catalog tier filter via each spec's
// Edition.
func ActionSpecs(client *gitlabclient.Client, _ bool) []toolutil.ActionSpec {
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
		// gitlab_upload_user_avatar — set the current authenticated user's avatar image (non-destructive replace).
		userUpdateSpec("upload_avatar", toolutil.RouteAction(client, UploadCurrentUserAvatar), "gitlab_upload_user_avatar"),
		// Instance-level service accounts are Free tier (admin-only) per
		// doc/api/service_accounts.md (page tier = Free, Premium, Ultimate; the
		// Instance section adds an admin requirement, not a licensing tier).
		userCreateSpec("create_service_account", toolutil.RouteAction(client, CreateServiceAccount), "gitlab_create_service_account"),
		userReadSpec("list_service_accounts", toolutil.RouteAction(client, ListServiceAccounts), "gitlab_list_service_accounts"),
		userUpdateSpec("update_service_account", toolutil.RouteAction(client, UpdateInstanceServiceAccount), "gitlab_update_instance_service_account"),
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

// userToolMeta carries the discovery metadata (R-META) for a user individual
// tool whose only customization is purpose-specific text. Tools needing richer
// metadata (parameter guidance, conditional aliases) stay in the switch.
type userToolMeta struct {
	usage          string
	aliases        []string
	relatedActions []string
	description    string
}

// userToolMetadata maps individual tool names to their discovery metadata.
// Every entry supplies a purpose-specific Usage, natural-language aliases,
// RelatedActions cross-links, and a "Returns: … See also: …" description so the
// metadata-completeness audit (R-META) reports no gaps for the users package.
var userToolMetadata = map[string]userToolMeta{
	"gitlab_get_user_status": {
		usage:          "Get a user's status (emoji, message, availability, clear-at time) by numeric user_id. Use when the prompt asks whether someone is busy or what their status message says.",
		aliases:        []string{"get user status", "show user availability", "user status"},
		relatedActions: []string{"user.get", "user.current_user_status", "user.set_status"},
		description:    "Get a user's status by ID. Returns: emoji, message, availability, and clear-status time. See also: gitlab_current_user_status, gitlab_set_user_status, gitlab_get_user.",
	},
	"gitlab_set_user_status": {
		usage:          "Set the authenticated user's status with emoji, message, availability, and an optional auto-clear duration. Use when the prompt asks to update your own status.",
		aliases:        []string{"set my status", "update user status", "change status"},
		relatedActions: []string{"user.current_user_status", "user.get_status"},
		description:    "Set the current user's status. Returns: the updated emoji, message, availability, and clear-status time. See also: gitlab_current_user_status, gitlab_get_user_status.",
	},
	"gitlab_current_user_status": {
		usage:          "Get the authenticated user's own status (emoji, message, availability). Use when the prompt asks about the caller's current status.",
		aliases:        []string{"my status", "current user status", "show my status"},
		relatedActions: []string{"user.current", "user.set_status", "user.get_status"},
		description:    "Get the current user's status. Returns: emoji, message, availability, and clear-status time. See also: gitlab_set_user_status, gitlab_get_user_status, gitlab_user_current.",
	},
	"gitlab_list_ssh_keys": {
		usage:          "List the authenticated user's SSH keys with pagination. Use when the prompt asks which SSH keys are on the current account.",
		aliases:        []string{"list my ssh keys", "list ssh keys", "show ssh keys"},
		relatedActions: []string{"user.get_ssh_key", "user.add_ssh_key", "user.delete_ssh_key"},
		description:    "List the current user's SSH keys. Returns: key summaries with ID, title, fingerprint, usage type, and expiry. See also: gitlab_get_ssh_key, gitlab_add_ssh_key, gitlab_delete_ssh_key.",
	},
	"gitlab_get_ssh_key": {
		usage:          "Get one of the authenticated user's SSH keys by key_id. Use when the prompt references a specific key and needs its details.",
		aliases:        []string{"get ssh key", "show ssh key", "lookup ssh key"},
		relatedActions: []string{"user.ssh_keys", "user.add_ssh_key", "user.delete_ssh_key"},
		description:    "Get one of the current user's SSH keys by ID. Returns: key ID, title, public key, usage type, and expiry. See also: gitlab_list_ssh_keys, gitlab_add_ssh_key, gitlab_delete_ssh_key.",
	},
	"gitlab_add_ssh_key": {
		usage:          "Add an SSH public key to the authenticated user's account with a title and optional expiry and usage type. Use when the prompt asks to register a new key.",
		aliases:        []string{"add ssh key", "create ssh key", "register ssh key"},
		relatedActions: []string{"user.ssh_keys", "user.get_ssh_key", "user.delete_ssh_key"},
		description:    "Add an SSH key to the current user. Returns: created key ID, title, usage type, and expiry. See also: gitlab_list_ssh_keys, gitlab_get_ssh_key, gitlab_delete_ssh_key.",
	},
	"gitlab_delete_ssh_key": {
		usage:          "Delete one of the authenticated user's SSH keys by key_id. Use when the prompt asks to remove a key from the current account.",
		aliases:        []string{"delete ssh key", "remove ssh key", "revoke ssh key"},
		relatedActions: []string{"user.ssh_keys", "user.get_ssh_key", "user.add_ssh_key"},
		description:    "Delete one of the current user's SSH keys. Returns: confirmation with the deleted key ID. See also: gitlab_list_ssh_keys, gitlab_get_ssh_key, gitlab_add_ssh_key.",
	},
	"gitlab_list_ssh_keys_for_user": {
		usage:          "List SSH keys belonging to a specific user by user_id with pagination. Use when the prompt asks which keys another user has registered.",
		aliases:        []string{"list ssh keys for user", "user ssh keys", "show another user's keys"},
		relatedActions: []string{"user.get_ssh_key_for_user", "user.add_ssh_key_for_user", "user.delete_ssh_key_for_user"},
		description:    "List a specific user's SSH keys. Returns: key summaries with ID, title, fingerprint, usage type, and expiry. See also: gitlab_get_ssh_key_for_user, gitlab_add_ssh_key_for_user, gitlab_delete_ssh_key_for_user.",
	},
	"gitlab_get_ssh_key_for_user": {
		usage:          "Get a specific SSH key for a specific user by user_id and key_id. Use when the prompt references one of another user's keys.",
		aliases:        []string{"get ssh key for user", "show user's ssh key", "lookup user ssh key"},
		relatedActions: []string{"user.ssh_keys_for_user", "user.add_ssh_key_for_user", "user.delete_ssh_key_for_user"},
		description:    "Get a specific user's SSH key by ID. Returns: key ID, title, public key, usage type, and expiry. See also: gitlab_list_ssh_keys_for_user, gitlab_add_ssh_key_for_user, gitlab_delete_ssh_key_for_user.",
	},
	"gitlab_add_ssh_key_for_user": {
		usage:          "Add an SSH public key to a specific user's account (admin only) with title and optional expiry. Use when the prompt asks to register a key on behalf of another user.",
		aliases:        []string{"add ssh key for user", "create ssh key for user", "register user ssh key"},
		relatedActions: []string{"user.ssh_keys_for_user", "user.get_ssh_key_for_user", "user.delete_ssh_key_for_user"},
		description:    "Add an SSH key to a specific user. Returns: created key ID, title, usage type, and expiry. Requires admin token. See also: gitlab_list_ssh_keys_for_user, gitlab_get_ssh_key_for_user, gitlab_delete_ssh_key_for_user.",
	},
	"gitlab_delete_ssh_key_for_user": {
		usage:          "Delete a specific SSH key from a specific user's account (admin only) by user_id and key_id. Use when the prompt asks to remove another user's key.",
		aliases:        []string{"delete ssh key for user", "remove user ssh key", "revoke user ssh key"},
		relatedActions: []string{"user.ssh_keys_for_user", "user.get_ssh_key_for_user", "user.add_ssh_key_for_user"},
		description:    "Delete a specific user's SSH key. Returns: confirmation with the deleted key ID. Requires admin token. See also: gitlab_list_ssh_keys_for_user, gitlab_get_ssh_key_for_user, gitlab_add_ssh_key_for_user.",
	},
	"gitlab_list_emails": {
		usage:          "List the authenticated user's registered email addresses. Use when the prompt asks which emails are on the current account.",
		aliases:        []string{"list emails", "list my emails", "show email addresses"},
		relatedActions: []string{"user.current", "user.modify"},
		description:    "List the current user's email addresses. Returns: email entries with ID, address, and confirmation time. See also: gitlab_user_current, gitlab_modify_user.",
	},
	"gitlab_list_user_contribution_events": {
		usage:          "List a user's recent contribution events (pushes, comments, merges) filtered by action, target type, date range, and scope. Use when the prompt asks what a user has done recently.",
		aliases:        []string{"list contribution events", "user activity feed", "recent contributions"},
		relatedActions: []string{"user.get", "user.activities", "user.associations_count"},
		description:    "List a user's contribution events. Returns: event entries with action, target type/title, project, and timestamp. See also: gitlab_get_user, gitlab_get_user_activities, gitlab_get_user_associations_count.",
	},
	"gitlab_get_user_associations_count": {
		usage:          "Get a user's association counts (groups, projects, issues, merge requests) by user_id. Use when the prompt asks how much a user owns or is involved in before deletion.",
		aliases:        []string{"user associations count", "count user associations", "user ownership counts"},
		relatedActions: []string{"user.get", "user.memberships", "user.delete"},
		description:    "Get a user's association counts. Returns: groups, projects, issues, and merge-requests counts. See also: gitlab_get_user, gitlab_get_user_memberships, gitlab_delete_user.",
	},
	"gitlab_get_user_activities": {
		usage:          "List recent user activities (last_activity_on per username) optionally filtered by a from-date (admin only). Use when the prompt asks who has been active recently.",
		aliases:        []string{"list user activities", "user activity report", "last activity dates"},
		relatedActions: []string{"user.list", "user.contribution_events", "user.memberships"},
		description:    "List user activities (admin). Returns: username and last-activity date per user. See also: gitlab_list_users, gitlab_list_user_contribution_events, gitlab_get_user_memberships.",
	},
	"gitlab_get_user_memberships": {
		usage:          "List a user's project and group memberships by user_id, optionally filtered by type (Project or Namespace). Use when the prompt asks where a user is a member and at what access level.",
		aliases:        []string{"list user memberships", "user memberships", "where is user a member"},
		relatedActions: []string{"user.get", "user.associations_count", "user.activities"},
		description:    "List a user's memberships. Returns: source ID, name, type, and access level per membership. See also: gitlab_get_user, gitlab_get_user_associations_count, gitlab_get_user_activities.",
	},
	"gitlab_create_user_runner": {
		usage:          "Register a CI runner scoped to the current user as instance, group, or project type. Use when the prompt asks to create a runner and provide an authentication token.",
		aliases:        []string{"create user runner", "register user runner", "new user ci runner"},
		relatedActions: []string{"user.current", "user.create_current_user_pat"},
		description:    "Create a runner linked to the current user. Returns: runner ID, authentication token, and token expiry. See also: gitlab_user_current, gitlab_create_current_user_pat.",
	},
	"gitlab_create_current_user_pat": {
		usage:          "Create a personal access token for the authenticated user with a name, scopes, and optional expiry. Use when the prompt asks to mint a new token for the current account.",
		aliases:        []string{"create personal access token", "create pat", "new access token"},
		relatedActions: []string{"user.current", "user.create_runner"},
		description:    "Create a personal access token for the current user. Returns: token ID, the secret token, scopes, and expiry. See also: gitlab_user_current, gitlab_create_user_runner.",
	},
	"gitlab_upload_user_avatar": {
		usage:          "Set or replace the current authenticated user's avatar image. Provide a filename plus exactly one of file_path (a local image on the MCP server) or content_base64 (base64-encoded JPG/PNG/GIF under 200 KB). Targets the token's own user; there is no user_id parameter.",
		aliases:        []string{"upload my avatar", "set current user avatar", "change my profile picture", "update user avatar"},
		relatedActions: []string{"user.current", "user.modify", "user.current_user_status"},
		description:    "Upload the current user's avatar. Returns: the updated user profile including the new avatar URL. Provide filename and exactly one of file_path or content_base64. See also: gitlab_user_current, gitlab_modify_user.",
	},
	"gitlab_delete_user_identity": {
		usage:          "Delete a user's external authentication identity by user_id and provider name (admin only). Use when the prompt asks to unlink an SSO/LDAP identity from a user.",
		aliases:        []string{"delete user identity", "remove identity provider", "unlink user identity"},
		relatedActions: []string{"user.get", "user.modify"},
		description:    "Delete a user's external identity. Returns: confirmation with user ID and provider. Requires admin token. See also: gitlab_get_user, gitlab_modify_user.",
	},
	"gitlab_modify_user": {
		usage:          "Update an existing user's profile fields (email, name, flags, social links) by user_id (admin only). Use when the prompt asks to change a user's account details.",
		aliases:        []string{"modify user", "update user", "edit user account"},
		relatedActions: []string{"user.get", "user.create", "user.delete"},
		description:    "Modify an existing user. Returns: the updated user profile metadata. See also: gitlab_get_user, gitlab_create_user, gitlab_delete_user.",
	},
	"gitlab_delete_user": {
		usage:          "Delete a user account by user_id (admin only); supports hard delete to remove all contributions. Use only when the prompt explicitly asks to remove a user.",
		aliases:        []string{"delete user", "remove user account", "destroy user"},
		relatedActions: []string{"user.get", "user.block", "user.associations_count"},
		description:    "Delete a user account. Returns: confirmation with the deleted user ID. Requires admin token. See also: gitlab_get_user, gitlab_block_user, gitlab_get_user_associations_count.",
	},
	"gitlab_block_user": {
		usage:          "Block a user from signing in by user_id (admin only); reversible via unblock. Use when the prompt asks to suspend a user's access.",
		aliases:        []string{"block user", "suspend user", "disable user sign-in"},
		relatedActions: []string{"user.unblock", "user.ban", "user.get"},
		description:    "Block a user from signing in. Returns: confirmation with the user ID and action. Reversible via gitlab_unblock_user. See also: gitlab_unblock_user, gitlab_ban_user, gitlab_get_user.",
	},
	"gitlab_unblock_user": {
		usage:          "Unblock a previously blocked user by user_id (admin only). Use when the prompt asks to restore a blocked user's access.",
		aliases:        []string{"unblock user", "restore user access", "re-enable user"},
		relatedActions: []string{"user.block", "user.activate", "user.get"},
		description:    "Unblock a previously blocked user. Returns: confirmation with the user ID and action. See also: gitlab_block_user, gitlab_activate_user, gitlab_get_user.",
	},
	"gitlab_ban_user": {
		usage:          "Ban a user by user_id (admin only); reversible via unban. Use when the prompt asks to ban a user, hiding their content.",
		aliases:        []string{"ban user", "block and hide user", "ban account"},
		relatedActions: []string{"user.unban", "user.block", "user.get"},
		description:    "Ban a user. Returns: confirmation with the user ID and action. Reversible via gitlab_unban_user. See also: gitlab_unban_user, gitlab_block_user, gitlab_get_user.",
	},
	"gitlab_unban_user": {
		usage:          "Unban a previously banned user by user_id (admin only). Use when the prompt asks to lift a ban.",
		aliases:        []string{"unban user", "lift user ban", "restore banned user"},
		relatedActions: []string{"user.ban", "user.unblock", "user.get"},
		description:    "Unban a previously banned user. Returns: confirmation with the user ID and action. See also: gitlab_ban_user, gitlab_unblock_user, gitlab_get_user.",
	},
	"gitlab_activate_user": {
		usage:          "Activate a deactivated user account by user_id (admin only). Use when the prompt asks to reactivate a dormant user.",
		aliases:        []string{"activate user", "reactivate user", "enable user account"},
		relatedActions: []string{"user.deactivate", "user.unblock", "user.get"},
		description:    "Activate a deactivated user. Returns: confirmation with the user ID and action. See also: gitlab_deactivate_user, gitlab_unblock_user, gitlab_get_user.",
	},
	"gitlab_deactivate_user": {
		usage:          "Deactivate an inactive user account by user_id (admin only); reversible via activate. Use when the prompt asks to deactivate a dormant user.",
		aliases:        []string{"deactivate user", "make user dormant", "disable inactive user"},
		relatedActions: []string{"user.activate", "user.block", "user.get"},
		description:    "Deactivate an active user. Returns: confirmation with the user ID and action. Reversible via gitlab_activate_user. See also: gitlab_activate_user, gitlab_block_user, gitlab_get_user.",
	},
	"gitlab_approve_user": {
		usage:          "Approve a pending user sign-up by user_id (admin only). Use when the prompt asks to approve a user awaiting admin approval.",
		aliases:        []string{"approve user", "approve pending signup", "accept user registration"},
		relatedActions: []string{"user.reject", "user.get", "user.list"},
		description:    "Approve a pending user sign-up. Returns: confirmation with the user ID and action. See also: gitlab_reject_user, gitlab_get_user, gitlab_list_users.",
	},
	"gitlab_reject_user": {
		usage:          "Reject a pending user sign-up by user_id (admin only); this permanently deletes the pending user. Use only when the prompt asks to reject a registration.",
		aliases:        []string{"reject user", "deny pending signup", "decline user registration"},
		relatedActions: []string{"user.approve", "user.get", "user.list"},
		description:    "Reject a pending user sign-up. Returns: confirmation with the user ID and action. Permanently deletes the pending user. See also: gitlab_approve_user, gitlab_get_user, gitlab_list_users.",
	},
	"gitlab_disable_two_factor": {
		usage:          "Disable two-factor authentication for a user by user_id (admin only). Use when the prompt asks to clear a user's locked-out 2FA.",
		aliases:        []string{"disable two factor", "disable 2fa", "reset user 2fa"},
		relatedActions: []string{"user.get", "user.modify"},
		description:    "Disable 2FA for a user. Returns: confirmation with the user ID and action. See also: gitlab_get_user, gitlab_modify_user.",
	},
	"gitlab_create_service_account": {
		usage:          "Create an instance-level service account with optional name, username, and email. Admin-only. Use when the prompt asks to provision a service account.",
		aliases:        []string{"create service account", "new service account", "provision service account"},
		relatedActions: []string{"user.list_service_accounts", "user.update_service_account"},
		description:    "Create an instance-level service account. Returns: the created service account with ID, username, name, and email. Requires admin token. See also: gitlab_list_service_accounts, gitlab_update_instance_service_account.",
	},
	"gitlab_list_service_accounts": {
		usage:          "List instance-level service accounts with order_by/sort and pagination. Admin-only. Use when the prompt asks which service accounts exist.",
		aliases:        []string{"list service accounts", "show service accounts", "service account inventory"},
		relatedActions: []string{"user.create_service_account", "user.update_service_account"},
		description:    "List instance-level service accounts. Returns: service account summaries with ID, username, name, and email. Requires admin token. See also: gitlab_create_service_account, gitlab_update_instance_service_account.",
	},
	"gitlab_update_instance_service_account": {
		usage:          "Update an instance-level service account. Allows updating name, username, and email. Returns: updated service account with id, username, name, email, and unconfirmed_email. Admin-only.",
		aliases:        []string{"update instance service account", "modify service account"},
		relatedActions: []string{"user.create_service_account", "user.list_service_accounts"},
		description:    "Update an instance-level service account. Returns: updated service account object including email and unconfirmed_email. Requires admin token. See also: gitlab_create_service_account, gitlab_list_service_accounts.",
	},
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
		if actionName == "me" {
			options.Aliases = []string{"whoami", "my account", "current user identity"}
		} else {
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
	default:
		if meta, ok := userToolMetadata[individualTool]; ok {
			options.Usage = meta.usage
			options.Aliases = meta.aliases
			options.RelatedActions = meta.relatedActions
			options.IndividualTool.Description = meta.description
		}
	}

	return options
}
