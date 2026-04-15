//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobtokenscope"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/settings"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/topics"
)

// TestMeta_Admin exercises admin-level meta-tool actions (topics, settings).
func TestMeta_Admin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("Meta/Admin/TopicList", func(t *testing.T) {
		out, err := callToolOn[topics.ListOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "topic_list",
			"params": map[string]any{},
		})
		requireNoError(t, err, "meta admin topic list")
		t.Logf("Listed %d topics", len(out.Topics))
	})

	t.Run("Meta/Admin/SettingsGet", func(t *testing.T) {
		out, err := callToolOn[settings.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
			"action": "settings_get",
			"params": map[string]any{},
		})
		requireNoError(t, err, "meta admin settings get")
		requireTrue(t, len(out.Settings) > 0, "expected non-empty settings map, got %d keys", len(out.Settings))
		t.Logf("Admin settings: %d keys", len(out.Settings))
	})
}

// TestMeta_JobTokens exercises job listing and token scope via the gitlab_job meta-tool.
func TestMeta_JobTokens(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("Meta/Job/ListProject", func(t *testing.T) {
		_, err := callToolOn[jobs.ListOutput](ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "list_project",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "meta job list_project")
		t.Log("Job list_project OK (may be empty without CI pipeline)")
	})

	t.Run("Meta/Job/TokenScopeGet", func(t *testing.T) {
		out, err := callToolOn[jobtokenscope.AccessSettingsOutput](ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "token_scope_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "meta job token_scope_get")
		t.Logf("Job token scope: inbound_enabled=%v", out.InboundEnabled)
	})
}
