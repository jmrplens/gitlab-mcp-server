//go:build e2e && !enterprise

// notifications_ce_test.go tests the notification settings MCP tools against a live GitLab
// instance. Covers global and per-project notification level retrieval via the
// gitlab_user meta-tool.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/notifications"
)

// TestMeta_Notifications exercises notification settings via the gitlab_user meta-tool.
//
// Reading notification settings is not side-effect free: GitLab materializes the
// row on first access (see materializeGlobalNotificationSetting), so this test
// takes [CapabilityCurrentUserState] to serialize against the tests that write
// those settings, even though it only reads.
func TestMeta_Notifications(t *testing.T) {
	t.Parallel()
	RunWithCapabilities(t, []Capability{CapabilityCurrentUserState}, func(_ *E2EContext) {
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
	})
}
