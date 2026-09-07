//go:build e2e && !enterprise

// environments_ce_test.go tests the environment MCP tools against a live GitLab instance.
// Covers create, get, list, update, stop, and delete for both individual and meta-tool modes.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/environments"
)

// TestIndividual_Environments exercises the environment lifecycle using
// individual tools: create → get → list → update → stop → delete.
//
// The test creates a project fixture, then runs six subtests that drive
// the gitlab_create_environment, gitlab_get_environment,
// gitlab_list_environments, gitlab_update_environment, gitlab_stop_
// environment, and gitlab_delete_environment tools. Each subtest asserts
// the expected ID or name round-trips through the GitLab API.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_Environments(t *testing.T) {
	if !sess.enterprise {
		t.Parallel()
	}
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := e2eTimeoutContext(120*time.Second, 300*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	var envID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[environments.Output](ctx, sess.individual, "gitlab_environment_create", environments.CreateInput{
			ProjectID: proj.pidOf(),
			Name:      "e2e-staging",
		})
		requireNoError(t, err, "create environment")
		requireTruef(t, out.Name == "e2e-staging", "expected name e2e-staging, got %s", out.Name)
		envID = out.ID
		t.Logf("Created environment %s (ID=%d)", out.Name, out.ID)
	})

	t.Run("Get", func(t *testing.T) {
		requireTruef(t, envID > 0, "envID not set")
		out, err := callToolOn[environments.Output](ctx, sess.individual, "gitlab_environment_get", environments.GetInput{
			ProjectID:     proj.pidOf(),
			EnvironmentID: envID,
		})
		requireNoError(t, err, "get environment")
		requireTruef(t, out.ID == envID, "expected ID %d, got %d", envID, out.ID)
		t.Logf("Got environment %s", out.Name)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[environments.ListOutput](ctx, sess.individual, "gitlab_environment_list", environments.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list environments")
		requireTruef(t, len(out.Environments) >= 1, "expected at least 1 environment, got %d", len(out.Environments))
		t.Logf("Listed %d environments", len(out.Environments))
	})

	t.Run("Update", func(t *testing.T) {
		requireTruef(t, envID > 0, "envID not set")
		out, err := callToolOn[environments.Output](ctx, sess.individual, "gitlab_environment_update", environments.UpdateInput{
			ProjectID:     proj.pidOf(),
			EnvironmentID: envID,
			ExternalURL:   "https://staging.example.com",
		})
		requireNoError(t, err, "update environment")
		requireTruef(t, out.ExternalURL == "https://staging.example.com", "expected external URL")
		t.Logf("Updated environment %s", out.Name)
	})

	t.Run("Stop", func(t *testing.T) {
		requireTruef(t, envID > 0, "envID not set")
		out, err := callToolOn[environments.Output](ctx, sess.individual, "gitlab_environment_stop", environments.StopInput{
			ProjectID:     proj.pidOf(),
			EnvironmentID: envID,
		})
		requireNoError(t, err, "stop environment")
		requireTruef(t, out.ID == envID, "stop answered for environment %d, want %d", out.ID, envID)
		// The fixture environment has no stop action job, so GitLab completes the
		// stop in the request rather than leaving it "stopping".
		requireTruef(t, out.State == "stopped", "environment %d state = %q after stopping, want %q", envID, out.State, "stopped")
		t.Logf("Stopped environment %s (state=%s)", out.Name, out.State)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTruef(t, envID > 0, "envID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_environment_delete", environments.DeleteInput{
			ProjectID:     proj.pidOf(),
			EnvironmentID: envID,
		})
		requireNoError(t, err, "delete environment")
		requireGoneOn(ctx, t, sess.individual, "environment after delete", "gitlab_environment_get",
			environments.GetInput{ProjectID: proj.pidOf(), EnvironmentID: envID})
		t.Log("Deleted environment")
	})
}

// TestMeta_Environments exercises the same environment lifecycle via the
// gitlab_environment meta-tool.
//
// The test mirrors [TestIndividual_Environments] but drives every step
// with {action, params} arguments through the catalog-backed
// gitlab_environment tool. Each subtest asserts the same outcome and
// verifies the tool name stays constant across the lifecycle.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_Environments(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var envID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[environments.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "create",
			"params": map[string]any{"project_id": proj.pidStr(), "name": "e2e-meta-staging"},
		})
		requireNoError(t, err, "meta create environment")
		requireTruef(t, out.Name == "e2e-meta-staging", "expected name e2e-meta-staging")
		envID = out.ID
		t.Logf("Created environment %s (ID=%d) via meta-tool", out.Name, out.ID)
	})

	t.Run("Get", func(t *testing.T) {
		requireTruef(t, envID > 0, "envID not set")
		out, err := callToolOn[environments.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "get",
			"params": map[string]any{"project_id": proj.pidStr(), "environment_id": envID},
		})
		requireNoError(t, err, "meta get environment")
		requireTruef(t, out.ID == envID, "expected ID %d", envID)
		t.Logf("Got environment %s via meta-tool", out.Name)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[environments.ListOutput](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta list environments")
		requireTruef(t, len(out.Environments) >= 1, "expected at least 1 environment")
		t.Logf("Listed %d environments via meta-tool", len(out.Environments))
	})

	t.Run("Update", func(t *testing.T) {
		requireTruef(t, envID > 0, "envID not set")
		out, err := callToolOn[environments.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "update",
			"params": map[string]any{"project_id": proj.pidStr(), "environment_id": envID, "external_url": "https://meta-staging.example.com"},
		})
		requireNoError(t, err, "meta update environment")
		requireTruef(t, out.ID == envID, "update answered for environment %d, want %d", out.ID, envID)
		requireTruef(t, out.ExternalURL == "https://meta-staging.example.com",
			"environment external_url = %q, want the updated one", out.ExternalURL)
		t.Logf("Updated environment %s via meta-tool", out.Name)
	})

	t.Run("Stop", func(t *testing.T) {
		requireTruef(t, envID > 0, "envID not set")
		out, err := callToolOn[environments.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "stop",
			"params": map[string]any{"project_id": proj.pidStr(), "environment_id": envID},
		})
		requireNoError(t, err, "meta stop environment")
		requireTruef(t, out.State == "stopped", "environment %d state = %q after stopping, want %q", envID, out.State, "stopped")
		t.Logf("Stopped environment %s via meta-tool", out.Name)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTruef(t, envID > 0, "envID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "delete",
			"params": map[string]any{"project_id": proj.pidStr(), "environment_id": envID},
		})
		requireNoError(t, err, "meta delete environment")
		requireGoneOn(ctx, t, sess.meta, "environment after delete", "gitlab_environment", map[string]any{
			"action": "get",
			"params": map[string]any{"project_id": proj.pidStr(), "environment_id": envID},
		})
		t.Log("Deleted environment via meta-tool")
	})
}
