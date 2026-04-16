//go:build e2e

package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelineschedules"
)

// TestMeta_PipelineSchedulesExtended exercises actions not covered by pipelineschedules_test.go:
// get, edit_variable, list_triggered_pipelines.
func TestMeta_PipelineSchedulesExtended(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, proj, "main", "sched.txt", "content", "init for schedules")

	// Create a schedule for testing
	schedOut, schedErr := callToolOn[pipelineschedules.Output](ctx, sess.meta, "gitlab_pipeline_schedule", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id":  proj.pidStr(),
			"description": "e2e-extended-schedule",
			"ref":         "main",
			"cron":        "0 1 * * *",
		},
	})
	requireNoError(t, schedErr, "create schedule")
	schedID := strconv.Itoa(schedOut.ID)
	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_pipeline_schedule", map[string]any{
			"action": "delete",
			"params": map[string]any{"project_id": proj.pidStr(), "schedule_id": schedID},
		})
	}()

	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[pipelineschedules.Output](ctx, sess.meta, "gitlab_pipeline_schedule", map[string]any{
			"action": "get",
			"params": map[string]any{"project_id": proj.pidStr(), "schedule_id": schedID},
		})
		requireNoError(t, err, "get schedule")
		requireTrue(t, out.ID == schedOut.ID, "get: schedule ID mismatch")
		t.Logf("Got schedule %d: %s", out.ID, out.Description)
	})

	// Create a variable, then edit it
	varKey := "SCHED_VAR"
	_, schedErr = callToolOn[pipelineschedules.VariableOutput](ctx, sess.meta, "gitlab_pipeline_schedule", map[string]any{
		"action": "create_variable",
		"params": map[string]any{
			"project_id":  proj.pidStr(),
			"schedule_id": schedID,
			"key":         varKey,
			"value":       "original",
		},
	})
	requireNoError(t, schedErr, "create_variable for edit test")

	t.Run("EditVariable", func(t *testing.T) {
		out, err := callToolOn[pipelineschedules.VariableOutput](ctx, sess.meta, "gitlab_pipeline_schedule", map[string]any{
			"action": "edit_variable",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"schedule_id": schedID,
				"key":         varKey,
				"value":       "edited",
			},
		})
		requireNoError(t, err, "edit_variable")
		requireTrue(t, out.Key == varKey, "edit_variable: key mismatch")
		t.Logf("Edited variable %s", out.Key)
	})

	t.Run("ListTriggeredPipelines", func(t *testing.T) {
		out, err := callToolOn[pipelineschedules.TriggeredPipelinesListOutput](ctx, sess.meta, "gitlab_pipeline_schedule", map[string]any{
			"action": "list_triggered_pipelines",
			"params": map[string]any{"project_id": proj.pidStr(), "schedule_id": schedID},
		})
		requireNoError(t, err, "list_triggered_pipelines")
		t.Logf("Triggered pipelines: %d", len(out.Pipelines))
	})
}
