//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/freezeperiods"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/protectedenvs"
)

// TestMeta_EnvironmentsProtected exercises protected environment actions.
func TestMeta_EnvironmentsProtected(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	// Create an environment to protect
	envName := "staging-" + uniqueName("")
	_, err := callToolOn[struct{ Name string }](ctx, sess.meta, "gitlab_environment", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"name":       envName,
		},
	})
	requireNoError(t, err, "create environment")

	t.Run("ProtectedList", func(t *testing.T) {
		out, err := callToolOn[protectedenvs.ListOutput](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "protected_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "protected_list")
		t.Logf("Protected environments: %d", len(out.Environments))
	})

	t.Run("ProtectedProtect", func(t *testing.T) {
		out, err := callToolOn[protectedenvs.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "protected_protect",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       envName,
			},
		})
		requireNoError(t, err, "protected_protect")
		requireTrue(t, out.Name == envName, "protected_protect: name mismatch")
		t.Logf("Protected environment: %s", out.Name)
	})

	t.Run("ProtectedGet", func(t *testing.T) {
		out, err := callToolOn[protectedenvs.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "protected_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       envName,
			},
		})
		requireNoError(t, err, "protected_get")
		requireTrue(t, out.Name == envName, "protected_get: name mismatch")
	})

	t.Run("ProtectedUnprotect", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "protected_unprotect",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       envName,
			},
		})
		requireNoError(t, err, "protected_unprotect")
	})
}

// TestMeta_EnvironmentsFreeze exercises freeze period CRUD.
func TestMeta_EnvironmentsFreeze(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	var freezeID int64

	t.Run("FreezeList", func(t *testing.T) {
		out, err := callToolOn[freezeperiods.ListOutput](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "freeze_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "freeze_list")
		t.Logf("Freeze periods: %d", len(out.FreezePeriods))
	})

	t.Run("FreezeCreate", func(t *testing.T) {
		out, err := callToolOn[freezeperiods.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "freeze_create",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"freeze_start":  "0 23 * * 5",
				"freeze_end":    "0 7 * * 1",
				"cron_timezone": "UTC",
			},
		})
		requireNoError(t, err, "freeze_create")
		requireTrue(t, out.ID > 0, "freeze_create: expected ID > 0")
		freezeID = out.ID
		t.Logf("Created freeze period %d", freezeID)
	})
	defer func() {
		if freezeID > 0 {
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_environment", map[string]any{
				"action": "freeze_delete",
				"params": map[string]any{
					"project_id":       proj.pidStr(),
					"freeze_period_id": freezeID,
				},
			})
		}
	}()

	t.Run("FreezeGet", func(t *testing.T) {
		requireTrue(t, freezeID > 0, "freezeID not set")
		out, err := callToolOn[freezeperiods.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "freeze_get",
			"params": map[string]any{
				"project_id":       proj.pidStr(),
				"freeze_period_id": freezeID,
			},
		})
		requireNoError(t, err, "freeze_get")
		requireTrue(t, out.ID == freezeID, "freeze_get: ID mismatch")
	})

	t.Run("FreezeUpdate", func(t *testing.T) {
		requireTrue(t, freezeID > 0, "freezeID not set")
		out, err := callToolOn[freezeperiods.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "freeze_update",
			"params": map[string]any{
				"project_id":       proj.pidStr(),
				"freeze_period_id": freezeID,
				"freeze_start":     "0 22 * * 5",
			},
		})
		requireNoError(t, err, "freeze_update")
		requireTrue(t, out.ID == freezeID, "freeze_update: ID mismatch")
	})
}

// TestMeta_DeploymentsExtended exercises deployment CRUD via gitlab_deployment.
func TestMeta_DeploymentsExtended(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	// Create an environment for deployments
	envName := "production-" + uniqueName("")
	_, err := callToolOn[struct{ Name string }](ctx, sess.meta, "gitlab_environment", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"name":       envName,
		},
	})
	requireNoError(t, err, "create environment for deployments")

	t.Run("DeploymentList", func(t *testing.T) {
		out, err := callToolOn[deployments.ListOutput](ctx, sess.meta, "gitlab_deployment", map[string]any{
			"action": "list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "deployment list")
		t.Logf("Deployments: %d", len(out.Deployments))
	})

	t.Run("DeploymentCreate", func(t *testing.T) {
		commitFileMeta(ctx, t, sess.meta, proj, "main", "deploy.txt", "deploy content", "deployment commit")
		out, err := callToolOn[deployments.Output](ctx, sess.meta, "gitlab_deployment", map[string]any{
			"action": "create",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"environment": envName,
				"sha":         "main",
				"ref":         "main",
				"tag":         false,
				"status":      "success",
			},
		})
		requireNoError(t, err, "deployment create")
		requireTrue(t, out.ID > 0, "deployment create: expected ID > 0")
		t.Logf("Created deployment %d", out.ID)
	})
}
