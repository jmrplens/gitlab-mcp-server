//go:build e2e && !enterprise

// wait_ce_test.go tests the gitlab_pipeline_wait and gitlab_job_wait MCP tools
// against a live GitLab instance. Requires a CI runner.
// Covers both individual tools and the gitlab_pipeline/gitlab_job meta-tools.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pipelines"
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
	if !sess.enterprise {
		t.Parallel()
	}
	RunWithCapabilities(t, []Capability{CapabilityRunner}, func(_ *E2EContext) {
		ctx, cancel := waitToolContext(t)
		defer cancel()

		runIndividualWaitToolFlow(ctx, t)
		runMetaWaitToolFlow(ctx, t)
	})
}

func waitToolContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Second)
	if deadline, ok := t.Deadline(); ok {
		cancel()
		return context.WithDeadline(context.Background(), deadline)
	}
	return ctx, cancel
}

func runIndividualWaitToolFlow(ctx context.Context, t *testing.T) {
	t.Helper()
	proj := createProject(ctx, t, sess.individual)
	commitFile(ctx, t, sess.individual, proj, "main", "init.txt", "bootstrap", "init commit")
	commitIndividualWaitCI(ctx, t, proj)
	pipelineID := createIndividualWaitPipeline(ctx, t, proj)
	waitIndividualPipeline(ctx, t, proj, pipelineID)
	jobID := firstIndividualPipelineJob(ctx, t, proj, pipelineID)
	waitIndividualJob(ctx, t, proj, jobID)
}

func commitIndividualWaitCI(ctx context.Context, t *testing.T, proj ProjectFixture) {
	t.Helper()
	commitFileCreateOrUpdate(ctx, t, sess.individual, proj, "main", ".gitlab-ci.yml", waitCIYAML, "ci: add .gitlab-ci.yml for wait tool tests")
}

func createIndividualWaitPipeline(ctx context.Context, t *testing.T, proj ProjectFixture) int64 {
	t.Helper()
	var pipelineID int64
	t.Run("Individual/PipelineCreate", func(t *testing.T) {
		out, err := callToolOn[pipelines.DetailOutput](ctx, sess.individual, "gitlab_pipeline_create", pipelines.CreateInput{ProjectID: proj.pidOf(), Ref: "main"})
		if err != nil {
			t.Fatalf("pipeline create: %v", err)
		}
		if out.ID <= 0 {
			t.Fatal("expected positive pipeline ID")
		}
		pipelineID = out.ID
		t.Logf("Created pipeline ID=%d status=%s", pipelineID, out.Status)
	})
	return pipelineID
}

func waitIndividualPipeline(ctx context.Context, t *testing.T, proj ProjectFixture, pipelineID int64) {
	t.Helper()
	t.Run("Individual/PipelineWait", func(t *testing.T) {
		drainSidekiq(ctx, t, sess.glClient)
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
		assertWaitOutput(t, "pipeline", out.FinalStatus, out.TimedOut, out.PollCount, out.WaitedFor)
	})
}

func firstIndividualPipelineJob(ctx context.Context, t *testing.T, proj ProjectFixture, pipelineID int64) int64 {
	t.Helper()
	var jobID int64
	t.Run("Individual/JobList", func(t *testing.T) {
		out, err := callToolOn[jobs.ListOutput](ctx, sess.individual, "gitlab_job_list", jobs.ListInput{ProjectID: proj.pidOf(), PipelineID: pipelineID})
		if err != nil {
			t.Fatalf("job list: %v", err)
		}
		if len(out.Jobs) == 0 {
			t.Fatal("expected at least 1 job")
		}
		jobID = out.Jobs[0].ID
		t.Logf("Found %d jobs; first: ID=%d name=%s status=%s", len(out.Jobs), jobID, out.Jobs[0].Name, out.Jobs[0].Status)
	})
	return jobID
}

func waitIndividualJob(ctx context.Context, t *testing.T, proj ProjectFixture, jobID int64) {
	t.Helper()
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
		assertWaitOutput(t, "job", out.FinalStatus, out.TimedOut, out.PollCount, out.WaitedFor)
	})
}

func runMetaWaitToolFlow(ctx context.Context, t *testing.T) {
	t.Helper()
	projM := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, projM, "main", "init.txt", "bootstrap", "init commit")
	commitMetaWaitCI(ctx, t, projM)
	pipelineID := createMetaWaitPipeline(ctx, t, projM)
	waitMetaPipeline(ctx, t, projM, pipelineID)
	jobID := firstMetaPipelineJob(ctx, t, projM, pipelineID)
	waitMetaJob(ctx, t, projM, jobID)
}

func commitMetaWaitCI(ctx context.Context, t *testing.T, projM ProjectFixture) {
	t.Helper()
	commitFileCreateOrUpdateMeta(ctx, t, sess.meta, projM, "main", ".gitlab-ci.yml", waitCIYAML, "ci: add .gitlab-ci.yml for wait tool tests")
}

func createMetaWaitPipeline(ctx context.Context, t *testing.T, projM ProjectFixture) int64 {
	t.Helper()
	var pipelineID int64
	t.Run("Meta/PipelineCreate", func(t *testing.T) {
		out, err := callToolOn[pipelines.DetailOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "create",
			"params": map[string]any{"project_id": projM.pidStr(), "ref": "main"},
		})
		if err != nil {
			t.Fatalf("meta pipeline create: %v", err)
		}
		pipelineID = out.ID
		t.Logf("Meta created pipeline ID=%d", pipelineID)
	})
	return pipelineID
}

func waitMetaPipeline(ctx context.Context, t *testing.T, projM ProjectFixture, pipelineID int64) {
	t.Helper()
	t.Run("Meta/PipelineWait", func(t *testing.T) {
		drainSidekiq(ctx, t, sess.glClient)
		timeoutSeconds := 600
		if sess.enterprise {
			timeoutSeconds = 1200
		}
		out, err := callToolOn[pipelines.WaitOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
			"action": "wait",
			"params": map[string]any{"project_id": projM.pidStr(), "pipeline_id": pipelineID, "interval_seconds": 5, "timeout_seconds": timeoutSeconds, "fail_on_error": false},
		})
		if err != nil {
			t.Fatalf("meta pipeline wait: %v", err)
		}
		assertWaitOutput(t, "meta pipeline", out.FinalStatus, out.TimedOut, out.PollCount, out.WaitedFor)
	})
}

func firstMetaPipelineJob(ctx context.Context, t *testing.T, projM ProjectFixture, pipelineID int64) int64 {
	t.Helper()
	var jobID int64
	t.Run("Meta/JobList", func(t *testing.T) {
		out, err := callToolOn[jobs.ListOutput](ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "list",
			"params": map[string]any{"project_id": projM.pidStr(), "pipeline_id": pipelineID},
		})
		if err != nil {
			t.Fatalf("meta job list: %v", err)
		}
		if len(out.Jobs) == 0 {
			t.Fatal("expected at least 1 job (meta)")
		}
		jobID = out.Jobs[0].ID
	})
	return jobID
}

func waitMetaJob(ctx context.Context, t *testing.T, projM ProjectFixture, jobID int64) {
	t.Helper()
	t.Run("Meta/JobWait", func(t *testing.T) {
		timeoutSeconds := 180
		if sess.enterprise {
			timeoutSeconds = 900
		}
		out, err := callToolOn[jobs.WaitOutput](ctx, sess.meta, "gitlab_job", map[string]any{
			"action": "wait",
			"params": map[string]any{"project_id": projM.pidStr(), "job_id": jobID, "interval_seconds": 5, "timeout_seconds": timeoutSeconds, "fail_on_error": false},
		})
		if err != nil {
			t.Fatalf("meta job wait: %v", err)
		}
		assertWaitOutput(t, "meta job", out.FinalStatus, out.TimedOut, out.PollCount, out.WaitedFor)
	})
}

func assertWaitOutput(t *testing.T, label, finalStatus string, timedOut bool, pollCount int, waitedFor string) {
	t.Helper()
	if finalStatus == "" {
		t.Fatalf("expected non-empty FinalStatus for %s", label)
	}
	if timedOut {
		t.Fatalf("%s wait timed out, last status: %s", label, finalStatus)
	}
	if pollCount <= 0 {
		t.Errorf("expected PollCount > 0 for %s", label)
	}
	if waitedFor == "" {
		t.Errorf("expected non-empty WaitedFor for %s", label)
	}
	t.Logf("%s wait done: status=%s waited=%s polls=%d", label, finalStatus, waitedFor, pollCount)
}
