//go:build e2e

// users_test.go covers the instance-level service accounts of the user
// group: list them, create one, update it. The account is a user, so it is
// removed through the fixture library's user deletion rather than through
// a delete of its own, which the group has none for.

package common

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestUserServiceAccounts_Instance_ListsCreatesAndUpdates creates one
// instance service account per surface, finds it in the listing and
// renames it. Service accounts are an administrator's to create.
//
// Replaces: TestEE_MetaUserServiceAccounts
func TestUserServiceAccounts_Instance_ListsCreatesAndUpdates(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		before := harness.Do[users.ServiceAccountListOutput](s, actionUserListServiceAccounts, nil)
		e.T.Logf("%d instance service account(s) before the create", len(before.Accounts))

		name := e.Name("sa")
		created := harness.Do[users.Output](s, actionUserCreateServiceAccount, map[string]any{"name": name, "username": name})
		if created.ID == 0 {
			e.T.Fatalf("create_service_account answered %+v, want an account with an ID", created)
		}
		// The account is a user, and the user group has no delete of its own
		// for a service account: the test's cleanup removes it the way an
		// agent's session would, through the user delete, and the fixture's
		// deletion behind it is the safety net for a delete that failed.
		e.Defer("service account "+name, func(ctx context.Context) error {
			return fixture.DeleteUser(ctx, e.Client(), created.ID)
		})
		e.T.Cleanup(func() {
			if _, err := harness.Try[users.DeleteOutput](s, actionUserDelete, map[string]any{"user_id": created.ID}, harness.For(harness.PurposeCleanup)); err != nil {
				e.T.Logf("the cleanup delete of service account %d answered: %v", created.ID, err)
			}
		})

		// Newest first, so the account just created is on the first page
		// whatever earlier runs left on the instance.
		after := harness.Do[users.ServiceAccountListOutput](s, actionUserListServiceAccounts, map[string]any{"order_by": "id", "sort": "desc"})
		found := false
		for _, account := range after.Accounts {
			if account.ID == created.ID {
				found = true
			}
		}
		if !found {
			e.T.Errorf("the listing does not hold the created account %d: %+v", created.ID, after.Accounts)
		}

		updated := harness.Do[users.Output](s, actionUserUpdateServiceAccount, map[string]any{
			"service_account_id": created.ID, "name": "Updated " + name,
		})
		if updated.ID != created.ID {
			e.T.Errorf("update_service_account answered account %d, want %d", updated.ID, created.ID)
		}
	})
}
