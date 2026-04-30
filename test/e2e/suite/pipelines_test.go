//go:build e2e

// pipelines_test.go tests the pipeline and job MCP tools against a live GitLab instance.
// Requires Docker mode with a CI runner. Covers pipeline create, get, list, retry, delete,
// and job list, get, trace for both individual tools and the gitlab_pipeline/gitlab_job meta-tools.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelines"
)

// pipelineCIYAML is a minimal .gitlab-ci.yml that runs a single fast job
// for pipeline E2E tests. Uses no runner tags to ensure execution on any runner.
const pipelineCIYAML = `stages:
  - test

fast-pass:
  stage: test
  script:
    - echo "E2E pipeline test job"
  tags: []
`

// TestPipelines exercises the pipeline lifecycle: create, get, list, wait for jobs,
// job get, job trace, retry, and delete. Requires a CI runner.
//
// NOT parallelized: pipeline-heavy tests share a single CI runner in Docker mode.
// Running them concurrently causes pipelines to queue, leading to spurious
// timeouts on slower hosts.
func TestPipelines(t *testing.T) {
	RunWithCapabilities(t, []Capability{CapabilityRunner}, func(t *testing.T, _ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Second)
		if deadline, ok := t.Deadline(); ok {
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), deadline)
		}
		defer cancel()

		// --- Individual tool session ---
		proj := createProject(ctx, t, sess.individual)
		commitFile(ctx, t, sess.individual, proj, "main", "init.txt", "bootstrap", "init commit")

		// Commit a .gitlab-ci.yml to enable pipelines.
		_, ciErr := callToolOn[commits.Output](ctx, sess.individual, "gitlab_commit_create", commits.CreateInput{
			ProjectID:     proj.pidOf(),
			Branch:        "main",
			CommitMessage: "ci: add .gitlab-ci.yml for pipeline tests",
			Actions: []commits.Action{{
				Action:   "create",
				FilePath: ".gitlab-ci.yml",
				Content:  pipelineCIYAML,
			}},
		})
		if ciErr != nil {
			t.Fatalf("commit CI config: %v", ciErr)
		}

		var pipelineID int64
		var jobID int64

		t.Run("Individual/Create", func(t *testing.T) {
			out, err := callToolOn[pipelines.DetailOutput](ctx, sess.individual, "gitlab_pipeline_create", pipelines.CreateInput{
				ProjectID: proj.pidOf(),
				Ref:       "main",
			})
			if err != nil {
				t.Fatalf("pipeline create: %v", err)
			}
			if out.ID <= 0 {
				t.Fatal("expected positive pipeline ID")
			}
			pipelineID = out.ID
			t.Logf("Created pipeline ID=%d status=%s", pipelineID, out.Status)
		})

		t.Run("Individual/Get", func(t *testing.T) {
			out, err := callToolOn[pipelines.DetailOutput](ctx, sess.individual, "gitlab_pipeline_get", pipelines.GetInput{
				ProjectID:  proj.pidOf(),
				PipelineID: pipelineID,
			})
			if err != nil {
				t.Fatalf("pipeline get: %v", err)
			}
			if out.ID != pipelineID {
				t.Fatalf("expected pipeline ID %d, got %d", pipelineID, out.ID)
			}
		})

		t.Run("Individual/List", func(t *testing.T) {
			out, err := callToolOn[pipelines.ListOutput](ctx, sess.individual, "gitlab_pipeline_list", pipelines.ListInput{
				ProjectID: proj.pidOf(),
			})
			if err != nil {
				t.Fatalf("pipeline list: %v", err)
			}
			if len(out.Pipelines) == 0 {
				t.Fatal("expected at least one pipeline")
			}
		})

		t.Run("Individual/WaitAndJobList", func(t *testing.T) {
			status := waitForPipeline(t, sess.glClient, proj.ID, pipelineID, 900*time.Second)
			t.Logf("Pipeline %d finished with status: %s", pipelineID, status)

			out, err := callToolOn[jobs.ListOutput](ctx, sess.individual, "gitlab_job_list", jobs.ListInput{
				ProjectID:  proj.pidOf(),
				PipelineID: pipelineID,
			})
			if err != nil {
				t.Fatalf("job list: %v", err)
			}
			if len(out.Jobs) == 0 {
				t.Fatal("expected at least 1 job")
			}
			jobID = out.Jobs[0].ID
			t.Logf("Listed %d jobs; first: ID=%d name=%s status=%s", len(out.Jobs), jobID, out.Jobs[0].Name, out.Jobs[0].Status)
		})

		t.Run("Individual/JobGet", func(t *testing.T) {
			out, err := callToolOn[jobs.Output](ctx, sess.individual, "gitlab_job_get", jobs.GetInput{
				ProjectID: proj.pidOf(),
				JobID:     jobID,
			})
			if err != nil {
				t.Fatalf("job get: %v", err)
			}
			if out.ID != jobID {
				t.Fatalf("expected job ID %d, got %d", jobID, out.ID)
			}
		})

		t.Run("Individual/JobTrace", func(t *testing.T) {
			out, err := callToolOn[jobs.TraceOutput](ctx, sess.individual, "gitlab_job_trace", jobs.TraceInput{
				ProjectID: proj.pidOf(),
				JobID:     jobID,
			})
			if err != nil {
				t.Fatalf("job trace: %v", err)
			}
			if len(out.Trace) == 0 {
				t.Fatal("expected non-empty job trace")
			}
			t.Logf("Job trace: %d chars, truncated=%v", len(out.Trace), out.Truncated)
		})

		t.Run("Individual/Retry", func(t *testing.T) {
			out, err := callToolOn[pipelines.DetailOutput](ctx, sess.individual, "gitlab_pipeline_retry", pipelines.ActionInput{
				ProjectID:  proj.pidOf(),
				PipelineID: pipelineID,
			})
			if err != nil {
				t.Fatalf("pipeline retry: %v", err)
			}
			t.Logf("Retried pipeline: ID=%d status=%s", out.ID, out.Status)
			waitForPipeline(t, sess.glClient, proj.ID, pipelineID, 900*time.Second)
		})

		t.Run("Individual/Delete", func(t *testing.T) {
			err := callToolVoidOn(ctx, sess.individual, "gitlab_pipeline_delete", pipelines.DeleteInput{
				ProjectID:  proj.pidOf(),
				PipelineID: pipelineID,
			})
			if err != nil {
				t.Fatalf("pipeline delete: %v", err)
			}
		})

		// --- Meta-tool session ---
		projM := createProjectMeta(ctx, t, sess.meta)
		commitFileMeta(ctx, t, sess.meta, projM, "main", "init.txt", "bootstrap", "init commit")

		// Commit CI config via meta.
		_, ciErr = callToolOn[commits.Output](ctx, sess.meta, "gitlab_repository", map[string]any{
			"action": "commit_create",
			"params": map[string]any{
				"project_id":     projM.pidStr(),
				"branch":         "main",
				"commit_message": "ci: add .gitlab-ci.yml",
				"actions": []map[string]any{{
					"action":    "create",
					"file_path": ".gitlab-ci.yml",
					"content":   pipelineCIYAML,
				}},
			},
		})
		if ciErr != nil {
			t.Fatalf("meta commit CI config: %v", ciErr)
		}

		var mPipelineID int64
		var mJobID int64

		t.Run("Meta/Create", func(t *testing.T) {
			out, err := callToolOn[pipelines.DetailOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
				"action": "create",
				"params": map[string]any{
					"project_id": projM.pidStr(),
					"ref":        "main",
				},
			})
			if err != nil {
				t.Fatalf("meta pipeline create: %v", err)
			}
			mPipelineID = out.ID
			t.Logf("Meta created pipeline ID=%d", mPipelineID)
		})

		t.Run("Meta/Get", func(t *testing.T) {
			out, err := callToolOn[pipelines.DetailOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
				"action": "get",
				"params": map[string]any{
					"project_id":  projM.pidStr(),
					"pipeline_id": mPipelineID,
				},
			})
			if err != nil {
				t.Fatalf("meta pipeline get: %v", err)
			}
			if out.ID != mPipelineID {
				t.Fatalf("expected pipeline ID %d, got %d", mPipelineID, out.ID)
			}
		})

		t.Run("Meta/List", func(t *testing.T) {
			out, err := callToolOn[pipelines.ListOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
				"action": "list",
				"params": map[string]any{
					"project_id": projM.pidStr(),
				},
			})
			if err != nil {
				t.Fatalf("meta pipeline list: %v", err)
			}
			if len(out.Pipelines) == 0 {
				t.Fatal("expected at least one pipeline (meta)")
			}
		})

		t.Run("Meta/WaitAndJobList", func(t *testing.T) {
			status := waitForPipeline(t, sess.glClient, projM.ID, mPipelineID, 900*time.Second)
			t.Logf("Meta pipeline %d finished: %s", mPipelineID, status)

			out, err := callToolOn[jobs.ListOutput](ctx, sess.meta, "gitlab_job", map[string]any{
				"action": "list",
				"params": map[string]any{
					"project_id":  projM.pidStr(),
					"pipeline_id": mPipelineID,
				},
			})
			if err != nil {
				t.Fatalf("meta job list: %v", err)
			}
			if len(out.Jobs) == 0 {
				t.Fatal("expected at least 1 job (meta)")
			}
			mJobID = out.Jobs[0].ID
		})

		t.Run("Meta/JobGet", func(t *testing.T) {
			out, err := callToolOn[jobs.Output](ctx, sess.meta, "gitlab_job", map[string]any{
				"action": "get",
				"params": map[string]any{
					"project_id": projM.pidStr(),
					"job_id":     mJobID,
				},
			})
			if err != nil {
				t.Fatalf("meta job get: %v", err)
			}
			if out.ID != mJobID {
				t.Fatalf("expected job ID %d, got %d", mJobID, out.ID)
			}
		})

		t.Run("Meta/JobTrace", func(t *testing.T) {
			out, err := callToolOn[jobs.TraceOutput](ctx, sess.meta, "gitlab_job", map[string]any{
				"action": "trace",
				"params": map[string]any{
					"project_id": projM.pidStr(),
					"job_id":     mJobID,
				},
			})
			if err != nil {
				t.Fatalf("meta job trace: %v", err)
			}
			if len(out.Trace) == 0 {
				t.Fatal("expected non-empty job trace (meta)")
			}
		})

		t.Run("Meta/Retry", func(t *testing.T) {
			_, err := callToolOn[pipelines.DetailOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
				"action": "retry",
				"params": map[string]any{
					"project_id":  projM.pidStr(),
					"pipeline_id": mPipelineID,
				},
			})
			if err != nil {
				t.Fatalf("meta pipeline retry: %v", err)
			}
			waitForPipeline(t, sess.glClient, projM.ID, mPipelineID, 900*time.Second)
		})

		t.Run("Meta/Delete", func(t *testing.T) {
			err := callToolVoidOn(ctx, sess.meta, "gitlab_pipeline", map[string]any{
				"action": "delete",
				"params": map[string]any{
					"project_id":  projM.pidStr(),
					"pipeline_id": mPipelineID,
				},
			})
			if err != nil {
				t.Fatalf("meta pipeline delete: %v", err)
			}
		})
	})
}
