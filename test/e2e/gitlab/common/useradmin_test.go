//go:build e2e

// useradmin_test.go covers what an administrator does to another account:
// creates it, changes it, walks it through blocked, deactivated and banned
// and back, and deletes it; approves and rejects the sign-ups the setup
// script left pending; removes a provider identity; mints a runner; and is
// refused a two-factor reset on an account that never enrolled. Every state
// the server reports is read back through client-go, so the assertion is on
// GitLab's record and not only on the tool's echo of the request.

package common

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The polling a deletion takes: GitLab deletes a user from a background
// job, so the record outlives the answer that scheduled its removal.
const (
	userDeletionInterval = 2 * time.Second
	userDeletionWait     = 90 * time.Second
)

// What a user creation through the server is retried on, for the password
// GitLab occasionally refuses as commonly used.
const (
	userCreateAttempts   = 5
	userCreateRetryDelay = time.Second
)

// The user states GitLab reports, as the users API spells them.
const (
	userStateActive      = "active"
	userStateBlocked     = "blocked"
	userStateDeactivated = "deactivated"
	userStateBanned      = "banned"
)

// The identity a fixture user is created with for the identity scenario. A
// provider does not have to be configured on the instance for an identity
// naming it to exist on an account, which is the only way to seed one
// without an OAuth flow.
const identityProvider = "google_oauth2"

// The runner the runner scenario mints: an instance runner, which only an
// administrator may create.
const instanceRunnerType = "instance_type"

// TestUserAdmin_Lifecycle_CreatesChangesStateAndDeletes creates one user per
// surface through the server, gives it a bio, blocks and unblocks it,
// deactivates and activates it, bans and unbans it, and deletes it, reading
// GitLab's state after every transition. The deletions are watched for once
// the surfaces are done, since GitLab performs them from a background job,
// and the fixture's own deletion is the safety net behind the delete.
//
// Replaces: TestMeta_UserAdmin, TestIndividual_UserManagement
func TestUserAdmin_Lifecycle_CreatesChangesStateAndDeletes(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	var deleted []int64
	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		username := e.Name("usr")

		created := createUserThroughTheServer(e, s, username)
		if created.ID == 0 || created.Username != username {
			e.T.Fatalf("user create answered %+v, want an account named %q with an ID", created, username)
		}
		e.Defer("user "+username, func(ctx context.Context) error {
			return fixture.DeleteUser(ctx, e.Client(), created.ID)
		})
		id := map[string]any{"user_id": created.ID}
		if state := userState(e, created.ID); state != userStateActive {
			e.T.Errorf("a user created with skip_confirmation is %q, want %q", state, userStateActive)
		}

		bio := "modified by the e2e suite on the " + string(surface) + " surface"
		modified := harness.Do[users.Output](s, actionUserModify, withParams(id, map[string]any{"bio": bio}))
		if modified.ID != created.ID || modified.Bio != bio {
			e.T.Errorf("user modify answered id %d bio %q, want %d %q", modified.ID, modified.Bio, created.ID, bio)
		}

		assertAdminTransition(e, s, actionUserBlock, created.ID, "blocked", userStateBlocked)
		assertAdminTransition(e, s, actionUserUnblock, created.ID, "unblocked", userStateActive)
		assertAdminTransition(e, s, actionUserDeactivate, created.ID, "deactivated", userStateDeactivated)
		assertAdminTransition(e, s, actionUserActivate, created.ID, "activated", userStateActive)
		assertAdminTransition(e, s, actionUserBan, created.ID, "banned", userStateBanned)
		assertAdminTransition(e, s, actionUserUnban, created.ID, "unbanned", userStateActive)

		removed := harness.Do[users.DeleteOutput](s, actionUserDelete, id)
		if !removed.Deleted || removed.UserID != created.ID {
			e.T.Errorf("user delete answered %+v, want deleted=true for user %d", removed, created.ID)
		}
		deleted = append(deleted, created.ID)
	})
	awaitUserRemovals(e, deleted)
}

// assertAdminTransition runs one state-changing action on a user, checks the
// confirmation names the action and the user, and reads the state GitLab
// holds afterwards.
func assertAdminTransition(e *harness.Env, s *harness.Session, id harness.ActionID, userID int64, wantAction, wantState string) {
	e.T.Helper()

	out := harness.Do[users.AdminActionOutput](s, id, map[string]any{"user_id": userID})
	if !out.Success || out.Action != wantAction || out.UserID != userID {
		e.T.Errorf("%s answered %+v, want success with action %q for user %d", id, out, wantAction, userID)
	}
	if state := userState(e, userID); state != wantState {
		e.T.Errorf("after %s GitLab holds user %d as %q, want %q", id, userID, state, wantState)
	}
}

// userState reads a user's state through client-go, beside the server under
// test.
func userState(e *harness.Env, userID int64) string {
	e.T.Helper()

	user, _, err := e.Client().GL().Users.GetUser(userID, &gl.GetUserOptions{}, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("reading user %d back: %v", userID, err)
		return ""
	}
	return user.State
}

// weakPasswordRefusal reports the one refusal a fresh password fixes.
//
// It is deliberately narrower than fixture.UserCreateRetryable, which also
// retries the transient failures. The create is not idempotent: it carries a
// username, so an attempt GitLab committed before the answer was lost would
// meet its own account on the next one and be refused for the name. GitLab
// judges the password before it writes anything, so this refusal is the one
// case where nothing was created and the same username is free to send again.
func weakPasswordRefusal(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "commonly used")
}

// createUserThroughTheServer creates one account through the server, minting a
// fresh password on every attempt.
//
// GitLab judges a password against its own weak-password rules, and a random
// one occasionally lands on something it refuses; the fixture builder mints a
// new one per attempt for exactly that reason (fixture.UserCreateRetryable),
// and a scenario that creates its user through the server rather than through
// the builder needs the same treatment or it fails before its subject begins.
func createUserThroughTheServer(e *harness.Env, s *harness.Session, username string) users.Output {
	e.T.Helper()

	created, err := harness.Retry(e.Ctx, e.T, "user create "+username, userCreateAttempts, userCreateRetryDelay,
		func(int) (users.Output, bool, string, error) {
			out, tryErr := harness.Try[users.Output](s, actionUserCreate, map[string]any{
				"email":             username + "@e2e-test.invalid",
				"name":              "E2E " + username,
				"username":          username,
				"password":          "Pw-" + rand.Text(),
				"skip_confirmation": true,
			})
			return out, weakPasswordRefusal(tryErr), "the random password was refused as commonly used", tryErr
		})
	if err != nil {
		e.T.Fatalf("creating the user %q through the server: %v", username, err)
	}
	return created
}

// awaitUserRemovals waits for the accounts the surfaces deleted to leave the
// instance, and writes down what GitLab still holds when they have not left
// within the window.
//
// The wait is one for all the surfaces rather than one each, and it runs after
// the last of them, so the accounts deleted earlier are already queued while it
// waits. An expiry with some of them gone is logged rather than failed on:
// GitLab removes a user from a background job, so what that measures is the
// depth of the instance's job queue, which is not the thing under test. An
// expiry with none of them gone is failed on, because a queue that moved for
// nobody is not a slow queue: it is a delete the instance accepted and never
// performed, and nothing else in these tests would see it.
func awaitUserRemovals(e *harness.Env, userIDs []int64) {
	e.T.Helper()

	// GitLab removes a rejected or deleted account in a background job, so the
	// queue is drained before the wait rather than polled through: on a Docker
	// instance carrying a whole suite's worth of jobs the removal can sit
	// behind them for longer than any window this test would be right to keep
	// open. This is the same drain the fixtures do before a readiness wait.
	fixture.DrainSidekiq(e.Ctx, e.Client())

	remaining := slices.Clone(userIDs)
	err := harness.Poll(e.Ctx, userDeletionInterval, userDeletionWait, func() (bool, string, error) {
		remaining = slices.DeleteFunc(remaining, func(userID int64) bool { return userIsGone(e, userID) })
		if len(remaining) == 0 {
			return true, "", nil
		}
		return false, fmt.Sprintf("users %v still answer", remaining), nil
	})
	if err == nil {
		return
	}
	// How many are left is what separates a slow queue from a delete that does
	// nothing. Some gone and some pending is the queue, and that is not the
	// thing under test; none gone at all is the delete, and this is the only
	// place a test would see that.
	if len(remaining) == len(userIDs) {
		// What state they are in decides whose defect this is, and nothing
		// else in the suite can say it. gitlab_reject_user tells a model the
		// rejection "permanently deletes the pending user", so an account that
		// is merely blocked or still pending makes that description false,
		// while one that is simply slow to go makes this a wait that is too
		// short. The states are read here rather than guessed at.
		e.T.Errorf("none of the deleted users %v had left the instance after %s; the deletes were accepted and "+
			"nothing was removed. Their states now: %s. Error: %v",
			remaining, userDeletionWait, describeUserStates(e, remaining), err)
		return
	}
	e.T.Logf("the background deletion of users %v had not landed within %s, with %d of %d already gone: %v",
		remaining, userDeletionWait, len(userIDs)-len(remaining), len(userIDs), err)
}

// userIsGone reports whether a user answers 404, which is where a deletion
// GitLab performs from a background job ends.
// describeUserStates renders what the instance now holds for each user, for a
// failure that has to say whether an account survived a rejection and in what
// condition. It never fails the test: it is called from one that has already
// failed, and a read that cannot answer says so in place of a state.
func describeUserStates(e *harness.Env, userIDs []int64) string {
	parts := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		user, _, err := e.Client().GL().Users.GetUser(userID, &gl.GetUserOptions{}, gl.WithContext(e.Ctx))
		switch {
		case err != nil:
			parts = append(parts, fmt.Sprintf("%d=unreadable(%v)", userID, err))
		case user == nil:
			parts = append(parts, fmt.Sprintf("%d=no user in the answer", userID))
		default:
			parts = append(parts, fmt.Sprintf("%d=%s", userID, user.State))
		}
	}
	return strings.Join(parts, " ")
}

func userIsGone(e *harness.Env, userID int64) bool {
	_, _, err := e.Client().GL().Users.GetUser(userID, &gl.GetUserOptions{}, gl.WithContext(e.Ctx))
	return err != nil && fixture.IsStatus(err, http.StatusNotFound)
}

// TestUserAdmin_PendingSignups_ApprovesOneAndRejectsTheOther consumes the
// pair of users the setup script left in the pending-approval state for
// this package and surface: approves one, which GitLab then reports active,
// and rejects the other, which GitLab then deletes. The approved user is
// removed afterwards; a rejection removes its own.
//
// Replaces: TestIndividual_UserApproveReject
func TestUserAdmin_PendingSignups_ApprovesOneAndRejectsTheOther(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	var rejected []int64
	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		approveID, rejectID := fixture.PendingApprovalUsers(e, surface)
		e.Defer("approved user", func(ctx context.Context) error {
			return fixture.DeleteUser(ctx, e.Client(), approveID)
		})

		approved := harness.Do[users.AdminActionOutput](s, actionUserApprove, map[string]any{"user_id": approveID})
		if !approved.Success || approved.Action != "approved" || approved.UserID != approveID {
			e.T.Errorf("user approve answered %+v, want success with action approved for user %d", approved, approveID)
		}
		if state := userState(e, approveID); state != userStateActive {
			e.T.Errorf("after the approval GitLab holds user %d as %q, want %q", approveID, state, userStateActive)
		}

		refused := harness.Do[users.AdminActionOutput](s, actionUserReject, map[string]any{"user_id": rejectID})
		if !refused.Success || refused.Action != "rejected" || refused.UserID != rejectID {
			e.T.Errorf("user reject answered %+v, want success with action rejected for user %d", refused, rejectID)
		}
		rejected = append(rejected, rejectID)
	})
	awaitUserRemovals(e, rejected)
}

// TestUserAdmin_DeleteIdentity_RemovesAProviderIdentity creates a user
// carrying a provider identity, removes it through the server on each
// surface and reads back through client-go that the account carries no
// identity for that provider any more.
//
// Replaces: TestIndividual_UserDeleteIdentity
func TestUserAdmin_DeleteIdentity_RemovesAProviderIdentity(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		user := fixture.NewUser(e, "ident", withIdentity(identityProvider, e.Name("uid")))
		if !hasIdentity(e, user.ID, identityProvider) {
			e.T.Fatalf("user %d was created without the %s identity the scenario removes", user.ID, identityProvider)
		}

		deleted := harness.Do[users.DeleteUserIdentityOutput](s, actionUserDeleteIdentity, map[string]any{"user_id": user.ID, "provider": identityProvider})
		if !deleted.Deleted || deleted.Provider != identityProvider || deleted.UserID != user.ID {
			e.T.Errorf("delete_identity answered %+v, want deleted=true for provider %s of user %d", deleted, identityProvider, user.ID)
		}
		if hasIdentity(e, user.ID, identityProvider) {
			e.T.Errorf("user %d still carries a %s identity after its delete", user.ID, identityProvider)
		}
	})
}

// withIdentity creates the user with an identity for the given provider,
// which the admin API accepts whether or not the instance configures the
// provider.
func withIdentity(provider, externUID string) fixture.UserOption {
	return func(opts *gl.CreateUserOptions) {
		opts.Provider = new(provider)
		opts.ExternUID = new(externUID)
	}
}

// hasIdentity reads a user through client-go and reports whether one of its
// identities names the provider.
func hasIdentity(e *harness.Env, userID int64, provider string) bool {
	e.T.Helper()

	user, _, err := e.Client().GL().Users.GetUser(userID, &gl.GetUserOptions{}, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("reading user %d back: %v", userID, err)
		return false
	}
	for _, identity := range user.Identities {
		if identity.Provider == provider {
			return true
		}
	}
	return false
}

// TestUserAdmin_CreateRunner_MintsAnInstanceRunner creates an instance
// runner bound to the run user on every surface, checks the answer carries
// the runner and its authentication token, reads the runner back through
// client-go, and removes it afterwards. The runner is never registered as
// a process, so the stack's own runner is untouched.
//
// Replaces: TestIndividual_UserCreateRunner
func TestUserAdmin_CreateRunner_MintsAnInstanceRunner(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		description := e.Name("runner")

		minted := harness.Do[users.UserRunnerOutput](s, actionUserCreateRunner, map[string]any{"runner_type": instanceRunnerType, "description": description})
		if minted.ID == 0 || minted.Token == "" {
			e.T.Fatalf("create_runner answered %+v, want a runner with an ID and its authentication token", minted)
		}
		e.Defer("runner "+description, func(ctx context.Context) error {
			_, err := e.Client().GL().Runners.RemoveRunner(minted.ID, gl.WithContext(ctx))
			if err != nil && !fixture.IsStatus(err, http.StatusNotFound) {
				return err
			}
			return nil
		})

		details, _, err := e.Client().GL().Runners.GetRunnerDetails(minted.ID, gl.WithContext(e.Ctx))
		if err != nil {
			e.T.Fatalf("reading runner %d back: %v", minted.ID, err)
		}
		if details.Description != description || details.RunnerType != instanceRunnerType {
			e.T.Errorf("runner %d reads back as %q of type %q, want %q of type %q", minted.ID, details.Description, details.RunnerType, description, instanceRunnerType)
		}
	})
}

// TestUserAdmin_DisableTwoFactor_RefusesAnAccountWithoutIt asks the server
// to disable two-factor authentication for a user who never enrolled, on
// every surface, and checks GitLab's refusal reaches the caller with its
// reason. Enrollment cannot be done through the API, so the refusal is the
// only answer this action can be shown to give end to end.
//
// Replaces: TestIndividual_UserDisableTwoFactor
func TestUserAdmin_DisableTwoFactor_RefusesAnAccountWithoutIt(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.User {
		return fixture.NewUser(e, "twofa")
	}, func(e *harness.Env, surface harness.Surface, user fixture.User) {
		s := e.On(surface)
		refusal := harness.ExpectToolError(s, actionUserDisableTwoFactor, map[string]any{"user_id": user.ID}, "not enabled")
		e.T.Logf("disable_two_factor on user %d was refused: %s", user.ID, firstLine(refusal))
	})
}
