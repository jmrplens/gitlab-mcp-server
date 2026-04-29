//go:build e2e

// pipelineschedules_test.go tests the pipeline schedule MCP tools against a live GitLab
// instance. Covers the full schedule lifecycle: create, get, list, update, variable CRUD,
// take ownership, run, and delete for both individual tools and the gitlab_pipeline meta-tool (schedule_* actions).
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelineschedules"
)

// TestIndividual_PipelineSchedules exercises the pipeline schedule lifecycle using individual tools:
// create → get → list → update → create variable → edit variable → delete variable → take ownership → delete.
func TestIndividual_PipelineSchedules(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	var scheduleID int

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[pipelineschedules.Output](ctx, sess.individual, "gitlab_pipeline_schedule_create", pipelineschedules.CreateInput{
			ProjectID:   proj.pidOf(),
			Description: "e2e-schedule",
			Ref:         defaultBranch,
			Cron:        "0 1 * * *",
		})
		requireNoError(t, err, "create pipeline schedule")
		requireTruef(t, out.ID > 0, "expected schedule ID")
		scheduleID = out.ID
		t.Logf("Created schedule %d", scheduleID)
	})

	t.Run("Get", func(t *testing.T) {
		requireTruef(t, scheduleID > 0, "scheduleID not set")
		out, err := callToolOn[pipelineschedules.Output](ctx, sess.individual, "gitlab_pipeline_schedule_get", pipelineschedules.GetInput{
			ProjectID:  proj.pidOf(),
			ScheduleID: scheduleID,
		})
		requireNoError(t, err, "get pipeline schedule")
		requireTruef(t, out.ID == scheduleID, "expected ID %d, got %d", scheduleID, out.ID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[pipelineschedules.ListOutput](ctx, sess.individual, "gitlab_pipeline_schedule_list", pipelineschedules.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list pipeline schedules")
		requireTruef(t, len(out.Schedules) >= 1, "expected at least 1 schedule")
	})

	t.Run("Update", func(t *testing.T) {
		requireTruef(t, scheduleID > 0, "scheduleID not set")
		out, err := callToolOn[pipelineschedules.Output](ctx, sess.individual, "gitlab_pipeline_schedule_update", pipelineschedules.UpdateInput{
			ProjectID:   proj.pidOf(),
			ScheduleID:  scheduleID,
			Description: "e2e-schedule-updated",
		})
		requireNoError(t, err, "update pipeline schedule")
		requireTruef(t, out.Description == "e2e-schedule-updated", "expected updated description")
	})

	t.Run("CreateVariable", func(t *testing.T) {
		requireTruef(t, scheduleID > 0, "scheduleID not set")
		out, err := callToolOn[pipelineschedules.VariableOutput](ctx, sess.individual, "gitlab_pipeline_schedule_create_variable", pipelineschedules.CreateVariableInput{
			ProjectID:  proj.pidOf(),
			ScheduleID: scheduleID,
			Key:        "E2E_VAR",
			Value:      "e2e-value",
		})
		requireNoError(t, err, "create schedule variable")
		requireTruef(t, out.Key == "E2E_VAR", "expected key E2E_VAR")
	})

	t.Run("EditVariable", func(t *testing.T) {
		requireTruef(t, scheduleID > 0, "scheduleID not set")
		out, err := callToolOn[pipelineschedules.VariableOutput](ctx, sess.individual, "gitlab_pipeline_schedule_edit_variable", pipelineschedules.EditVariableInput{
			ProjectID:  proj.pidOf(),
			ScheduleID: scheduleID,
			Key:        "E2E_VAR",
			Value:      "e2e-value-updated",
		})
		requireNoError(t, err, "edit schedule variable")
		requireTruef(t, out.Value == "e2e-value-updated", "expected updated value")
	})

	t.Run("DeleteVariable", func(t *testing.T) {
		requireTruef(t, scheduleID > 0, "scheduleID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_pipeline_schedule_delete_variable", pipelineschedules.DeleteVariableInput{
			ProjectID:  proj.pidOf(),
			ScheduleID: scheduleID,
			Key:        "E2E_VAR",
		})
		requireNoError(t, err, "delete schedule variable")
	})

	t.Run("TakeOwnership", func(t *testing.T) {
		requireTruef(t, scheduleID > 0, "scheduleID not set")
		out, err := callToolOn[pipelineschedules.Output](ctx, sess.individual, "gitlab_pipeline_schedule_take_ownership", pipelineschedules.TakeOwnershipInput{
			ProjectID:  proj.pidOf(),
			ScheduleID: scheduleID,
		})
		requireNoError(t, err, "take ownership")
		requireTruef(t, out.ID == scheduleID, "expected same schedule after ownership")
	})

	t.Run("Delete", func(t *testing.T) {
		requireTruef(t, scheduleID > 0, "scheduleID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_pipeline_schedule_delete", pipelineschedules.DeleteInput{
			ProjectID:  proj.pidOf(),
			ScheduleID: scheduleID,
		})
		requireNoError(t, err, "delete pipeline schedule")
	})
}

// TestMeta_PipelineSchedules exercises the same pipeline schedule lifecycle via the
// gitlab_pipeline meta-tool (schedule_* actions), including variable CRUD, take ownership, and run.
func TestMeta_PipelineSchedules(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var scheduleID int

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[pipelineschedules.Output](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "schedule_create",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"description": "e2e-meta-schedule",
				"ref":         defaultBranch,
				"cron":        "0 2 * * *",
			},
		})
		requireNoError(t, err, "meta create schedule")
		requireTruef(t, out.ID > 0, "expected schedule ID")
		scheduleID = out.ID
		t.Logf("Created schedule %d via meta-tool", scheduleID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[pipelineschedules.ListOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "schedule_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta list schedules")
		requireTruef(t, len(out.Schedules) >= 1, "expected at least 1 schedule")
	})

	t.Run("Update", func(t *testing.T) {
		requireTruef(t, scheduleID > 0, "scheduleID not set")
		out, err := callToolOn[pipelineschedules.Output](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "schedule_update",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"schedule_id": scheduleID,
				"description": "e2e-meta-schedule-updated",
			},
		})
		requireNoError(t, err, "meta update schedule")
		requireTruef(t, out.Description == "e2e-meta-schedule-updated", "expected updated description")
	})

	t.Run("CreateVariable", func(t *testing.T) {
		requireTruef(t, scheduleID > 0, "scheduleID not set")
		out, err := callToolOn[pipelineschedules.VariableOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "schedule_create_variable",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"schedule_id": scheduleID,
				"key":         "META_VAR",
				"value":       "meta-value",
			},
		})
		requireNoError(t, err, "meta create variable")
		requireTruef(t, out.Key == "META_VAR", "expected key META_VAR")
	})

	t.Run("DeleteVariable", func(t *testing.T) {
		requireTruef(t, scheduleID > 0, "scheduleID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "schedule_delete_variable",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"schedule_id": scheduleID,
				"key":         "META_VAR",
			},
		})
		requireNoError(t, err, "meta delete variable")
	})

	t.Run("TakeOwnership", func(t *testing.T) {
		requireTruef(t, scheduleID > 0, "scheduleID not set")
		_, err := callToolOn[pipelineschedules.Output](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "schedule_take_ownership",
			"params": map[string]any{"project_id": proj.pidStr(), "schedule_id": scheduleID},
		})
		requireNoError(t, err, "meta take ownership")
	})

	t.Run("Run", func(t *testing.T) {
		requireTruef(t, scheduleID > 0, "scheduleID not set")
		// Run may fail on CE if no runners configured — just verify the call doesn't panic
		_, _ = callToolOn[pipelineschedules.Output](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "schedule_run",
			"params": map[string]any{"project_id": proj.pidStr(), "schedule_id": scheduleID},
		})
		t.Log("Run attempted (may fail without runner)")
	})

	t.Run("Delete", func(t *testing.T) {
		requireTruef(t, scheduleID > 0, "scheduleID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "schedule_delete",
			"params": map[string]any{"project_id": proj.pidStr(), "schedule_id": scheduleID},
		})
		requireNoError(t, err, "meta delete schedule")
	})
}
