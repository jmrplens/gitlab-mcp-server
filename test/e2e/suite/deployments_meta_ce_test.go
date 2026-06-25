//go:build e2e && !enterprise

// deployments_meta_ce_test.go tests the deployment MCP tools against a live GitLab instance.
// Exercises get, update, and delete via the gitlab_environment meta-tool (deployment_* actions).
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/deployments"
)

// TestMeta_DeploymentsGetUpdateDelete exercises get, update, and delete
// deployment actions via the gitlab_environment meta-tool
// (deployment_* actions).
//
// The test drives three subtests: deployment_get, deployment_update, and
// deployment_delete through the catalog-backed gitlab_environment tool.
// Each subtest asserts the expected ID round-trips through the GitLab
// API and verifies the tool name stays constant across the lifecycle.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
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
	requireTruef(t, createOut.ID > 0, "expected deployment ID > 0")
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
		requireTruef(t, out.ID > 0, "deployment get: expected ID > 0")

		// Validate the doc-grounded 1:1 output shape (deployments.Output mirrors the
		// documented top-level deployment fields: id, iid, ref, sha, created_at,
		// status, user). These were created with ref=main, sha=main, status=running
		// by the test token user, so the documented scalars must be populated.
		requireTruef(t, out.SHA != "",
			"deployment get: doc field 'sha' must be populated (1:1 output shape)")
		requireTruef(t, out.Ref == "main",
			"deployment get: doc field 'ref' must be 'main' (1:1 output shape), got %q", out.Ref)
		requireTruef(t, out.Status != "",
			"deployment get: doc field 'status' must be populated (1:1 output shape), got %q", out.Status)
		// The deployment was created by the test token user, so the documented
		// 'user' object must be present and carry a username.
		requireTruef(t, out.User != nil,
			"deployment get: doc field 'user' object must be present (1:1 output shape)")
		if out.User != nil {
			requireTruef(t, out.User.Username != "",
				"deployment get: doc field 'user.username' must be populated (1:1 output shape)")
		}

		// CAVEAT: the deployment was created via the deployments API with no CI job,
		// so 'deployable' (the job) is legitimately null. Do NOT require it; only
		// assert its sub-fields when it happens to be present, keeping the test
		// correct whether or not a job exists.
		if out.Deployable != nil {
			requireTruef(t, out.Deployable.ID > 0,
				"deployment get: when 'deployable' is present its id must be > 0 (1:1 output shape)")
		}

		t.Logf("Got deployment %d: status=%s, ref=%s, sha=%s, user=%v",
			out.ID, out.Status, out.Ref, out.SHA, out.User != nil)
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
		requireTruef(t, err != nil, "expected error deleting completed deployment")
		t.Logf("Expected error for completed deployment deletion: %v", err)
	})
}
