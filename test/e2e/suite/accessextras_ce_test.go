//go:build e2e && !enterprise

// accessextras_ce_test.go covers previously unexercised access-domain
// actions that do not need an admin token: group deploy tokens
// (access.deploy_token_create_group / get_group / list_group /
// delete_group), group email invitations (access.invite_group /
// access.invite_list_group), group access tokens (access.token_group_list /
// token_group_get / token_group_rotate / token_group_rotate_self), and
// project access token self-rotation (access.token_project_rotate_self).
//
// The *_rotate_self actions operate on the credential that authenticates
// the call, so this file also provides the shared helper that spins up an
// extra in-process MCP session bound to an arbitrary token.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/accesstokens"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/deploytokens"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/invites"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// accessExtrasStartSession builds an extra in-process MCP server/client pair
// whose GitLab client authenticates with the given token. This is the only
// way to exercise *_self token operations and non-member access requests
// through MCP, because those endpoints act on the calling credential itself.
// The session is closed via t.Cleanup.
func accessExtrasStartSession(t *testing.T, label, token string) *mcp.ClientSession {
	t.Helper()
	baseURL := os.Getenv("GITLAB_URL")
	requireTruef(t, baseURL != "", "GITLAB_URL must be set to start an extra MCP session")
	skipTLS := strings.EqualFold(os.Getenv("GITLAB_SKIP_TLS_VERIFY"), "true")
	client, err := gitlabclient.NewClientWithToken(baseURL, token, skipTLS)
	requireNoError(t, err, "create GitLab client for extra session "+label)
	running, err := startE2ESession(
		"gitlab-mcp-server-e2e-"+label,
		"e2e-"+label+"-client",
		nil,
		configureIndividualE2EServer(client, sess.enterprise),
	)
	requireNoError(t, err, "start extra MCP session "+label)
	t.Cleanup(func() {
		_ = running.session.Close()
		running.cancel()
	})
	return running.session
}

// accessExtrasExpiry returns a token expiry 30 days out, comfortably inside
// GitLab's maximum allowed token lifetime.
func accessExtrasExpiry() gl.ISOTime {
	return gl.ISOTime(time.Now().AddDate(0, 0, 30))
}

// TestIndividual_GroupDeployTokens exercises the group deploy token
// lifecycle through individual MCP tools: create → get → list → delete
// (access.deploy_token_create_group, access.deploy_token_get_group,
// access.deploy_token_list_group, access.deploy_token_delete_group).
//
// The test creates a group fixture, mints a read_repository deploy token on
// it, reads it back by ID, confirms it appears in the group listing, and
// deletes it. The token value is only returned at creation time, which the
// create subtest asserts.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_GroupDeployTokens(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured (group fixture)")
	}
	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	grp := CreateGroupMeta(ctx, e2e, sess.meta, "dtok")
	groupID := toolutil.StringOrInt(grp.gidStr())
	var tokenID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[deploytokens.Output](ctx, sess.individual, "gitlab_deploy_token_create_group", deploytokens.CreateGroupInput{
			GroupID: groupID,
			Name:    uniqueName("dtok"),
			Scopes:  []string{"read_repository"},
		})
		requireNoError(t, err, "create group deploy token")
		requireTruef(t, out.ID > 0, "expected deploy token ID > 0")
		requireTruef(t, out.Token != "", "expected deploy token value on creation")
		tokenID = out.ID
		t.Logf("Created group deploy token %d", tokenID)
	})

	t.Run("Get", func(t *testing.T) {
		requireTruef(t, tokenID > 0, "tokenID not set")
		out, err := callToolOn[deploytokens.Output](ctx, sess.individual, "gitlab_deploy_token_get_group", deploytokens.GetGroupInput{
			GroupID:       groupID,
			DeployTokenID: tokenID,
		})
		requireNoError(t, err, "get group deploy token")
		requireTruef(t, out.ID == tokenID, "expected token ID %d, got %d", tokenID, out.ID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[deploytokens.ListOutput](ctx, sess.individual, "gitlab_deploy_token_list_group", deploytokens.ListGroupInput{
			GroupID: groupID,
		})
		requireNoError(t, err, "list group deploy tokens")
		requireTruef(t, len(out.DeployTokens) >= 1, "expected at least 1 deploy token, got %d", len(out.DeployTokens))
		t.Logf("Listed %d group deploy tokens", len(out.DeployTokens))
	})

	t.Run("Delete", func(t *testing.T) {
		requireTruef(t, tokenID > 0, "tokenID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_deploy_token_delete_group", deploytokens.DeleteGroupInput{
			GroupID:       groupID,
			DeployTokenID: tokenID,
		})
		requireNoError(t, err, "delete group deploy token")
		t.Logf("Deleted group deploy token %d", tokenID)
	})
}

// TestIndividual_GroupInvitations exercises access.invite_group and
// access.invite_list_group through individual MCP tools.
//
// The test invites a unique, non-registered email address to a fresh group
// (email invitations never send outside the disposable test scope because
// the address is fake) and then confirms the pending invitation shows up in
// the group's invitation listing. The invitation disappears with the group
// fixture cleanup.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_GroupInvitations(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured (group fixture)")
	}
	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	grp := CreateGroupMeta(ctx, e2e, sess.meta, "invite")
	groupID := toolutil.StringOrInt(grp.gidStr())
	email := uniqueName("invitee") + "@e2e-test.example"

	t.Run("Invite", func(t *testing.T) {
		out, err := callToolOn[invites.InviteResultOutput](ctx, sess.individual, "gitlab_group_invite", invites.GroupInvitesInput{
			GroupID:     groupID,
			Email:       email,
			AccessLevel: 30,
		})
		requireNoError(t, err, "invite email to group")
		requireTruef(t, out.Status == "success", "expected invitation status success, got %q (%v)", out.Status, out.Message)
		t.Logf("Invited %s to group %s", email, grp.Path)
	})

	t.Run("ListPending", func(t *testing.T) {
		out, err := callToolOn[invites.ListPendingInvitationsOutput](ctx, sess.individual, "gitlab_group_invite_list_pending", invites.ListPendingGroupInvitationsInput{
			GroupID: groupID,
		})
		requireNoError(t, err, "list pending group invitations")
		found := false
		for _, inv := range out.Invitations {
			if inv.InviteEmail == email {
				found = true
				break
			}
		}
		requireTruef(t, found, "expected pending invitation for %s among %d invitations", email, len(out.Invitations))
		t.Logf("Found pending invitation for %s", email)
	})
}

// TestIndividual_GroupAccessTokens exercises access.token_group_list,
// access.token_group_get, access.token_group_rotate, and
// access.token_group_rotate_self through individual MCP tools.
//
// The test seeds one group access token via the raw API (creation is covered
// elsewhere), lists and gets it through MCP, rotates it as the group owner,
// and finally self-rotates the rotated token through an extra MCP session
// authenticated as that token — rotate_self acts on the calling credential,
// so it cannot be exercised from the suite's own session. The latest token
// generation is revoked during cleanup so the bot membership never outlives
// the group fixture.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_GroupAccessTokens(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured (group fixture)")
	}
	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	grp := CreateGroupMeta(ctx, e2e, sess.meta, "gat")
	groupID := toolutil.StringOrInt(grp.gidStr())

	expiry := accessExtrasExpiry()
	created, _, seedErr := sess.glClient.GL().GroupAccessTokens.CreateGroupAccessToken(grp.ID, &gl.CreateGroupAccessTokenOptions{
		Name:        new(uniqueName("gat")),
		Scopes:      &[]string{"api"},
		AccessLevel: new(gl.MaintainerPermissions),
		ExpiresAt:   &expiry,
	}, gl.WithContext(ctx))
	requireNoError(t, seedErr, "create group access token fixture")
	requireTruef(t, created.Token != "", "expected group access token value on creation")

	latestTokenID := created.ID
	defer func() {
		cctx, ccancel := cleanupContext(defaultCleanupTimeout)
		defer ccancel()
		// Revoke whichever generation is current; group deletion would also
		// remove the bot, this just keeps the fixture window minimal.
		_, _ = sess.glClient.GL().GroupAccessTokens.RevokeGroupAccessToken(grp.ID, latestTokenID, gl.WithContext(cctx))
	}()

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[accesstokens.ListOutput](ctx, sess.individual, "gitlab_group_access_token_list", accesstokens.GroupListInput{
			GroupID: groupID,
		})
		requireNoError(t, err, "list group access tokens")
		requireTruef(t, len(out.Tokens) >= 1, "expected at least 1 group access token, got %d", len(out.Tokens))
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[accesstokens.Output](ctx, sess.individual, "gitlab_group_access_token_get", accesstokens.GroupGetInput{
			GroupID: groupID,
			TokenID: created.ID,
		})
		requireNoError(t, err, "get group access token")
		requireTruef(t, out.ID == created.ID, "expected token ID %d, got %d", created.ID, out.ID)
	})

	var rotatedToken string
	t.Run("Rotate", func(t *testing.T) {
		out, err := callToolOn[accesstokens.Output](ctx, sess.individual, "gitlab_group_access_token_rotate", accesstokens.GroupRotateInput{
			GroupID: groupID,
			TokenID: created.ID,
		})
		requireNoError(t, err, "rotate group access token")
		requireTruef(t, out.Token != "", "expected rotated token value")
		requireTruef(t, out.ID != created.ID, "expected rotation to mint a new token ID")
		latestTokenID = out.ID
		rotatedToken = out.Token
		t.Logf("Rotated group access token %d → %d", created.ID, out.ID)
	})

	t.Run("RotateSelf", func(t *testing.T) {
		requireTruef(t, rotatedToken != "", "rotated token value not captured")
		//nolint:contextcheck // The extra MCP session outlives this call context; its lifetime is bound to t.Cleanup.
		selfSession := accessExtrasStartSession(t, "gat-self", rotatedToken)
		out, err := callToolOn[accesstokens.Output](ctx, selfSession, "gitlab_group_access_token_rotate_self", accesstokens.GroupRotateSelfInput{
			GroupID: groupID,
		})
		requireNoError(t, err, "self-rotate group access token")
		requireTruef(t, out.Token != "", "expected self-rotated token value")
		latestTokenID = out.ID
		t.Logf("Self-rotated group access token → %d", out.ID)
	})
}

// TestIndividual_ProjectAccessTokenRotateSelf exercises
// access.token_project_rotate_self through the
// gitlab_project_access_token_rotate_self individual tool.
//
// The test seeds a project access token via the raw API (creation is covered
// elsewhere) and self-rotates it through an extra MCP session authenticated
// as that token, because the endpoint acts on the calling credential. The
// rotated generation is revoked during cleanup; the project fixture cleanup
// removes the bot membership regardless.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_ProjectAccessTokenRotateSelf(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	expiry := accessExtrasExpiry()
	created, _, err := sess.glClient.GL().ProjectAccessTokens.CreateProjectAccessToken(proj.ID, &gl.CreateProjectAccessTokenOptions{
		Name:        new(uniqueName("pat-proj")),
		Scopes:      &[]string{"api"},
		AccessLevel: new(gl.MaintainerPermissions),
		ExpiresAt:   &expiry,
	}, gl.WithContext(ctx))
	requireNoError(t, err, "create project access token fixture")
	requireTruef(t, created.Token != "", "expected project access token value on creation")

	latestTokenID := created.ID
	defer func() {
		cctx, ccancel := cleanupContext(defaultCleanupTimeout)
		defer ccancel()
		_, _ = sess.glClient.GL().ProjectAccessTokens.RevokeProjectAccessToken(proj.ID, latestTokenID, gl.WithContext(cctx))
	}()

	selfSession := accessExtrasStartSession(t, "prjtok-self", created.Token)
	// A token created moments ago can transiently 404 on its own self-rotate
	// endpoint; under full-suite load the window exceeds 20 seconds, so poll
	// patiently before asserting.
	out, err := retryWithBackoffInterval(ctx, t, "self-rotate project access token", 8, 5*time.Second, func(int) (accesstokens.Output, bool, string, error) {
		rotated, rotateErr := callToolOn[accesstokens.Output](ctx, selfSession, "gitlab_project_access_token_rotate_self", accesstokens.ProjectRotateSelfInput{
			ProjectID: proj.pidOf(),
		})
		return rotated, rotateErr != nil && isHTTPStatus(rotateErr, 404), "token not yet rotatable", rotateErr
	})
	requireNoError(t, err, "self-rotate project access token")
	requireTruef(t, out.Token != "", "expected self-rotated token value")
	latestTokenID = out.ID
	t.Logf("Self-rotated project access token %d → %d", created.ID, out.ID)
}
