//go:build e2e && enterprise

// users_meta_ee_test.go exercises GitLab EE Premium/Ultimate-only user
// meta-tool actions: instance-level service accounts (Premium/Ultimate
// only) and the current-user PAT action (available on all tiers).
//
// The CE equivalent test (TestMeta_UserServiceAccounts in
// users_meta_ce_test.go) is excluded from the EE build, so we
// duplicate the coverage here for the enterprise build to keep the
// EE suite complete and self-contained.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/users"
)

// TestEE_MetaUserServiceAccounts exercises EE instance service account
// operations via the gitlab_user meta-tool. Service accounts are
// Premium/Ultimate-only; the current-user PAT test runs on all tiers.
func TestEE_MetaUserServiceAccounts(t *testing.T) {
	if !sess.enterprise {
		t.Skip("user service accounts require GitLab Premium/Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Run("ListServiceAccounts", func(t *testing.T) {
		out, err := callToolOn[users.ServiceAccountListOutput](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "list_service_accounts",
			"params": map[string]any{},
		})
		requireNoError(t, err, "list_service_accounts")
		t.Logf("Service accounts: %d", len(out.Accounts))
	})

	var serviceAccountID int64

	// Register the cleanup on the parent test so the service account
	// survives across subtests (notably the subsequent UpdateServiceAccount
	// subtest, which needs the account to still exist).
	t.Run("CreateServiceAccount", func(t *testing.T) {
		saName := "sa-e2e"
		out, err := callToolOn[users.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "create_service_account",
			"params": map[string]any{
				"name":     saName,
				"username": saName,
			},
		})
		requireNoError(t, err, "create_service_account")
		requireTruef(t, out.ID > 0, "create_service_account: expected ID > 0")
		serviceAccountID = out.ID
		t.Logf("Created instance service account %d: %s", out.ID, saName)
	})

	// Delete the service account once the entire test (all subtests) is
	// done. Registering on the parent t ensures the cleanup runs after
	// the UpdateServiceAccount subtest has had a chance to operate on
	// the still-existing service account.
	t.Cleanup(func() {
		if serviceAccountID == 0 {
			return
		}
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = callToolVoidOn(cleanCtx, sess.meta, "gitlab_user", map[string]any{
			"action": "delete",
			"params": map[string]any{"user_id": serviceAccountID},
		})
	})

	t.Run("UpdateServiceAccount", func(t *testing.T) {
		requireTruef(t, serviceAccountID > 0, "serviceAccountID not set")
		out, err := callToolOn[users.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "update_service_account",
			"params": map[string]any{
				"service_account_id": serviceAccountID,
				"name":               "Updated EE Service Account",
			},
		})
		requireNoError(t, err, "update_service_account")
		requireTruef(t, out.ID == serviceAccountID, "update_service_account: ID mismatch: got %d want %d", out.ID, serviceAccountID)
		t.Logf("Updated instance service account %d", out.ID)
	})
}
