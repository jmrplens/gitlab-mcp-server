//go:build e2e

// failed_job_test.go drives the failed job wait against the stub: a listing
// that carries only running jobs at first, a listing endpoint that errors
// while GitLab is still creating the pipeline, and a budget that runs out.
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

// TestWaitForFailedJob_FailsLater_ReturnsTheJobID checks that the wait sits
// through a listing of running jobs and answers with the one that failed,
// leaving the manual job beside it alone.
func TestWaitForFailedJob_FailsLater_ReturnsTheJobID(t *testing.T) {
	fastFailedJobPolls(t)
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodGet, jobsPath,
		stubOK([]any{map[string]any{"id": 71, "name": "failing-fixture", "status": "running"}}),
		stubOK([]any{
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
