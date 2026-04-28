//go:build e2e

// wait_test.go tests the gitlab_pipeline_wait and gitlab_job_wait MCP tools
// against a live GitLab instance. Requires Docker mode with a CI runner.
// Covers both individual tools and the gitlab_pipeline/gitlab_job meta-tools.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelines"
)

// waitCIYAML is a minimal .gitlab-ci.yml with a fast job for wait tool tests.
const waitCIYAML = `stages:
  - test

wait-job:
  stage: test
  script:
    - echo "E2E wait tool test"
  tags: []
`

// TestWaitTools exercises gitlab_pipeline_wait and gitlab_job_wait for both
// individual and meta-tool sessions. Creates a pipeline, waits for it via the
// MCP wait tool (not the direct API helper), then waits for each job.
func TestWaitTools(t *testing.T) {
	t.Parallel()
	if !isDockerMode() {
		t.Skip("wait tool tests require Docker mode with CI runner")
	}

	ctx := context.Background()

	// --- Individual tool session ---
	proj := createProject(ctx, t, sess.individual)
	commitFile(ctx, t, sess.individual, proj, "main", "init.txt", "bootstrap", "init commit")

	_, ciErr := callToolOn[commits.Output](ctx, sess.individual, "gitlab_commit_create", commits.CreateInput{
		ProjectID:     proj.pidOf(),
		Branch:        "main",
		CommitMessage: "ci: add .gitlab-ci.yml for wait tool tests",
		Actions: []commits.Action{{
			Action:   "create",
			FilePath: ".gitlab-ci.yml",
			Content:  waitCIYAML,
		}},
	})
	if ciErr != nil {
		t.Fatalf("commit CI config: %v", ciErr)
	}

	var pipelineID int64

	t.Run("Individual/PipelineCreate", func(t *testing.T) {
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

	t.Run("Individual/PipelineWait", func(t *testing.T) {
		drainSidekiq(ctx, t)
		failOnErr := false
		out, err := callToolOn[pipelines.WaitOutput](ctx, sess.individual, "gitlab_pipeline_wait", pipelines.WaitInput{
			ProjectID:       proj.pidOf(),
			PipelineID:      pipelineID,
			IntervalSeconds: 5,
			TimeoutSeconds:  600,
			FailOnError:     &failOnErr,
		})
		if err != nil {
			t.Fatalf("pipeline wait: %v", err)
		}
		if out.FinalStatus == "" {
			t.Fatal("expected non-empty FinalStatus")
		}
		if out.TimedOut {
			t.Fatalf("pipeline wait timed out, last status: %s", out.FinalStatus)
		}
		if out.PollCount <= 0 {
			t.Error("expected PollCount > 0")
		}
		if out.WaitedFor == "" {
			t.Error("expected non-empty WaitedFor")
		}
		t.Logf("Pipeline wait done: status=%s waited=%s polls=%d", out.FinalStatus, out.WaitedFor, out.PollCount)
	})

	var jobID int64

	t.Run("Individual/JobList", func(t *testing.T) {
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
		t.Logf("Found %d jobs; first: ID=%d name=%s status=%s", len(out.Jobs), jobID, out.Jobs[0].Name, out.Jobs[0].Status)
	})

	t.Run("Individual/JobWait", func(t *testing.T) {
		failOnErr := false
		out, err := callToolOn[jobs.WaitOutput](ctx, sess.individual, "gitlab_job_wait", jobs.WaitInput{
			ProjectID:       proj.pidOf(),
			JobID:           jobID,
			IntervalSeconds: 5,
			TimeoutSeconds:  600,
			FailOnError:     &failOnErr,
		})
		if err != nil {
			t.Fatalf("job wait: %v", err)
		}
		if out.FinalStatus == "" {
			t.Fatal("expected non-empty FinalStatus")
		}
		if out.TimedOut {
			t.Fatalf("job wait timed out, last status: %s", out.FinalStatus)
		}
		if out.PollCount <= 0 {
			t.Error("expected PollCount > 0")
		}
		if out.WaitedFor == "" {
			t.Error("expected non-empty WaitedFor")
		}
		t.Logf("Job wait done: status=%s waited=%s polls=%d", out.FinalStatus, out.WaitedFor, out.PollCount)
	})

	// --- Meta-tool session ---
	projM := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, projM, "main", "init.txt", "bootstrap", "init commit")

	_, ciErr = callToolOn[commits.Output](ctx, sess.meta, "gitlab_repository", map[string]any{
		"action": "commit_create",
		"params": map[string]any{
			"project_id":     projM.pidStr(),
			"branch":         "main",
			"commit_message": "ci: add .gitlab-ci.yml for wait tool tests",
			"actions": []map[string]any{{
				"action":    "create",
				"file_path": ".gitlab-ci.yml",
				"content":   waitCIYAML,
			}},
		},
	})
	if ciErr != nil {
		t.Fatalf("meta commit CI config: %v", ciErr)
	}

	var mPipelineID int64

	t.Run("Meta/PipelineCreate", func(t *testing.T) {
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

	t.Run("Meta/PipelineWait", func(t *testing.T) {
		drainSidekiq(ctx, t)
		out, err := callToolOn[pipelines.WaitOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "wait",
			"params": map[string]any{
				"project_id":       projM.pidStr(),
				"pipeline_id":      mPipelineID,
				"interval_seconds": 5,
				"timeout_seconds":  180,
				"fail_on_error":    false,
			},
		})
		if err != nil {
			t.Fatalf("meta pipeline wait: %v", err)
		}
		if out.FinalStatus == "" {
			t.Fatal("expected non-empty FinalStatus (meta)")
		}
		if out.TimedOut {
			t.Fatalf("meta pipeline wait timed out, last status: %s", out.FinalStatus)
		}
		t.Logf("Meta pipeline wait done: status=%s waited=%s polls=%d", out.FinalStatus, out.WaitedFor, out.PollCount)
	})

	var mJobID int64

	t.Run("Meta/JobList", func(t *testing.T) {
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

	t.Run("Meta/JobWait", func(t *testing.T) {
		out, err := callToolOn[jobs.WaitOutput](ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "wait",
			"params": map[string]any{
				"project_id":       projM.pidStr(),
				"job_id":           mJobID,
				"interval_seconds": 5,
				"timeout_seconds":  180,
				"fail_on_error":    false,
			},
		})
		if err != nil {
			t.Fatalf("meta job wait: %v", err)
		}
		if out.FinalStatus == "" {
			t.Fatal("expected non-empty FinalStatus (meta)")
		}
		if out.TimedOut {
			t.Fatalf("meta job wait timed out, last status: %s", out.FinalStatus)
		}
		t.Logf("Meta job wait done: status=%s waited=%s polls=%d", out.FinalStatus, out.WaitedFor, out.PollCount)
	})
}
