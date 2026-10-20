//go:build e2e

// failed_job_test.go drives the failed job builder and its wait against the
// stub: the builder whole, then a listing that carries only running jobs at
// first, a listing endpoint that errors while GitLab is still creating the
// pipeline, and a budget that runs out.
//
// It also pins the two facts about the CI configuration the file's comment
// rests on, because both are invisible in a unit test against a stub: the
// artifacts are kept whatever the job's exit status, and the job that carries
// them exits non-zero.

package fixture

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// fastFailedJobPolls makes the wait ask the stub as fast as a unit test can
// afford, restoring the real cadence afterwards.
func fastFailedJobPolls(t *testing.T) {
	t.Helper()
	saved := failedJobPollInterval
	failedJobPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { failedJobPollInterval = saved })
}

// shortFailedJobWait gives the builder's wait the budget a test needs,
// polling as fast as fastFailedJobPolls does, and restores both afterwards.
// A test that drives the builder whole sets one too, so a defect that keeps
// the wait from ending fails the test at that budget rather than holding it
// for the ten minutes a runner is given.
func shortFailedJobWait(t *testing.T, wait time.Duration) {
	t.Helper()
	fastFailedJobPolls(t)
	saved := failedJobWait
	failedJobWait = wait
	t.Cleanup(func() { failedJobWait = saved })
}

// jobsPath is where the pipeline's job listing lives.
const jobsPath = "/api/v4/projects/1/pipelines/5/jobs"

// TestFailingCIYAML_Shape_KeepsTheArtifactOfAJobThatFails pins the two lines
// the artifact cases depend on: GitLab keeps nothing from a failed job unless
// the artifacts are declared `when: always`, and the job has to fail for the
// fixture to be a failed job at all.
func TestFailingCIYAML_Shape_KeepsTheArtifactOfAJobThatFails(t *testing.T) {
	if !strings.Contains(FailingCIYAML, "when: always") {
		t.Error("FailingCIYAML declares no `when: always`; GitLab keeps no artifact from a job that failed")
	}
	if !strings.Contains(FailingCIYAML, "exit 1") {
		t.Error("FailingCIYAML has no failing job, so nothing it builds is a failed job fixture")
	}
	if !strings.Contains(FailingCIYAML, FailedJobArtifactPath) {
		t.Errorf("FailingCIYAML never names %s, which is the path an artifact case asks for", FailedJobArtifactPath)
	}
}

// failedJobProject is the project the failed job builder is driven against,
// on a default branch other than the library's own, so a builder that named
// the library's default rather than the project's would be seen to.
var failedJobProject = Project{ID: 1, DefaultBranch: "trunk"}

// scriptFailedJob tells the stub to accept what the failed job builder asks
// for before its wait: the runner it checks for, the configuration's commit
// and the pipeline, numbered 5 so its jobs are at jobsPath.
func scriptFailedJob(stub *stubGitLab) {
	stub.answers(http.MethodGet, "/api/v4/runners", oneRunner)
	stub.answers(http.MethodPost, "/api/v4/projects/1/repository/commits", stubCreated(map[string]any{"id": "ci-sha", "short_id": "ci-sha"}))
	stub.answers(http.MethodPost, "/api/v4/projects/1/pipeline", stubCreated(map[string]any{"id": 5, "ref": "trunk", "sha": "ci-sha", "status": "created"}))
}

// TestNewFailedJob_Detached_SkipsThePushPipelineAndWaitsForTheFailure drives
// the builder whole: the failing configuration lands on the default branch
// with a message that keeps the push from running it, the pipeline is asked
// for on that branch, and the job that failed in it is handed back with the
// artifact path. Without the marker the push ran the failing job a second
// time, on the one runner a model evaluation run shares.
func TestNewFailedJob_Detached_SkipsThePushPipelineAndWaitsForTheFailure(t *testing.T) {
	shortFailedJobWait(t, 10*time.Second)
	stub, e := detachedStub(t)
	scriptFailedJob(stub)
	stub.answers(http.MethodGet, jobsPath, stubOK([]any{
		map[string]any{"id": 71, "name": "failing-fixture", "status": failedJobStatus},
		map[string]any{"id": 72, "name": "manual-fixture", "status": "manual"},
	}))

	got := NewFailedJob(e, failedJobProject)

	if want := (FailedJob{PipelineID: 5, JobID: 71, ArtifactPath: FailedJobArtifactPath}); got != want {
		t.Errorf("NewFailedJob() = %+v, want %+v", got, want)
	}
	assertSkipsThePushPipeline(t, ciCommitTo(t, stub, 1), "trunk", FailingCIYAML)
	if sent := requestTo(t, stub, http.MethodPost, "/api/v4/projects/1/pipeline").Body["ref"]; sent != "trunk" {
		t.Errorf("NewFailedJob() asked for a pipeline on %v, want the default branch trunk", sent)
	}
}

// TestWaitForFailedJob_FailsLater_ReturnsTheJobID checks that the wait sits
// through a listing of running jobs and answers with the one that failed,
// leaving the manual job beside it alone and passing over a null row.
func TestWaitForFailedJob_FailsLater_ReturnsTheJobID(t *testing.T) {
	fastFailedJobPolls(t)
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodGet, jobsPath,
		stubOK([]any{map[string]any{"id": 71, "name": "failing-fixture", "status": "running"}}),
		stubOK([]any{
			nil,
			map[string]any{"id": 71, "name": "failing-fixture", "status": failedJobStatus},
			map[string]any{"id": 72, "name": "manual-fixture", "status": "manual"},
		}),
	)

	got, err := waitForFailedJob(t.Context(), client, 1, 5, time.Second)
	if err != nil {
		t.Fatalf("waitForFailedJob() error = %v, want nil", err)
	}
	if got != 71 {
		t.Errorf("waitForFailedJob() = %d, want the failed job 71", got)
	}
}

// TestWaitForFailedJob_ListingErrors_IsAStateToWaitThrough checks that an
// endpoint answering about a pipeline GitLab has not finished creating does
// not end the wait, which is what the comment in the poll says.
func TestWaitForFailedJob_ListingErrors_IsAStateToWaitThrough(t *testing.T) {
	fastFailedJobPolls(t)
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodGet, jobsPath,
		stubRefusal(http.StatusNotFound, "404 Not found"),
		stubOK([]any{map[string]any{"id": 80, "name": "failing-fixture", "status": failedJobStatus}}),
	)

	got, err := waitForFailedJob(t.Context(), client, 1, 5, time.Second)
	if err != nil {
		t.Fatalf("waitForFailedJob() error = %v, want the listing error waited through", err)
	}
	if got != 80 {
		t.Errorf("waitForFailedJob() = %d, want the failed job 80", got)
	}
}

// TestWaitForFailedJob_NeverFails_EndsWithTheStatusesItSaw checks that a
// budget that runs out reports what every job was doing, since "no failed
// job" on its own says nothing about why.
func TestWaitForFailedJob_NeverFails_EndsWithTheStatusesItSaw(t *testing.T) {
	fastFailedJobPolls(t)
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodGet, jobsPath,
		stubOK([]any{map[string]any{"id": 90, "name": "failing-fixture", "status": "pending"}}),
	)
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	got, err := waitForFailedJob(ctx, client, 1, 5, 300*time.Millisecond)
	if got != 0 {
		t.Errorf("waitForFailedJob() = %d, want 0 when no job failed", got)
	}
	if err == nil || !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, harness.ErrPollTimeout) {
		t.Errorf("waitForFailedJob() error = %v, want the wait to run out", err)
	}
}
