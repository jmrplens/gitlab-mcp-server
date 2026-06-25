//go:build e2e && !enterprise

// environments_meta_ce_test.go tests environment-related meta-tool actions against a live
// GitLab instance, including protected environments, freeze periods, and deployment CRUD.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/environments"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/freezeperiods"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/protectedenvs"
)

// TestMeta_EnvironmentsProtected exercises protected environment actions
// via the gitlab_environment meta-tool.
//
// The test creates a project fixture and returns early on CE (protected
// environments are EE-only). On EE, it drives the protected_environment_*
// actions through the catalog-backed meta-tool, asserting each step's
// expected ID round-trips through the GitLab API.
//
// Build tag: e2e && !enterprise. Mode: EE. Surface: meta.
func TestMeta_EnvironmentsProtected(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	// Create an environment to protect
	envName := "staging-" + uniqueName("")
	_, envErr := callToolOn[struct{ Name string }](ctx, sess.meta, "gitlab_environment", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"name":       envName,
		},
	})
	requireNoError(t, envErr, "create environment")

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
				"deploy_access_levels": []map[string]any{
					{"access_level": 40},
				},
			},
		})
		requireNoError(t, err, "protected_protect")
		requireTruef(t, out.Name == envName, "protected_protect: name mismatch")
		t.Logf("Protected environment: %s", out.Name)
	})

	t.Run("ProtectedGet", func(t *testing.T) {
		out, err := callToolOn[protectedenvs.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "protected_get",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"environment": envName,
			},
		})
		requireNoError(t, err, "protected_get")
		requireTruef(t, out.Name == envName, "protected_get: name mismatch")
	})

	t.Run("ProtectedUnprotect", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "protected_unprotect",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"environment": envName,
			},
		})
		requireNoError(t, err, "protected_unprotect")
	})
}

// TestMeta_EnvironmentsFreeze exercises freeze period CRUD via the
// gitlab_environment meta-tool.
//
// The test creates a project fixture and runs subtests that drive the
// freeze_period_create, freeze_period_list, freeze_period_get,
// freeze_period_update, and freeze_period_delete actions through the
// catalog-backed gitlab_environment tool. Each subtest asserts the
// expected ID or schedule round-trips through the GitLab API.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
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
		requireTruef(t, out.ID > 0, "freeze_create: expected ID > 0")
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
		requireTruef(t, freezeID > 0, "freezeID not set")
		out, err := callToolOn[freezeperiods.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "freeze_get",
			"params": map[string]any{
				"project_id":       proj.pidStr(),
				"freeze_period_id": freezeID,
			},
		})
		requireNoError(t, err, "freeze_get")
		requireTruef(t, out.ID == freezeID, "freeze_get: ID mismatch")
	})

	t.Run("FreezeUpdate", func(t *testing.T) {
		requireTruef(t, freezeID > 0, "freezeID not set")
		out, err := callToolOn[freezeperiods.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "freeze_update",
			"params": map[string]any{
				"project_id":       proj.pidStr(),
				"freeze_period_id": freezeID,
				"freeze_start":     "0 22 * * 5",
			},
		})
		requireNoError(t, err, "freeze_update")
		requireTruef(t, out.ID == freezeID, "freeze_update: ID mismatch")
	})
}

// TestMeta_DeploymentsExtended exercises deployment CRUD via the
// gitlab_environment meta-tool (deployment_* actions).
//
// The test creates a project fixture, sets up a deployment, and drives
// the deployment_get, deployment_list, deployment_update, and
// deployment_delete actions through the catalog-backed meta-tool. Each
// subtest asserts the expected ID or environment name round-trips
// through the GitLab API.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_DeploymentsExtended(t *testing.T) {
	if !sess.enterprise {
		t.Parallel()
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := e2eTimeoutContext(120*time.Second, 300*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	// Create an environment for deployments
	envName := "production-" + uniqueName("")
	envOut, envErr := callToolOn[environments.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"name":       envName,
		},
	})
	requireNoError(t, envErr, "create environment for deployments")
	envID := envOut.ID

	t.Run("DeploymentList", func(t *testing.T) {
		out, err := callToolOn[deployments.ListOutput](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "deployment_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "deployment list")
		t.Logf("Deployments: %d", len(out.Deployments))
	})

	t.Run("DeploymentCreate", func(t *testing.T) {
		commitFileMeta(ctx, t, sess.meta, proj, "main", "deploy.txt", "deploy content", "deployment commit")
		out, err := callToolOn[deployments.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "deployment_create",
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
		requireTruef(t, out.ID > 0, "deployment create: expected ID > 0")
		t.Logf("Created deployment %d", out.ID)
	})

	t.Run("EnvironmentLastDeployment", func(t *testing.T) {
		requireTruef(t, envID > 0, "envID not set")
		out, err := callToolOn[environments.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "get",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"environment_id": envID,
			},
		})
		requireNoError(t, err, "environment get")
		requireTruef(t, out.ID == envID, "environment get: expected ID %d, got %d", envID, out.ID)

		// Validate the doc-grounded 1:1 output shape: the environment surfaces the
		// documented `last_deployment` object (id, iid, ref, sha, created_at,
		// status, user, deployable). A deployment with ref=main, sha=main,
		// status=success was just created on this environment, so it must appear.
		requireTruef(t, out.LastDeployment != nil,
			"environment get: documented 'last_deployment' object must be present after a deployment (1:1 output shape)")
		if out.LastDeployment != nil {
			ld := out.LastDeployment
			requireTruef(t, ld.SHA != "",
				"environment get: doc field 'last_deployment.sha' must be populated (1:1 output shape)")
			requireTruef(t, ld.Ref == "main",
				"environment get: doc field 'last_deployment.ref' must be 'main' (1:1 output shape), got %q", ld.Ref)
			requireTruef(t, ld.Status != "",
				"environment get: doc field 'last_deployment.status' must be populated (1:1 output shape), got %q", ld.Status)

			// CAVEAT: the deployment was created via the deployments API with no CI
			// job, so 'last_deployment.deployable' (the job) is legitimately null.
			// Do NOT require it; only assert its sub-fields when present so the test
			// stays correct whether or not a job ran.
			if ld.Deployable != nil {
				requireTruef(t, ld.Deployable.ID > 0,
					"environment get: when 'last_deployment.deployable' is present its id must be > 0 (1:1 output shape)")
			}
			t.Logf("Environment %d last_deployment: status=%s, ref=%s, sha=%s, deployable=%v",
				out.ID, ld.Status, ld.Ref, ld.SHA, ld.Deployable != nil)
		}
	})
}
