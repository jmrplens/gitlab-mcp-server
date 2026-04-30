//go:build e2e

// groupserviceaccounts_test.go exercises EE-only group service account actions
// through the gitlab_group meta-tool when the target GitLab instance supports them.
package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groupserviceaccounts"
)

// TestMeta_GroupServiceAccounts exercises the service account CRUD and PAT
// management actions (service_account_list, service_account_create,
// service_account_update, service_account_delete, service_account_pat_list,
// service_account_pat_create, service_account_pat_revoke) via the gitlab_group
// meta-tool. Group service accounts are EE-only (returns 404 on CE).
func TestMeta_GroupServiceAccounts(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	if sess.enterprise {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// Create a test group.
		grpName := uniqueName("grp-sa")
		grpOut, setupErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "create",
			"params": map[string]any{
				"name":       grpName,
				"path":       grpName,
				"visibility": "private",
			},
		})
		requireNoError(t, setupErr, "create group")
		groupID := grpOut.ID
		groupIDStr := strconv.FormatInt(groupID, 10)
		t.Logf("Created group %d: %s", groupID, grpName)

		defer func() {
			if groupID > 0 {
				_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
					"action": "delete",
					"params": map[string]any{"group_id": groupIDStr},
				})
			}
		}()

		// List — should be empty on a fresh group.
		t.Run("ServiceAccountList_Empty", func(t *testing.T) {
			out, err := callToolOn[groupserviceaccounts.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "service_account_list",
				"params": map[string]any{"group_id": groupIDStr},
			})
			requireNoError(t, err, "service_account_list (empty)")
			requireTruef(t, len(out.Accounts) == 0, "expected 0 service accounts, got %d", len(out.Accounts))
			t.Logf("Service accounts: %d (expected 0)", len(out.Accounts))
		})

		// Create a service account.
		var saID int64
		t.Run("ServiceAccountCreate", func(t *testing.T) {
			saName := uniqueName("sa-grp")
			out, err := callToolOn[groupserviceaccounts.Output](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "service_account_create",
				"params": map[string]any{
					"group_id": groupIDStr,
					"name":     saName,
					"username": saName,
				},
			})
			requireNoError(t, err, "service_account_create")
			requireTruef(t, out.ID > 0, "service_account_create: expected ID > 0")
			saID = out.ID
			t.Logf("Created service account %d: %s", saID, out.Username)
		})

		// Update the service account.
		t.Run("ServiceAccountUpdate", func(t *testing.T) {
			requireTruef(t, saID > 0, "saID not set")
			out, err := callToolOn[groupserviceaccounts.Output](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "service_account_update",
				"params": map[string]any{
					"group_id":           groupIDStr,
					"service_account_id": saID,
					"name":               "Updated SA Name",
				},
			})
			requireNoError(t, err, "service_account_update")
			requireTruef(t, out.ID == saID, "service_account_update: ID mismatch")
			t.Logf("Updated service account %d", out.ID)
		})

		// List — should now have one service account.
		t.Run("ServiceAccountList_One", func(t *testing.T) {
			out, err := callToolOn[groupserviceaccounts.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "service_account_list",
				"params": map[string]any{"group_id": groupIDStr},
			})
			requireNoError(t, err, "service_account_list (one)")
			requireTruef(t, len(out.Accounts) >= 1, "expected at least 1 service account, got %d", len(out.Accounts))
			t.Logf("Service accounts: %d", len(out.Accounts))
		})

		// Create a PAT for the service account.
		var patID int64
		t.Run("ServiceAccountPATCreate", func(t *testing.T) {
			requireTruef(t, saID > 0, "saID not set")
			out, err := callToolOn[groupserviceaccounts.PATOutput](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "service_account_pat_create",
				"params": map[string]any{
					"group_id":           groupIDStr,
					"service_account_id": saID,
					"name":               "e2e-pat",
					"scopes":             []string{"api"},
				},
			})
			requireNoError(t, err, "service_account_pat_create")
			requireTruef(t, out.ID > 0, "service_account_pat_create: expected ID > 0")
			patID = out.ID
			t.Logf("Created PAT %d for service account %d", patID, saID)
		})

		// List PATs.
		t.Run("ServiceAccountPATList", func(t *testing.T) {
			requireTruef(t, saID > 0, "saID not set")
			out, err := callToolOn[groupserviceaccounts.ListPATOutput](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "service_account_pat_list",
				"params": map[string]any{
					"group_id":           groupIDStr,
					"service_account_id": saID,
				},
			})
			requireNoError(t, err, "service_account_pat_list")
			requireTruef(t, len(out.Tokens) >= 1, "expected at least 1 PAT, got %d", len(out.Tokens))
			t.Logf("PATs: %d", len(out.Tokens))
		})

		// Revoke PAT.
		t.Run("ServiceAccountPATRevoke", func(t *testing.T) {
			requireTruef(t, patID > 0, "patID not set")
			err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "service_account_pat_revoke",
				"params": map[string]any{
					"group_id":           groupIDStr,
					"service_account_id": saID,
					"token_id":           patID,
				},
			})
			requireNoError(t, err, "service_account_pat_revoke")
			t.Logf("Revoked PAT %d", patID)
		})

		// Delete the service account.
		t.Run("ServiceAccountDelete", func(t *testing.T) {
			requireTruef(t, saID > 0, "saID not set")
			err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "service_account_delete",
				"params": map[string]any{
					"group_id":           groupIDStr,
					"service_account_id": saID,
					"hard_delete":        true,
				},
			})
			requireNoError(t, err, "service_account_delete")
			t.Logf("Deleted service account %d", saID)
		})
	}
}
