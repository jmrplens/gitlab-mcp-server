//go:build e2e

// notifications_test.go covers the run user's notification settings: the
// global level read and changed, and the per-project and per-group levels
// read on fresh objects, where GitLab reports them as following the global
// one, and then set. The settings belong to the account every session runs
// on, so the test holds the current-user lock and restores the global level
// it found. Even the reads hold it: GitLab materializes the setting row on
// first access.

package common

import (
	"context"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/notifications"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The notification levels the scenario reads and writes, as GitLab spells
// them.
const (
	// notificationLevelGlobal is what a project or group without a setting of
	// its own reports: it follows the account's global level.
	notificationLevelGlobal = "global"
	// notificationLevelWatch is the level the project and group are set to.
	notificationLevelWatch = "watch"
	// notificationLevelParticipating is the level the global setting is
	// changed to, unless it already is, in which case watch is.
	notificationLevelParticipating = "participating"
)

// TestNotifications_GlobalProjectAndGroup_ReadAndUpdate reads and changes
// the run user's global level on every surface, then reads the level of a
// project and a group of the surface's own, which follow the global one,
// and sets each to watch.
//
// Replaces: TestMeta_UserNamespacesNotifications, TestMeta_Notifications
func TestNotifications_GlobalProjectAndGroup_ReadAndUpdate(t *testing.T) {
	e := harness.New(t, harness.Locks(harness.LockCurrentUserState))
	restoreGlobalNotificationLevel(e)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		global := harness.Do[notifications.Output](s, actionUserNotificationGlobalGet, nil)
		if global.Level == "" {
			e.T.Fatalf("notification_global_get answered %+v, want a level", global)
		}
		target := notificationLevelParticipating
		if global.Level == target {
			target = notificationLevelWatch
		}
		updated := harness.Do[notifications.Output](s, actionUserNotificationGlobalUpdate, map[string]any{"level": target})
		if updated.Level != target {
			e.T.Errorf("notification_global_update answered level %q, want %q", updated.Level, target)
		}
		again := harness.Do[notifications.Output](s, actionUserNotificationGlobalGet, nil)
		if again.Level != target {
			e.T.Errorf("notification_global_get answered level %q right after the update to %q", again.Level, target)
		}

		project := fixture.NewProject(e, fixture.WithNamePrefix("notif"))
		fresh := harness.Do[notifications.Output](s, actionUserNotificationProjectGet, map[string]any{"project_id": project.IDParam()})
		if fresh.Level != notificationLevelGlobal {
			e.T.Errorf("a fresh project's notification level is %q, want %q", fresh.Level, notificationLevelGlobal)
		}
		watched := harness.Do[notifications.Output](s, actionUserNotificationProjectUpdate, map[string]any{"project_id": project.IDParam(), "level": notificationLevelWatch})
		if watched.Level != notificationLevelWatch {
			e.T.Errorf("notification_project_update answered level %q, want %q", watched.Level, notificationLevelWatch)
		}

		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("notif"))
		freshGroup := harness.Do[notifications.Output](s, actionUserNotificationGroupGet, map[string]any{"group_id": group.IDParam()})
		if freshGroup.Level != notificationLevelGlobal {
			e.T.Errorf("a fresh group's notification level is %q, want %q", freshGroup.Level, notificationLevelGlobal)
		}
		watchedGroup := harness.Do[notifications.Output](s, actionUserNotificationGroupUpdate, map[string]any{"group_id": group.IDParam(), "level": notificationLevelWatch})
		if watchedGroup.Level != notificationLevelWatch {
			e.T.Errorf("notification_group_update answered level %q, want %q", watchedGroup.Level, notificationLevelWatch)
		}
	})
}

// restoreGlobalNotificationLevel reads the run user's global level through
// client-go and registers putting it back, so the account is left as found.
func restoreGlobalNotificationLevel(e *harness.Env) {
	e.T.Helper()

	before, _, err := e.Client().GL().NotificationSettings.GetGlobalSettings(gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("reading the run user's global notification level before changing it: %v", err)
	}
	e.Defer("the run user's global notification level", func(ctx context.Context) error {
		_, _, restoreErr := e.Client().GL().NotificationSettings.UpdateGlobalSettings(&gl.NotificationSettingsOptions{Level: &before.Level}, gl.WithContext(ctx))
		return restoreErr
	})
}
