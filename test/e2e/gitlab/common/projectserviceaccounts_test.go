//go:build e2e

// projectserviceaccounts_test.go covers a project's service accounts and
// their tokens: create, update, list, mint a token, list the tokens, rotate
// it, revoke it, delete the account. The actions are Free in the catalog
// and the endpoints answer on every edition, so the scenario runs on both
// runtimes; the old suite kept it in its Enterprise half.

package common

import (
	"maps"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projectserviceaccounts"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The lifetimes the fixture tokens are minted and rotated with.
const (
	serviceAccountTokenLifetime = 7 * 24 * time.Hour
	serviceAccountRotatedLife   = 14 * 24 * time.Hour
)

// TestProjectServiceAccounts_Lifecycle_AccountAndTokens walks one service
// account per surface, on a project of the surface's own, through its
// whole life and its token's.
//
// Replaces: TestMeta_ProjectServiceAccounts
func TestProjectServiceAccounts_Lifecycle_AccountAndTokens(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("projsa"))
		id := project.IDParam()

		empty := harness.Do[projectserviceaccounts.ListOutput](s, actionProjectServiceAccountList, map[string]any{"project_id": id})
		if len(empty.Accounts) != 0 {
			e.T.Errorf("a fresh project lists %d service accounts: %+v", len(empty.Accounts), empty.Accounts)
		}

		name := e.Name("sa")
		created := harness.Do[projectserviceaccounts.Output](s, actionProjectServiceAccountCreate, map[string]any{
			"project_id": id, "name": name, "username": name,
		})
		if created.ID == 0 {
			e.T.Fatalf("service_account_create answered %+v, want an account with an ID", created)
		}
		account := map[string]any{"project_id": id, "service_account_id": created.ID}

		updated := harness.Do[projectserviceaccounts.Output](s, actionProjectServiceAccountUpdate, withParams(account, map[string]any{"name": "Updated " + name}))
		if updated.ID != created.ID {
			e.T.Errorf("service_account_update answered account %d, want %d", updated.ID, created.ID)
		}
		listed := harness.Do[projectserviceaccounts.ListOutput](s, actionProjectServiceAccountList, map[string]any{"project_id": id})
		found := false
		for _, item := range listed.Accounts {
			if item.ID == created.ID {
				found = true
			}
		}
		if !found {
			e.T.Errorf("the listing does not hold the created account %d: %+v", created.ID, listed.Accounts)
		}

		token := harness.Do[projectserviceaccounts.PATOutput](s, actionProjectServiceAccountPATCreate, withParams(account, map[string]any{
			"name": e.Name("pat"), "scopes": []string{"api"}, "expires_at": time.Now().Add(serviceAccountTokenLifetime).Format(time.DateOnly),
		}))
		if token.ID == 0 {
			e.T.Fatalf("service_account_pat_create answered %+v, want a token with an ID", token)
		}
		tokens := harness.Do[projectserviceaccounts.ListPATOutput](s, actionProjectServiceAccountPATList, account)
		if len(tokens.Tokens) == 0 {
			e.T.Errorf("the token listing of account %d is empty right after minting one", created.ID)
		}
		rotated := harness.Do[projectserviceaccounts.PATOutput](s, actionProjectServiceAccountPATRotate, withParams(account, map[string]any{
			"token_id": token.ID, "expires_at": time.Now().Add(serviceAccountRotatedLife).Format(time.DateOnly),
		}))
		if rotated.ID == 0 {
			e.T.Fatalf("service_account_pat_rotate answered %+v, want a token with an ID", rotated)
		}
		harness.DoVoid(s, actionProjectServiceAccountPATRevoke, withParams(account, map[string]any{"token_id": rotated.ID}))

		harness.DoVoid(s, actionProjectServiceAccountDelete, withParams(account, map[string]any{"hard_delete": true}))
	})
}

// withParams returns one parameter map holding both, for a call that shares
// a scope with its siblings and adds its own.
func withParams(shared, own map[string]any) map[string]any {
	merged := make(map[string]any, len(shared)+len(own))
	maps.Copy(merged, shared)
	maps.Copy(merged, own)
	return merged
}
