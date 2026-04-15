//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/environments"
)

func TestIndividual_Environments(t *testing.T) {
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	var envID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[environments.Output](ctx, sess.individual, "gitlab_environment_create", environments.CreateInput{
			ProjectID: proj.pidOf(),
			Name:      "e2e-staging",
		})
		requireNoError(t, err, "create environment")
		requireTrue(t, out.Name == "e2e-staging", "expected name e2e-staging, got %s", out.Name)
		envID = out.ID
		t.Logf("Created environment %s (ID=%d)", out.Name, out.ID)
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, envID > 0, "envID not set")
		out, err := callToolOn[environments.Output](ctx, sess.individual, "gitlab_environment_get", environments.GetInput{
			ProjectID:     proj.pidOf(),
			EnvironmentID: envID,
		})
		requireNoError(t, err, "get environment")
		requireTrue(t, out.ID == envID, "expected ID %d, got %d", envID, out.ID)
		t.Logf("Got environment %s", out.Name)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[environments.ListOutput](ctx, sess.individual, "gitlab_environment_list", environments.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list environments")
		requireTrue(t, len(out.Environments) >= 1, "expected at least 1 environment, got %d", len(out.Environments))
		t.Logf("Listed %d environments", len(out.Environments))
	})

	t.Run("Update", func(t *testing.T) {
		requireTrue(t, envID > 0, "envID not set")
		out, err := callToolOn[environments.Output](ctx, sess.individual, "gitlab_environment_update", environments.UpdateInput{
			ProjectID:     proj.pidOf(),
			EnvironmentID: envID,
			ExternalURL:   "https://staging.example.com",
		})
		requireNoError(t, err, "update environment")
		requireTrue(t, out.ExternalURL == "https://staging.example.com", "expected external URL")
		t.Logf("Updated environment %s", out.Name)
	})

	t.Run("Stop", func(t *testing.T) {
		requireTrue(t, envID > 0, "envID not set")
		out, err := callToolOn[environments.Output](ctx, sess.individual, "gitlab_environment_stop", environments.StopInput{
			ProjectID:     proj.pidOf(),
			EnvironmentID: envID,
		})
		requireNoError(t, err, "stop environment")
		t.Logf("Stopped environment %s (state=%s)", out.Name, out.State)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, envID > 0, "envID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_environment_delete", environments.DeleteInput{
			ProjectID:     proj.pidOf(),
			EnvironmentID: envID,
		})
		requireNoError(t, err, "delete environment")
		t.Log("Deleted environment")
	})
}

func TestMeta_Environments(t *testing.T) {
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
		requireTrue(t, out.Name == "e2e-meta-staging", "expected name e2e-meta-staging")
		envID = out.ID
		t.Logf("Created environment %s (ID=%d) via meta-tool", out.Name, out.ID)
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, envID > 0, "envID not set")
		out, err := callToolOn[environments.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "get",
			"params": map[string]any{"project_id": proj.pidStr(), "environment_id": envID},
		})
		requireNoError(t, err, "meta get environment")
		requireTrue(t, out.ID == envID, "expected ID %d", envID)
		t.Logf("Got environment %s via meta-tool", out.Name)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[environments.ListOutput](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta list environments")
		requireTrue(t, len(out.Environments) >= 1, "expected at least 1 environment")
		t.Logf("Listed %d environments via meta-tool", len(out.Environments))
	})

	t.Run("Update", func(t *testing.T) {
		requireTrue(t, envID > 0, "envID not set")
		out, err := callToolOn[environments.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "update",
			"params": map[string]any{"project_id": proj.pidStr(), "environment_id": envID, "external_url": "https://meta-staging.example.com"},
		})
		requireNoError(t, err, "meta update environment")
		t.Logf("Updated environment %s via meta-tool", out.Name)
	})

	t.Run("Stop", func(t *testing.T) {
		requireTrue(t, envID > 0, "envID not set")
		out, err := callToolOn[environments.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "stop",
			"params": map[string]any{"project_id": proj.pidStr(), "environment_id": envID},
		})
		requireNoError(t, err, "meta stop environment")
		t.Logf("Stopped environment %s via meta-tool", out.Name)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, envID > 0, "envID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "delete",
			"params": map[string]any{"project_id": proj.pidStr(), "environment_id": envID},
		})
		requireNoError(t, err, "meta delete environment")
		t.Log("Deleted environment via meta-tool")
	})
}
