//go:build e2e && enterprise

// runner_meta_ee_test.go tests runner management via the gitlab_runner meta-tool
// against a live GitLab EE/Ultimate instance. Exercises list, get, enable, disable,
// list_managers, and controller operations with a real Docker runner. Uses the runner
// registered by scripts/register-runner.sh.
//
// NOT parallelized: runner tests share a single registered runner; running them
// concurrently causes queue contention and spurious timeouts.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/runnercontrollers"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/runners"
)

// TestEE_MetaRunnerManagement exercises runner management on GitLab EE/Ultimate.
// Uses the Docker-registered runner (description "e2e-docker-runner") created
// by scripts/register-runner.sh. Validates that runner operations work correctly
// with an Enterprise-tier GitLab instance.
func TestEE_MetaRunnerManagement(t *testing.T) {
	if !sess.enterprise {
		t.Skip("EE runner management requires GitLab EE/Ultimate")
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	RunWithCapabilities(t, []Capability{CapabilityRunner}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		proj := createProjectMeta(ctx, t, sess.meta)
		runnerID := findDockerRunnerID(ctx, t)
		t.Logf("Docker runner ID: %d", runnerID)

		t.Run("ListAll", func(t *testing.T) {
			out, err := callToolOn[runners.ListOutput](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "list_all",
				"params": map[string]any{},
			})
			requireNoError(t, err, "list all runners")
			requireTruef(t, len(out.Runners) >= 1, "expected at least 1 runner, got %d", len(out.Runners))
			t.Logf("Listed %d runner(s)", len(out.Runners))
		})

		t.Run("ListProject", func(t *testing.T) {
			out, err := callToolOn[runners.ListOutput](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "list_project",
				"params": map[string]any{"project_id": proj.pidStr()},
			})
			requireNoError(t, err, "list project runners")
			t.Logf("Project %s has %d runner(s)", proj.Path, len(out.Runners))
		})

		t.Run("ListGroup", func(t *testing.T) {
			_, err := callToolOn[runners.ListOutput](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "list_group",
				"params": map[string]any{"group_id": "0"},
			})
			if err != nil && !isHTTPStatus(err, 404) {
				requireNoError(t, err, "list group runners")
			}
			t.Log("list_group routed OK")
		})

		t.Run("Get", func(t *testing.T) {
			out, err := callToolOn[runners.DetailsOutput](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "get",
				"params": map[string]any{"runner_id": runnerID},
			})
			requireNoError(t, err, "get runner")
			requireTruef(t, out.ID == runnerID, "runner ID mismatch: got %d, want %d", out.ID, runnerID)
			t.Logf("Got runner: ID=%d name=%q paused=%v shared=%v", out.ID, out.Name, out.Paused, out.IsShared)
		})

		t.Run("EnableProject_GracefulError", func(t *testing.T) {
			// Shared runners (instance-type) may not be assignable per-project in EE.
			_, err := callToolOn[runners.Output](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "enable_project",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"runner_id":  runnerID,
				},
			})
			if err != nil {
				t.Logf("enable_project error (shared runner may not support per-project assignment): %v", err)
			} else {
				t.Logf("Enabled runner %d on project %s", runnerID, proj.Path)
			}
		})

		t.Run("DisableProject_GracefulError", func(t *testing.T) {
			err := callToolVoidOn(ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "disable_project",
				"params": map[string]any{
					"project_id": proj.pidStr(),
					"runner_id":  runnerID,
				},
			})
			if err != nil {
				t.Logf("disable_project error (expected for unassigned shared runner): %v", err)
			} else {
				t.Logf("Disabled runner %d on project %s", runnerID, proj.Path)
			}
		})

		t.Run("ListManagers", func(t *testing.T) {
			out, err := callToolOn[runners.ManagerListOutput](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "list_managers",
				"params": map[string]any{"runner_id": runnerID},
			})
			requireNoError(t, err, "list runner managers")
			t.Logf("Runner managers: %d", len(out.Managers))
		})

		t.Run("ControllerList", func(t *testing.T) {
			out, err := callToolOn[runnercontrollers.ListOutput](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "controller_list",
				"params": map[string]any{},
			})
			if err != nil && !isHTTPStatus(err, 404) {
				requireNoError(t, err, "controller list")
			}
			t.Logf("Runner controllers: %d", len(out.Controllers))
		})

		t.Run("ControllerGet_Graceful404", func(t *testing.T) {
			_, err := callToolOn[runnercontrollers.Output](ctx, sess.meta, "gitlab_runner", map[string]any{
				"action": "controller_get",
				"params": map[string]any{"controller_id": 999999},
			})
			// 404 is expected for a non-existent controller ID. If no error is
			// returned the API has changed and this test needs to be updated.
			if err == nil {
				t.Fatal("controller_get for non-existent controller_id returned no error; expected 404")
			}
			if !isHTTPStatus(err, 404) {
				requireNoError(t, err, "controller get")
			}
			t.Log("controller_get 404 handled gracefully")
		})
	})
}

// findDockerRunnerID queries the instance runner list and returns the ID of
// the "e2e-docker-runner" registered by scripts/register-runner.sh.
func findDockerRunnerID(ctx context.Context, t *testing.T) int64 {
	t.Helper()
	out, err := callToolOn[runners.ListOutput](ctx, sess.meta, "gitlab_runner", map[string]any{
		"action": "list_all",
		"params": map[string]any{},
	})
	requireNoError(t, err, "list all runners to find docker runner")
	for _, r := range out.Runners {
		if r.Description == "e2e-docker-runner" {
			return r.ID
		}
	}
	t.Fatalf("e2e-docker-runner not found in runner list: got %d runners", len(out.Runners))
	return 0
}
