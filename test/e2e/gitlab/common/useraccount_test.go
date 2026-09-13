//go:build e2e

// useraccount_test.go covers what the run user's own account answers about
// itself: the alias of the current-user read, the listing filtered to the
// account and the read by ID, the counts and the event feed an account
// carries, the avatar lookup, the status it sets and reads back, the avatar
// it uploads and the token it mints for itself. Every session of the run
// shares this account, so the writes hold the current-user lock and put back
// what they changed.

package common

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/avatar"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The values the account writes carry.
const (
	// statusEmoji is the emoji the status scenario sets, one GitLab ships.
	statusEmoji = "coffee"
	// avatarFilename names the PNG the avatar upload sends.
	avatarFilename = "e2e-avatar.png"
	// avatarLookupEmail is an address the avatar lookup hashes; it belongs to
	// nobody, which is the case that exercises the gravatar fallback.
	avatarLookupEmail = "e2e-avatar-lookup@e2e-test.invalid"
	// avatarLookupSize is the pixel size the avatar lookup asks for. It is
	// passed rather than left out because the individual surface derives its
	// required list from the input struct's json tags, where a field without
	// omitempty is required, so the size the handler treats as optional is a
	// required property of gitlab_get_avatar and the call is refused without
	// it. The other two surfaces take the required list from the jsonschema
	// tags instead, where only the email is marked, and accept the same call
	// with no size at all.
	avatarLookupSize = 64
	// selfTokenScope is the one scope GitLab lets an account mint a token
	// with for itself.
	selfTokenScope = "k8s_proxy"
	// selfTokenLifetime is how long that token lives.
	selfTokenLifetime = 30 * 24 * time.Hour
)

// TestUserMe_Alias_NamesTheRunUser reads the authenticated user through the
// alias the user tool keeps for it and checks it is the run's user. The
// individual surface is left out because the alias declares the tool name
// user.current already registered, so there it is user.current that answers.
//
// Replaces: TestMeta_UserSelf
func TestUserMe_Alias_NamesTheRunUser(t *testing.T) {
	e := harness.New(t)
	rt := e.Runtime()

	harness.OnSurfaces(e, "user.me declares the individual tool name user.current owns, so the individual surface reaches the account through user.current",
		[]harness.Surface{harness.SurfaceDynamic, harness.SurfaceMeta}, func(e *harness.Env, surface harness.Surface) {
			s := e.On(surface)
			me := harness.Do[users.Output](s, actionUserMe, nil)
			if me.ID != rt.UserID || me.Username != rt.Username {
				e.T.Errorf("user me answered %d %q, want the run's user %d %q", me.ID, me.Username, rt.UserID, rt.Username)
			}
		})
}

// TestUserAccount_ListAndGet_FindTheRunUser lists users filtered to the run
// user's name and reads the account by ID, on every surface, checking both
// answer with the run's own user.
//
// Replaces: TestIndividual_Users, TestMeta_Users
func TestUserAccount_ListAndGet_FindTheRunUser(t *testing.T) {
	e := harness.New(t)
	rt := e.Runtime()

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		listed := harness.Do[users.ListOutput](s, actionUserList, map[string]any{"username": rt.Username})
		if len(listed.Users) != 1 || listed.Users[0].ID != rt.UserID {
			e.T.Errorf("user list filtered to %q answered %d user(s) %+v, want exactly the run's user %d", rt.Username, len(listed.Users), listed.Users, rt.UserID)
		}

		got := harness.Do[users.Output](s, actionUserGet, map[string]any{"user_id": rt.UserID})
		if got.ID != rt.UserID || got.Username != rt.Username {
			e.T.Errorf("user get answered %d %q, want %d %q", got.ID, got.Username, rt.UserID, rt.Username)
		}
	})
}

// TestUserAccount_CountsFeedAndAvatar_AnswerForTheRunUser reads the
// association counts and the contribution events of the run user, which the
// shared World gives an issue, a merge request and a push to count, and
// resolves an avatar for an address nobody owns.
//
// Replaces: TestMeta_UserSelf
func TestUserAccount_CountsFeedAndAvatar_AnswerForTheRunUser(t *testing.T) {
	e := harness.New(t)
	rt := e.Runtime()
	fixture.SharedWorld(e)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		counts := harness.Do[users.AssociationsCountOutput](s, actionUserAssociationsCount, map[string]any{"user_id": rt.UserID})
		if counts.IssuesCount < 1 || counts.MergeRequestsCount < 1 {
			e.T.Errorf("associations_count answered %+v, want at least the World's issue and merge request the run user authored", counts)
		}

		events := harness.Do[users.ContributionEventsOutput](s, actionUserContributionEvents, map[string]any{"user_id": rt.UserID})
		if len(events.Events) == 0 {
			e.T.Error("contribution_events answered no event for the run user, and the World was built by it")
		}
		for _, event := range events.Events {
			if event.ActionName == "" {
				e.T.Errorf("contribution event %d carries no action name: %+v", event.ID, event)
			}
		}

		found := harness.Do[avatar.GetOutput](s, actionUserAvatarGet, map[string]any{"email": avatarLookupEmail, "size": avatarLookupSize})
		if found.AvatarURL == "" {
			e.T.Errorf("avatar_get answered no URL for %s", avatarLookupEmail)
		}
	})
}

// TestUserAccount_AdminReads_MembershipsAndActivities reads the run user's
// memberships, which the shared World's group is one of, and the instance's
// user activity feed, which the run user is in after building the World.
// Both endpoints answer administrators only.
//
// Replaces: TestMeta_UserSelf
func TestUserAccount_AdminReads_MembershipsAndActivities(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	rt := e.Runtime()
	world := fixture.SharedWorld(e)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		memberships := harness.Do[users.UserMembershipsOutput](s, actionUserMemberships, map[string]any{"user_id": rt.UserID, "type": "Namespace", "per_page": 100})
		owned := false
		for _, membership := range memberships.Memberships {
			if membership.SourceID == world.Group.ID && membership.SourceType == "Namespace" {
				owned = true
				if membership.AccessLevel != int64(gl.OwnerPermissions) {
					e.T.Errorf("the run user holds access level %d on the World's group %d, want owner (%d)", membership.AccessLevel, world.Group.ID, gl.OwnerPermissions)
				}
			}
		}
		if !owned {
			e.T.Errorf("the memberships of user %d do not name the World's group %d: %+v", rt.UserID, world.Group.ID, memberships.Memberships)
		}

		activities := harness.Do[users.UserActivitiesOutput](s, actionUserActivities, map[string]any{"per_page": 100})
		if !slices.ContainsFunc(activities.Activities, func(activity users.UserActivityOutput) bool { return activity.Username == rt.Username }) {
			e.T.Errorf("the activity feed does not name the run user %q, who pushed the World's commit: %+v", rt.Username, activities.Activities)
		}
	})
}

// TestUserStatus_SetAndReadBack_RoundTrips sets the run user's status on
// every surface and reads it back twice, as the current user's status and
// by user ID, holding the current-user lock and restoring the status that
// was there before.
//
// Replaces: TestMeta_UserSelf
func TestUserStatus_SetAndReadBack_RoundTrips(t *testing.T) {
	e := harness.New(t, harness.Locks(harness.LockCurrentUserState))
	rt := e.Runtime()
	restoreUserStatus(e)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		message := "e2e " + string(surface) + " " + e.RunID()

		set := harness.Do[users.StatusOutput](s, actionUserSetStatus, map[string]any{"emoji": statusEmoji, "message": message})
		if set.Emoji != statusEmoji || set.Message != message {
			e.T.Errorf("set_status answered emoji %q message %q, want %q %q", set.Emoji, set.Message, statusEmoji, message)
		}

		current := harness.Do[users.StatusOutput](s, actionUserCurrentStatus, nil)
		if current.Emoji != statusEmoji || current.Message != message {
			e.T.Errorf("current_user_status answered emoji %q message %q right after the set, want %q %q", current.Emoji, current.Message, statusEmoji, message)
		}

		got := harness.Do[users.StatusOutput](s, actionUserGetStatus, map[string]any{"user_id": rt.UserID})
		if got.Emoji != statusEmoji || got.Message != message {
			e.T.Errorf("get_status of user %d answered emoji %q message %q, want %q %q", rt.UserID, got.Emoji, got.Message, statusEmoji, message)
		}
	})
}

// restoreUserStatus reads the run user's status through client-go and
// registers putting it back, so a test that changes it leaves the account
// as it found it. A status that was empty is cleared, which is what setting
// an empty emoji and message does.
func restoreUserStatus(e *harness.Env) {
	e.T.Helper()

	before, _, err := e.Client().GL().Users.CurrentUserStatus(gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("reading the run user's status before changing it: %v", err)
	}
	e.Defer("the run user's status", func(ctx context.Context) error {
		_, _, restoreErr := e.Client().GL().Users.SetUserStatus(&gl.UserStatusOptions{
			Emoji:   new(before.Emoji),
			Message: new(before.Message),
		}, gl.WithContext(ctx))
		return restoreErr
	})
}

// TestUserAvatar_Upload_SetsTheRunUsersAvatar uploads a one-pixel PNG as the
// run user's avatar on every surface and checks the profile comes back with
// an avatar URL. GitLab keeps one avatar per account and offers no way to
// remove it, so the upload replaces whatever was there and stays.
//
// Replaces: TestIndividual_UserAvatarUpload
func TestUserAvatar_Upload_SetsTheRunUsersAvatar(t *testing.T) {
	e := harness.New(t, harness.Locks(harness.LockCurrentUserState))
	png := fixture.PNGBase64(e)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		uploaded := harness.Do[users.Output](s, actionUserUploadAvatar, map[string]any{"filename": avatarFilename, "content_base64": png})
		if uploaded.AvatarURL == "" {
			e.T.Errorf("upload_avatar answered %+v, want a profile carrying the new avatar's URL", uploaded)
		}
	})
}

// TestUserPAT_CurrentUser_MintsAK8sProxyToken mints a token for the run
// user on every surface, with the one scope GitLab lets an account grant
// itself, checks the answer carries the secret and names the account, and
// revokes it afterwards through client-go.
//
// Replaces: TestMeta_UserServiceAccounts
func TestUserPAT_CurrentUser_MintsAK8sProxyToken(t *testing.T) {
	e := harness.New(t)
	rt := e.Runtime()

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		name := e.Name("selfpat")

		minted := harness.Do[users.CurrentUserPATOutput](s, actionUserCreateCurrentUserPAT, map[string]any{
			"name": name, "scopes": []string{selfTokenScope}, "expires_at": time.Now().Add(selfTokenLifetime).Format(time.DateOnly),
		})
		if minted.ID == 0 || minted.Token == "" {
			e.T.Fatalf("create_current_user_pat answered %+v, want a token with an ID and its secret", minted)
		}
		e.Defer("token "+name, func(ctx context.Context) error { return revokePersonalToken(ctx, e, minted.ID) })
		if minted.Name != name || minted.UserID != rt.UserID || !slices.Contains(minted.Scopes, selfTokenScope) {
			e.T.Errorf("create_current_user_pat answered name %q user %d scopes %v, want %q %d [%s]", minted.Name, minted.UserID, minted.Scopes, name, rt.UserID, selfTokenScope)
		}
	})
}

// revokePersonalToken revokes a personal access token by ID through
// client-go, tolerating one already gone or already revoked, which is the
// state a test that revoked or rotated it leaves behind.
func revokePersonalToken(ctx context.Context, e *harness.Env, tokenID int64) error {
	_, err := e.Client().GL().PersonalAccessTokens.RevokePersonalAccessTokenByID(tokenID, gl.WithContext(ctx))
	if err != nil && !fixture.IsStatus(err, http.StatusNotFound) && !fixture.IsStatus(err, http.StatusBadRequest) {
		return err
	}
	return nil
}
