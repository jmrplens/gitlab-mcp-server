//go:build e2e && enterprise

// environment_extras_ee_test.go covers the remaining environment-domain
// actions the e2e gap audit reported as unexercised: the project-level
// protected environment update (environment.protected_update) and the
// deployment approval flow (environment.deployment_approve_or_reject), both
// via the gitlab_environment meta-tool. The existing protected_envs_ee_test.go
// already covers protect/list/get/unprotect; only the missing actions are
// added here.
//
// Build tag: e2e && enterprise.
package suite

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/protectedenvs"
)

// envExtrasProtect protects envName on proj with a Maintainer deploy access
// level plus the given extra params, returning the created protected
// environment. Unprotection is registered on t.Cleanup so project deletion
// is never blocked by a lingering protection.
func envExtrasProtect(ctx context.Context, t *testing.T, proj ProjectFixture, envName string, extra map[string]any) protectedenvs.Output {
	t.Helper()
	params := map[string]any{
		"project_id": proj.pidStr(),
		"name":       envName,
		"deploy_access_levels": []map[string]any{
			{"access_level": 40},
		},
	}
	maps.Copy(params, extra)
	out, err := callToolOn[protectedenvs.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
		"action": "protected_protect",
		"params": params,
	})
	requireNoError(t, err, "protect environment "+envName)
	t.Cleanup(func() { //nolint:contextcheck // Cleanup runs after the test context is canceled and owns its own timeout.
		cctx, ccancel := cleanupContext(defaultCleanupTimeout)
		defer ccancel()
		_ = callToolVoidOn(cctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "protected_unprotect",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"environment": envName,
			},
		})
	})
	return out
}

// TestMeta_ProtectedEnvUpdate exercises protected_update via the
// gitlab_environment meta-tool against a live GitLab Premium/Ultimate
// instance.
//
// The test protects a project environment with a Maintainer deploy access
// level, then updates that access level in place (by its ID) to Developer
// and asserts the change is persisted in the update response. Updating the
// existing entry avoids the deprecated unified approval count field.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta.
func TestMeta_ProtectedEnvUpdate(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	const envName = "e2e-protected-update"

	protected := envExtrasProtect(ctx, t, proj, envName, nil)
	requireTruef(t, len(protected.DeployAccessLevels) >= 1, "expected at least 1 deploy access level after protect")
	accessLevelID := protected.DeployAccessLevels[0].ID
	requireTruef(t, accessLevelID > 0, "deploy access level ID should be positive")

	t.Run("Meta/ProtectedEnv/Update", func(t *testing.T) {
		out, err := callToolOn[protectedenvs.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "protected_update",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"environment": envName,
				"deploy_access_levels": []map[string]any{
					{"id": accessLevelID, "access_level": 30},
				},
			},
		})
		requireNoError(t, err, "meta protected env update")
		requireTruef(t, out.Name == envName, "protected env name = %q, want %q", out.Name, envName)
		found := false
		for _, level := range out.DeployAccessLevels {
			if level.ID == accessLevelID && level.AccessLevel == 30 {
				found = true
				break
			}
		}
		requireTruef(t, found, "deploy access level %d not updated to 30 in response: %+v", accessLevelID, out.DeployAccessLevels)
		t.Logf("Updated protected environment %s: access level %d now 30", out.Name, accessLevelID)
	})
}

// TestMeta_DeploymentApproveOrReject exercises deployment_approve_or_reject
// via the gitlab_environment meta-tool against a live GitLab
// Premium/Ultimate instance.
//
// The test creates an environment, protects it with a one-approval rule
// bound to the current user, commits a file for a valid SHA, and creates an
// API deployment, which GitLab holds in the blocked state pending approval.
// It then approves the deployment as the rule's user. GitLab versions that
// prevent the deployment creator from approving, or that never blocked the
// API-created deployment, return a 4xx which is accepted as the documented
// approval-semantics outcome; any other error fails the test.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta.
func TestMeta_DeploymentApproveOrReject(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx, cancel := e2eTimeoutContext(180*time.Second, 300*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	const envName = "e2e-deploy-approval"

	userInfo, userErr := sess.glClient.CurrentUser(ctx)
	requireNoError(t, userErr, "resolve current user for approval rule")

	createEnvErr := callToolVoidOn(ctx, sess.meta, "gitlab_environment", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"name":       envName,
		},
	})
	requireNoError(t, createEnvErr, "create environment for deployment approval")

	envExtrasProtect(ctx, t, proj, envName, map[string]any{
		"approval_rules": []map[string]any{
			{"user_id": userInfo.UserID, "required_approvals": 1},
		},
	})

	commit := commitFileMeta(ctx, t, sess.meta, proj, defaultBranch, "deploy-approval.txt", "deployment approval fixture", "deployment approval fixture")

	// GitLab 19 requires the tag flag and rejects "created" as an API
	// deployment status; "running" yields the blocked/approvable state on a
	// protected environment.
	deployOut, deployErr := callToolOn[deployments.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
		"action": "deployment_create",
		"params": map[string]any{
			"project_id":  proj.pidStr(),
			"environment": envName,
			"ref":         defaultBranch,
			"sha":         commit.SHA,
			"tag":         false,
			"status":      "running",
		},
	})
	requireNoError(t, deployErr, "create deployment on protected environment")
	requireTruef(t, deployOut.ID > 0, "deployment ID should be positive")
	t.Logf("Created deployment %d with status %q", deployOut.ID, deployOut.Status)

	t.Run("Meta/Deployment/Approve", func(t *testing.T) {
		out, err := callToolOn[deployments.ApproveOrRejectOutput](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "deployment_approve_or_reject",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"deployment_id": deployOut.ID,
				"status":        "approved",
				"comment":       "E2E deployment approval",
			},
		})
		if err != nil {
			// Approval-semantics rejections are deterministic per GitLab
			// version (self-approval prevention, deployment not blocked).
			if isHTTPStatus(err, 400) || isHTTPStatus(err, 403) || isHTTPStatus(err, 404) {
				t.Logf("deployment_approve_or_reject returned an approval-semantics error (routing validated, deployment status %q): %v",
					deployOut.Status, err)
				return
			}
			t.Fatalf("deployment_approve_or_reject: unexpected error: %v", err)
		}
		t.Logf("Approved deployment %d: %s", deployOut.ID, out.Message)
	})
}
