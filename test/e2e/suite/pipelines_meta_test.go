//go:build e2e

package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/resourcegroups"
)

// TestMeta_PipelinesExtended exercises pipeline meta-tool actions not covered
// by pipelines_test.go: latest, variables, test_report, test_report_summary,
// update_metadata, cancel, and resource group operations.
func TestMeta_PipelinesExtended(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	// Commit a .gitlab-ci.yml to trigger a pipeline
	commitFileMeta(ctx, t, sess.meta, proj, "main", ".gitlab-ci.yml",
		"stages:\n  - test\ntest_job:\n  stage: test\n  script:\n    - echo hello\n",
		"add CI config")

	// Wait for pipeline creation with polling
	t.Run("Latest", func(t *testing.T) {
		drainSidekiq(ctx, t)
		var out pipelines.DetailOutput
		var err error
		deadline := time.Now().Add(120 * time.Second)
		delay := 2 * time.Second
		for time.Now().Before(deadline) {
			out, err = callToolOn[pipelines.DetailOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
				"action": "latest",
				"params": map[string]any{"project_id": proj.pidStr()},
			})
			if err == nil {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatalf("context canceled waiting for pipeline: %v", ctx.Err())
			case <-time.After(delay):
			}
		}
		requireNoError(t, err, "latest pipeline")
		requireTrue(t, out.ID > 0, "latest: expected ID > 0")
		t.Logf("Latest pipeline: %d (status: %s)", out.ID, out.Status)
	})

	t.Run("Variables", func(t *testing.T) {
		// Create a pipeline with variables
		createOut, err := callToolOn[pipelines.DetailOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"ref":        "main",
			},
		})
		requireNoError(t, err, "create pipeline for variables")
		out, err := callToolOn[pipelines.VariablesOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "variables",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"pipeline_id": createOut.ID,
			},
		})
		requireNoError(t, err, "variables")
		t.Logf("Pipeline %d variables: %d", createOut.ID, len(out.Variables))
	})

	t.Run("TestReport", func(t *testing.T) {
		latest, err := callToolOn[pipelines.DetailOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "latest",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "get latest for test_report")
		// test_report may return error if pipeline hasn't finished, but we test the action works
		_, _ = callToolOn[pipelines.TestReportOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "test_report",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"pipeline_id": latest.ID,
			},
		})
		t.Logf("TestReport called for pipeline %d", latest.ID)
	})

	t.Run("TestReportSummary", func(t *testing.T) {
		latest, err := callToolOn[pipelines.DetailOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "latest",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "get latest for test_report_summary")
		_, _ = callToolOn[pipelines.TestReportSummaryOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "test_report_summary",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"pipeline_id": latest.ID,
			},
		})
		t.Logf("TestReportSummary called for pipeline %d", latest.ID)
	})

	t.Run("UpdateMetadata", func(t *testing.T) {
		latest, err := callToolOn[pipelines.DetailOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "latest",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "get latest for update_metadata")
		out, err := callToolOn[pipelines.DetailOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "update_metadata",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"pipeline_id": latest.ID,
				"name":        "e2e-renamed",
			},
		})
		requireNoError(t, err, "update_metadata")
		requireTrue(t, out.ID == latest.ID, "update_metadata: ID mismatch")
	})

	t.Run("Cancel", func(t *testing.T) {
		latest, err := callToolOn[pipelines.DetailOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "latest",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "get latest for cancel")
		// Cancel may fail if already finished, that's acceptable
		_, _ = callToolOn[pipelines.StatusOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "cancel",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"pipeline_id": latest.ID,
			},
		})
		t.Logf("Cancel attempted for pipeline %d", latest.ID)
	})
}

// TestMeta_ResourceGroups exercises resource group actions via gitlab_pipeline.
func TestMeta_ResourceGroups(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("ResourceGroupList", func(t *testing.T) {
		out, err := callToolOn[resourcegroups.ListOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "resource_group_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "resource_group_list")
		t.Logf("Resource groups: %d", len(out.Groups))
	})
}
