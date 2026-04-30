// wait_test.go contains unit tests for the pipeline Wait polling tool.
// Tests use httptest with staged responses to simulate pipeline state transitions.
package pipelines

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const pathWaitPipeline = "/api/v4/projects/42/pipelines/10"

func pipelineJSON(status string) string {
	return `{
		"id":10,"iid":10,"project_id":42,"status":"` + status + `","source":"push",
		"ref":"main","sha":"abc123","before_sha":"def456","name":"Build","tag":false,
		"duration":120,"queued_duration":5,
		"web_url":"https://gitlab.example.com/-/pipelines/10",
		"created_at":"2026-03-01T10:00:00Z","updated_at":"2026-03-01T10:02:00Z",
		"user":{"username":"testuser"}
	}`
}

// TestWait_ImmediateSuccess verifies that Wait returns immediately
// when the pipeline is already in a terminal state on the first poll.
func TestWait_ImmediateSuccess(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitPipeline {
			testutil.RespondJSON(w, http.StatusOK, pipelineJSON("success"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		PipelineID:      10,
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
	if out.Pipeline.ID != 10 {
		t.Errorf("Pipeline.ID = %d, want 10", out.Pipeline.ID)
	}
}

// TestWait_FailedPipeline_FailOnError verifies that Wait returns an error
// when the pipeline finishes with "failed" status and fail_on_error is true.
func TestWait_FailedPipeline_FailOnError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitPipeline {
			testutil.RespondJSON(w, http.StatusOK, pipelineJSON("failed"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		PipelineID:      10,
		IntervalSeconds: 5,
		TimeoutSeconds:  30,
	})
	if err == nil {
		t.Fatal("Wait() expected error for failed pipeline, got nil")
	}
	if out.FinalStatus != "failed" {
		t.Errorf("FinalStatus = %q, want %q", out.FinalStatus, "failed")
	}
}

// TestWait_FailedPipeline_NoFailOnError verifies that Wait returns normally
// (no error) when fail_on_error is false even if the pipeline failed.
func TestWait_FailedPipeline_NoFailOnError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitPipeline {
			testutil.RespondJSON(w, http.StatusOK, pipelineJSON("failed"))
			return
		}
		http.NotFound(w, r)
	}))

	failOnError := false
	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		PipelineID:      10,
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

// TestWait_Timeout verifies that Wait returns TimedOut=true when the
// pipeline stays in a running state and the timeout expires.
func TestWait_Timeout(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitPipeline {
			testutil.RespondJSON(w, http.StatusOK, pipelineJSON("running"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		PipelineID:      10,
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

// TestWait_PollingTransition verifies that Wait polls multiple times
// before the pipeline transitions from running to success.
func TestWait_PollingTransition(t *testing.T) {
	var callCount atomic.Int32
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitPipeline {
			n := callCount.Add(1)
			if n >= 2 {
				testutil.RespondJSON(w, http.StatusOK, pipelineJSON("success"))
			} else {
				testutil.RespondJSON(w, http.StatusOK, pipelineJSON("running"))
			}
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		PipelineID:      10,
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

// TestWait_CanceledContext verifies that Wait respects context cancellation.
func TestWait_CanceledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pipelineJSON("running"))
	}))

	ctx := testutil.CancelledCtx(t)

	_, err := Wait(ctx, nil, client, WaitInput{
		ProjectID:       "42",
		PipelineID:      10,
		IntervalSeconds: 5,
		TimeoutSeconds:  60,
	})
	if err == nil {
		t.Fatal("Wait() expected error for canceled context, got nil")
	}
}

// TestWait_EmptyProjectID verifies that Wait returns an error for empty project_id.
func TestWait_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Wait(context.Background(), nil, client, WaitInput{
		PipelineID: 10,
	})
	if err == nil {
		t.Fatal("Wait() expected error for empty project_id, got nil")
	}
}

// TestWait_InvalidPipelineID verifies that Wait returns an error for pipeline_id <= 0.
func TestWait_InvalidPipelineID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:  "42",
		PipelineID: 0,
	})
	if err == nil {
		t.Fatal("Wait() expected error for invalid pipeline_id, got nil")
	}
}

// TestWait_APIError verifies that Wait wraps GitLab API errors correctly.
func TestWait_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		PipelineID:      10,
		IntervalSeconds: 5,
		TimeoutSeconds:  10,
	})
	if err == nil {
		t.Fatal("Wait() expected error for API failure, got nil")
	}
}

// TestWait_CanceledPipeline verifies that Wait returns an error for canceled pipelines
// when fail_on_error is true (default).
func TestWait_CanceledPipeline(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitPipeline {
			testutil.RespondJSON(w, http.StatusOK, pipelineJSON("canceled"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		PipelineID:      10,
		IntervalSeconds: 5,
		TimeoutSeconds:  30,
	})
	if err == nil {
		t.Fatal("Wait() expected error for canceled pipeline, got nil")
	}
	if out.FinalStatus != "canceled" {
		t.Errorf("FinalStatus = %q, want %q", out.FinalStatus, "canceled")
	}
}

// TestWait_SkippedPipeline verifies that Wait returns successfully for skipped pipelines.
func TestWait_SkippedPipeline(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitPipeline {
			testutil.RespondJSON(w, http.StatusOK, pipelineJSON("skipped"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		PipelineID:      10,
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

// TestWait_ManualPipeline verifies that Wait returns successfully for pipelines
// with "manual" terminal status (a pipeline with only manual jobs).
func TestWait_ManualPipeline(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitPipeline {
			testutil.RespondJSON(w, http.StatusOK, pipelineJSON("manual"))
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		PipelineID:      10,
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

// TestWait_ContextCanceledDuringPoll verifies that Wait respects context
// cancellation that occurs during the polling loop (not before entry).
// Uses a short-lived context that expires after the first poll but before
// the ticker (5 s min), ensuring the select picks ctx.Done deterministically.
func TestWait_ContextCanceledDuringPoll(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitPipeline {
			testutil.RespondJSON(w, http.StatusOK, pipelineJSON("running"))
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Wait(ctx, nil, client, WaitInput{
		ProjectID:       "42",
		PipelineID:      10,
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

// TestFormatWaitMarkdown_Success verifies markdown rendering for a successfully completed pipeline.
func TestFormatWaitMarkdown_Success(t *testing.T) {
	md := FormatWaitMarkdown(WaitOutput{
		Pipeline:    DetailOutput{ID: 10, Status: "success", WebURL: "https://gitlab.example.com/-/pipelines/10"},
		WaitedFor:   "30s",
		PollCount:   3,
		FinalStatus: "success",
	})
	if !strings.Contains(md, "Pipeline #10") {
		t.Error("expected 'Pipeline #10' in markdown")
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

// TestFormatWaitMarkdown_Failed verifies markdown rendering for a failed pipeline with hints.
func TestFormatWaitMarkdown_Failed(t *testing.T) {
	md := FormatWaitMarkdown(WaitOutput{
		Pipeline:    DetailOutput{ID: 10, Status: "failed", WebURL: "https://gitlab.example.com/-/pipelines/10"},
		WaitedFor:   "45s",
		PollCount:   5,
		FinalStatus: "failed",
	})
	if !strings.Contains(md, "Pipeline #10") {
		t.Error("expected 'Pipeline #10' in markdown")
	}
	if !strings.Contains(md, "failed") {
		t.Error("expected 'failed' in markdown")
	}
	if !strings.Contains(md, "gitlab_job") {
		t.Error("expected hint about jobs in markdown for failed pipeline")
	}
	if !strings.Contains(md, "gitlab_pipeline_retry") {
		t.Error("expected hint about retry in markdown for failed pipeline")
	}
}

// TestFormatWaitMarkdown_TimedOut verifies markdown rendering for a timed-out wait.
func TestFormatWaitMarkdown_TimedOut(t *testing.T) {
	md := FormatWaitMarkdown(WaitOutput{
		Pipeline:    DetailOutput{ID: 10, Status: "running", WebURL: "https://gitlab.example.com/-/pipelines/10"},
		WaitedFor:   "300s",
		PollCount:   30,
		FinalStatus: "running",
		TimedOut:    true,
	})
	if !strings.Contains(md, "Timed Out") {
		t.Error("expected 'Timed Out' in markdown")
	}
	if !strings.Contains(md, "Pipeline #10") {
		t.Error("expected 'Pipeline #10' in markdown")
	}
	if !strings.Contains(md, "gitlab_pipeline_wait") {
		t.Error("expected hint about calling wait again")
	}
	if !strings.Contains(md, "gitlab_pipeline_cancel") {
		t.Error("expected hint about cancel")
	}
}

// TestFormatWaitMarkdown_Canceled verifies markdown rendering for a canceled pipeline.
func TestFormatWaitMarkdown_Canceled(t *testing.T) {
	md := FormatWaitMarkdown(WaitOutput{
		Pipeline:    DetailOutput{ID: 10, Status: "canceled", WebURL: "https://gitlab.example.com/-/pipelines/10"},
		WaitedFor:   "15s",
		PollCount:   2,
		FinalStatus: "canceled",
	})
	if !strings.Contains(md, "Pipeline #10") {
		t.Error("expected 'Pipeline #10' in markdown")
	}
	if !strings.Contains(md, "canceled") {
		t.Error("expected 'canceled' in markdown")
	}
}
