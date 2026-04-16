//go:build e2e

package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pipelines"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/samplingtools"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ciYAML is a minimal CI configuration for pipeline E2E tests.
const ciYAML = `stages:
  - test

fast-pass:
  stage: test
  script:
    - echo "E2E fast-pass job"
  tags: []
`

// TestIndividual_CIRunner exercises the full pipeline/job lifecycle using
// individual MCP tools. Requires a CI runner (Docker mode or self-hosted runner).
// Subtests are sequential because they share pipeline/job state.
func TestIndividual_CIRunner(t *testing.T) {
	t.Parallel()
	if !hasRunner() {
		t.Skip("CI runner not available — skipping pipeline/job tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	unprotectMain(ctx, t, proj)

	var pipelineID int64
	var jobID int64

	t.Run("CommitCIConfig", func(t *testing.T) {
		_, err := callToolOn[commits.Output](ctx, sess.individual, "gitlab_commit_create", commits.CreateInput{
			ProjectID:     proj.pidOf(),
			Branch:        defaultBranch,
			CommitMessage: "ci: add .gitlab-ci.yml for E2E pipeline tests",
			Actions: []commits.Action{{
				Action:   "create",
				FilePath: ".gitlab-ci.yml",
				Content:  ciYAML,
			}},
		})
		requireNoError(t, err, "commit CI config")
		t.Logf("Committed .gitlab-ci.yml to %s", defaultBranch)
	})

	pidStr := strconv.FormatInt(proj.ID, 10)

	t.Run("PipelineCreate", func(t *testing.T) {
		out, err := callToolOn[pipelines.DetailOutput](ctx, sess.individual, "gitlab_pipeline_create", pipelines.CreateInput{
			ProjectID: toolutil.StringOrInt(pidStr),
			Ref:       defaultBranch,
		})
		requireNoError(t, err, "pipeline create")
		requireTrue(t, out.ID > 0, "pipeline ID should be positive")
		pipelineID = out.ID
		t.Logf("Created pipeline: ID=%d status=%s ref=%s", out.ID, out.Status, out.Ref)
	})

	t.Run("PipelineGet", func(t *testing.T) {
		requireTrue(t, pipelineID > 0, "pipeline ID not set")

		out, err := callToolOn[pipelines.DetailOutput](ctx, sess.individual, "gitlab_pipeline_get", pipelines.GetInput{
			ProjectID:  toolutil.StringOrInt(pidStr),
			PipelineID: pipelineID,
		})
		requireNoError(t, err, "pipeline get")
		requireTrue(t, out.ID == pipelineID, "expected pipeline ID %d, got %d", pipelineID, out.ID)
		t.Logf("Got pipeline: ID=%d status=%s", out.ID, out.Status)
	})

	t.Run("PipelineList", func(t *testing.T) {
		out, err := callToolOn[pipelines.ListOutput](ctx, sess.individual, "gitlab_pipeline_list", pipelines.ListInput{
			ProjectID: toolutil.StringOrInt(pidStr),
		})
		requireNoError(t, err, "pipeline list")
		requireTrue(t, len(out.Pipelines) >= 1, "expected at least 1 pipeline, got %d", len(out.Pipelines))
		t.Logf("Listed %d pipelines", len(out.Pipelines))
	})

	t.Run("WaitAndJobList", func(t *testing.T) {
		requireTrue(t, pipelineID > 0, "pipeline ID not set")

		status := waitForPipeline(t, proj.ID, pipelineID, 180*time.Second)
		t.Logf("Pipeline %d finished with status: %s", pipelineID, status)

		out, err := callToolOn[jobs.ListOutput](ctx, sess.individual, "gitlab_job_list", jobs.ListInput{
			ProjectID:  toolutil.StringOrInt(pidStr),
			PipelineID: pipelineID,
		})
		requireNoError(t, err, "job list")
		requireTrue(t, len(out.Jobs) >= 1, "expected at least 1 job, got %d", len(out.Jobs))
		jobID = out.Jobs[0].ID
		t.Logf("Listed %d jobs; first job: ID=%d name=%s status=%s", len(out.Jobs), out.Jobs[0].ID, out.Jobs[0].Name, out.Jobs[0].Status)
	})

	t.Run("JobGet", func(t *testing.T) {
		requireTrue(t, jobID > 0, "job ID not set")

		out, err := callToolOn[jobs.Output](ctx, sess.individual, "gitlab_job_get", jobs.GetInput{
			ProjectID: toolutil.StringOrInt(pidStr),
			JobID:     jobID,
		})
		requireNoError(t, err, "job get")
		requireTrue(t, out.ID == jobID, "expected job ID %d, got %d", jobID, out.ID)
		t.Logf("Got job: ID=%d name=%s status=%s", out.ID, out.Name, out.Status)
	})

	t.Run("JobTrace", func(t *testing.T) {
		requireTrue(t, jobID > 0, "job ID not set")

		out, err := callToolOn[jobs.TraceOutput](ctx, sess.individual, "gitlab_job_trace", jobs.TraceInput{
			ProjectID: toolutil.StringOrInt(pidStr),
			JobID:     jobID,
		})
		requireNoError(t, err, "job trace")
		requireTrue(t, len(out.Trace) > 0, "expected non-empty job trace")
		t.Logf("Got job trace: %d chars (truncated=%v)", len(out.Trace), out.Truncated)
	})

	t.Run("SamplingAnalyzePipelineFailure", func(t *testing.T) {
		requireTrue(t, pipelineID > 0, "pipeline ID not set")

		out, err := callToolOn[samplingtools.AnalyzePipelineFailureOutput](ctx, sess.sampling, "gitlab_analyze_pipeline_failure", samplingtools.AnalyzePipelineFailureInput{
			ProjectID:  toolutil.StringOrInt(pidStr),
			PipelineID: pipelineID,
		})
		requireNoError(t, err, "sampling analyze pipeline failure")
		requireTrue(t, out.Analysis != "", "expected non-empty analysis")
		requireTrue(t, out.Model == "e2e-mock-model", "expected mock model, got %q", out.Model)
		t.Logf("Analyzed pipeline failure: model=%s, analysis_len=%d", out.Model, len(out.Analysis))
	})

	t.Run("PipelineRetry", func(t *testing.T) {
		requireTrue(t, pipelineID > 0, "pipeline ID not set")

		out, err := callToolOn[pipelines.DetailOutput](ctx, sess.individual, "gitlab_pipeline_retry", pipelines.ActionInput{
			ProjectID:  toolutil.StringOrInt(pidStr),
			PipelineID: pipelineID,
		})
		requireNoError(t, err, "pipeline retry")
		t.Logf("Retried pipeline: ID=%d status=%s", out.ID, out.Status)

		waitForPipeline(t, proj.ID, pipelineID, 180*time.Second)
	})

	t.Run("PipelineDelete", func(t *testing.T) {
		requireTrue(t, pipelineID > 0, "pipeline ID not set")

		err := callToolVoidOn(ctx, sess.individual, "gitlab_pipeline_delete", pipelines.DeleteInput{
			ProjectID:  toolutil.StringOrInt(pidStr),
			PipelineID: pipelineID,
		})
		requireNoError(t, err, "pipeline delete")
		pipelineID = 0
		t.Logf("Deleted pipeline")
	})
}
