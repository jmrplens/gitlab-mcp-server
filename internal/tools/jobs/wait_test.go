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

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const pathWaitJob = "/api/v4/projects/42/jobs/100"

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
func TestJobWait_Timeout(t *testing.T) {
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
		IntervalSeconds: 5,
		TimeoutSeconds:  1,
	})
	if err != nil {
		t.Fatalf("Wait() unexpected error: %v", err)
	}
	if !out.TimedOut {
		t.Error("TimedOut should be true")
	}
	if out.FinalStatus != "running" {
		t.Errorf("FinalStatus = %q, want %q", out.FinalStatus, "running")
	}
}

// TestJobWait_PollingTransition verifies that Wait polls multiple times
// before the job transitions from running to success.
func TestJobWait_PollingTransition(t *testing.T) {
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

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

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
func TestJobWait_ContextCanceledDuringPoll(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
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

// TestJobClampInterval verifies interval clamping to valid range.
func TestJobClampInterval(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"below min returns default", 2, defaultInterval},
		{"zero returns default", 0, defaultInterval},
		{"above max returns max", 100, maxInterval},
		{"in range returns value", 15, 15},
		{"at min returns min", minInterval, minInterval},
		{"at max returns max", maxInterval, maxInterval},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampInterval(tt.in)
			if got != tt.want {
				t.Errorf("clampInterval(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// TestJobClampTimeout verifies timeout clamping to valid range.
func TestJobClampTimeout(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"below min returns default", 0, defaultTimeout},
		{"above max returns max", 5000, maxTimeout},
		{"in range returns value", 120, 120},
		{"at min returns min", minTimeout, minTimeout},
		{"at max returns max", maxTimeout, maxTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampTimeout(tt.in)
			if got != tt.want {
				t.Errorf("clampTimeout(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}
