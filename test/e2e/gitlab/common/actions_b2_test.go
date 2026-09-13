//go:build e2e

// actions_b2_test.go names the catalog actions the user, access, token and
// to-do families drive, once each and as typed constants, for the reason
// actions_test.go gives: the static gate reads them out of the type
// checker's record, and a literal would be invisible to it. The constants
// actions_test.go already declares (user.current, user.delete and the rest)
// are reused rather than repeated.

package common

import "github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"

// The run user's own account: the alias of the current-user read, the
// listing and the read by ID, the status, the counts, the avatar lookup and
// the token the account mints for itself.
const (
	actionUserMe                   harness.ActionID = "user.me"
	actionUserList                 harness.ActionID = "user.list"
	actionUserGet                  harness.ActionID = "user.get"
	actionUserCurrentStatus        harness.ActionID = "user.current_user_status"
	actionUserSetStatus            harness.ActionID = "user.set_status"
	actionUserGetStatus            harness.ActionID = "user.get_status"
	actionUserAssociationsCount    harness.ActionID = "user.associations_count"
	actionUserMemberships          harness.ActionID = "user.memberships"
	actionUserActivities           harness.ActionID = "user.activities"
	actionUserAvatarGet            harness.ActionID = "user.avatar_get"
	actionUserUploadAvatar         harness.ActionID = "user.upload_avatar"
	actionUserCreateCurrentUserPAT harness.ActionID = "user.create_current_user_pat"
)

// The administrator's operations on another account: its whole life, the
// pending sign-ups, the identity, the runner and the two-factor reset.
const (
	actionUserCreate           harness.ActionID = "user.create"
	actionUserModify           harness.ActionID = "user.modify"
	actionUserBlock            harness.ActionID = "user.block"
	actionUserUnblock          harness.ActionID = "user.unblock"
	actionUserDeactivate       harness.ActionID = "user.deactivate"
	actionUserActivate         harness.ActionID = "user.activate"
	actionUserBan              harness.ActionID = "user.ban"
	actionUserUnban            harness.ActionID = "user.unban"
	actionUserApprove          harness.ActionID = "user.approve"
	actionUserReject           harness.ActionID = "user.reject"
	actionUserDeleteIdentity   harness.ActionID = "user.delete_identity"
	actionUserCreateRunner     harness.ActionID = "user.create_runner"
	actionUserDisableTwoFactor harness.ActionID = "user.disable_two_factor"
)

// SSH keys, on the run user's own account and on another user's, and the
// two administrator lookups of a key by its ID and by its fingerprint.
const (
	actionUserAddSSHKey           harness.ActionID = "user.add_ssh_key"
	actionUserGetSSHKey           harness.ActionID = "user.get_ssh_key"
	actionUserDeleteSSHKey        harness.ActionID = "user.delete_ssh_key"
	actionUserSSHKeysForUser      harness.ActionID = "user.ssh_keys_for_user"
	actionUserAddSSHKeyForUser    harness.ActionID = "user.add_ssh_key_for_user"
	actionUserGetSSHKeyForUser    harness.ActionID = "user.get_ssh_key_for_user"
	actionUserDeleteSSHKeyForUser harness.ActionID = "user.delete_ssh_key_for_user"
	actionUserKeyGetWithUser      harness.ActionID = "user.key_get_with_user"
	actionUserKeyGetByFingerprint harness.ActionID = "user.key_get_by_fingerprint"
)

// Email addresses, on the run user's own account and on another user's.
const (
	actionUserEmails             harness.ActionID = "user.emails"
	actionUserAddEmail           harness.ActionID = "user.add_email"
	actionUserGetEmail           harness.ActionID = "user.get_email"
	actionUserDeleteEmail        harness.ActionID = "user.delete_email"
	actionUserEmailsForUser      harness.ActionID = "user.emails_for_user"
	actionUserAddEmailForUser    harness.ActionID = "user.add_email_for_user"
	actionUserDeleteEmailForUser harness.ActionID = "user.delete_email_for_user"
)

// GPG keys, on the run user's own account and on another user's.
const (
	actionUserAddGPGKey           harness.ActionID = "user.add_gpg_key"
	actionUserGetGPGKey           harness.ActionID = "user.get_gpg_key"
	actionUserDeleteGPGKey        harness.ActionID = "user.delete_gpg_key"
	actionUserGPGKeysForUser      harness.ActionID = "user.gpg_keys_for_user"
	actionUserAddGPGKeyForUser    harness.ActionID = "user.add_gpg_key_for_user"
	actionUserGetGPGKeyForUser    harness.ActionID = "user.get_gpg_key_for_user"
	actionUserDeleteGPGKeyForUser harness.ActionID = "user.delete_gpg_key_for_user"
)

// Impersonation tokens of another user, and the personal token an
// administrator mints for one.
const (
	actionUserListImpersonationTokens   harness.ActionID = "user.list_impersonation_tokens"
	actionUserCreateImpersonationToken  harness.ActionID = "user.create_impersonation_token"
	actionUserGetImpersonationToken     harness.ActionID = "user.get_impersonation_token"
	actionUserRevokeImpersonationToken  harness.ActionID = "user.revoke_impersonation_token"
	actionUserCreatePersonalAccessToken harness.ActionID = "user.create_personal_access_token"
)

// Namespaces, which the user tool reads.
const (
	actionUserNamespaceList   harness.ActionID = "user.namespace_list"
	actionUserNamespaceSearch harness.ActionID = "user.namespace_search"
	actionUserNamespaceExists harness.ActionID = "user.namespace_exists"
	actionUserNamespaceGet    harness.ActionID = "user.namespace_get"
)

// Notification settings, global and per project or group.
const (
	actionUserNotificationGlobalGet     harness.ActionID = "user.notification_global_get"
	actionUserNotificationGlobalUpdate  harness.ActionID = "user.notification_global_update"
	actionUserNotificationProjectGet    harness.ActionID = "user.notification_project_get"
	actionUserNotificationProjectUpdate harness.ActionID = "user.notification_project_update"
	actionUserNotificationGroupGet      harness.ActionID = "user.notification_group_get"
	actionUserNotificationGroupUpdate   harness.ActionID = "user.notification_group_update"
)

// To-do items and the event feeds.
const (
	actionUserTodoList               harness.ActionID = "user.todo_list"
	actionUserTodoMarkDone           harness.ActionID = "user.todo_mark_done"
	actionUserTodoMarkAllDone        harness.ActionID = "user.todo_mark_all_done"
	actionUserContributionEvents     harness.ActionID = "user.contribution_events"
	actionUserEventListContributions harness.ActionID = "user.event_list_contributions"
	actionUserEventListProject       harness.ActionID = "user.event_list_project"
)

// The projects a user contributed to and starred, and a project's members.
const (
	actionProjectListUserContributed harness.ActionID = "project.list_user_contributed"
	actionProjectListUserStarred     harness.ActionID = "project.list_user_starred"
	actionProjectMembers             harness.ActionID = "project.members"
	actionProjectMemberGet           harness.ActionID = "project.member_get"
)

// Project, group and personal access tokens, with the self operations a
// token performs on itself.
const (
	actionAccessTokenProjectCreate      harness.ActionID = "access.token_project_create"
	actionAccessTokenProjectGet         harness.ActionID = "access.token_project_get"
	actionAccessTokenProjectList        harness.ActionID = "access.token_project_list"
	actionAccessTokenProjectRotate      harness.ActionID = "access.token_project_rotate"
	actionAccessTokenProjectRotateSelf  harness.ActionID = "access.token_project_rotate_self"
	actionAccessTokenProjectRevoke      harness.ActionID = "access.token_project_revoke"
	actionAccessTokenGroupCreate        harness.ActionID = "access.token_group_create"
	actionAccessTokenGroupGet           harness.ActionID = "access.token_group_get"
	actionAccessTokenGroupList          harness.ActionID = "access.token_group_list"
	actionAccessTokenGroupRotate        harness.ActionID = "access.token_group_rotate"
	actionAccessTokenGroupRotateSelf    harness.ActionID = "access.token_group_rotate_self"
	actionAccessTokenGroupRevoke        harness.ActionID = "access.token_group_revoke"
	actionAccessTokenPersonalGet        harness.ActionID = "access.token_personal_get"
	actionAccessTokenPersonalList       harness.ActionID = "access.token_personal_list"
	actionAccessTokenPersonalRotate     harness.ActionID = "access.token_personal_rotate"
	actionAccessTokenPersonalRevoke     harness.ActionID = "access.token_personal_revoke"
	actionAccessTokenPersonalRotateSelf harness.ActionID = "access.token_personal_rotate_self"
	actionAccessTokenPersonalRevokeSelf harness.ActionID = "access.token_personal_revoke_self"
)

// Access requests, from the requester's side and from the owner's.
const (
	actionAccessRequestProject     harness.ActionID = "access.request_project"
	actionAccessRequestGroup       harness.ActionID = "access.request_group"
	actionAccessRequestListProject harness.ActionID = "access.request_list_project"
	actionAccessRequestListGroup   harness.ActionID = "access.request_list_group"
	actionAccessApproveProject     harness.ActionID = "access.approve_project"
	actionAccessApproveGroup       harness.ActionID = "access.approve_group"
	actionAccessDenyProject        harness.ActionID = "access.deny_project"
	actionAccessDenyGroup          harness.ActionID = "access.deny_group"
)

// Deploy tokens of a project, of a group and of the whole instance.
const (
	actionAccessDeployTokenListAll       harness.ActionID = "access.deploy_token_list_all"
	actionAccessDeployTokenListProject   harness.ActionID = "access.deploy_token_list_project"
	actionAccessDeployTokenCreateProject harness.ActionID = "access.deploy_token_create_project"
	actionAccessDeployTokenGetProject    harness.ActionID = "access.deploy_token_get_project"
	actionAccessDeployTokenDeleteProject harness.ActionID = "access.deploy_token_delete_project"
	actionAccessDeployTokenListGroup     harness.ActionID = "access.deploy_token_list_group"
	actionAccessDeployTokenCreateGroup   harness.ActionID = "access.deploy_token_create_group"
	actionAccessDeployTokenGetGroup      harness.ActionID = "access.deploy_token_get_group"
	actionAccessDeployTokenDeleteGroup   harness.ActionID = "access.deploy_token_delete_group"
)

// Deploy keys shared across projects, listed per instance and per user, and
// the instance-level key an administrator adds.
const (
	actionAccessDeployKeyAdd             harness.ActionID = "access.deploy_key_add"
	actionAccessDeployKeyEnable          harness.ActionID = "access.deploy_key_enable"
	actionAccessDeployKeyDelete          harness.ActionID = "access.deploy_key_delete"
	actionAccessDeployKeyListAll         harness.ActionID = "access.deploy_key_list_all"
	actionAccessDeployKeyListUserProject harness.ActionID = "access.deploy_key_list_user_project"
	actionAccessDeployKeyAddInstance     harness.ActionID = "access.deploy_key_add_instance"
)

// Invitations by email to a project and to a group.
const (
	actionAccessInviteProject     harness.ActionID = "access.invite_project"
	actionAccessInviteListProject harness.ActionID = "access.invite_list_project"
	actionAccessInviteGroup       harness.ActionID = "access.invite_group"
	actionAccessInviteListGroup   harness.ActionID = "access.invite_list_group"
)
