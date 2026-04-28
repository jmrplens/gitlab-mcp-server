//go:build e2e

// protectedenvs_test.go tests the protected environment MCP tools against a live GitLab
// instance. Covers protect, list, get, and unprotect via the gitlab_environment meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/protectedenvs"
)

// TestMeta_ProtectedEnvs exercises protected environment CRUD via the
// gitlab_environment meta-tool.
// Requires GitLab Premium/Ultimate (GITLAB_ENTERPRISE=true).
func TestMeta_ProtectedEnvs(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}

	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	const envName = "e2e-staging"

	t.Run("Meta/ProtectedEnv/Protect", func(t *testing.T) {
		out, err := callToolOn[protectedenvs.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "protected_protect",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       envName,
			},
		})
		requireNoError(t, err, "meta protected env protect")
		requireTruef(t, out.Name == envName, "expected protected env name %q, got %q", envName, out.Name)
		t.Logf("Protected environment: %s", out.Name)
	})

	t.Run("Meta/ProtectedEnv/List", func(t *testing.T) {
		out, err := callToolOn[protectedenvs.ListOutput](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "protected_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "meta protected env list")
		requireTruef(t, len(out.Environments) >= 1, "expected at least 1 protected environment")
		t.Logf("Listed %d protected environment(s)", len(out.Environments))
	})

	t.Run("Meta/ProtectedEnv/Get", func(t *testing.T) {
		out, err := callToolOn[protectedenvs.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "protected_get",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"environment": envName,
			},
		})
		requireNoError(t, err, "meta protected env get")
		requireTruef(t, out.Name == envName, "protected env name mismatch")
		t.Logf("Got protected environment: %s", out.Name)
	})

	t.Run("Meta/ProtectedEnv/Unprotect", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "protected_unprotect",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"environment": envName,
			},
		})
		requireNoError(t, err, "meta protected env unprotect")
		t.Logf("Unprotected environment: %s", envName)
	})
}
