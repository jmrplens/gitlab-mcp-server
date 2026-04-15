//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/freezeperiods"
)

// TestMeta_FreezePeriods exercises freeze period CRUD via the gitlab_environment meta-tool.
func TestMeta_FreezePeriods(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	var freezePeriodID int64

	t.Run("Meta/FreezePeriod/Create", func(t *testing.T) {
		out, err := callToolOn[freezeperiods.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "freeze_create",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"freeze_start":  "0 23 * * 5",
				"freeze_end":    "0 7 * * 1",
				"cron_timezone": "UTC",
			},
		})
		requireNoError(t, err, "freeze period create")
		requireTrue(t, out.ID > 0, "expected positive freeze period ID")
		freezePeriodID = out.ID
		t.Logf("Created freeze period %d", out.ID)
	})

	t.Run("Meta/FreezePeriod/List", func(t *testing.T) {
		requireTrue(t, freezePeriodID > 0, "freezePeriodID not set")
		out, err := callToolOn[freezeperiods.ListOutput](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "freeze_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "freeze period list")
		requireTrue(t, len(out.FreezePeriods) >= 1, "expected at least 1 freeze period")
		t.Logf("Listed %d freeze period(s)", len(out.FreezePeriods))
	})

	t.Run("Meta/FreezePeriod/Get", func(t *testing.T) {
		requireTrue(t, freezePeriodID > 0, "freezePeriodID not set")
		out, err := callToolOn[freezeperiods.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "freeze_get",
			"params": map[string]any{
				"project_id":       proj.pidStr(),
				"freeze_period_id": fmt.Sprintf("%d", freezePeriodID),
			},
		})
		requireNoError(t, err, "freeze period get")
		requireTrue(t, out.ID == freezePeriodID, "freeze period ID mismatch")
		t.Logf("Got freeze period %d", out.ID)
	})

	t.Run("Meta/FreezePeriod/Update", func(t *testing.T) {
		requireTrue(t, freezePeriodID > 0, "freezePeriodID not set")
		out, err := callToolOn[freezeperiods.Output](ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "freeze_update",
			"params": map[string]any{
				"project_id":       proj.pidStr(),
				"freeze_period_id": fmt.Sprintf("%d", freezePeriodID),
				"cron_timezone":    "America/New_York",
			},
		})
		requireNoError(t, err, "freeze period update")
		requireTrue(t, out.ID == freezePeriodID, "freeze period ID mismatch after update")
		t.Logf("Updated freeze period %d", out.ID)
	})

	t.Run("Meta/FreezePeriod/Delete", func(t *testing.T) {
		requireTrue(t, freezePeriodID > 0, "freezePeriodID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_environment", map[string]any{
			"action": "freeze_delete",
			"params": map[string]any{
				"project_id":       proj.pidStr(),
				"freeze_period_id": fmt.Sprintf("%d", freezePeriodID),
			},
		})
		requireNoError(t, err, "freeze period delete")
		t.Logf("Deleted freeze period %d", freezePeriodID)
	})
}
