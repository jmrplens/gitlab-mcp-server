//go:build e2e

// impersonationtokens_test.go covers the tokens an administrator holds for
// another account: an impersonation token from creation to revocation, with
// the read by ID and the listings between, and the personal access token an
// administrator mints on a user's behalf. Both are minted for one fixture
// user, never for the run's own account, whose token the whole run depends
// on.

package common

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/impersonationtokens"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The lifetime every token minted here carries; a date GitLab accepts under
// any expiry policy, and long past the run.
const adminMintedTokenLifetime = 30 * 24 * time.Hour

// The scopes the minted tokens carry.
const (
	impersonationTokenScope = "api"
	adminMintedPATScope     = "read_api"
)

// TestImpersonationTokens_Lifecycle_CreateGetListRevoke mints one
// impersonation token per surface for a fixture user, reads it by ID, finds
// it among the user's tokens, revokes it and checks the active listing no
// longer holds it: a revoked token stays listed under every state, so the
// active listing is what observes the revocation.
//
// Replaces: TestMeta_UserAdmin
func TestImpersonationTokens_Lifecycle_CreateGetListRevoke(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.User {
		return fixture.NewUser(e, "imp")
	}, func(e *harness.Env, surface harness.Surface, user fixture.User) {
		s := e.On(surface)
		owner := map[string]any{"user_id": user.ID}
		name := e.Name("imptok")

		created := harness.Do[impersonationtokens.Output](s, actionUserCreateImpersonationToken, withParams(owner, map[string]any{
			"name": name, "scopes": []string{impersonationTokenScope}, "expires_at": time.Now().Add(adminMintedTokenLifetime).Format(time.DateOnly),
		}))
		if created.ID == 0 || created.Token == "" {
			e.T.Fatalf("create_impersonation_token answered %+v, want a token with an ID and its secret", created)
		}
		if created.Name != name || !created.Impersonation || created.UserID != user.ID {
			e.T.Errorf("create_impersonation_token answered name %q impersonation %t user %d, want %q true %d", created.Name, created.Impersonation, created.UserID, name, user.ID)
		}
		token := withParams(owner, map[string]any{"token_id": created.ID})

		got := harness.Do[impersonationtokens.Output](s, actionUserGetImpersonationToken, token)
		if got.ID != created.ID || got.Name != name || !got.Active {
			e.T.Errorf("get_impersonation_token answered id %d name %q active %t, want %d %q true", got.ID, got.Name, got.Active, created.ID, name)
		}

		listed := harness.Do[impersonationtokens.ListOutput](s, actionUserListImpersonationTokens, owner)
		if !containsID(impersonationTokenIDs(listed.Tokens), created.ID) {
			e.T.Errorf("the impersonation tokens of user %d do not hold %d: %+v", user.ID, created.ID, listed.Tokens)
		}

		revoked := harness.Do[impersonationtokens.RevokeOutput](s, actionUserRevokeImpersonationToken, token)
		if !revoked.Revoked || revoked.TokenID != created.ID || revoked.UserID != user.ID {
			e.T.Errorf("revoke_impersonation_token answered %+v, want revoked=true for token %d of user %d", revoked, created.ID, user.ID)
		}
		active := harness.Do[impersonationtokens.ListOutput](s, actionUserListImpersonationTokens, withParams(owner, map[string]any{"state": "active"}))
		if containsID(impersonationTokenIDs(active.Tokens), created.ID) {
			e.T.Errorf("token %d is still listed as active after its revocation", created.ID)
		}
	})
}

// TestImpersonationTokens_AdminMintedPAT_NamesTheUser mints a personal
// access token for a fixture user on every surface, as an administrator,
// checks the answer carries the secret, the scopes and the owner, and
// revokes it afterwards through client-go.
//
// Replaces: TestMeta_UserAdmin
func TestImpersonationTokens_AdminMintedPAT_NamesTheUser(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.User {
		return fixture.NewUser(e, "patuser")
	}, func(e *harness.Env, surface harness.Surface, user fixture.User) {
		s := e.On(surface)
		name := e.Name("pat")

		minted := harness.Do[impersonationtokens.PATOutput](s, actionUserCreatePersonalAccessToken, map[string]any{
			"user_id": user.ID, "name": name, "scopes": []string{adminMintedPATScope}, "expires_at": time.Now().Add(adminMintedTokenLifetime).Format(time.DateOnly),
		})
		if minted.ID == 0 || minted.Token == "" {
			e.T.Fatalf("create_personal_access_token answered %+v, want a token with an ID and its secret", minted)
		}
		e.Defer("token "+name, func(ctx context.Context) error { return revokePersonalToken(ctx, e, minted.ID) })
		if minted.Name != name || minted.UserID != user.ID || !slices.Contains(minted.Scopes, adminMintedPATScope) {
			e.T.Errorf("create_personal_access_token answered name %q user %d scopes %v, want %q %d [%s]", minted.Name, minted.UserID, minted.Scopes, name, user.ID, adminMintedPATScope)
		}
	})
}

// impersonationTokenIDs collects the IDs of listed impersonation tokens.
func impersonationTokenIDs(listed []impersonationtokens.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, token := range listed {
		ids = append(ids, token.ID)
	}
	return ids
}
