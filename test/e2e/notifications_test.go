//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/notifications"
)

// TestMeta_Notifications exercises notification settings via the gitlab_user meta-tool.
func TestMeta_Notifications(t *testing.T) {
	ctx := context.Background()

	t.Run("Meta/Notification/GlobalGet", func(t *testing.T) {
		out, err := callToolOn[notifications.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "notification_global_get",
			"params": map[string]any{},
		})
		requireNoError(t, err, "global notification get")
		t.Logf("Global notification level: %s", out.Level)
	})

	t.Run("Meta/Notification/ProjectGet", func(t *testing.T) {
		proj := createProjectMeta(ctx, t, sess.meta)
		out, err := callToolOn[notifications.Output](ctx, sess.meta, "gitlab_user", map[string]any{
			"action": "notification_project_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "project notification get")
		t.Logf("Project notification level: %s", out.Level)
	})
}
