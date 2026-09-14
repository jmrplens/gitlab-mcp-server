//go:build e2e

// accesstokens_test.go covers the three kinds of access token the access
// tool manages, each through its whole life: a project token and a group
// token created, read, listed, rotated by their owner, rotated by
// themselves from a session started on their own value, and revoked; and a
// personal token, introspected, rotated and revoked by an administrator,
// then rotated and revoked by itself. The self operations act on the
// credential that makes the call, which is why each runs in a session the
// harness starts on that token and closes with the subtest. No operation
// here ever touches the run's own token.

package common

import (
	"context"
	"net/http"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/accesstokens"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The shape every token minted here has.
const (
	// accessTokenScope is the scope the tokens carry: api, so the session
	// started on one is served the write-capable surface a rotation needs.
	accessTokenScope = "api"
	// accessTokenLifetime is how long a minted token lives, and a rotated one
	// is given the same again.
	accessTokenLifetime = 30 * 24 * time.Hour
	// tokenStateActive is the listing filter that leaves revoked tokens out,
	// which is what observes a revocation: a revoked token stays listed under
	// the default filter.
	tokenStateActive = "active"
)

// The polling a bot's self-rotation is given. A bot user created moments ago
// has its project authorizations refreshed from a background job, and until
// that lands its own project answers 404 to it; under a whole suite's load
// the old suite saw the window exceed two minutes.
const (
	rotateSelfInterval = 5 * time.Second
	rotateSelfWait     = 4 * time.Minute
)

// TestAccessTokens_Project_Lifecycle_ThroughSelfRotation mints one project
// token per surface on a project of the test's own, reads it, finds it
// listed, rotates it, has the rotated token rotate itself from a session on
// its own value, revokes the latest generation and checks the active
// listing no longer holds it.
//
// Replaces: TestIndividual_AccessTokens, TestMeta_AccessTokens, TestMeta_AccessTokensProject, TestIndividual_ProjectAccessTokenRotateSelf
func TestAccessTokens_Project_Lifecycle_ThroughSelfRotation(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("ptok"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		scope := map[string]any{"project_id": project.IDParam()}
		name := e.Name("ptok")

		created := harness.Do[accesstokens.Output](s, actionAccessTokenProjectCreate, withParams(scope, map[string]any{
			"name": name, "scopes": []string{accessTokenScope}, "access_level": int(gl.MaintainerPermissions), "expires_at": accessTokenExpiry(),
		}))
		if created.ID == 0 || created.Token == "" {
			e.T.Fatalf("token_project_create answered %+v, want a token with an ID and its secret", created)
		}
		latest := created.ID
		e.Defer("project token "+name, func(ctx context.Context) error {
			_, err := e.Client().GL().ProjectAccessTokens.RevokeProjectAccessToken(project.ID, latest, gl.WithContext(ctx))
			return tolerateGoneToken(err)
		})

		got := harness.Do[accesstokens.Output](s, actionAccessTokenProjectGet, withParams(scope, map[string]any{"token_id": created.ID}))
		if got.ID != created.ID || got.Name != name || !got.Active {
			e.T.Errorf("token_project_get answered id %d name %q active %t, want %d %q true", got.ID, got.Name, got.Active, created.ID, name)
		}
		listed := harness.Do[accesstokens.ListOutput](s, actionAccessTokenProjectList, scope)
		if !containsID(accessTokenIDs(listed.Tokens), created.ID) {
			e.T.Errorf("the tokens of project %d do not hold %d: %+v", project.ID, created.ID, listed.Tokens)
		}

		rotated := harness.Do[accesstokens.Output](s, actionAccessTokenProjectRotate, withParams(scope, map[string]any{"token_id": created.ID, "expires_at": accessTokenExpiry()}))
		if rotated.ID == 0 || rotated.ID == created.ID || rotated.Token == "" {
			e.T.Fatalf("token_project_rotate answered %+v, want a new token with a new ID and its secret", rotated)
		}
		latest = rotated.ID

		// The rotated token rotates itself, from a session that runs on it.
		self := e.Session(harness.ServerConfig{Surface: surface, Token: rotated.Token, Private: true})
		selfRotated := harness.Eventually(self, actionAccessTokenProjectRotateSelf, withParams(scope, map[string]any{"expires_at": accessTokenExpiry()}),
			rotateSelfInterval, rotateSelfWait, func(out accesstokens.Output) bool { return out.ID != 0 && out.Token != "" })
		if selfRotated.ID == rotated.ID {
			e.T.Errorf("token_project_rotate_self answered the same ID %d as the token it ran on", rotated.ID)
		}
		latest = selfRotated.ID

		harness.DoVoid(s, actionAccessTokenProjectRevoke, withParams(scope, map[string]any{"token_id": selfRotated.ID}))
		active := harness.Do[accesstokens.ListOutput](s, actionAccessTokenProjectList, withParams(scope, map[string]any{"state": tokenStateActive}))
		if containsID(accessTokenIDs(active.Tokens), selfRotated.ID) {
			e.T.Errorf("token %d is still listed as active after its revocation", selfRotated.ID)
		}
	})
}

// TestAccessTokens_Group_Lifecycle_ThroughSelfRotation is the project
// lifecycle on a group of the test's own: mint, read, list, rotate, rotate
// from the token's own session, revoke and check the active listing.
//
// Replaces: TestIndividual_GroupAccessTokens
func TestAccessTokens_Group_Lifecycle_ThroughSelfRotation(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("gtok"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		scope := map[string]any{"group_id": group.IDParam()}
		name := e.Name("gtok")

		created := harness.Do[accesstokens.Output](s, actionAccessTokenGroupCreate, withParams(scope, map[string]any{
			"name": name, "scopes": []string{accessTokenScope}, "access_level": int(gl.MaintainerPermissions), "expires_at": accessTokenExpiry(),
		}))
		if created.ID == 0 || created.Token == "" {
			e.T.Fatalf("token_group_create answered %+v, want a token with an ID and its secret", created)
		}
		latest := created.ID
		e.Defer("group token "+name, func(ctx context.Context) error {
			_, err := e.Client().GL().GroupAccessTokens.RevokeGroupAccessToken(group.ID, latest, gl.WithContext(ctx))
			return tolerateGoneToken(err)
		})

		got := harness.Do[accesstokens.Output](s, actionAccessTokenGroupGet, withParams(scope, map[string]any{"token_id": created.ID}))
		if got.ID != created.ID || got.Name != name || !got.Active {
			e.T.Errorf("token_group_get answered id %d name %q active %t, want %d %q true", got.ID, got.Name, got.Active, created.ID, name)
		}
		listed := harness.Do[accesstokens.ListOutput](s, actionAccessTokenGroupList, scope)
		if !containsID(accessTokenIDs(listed.Tokens), created.ID) {
			e.T.Errorf("the tokens of group %d do not hold %d: %+v", group.ID, created.ID, listed.Tokens)
		}

		rotated := harness.Do[accesstokens.Output](s, actionAccessTokenGroupRotate, withParams(scope, map[string]any{"token_id": created.ID, "expires_at": accessTokenExpiry()}))
		if rotated.ID == 0 || rotated.ID == created.ID || rotated.Token == "" {
			e.T.Fatalf("token_group_rotate answered %+v, want a new token with a new ID and its secret", rotated)
		}
		latest = rotated.ID

		self := e.Session(harness.ServerConfig{Surface: surface, Token: rotated.Token, Private: true})
		selfRotated := harness.Eventually(self, actionAccessTokenGroupRotateSelf, withParams(scope, map[string]any{"expires_at": accessTokenExpiry()}),
			rotateSelfInterval, rotateSelfWait, func(out accesstokens.Output) bool { return out.ID != 0 && out.Token != "" })
		if selfRotated.ID == rotated.ID {
			e.T.Errorf("token_group_rotate_self answered the same ID %d as the token it ran on", rotated.ID)
		}
		latest = selfRotated.ID

		harness.DoVoid(s, actionAccessTokenGroupRevoke, withParams(scope, map[string]any{"token_id": selfRotated.ID}))
		active := harness.Do[accesstokens.ListOutput](s, actionAccessTokenGroupList, withParams(scope, map[string]any{"state": tokenStateActive}))
		if containsID(accessTokenIDs(active.Tokens), selfRotated.ID) {
			e.T.Errorf("token %d is still listed as active after its revocation", selfRotated.ID)
		}
	})
}

// TestAccessTokens_Personal_AdminAndSelfOperations mints two tokens per
// surface for one fixture user through the admin API. The first is what an
// administrator does to somebody else's token: reads it, finds it in the
// user's listing, rotates it and revokes the rotation. The second is what a
// token does to itself: rotates itself from a session on its own value,
// and the rotated one revokes itself from a session on its own. Rotation
// comes before revocation because each ends the credential it ran on, and
// the administrator's listing is where a self-revocation shows, since the
// revoked credential can no longer read anything.
//
// Replaces: TestIndividual_PersonalAccessTokenAdminOps, TestIndividual_PersonalAccessTokenSelfOps, TestMeta_AccessTokensPersonal
func TestAccessTokens_Personal_AdminAndSelfOperations(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.User {
		return fixture.NewUser(e, "pat")
	}, func(e *harness.Env, surface harness.Surface, user fixture.User) {
		s := e.On(surface)
		owner := map[string]any{"user_id": user.ID}

		administered := fixture.NewToken(e, user, accessTokenScope)
		got := harness.Do[accesstokens.Output](s, actionAccessTokenPersonalGet, map[string]any{"token_id": administered.ID})
		if got.ID != administered.ID || got.UserID != user.ID || !got.Active {
			e.T.Errorf("token_personal_get answered id %d user %d active %t, want %d %d true", got.ID, got.UserID, got.Active, administered.ID, user.ID)
		}
		listed := harness.Do[accesstokens.ListOutput](s, actionAccessTokenPersonalList, owner)
		if !containsID(accessTokenIDs(listed.Tokens), administered.ID) {
			e.T.Errorf("the tokens of user %d do not hold %d: %+v", user.ID, administered.ID, listed.Tokens)
		}
		rotated := harness.Do[accesstokens.Output](s, actionAccessTokenPersonalRotate, map[string]any{"token_id": administered.ID, "expires_at": accessTokenExpiry()})
		if rotated.ID == 0 || rotated.ID == administered.ID || rotated.Token == "" {
			e.T.Fatalf("token_personal_rotate answered %+v, want a new token with a new ID and its secret", rotated)
		}
		e.Defer("rotated token", func(ctx context.Context) error { return revokePersonalToken(ctx, e, rotated.ID) })
		harness.DoVoid(s, actionAccessTokenPersonalRevoke, map[string]any{"token_id": rotated.ID})
		active := harness.Do[accesstokens.ListOutput](s, actionAccessTokenPersonalList, withParams(owner, map[string]any{"state": tokenStateActive}))
		if containsID(accessTokenIDs(active.Tokens), rotated.ID) {
			e.T.Errorf("token %d is still listed as active after its revocation", rotated.ID)
		}

		self := fixture.NewToken(e, user, accessTokenScope)
		rotating := e.Session(harness.ServerConfig{Surface: surface, Token: self.Value, Private: true})
		selfRotated := harness.Do[accesstokens.Output](rotating, actionAccessTokenPersonalRotateSelf, map[string]any{"expires_at": accessTokenExpiry()})
		if selfRotated.ID == 0 || selfRotated.ID == self.ID || selfRotated.Token == "" {
			e.T.Fatalf("token_personal_rotate_self answered %+v, want a new token with a new ID and its secret", selfRotated)
		}
		e.Defer("self-rotated token", func(ctx context.Context) error { return revokePersonalToken(ctx, e, selfRotated.ID) })

		revoking := e.Session(harness.ServerConfig{Surface: surface, Token: selfRotated.Token, Private: true})
		harness.DoVoid(revoking, actionAccessTokenPersonalRevokeSelf, nil)
		stillActive := harness.Do[accesstokens.ListOutput](s, actionAccessTokenPersonalList, withParams(owner, map[string]any{"state": tokenStateActive}))
		if containsID(accessTokenIDs(stillActive.Tokens), selfRotated.ID) {
			e.T.Errorf("token %d is still listed as active after revoking itself", selfRotated.ID)
		}
	})
}

// accessTokenExpiry spells the expiry every token minted or rotated here is
// given, in the date form the actions take.
func accessTokenExpiry() string {
	return time.Now().Add(accessTokenLifetime).Format(time.DateOnly)
}

// tolerateGoneToken turns the answers a revocation gets for a token that is
// already gone or already revoked into no error, which is the state a test
// that rotated or revoked it leaves behind.
func tolerateGoneToken(err error) error {
	if err != nil && !fixture.IsStatus(err, http.StatusNotFound) && !fixture.IsStatus(err, http.StatusBadRequest) {
		return err
	}
	return nil
}

// accessTokenIDs collects the IDs of listed access tokens.
func accessTokenIDs(listed []accesstokens.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, token := range listed {
		ids = append(ids, token.ID)
	}
	return ids
}
