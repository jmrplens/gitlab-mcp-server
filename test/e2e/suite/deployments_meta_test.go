//go:build e2e

// deployments_meta_test.go tests the deployment MCP tools against a live GitLab instance.
// Exercises get, update, and delete via the gitlab_environment meta-tool (deployment_* actions).
package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deployments"
)

// TestMeta_DeploymentsGetUpdateDelete exercises get, update, and delete
// deployment actions via the gitlab_environment meta-tool (deployment_* actions).
func TestMeta_DeploymentsGetUpdateDelete(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	// Create environment
	envName := uniqueName("deploy-env")
	callToolVoidOn(ctx, sess.meta, "gitlab_environment", map[string]any{
		"action": "create",
		"params": map[string]any{"project_id": proj.pidStr(), "name": envName},
	})

	// Commit a file so there's a valid SHA
	commitFileMeta(ctx, t, sess.meta, proj, "main", "deploy-get.txt", "deploy content", "deploy commit")

	// Create a deployment
	createOut, createErr := callToolOn[deployments.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
		"action": "deployment_create",
		"params": map[string]any{
			"project_id":  proj.pidStr(),
			"environment": envName,
			"sha":         "main",
			"ref":         "main",
			"tag":         false,
			"status":      "running",
		},
	})
	requireNoError(t, createErr, "deployment create")
	requireTrue(t, createOut.ID > 0, "expected deployment ID > 0")
	deployID := strconv.Itoa(createOut.ID)

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[deployments.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "deployment_get",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"deployment_id": deployID,
			},
		})
		requireNoError(t, err, "deployment get")
		requireTrue(t, out.ID > 0, "deployment get: expected ID > 0")
		t.Logf("Got deployment %d: status=%s, ref=%s", out.ID, out.Status, out.Ref)
	})

	t.Run("Update", func(t *testing.T) {
		out, err := callToolOn[deployments.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "deployment_update",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"deployment_id": deployID,
				"status":        "success",
			},
		})
		requireNoError(t, err, "deployment update")
		t.Logf("Updated deployment %d: status=%s", out.ID, out.Status)
	})

	t.Run("Delete", func(t *testing.T) {
		// Deployment was updated to "success" status — GitLab blocks deletion of completed deployments
		err := callToolVoidOn(ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "deployment_delete",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"deployment_id": deployID,
			},
		})
		requireTrue(t, err != nil, "expected error deleting completed deployment")
		t.Logf("Expected error for completed deployment deletion: %v", err)
	})
}
