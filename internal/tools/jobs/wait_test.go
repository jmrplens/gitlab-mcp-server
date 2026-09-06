// wait_test.go contains unit tests for the job Wait polling tool.
// Tests use httptest with staged responses to simulate job state transitions.
package jobs

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

const pathWaitJob = "/api/v4/projects/42/jobs/100"

// useFastJobPollDuration scales every polling duration from seconds to
// milliseconds, the timeout included, so a twenty-second wait takes twenty
// milliseconds.
//
// Only a test about the timeout itself wants this. It makes the whole wait
// shorter than one round trip to the httptest server on a loaded machine, so
// the first poll can be cut off by the deadline before it answers and Wait can
// time out having read nothing at all.
func useFastJobPollDuration(t *testing.T) {
	t.Helper()
	original := pollDuration
	pollDuration = func(seconds int) time.Duration { return time.Duration(seconds) * time.Millisecond }
	t.Cleanup(func() { pollDuration = original })
}

// useFastJobPollInterval scales only the polling interval to milliseconds and
// leaves every other duration, the timeout included, in real seconds. A test
// about polling is then never ended by a clock it is not about, however slow
// the machine running it.
//
// The interval is passed in rather than read back because pollDuration is
// handed the value after [toolutil.ClampPollInterval] has applied the 5..60
// bounds, so a caller asking for 1 would see 10 here.
func useFastJobPollInterval(t *testing.T, intervalSeconds int) {
	t.Helper()
	original := pollDuration
	pollDuration = func(seconds int) time.Duration {
		if seconds == intervalSeconds {
			return time.Millisecond
		}
		return original(seconds)
	}
	t.Cleanup(func() { pollDuration = original })
}

func jobWithStatus(status string) string {
	return `{
		"id":100,"name":"build","stage":"build","status":"` + status + `",
		"ref":"main","tag":false,"allow_failure":false,
		"duration":45.5,"queued_duration":2.1,
		"web_url":"https://gitlab.example.com/-/jobs/100",
		"pipeline":{"id":10},
		"created_at":"2026-03-01T10:00:00Z",
		"started_at":"2026-03-01T10:00:05Z",
		"finished_at":"2026-03-01T10:00:50Z",
		"user":{"username":"testuser"},
		"runner":{"id":1}
	}`
}

// TestJobWait_ImmediateSuccess verifies that Wait returns immediately
// when the job is already in a terminal state on the first poll.
func TestJobWait_ImmediateSuccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitJob {
			testutil.RespondJSON(w, http.StatusOK, jobWithStatus("success"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		JobID:           100,
		IntervalSeconds: 5,
		TimeoutSeconds:  30,
	})
	if err != nil {
		t.Fatalf("Wait() unexpected error: %v", err)
	}
	if out.FinalStatus != "success" {
		t.Errorf("FinalStatus = %q, want %q", out.FinalStatus, "success")
	}
	if out.PollCount != 1 {
		t.Errorf("PollCount = %d, want 1", out.PollCount)
	}
	if out.TimedOut {
		t.Error("TimedOut should be false")
	}
	if out.Job.ID != 100 {
		t.Errorf("Job.ID = %d, want 100", out.Job.ID)
	}
}

// TestJobWait_FailedJob_FailOnError verifies that Wait returns an error
// when the job finishes with "failed" status and fail_on_error is true (default).
func TestJobWait_FailedJob_FailOnError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitJob {
			testutil.RespondJSON(w, http.StatusOK, jobWithStatus("failed"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		JobID:           100,
		IntervalSeconds: 5,
		TimeoutSeconds:  30,
	})
	if err == nil {
		t.Fatal("Wait() expected error for failed job, got nil")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("error = %q, want to contain 'failed'", err.Error())
	}
	if out.FinalStatus != "failed" {
		t.Errorf("FinalStatus = %q, want %q", out.FinalStatus, "failed")
	}
}

// TestJobWait_FailedJob_NoFailOnError verifies that Wait returns normally
// (no error) when fail_on_error is false even if the job failed.
func TestJobWait_FailedJob_NoFailOnError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitJob {
			testutil.RespondJSON(w, http.StatusOK, jobWithStatus("failed"))
			return
		}
		http.NotFound(w, r)
	}))

	failOnError := false
	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		JobID:           100,
		IntervalSeconds: 5,
		TimeoutSeconds:  30,
		FailOnError:     &failOnError,
	})
	if err != nil {
		t.Fatalf("Wait() unexpected error with fail_on_error=false: %v", err)
	}
	if out.FinalStatus != "failed" {
		t.Errorf("FinalStatus = %q, want %q", out.FinalStatus, "failed")
	}
}

// TestJobWait_Timeout verifies that Wait returns TimedOut=true when the
// job stays in a running state and the timeout expires.
//
// It is about the timeout and nothing else. The scaled clock makes the whole
// wait twenty milliseconds, which on a loaded runner is shorter than one round
// trip to the httptest server: the deadline then cuts the first poll off before
// it answers and Wait times out having read no status, so FinalStatus is empty.
// That is the documented behavior, pinned without any I/O by waitpoll's
// TestPoll_PollReceivesTimeoutContext, and asserting "running" unconditionally
// here is what went red on slow Windows runners (issue 548).
//
// PollCount is what makes the status assertion exact rather than merely
// weakened. The loop increments it before each poll and only reaches a second
// iteration once the first poll has returned a non-terminal status, so a count
// of two or more means a status was really read, and Wait must report it. A
// count the fake kept of the requests it answered would be weaker: the server
// can answer a request whose response the deadline stops the client from
// reading.
func TestJobWait_Timeout(t *testing.T) {
	useFastJobPollDuration(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitJob {
			testutil.RespondJSON(w, http.StatusOK, jobWithStatus("running"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		JobID:           100,
		IntervalSeconds: 1,
		TimeoutSeconds:  20,
	})
	if err != nil {
		t.Fatalf("Wait() unexpected error: %v", err)
	}
	if !out.TimedOut {
		t.Error("TimedOut should be true")
	}
	if out.FinalStatus != "" && out.FinalStatus != "running" {
		t.Errorf("FinalStatus = %q, want %q or empty", out.FinalStatus, "running")
	}
	if out.PollCount >= 2 && out.FinalStatus != "running" {
		t.Errorf("FinalStatus = %q after %d polls, want %q once a poll has answered",
			out.FinalStatus, out.PollCount, "running")
	}
}

// TestJobWait_PollingTransition verifies that Wait polls multiple times
// before the job transitions from running to success.
//
// Only the interval is scaled. Scaling the timeout as well left the whole
// transition sixty milliseconds in which to make two round trips to the
// httptest server, which is the same clock this file's timeout test is about
// (issue 548), and losing that race here would have failed on a status Wait
// never got to read rather than on the polling this test is for.
func TestJobWait_PollingTransition(t *testing.T) {
	useFastJobPollInterval(t, 5)

	var callCount atomic.Int32
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitJob {
			n := callCount.Add(1)
			if n >= 2 {
				testutil.RespondJSON(w, http.StatusOK, jobWithStatus("success"))
			} else {
				testutil.RespondJSON(w, http.StatusOK, jobWithStatus("running"))
			}
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		JobID:           100,
		IntervalSeconds: 5,
		TimeoutSeconds:  60,
	})
	if err != nil {
		t.Fatalf("Wait() unexpected error: %v", err)
	}
	if out.FinalStatus != "success" {
		t.Errorf("FinalStatus = %q, want %q", out.FinalStatus, "success")
	}
	if out.PollCount < 2 {
		t.Errorf("PollCount = %d, want >= 2", out.PollCount)
	}
}

// TestJobWait_CanceledContext verifies that Wait respects context cancellation.
func TestJobWait_CanceledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, jobWithStatus("running"))
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Wait(ctx, nil, client, WaitInput{
		ProjectID:       "42",
		JobID:           100,
		IntervalSeconds: 5,
		TimeoutSeconds:  60,
	})
	if err == nil {
		t.Fatal("Wait() expected error for canceled context, got nil")
	}
}

// TestJobWait_EmptyProjectID verifies that Wait returns an error for empty project_id.
func TestJobWait_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Wait(context.Background(), nil, client, WaitInput{
		JobID: 100,
	})
	if err == nil {
		t.Fatal("Wait() expected error for empty project_id, got nil")
	}
}

// TestJobWait_InvalidJobID verifies that Wait returns an error for job_id <= 0.
func TestJobWait_InvalidJobID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID: "42",
		JobID:     0,
	})
	if err == nil {
		t.Fatal("Wait() expected error for invalid job_id, got nil")
	}
}

// TestJobWait_APIError verifies that Wait wraps GitLab API errors correctly.
func TestJobWait_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		JobID:           100,
		IntervalSeconds: 5,
		TimeoutSeconds:  10,
	})
	if err == nil {
		t.Fatal("Wait() expected error for API failure, got nil")
	}
}

// TestJobWait_CanceledJob verifies that Wait returns an error for canceled jobs
// when fail_on_error is true (default).
func TestJobWait_CanceledJob(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitJob {
			testutil.RespondJSON(w, http.StatusOK, jobWithStatus("canceled"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		JobID:           100,
		IntervalSeconds: 5,
		TimeoutSeconds:  30,
	})
	if err == nil {
		t.Fatal("Wait() expected error for canceled job, got nil")
	}
	if out.FinalStatus != "canceled" {
		t.Errorf("FinalStatus = %q, want %q", out.FinalStatus, "canceled")
	}
}

// TestJobWait_SkippedJob verifies that Wait returns successfully for skipped jobs.
func TestJobWait_SkippedJob(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitJob {
			testutil.RespondJSON(w, http.StatusOK, jobWithStatus("skipped"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		JobID:           100,
		IntervalSeconds: 5,
		TimeoutSeconds:  30,
	})
	if err != nil {
		t.Fatalf("Wait() unexpected error: %v", err)
	}
	if out.FinalStatus != "skipped" {
		t.Errorf("FinalStatus = %q, want %q", out.FinalStatus, "skipped")
	}
}

// TestJobWait_ManualJob verifies that Wait returns successfully for jobs
// with "manual" terminal status.
func TestJobWait_ManualJob(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitJob {
			testutil.RespondJSON(w, http.StatusOK, jobWithStatus("manual"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		JobID:           100,
		IntervalSeconds: 5,
		TimeoutSeconds:  30,
	})
	if err != nil {
		t.Fatalf("Wait() unexpected error: %v", err)
	}
	if out.FinalStatus != "manual" {
		t.Errorf("FinalStatus = %q, want %q", out.FinalStatus, "manual")
	}
}

// TestJobWait_ContextCanceledDuringPoll verifies that Wait respects context
// cancellation that occurs during the polling loop (not before entry).
// Uses a short-lived context that expires after the first poll but before
// the ticker (5 s min), ensuring the select picks ctx.Done deterministically.
//
// Only the interval is scaled, so the caller's two-millisecond context is the
// only clock that can end this wait. With the timeout scaled too it was three
// hundred milliseconds away, near enough that a stalled machine could time out
// instead of canceling and return no error at all.
func TestJobWait_ContextCanceledDuringPoll(t *testing.T) {
	useFastJobPollInterval(t, 5)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Millisecond)
	defer cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitJob {
			testutil.RespondJSON(w, http.StatusOK, jobWithStatus("running"))
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Wait(ctx, nil, client, WaitInput{
		ProjectID:       "42",
		JobID:           100,
		IntervalSeconds: 5,
		TimeoutSeconds:  300,
	})
	if err == nil {
		t.Fatal("Wait() expected error for context canceled during polling, got nil")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("expected context deadline exceeded, got: %v", err)
	}
}

// TestFormatWaitMarkdown_Success verifies markdown rendering for a successfully completed job.
func TestFormatWaitMarkdown_Success(t *testing.T) {
	md := FormatWaitMarkdown(WaitOutput{
		Job:         Output{ID: 100, Name: "build", Status: "success", WebURL: "https://gitlab.example.com/-/jobs/100"},
		WaitedFor:   "30s",
		PollCount:   3,
		FinalStatus: "success",
	})
	if !strings.Contains(md, "Job #100") {
		t.Error("expected 'Job #100' in markdown")
	}
	if !strings.Contains(md, "success") {
		t.Error("expected 'success' in markdown")
	}
	if !strings.Contains(md, "30s") {
		t.Error("expected waited duration in markdown")
	}
	if !strings.Contains(md, "3 polls") {
		t.Error("expected poll count in markdown")
	}
	if strings.Contains(md, "Timed Out") {
		t.Error("should not contain 'Timed Out' for success")
	}
}

// TestFormatWaitMarkdown_Failed verifies markdown rendering for a failed job with hints.
func TestFormatWaitMarkdown_Failed(t *testing.T) {
	md := FormatWaitMarkdown(WaitOutput{
		Job:         Output{ID: 100, Name: "build", Status: "failed", WebURL: "https://gitlab.example.com/-/jobs/100"},
		WaitedFor:   "45s",
		PollCount:   5,
		FinalStatus: "failed",
	})
	if !strings.Contains(md, "Job #100") {
		t.Error("expected 'Job #100' in markdown")
	}
	if !strings.Contains(md, "failed") {
		t.Error("expected 'failed' in markdown")
	}
	if !strings.Contains(md, "gitlab_job") {
		t.Error("expected hint about job trace in markdown for failed job")
	}
}

// TestFormatWaitMarkdown_TimedOut verifies markdown rendering for a timed-out job wait.
func TestFormatWaitMarkdown_TimedOut(t *testing.T) {
	md := FormatWaitMarkdown(WaitOutput{
		Job:         Output{ID: 100, Name: "build", Status: "running", WebURL: "https://gitlab.example.com/-/jobs/100"},
		WaitedFor:   "300s",
		PollCount:   30,
		FinalStatus: "running",
		TimedOut:    true,
	})
	if !strings.Contains(md, "Timed Out") {
		t.Error("expected 'Timed Out' in markdown")
	}
	if !strings.Contains(md, "Job #100") {
		t.Error("expected 'Job #100' in markdown")
	}
	if !strings.Contains(md, "gitlab_job_wait") {
		t.Error("expected hint about calling wait again")
	}
	if !strings.Contains(md, "gitlab_job_cancel") {
		t.Error("expected hint about cancel")
	}
}

// TestFormatWaitMarkdown_Canceled verifies markdown rendering for a canceled job.
func TestFormatWaitMarkdown_Canceled(t *testing.T) {
	md := FormatWaitMarkdown(WaitOutput{
		Job:         Output{ID: 100, Name: "build", Status: "canceled", WebURL: "https://gitlab.example.com/-/jobs/100"},
		WaitedFor:   "15s",
		PollCount:   2,
		FinalStatus: "canceled",
	})
	if !strings.Contains(md, "Job #100") {
		t.Error("expected 'Job #100' in markdown")
	}
	if !strings.Contains(md, "canceled") {
		t.Error("expected 'canceled' in markdown")
	}
}
