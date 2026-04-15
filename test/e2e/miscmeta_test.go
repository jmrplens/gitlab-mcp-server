//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/branchrules"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/cicatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/deployments"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/featureflags"
)

// TestMeta_FeatureFlags exercises feature flag listing via the gitlab_feature_flags meta-tool.
// Feature flags may require a Premium/Ultimate license; errors are fatal.
func TestMeta_FeatureFlags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/FeatureFlag/List", func(t *testing.T) {
		_, err := callToolOn[featureflags.ListOutput](ctx, sess.meta, "gitlab_feature_flags", map[string]any{
			"action": "feature_flag_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		if err != nil {
			t.Fatalf("feature flags failed: %v", err)
		}
		t.Log("Feature flag list OK")
	})
}

// TestMeta_BranchRules exercises branch rule listing via the gitlab_branch_rule meta-tool.
func TestMeta_BranchRules(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/BranchRule/List", func(t *testing.T) {
		out, err := callToolOn[branchrules.ListOutput](ctx, sess.meta, "gitlab_branch_rule", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_path": proj.Path,
			},
		})
		requireNoError(t, err, "branch rule list")
		t.Logf("Project %s has %d branch rule(s)", proj.Path, len(out.Rules))
	})
}

// TestMeta_CICatalog exercises CI/CD catalog listing via the gitlab_ci_catalog meta-tool.
func TestMeta_CICatalog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("Meta/CICatalog/List", func(t *testing.T) {
		out, err := callToolOn[cicatalog.ListOutput](ctx, sess.meta, "gitlab_ci_catalog", map[string]any{
			"action": "list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "ci catalog list")
		t.Logf("Found %d CI/CD catalog resource(s)", len(out.Resources))
	})
}

// TestMeta_Deployments exercises deployment listing via the gitlab_deployment meta-tool.
func TestMeta_Deployments(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/Deployment/List", func(t *testing.T) {
		_, err := callToolOn[deployments.ListOutput](ctx, sess.meta, "gitlab_deployment", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "deployment list")
		t.Log("Deployment list OK (may be empty without CI pipeline)")
	})
}

// TestMeta_UserKeys exercises SSH and GPG key listing via the gitlab_user meta-tool.
func TestMeta_UserKeys(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("Meta/User/SSHKeys", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "ssh_keys",
			"params": map[string]any{},
		})
		requireNoError(t, err, "user ssh_keys")
		t.Log("SSH keys OK")
	})

	t.Run("Meta/User/GPGKeys", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "gpg_keys",
			"params": map[string]any{},
		})
		requireNoError(t, err, "user gpg_keys")
		t.Log("GPG keys OK")
	})
}
