//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/accesstokens"
)

func expiresAtNextYear() string {
	return time.Now().AddDate(1, 0, 0).Format("2006-01-02")
}

func TestIndividual_AccessTokens(t *testing.T) {
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	var tokenID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[accesstokens.Output](ctx, sess.individual, "gitlab_project_access_token_create", accesstokens.ProjectCreateInput{
			ProjectID: proj.pidOf(),
			Name:      "e2e-token",
			Scopes:    []string{"read_api"},
			ExpiresAt: expiresAtNextYear(),
		})
		requireNoError(t, err, "create project access token")
		requireTrue(t, out.ID > 0, "expected token ID")
		tokenID = out.ID
		t.Logf("Created token %d", tokenID)
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, tokenID > 0, "tokenID not set")
		out, err := callToolOn[accesstokens.Output](ctx, sess.individual, "gitlab_project_access_token_get", accesstokens.ProjectGetInput{
			ProjectID: proj.pidOf(),
			TokenID:   tokenID,
		})
		requireNoError(t, err, "get project access token")
		requireTrue(t, out.ID == tokenID, "expected ID %d", tokenID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[accesstokens.ListOutput](ctx, sess.individual, "gitlab_project_access_token_list", accesstokens.ProjectListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list project access tokens")
		requireTrue(t, len(out.Tokens) >= 1, "expected at least 1 token")
	})

	t.Run("Revoke", func(t *testing.T) {
		requireTrue(t, tokenID > 0, "tokenID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_project_access_token_revoke", accesstokens.ProjectRevokeInput{
			ProjectID: proj.pidOf(),
			TokenID:   tokenID,
		})
		requireNoError(t, err, "revoke project access token")
	})
}

func TestMeta_AccessTokens(t *testing.T) {
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var tokenID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[accesstokens.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "token_project_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       "e2e-meta-token",
				"scopes":     []string{"read_api"},
				"expires_at": expiresAtNextYear(),
			},
		})
		requireNoError(t, err, "meta create token")
		requireTrue(t, out.ID > 0, "expected token ID")
		tokenID = out.ID
		t.Logf("Created token %d via meta-tool", tokenID)
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, tokenID > 0, "tokenID not set")
		out, err := callToolOn[accesstokens.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "token_project_get",
			"params": map[string]any{"project_id": proj.pidStr(), "token_id": tokenID},
		})
		requireNoError(t, err, "meta get token")
		requireTrue(t, out.ID == tokenID, "expected ID %d", tokenID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[accesstokens.ListOutput](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "token_project_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta list tokens")
		requireTrue(t, len(out.Tokens) >= 1, "expected at least 1 token")
	})

	t.Run("Revoke", func(t *testing.T) {
		requireTrue(t, tokenID > 0, "tokenID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "token_project_revoke",
			"params": map[string]any{"project_id": proj.pidStr(), "token_id": tokenID},
		})
		requireNoError(t, err, "meta revoke token")
	})
}
