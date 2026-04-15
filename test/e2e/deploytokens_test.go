//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploytokens"
)

// TestMeta_DeployTokens exercises project deploy token CRUD via the gitlab_access meta-tool.
func TestMeta_DeployTokens(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	var tokenID int64

	t.Run("Meta/DeployToken/Create", func(t *testing.T) {
		out, err := callToolOn[deploytokens.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_token_create_project",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       "e2e-deploy-token",
				"scopes":     []string{"read_repository"},
			},
		})
		requireNoError(t, err, "deploy token create")
		requireTrue(t, out.ID > 0, "expected positive deploy token ID")
		tokenID = out.ID
		t.Logf("Created deploy token %d (%s)", out.ID, out.Name)
	})

	t.Run("Meta/DeployToken/List", func(t *testing.T) {
		requireTrue(t, tokenID > 0, "tokenID not set")
		out, err := callToolOn[deploytokens.ListOutput](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_token_list_project",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "deploy token list")
		requireTrue(t, len(out.DeployTokens) >= 1, "expected at least 1 deploy token")
		t.Logf("Listed %d deploy token(s)", len(out.DeployTokens))
	})

	t.Run("Meta/DeployToken/Get", func(t *testing.T) {
		requireTrue(t, tokenID > 0, "tokenID not set")
		out, err := callToolOn[deploytokens.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_token_get_project",
			"params": map[string]any{
				"project_id":      proj.pidStr(),
				"deploy_token_id": fmt.Sprintf("%d", tokenID),
			},
		})
		requireNoError(t, err, "deploy token get")
		requireTrue(t, out.ID == tokenID, "deploy token ID mismatch")
		t.Logf("Got deploy token %d (%s)", out.ID, out.Name)
	})

	t.Run("Meta/DeployToken/Delete", func(t *testing.T) {
		requireTrue(t, tokenID > 0, "tokenID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_token_delete_project",
			"params": map[string]any{
				"project_id":      proj.pidStr(),
				"deploy_token_id": fmt.Sprintf("%d", tokenID),
			},
		})
		requireNoError(t, err, "deploy token delete")
		t.Logf("Deleted deploy token %d", tokenID)
	})
}
