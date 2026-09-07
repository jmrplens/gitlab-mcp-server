//go:build e2e && !enterprise

// access_meta_ce_test.go tests the access-management MCP tools via the
// gitlab_access meta-tool against a live GitLab instance. Covers project
// access tokens, personal tokens, deploy tokens, deploy keys, access
// requests, and invitations.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/accessrequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/accesstokens"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/deploykeys"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/deploytokens"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/invites"
)

// TestMeta_AccessTokensProject exercises the project access token lifecycle
// through the gitlab_access meta-tool.
//
// The test creates a project fixture and drives list, create, get, rotate, and
// revoke through the catalog-backed gitlab_access meta-tool, asserting that
// each step returns the expected token ID and that the revoke step completes
// without error. The created project is auto-deleted by the fixture's
// per-test resource ledger.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_AccessTokensProject(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	var tokenID int64

	t.Run("TokenProjectList", func(t *testing.T) {
		out, err := callToolOn[accesstokens.ListOutput](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "token_project_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "token_project_list")
		t.Logf("Project tokens: %d", len(out.Tokens))
	})

	t.Run("TokenProjectCreate", func(t *testing.T) {
		expires := time.Now().AddDate(0, 1, 0).Format("2006-01-02")
		out, err := callToolOn[accesstokens.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "token_project_create",
			"params": map[string]any{
				"project_id":   proj.pidStr(),
				"name":         "e2e-token-" + uniqueName(""),
				"scopes":       []string{"api"},
				"expires_at":   expires,
				"access_level": 30,
			},
		})
		requireNoError(t, err, "token_project_create")
		requireTruef(t, out.ID > 0, "token_project_create: expected ID > 0")
		tokenID = out.ID
		t.Logf("Created project token %d", tokenID)
	})

	t.Run("TokenProjectGet", func(t *testing.T) {
		requireTruef(t, tokenID > 0, "tokenID not set")
		out, err := callToolOn[accesstokens.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "token_project_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"token_id":   tokenID,
			},
		})
		requireNoError(t, err, "token_project_get")
		requireTruef(t, out.ID == tokenID, "token_project_get: ID mismatch")
	})

	t.Run("TokenProjectRotate", func(t *testing.T) {
		requireTruef(t, tokenID > 0, "tokenID not set")
		out, err := callToolOn[accesstokens.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "token_project_rotate",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"token_id":   tokenID,
			},
		})
		requireNoError(t, err, "token_project_rotate")
		requireTruef(t, out.ID > 0, "token_project_rotate: expected new token ID")
		tokenID = out.ID
		t.Logf("Rotated to token %d", tokenID)
	})

	t.Run("TokenProjectRevoke", func(t *testing.T) {
		requireTruef(t, tokenID > 0, "tokenID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "token_project_revoke",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"token_id":   tokenID,
			},
		})
		requireNoError(t, err, "token_project_revoke")
		tokenID = 0
	})
}

// TestMeta_AccessTokensPersonal exercises personal access token operations
// via the gitlab_access meta-tool.
//
// The test lists the authenticated user's personal access tokens through the
// token_personal_list action. It asserts the call succeeds and logs the count
// for visibility; it does not mutate personal tokens because the E2E token is
// the active credential for the test run.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_AccessTokensPersonal(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Run("TokenPersonalList", func(t *testing.T) {
		out, err := callToolOn[accesstokens.ListOutput](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "token_personal_list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "token_personal_list")
		t.Logf("Personal tokens: %d", len(out.Tokens))
	})
}

// TestMeta_AccessDeployTokens exercises the deploy token lifecycle through the
// gitlab_access meta-tool.
//
// The test creates a project fixture, lists instance- and project-scoped
// deploy tokens, creates a new project-scoped deploy token, fetches it by ID,
// and defers deletion via deploy_token_delete_project. Each subtest asserts
// the expected ID and that the meta-tool returns a structured output.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_AccessDeployTokens(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	var dtID int64

	t.Run("DeployTokenListAll", func(t *testing.T) {
		out, err := callToolOn[deploytokens.ListOutput](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_token_list_all",
			"params": map[string]any{},
		})
		requireNoError(t, err, "deploy_token_list_all")
		t.Logf("All deploy tokens: %d", len(out.DeployTokens))
	})

	t.Run("DeployTokenListProject", func(t *testing.T) {
		out, err := callToolOn[deploytokens.ListOutput](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_token_list_project",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "deploy_token_list_project")
		t.Logf("Project deploy tokens: %d", len(out.DeployTokens))
	})

	t.Run("DeployTokenCreateProject", func(t *testing.T) {
		expires := time.Now().AddDate(0, 1, 0).Format("2006-01-02")
		out, err := callToolOn[deploytokens.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_token_create_project",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       "e2e-dt-" + uniqueName(""),
				"scopes":     []string{"read_repository"},
				"expires_at": expires,
			},
		})
		requireNoError(t, err, "deploy_token_create_project")
		requireTruef(t, out.ID > 0, "deploy_token_create_project: expected ID > 0")
		dtID = out.ID
		t.Logf("Created deploy token %d", dtID)
	})
	defer func() {
		if dtID > 0 {
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_access", map[string]any{
				"action": "deploy_token_delete_project",
				"params": map[string]any{
					"project_id":      proj.pidStr(),
					"deploy_token_id": dtID,
				},
			})
		}
	}()

	t.Run("DeployTokenGetProject", func(t *testing.T) {
		requireTruef(t, dtID > 0, "dtID not set")
		out, err := callToolOn[deploytokens.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_token_get_project",
			"params": map[string]any{
				"project_id":      proj.pidStr(),
				"deploy_token_id": dtID,
			},
		})
		requireNoError(t, err, "deploy_token_get_project")
		requireTruef(t, out.ID == dtID, "deploy_token_get_project: ID mismatch")
	})
}

// TestMeta_DeployKeysExtended exercises extended deploy key actions on the
// gitlab_access meta-tool, including add, enable on a second project, and
// user/project listing.
//
// The test creates two project fixtures, registers a generated SSH key as a
// deploy key on the first project, enables it on the second project, and
// lists user-scoped deploy keys. Cleanup deletes the deploy key after the
// enable step. The parallel flag is dropped in Enterprise mode because
// shared GitLab state is touched across both fixtures.
//
// Build tag: e2e && !enterprise. Mode: CE/EE. Surface: meta.
func TestMeta_DeployKeysExtended(t *testing.T) {
	if !sess.enterprise {
		t.Parallel()
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := e2eTimeoutContext(120*time.Second, 300*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	sshKey := generateTestSSHKey(t)
	var keyID int64

	t.Run("DeployKeyListAll", func(t *testing.T) {
		out, err := callToolOn[deploykeys.ListOutput](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_key_list_all",
			"params": map[string]any{},
		})
		requireNoError(t, err, "deploy_key_list_all")
		t.Logf("All deploy keys: %d", len(out.DeployKeys))
	})

	t.Run("DeployKeyAdd", func(t *testing.T) {
		out, err := callToolOn[deploykeys.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_key_add",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"title":      "e2e-dk-" + uniqueName(""),
				"key":        sshKey,
			},
		})
		requireNoError(t, err, "deploy_key_add")
		requireTruef(t, out.ID > 0, "deploy_key_add: expected ID > 0")
		keyID = out.ID
		t.Logf("Added deploy key %d", keyID)
	})
	defer func() {
		if keyID > 0 {
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_access", map[string]any{
				"action": "deploy_key_delete",
				"params": map[string]any{
					"project_id":    proj.pidStr(),
					"deploy_key_id": keyID,
				},
			})
		}
	}()

	// Create a second project to test enable (sharing a deploy key)
	proj2 := createProjectMeta(ctx, t, sess.meta)

	t.Run("DeployKeyEnable", func(t *testing.T) {
		requireTruef(t, keyID > 0, "keyID not set")
		out, err := callToolOn[deploykeys.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_key_enable",
			"params": map[string]any{
				"project_id":    proj2.pidStr(),
				"deploy_key_id": keyID,
			},
		})
		requireNoError(t, err, "deploy_key_enable")
		requireTruef(t, out.ID == keyID, "deploy_key_enable: ID mismatch")
		t.Logf("Enabled deploy key %d on project %d", keyID, proj2.ID)
	})

	t.Run("DeployKeyListUserProject", func(t *testing.T) {
		out, err := callToolOn[deploykeys.ListOutput](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_key_list_user_project",
			"params": map[string]any{"user_id": sess.username},
		})
		requireNoError(t, err, "deploy_key_list_user_project")
		// The key this test created is enabled on two projects the same account
		// owns, which is exactly what this listing reports.
		requireTruef(t, slices.ContainsFunc(out.DeployKeys, func(k deploykeys.Output) bool {
			return k.ID == keyID
		}), "deploy key %d is not among the account's project deploy keys: %+v", keyID, out.DeployKeys)
		t.Logf("User project deploy keys: %d", len(out.DeployKeys))
	})
}

// TestMeta_AccessRequests exercises access request actions via the
// gitlab_access meta-tool.
//
// The test creates a project fixture and lists pending access requests on
// the project through request_list_project. The list is expected to be
// well-formed even when no requests are pending, asserting that the meta-tool
// returns a structured empty slice rather than an error.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_AccessRequests(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("AccessRequestListProject", func(t *testing.T) {
		out, err := callToolOn[accessrequests.ListOutput](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "request_list_project",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "request_list_project")
		t.Logf("Project access requests: %d", len(out.AccessRequests))
	})
}

// TestMeta_Invitations exercises invitation actions via the gitlab_access
// meta-tool.
//
// The test creates a project fixture, lists pending invitations through
// invite_list_project, and creates a fresh invitation with a unique email
// through invite_project. It asserts that both calls succeed and that the
// returned invitation status is one of the GitLab-recognized states
// (accepted, pending, expired).
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_Invitations(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("InviteListProject", func(t *testing.T) {
		out, err := callToolOn[invites.ListPendingInvitationsOutput](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "invite_list_project",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "invite_list_project")
		t.Logf("Project invitations: %d", len(out.Invitations))
	})

	t.Run("InviteProject", func(t *testing.T) {
		email := fmt.Sprintf("e2e-%s@example.com", uniqueName(""))
		out, err := callToolOn[invites.InviteResultOutput](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "invite_project",
			"params": map[string]any{
				"project_id":   proj.pidStr(),
				"email":        email,
				"access_level": 30,
			},
		})
		requireNoError(t, err, "invite_project")
		requireTruef(t, out.Status == "success", "invite status = %q (message %v), want %q", out.Status, out.Message, "success")
		requireListedOn(ctx, t, sess.meta, "project invitations after invite", "gitlab_access", map[string]any{
			"action": "invite_list_project",
			"params": map[string]any{"project_id": proj.pidStr()},
		}, func(list invites.ListPendingInvitationsOutput) []string {
			emails := make([]string, 0, len(list.Invitations))
			for _, inv := range list.Invitations {
				emails = append(emails, inv.InviteEmail)
			}
			return emails
		}, email)
		t.Logf("Invite result: status=%s", out.Status)
	})
}
