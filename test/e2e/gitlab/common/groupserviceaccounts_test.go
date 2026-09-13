//go:build e2e

// groupserviceaccounts_test.go covers a group's service accounts and their
// tokens: a fresh group has none, one is created, renamed and listed, a
// token is minted for it, listed, rotated and revoked, and the account is
// deleted. The actions are Free in the catalog and the endpoints answer on
// every edition, so the scenario runs on both runtimes; the old suite kept
// all but the rotation in its Enterprise half.

package common

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupserviceaccounts"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// activeTokenIDs lists the ids of the tokens of a listing that are still
// usable, which is what a revocation takes away.
func activeTokenIDs(tokens []groupserviceaccounts.PATOutput) []int64 {
	ids := make([]int64, 0, len(tokens))
	for _, token := range tokens {
		if token.Active && !token.Revoked {
			ids = append(ids, token.ID)
		}
	}
	return ids
}

// TestGroupServiceAccounts_Lifecycle_AccountAndToken walks one service
// account per surface, in a group of the surface's own so the empty listing
// before is exact, through its whole life and its token's.
//
// Replaces: TestMeta_GroupServiceAccounts
func TestGroupServiceAccounts_Lifecycle_AccountAndToken(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("grpsa"))
		params := map[string]any{"group_id": group.IDParam()}

		empty := harness.Do[groupserviceaccounts.ListOutput](s, actionGroupServiceAccountList, params)
		if len(empty.Accounts) != 0 {
			e.T.Errorf("a fresh group lists %d service account(s): %+v", len(empty.Accounts), empty.Accounts)
		}

		name := e.Name("sa")
		created := harness.Do[groupserviceaccounts.Output](s, actionGroupServiceAccountCreate, withParams(params, map[string]any{"name": name, "username": name}))
		if created.ID == 0 || created.Username != name {
			e.T.Fatalf("service_account_create answered %+v, want the account %q with an ID", created, name)
		}
		// The account is a user, and outlives its group: the fixture's user
		// deletion is the safety net for a delete the test did not reach.
		e.Defer("service account "+name, func(ctx context.Context) error {
			return fixture.DeleteUser(ctx, e.Client(), created.ID)
		})
		account := withParams(params, map[string]any{"service_account_id": created.ID})

		updated := harness.Do[groupserviceaccounts.Output](s, actionGroupServiceAccountUpdate, withParams(account, map[string]any{"name": "Updated " + name}))
		if updated.ID != created.ID || updated.Name != "Updated "+name {
			e.T.Errorf("service_account_update answered %+v, want account %d renamed", updated, created.ID)
		}
		listed := harness.Do[groupserviceaccounts.ListOutput](s, actionGroupServiceAccountList, params)
		if len(listed.Accounts) != 1 || listed.Accounts[0].ID != created.ID {
			e.T.Errorf("the group lists the service accounts %+v, want exactly the created %d", listed.Accounts, created.ID)
		}

		token := harness.Do[groupserviceaccounts.PATOutput](s, actionGroupServiceAccountPATCreate, withParams(account, map[string]any{
			"name": e.Name("pat"), "scopes": []string{"api"},
		}))
		if token.ID == 0 || token.Token == "" {
			e.T.Fatalf("service_account_pat_create answered %+v, want a token with an ID and its secret", token)
		}
		tokens := harness.Do[groupserviceaccounts.ListPATOutput](s, actionGroupServiceAccountPATList, account)
		if !containsID(activeTokenIDs(tokens.Tokens), token.ID) {
			e.T.Errorf("the account's active tokens %v do not hold the minted %d", activeTokenIDs(tokens.Tokens), token.ID)
		}

		// A rotation revokes the token and mints another in its place, so
		// the revocation below is of the replacement.
		rotated := harness.Do[groupserviceaccounts.PATOutput](s, actionGroupServiceAccountPATRotate, withParams(account, map[string]any{
			"token_id": token.ID, "expires_at": time.Now().Add(serviceAccountRotatedLife).Format(time.DateOnly),
		}))
		if rotated.ID == 0 || rotated.ID == token.ID || rotated.Token == "" {
			e.T.Fatalf("service_account_pat_rotate answered %+v, want a new token with an ID and its secret", rotated)
		}

		harness.DoVoid(s, actionGroupServiceAccountPATRevoke, withParams(account, map[string]any{"token_id": rotated.ID}))
		after := harness.Do[groupserviceaccounts.ListPATOutput](s, actionGroupServiceAccountPATList, account)
		if active := activeTokenIDs(after.Tokens); containsID(active, rotated.ID) || containsID(active, token.ID) {
			e.T.Errorf("the account's active tokens %v still hold %d or %d after the rotation and the revocation", active, token.ID, rotated.ID)
		}

		harness.DoVoid(s, actionGroupServiceAccountDelete, withParams(account, map[string]any{"hard_delete": true}))
	})
}
