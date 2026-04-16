//go:build e2e

package suite

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/accessrequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/accesstokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploykeys"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploytokens"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/invites"
)

// TestMeta_AccessTokensProject exercises project access token CRUD via gitlab_access.
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
		requireTrue(t, out.ID > 0, "token_project_create: expected ID > 0")
		tokenID = out.ID
		t.Logf("Created project token %d", tokenID)
	})

	t.Run("TokenProjectGet", func(t *testing.T) {
		requireTrue(t, tokenID > 0, "tokenID not set")
		out, err := callToolOn[accesstokens.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "token_project_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"token_id":   tokenID,
			},
		})
		requireNoError(t, err, "token_project_get")
		requireTrue(t, out.ID == tokenID, "token_project_get: ID mismatch")
	})

	t.Run("TokenProjectRotate", func(t *testing.T) {
		requireTrue(t, tokenID > 0, "tokenID not set")
		out, err := callToolOn[accesstokens.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "token_project_rotate",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"token_id":   tokenID,
			},
		})
		requireNoError(t, err, "token_project_rotate")
		requireTrue(t, out.ID > 0, "token_project_rotate: expected new token ID")
		tokenID = out.ID
		t.Logf("Rotated to token %d", tokenID)
	})

	t.Run("TokenProjectRevoke", func(t *testing.T) {
		requireTrue(t, tokenID > 0, "tokenID not set")
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

// TestMeta_AccessTokensPersonal exercises personal access token operations.
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

// TestMeta_AccessDeployTokens exercises deploy token CRUD via gitlab_access.
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
		requireTrue(t, out.ID > 0, "deploy_token_create_project: expected ID > 0")
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
		requireTrue(t, dtID > 0, "dtID not set")
		out, err := callToolOn[deploytokens.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_token_get_project",
			"params": map[string]any{
				"project_id":      proj.pidStr(),
				"deploy_token_id": dtID,
			},
		})
		requireNoError(t, err, "deploy_token_get_project")
		requireTrue(t, out.ID == dtID, "deploy_token_get_project: ID mismatch")
	})
}

// TestMeta_DeployKeysExtended exercises extended deploy key actions.
func TestMeta_DeployKeysExtended(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
		requireTrue(t, out.ID > 0, "deploy_key_add: expected ID > 0")
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
		requireTrue(t, keyID > 0, "keyID not set")
		out, err := callToolOn[deploykeys.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_key_enable",
			"params": map[string]any{
				"project_id":    proj2.pidStr(),
				"deploy_key_id": keyID,
			},
		})
		requireNoError(t, err, "deploy_key_enable")
		requireTrue(t, out.ID == keyID, "deploy_key_enable: ID mismatch")
		t.Logf("Enabled deploy key %d on project %d", keyID, proj2.ID)
	})

	t.Run("DeployKeyListUserProject", func(t *testing.T) {
		out, err := callToolOn[deploykeys.ListOutput](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_key_list_user_project",
			"params": map[string]any{"user_id": os.Getenv("GITLAB_USER")},
		})
		requireNoError(t, err, "deploy_key_list_user_project")
		t.Logf("User project deploy keys: %d", len(out.DeployKeys))
	})
}

// TestMeta_AccessRequests exercises access request actions.
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

// TestMeta_Invitations exercises invitation actions.
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
		t.Logf("Invite result: status=%s", out.Status)
	})
}
