//go:build e2e && !enterprise

// access_extras2_ce_test.go covers previously unexercised access-domain
// actions that need an admin token and second authenticated identities:
// access requests from non-member users (access.request_project /
// access.request_group / access.request_list_group /
// access.approve_project / access.approve_group / access.deny_project /
// access.deny_group), personal access token administration on disposable
// tokens (access.token_personal_get / token_personal_rotate /
// token_personal_revoke), personal token self-service operations
// (access.token_personal_rotate_self / token_personal_revoke_self), and
// instance deploy keys (access.deploy_key_add_instance).
//
// Every test is gated to Docker mode: they create throwaway users and PATs
// through the admin API and must never run against a real instance or touch
// the suite's own credentials.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/accessrequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/accesstokens"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/deploykeys"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// accessExtrasCreateUser creates a disposable non-member user via the admin
// API, reusing the user-domain helper so cleanup semantics stay identical.
func accessExtrasCreateUser(ctx context.Context, t *testing.T, prefix string) *gl.User {
	t.Helper()
	return userExtrasCreateUser(ctx, t, prefix, nil)
}

// accessExtrasCreatePAT issues a disposable personal access token for the
// given user through the admin API. All token operations in this file run
// against tokens minted here — never against the suite's own credential.
func accessExtrasCreatePAT(ctx context.Context, t *testing.T, userID int64, prefix string) *gl.PersonalAccessToken {
	t.Helper()
	expiry := accessExtrasExpiry()
	pat, _, err := sess.glClient.GL().Users.CreatePersonalAccessToken(userID, &gl.CreatePersonalAccessTokenOptions{
		Name:      new(uniqueName(prefix)),
		Scopes:    &[]string{"api"},
		ExpiresAt: &expiry,
	}, gl.WithContext(ctx))
	requireNoError(t, err, "create disposable personal access token")
	requireTruef(t, pat.Token != "", "expected PAT value on creation")
	return pat
}

// TestIndividual_AccessRequests exercises the full access-request lifecycle
// through individual MCP tools: request_project, request_group,
// request_list_group, approve_project, approve_group, deny_project, and
// deny_group.
//
// Two disposable users are created via the admin API, each with its own PAT
// and its own in-process MCP session, because access requests must originate
// from authenticated non-members. The target project and group are made
// public with access requests enabled so the requesters can see them. The
// suite user (owner) then lists the group requests and approves the first
// user while denying the second on both scopes.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: individual. Admin token required.
func TestIndividual_AccessRequests(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured (group fixture)")
	}
	if !isDockerMode() {
		t.Skip("access-request coverage creates admin-provisioned second users (Docker instance only)")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(e2e *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		grp := CreateGroupMeta(ctx, e2e, sess.meta, "areq")
		proj := CreateProject(ctx, e2e, sess.individual)
		groupID := toolutil.StringOrInt(grp.gidStr())

		// Non-members can only request access to targets they can see, and
		// only when the setting allows it.
		_, _, cfgErr := sess.glClient.GL().Projects.EditProject(proj.ID, &gl.EditProjectOptions{
			Visibility:           new(gl.PublicVisibility),
			RequestAccessEnabled: new(true),
		}, gl.WithContext(ctx))
		requireNoError(t, cfgErr, "make project public with access requests enabled")
		_, _, cfgErr = sess.glClient.GL().Groups.UpdateGroup(grp.ID, &gl.UpdateGroupOptions{
			Visibility:           new(gl.PublicVisibility),
			RequestAccessEnabled: new(true),
		}, gl.WithContext(ctx))
		requireNoError(t, cfgErr, "make group public with access requests enabled")

		userApprove := accessExtrasCreateUser(ctx, t, "areq-a")
		patApprove := accessExtrasCreatePAT(ctx, t, userApprove.ID, "areq-a")
		sessionApprove := accessExtrasStartSession(t, "areq-a", patApprove.Token)

		userDeny := accessExtrasCreateUser(ctx, t, "areq-d")
		patDeny := accessExtrasCreatePAT(ctx, t, userDeny.ID, "areq-d")
		sessionDeny := accessExtrasStartSession(t, "areq-d", patDeny.Token)

		t.Run("RequestProject", func(t *testing.T) {
			for _, requester := range []struct {
				name    string
				session *mcp.ClientSession
			}{
				{"approvee", sessionApprove},
				{"denyee", sessionDeny},
			} {
				t.Run(requester.name, func(t *testing.T) {
					out, err := callToolOn[accessrequests.Output](ctx, requester.session, "gitlab_access_request_request_project", accessrequests.RequestProjectInput{
						ProjectID: proj.pidOf(),
					})
					requireNoError(t, err, "request project access ("+requester.name+")")
					requireTruef(t, out.ID > 0, "expected requester user ID in access request")
				})
			}
			t.Log("Both users requested project access")
		})

		t.Run("RequestGroup", func(t *testing.T) {
			for _, requester := range []struct {
				name    string
				session *mcp.ClientSession
			}{
				{"approvee", sessionApprove},
				{"denyee", sessionDeny},
			} {
				t.Run(requester.name, func(t *testing.T) {
					out, err := callToolOn[accessrequests.Output](ctx, requester.session, "gitlab_access_request_request_group", accessrequests.RequestGroupInput{
						GroupID: groupID,
					})
					requireNoError(t, err, "request group access ("+requester.name+")")
					requireTruef(t, out.ID > 0, "expected requester user ID in access request")
				})
			}
			t.Log("Both users requested group access")
		})

		t.Run("ListGroup", func(t *testing.T) {
			out, err := callToolOn[accessrequests.ListOutput](ctx, sess.individual, "gitlab_access_request_list_group", accessrequests.ListGroupInput{
				GroupID: groupID,
			})
			requireNoError(t, err, "list group access requests")
			requireTruef(t, len(out.AccessRequests) >= 2, "expected at least 2 pending group access requests, got %d", len(out.AccessRequests))
			t.Logf("Listed %d group access requests", len(out.AccessRequests))
		})

		t.Run("ApproveProject", func(t *testing.T) {
			out, err := callToolOn[accessrequests.Output](ctx, sess.individual, "gitlab_access_request_approve_project", accessrequests.ApproveProjectInput{
				ProjectID:   proj.pidOf(),
				UserID:      userApprove.ID,
				AccessLevel: 30,
			})
			requireNoError(t, err, "approve project access request")
			requireTruef(t, out.AccessLevel == 30, "expected granted access level 30, got %d", out.AccessLevel)
			t.Logf("Approved project access for user %d", userApprove.ID)
		})

		t.Run("DenyProject", func(t *testing.T) {
			err := callToolVoidOn(ctx, sess.individual, "gitlab_access_request_deny_project", accessrequests.DenyProjectInput{
				ProjectID: proj.pidOf(),
				UserID:    userDeny.ID,
			})
			requireNoError(t, err, "deny project access request")
			t.Logf("Denied project access for user %d", userDeny.ID)
		})

		t.Run("ApproveGroup", func(t *testing.T) {
			out, err := callToolOn[accessrequests.Output](ctx, sess.individual, "gitlab_access_request_approve_group", accessrequests.ApproveGroupInput{
				GroupID:     groupID,
				UserID:      userApprove.ID,
				AccessLevel: 30,
			})
			requireNoError(t, err, "approve group access request")
			requireTruef(t, out.AccessLevel == 30, "expected granted access level 30, got %d", out.AccessLevel)
			t.Logf("Approved group access for user %d", userApprove.ID)
		})

		t.Run("DenyGroup", func(t *testing.T) {
			err := callToolVoidOn(ctx, sess.individual, "gitlab_access_request_deny_group", accessrequests.DenyGroupInput{
				GroupID: groupID,
				UserID:  userDeny.ID,
			})
			requireNoError(t, err, "deny group access request")
			t.Logf("Denied group access for user %d", userDeny.ID)
		})
	})
}

// TestIndividual_PersonalAccessTokenAdminOps exercises
// access.token_personal_get, access.token_personal_rotate, and
// access.token_personal_revoke through individual MCP tools.
//
// A disposable user and PAT are provisioned via the admin API; the suite's
// admin session then introspects the token by ID, rotates it (which mints a
// new token ID and invalidates the old one), and revokes the rotated
// generation. The suite's own credential is never involved.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: individual. Admin token required.
func TestIndividual_PersonalAccessTokenAdminOps(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if !isDockerMode() {
		t.Skip("PAT admin coverage provisions disposable users and tokens (Docker instance only)")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		user := accessExtrasCreateUser(ctx, t, "pat-adm")
		pat := accessExtrasCreatePAT(ctx, t, user.ID, "pat-adm")
		latestTokenID := pat.ID

		t.Run("Get", func(t *testing.T) {
			out, err := callToolOn[accesstokens.Output](ctx, sess.individual, "gitlab_personal_access_token_get", accesstokens.PersonalGetInput{
				TokenID: pat.ID,
			})
			requireNoError(t, err, "get personal access token")
			requireTruef(t, out.ID == pat.ID, "expected token ID %d, got %d", pat.ID, out.ID)
			requireTruef(t, out.UserID == user.ID, "expected token owner %d, got %d", user.ID, out.UserID)
		})

		t.Run("Rotate", func(t *testing.T) {
			out, err := callToolOn[accesstokens.Output](ctx, sess.individual, "gitlab_personal_access_token_rotate", accesstokens.PersonalRotateInput{
				TokenID: pat.ID,
			})
			requireNoError(t, err, "rotate personal access token")
			requireTruef(t, out.Token != "", "expected rotated token value")
			requireTruef(t, out.ID != pat.ID, "expected rotation to mint a new token ID")
			latestTokenID = out.ID
			t.Logf("Rotated PAT %d → %d", pat.ID, out.ID)
		})

		t.Run("Revoke", func(t *testing.T) {
			err := callToolVoidOn(ctx, sess.individual, "gitlab_personal_access_token_revoke", accesstokens.PersonalRevokeInput{
				TokenID: latestTokenID,
			})
			requireNoError(t, err, "revoke personal access token")
			t.Logf("Revoked PAT %d", latestTokenID)
		})
	})
}

// TestIndividual_PersonalAccessTokenSelfOps exercises
// access.token_personal_rotate_self and access.token_personal_revoke_self
// through individual MCP tools.
//
// Both endpoints act on the credential making the call, so the test
// provisions a disposable user and PAT, opens an MCP session authenticated
// as that PAT, and self-rotates it (invalidating the session's token). It
// then opens a second session with the freshly rotated token and
// self-revokes it — rotate first, revoke last, because each step kills the
// credential it ran on.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: individual. Admin token required.
func TestIndividual_PersonalAccessTokenSelfOps(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if !isDockerMode() {
		t.Skip("PAT self-service coverage provisions disposable users and tokens (Docker instance only)")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()

		user := accessExtrasCreateUser(ctx, t, "pat-self")
		pat := accessExtrasCreatePAT(ctx, t, user.ID, "pat-self")

		var rotatedToken string
		t.Run("RotateSelf", func(t *testing.T) {
			//nolint:contextcheck // The extra MCP session outlives this call context; its lifetime is bound to t.Cleanup.
			selfSession := accessExtrasStartSession(t, "pat-self", pat.Token)
			out, err := callToolOn[accesstokens.Output](ctx, selfSession, "gitlab_personal_access_token_rotate_self", accesstokens.PersonalRotateSelfInput{})
			requireNoError(t, err, "self-rotate personal access token")
			requireTruef(t, out.Token != "", "expected self-rotated token value")
			rotatedToken = out.Token
			t.Logf("Self-rotated PAT %d → %d", pat.ID, out.ID)
		})

		t.Run("RevokeSelf", func(t *testing.T) {
			requireTruef(t, rotatedToken != "", "rotated token value not captured")
			//nolint:contextcheck // The extra MCP session outlives this call context; its lifetime is bound to t.Cleanup.
			revokeSession := accessExtrasStartSession(t, "pat-self-revoke", rotatedToken)
			err := callToolVoidOn(ctx, revokeSession, "gitlab_personal_access_token_revoke_self", accesstokens.PersonalRevokeSelfInput{})
			requireNoError(t, err, "self-revoke personal access token")
			t.Log("Self-revoked rotated PAT")
		})
	})
}

// TestIndividual_InstanceDeployKeyAdd exercises
// access.deploy_key_add_instance through the gitlab_deploy_key_add_instance
// individual tool.
//
// The test adds a freshly generated public key as an instance-wide deploy
// key and asserts the created key ID round-trips. GitLab exposes no
// instance-level deploy key deletion endpoint, so the key intentionally
// remains on the disposable Docker instance — which is exactly why the test
// is Docker-gated.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: individual. Admin token required.
func TestIndividual_InstanceDeployKeyAdd(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if !isDockerMode() {
		t.Skip("instance deploy keys cannot be deleted via API; only acceptable on the disposable Docker instance")
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		out, err := callToolOn[deploykeys.InstanceOutput](ctx, sess.individual, "gitlab_deploy_key_add_instance", deploykeys.AddInstanceInput{
			Title: uniqueName("inst-key"),
			Key:   generateTestSSHKey(t),
		})
		requireNoError(t, err, "add instance deploy key")
		requireTruef(t, out.ID > 0, "expected instance deploy key ID > 0")
		t.Logf("Added instance deploy key %d", out.ID)
	})
}
