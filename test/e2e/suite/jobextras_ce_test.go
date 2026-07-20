//go:build e2e && !enterprise

// jobextras_ce_test.go tests the CI job artifact, manual-job, bridge, and
// erase MCP tools against a live GitLab instance with a CI runner. Covers
// job.artifacts, job.download_artifacts, job.download_single_artifact,
// job.download_single_artifact_by_ref, job.keep_artifacts,
// job.delete_artifacts, job.erase, job.retry, job.cancel, job.play, and
// job.list_bridges through the individual tool surface.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/jobs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/pipelines"
)

// Job names, artifact path/content, and repeated tool names shared by the
// job extras subtests. The names must match the jobs declared in
// [jobExtrasCIYAML].
const (
	jobExtrasBuildJob        = "build-artifacts"
	jobExtrasManualJob       = "manual-sleep"
	jobExtrasBridgeJob       = "trigger-child"
	jobExtrasArtifactPath    = "out/artifact.txt"
	jobExtrasArtifactContent = "E2E job extras artifact content"
	jobExtrasToolCancel      = "gitlab_job_cancel"
)

// jobExtrasChildCIYAML is the child pipeline configuration triggered by the
// bridge job so job.list_bridges has a bridge to observe. Uses no runner tags
// to ensure execution on any runner, matching the suite's CI YAML convention.
const jobExtrasChildCIYAML = `child-pass:
  script:
    - echo "E2E child pipeline job"
  tags: []
`

// jobExtrasCIYAML drives one pipeline with a fast artifact-producing job, a
// manual long-sleep job (playable and deterministically cancellable while it
// sleeps), and a bridge that triggers a child pipeline. Manual jobs are
// allow_failure by default, so the pipeline reaches "success" without the
// sleep job ever running.
const jobExtrasCIYAML = `stages:
  - test
  - downstream

build-artifacts:
  stage: test
  script:
    - mkdir -p out
    - echo "E2E job extras artifact content" > out/artifact.txt
  artifacts:
    paths:
      - out/
    expire_in: 1 day
  tags: []

manual-sleep:
  stage: test
  script:
    - sleep 300
  when: manual
  tags: []

trigger-child:
  stage: downstream
  trigger:
    include:
      - local: child-ci.yml
`

// TestIndividual_JobExtras exercises the previously uncovered job actions on
// a single pipeline using individual MCP tools: list_bridges, artifacts,
// download_artifacts, download_single_artifact,
// download_single_artifact_by_ref, keep_artifacts, play, cancel, retry,
// delete_artifacts, and erase.
//
// NOT parallelized: pipeline-heavy tests share a single CI runner. Running
// them concurrently causes pipelines to queue, leading to spurious timeouts.
// Outside Docker mode the test skips when no runner picks up the pipeline's
// jobs within the pickup budget.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_JobExtras(t *testing.T) {
	RunWithCapabilities(t, []Capability{CapabilityRunner}, func(_ *E2EContext) {
		ctx, cancel := context.WithTimeout(context.Background(), 1800*time.Second)
		defer cancel()

		proj := jobExtrasSetupProject(ctx, t)
		pipelineID := jobExtrasCreatePipeline(ctx, t, proj)
		status := jobExtrasWaitPipelineOrSkip(ctx, t, proj, pipelineID)
		requireTruef(t, status == "success", "expected pipeline %d to succeed, got %q", pipelineID, status)

		buildJob := jobExtrasFindJob(ctx, t, proj, pipelineID, jobExtrasBuildJob)
		manualJob := jobExtrasFindJob(ctx, t, proj, pipelineID, jobExtrasManualJob)

		runJobExtrasBridgeAndArtifactReads(ctx, t, proj, pipelineID, buildJob.ID)
		runJobExtrasManualLifecycle(ctx, t, proj, manualJob.ID)
		runJobExtrasArtifactCleanupOps(ctx, t, proj, buildJob.ID)
	})
}

// jobExtrasSetupProject creates a project and seeds it with the child
// pipeline config and the main CI YAML. The child config is committed first
// so the push-triggered pipeline for the main YAML can already resolve the
// local include.
func jobExtrasSetupProject(ctx context.Context, t *testing.T) ProjectFixture {
	t.Helper()
	proj := createProject(ctx, t, sess.individual)
	commitFile(ctx, t, sess.individual, proj, defaultBranch, "child-ci.yml", jobExtrasChildCIYAML, "ci: add child pipeline config")
	commitFileCreateOrUpdate(ctx, t, sess.individual, proj, defaultBranch, ".gitlab-ci.yml", jobExtrasCIYAML, "ci: add job extras pipeline config")
	return proj
}

// jobExtrasCreatePipeline creates a pipeline on the default branch and
// returns its ID. Pipeline creation is already covered elsewhere; it is
// reused here only as fixture setup.
func jobExtrasCreatePipeline(ctx context.Context, t *testing.T, proj ProjectFixture) int64 {
	t.Helper()
	out, err := callToolOn[pipelines.DetailOutput](ctx, sess.individual, "gitlab_pipeline_create", pipelines.CreateInput{
		ProjectID: proj.pidOf(),
		Ref:       defaultBranch,
	})
	requireNoError(t, err, "pipeline create")
	requireTruef(t, out.ID > 0, "expected positive pipeline ID")
	t.Logf("Created job extras pipeline ID=%d status=%s", out.ID, out.Status)
	return out.ID
}

// jobExtrasWaitPipelineOrSkip waits for the pipeline to reach a terminal
// status and returns it. Outside Docker mode a registered runner may still
// never pick up the suite's untagged jobs, so the test skips when the
// pipeline has not left the queue within the pickup budget instead of
// failing after the full wait.
func jobExtrasWaitPipelineOrSkip(ctx context.Context, t *testing.T, proj ProjectFixture, pipelineID int64) string {
	t.Helper()
	if !isDockerMode() && !jobExtrasPipelinePickedUp(ctx, proj.ID, pipelineID, 180*time.Second) {
		t.Skipf("no runner picked up jobs for pipeline %d within 180s; job extras need a runner that processes untagged jobs", pipelineID)
	}
	return waitForPipeline(ctx, t, sess.glClient, proj.ID, pipelineID, 900*time.Second)
}

// jobExtrasPipelinePickedUp polls until the pipeline leaves the queued
// statuses (created/pending/waiting_for_resource), reporting whether a
// runner started processing it within the budget. Transient API errors are
// tolerated and re-polled.
func jobExtrasPipelinePickedUp(ctx context.Context, projectID, pipelineID int64, budget time.Duration) bool {
	pollCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	picked := false
	_ = Poll(pollCtx, 5*time.Second, budget, func() (bool, string, error) {
		p, _, err := sess.glClient.GL().Pipelines.GetPipeline(projectID, pipelineID, gl.WithContext(pollCtx))
		if err != nil {
			return false, fmt.Sprintf("pipeline %d: %v", pipelineID, err), nil
		}
		switch p.Status {
		case "created", "pending", "waiting_for_resource":
			return false, "pipeline status " + p.Status, nil
		default:
			picked = true
			return true, "pipeline status " + p.Status, nil
		}
	})
	return picked
}

// jobExtrasFindJob lists the pipeline's jobs and returns the one with the
// given name, failing the test when it is absent.
func jobExtrasFindJob(ctx context.Context, t *testing.T, proj ProjectFixture, pipelineID int64, name string) jobs.Output {
	t.Helper()
	out, err := callToolOn[jobs.ListOutput](ctx, sess.individual, "gitlab_job_list", jobs.ListInput{
		ProjectID:  proj.pidOf(),
		PipelineID: pipelineID,
	})
	requireNoError(t, err, "job list")
	for _, j := range out.Jobs {
		if j.Name == name {
			return j
		}
	}
	t.Fatalf("job %q not found among %d jobs of pipeline %d", name, len(out.Jobs), pipelineID)
	return jobs.Output{}
}

// runJobExtrasBridgeAndArtifactReads drives the read-side extras against the
// finished pipeline: bridge listing plus the four artifact download shapes
// and keep_artifacts. Ordered before the destructive artifact operations so
// the archive still exists.
func runJobExtrasBridgeAndArtifactReads(ctx context.Context, t *testing.T, proj ProjectFixture, pipelineID, buildJobID int64) {
	t.Helper()

	t.Run("ListBridges", func(t *testing.T) {
		out, err := callToolOn[jobs.BridgeListOutput](ctx, sess.individual, "gitlab_job_list_bridges", jobs.BridgeListInput{
			ProjectID:  proj.pidOf(),
			PipelineID: pipelineID,
		})
		requireNoError(t, err, "job list bridges")
		requireTruef(t, len(out.Bridges) >= 1, "expected at least 1 bridge job, got %d", len(out.Bridges))
		requireTruef(t, out.Bridges[0].Name == jobExtrasBridgeJob, "expected bridge %q, got %q", jobExtrasBridgeJob, out.Bridges[0].Name)
		t.Logf("Listed %d bridges; first: %s status=%s", len(out.Bridges), out.Bridges[0].Name, out.Bridges[0].Status)
	})

	t.Run("Artifacts", func(t *testing.T) {
		out, err := callToolOn[jobs.ArtifactsOutput](ctx, sess.individual, "gitlab_job_artifacts", jobs.GetInput{
			ProjectID: proj.pidOf(),
			JobID:     buildJobID,
		})
		requireNoError(t, err, "job artifacts")
		requireTruef(t, out.Size > 0, "expected non-empty artifact archive")
		archive, decErr := base64.StdEncoding.DecodeString(out.Content)
		requireNoError(t, decErr, "decode artifact archive")
		requireTruef(t, len(archive) == out.Size, "expected decoded archive of %d bytes, got %d", out.Size, len(archive))
		t.Logf("Downloaded artifact archive: %d bytes (truncated=%v)", out.Size, out.Truncated)
	})

	t.Run("DownloadArtifacts", func(t *testing.T) {
		out, err := callToolOn[jobs.ArtifactsOutput](ctx, sess.individual, "gitlab_job_download_artifacts", jobs.DownloadArtifactsInput{
			ProjectID: proj.pidOf(),
			RefName:   defaultBranch,
			JobName:   jobExtrasBuildJob,
		})
		requireNoError(t, err, "job download artifacts by ref")
		requireTruef(t, out.Size > 0, "expected non-empty artifact archive for ref %s", defaultBranch)
	})

	t.Run("DownloadSingleArtifact", func(t *testing.T) {
		out, err := callToolOn[jobs.SingleArtifactOutput](ctx, sess.individual, "gitlab_job_download_single_artifact", jobs.SingleArtifactInput{
			ProjectID:    proj.pidOf(),
			JobID:        buildJobID,
			ArtifactPath: jobExtrasArtifactPath,
		})
		requireNoError(t, err, "job download single artifact")
		got := strings.TrimSpace(out.Content)
		requireTruef(t, got == jobExtrasArtifactContent, "expected artifact content %q, got %q", jobExtrasArtifactContent, got)
	})

	t.Run("DownloadSingleArtifactByRef", func(t *testing.T) {
		out, err := callToolOn[jobs.SingleArtifactOutput](ctx, sess.individual, "gitlab_job_download_single_artifact_by_ref", jobs.SingleArtifactRefInput{
			ProjectID:    proj.pidOf(),
			RefName:      defaultBranch,
			ArtifactPath: jobExtrasArtifactPath,
			JobName:      jobExtrasBuildJob,
		})
		requireNoError(t, err, "job download single artifact by ref")
		got := strings.TrimSpace(out.Content)
		requireTruef(t, got == jobExtrasArtifactContent, "expected artifact content %q, got %q", jobExtrasArtifactContent, got)
	})

	t.Run("KeepArtifacts", func(t *testing.T) {
		out, err := callToolOn[jobs.Output](ctx, sess.individual, "gitlab_job_keep_artifacts", jobs.ActionInput{
			ProjectID: proj.pidOf(),
			JobID:     buildJobID,
		})
		requireNoError(t, err, "job keep artifacts")
		requireTruef(t, out.ID == buildJobID, "expected job ID %d, got %d", buildJobID, out.ID)
	})
}

// runJobExtrasManualLifecycle drives the manual sleep job through play →
// cancel → retry. Playing a manual job starts the same job ID; canceling a
// pending/running sleep is deterministic; retrying the canceled job creates
// a new job, which is then best-effort canceled so a 300 s sleep cannot
// occupy the shared runner after the test.
func runJobExtrasManualLifecycle(ctx context.Context, t *testing.T, proj ProjectFixture, manualJobID int64) {
	t.Helper()
	var retriedID int64

	t.Run("Play", func(t *testing.T) {
		out, err := callToolOn[jobs.Output](ctx, sess.individual, "gitlab_job_play", jobs.PlayInput{
			ProjectID: proj.pidOf(),
			JobID:     manualJobID,
		})
		requireNoError(t, err, "job play")
		requireTruef(t, out.ID == manualJobID, "expected played job ID %d, got %d", manualJobID, out.ID)
		t.Logf("Played manual job %d: status=%s", out.ID, out.Status)
	})

	// Playing a manual job only queues it. Canceling before the runner has the
	// sleep job stably running races an optimistic-lock "409 Resource lock", and
	// retrying a not-yet-canceled job is a "403 not retryable". Gate each action
	// on the job's actual status so the lifecycle is deterministic.
	runningStatus := jobExtrasWaitJobStatus(ctx, t, proj.ID, manualJobID, 180*time.Second, "running")
	t.Logf("Manual job %d reached status %q; safe to cancel", manualJobID, runningStatus)

	t.Run("Cancel", func(t *testing.T) {
		// The job is running; a rare optimistic-lock 409 can still occur if the
		// runner updates the build at the same instant, so retry the transient
		// conflict until the cancel takes.
		var out jobs.Output
		err := Poll(ctx, 2*time.Second, 60*time.Second, func() (bool, string, error) {
			o, cerr := callToolOn[jobs.Output](ctx, sess.individual, jobExtrasToolCancel, jobs.CancelInput{
				ProjectID: proj.pidOf(),
				JobID:     manualJobID,
			})
			if cerr != nil {
				if isTransientJobConflict(cerr) {
					return false, "cancel conflict, retrying: " + cerr.Error(), nil
				}
				return false, "", cerr
			}
			out = o
			return true, "canceled", nil
		})
		requireNoError(t, err, "job cancel")
		requireTruef(t, out.ID == manualJobID, "expected canceled job ID %d, got %d", manualJobID, out.ID)
		t.Logf("Canceled job %d: status=%s", out.ID, out.Status)
	})

	// Retrying requires the cancel to have settled to the terminal "canceled"
	// status; a still-canceling job is not yet retryable.
	canceledStatus := jobExtrasWaitJobStatus(ctx, t, proj.ID, manualJobID, 120*time.Second, "canceled")
	t.Logf("Manual job %d reached status %q; safe to retry", manualJobID, canceledStatus)

	t.Run("Retry", func(t *testing.T) {
		out, err := callToolOn[jobs.Output](ctx, sess.individual, "gitlab_job_retry", jobs.ActionInput{
			ProjectID: proj.pidOf(),
			JobID:     manualJobID,
		})
		requireNoError(t, err, "job retry")
		requireTruef(t, out.ID > 0, "expected positive retried job ID")
		retriedID = out.ID
		t.Logf("Retried job %d as new job %d: status=%s", manualJobID, out.ID, out.Status)
	})

	jobExtrasCancelBestEffort(ctx, t, proj, retriedID)
}

// jobExtrasWaitJobStatus polls a job until its status matches one of want,
// returning the observed status and failing the test if the budget elapses
// first. Transient API errors are tolerated and re-polled. Used to gate the
// manual-job cancel/retry lifecycle on the job's real state instead of racing
// GitLab's build state machine.
func jobExtrasWaitJobStatus(ctx context.Context, t *testing.T, projectID, jobID int64, budget time.Duration, want ...string) string {
	t.Helper()
	wantSet := make(map[string]bool, len(want))
	for _, s := range want {
		wantSet[s] = true
	}
	var observed string
	err := Poll(ctx, 3*time.Second, budget, func() (bool, string, error) {
		j, _, gerr := sess.glClient.GL().Jobs.GetJob(projectID, jobID, gl.WithContext(ctx))
		if gerr != nil {
			return false, fmt.Sprintf("job %d: %v", jobID, gerr), nil
		}
		observed = j.Status
		return wantSet[j.Status], "job status " + j.Status, nil
	})
	requireNoError(t, err, fmt.Sprintf("wait job %d for %v", jobID, want))
	return observed
}

// isTransientJobConflict reports whether a job action error is a retryable
// optimistic-lock conflict (HTTP 409 / "Resource lock") that GitLab raises when
// a build row is updated concurrently, as opposed to a terminal failure.
func isTransientJobConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "409") ||
		strings.Contains(msg, "resource lock") ||
		strings.Contains(msg, "conflict")
}

// jobExtrasCancelBestEffort cancels a job without failing the test: the
// retried sleep job only needs to be stopped so it cannot block later
// pipeline tests, and a force cancel absorbs non-cancellable states (for
// example when the retried manual job returned to the manual state).
func jobExtrasCancelBestEffort(ctx context.Context, t *testing.T, proj ProjectFixture, jobID int64) {
	t.Helper()
	if jobID <= 0 {
		return
	}
	_, err := callToolOn[jobs.Output](ctx, sess.individual, jobExtrasToolCancel, jobs.CancelInput{
		ProjectID: proj.pidOf(),
		JobID:     jobID,
	})
	if err == nil {
		return
	}
	_, forceErr := callToolOn[jobs.Output](ctx, sess.individual, jobExtrasToolCancel, jobs.CancelInput{
		ProjectID: proj.pidOf(),
		JobID:     jobID,
		Force:     true,
	})
	if forceErr != nil {
		t.Logf("best-effort cancel of retried job %d failed (non-fatal): %v / force: %v", jobID, err, forceErr)
	}
}

// runJobExtrasArtifactCleanupOps deletes the build job's artifacts and then
// erases the job. Ordered last because these destructive operations remove
// the archive and trace that the read subtests depend on; erase stays valid
// after delete_artifacts because the trace still exists.
func runJobExtrasArtifactCleanupOps(ctx context.Context, t *testing.T, proj ProjectFixture, buildJobID int64) {
	t.Helper()

	t.Run("DeleteArtifacts", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.individual, "gitlab_job_delete_artifacts", jobs.DeleteArtifactsInput{
			ProjectID: proj.pidOf(),
			JobID:     buildJobID,
		})
		requireNoError(t, err, "job delete artifacts")
	})

	t.Run("Erase", func(t *testing.T) {
		out, err := callToolOn[jobs.Output](ctx, sess.individual, "gitlab_job_erase", jobs.ActionInput{
			ProjectID: proj.pidOf(),
			JobID:     buildJobID,
		})
		requireNoError(t, err, "job erase")
		requireTruef(t, out.ID == buildJobID, "expected erased job ID %d, got %d", buildJobID, out.ID)
	})
}
