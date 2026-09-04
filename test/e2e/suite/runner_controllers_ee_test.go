//go:build e2e && enterprise

// runner_controllers_ee_test.go covers the runner controller lifecycle
// actions the e2e gap audit reported as unexercised: controller_create,
// controller_update, controller_delete, the controller token family
// (create/get/list/rotate/revoke), and the controller scope family
// (list/add_instance/remove_instance/add_runner/remove_runner), all via the
// gitlab_runner meta-tool. Runner controllers are an experimental,
// admin-only Ultimate API; when the pinned EE image predates the endpoints
// the create probe returns 404 and the whole test skips with a
// version-gated reason. controller_list and controller_get are already
// covered by runner_meta_ee_test.go.
//
// NOT parallelized: the scope subtests mutate the shared instance runner
// registered by scripts/register-runner.sh.
//
// Build tag: e2e && enterprise.
package suite

import (
	"context"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/runnercontrollers"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/runnercontrollerscopes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/runnercontrollertokens"
)

// runnerCtlFindRunnerID returns the ID of any instance-level runner, or 0
// when none is registered. The runner-scope subtests need a real runner ID;
// non-Docker EE runs may have none, in which case those subtests skip.
func runnerCtlFindRunnerID(ctx context.Context, t *testing.T) int64 {
	t.Helper()
	runners, _, err := sess.glClient.GL().Runners.ListAllRunners(&gl.ListRunnersOptions{}, gl.WithContext(ctx))
	if err != nil {
		t.Logf("listing instance runners failed (runner scope subtests will skip): %v", err)
		return 0
	}
	if len(runners) == 0 {
		return 0
	}
	// Prefer the compose-registered runner by its known description so a
	// leftover throwaway runner from a previous partial run cannot win by
	// list ordering.
	for _, runner := range runners {
		if runner.Description == "e2e-docker-runner" {
			return runner.ID
		}
	}
	return runners[0].ID
}

// runnerCtlScopeConflict reports whether err is the benign "scope already
// assigned" outcome of an add-scope call on a controller that GitLab
// auto-scoped at creation time.
func runnerCtlScopeConflict(err error) bool {
	if err == nil {
		return false
	}
	return isHTTPStatus(err, 409) || strings.Contains(strings.ToLower(err.Error()), "already")
}

// TestMeta_RunnerControllerLifecycle exercises the runner controller CRUD,
// token, and scope actions via the gitlab_runner meta-tool against a live
// GitLab Ultimate instance.
//
// The test creates a controller (probing API availability), updates it,
// walks the token lifecycle (create, list, get, rotate, revoke), walks the
// scope lifecycle (list, add/remove instance scope, add/remove runner scope
// against the Docker-registered runner when present), and finally deletes
// the controller. Every subtest asserts the returned IDs round-trip. When
// the pinned EE image lacks the runner controllers API the create probe
// returns 404 and the test skips as a group with the version-gated reason.
//
// Build tag: e2e && enterprise. Mode: EE. Surface: meta. Admin token required.
func TestMeta_RunnerControllerLifecycle(t *testing.T) {
	if !sess.enterprise {
		return
	}
	RunWithCapabilities(t, []Capability{CapabilityAdmin}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		// Availability probe doubles as the create coverage: a 404 here
		// means the whole controller API family does not exist yet.
		createOut, createErr := callToolOn[runnercontrollers.Output](ctx, sess.meta, "gitlab_runner", map[string]any{
			"action": "controller_create",
			"params": map[string]any{
				"description": uniqueName("e2e-runner-ctl"),
				"state":       "disabled",
			},
		})
		if createErr != nil && isHTTPStatus(createErr, 404) {
			t.Skipf("runner controllers API unavailable on this GitLab version (404) — version-gated skip for the whole controller family: %v", createErr)
		}
		requireNoError(t, createErr, "controller_create")
		requireTruef(t, createOut.ID > 0, "controller ID should be positive, got %d", createOut.ID)
		controllerID := createOut.ID
		t.Logf("Created runner controller %d (state %s)", controllerID, createOut.State)

		controllerDeleted := false
		t.Cleanup(func() {
			if controllerDeleted {
				return
			}
			cctx, ccancel := cleanupContext(defaultCleanupTimeout)
			defer ccancel()
			_ = callToolVoidOn(cctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "controller_delete",
				"params": map[string]any{"controller_id": controllerID},
			})
		})

		t.Run("ControllerUpdate", func(t *testing.T) {
			out, err := callToolOn[runnercontrollers.Output](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "controller_update",
				"params": map[string]any{
					"controller_id": controllerID,
					"description":   uniqueName("e2e-runner-ctl-updated"),
					"state":         "dry_run",
				},
			})
			requireNoError(t, err, "controller_update")
			requireTruef(t, out.ID == controllerID, "controller ID = %d, want %d", out.ID, controllerID)
			requireTruef(t, out.State == "dry_run", "controller state = %q, want dry_run", out.State)
			t.Logf("Updated runner controller %d to state %s", out.ID, out.State)
		})

		var tokenID int64

		t.Run("TokenCreate", func(t *testing.T) {
			out, err := callToolOn[runnercontrollertokens.Output](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "controller_token_create",
				"params": map[string]any{
					"controller_id": controllerID,
					"description":   "E2E controller token",
				},
			})
			requireNoError(t, err, "controller_token_create")
			requireTruef(t, out.ID > 0, "token ID should be positive, got %d", out.ID)
			tokenID = out.ID
			t.Logf("Created controller token %d (secret returned: %v)", out.ID, out.Token != "")
		})

		t.Run("TokenList", func(t *testing.T) {
			requireTruef(t, tokenID > 0, "tokenID not set by the create subtest")
			out, err := callToolOn[runnercontrollertokens.ListOutput](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "controller_token_list",
				"params": map[string]any{"controller_id": controllerID},
			})
			requireNoError(t, err, "controller_token_list")
			requireTruef(t, len(out.Tokens) >= 1, "expected at least 1 controller token, got %d", len(out.Tokens))
			t.Logf("Controller %d has %d token(s)", controllerID, len(out.Tokens))
		})

		t.Run("TokenGet", func(t *testing.T) {
			requireTruef(t, tokenID > 0, "tokenID not set by the create subtest")
			out, err := callToolOn[runnercontrollertokens.Output](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "controller_token_get",
				"params": map[string]any{
					"controller_id": controllerID,
					"token_id":      tokenID,
				},
			})
			requireNoError(t, err, "controller_token_get")
			requireTruef(t, out.ID == tokenID, "token ID = %d, want %d", out.ID, tokenID)
		})

		t.Run("TokenRotate", func(t *testing.T) {
			requireTruef(t, tokenID > 0, "tokenID not set by the create subtest")
			out, err := callToolOn[runnercontrollertokens.Output](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "controller_token_rotate",
				"params": map[string]any{
					"controller_id": controllerID,
					"token_id":      tokenID,
				},
			})
			requireNoError(t, err, "controller_token_rotate")
			requireTruef(t, out.ID > 0, "rotated token ID should be positive, got %d", out.ID)
			// Rotation may mint a new token record; track the current ID so
			// the revoke below always targets a live token.
			tokenID = out.ID
			t.Logf("Rotated controller token, current token ID %d", tokenID)
		})

		t.Run("TokenRevoke", func(t *testing.T) {
			requireTruef(t, tokenID > 0, "tokenID not set by the create subtest")
			err := callToolVoidOn(ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "controller_token_revoke",
				"params": map[string]any{
					"controller_id": controllerID,
					"token_id":      tokenID,
				},
			})
			requireNoError(t, err, "controller_token_revoke")
			t.Logf("Revoked controller token %d", tokenID)
		})

		t.Run("ScopeList", func(t *testing.T) {
			out, err := callToolOn[runnercontrollerscopes.ScopesOutput](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "controller_scope_list",
				"params": map[string]any{"controller_id": controllerID},
			})
			requireNoError(t, err, "controller_scope_list")
			t.Logf("Controller %d scopes: %d instance, %d runner",
				controllerID, len(out.InstanceLevelScopings), len(out.RunnerLevelScopings))
		})

		t.Run("ScopeAddInstance", func(t *testing.T) {
			_, err := callToolOn[runnercontrollerscopes.InstanceScopeOutput](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "controller_scope_add_instance",
				"params": map[string]any{"controller_id": controllerID},
			})
			if runnerCtlScopeConflict(err) {
				t.Logf("instance scope already assigned (acceptable conflict outcome): %v", err)
				return
			}
			requireNoError(t, err, "controller_scope_add_instance")
			t.Log("Added instance scope")
		})

		t.Run("ScopeRemoveInstance", func(t *testing.T) {
			err := callToolVoidOn(ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "controller_scope_remove_instance",
				"params": map[string]any{"controller_id": controllerID},
			})
			requireNoError(t, err, "controller_scope_remove_instance")
			t.Log("Removed instance scope")
		})

		runnerID := runnerCtlFindRunnerID(ctx, t)

		t.Run("ScopeAddRunner", func(t *testing.T) {
			if runnerID == 0 {
				t.Skip("no instance runner registered; runner-scope coverage requires the Docker runner")
			}
			out, err := callToolOn[runnercontrollerscopes.RunnerScopeOutput](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "controller_scope_add_runner",
				"params": map[string]any{
					"controller_id": controllerID,
					"runner_id":     runnerID,
				},
			})
			if runnerCtlScopeConflict(err) {
				t.Logf("runner scope already assigned (acceptable conflict outcome): %v", err)
				return
			}
			requireNoError(t, err, "controller_scope_add_runner")
			requireTruef(t, out.RunnerID == runnerID, "scoped runner ID = %d, want %d", out.RunnerID, runnerID)
			t.Logf("Added runner %d to controller scope", runnerID)
		})

		t.Run("ScopeRemoveRunner", func(t *testing.T) {
			if runnerID == 0 {
				t.Skip("no instance runner registered; runner-scope coverage requires the Docker runner")
			}
			err := callToolVoidOn(ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "controller_scope_remove_runner",
				"params": map[string]any{
					"controller_id": controllerID,
					"runner_id":     runnerID,
				},
			})
			requireNoError(t, err, "controller_scope_remove_runner")
			t.Logf("Removed runner %d from controller scope", runnerID)
		})

		t.Run("ControllerDelete", func(t *testing.T) {
			err := callToolVoidOn(ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "controller_delete",
				"params": map[string]any{"controller_id": controllerID},
			})
			requireNoError(t, err, "controller_delete")
			controllerDeleted = true
			t.Logf("Deleted runner controller %d", controllerID)
		})
	})
}
