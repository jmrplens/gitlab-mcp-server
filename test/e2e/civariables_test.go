//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/civariables"
)

func TestIndividual_CIVariables(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	const varKey = "E2E_TEST_VAR"
	const varValue = "hello-e2e"

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[civariables.Output](ctx, sess.individual, "gitlab_ci_variable_create", civariables.CreateInput{
			ProjectID: proj.pidOf(),
			Key:       varKey,
			Value:     varValue,
		})
		requireNoError(t, err, "create CI variable")
		requireTrue(t, out.Key == varKey, "expected key %s, got %s", varKey, out.Key)
		requireTrue(t, out.Value == varValue, "expected value %s, got %s", varValue, out.Value)
		t.Logf("Created CI variable %s=%s", out.Key, out.Value)
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[civariables.Output](ctx, sess.individual, "gitlab_ci_variable_get", civariables.GetInput{
			ProjectID: proj.pidOf(),
			Key:       varKey,
		})
		requireNoError(t, err, "get CI variable")
		requireTrue(t, out.Key == varKey, "expected key %s, got %s", varKey, out.Key)
		t.Logf("Got CI variable %s", out.Key)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[civariables.ListOutput](ctx, sess.individual, "gitlab_ci_variable_list", civariables.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list CI variables")
		requireTrue(t, len(out.Variables) >= 1, "expected at least 1 variable, got %d", len(out.Variables))
		t.Logf("Listed %d CI variables", len(out.Variables))
	})

	t.Run("Update", func(t *testing.T) {
		out, err := callToolOn[civariables.Output](ctx, sess.individual, "gitlab_ci_variable_update", civariables.UpdateInput{
			ProjectID: proj.pidOf(),
			Key:       varKey,
			Value:     "updated-value",
		})
		requireNoError(t, err, "update CI variable")
		requireTrue(t, out.Value == "updated-value", "expected updated value, got %s", out.Value)
		t.Logf("Updated CI variable %s=%s", out.Key, out.Value)
	})

	t.Run("Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_ci_variable_delete", civariables.DeleteInput{
			ProjectID: proj.pidOf(),
			Key:       varKey,
		})
		requireNoError(t, err, "delete CI variable")
		t.Log("Deleted CI variable")
	})
}

func TestMeta_CIVariables(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	const varKey = "E2E_META_VAR"
	const varValue = "meta-hello"

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[civariables.Output](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "create",
			"params": map[string]any{"project_id": proj.pidStr(), "key": varKey, "value": varValue},
		})
		requireNoError(t, err, "meta create CI variable")
		requireTrue(t, out.Key == varKey, "expected key %s", varKey)
		t.Logf("Created CI variable %s via meta-tool", out.Key)
	})

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[civariables.Output](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "get",
			"params": map[string]any{"project_id": proj.pidStr(), "key": varKey},
		})
		requireNoError(t, err, "meta get CI variable")
		requireTrue(t, out.Key == varKey, "expected key %s", varKey)
		t.Logf("Got CI variable %s via meta-tool", out.Key)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[civariables.ListOutput](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta list CI variables")
		requireTrue(t, len(out.Variables) >= 1, "expected at least 1 variable")
		t.Logf("Listed %d CI variables via meta-tool", len(out.Variables))
	})

	t.Run("Update", func(t *testing.T) {
		out, err := callToolOn[civariables.Output](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "update",
			"params": map[string]any{"project_id": proj.pidStr(), "key": varKey, "value": "meta-updated"},
		})
		requireNoError(t, err, "meta update CI variable")
		requireTrue(t, out.Value == "meta-updated", "expected meta-updated, got %s", out.Value)
		t.Logf("Updated CI variable %s via meta-tool", out.Key)
	})

	t.Run("Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "delete",
			"params": map[string]any{"project_id": proj.pidStr(), "key": varKey},
		})
		requireNoError(t, err, "meta delete CI variable")
		t.Log("Deleted CI variable via meta-tool")
	})
}
