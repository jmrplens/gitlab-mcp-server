//go:build e2e

// deploykeys_test.go tests the deploy key MCP tools against a live GitLab instance.
// Covers add, get, list, update, and delete for both individual and meta-tool modes.
package suite

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deploykeys"
)

// generateTestSSHKey generates a fresh ED25519 SSH public key for deploy key tests.
func generateTestSSHKey(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("ssh public key: %v", err)
	}
	return string(ssh.MarshalAuthorizedKey(sshPub))
}

// TestIndividual_DeployKeys exercises the deploy key lifecycle using individual tools:
// add → get → list → update → delete. Generates a fresh ED25519 SSH key per run.
func TestIndividual_DeployKeys(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	sshKey := generateTestSSHKey(t)

	var keyID int64

	t.Run("Add", func(t *testing.T) {
		out, err := callToolOn[deploykeys.Output](ctx, sess.individual, "gitlab_deploy_key_add", deploykeys.AddInput{
			ProjectID: proj.pidOf(),
			Title:     "e2e-deploy-key",
			Key:       sshKey,
		})
		requireNoError(t, err, "add deploy key")
		requireTrue(t, out.ID > 0, "expected key ID")
		keyID = out.ID
		t.Logf("Added deploy key %d", keyID)
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, keyID > 0, "keyID not set")
		out, err := callToolOn[deploykeys.Output](ctx, sess.individual, "gitlab_deploy_key_get", deploykeys.GetInput{
			ProjectID:   proj.pidOf(),
			DeployKeyID: keyID,
		})
		requireNoError(t, err, "get deploy key")
		requireTrue(t, out.ID == keyID, "expected ID %d", keyID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[deploykeys.ListOutput](ctx, sess.individual, "gitlab_deploy_key_list_project", deploykeys.ListProjectInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list deploy keys")
		requireTrue(t, len(out.DeployKeys) >= 1, "expected at least 1 key")
	})

	t.Run("Update", func(t *testing.T) {
		requireTrue(t, keyID > 0, "keyID not set")
		out, err := callToolOn[deploykeys.Output](ctx, sess.individual, "gitlab_deploy_key_update", deploykeys.UpdateInput{
			ProjectID:   proj.pidOf(),
			DeployKeyID: keyID,
			Title:       "e2e-deploy-key-updated",
		})
		requireNoError(t, err, "update deploy key")
		requireTrue(t, out.Title == "e2e-deploy-key-updated", "expected updated title")
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, keyID > 0, "keyID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_deploy_key_delete", deploykeys.DeleteInput{
			ProjectID:   proj.pidOf(),
			DeployKeyID: keyID,
		})
		requireNoError(t, err, "delete deploy key")
	})
}

// TestMeta_DeployKeys exercises the same deploy key lifecycle via the gitlab_access meta-tool.
func TestMeta_DeployKeys(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	sshKey := generateTestSSHKey(t)

	var keyID int64

	t.Run("Add", func(t *testing.T) {
		out, err := callToolOn[deploykeys.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_key_add",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"title":      "e2e-meta-deploy-key",
				"key":        sshKey,
			},
		})
		requireNoError(t, err, "meta add deploy key")
		requireTrue(t, out.ID > 0, "expected key ID")
		keyID = out.ID
		t.Logf("Added deploy key %d via meta-tool", keyID)
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, keyID > 0, "keyID not set")
		out, err := callToolOn[deploykeys.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_key_get",
			"params": map[string]any{"project_id": proj.pidStr(), "deploy_key_id": keyID},
		})
		requireNoError(t, err, "meta get deploy key")
		requireTrue(t, out.ID == keyID, "expected ID %d", keyID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[deploykeys.ListOutput](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_key_list_project",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta list deploy keys")
		requireTrue(t, len(out.DeployKeys) >= 1, "expected at least 1 key")
	})

	t.Run("Update", func(t *testing.T) {
		requireTrue(t, keyID > 0, "keyID not set")
		out, err := callToolOn[deploykeys.Output](ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_key_update",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"deploy_key_id": keyID,
				"title":         "e2e-meta-key-updated",
			},
		})
		requireNoError(t, err, "meta update deploy key")
		requireTrue(t, out.Title == "e2e-meta-key-updated", "expected updated title")
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, keyID > 0, "keyID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_access", map[string]any{
			"action": "deploy_key_delete",
			"params": map[string]any{"project_id": proj.pidStr(), "deploy_key_id": keyID},
		})
		requireNoError(t, err, "meta delete deploy key")
	})
}
