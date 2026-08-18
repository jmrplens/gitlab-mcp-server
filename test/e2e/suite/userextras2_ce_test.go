//go:build e2e && !enterprise

// userextras2_ce_test.go covers previously unexercised admin-level
// user-domain actions on disposable users: GPG keys for another user
// (user.add_gpg_key_for_user / user.get_gpg_key_for_user /
// user.delete_gpg_key_for_user), identity deletion (user.delete_identity),
// runner creation (user.create_runner), pending-signup approval and
// rejection (user.approve / user.reject), and two-factor disabling
// (user.disable_two_factor).
//
// All tests here need an admin token and mutate instance-level state, so
// they are gated to Docker mode where the GitLab instance is disposable.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/usergpgkeys"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/users"
)

// userExtrasGPGKeyTwo is a second fixed ASCII-armored OpenPGP public key,
// distinct from userExtrasGPGKeyOne so current-user and for-user GPG tests
// never collide on GitLab's instance-unique fingerprint constraint.
const userExtrasGPGKeyTwo = `-----BEGIN PGP PUBLIC KEY BLOCK-----

xsBNBGpLgFsBCADRlxIKnNxKR1ZQ1ytbA6hLhyEkiY+BZpAm4CBP344LYXGp3eMX
pzx7NPZRU9a6juW7fEpuu2TMjyCGaAVpb5IuT5brSwt7YuevfEwfQXUPSLsVRIxR
a0EIvL1rsGVxUYXvupee+yLcq7uBzZUirSH4dCJ8KM0ODU6QHbN0tdVP8OBzAFz5
+L6NrMzWwgt0ddwyOgqAfnz+xnxAyJIueDog1almEFRimRJT6hq9dHQaCEqXASW6
l46EgC6hLdD6vUcCzOEXUeeV9IeRODYPcvekXL1LhfAekZTwyxSidFtIK8bibeZE
jHO8d25QUn1mFIVQqEJtha1HsUqPc47WdYZJABEBAAHNR0dpdExhYiBNQ1AgRTJF
IEZpeHR1cmUgVHdvIChlMmUgdGVzdCBmaXh0dXJlKSA8ZTJlLWdwZy10d29AZXhh
bXBsZS5jb20+wsBiBBMBCAAWBQJqS4BbCRB6n7GBlRlFigIbAwIZAQAAYAEIAJPr
rGPtswA+Jj0XKlDOSpCGdcyFyHMv5qCHjyK3HNkVG/RzGb/hia3wocXhgnajCeYT
Wu95LL0CogvNETbnVRaHKAdtrz8pTsjohmEp2QwyxqUiHnhACZy7+C5mz6bvW6Er
h/L3lhkhKaqQwXYofHdW0tFc+jiVThJCuG/fZMDwp50wQGJ5h36vrP+KuGmMdQvP
vFhzJpA52Dl8LuZIhu72J90iLJQwtn15NLxZXIseLVt+Bhw5XdHjoqbDvBzz+o99
v9bOFqaeYqjm443KG3+JH2ExLu+Lo0CRohMq/xe+ULCIuhfW2ySUE/1FRJPTRU3K
QGm3zlMJLlvrXbpNhzbOwE0EakuAWwEIANSVj+N0UNn5dZuTzxdSQy0FVCCd76rX
Kjmd0pq2Et+b/Ks5llQXETs5c/9kortaC4B3vL7twp43BKa3aoFpot+iUao27Q7j
J1eVyvLRlHuCy2A7emIBzArTGBEY1fH1JSiP9n+20fdH6TkU3MwpeSwL96BOEWER
JRAejHsqLE9n6Oae/RNKJyQ6TZon4iFbNQ1DqAi2OdDV/0R2x9QTISeIr8Qw1THy
nEKeIWsHb+pO+Gw08YSgemtH8S16zCmPH9LMyemNZizSzmA0DFwU3k6iBRFhf8M4
7W7fV7ZxS24/2gdT04ivgJVJR9PfbvNE6DoR2eX491jJXF8A71Abr88AEQEAAcLA
XwQYAQgAEwUCakuAWwkQep+xgZUZRYoCGwwAAAPZCAB++S0xWjnHGOrWjW9Pw3MI
tOCpP27hf6CH4Duyn2xcFqVcDNpx1BifJr/yyH4qgj+PQ9qI4WYy0rSnH5usN+/f
Mb2kJgM+BJLn5XG4Hl2CxEBgbsiFvUbCaj8CqeUq8MrwwtW8bZL+265FVtB0aW8k
F1ydgBMGFOJ/kdNFwuVueoQ6KFYz/K3MB2YNxY+BSa1JicBuwtGUaKm+gZnLw1Nm
uUrVRE4VBqnv+wZDrkJ6GkR2Vk+aLPDuT/QaizkrLN49r327ulUm+R9+dVau9B9M
O36iCpZSYroKDfO/aWyxgg1z89r7Bstm+pcx8XHA2NhF9URtvBP6MmLR6bg9tv/n
=/gg+
-----END PGP PUBLIC KEY BLOCK-----`

// userExtrasCreateUser creates a disposable user through the raw admin API
// and registers a best-effort deletion cleanup. The optional mutate callback
// lets callers attach extras (e.g. a provider identity) without a second
// helper per variant.
func userExtrasCreateUser(ctx context.Context, t *testing.T, prefix string, mutate func(*gl.CreateUserOptions)) *gl.User {
	t.Helper()
	uname := uniqueName(prefix)
	opts := &gl.CreateUserOptions{
		Email:            new(uname + "@e2e-test.local"),
		Name:             new("E2E " + uname),
		Username:         new(uname),
		Password:         new("E2eT!Gx9K#p2mNq$8BcZ"),
		SkipConfirmation: new(true),
	}
	if mutate != nil {
		mutate(opts)
	}
	user, _, err := sess.glClient.GL().Users.CreateUser(opts, gl.WithContext(ctx))
	requireNoError(t, err, "create disposable user")
	t.Cleanup(func() { //nolint:contextcheck // Cleanup runs after the test context is canceled and owns its own timeout.
		cctx, cancel := cleanupContext(defaultCleanupTimeout)
		defer cancel()
		// Rejected users are already gone; deletion failures are tolerable on
		// the disposable Docker instance.
		_, _ = sess.glClient.GL().Users.DeleteUser(user.ID, gl.WithContext(cctx))
	})
	return user
}

// TestIndividual_UserGPGKeysForUser exercises user.add_gpg_key_for_user,
// user.get_gpg_key_for_user, and user.delete_gpg_key_for_user through
// individual MCP tools against a disposable user.
//
// The test creates a throwaway user via the admin API, attaches the second
// embedded GPG fixture key to it, reads the key back, and deletes it. All
// three actions require an admin token.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: individual. Admin token required.
func TestIndividual_UserGPGKeysForUser(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if !isDockerMode() {
		t.Skip("disposable-user GPG coverage requires the Docker instance (admin user creation + instance-unique fingerprints)")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		user := userExtrasCreateUser(ctx, t, "usr-gpg", nil)
		var keyID int64

		t.Run("AddForUser", func(t *testing.T) {
			out, err := callToolOn[usergpgkeys.Output](ctx, sess.individual, "gitlab_add_gpg_key_for_user", usergpgkeys.AddForUserInput{
				UserID: user.ID,
				Key:    userExtrasGPGKeyTwo,
			})
			requireNoError(t, err, "add gpg key for user")
			requireTruef(t, out.ID > 0, "expected GPG key ID > 0")
			keyID = out.ID
			t.Logf("Added GPG key %d for user %d", keyID, user.ID)
		})

		t.Run("GetForUser", func(t *testing.T) {
			requireTruef(t, keyID > 0, "keyID not set")
			out, err := callToolOn[usergpgkeys.Output](ctx, sess.individual, "gitlab_get_gpg_key_for_user", usergpgkeys.GetForUserInput{
				UserID: user.ID,
				KeyID:  keyID,
			})
			requireNoError(t, err, "get gpg key for user")
			requireTruef(t, out.ID == keyID, "expected key ID %d, got %d", keyID, out.ID)
		})

		t.Run("DeleteForUser", func(t *testing.T) {
			requireTruef(t, keyID > 0, "keyID not set")
			err := callToolVoidOn(ctx, sess.individual, "gitlab_delete_gpg_key_for_user", usergpgkeys.DeleteForUserInput{
				UserID: user.ID,
				KeyID:  keyID,
			})
			requireNoError(t, err, "delete gpg key for user")
			t.Logf("Deleted GPG key %d for user %d", keyID, user.ID)
		})
	})
}

// TestIndividual_UserDeleteIdentity exercises user.delete_identity through
// the gitlab_delete_user_identity individual tool.
//
// The test creates a disposable user carrying a google_oauth2 provider
// identity (attached at creation time via the admin API, the only way to
// seed an identity without a real OAuth flow), then deletes that identity
// via MCP and asserts the confirmation payload.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: individual. Admin token required.
func TestIndividual_UserDeleteIdentity(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if !isDockerMode() {
		t.Skip("identity coverage requires admin user creation on the Docker instance")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		const provider = "google_oauth2"
		externUID := uniqueName("uid")
		user := userExtrasCreateUser(ctx, t, "usr-idn", func(opts *gl.CreateUserOptions) {
			opts.Provider = new(provider)
			opts.ExternUID = new(externUID)
		})

		out, err := callToolOn[users.DeleteUserIdentityOutput](ctx, sess.individual, "gitlab_delete_user_identity", users.DeleteUserIdentityInput{
			UserID:   user.ID,
			Provider: provider,
		})
		requireNoError(t, err, "delete user identity")
		requireTruef(t, out.Deleted, "expected deleted=true")
		requireTruef(t, out.Provider == provider, "expected provider %q, got %q", provider, out.Provider)
		t.Logf("Deleted identity %s for user %d", provider, user.ID)
	})
}

// TestIndividual_UserCreateRunner exercises user.create_runner through the
// gitlab_create_user_runner individual tool.
//
// The test creates an instance-type runner bound to the current admin user
// and asserts a runner ID and authentication token are returned. The runner
// is only created, never registered as a process, and is removed afterwards
// via the raw admin API so the fixture runner of the Docker compose stack is
// never touched.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: individual. Admin token required.
func TestIndividual_UserCreateRunner(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if !isDockerMode() {
		t.Skip("instance runner creation requires admin on the Docker instance")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		out, err := callToolOn[users.UserRunnerOutput](ctx, sess.individual, "gitlab_create_user_runner", users.CreateUserRunnerInput{
			RunnerType:  "instance_type",
			Description: "e2e throwaway runner (never registered)",
		})
		requireNoError(t, err, "create user runner")
		requireTruef(t, out.ID > 0, "expected runner ID > 0")
		requireTruef(t, out.Token != "", "expected runner authentication token")
		t.Logf("Created user runner %d", out.ID)

		_, err = sess.glClient.GL().Runners.RemoveRunner(out.ID, gl.WithContext(ctx))
		requireNoError(t, err, "remove throwaway runner")
	})
}

// TestIndividual_UserApproveReject exercises user.approve and user.reject
// through the gitlab_approve_user and gitlab_reject_user individual tools.
//
// The test consumes the two users setup-gitlab.sh seeds in the
// blocked_pending_approval state (created via the API, flipped through the
// Rails console — the state a real pending signup would have, which the API
// alone cannot produce). It approves one and rejects the other; rejection
// deletes its user and the approved user is removed in cleanup. Docker mode
// only: the seeded fixtures exist only on the disposable instance.
//
// Build tag: e2e && !enterprise. Mode: Docker. Surface: individual.
func TestIndividual_UserApproveReject(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if !isDockerMode() {
		t.Skip("signup-approval setting flip is only safe on the disposable Docker instance")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin, CapabilityInstanceGlobal}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		// setup-gitlab.sh seeds two users flipped to blocked_pending_approval
		// through the Rails console — the state a real pending signup would
		// have, which the API alone cannot produce (admin-created users are
		// born active and the require-admin-approval setting only affects
		// self-signups). Without the seeded IDs the test documents that with
		// a skip.
		approveID, approveErr := strconv.ParseInt(os.Getenv("E2E_PENDING_APPROVE_USER_ID"), 10, 64)
		rejectID, rejectErr := strconv.ParseInt(os.Getenv("E2E_PENDING_REJECT_USER_ID"), 10, 64)
		if approveErr != nil || rejectErr != nil {
			t.Skip("E2E_PENDING_APPROVE_USER_ID/E2E_PENDING_REJECT_USER_ID not seeded: pending signups cannot be produced via the API alone")
		}
		t.Cleanup(func() {
			cctx, ccancel := cleanupContext(defaultCleanupTimeout)
			defer ccancel()
			// The rejected user is already deleted by the reject action;
			// the approved user becomes active and is removed here.
			_, _ = sess.glClient.GL().Users.DeleteUser(approveID, gl.WithContext(cctx))
		})

		t.Run("Approve", func(t *testing.T) {
			out, err := callToolOn[users.AdminActionOutput](ctx, sess.individual, "gitlab_approve_user", users.AdminActionInput{
				UserID: approveID,
			})
			requireNoError(t, err, "approve user")
			requireTruef(t, out.Success, "approve should succeed")
			t.Logf("Approved user %d", approveID)
		})

		t.Run("Reject", func(t *testing.T) {
			out, err := callToolOn[users.AdminActionOutput](ctx, sess.individual, "gitlab_reject_user", users.AdminActionInput{
				UserID: rejectID,
			})
			requireNoError(t, err, "reject user")
			requireTruef(t, out.Success, "reject should succeed")
			t.Logf("Rejected user %d (rejection deletes the user)", rejectID)
		})
	})
}

// TestIndividual_UserDisableTwoFactor exercises user.disable_two_factor
// through the gitlab_disable_two_factor individual tool.
//
// TOTP enrollment cannot be performed through the API, so the test creates a
// disposable user without 2FA and asserts the endpoint returns its
// documented clean error ("Two-factor authentication is not enabled for this
// user", HTTP 400) instead of a transport failure. This exercises the full
// MCP round-trip for the action with the only state reachable in E2E.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: individual. Admin token required.
func TestIndividual_UserDisableTwoFactor(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if !isDockerMode() {
		t.Skip("2FA disable coverage requires admin user creation on the Docker instance")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		user := userExtrasCreateUser(ctx, t, "usr-2fa", nil)

		_, err := callToolOn[users.AdminActionOutput](ctx, sess.individual, "gitlab_disable_two_factor", users.AdminActionInput{
			UserID: user.ID,
		})
		requireTruef(t, err != nil, "expected error: user %d has no 2FA enrollment", user.ID)
		t.Logf("disable_two_factor returned the expected error for a user without 2FA: %v", err)
	})
}
