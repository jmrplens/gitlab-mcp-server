//go:build e2e

// pipelinetriggers_test.go tests the pipeline trigger MCP tools against a live GitLab
// instance. Covers trigger create, list, get, update, and delete via the gitlab_pipeline
// meta-tool.
package suite

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelinetriggers"
)

// TestMeta_PipelineTriggers exercises pipeline trigger CRUD via the gitlab_pipeline meta-tool.
func TestMeta_PipelineTriggers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	proj := createProjectMeta(ctx, t, sess.meta)

	var triggerID int64

	t.Run("Meta/PipelineTrigger/Create", func(t *testing.T) {
		out, err := callToolOn[pipelinetriggers.Output](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "trigger_create",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"description": "e2e-trigger",
			},
		})
		requireNoError(t, err, "pipeline trigger create")
		requireTrue(t, out.ID > 0, "expected positive trigger ID")
		triggerID = out.ID
		t.Logf("Created trigger %d", out.ID)
	})

	t.Run("Meta/PipelineTrigger/List", func(t *testing.T) {
		requireTrue(t, triggerID > 0, "triggerID not set")
		out, err := callToolOn[pipelinetriggers.ListOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "trigger_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "pipeline trigger list")
		requireTrue(t, len(out.Triggers) >= 1, "expected at least 1 trigger")
		t.Logf("Listed %d trigger(s)", len(out.Triggers))
	})

	t.Run("Meta/PipelineTrigger/Get", func(t *testing.T) {
		requireTrue(t, triggerID > 0, "triggerID not set")
		out, err := callToolOn[pipelinetriggers.Output](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "trigger_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"trigger_id": fmt.Sprintf("%d", triggerID),
			},
		})
		requireNoError(t, err, "pipeline trigger get")
		requireTrue(t, out.ID == triggerID, "trigger ID mismatch")
		t.Logf("Got trigger %d", out.ID)
	})

	t.Run("Meta/PipelineTrigger/Update", func(t *testing.T) {
		requireTrue(t, triggerID > 0, "triggerID not set")
		out, err := callToolOn[pipelinetriggers.Output](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "trigger_update",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"trigger_id":  fmt.Sprintf("%d", triggerID),
				"description": "e2e-trigger-updated",
			},
		})
		requireNoError(t, err, "pipeline trigger update")
		requireTrue(t, out.ID == triggerID, "trigger ID mismatch after update")
		t.Logf("Updated trigger %d", out.ID)
	})

	t.Run("Meta/PipelineTrigger/Delete", func(t *testing.T) {
		requireTrue(t, triggerID > 0, "triggerID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "trigger_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"trigger_id": fmt.Sprintf("%d", triggerID),
			},
		})
		requireNoError(t, err, "pipeline trigger delete")
		t.Logf("Deleted trigger %d", triggerID)
	})
}
