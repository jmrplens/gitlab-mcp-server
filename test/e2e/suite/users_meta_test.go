//go:build e2e

// users_meta_test.go tests GitLab user domain MCP tools via the gitlab_user
// meta-tool against a live GitLab instance. Covers self-info, status, emails,
// events, namespaces, notifications, SSH keys, GPG keys, impersonation tokens,
// admin operations (create/block/deactivate/ban/delete), and service accounts.
package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/avatar"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/events"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/impersonationtokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/keys"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/namespaces"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/notifications"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/todos"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/useremails"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/usergpgkeys"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/users"
)

// TestMeta_UserSelf exercises gitlab_user meta-tool actions that operate on the current user:
// me, current_user_status, set_status, get_status, emails, contribution_events,
// associations_count, memberships, avatar_get.
func TestMeta_UserSelf(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var currentUserID int64

	t.Run("Me", func(t *testing.T) {
		out, err := callToolOn[users.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "me",
			"params": map[string]any{},
		})
		requireNoError(t, err, "user me")
		requireTruef(t, out.ID > 0, "user me: expected user ID > 0")
		currentUserID = out.ID
		t.Logf("Me → user %d (%s)", out.ID, out.Username)
	})

	t.Run("CurrentUserStatus", func(t *testing.T) {
		out, err := callToolOn[users.StatusOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "current_user_status",
			"params": map[string]any{},
		})
		requireNoError(t, err, "current_user_status")
		t.Logf("Current status: emoji=%s message=%s", out.Emoji, out.Message)
	})

	t.Run("SetAndGetStatus", func(t *testing.T) {
		out, err := callToolOn[users.StatusOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "set_status",
			"params": map[string]any{
				"emoji":   "coffee",
				"message": "e2e-testing",
			},
		})
		requireNoError(t, err, "set_status")
		t.Logf("Set status: emoji=%s message=%s", out.Emoji, out.Message)

		// Get status back
		requireTruef(t, currentUserID > 0, "need currentUserID")
		got, err := callToolOn[users.StatusOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "get_status",
			"params": map[string]any{"user_id": currentUserID},
		})
		requireNoError(t, err, "get_status")
		t.Logf("Got status: emoji=%s message=%s", got.Emoji, got.Message)

		// Clear status
		_, _ = callToolOn[users.StatusOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "set_status",
			"params": map[string]any{"emoji": "", "message": ""},
		})
	})

	t.Run("Emails", func(t *testing.T) {
		out, err := callToolOn[users.EmailListOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "emails",
			"params": map[string]any{},
		})
		requireNoError(t, err, "emails")
		t.Logf("Emails: %d found", len(out.Emails))
	})

	t.Run("ContributionEvents", func(t *testing.T) {
		requireTruef(t, currentUserID > 0, "need currentUserID")
		out, err := callToolOn[users.ContributionEventsOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "contribution_events",
			"params": map[string]any{"user_id": currentUserID},
		})
		requireNoError(t, err, "contribution_events")
		t.Logf("Contribution events: %d", len(out.Events))
	})

	t.Run("AssociationsCount", func(t *testing.T) {
		requireTruef(t, currentUserID > 0, "need currentUserID")
		_, err := callToolOn[users.AssociationsCountOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "associations_count",
			"params": map[string]any{"user_id": currentUserID},
		})
		requireNoError(t, err, "associations_count")
	})

	t.Run("Memberships", func(t *testing.T) {
		requireTruef(t, currentUserID > 0, "need currentUserID")
		out, err := callToolOn[users.UserMembershipsOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "memberships",
			"params": map[string]any{"user_id": currentUserID},
		})
		requireNoError(t, err, "memberships")
		t.Logf("Memberships: %d", len(out.Memberships))
	})

	t.Run("Activities", func(t *testing.T) {
		out, err := callToolOn[users.UserActivitiesOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "activities",
			"params": map[string]any{},
		})
		requireNoError(t, err, "activities")
		t.Logf("Activities: %d", len(out.Activities))
	})

	t.Run("AvatarGet", func(t *testing.T) {
		out, err := callToolOn[avatar.GetOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "avatar_get",
			"params": map[string]any{"email": "test@example.com"},
		})
		requireNoError(t, err, "avatar_get")
		t.Logf("Avatar URL: %s", out.AvatarURL)
	})
}

// TestMeta_UserTodosEvents exercises gitlab_user meta-tool todo and event actions:
// todo_list, todo_mark_all_done, event_list_contributions, event_list_project.
func TestMeta_UserTodosEvents(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Run("TodoList", func(t *testing.T) {
		out, err := callToolOn[todos.ListOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "todo_list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "todo_list")
		t.Logf("Todos: %d", len(out.Todos))
	})

	t.Run("TodoMarkAllDone", func(t *testing.T) {
		out, err := callToolOn[todos.MarkAllDoneOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "todo_mark_all_done",
			"params": map[string]any{},
		})
		requireNoError(t, err, "todo_mark_all_done")
		t.Logf("Mark all done: %s", out.Message)
	})

	t.Run("EventListContributions", func(t *testing.T) {
		out, err := callToolOn[events.ListContributionEventsOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "event_list_contributions",
			"params": map[string]any{},
		})
		requireNoError(t, err, "event_list_contributions")
		t.Logf("Contribution events: %d", len(out.Events))
	})

	t.Run("EventListProject", func(t *testing.T) {
		proj := createProjectMeta(ctx, t, sess.meta)
		out, err := callToolOn[events.ListProjectEventsOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "event_list_project",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "event_list_project")
		t.Logf("Project events: %d", len(out.Events))
	})
}

// TestMeta_UserNamespacesNotifications exercises namespace and notification actions.
func TestMeta_UserNamespacesNotifications(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Run("NamespaceList", func(t *testing.T) {
		out, err := callToolOn[namespaces.ListOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "namespace_list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "namespace_list")
		requireTruef(t, len(out.Namespaces) > 0, "expected at least 1 namespace")
		t.Logf("Namespaces: %d", len(out.Namespaces))
	})

	t.Run("NamespaceSearch", func(t *testing.T) {
		username := sess.username
		out, err := callToolOn[namespaces.ListOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "namespace_search",
			"params": map[string]any{"query": username},
		})
		requireNoError(t, err, "namespace_search")
		t.Logf("Namespace search '%s': %d results", username, len(out.Namespaces))
	})

	t.Run("NamespaceExists", func(t *testing.T) {
		username := sess.username
		out, err := callToolOn[namespaces.ExistsOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "namespace_exists",
			"params": map[string]any{"id": username},
		})
		requireNoError(t, err, "namespace_exists")
		t.Logf("Namespace exists: %v", out.Exists)
	})

	t.Run("NamespaceGet", func(t *testing.T) {
		// First get current user to find their namespace
		usr, err := callToolOn[users.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "current",
			"params": map[string]any{},
		})
		requireNoError(t, err, "get current user")
		out, err := callToolOn[namespaces.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "namespace_get",
			"params": map[string]any{"id": strconv.FormatInt(usr.ID, 10)},
		})
		requireNoError(t, err, "namespace_get")
		requireTruef(t, out.ID > 0, "namespace_get: expected ID > 0")
		t.Logf("Namespace %d: %s", out.ID, out.Name)
	})

	t.Run("NotificationGlobalGet", func(t *testing.T) {
		out, err := callToolOn[notifications.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "notification_global_get",
			"params": map[string]any{},
		})
		requireNoError(t, err, "notification_global_get")
		t.Logf("Global notification level: %s", out.Level)
	})

	t.Run("NotificationGlobalUpdate", func(t *testing.T) {
		out, err := retryOnTransient(ctx, t, "notification_global_update", 3, func() (notifications.Output, error) {
			return callToolOn[notifications.Output](ctx, sess.meta, "gitlab_user", map[string]any{
				"action": "notification_global_update",
				"params": map[string]any{"level": "participating"},
			})
		})
		requireNoError(t, err, "notification_global_update")
		t.Logf("Updated global notification level: %s", out.Level)
	})

	t.Run("NotificationProjectGetUpdate", func(t *testing.T) {
		proj := createProjectMeta(ctx, t, sess.meta)
		out, err := callToolOn[notifications.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "notification_project_get",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "notification_project_get")
		t.Logf("Project notification level: %s", out.Level)

		upd, err := callToolOn[notifications.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "notification_project_update",
			"params": map[string]any{"project_id": proj.pidStr(), "level": "watch"},
		})
		requireNoError(t, err, "notification_project_update")
		t.Logf("Updated project notification level: %s", upd.Level)
	})

	t.Run("NotificationGroupGetUpdate", func(t *testing.T) {
		// Need a group — create one for the test
		grp, err := callToolOn[struct{ ID int64 }](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "create",
			"params": map[string]any{
				"name": uniqueName("notif-grp"),
				"path": uniqueName("notif-grp"),
			},
		})
		requireNoError(t, err, "create group for notification test")
		grpIDStr := strconv.FormatInt(grp.ID, 10)
		defer func() {
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "delete",
				"params": map[string]any{"group_id": grpIDStr},
			})
		}()

		out, err := callToolOn[notifications.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "notification_group_get",
			"params": map[string]any{"group_id": grpIDStr},
		})
		requireNoError(t, err, "notification_group_get")
		t.Logf("Group notification level: %s", out.Level)

		upd, err := callToolOn[notifications.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "notification_group_update",
			"params": map[string]any{"group_id": grpIDStr, "level": "watch"},
		})
		requireNoError(t, err, "notification_group_update")
		t.Logf("Updated group notification level: %s", upd.Level)
	})

	t.Run("KeyGetByFingerprint", func(t *testing.T) {
		// Create an SSH key so we always have one
		sshKey := generateTestSSHKey(t)
		addOut, err := callToolOn[users.SSHKeyOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "add_ssh_key",
			"params": map[string]any{
				"title": "e2e-fingerprint-test",
				"key":   sshKey,
			},
		})
		requireNoError(t, err, "add_ssh_key for fingerprint test")
		defer func() {
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_user", map[string]any{
				"action": "delete_ssh_key",
				"params": map[string]any{"key_id": addOut.ID},
			})
		}()

		out, err := callToolOn[keys.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "key_get_with_user",
			"params": map[string]any{"key_id": addOut.ID},
		})
		requireNoError(t, err, "key_get_with_user")
		requireTruef(t, out.ID > 0, "key_get_with_user: expected ID > 0")
		t.Logf("Key %d with user", out.ID)
	})
}

// TestMeta_UserSSHKeyLifecycle exercises the full SSH key lifecycle via gitlab_user.
func TestMeta_UserSSHKeyLifecycle(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	sshKey := generateTestSSHKey(t)
	var keyID int64

	t.Run("AddSSHKey", func(t *testing.T) {
		out, err := callToolOn[users.SSHKeyOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "add_ssh_key",
			"params": map[string]any{
				"title": "e2e-test-key-" + uniqueName(""),
				"key":   sshKey,
			},
		})
		requireNoError(t, err, "add_ssh_key")
		requireTruef(t, out.ID > 0, "add_ssh_key: expected ID > 0")
		keyID = out.ID
		t.Logf("Added SSH key %d", keyID)
	})

	t.Run("GetSSHKey", func(t *testing.T) {
		requireTruef(t, keyID > 0, "keyID not set")
		out, err := callToolOn[users.SSHKeyOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "get_ssh_key",
			"params": map[string]any{"key_id": keyID},
		})
		requireNoError(t, err, "get_ssh_key")
		requireTruef(t, out.ID == keyID, "get_ssh_key: ID mismatch")
		t.Logf("Got SSH key %d: %s", out.ID, out.Title)
	})

	t.Run("DeleteSSHKey", func(t *testing.T) {
		requireTruef(t, keyID > 0, "keyID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "delete_ssh_key",
			"params": map[string]any{"key_id": keyID},
		})
		requireNoError(t, err, "delete_ssh_key")
		t.Logf("Deleted SSH key %d", keyID)
	})
}

// TestMeta_UserAdmin exercises admin-level user operations via gitlab_user:
// create, modify, block/unblock, deactivate/activate, ban/unban,
// ssh_keys_for_user, add_ssh_key_for_user, emails_for_user, add_email_for_user,
// impersonation tokens, and finally delete.
func TestMeta_UserAdmin(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	uname := uniqueName("usr-adm")
	var testUserID int64

	t.Run("CreateUser", func(t *testing.T) {
		out, err := callToolOn[users.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "create",
			"params": map[string]any{
				"email":                 uname + "@e2e-test.local",
				"name":                  "E2E Test " + uname,
				"username":              uname,
				"password":              "E2eT!Gx9K#p2mNq$8BcZ",
				"skip_confirmation":     true,
				"force_random_password": false,
			},
		})
		requireNoError(t, err, "create user")
		requireTruef(t, out.ID > 0, "created user ID > 0")
		testUserID = out.ID
		t.Logf("Created user %d: %s", testUserID, uname)
	})
	defer func() {
		if testUserID > 0 {
			// Unblock first in case blocked
			_, _ = callToolOn[users.AdminActionOutput](ctx, sess.meta, "gitlab_user", map[string]any{
				"action": "unblock",
				"params": map[string]any{"user_id": testUserID},
			})
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_user", map[string]any{
				"action": "delete",
				"params": map[string]any{"user_id": testUserID},
			})
		}
	}()

	t.Run("ModifyUser", func(t *testing.T) {
		requireTruef(t, testUserID > 0, "testUserID not set")
		out, err := callToolOn[users.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "modify",
			"params": map[string]any{
				"user_id": testUserID,
				"bio":     "E2E test user - modified",
			},
		})
		requireNoError(t, err, "modify user")
		requireTruef(t, out.ID == testUserID, "modify: ID mismatch")
		t.Logf("Modified user %d", testUserID)
	})

	t.Run("BlockUser", func(t *testing.T) {
		requireTruef(t, testUserID > 0, "testUserID not set")
		out, err := callToolOn[users.AdminActionOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "block",
			"params": map[string]any{"user_id": testUserID},
		})
		requireNoError(t, err, "block user")
		requireTruef(t, out.Success, "block should succeed")
		t.Logf("Blocked user %d", testUserID)
	})

	t.Run("UnblockUser", func(t *testing.T) {
		requireTruef(t, testUserID > 0, "testUserID not set")
		out, err := callToolOn[users.AdminActionOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "unblock",
			"params": map[string]any{"user_id": testUserID},
		})
		requireNoError(t, err, "unblock user")
		requireTruef(t, out.Success, "unblock should succeed")
		t.Logf("Unblocked user %d", testUserID)
	})

	t.Run("DeactivateUser", func(t *testing.T) {
		requireTruef(t, testUserID > 0, "testUserID not set")
		out, err := callToolOn[users.AdminActionOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "deactivate",
			"params": map[string]any{"user_id": testUserID},
		})
		requireNoError(t, err, "deactivate user")
		requireTruef(t, out.Success, "deactivate should succeed")
	})

	t.Run("ActivateUser", func(t *testing.T) {
		requireTruef(t, testUserID > 0, "testUserID not set")
		out, err := callToolOn[users.AdminActionOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "activate",
			"params": map[string]any{"user_id": testUserID},
		})
		requireNoError(t, err, "activate user")
		requireTruef(t, out.Success, "activate should succeed")
	})

	t.Run("BanUser", func(t *testing.T) {
		requireTruef(t, testUserID > 0, "testUserID not set")
		out, err := callToolOn[users.AdminActionOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "ban",
			"params": map[string]any{"user_id": testUserID},
		})
		requireNoError(t, err, "ban user")
		requireTruef(t, out.Success, "ban should succeed")
	})

	t.Run("UnbanUser", func(t *testing.T) {
		requireTruef(t, testUserID > 0, "testUserID not set")
		out, err := callToolOn[users.AdminActionOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "unban",
			"params": map[string]any{"user_id": testUserID},
		})
		requireNoError(t, err, "unban user")
		requireTruef(t, out.Success, "unban should succeed")
	})

	// ── SSH keys for user ────────────────────────────────────────────────
	var userKeyID int64
	t.Run("SSHKeysForUser", func(t *testing.T) {
		requireTruef(t, testUserID > 0, "testUserID not set")
		out, err := callToolOn[users.SSHKeyListOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "ssh_keys_for_user",
			"params": map[string]any{"user_id": testUserID},
		})
		requireNoError(t, err, "ssh_keys_for_user")
		t.Logf("SSH keys for user %d: %d", testUserID, len(out.Keys))
	})

	t.Run("AddSSHKeyForUser", func(t *testing.T) {
		requireTruef(t, testUserID > 0, "testUserID not set")
		sshKey := generateTestSSHKey(t)
		out, err := callToolOn[users.SSHKeyOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "add_ssh_key_for_user",
			"params": map[string]any{
				"user_id": testUserID,
				"title":   "e2e-user-key",
				"key":     sshKey,
			},
		})
		requireNoError(t, err, "add_ssh_key_for_user")
		requireTruef(t, out.ID > 0, "add_ssh_key_for_user: expected key ID > 0")
		userKeyID = out.ID
		t.Logf("Added SSH key %d for user %d", userKeyID, testUserID)
	})

	t.Run("GetSSHKeyForUser", func(t *testing.T) {
		requireTruef(t, userKeyID > 0, "userKeyID not set")
		out, err := callToolOn[users.SSHKeyOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "get_ssh_key_for_user",
			"params": map[string]any{"user_id": testUserID, "key_id": userKeyID},
		})
		requireNoError(t, err, "get_ssh_key_for_user")
		requireTruef(t, out.ID == userKeyID, "get_ssh_key_for_user: ID mismatch")
	})

	t.Run("DeleteSSHKeyForUser", func(t *testing.T) {
		requireTruef(t, userKeyID > 0, "userKeyID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "delete_ssh_key_for_user",
			"params": map[string]any{"user_id": testUserID, "key_id": userKeyID},
		})
		requireNoError(t, err, "delete_ssh_key_for_user")
		t.Logf("Deleted SSH key %d for user %d", userKeyID, testUserID)
	})

	// ── Emails for user ──────────────────────────────────────────────────
	var emailID int64
	t.Run("EmailsForUser", func(t *testing.T) {
		requireTruef(t, testUserID > 0, "testUserID not set")
		out, err := callToolOn[useremails.ListOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "emails_for_user",
			"params": map[string]any{"user_id": testUserID},
		})
		requireNoError(t, err, "emails_for_user")
		t.Logf("Emails for user %d: %d", testUserID, len(out.Emails))
	})

	t.Run("AddEmailForUser", func(t *testing.T) {
		requireTruef(t, testUserID > 0, "testUserID not set")
		out, err := callToolOn[useremails.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "add_email_for_user",
			"params": map[string]any{
				"user_id":           testUserID,
				"email":             uname + "-extra@e2e-test.local",
				"skip_confirmation": true,
			},
		})
		requireNoError(t, err, "add_email_for_user")
		requireTruef(t, out.ID > 0, "add_email_for_user: expected email ID > 0")
		emailID = out.ID
		t.Logf("Added email %d for user %d", emailID, testUserID)
	})

	t.Run("GetEmail", func(t *testing.T) {
		requireTruef(t, emailID > 0, "emailID not set")
		// get_email fetches emails for the currently authenticated user, not testUserID— error expected
		_, err := callToolOn[useremails.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "get_email",
			"params": map[string]any{"email_id": emailID},
		})
		requireTruef(t, err != nil, "expected error: email belongs to different user")
		t.Logf("Expected error for cross-user email access: %v", err)
	})

	t.Run("DeleteEmailForUser", func(t *testing.T) {
		requireTruef(t, emailID > 0, "emailID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "delete_email_for_user",
			"params": map[string]any{"user_id": testUserID, "email_id": emailID},
		})
		requireNoError(t, err, "delete_email_for_user")
		t.Logf("Deleted email %d for user %d", emailID, testUserID)
	})

	// ── GPG keys for user ────────────────────────────────────────────────
	t.Run("GPGKeysForUser", func(t *testing.T) {
		requireTruef(t, testUserID > 0, "testUserID not set")
		out, err := callToolOn[usergpgkeys.ListOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "gpg_keys_for_user",
			"params": map[string]any{"user_id": testUserID},
		})
		requireNoError(t, err, "gpg_keys_for_user")
		t.Logf("GPG keys for user %d: %d", testUserID, len(out.Keys))
	})

	// ── Impersonation tokens ─────────────────────────────────────────────
	var impTokenID int64
	t.Run("ListImpersonationTokens", func(t *testing.T) {
		requireTruef(t, testUserID > 0, "testUserID not set")
		out, err := callToolOn[impersonationtokens.ListOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "list_impersonation_tokens",
			"params": map[string]any{"user_id": testUserID},
		})
		requireNoError(t, err, "list_impersonation_tokens")
		t.Logf("Impersonation tokens for user %d: %d", testUserID, len(out.Tokens))
	})

	t.Run("CreateImpersonationToken", func(t *testing.T) {
		requireTruef(t, testUserID > 0, "testUserID not set")
		out, err := callToolOn[impersonationtokens.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "create_impersonation_token",
			"params": map[string]any{
				"user_id":    testUserID,
				"name":       "e2e-imp-token",
				"scopes":     []string{"api"},
				"expires_at": time.Now().AddDate(0, 0, 364).Format("2006-01-02"),
			},
		})
		requireNoError(t, err, "create_impersonation_token")
		requireTruef(t, out.ID > 0, "create_impersonation_token: expected token ID > 0")
		impTokenID = out.ID
		t.Logf("Created impersonation token %d", impTokenID)
	})

	t.Run("GetImpersonationToken", func(t *testing.T) {
		requireTruef(t, impTokenID > 0, "impTokenID not set")
		out, err := callToolOn[impersonationtokens.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "get_impersonation_token",
			"params": map[string]any{"user_id": testUserID, "token_id": impTokenID},
		})
		requireNoError(t, err, "get_impersonation_token")
		requireTruef(t, out.ID == impTokenID, "get_impersonation_token: ID mismatch")
	})

	t.Run("RevokeImpersonationToken", func(t *testing.T) {
		requireTruef(t, impTokenID > 0, "impTokenID not set")
		_, err := callToolOn[impersonationtokens.RevokeOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "revoke_impersonation_token",
			"params": map[string]any{"user_id": testUserID, "token_id": impTokenID},
		})
		requireNoError(t, err, "revoke_impersonation_token")
		t.Logf("Revoked impersonation token %d", impTokenID)
	})

	// ── Personal Access Tokens (admin) ───────────────────────────────────
	t.Run("CreatePersonalAccessToken", func(t *testing.T) {
		requireTruef(t, testUserID > 0, "testUserID not set")
		out, err := callToolOn[impersonationtokens.PATOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "create_personal_access_token",
			"params": map[string]any{
				"user_id": testUserID,
				"name":    "e2e-pat",
				"scopes":  []string{"read_api"},
			},
		})
		requireNoError(t, err, "create_personal_access_token")
		requireTruef(t, out.ID > 0, "create_personal_access_token: expected ID > 0")
		t.Logf("Created PAT %d for user %d", out.ID, testUserID)
	})

	// Delete user is handled by defer above
}

// TestMeta_UserServiceAccounts exercises service account and current-user PAT
// operations via the gitlab_user meta-tool. Service accounts are EE-only
// (returns 404 on CE); the PAT test also runs on all tiers.
func TestMeta_UserServiceAccounts(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if sess.enterprise {
		t.Run("ListServiceAccounts", func(t *testing.T) {
			out, err := callToolOn[users.ServiceAccountListOutput](ctx, sess.meta, "gitlab_user", map[string]any{
				"action": "list_service_accounts",
				"params": map[string]any{},
			})
			requireNoError(t, err, "list_service_accounts")
			t.Logf("Service accounts: %d", len(out.Accounts))
		})

		t.Run("CreateServiceAccount", func(t *testing.T) {
			saName := uniqueName("sa-e2e")
			out, err := callToolOn[users.Output](ctx, sess.meta, "gitlab_user", map[string]any{
				"action": "create_service_account",
				"params": map[string]any{
					"name":     saName,
					"username": saName,
				},
			})
			requireNoError(t, err, "create_service_account")
			requireTruef(t, out.ID > 0, "create_service_account: expected ID > 0")
			t.Logf("Created service account %d: %s", out.ID, saName)
			// Clean up
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_user", map[string]any{
				"action": "delete",
				"params": map[string]any{"user_id": out.ID},
			})
		})
	}

	t.Run("CreateCurrentUserPAT", func(t *testing.T) {
		out, err := callToolOn[users.CurrentUserPATOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "create_current_user_pat",
			"params": map[string]any{
				"name":   "e2e-current-pat",
				"scopes": []string{"k8s_proxy"},
			},
		})
		requireNoError(t, err, "create_current_user_pat")
		requireTruef(t, out.ID > 0, "create_current_user_pat: expected ID > 0")
		t.Logf("Created current user PAT %d", out.ID)
	})
}
