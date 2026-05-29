//go:build e2e && !enterprise

// pipelines_ce_test.go tests the pipeline and job MCP tools against a live GitLab instance.
// Requires Docker mode with a CI runner. Covers pipeline create, get, list, retry, delete,
// and job list, get, trace for both individual tools and the gitlab_pipeline/gitlab_job meta-tools.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pipelines"
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
	RunWithCapabilities(t, []Capability{CapabilityRunner}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Second)
		if deadline, ok := t.Deadline(); ok {
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), deadline)
		}
		defer cancel()

		runIndividualPipelineLifecycle(ctx, t)
		runMetaPipelineLifecycle(ctx, t)
	})
}

func runIndividualPipelineLifecycle(ctx context.Context, t *testing.T) {
	t.Helper()
	proj := setupIndividualPipelineProject(ctx, t)

	var pipelineID int64
	var jobID int64

	t.Run("Individual/Create", func(t *testing.T) {
		pipelineID = createIndividualPipeline(ctx, t, proj)
	})

	t.Run("Individual/Get", func(t *testing.T) {
		assertIndividualPipelineGet(ctx, t, proj, pipelineID)
	})

	t.Run("Individual/List", func(t *testing.T) {
		assertIndividualPipelineList(ctx, t, proj)
	})

	t.Run("Individual/WaitAndJobList", func(t *testing.T) {
		jobID = waitAndListIndividualJobs(ctx, t, proj, pipelineID)
	})

	t.Run("Individual/JobGet", func(t *testing.T) {
		assertIndividualJobGet(ctx, t, proj, jobID)
	})

	t.Run("Individual/JobTrace", func(t *testing.T) {
		assertIndividualJobTrace(ctx, t, proj, jobID)
	})

	t.Run("Individual/Retry", func(t *testing.T) {
		retryIndividualPipeline(ctx, t, proj, pipelineID)
	})

	t.Run("Individual/Delete", func(t *testing.T) {
		deleteIndividualPipeline(ctx, t, proj, pipelineID)
	})
}

func setupIndividualPipelineProject(ctx context.Context, t *testing.T) ProjectFixture {
	t.Helper()
	proj := createProject(ctx, t, sess.individual)
	commitFile(ctx, t, sess.individual, proj, "main", "init.txt", "bootstrap", "init commit")
	commitFileCreateOrUpdate(ctx, t, sess.individual, proj, "main", ".gitlab-ci.yml", pipelineCIYAML, "ci: add .gitlab-ci.yml for pipeline tests")
	return proj
}

func createIndividualPipeline(ctx context.Context, t *testing.T, proj ProjectFixture) int64 {
	t.Helper()
	out, err := callToolOn[pipelines.DetailOutput](ctx, sess.individual, "gitlab_pipeline_create", pipelines.CreateInput{ProjectID: proj.pidOf(), Ref: "main"})
	if err != nil {
		t.Fatalf("pipeline create: %v", err)
	}
	if out.ID <= 0 {
		t.Fatal("expected positive pipeline ID")
	}
	t.Logf("Created pipeline ID=%d status=%s", out.ID, out.Status)
	return out.ID
}

func assertIndividualPipelineGet(ctx context.Context, t *testing.T, proj ProjectFixture, pipelineID int64) {
	t.Helper()
	out, err := callToolOn[pipelines.DetailOutput](ctx, sess.individual, "gitlab_pipeline_get", pipelines.GetInput{ProjectID: proj.pidOf(), PipelineID: pipelineID})
	if err != nil {
		t.Fatalf("pipeline get: %v", err)
	}
	if out.ID != pipelineID {
		t.Fatalf("expected pipeline ID %d, got %d", pipelineID, out.ID)
	}
}

func assertIndividualPipelineList(ctx context.Context, t *testing.T, proj ProjectFixture) {
	t.Helper()
	out, err := callToolOn[pipelines.ListOutput](ctx, sess.individual, "gitlab_pipeline_list", pipelines.ListInput{ProjectID: proj.pidOf()})
	if err != nil {
		t.Fatalf("pipeline list: %v", err)
	}
	if len(out.Pipelines) == 0 {
		t.Fatal("expected at least one pipeline")
	}
}

func waitAndListIndividualJobs(ctx context.Context, t *testing.T, proj ProjectFixture, pipelineID int64) int64 {
	t.Helper()
	status := waitForPipeline(ctx, t, sess.glClient, proj.ID, pipelineID, 900*time.Second)
	t.Logf("Pipeline %d finished with status: %s", pipelineID, status)
	out, err := callToolOn[jobs.ListOutput](ctx, sess.individual, "gitlab_job_list", jobs.ListInput{ProjectID: proj.pidOf(), PipelineID: pipelineID})
	if err != nil {
		t.Fatalf("job list: %v", err)
	}
	if len(out.Jobs) == 0 {
		t.Fatal("expected at least 1 job")
	}
	t.Logf("Listed %d jobs; first: ID=%d name=%s status=%s", len(out.Jobs), out.Jobs[0].ID, out.Jobs[0].Name, out.Jobs[0].Status)
	return out.Jobs[0].ID
}

func assertIndividualJobGet(ctx context.Context, t *testing.T, proj ProjectFixture, jobID int64) {
	t.Helper()
	out, err := callToolOn[jobs.Output](ctx, sess.individual, "gitlab_job_get", jobs.GetInput{ProjectID: proj.pidOf(), JobID: jobID})
	if err != nil {
		t.Fatalf("job get: %v", err)
	}
	if out.ID != jobID {
		t.Fatalf("expected job ID %d, got %d", jobID, out.ID)
	}
}

func assertIndividualJobTrace(ctx context.Context, t *testing.T, proj ProjectFixture, jobID int64) {
	t.Helper()
	out, err := callToolOn[jobs.TraceOutput](ctx, sess.individual, "gitlab_job_trace", jobs.TraceInput{ProjectID: proj.pidOf(), JobID: jobID})
	if err != nil {
		t.Fatalf("job trace: %v", err)
	}
	if len(out.Trace) == 0 {
		t.Fatal("expected non-empty job trace")
	}
	t.Logf("Job trace: %d chars, truncated=%v", len(out.Trace), out.Truncated)
}

func retryIndividualPipeline(ctx context.Context, t *testing.T, proj ProjectFixture, pipelineID int64) {
	t.Helper()
	out, err := callToolOn[pipelines.DetailOutput](ctx, sess.individual, "gitlab_pipeline_retry", pipelines.ActionInput{ProjectID: proj.pidOf(), PipelineID: pipelineID})
	if err != nil {
		t.Fatalf("pipeline retry: %v", err)
	}
	t.Logf("Retried pipeline: ID=%d status=%s", out.ID, out.Status)
	waitForPipeline(ctx, t, sess.glClient, proj.ID, pipelineID, 900*time.Second)
}

func deleteIndividualPipeline(ctx context.Context, t *testing.T, proj ProjectFixture, pipelineID int64) {
	t.Helper()
	err := callToolVoidOn(ctx, sess.individual, "gitlab_pipeline_delete", pipelines.DeleteInput{ProjectID: proj.pidOf(), PipelineID: pipelineID})
	if err != nil {
		t.Fatalf("pipeline delete: %v", err)
	}
}

func runMetaPipelineLifecycle(ctx context.Context, t *testing.T) {
	t.Helper()
	projM := setupMetaPipelineProject(ctx, t)

	var mPipelineID int64
	var mJobID int64

	t.Run("Meta/Create", func(t *testing.T) {
		mPipelineID = createMetaPipeline(ctx, t, projM)
	})

	t.Run("Meta/Get", func(t *testing.T) {
		assertMetaPipelineGet(ctx, t, projM, mPipelineID)
	})

	t.Run("Meta/List", func(t *testing.T) {
		assertMetaPipelineList(ctx, t, projM)
	})

	t.Run("Meta/WaitAndJobList", func(t *testing.T) {
		mJobID = waitAndListMetaJobs(ctx, t, projM, mPipelineID)
	})

	t.Run("Meta/JobGet", func(t *testing.T) {
		assertMetaJobGet(ctx, t, projM, mJobID)
	})

	t.Run("Meta/JobTrace", func(t *testing.T) {
		assertMetaJobTrace(ctx, t, projM, mJobID)
	})

	t.Run("Meta/Retry", func(t *testing.T) {
		retryMetaPipeline(ctx, t, projM, mPipelineID)
	})

	t.Run("Meta/Delete", func(t *testing.T) {
		deleteMetaPipeline(ctx, t, projM, mPipelineID)
	})
}

func setupMetaPipelineProject(ctx context.Context, t *testing.T) ProjectFixture {
	t.Helper()
	proj := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, proj, "main", "init.txt", "bootstrap", "init commit")
	commitFileCreateOrUpdateMeta(ctx, t, sess.meta, proj, "main", ".gitlab-ci.yml", pipelineCIYAML, "ci: add .gitlab-ci.yml")
	return proj
}

func createMetaPipeline(ctx context.Context, t *testing.T, proj ProjectFixture) int64 {
	t.Helper()
	out, err := callToolOn[pipelines.DetailOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
		"action": "create",
		"params": map[string]any{"project_id": proj.pidStr(), "ref": "main"},
	})
	if err != nil {
		t.Fatalf("meta pipeline create: %v", err)
	}
	t.Logf("Meta created pipeline ID=%d", out.ID)
	return out.ID
}

func assertMetaPipelineGet(ctx context.Context, t *testing.T, proj ProjectFixture, pipelineID int64) {
	t.Helper()
	out, err := callToolOn[pipelines.DetailOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
		"action": "get",
		"params": map[string]any{"project_id": proj.pidStr(), "pipeline_id": pipelineID},
	})
	if err != nil {
		t.Fatalf("meta pipeline get: %v", err)
	}
	if out.ID != pipelineID {
		t.Fatalf("expected pipeline ID %d, got %d", pipelineID, out.ID)
	}
}

func assertMetaPipelineList(ctx context.Context, t *testing.T, proj ProjectFixture) {
	t.Helper()
	out, err := callToolOn[pipelines.ListOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
		"action": "list",
		"params": map[string]any{"project_id": proj.pidStr()},
	})
	if err != nil {
		t.Fatalf("meta pipeline list: %v", err)
	}
	if len(out.Pipelines) == 0 {
		t.Fatal("expected at least one pipeline (meta)")
	}
}

func waitAndListMetaJobs(ctx context.Context, t *testing.T, proj ProjectFixture, pipelineID int64) int64 {
	t.Helper()
	status := waitForPipeline(ctx, t, sess.glClient, proj.ID, pipelineID, 900*time.Second)
	t.Logf("Meta pipeline %d finished: %s", pipelineID, status)
	out, err := callToolOn[jobs.ListOutput](ctx, sess.meta, "gitlab_job", map[string]any{
		"action": "list",
		"params": map[string]any{"project_id": proj.pidStr(), "pipeline_id": pipelineID},
	})
	if err != nil {
		t.Fatalf("meta job list: %v", err)
	}
	if len(out.Jobs) == 0 {
		t.Fatal("expected at least 1 job (meta)")
	}
	return out.Jobs[0].ID
}

func assertMetaJobGet(ctx context.Context, t *testing.T, proj ProjectFixture, jobID int64) {
	t.Helper()
	out, err := callToolOn[jobs.Output](ctx, sess.meta, "gitlab_job", map[string]any{
		"action": "get",
		"params": map[string]any{"project_id": proj.pidStr(), "job_id": jobID},
	})
	if err != nil {
		t.Fatalf("meta job get: %v", err)
	}
	if out.ID != jobID {
		t.Fatalf("expected job ID %d, got %d", jobID, out.ID)
	}
}

func assertMetaJobTrace(ctx context.Context, t *testing.T, proj ProjectFixture, jobID int64) {
	t.Helper()
	out, err := callToolOn[jobs.TraceOutput](ctx, sess.meta, "gitlab_job", map[string]any{
		"action": "trace",
		"params": map[string]any{"project_id": proj.pidStr(), "job_id": jobID},
	})
	if err != nil {
		t.Fatalf("meta job trace: %v", err)
	}
	if len(out.Trace) == 0 {
		t.Fatal("expected non-empty job trace (meta)")
	}
}

func retryMetaPipeline(ctx context.Context, t *testing.T, proj ProjectFixture, pipelineID int64) {
	t.Helper()
	_, err := callToolOn[pipelines.DetailOutput](ctx, sess.meta, "gitlab_pipeline", map[string]any{
		"action": "retry",
		"params": map[string]any{"project_id": proj.pidStr(), "pipeline_id": pipelineID},
	})
	if err != nil {
		t.Fatalf("meta pipeline retry: %v", err)
	}
	waitForPipeline(ctx, t, sess.glClient, proj.ID, pipelineID, 900*time.Second)
}

func deleteMetaPipeline(ctx context.Context, t *testing.T, proj ProjectFixture, pipelineID int64) {
	t.Helper()
	err := callToolVoidOn(ctx, sess.meta, "gitlab_pipeline", map[string]any{
		"action": "delete",
		"params": map[string]any{"project_id": proj.pidStr(), "pipeline_id": pipelineID},
	})
	if err != nil {
		t.Fatalf("meta pipeline delete: %v", err)
	}
}
