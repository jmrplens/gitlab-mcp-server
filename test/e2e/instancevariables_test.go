//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/instancevariables"
)

// TestMeta_CIVariablesInstance exercises instance-level CI variable CRUD via the gitlab_ci_variable meta-tool.
func TestMeta_CIVariablesInstance(t *testing.T) {
	ctx := context.Background()
	varKey := "E2E_INSTANCE_VAR"

	// Cleanup in case a previous run left the variable behind.
	t.Cleanup(func() {
		cleanCtx := context.Background()
		_ = callToolVoidOn(cleanCtx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "instance_delete",
			"params": map[string]any{"key": varKey},
		})
	})

	t.Run("Meta/CIVariableInstance/Create", func(t *testing.T) {
		out, err := callToolOn[instancevariables.Output](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "instance_create",
			"params": map[string]any{
				"key":   varKey,
				"value": "instance_test_value",
			},
		})
		requireNoError(t, err, "instance variable create")
		requireTrue(t, out.Key == varKey, "expected key %s, got %q", varKey, out.Key)
		t.Logf("Created instance variable: %s", out.Key)
	})

	t.Run("Meta/CIVariableInstance/List", func(t *testing.T) {
		out, err := callToolOn[instancevariables.ListOutput](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "instance_list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "instance variable list")
		requireTrue(t, len(out.Variables) >= 1, "expected at least 1 instance variable")
		t.Logf("Instance variables: %d", len(out.Variables))
	})

	t.Run("Meta/CIVariableInstance/Get", func(t *testing.T) {
		out, err := callToolOn[instancevariables.Output](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "instance_get",
			"params": map[string]any{"key": varKey},
		})
		requireNoError(t, err, "instance variable get")
		requireTrue(t, out.Key == varKey, "expected key %s, got %q", varKey, out.Key)
		t.Logf("Got instance variable: %s=%s", out.Key, out.Value)
	})

	t.Run("Meta/CIVariableInstance/Update", func(t *testing.T) {
		out, err := callToolOn[instancevariables.Output](ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "instance_update",
			"params": map[string]any{
				"key":   varKey,
				"value": "instance_updated_value",
			},
		})
		requireNoError(t, err, "instance variable update")
		requireTrue(t, out.Key == varKey, "expected key %s, got %q", varKey, out.Key)
		t.Logf("Updated instance variable: %s", out.Key)
	})

	t.Run("Meta/CIVariableInstance/Delete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_ci_variable", map[string]any{
			"action": "instance_delete",
			"params": map[string]any{"key": varKey},
		})
		requireNoError(t, err, "instance variable delete")
		t.Logf("Deleted instance variable %s", varKey)
	})
}
