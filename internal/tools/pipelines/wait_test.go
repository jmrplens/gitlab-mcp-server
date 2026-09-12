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

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

const pathWaitPipeline = "/api/v4/projects/42/pipelines/10"

func useFastPipelinePollDuration(t *testing.T) {
	t.Helper()
	original := pollDuration
	pollDuration = func(seconds int) time.Duration { return time.Duration(seconds) * time.Millisecond }
	t.Cleanup(func() { pollDuration = original })
}

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
	useFastPipelinePollDuration(t)

	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathWaitPipeline {
			testutil.RespondJSON(w, http.StatusOK, pipelineJSON("running"))
			return
		}
		http.NotFound(w, r)
	}))

	// The budget has to outlast a poll, not merely a tick. This test is the
	// only one here that reaches the deadline on purpose, so it is the only
	// one that needs a status observed before the deadline arrives: the poll
	// runs under a context bounded by that same deadline, and a poll cut short
	// leaves FinalStatus empty, which is honest but is not what this asserts.
	//
	// It used to ask for 20ms, which is barely one tick of Windows' 15.6ms
	// timer, so the first loopback round-trip could not finish inside it and
	// the assertion failed there and nowhere else. 200ms is ~40 polls at the
	// interval below. Do not trim it back: the seconds are milliseconds here,
	// and IntervalSeconds is 5 rather than 1 because ClampPollInterval floors
	// anything below PollMinInterval to the ten-second default, so a 1 read as
	// 10ms rather than the 1ms it appears to say.
	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		PipelineID:      10,
		IntervalSeconds: 5,
		TimeoutSeconds:  200,
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
	useFastPipelinePollDuration(t)

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

	// 200 for the same reason TestWait_Timeout asks for it, and it is the whole
	// budget rather than a margin: the seconds here are milliseconds, so this is
	// 200ms for two loopback round-trips, and a CI runner under load spends more
	// than that on one. At 60 the test asserted that Wait polls twice while
	// giving it a budget a single poll could exhaust, and it failed exactly that
	// way (PollCount = 1, FinalStatus = ""). Do not trim it back.
	out, err := Wait(context.Background(), nil, client, WaitInput{
		ProjectID:       "42",
		PipelineID:      10,
		IntervalSeconds: 5,
		TimeoutSeconds:  200,
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
	useFastPipelinePollDuration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Millisecond)
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

// waitPipelineRows is the pipeline the wait fixtures poll, rendered under the
// wait's own H3. The detail is embedded without its heading so the response
// carries one H2, one guidance section and no card nested inside another.
const waitPipelineRows = "\n### Pipeline Details\n\n" +
	"- **IID**: 0\n" +
	"- **Tag**: ❌\n" +
	"- **URL**: [https://gitlab.example.com/-/pipelines/10](https://gitlab.example.com/-/pipelines/10)\n"

// TestFormatWaitMarkdown_Success checks the whole rendering of a wait that
// ended well: the wait's own rows, the pipeline under one H3, and no guidance
// section, since a pipeline that succeeded needs no next step.
func TestFormatWaitMarkdown_Success(t *testing.T) {
	want := "## ✅ Pipeline #10: success\n\n" +
		"- **Waited**: 30s\n" +
		"- **Polls**: 3\n" +
		"- **Final Status**: success\n" +
		waitPipelineRows
	got := FormatWaitMarkdown(WaitOutput{
		Pipeline:    DetailOutput{ID: 10, Status: "success", WebURL: "https://gitlab.example.com/-/pipelines/10"},
		WaitedFor:   "30s",
		PollCount:   3,
		FinalStatus: "success",
	})
	if got != want {
		t.Errorf("FormatWaitMarkdown(success)\n got %q\nwant %q", got, want)
	}
}

// TestFormatWaitMarkdown_Failed checks that a failed wait names the two
// actions that follow it, by the canonical IDs every surface resolves.
func TestFormatWaitMarkdown_Failed(t *testing.T) {
	want := "## ❌ Pipeline #10: failed\n\n" +
		"- **Waited**: 45s\n" +
		"- **Polls**: 5\n" +
		"- **Final Status**: failed\n" +
		waitPipelineRows +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'job.list' to find the jobs that failed\n" +
		"- Use action 'pipeline.retry' to retry the failed jobs\n"
	got := FormatWaitMarkdown(WaitOutput{
		Pipeline:    DetailOutput{ID: 10, Status: "failed", WebURL: "https://gitlab.example.com/-/pipelines/10"},
		WaitedFor:   "45s",
		PollCount:   5,
		FinalStatus: "failed",
	})
	if got != want {
		t.Errorf("FormatWaitMarkdown(failed)\n got %q\nwant %q", got, want)
	}
}

// TestFormatWaitMarkdown_TimedOut checks that a timeout is marked with the
// warning sign rather than a tick — a tick on "Timed Out" reads as success —
// and that the heading names the status the pipeline is still in.
func TestFormatWaitMarkdown_TimedOut(t *testing.T) {
	want := "## ⏰ Pipeline #10: Timed Out (current: running)\n\n" +
		"- **Waited**: 300s\n" +
		"- **Polls**: 30\n" +
		"- **Final Status**: running\n" +
		"- ⚠️ **Timed Out**\n" +
		waitPipelineRows +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'pipeline.wait' to keep waiting for this pipeline\n" +
		"- Use action 'pipeline.cancel' to abort it instead\n"
	got := FormatWaitMarkdown(WaitOutput{
		Pipeline:    DetailOutput{ID: 10, Status: "running", WebURL: "https://gitlab.example.com/-/pipelines/10"},
		WaitedFor:   "300s",
		PollCount:   30,
		FinalStatus: "running",
		TimedOut:    true,
	})
	if got != want {
		t.Errorf("FormatWaitMarkdown(timed out)\n got %q\nwant %q", got, want)
	}
}

// TestFormatWaitMarkdown_Canceled checks a wait that ended on a cancellation:
// the outcome is neither a success nor a failure, so it carries the stop glyph
// and no hints.
func TestFormatWaitMarkdown_Canceled(t *testing.T) {
	want := "## ⛔ Pipeline #10: canceled\n\n" +
		"- **Waited**: 15s\n" +
		"- **Polls**: 2\n" +
		"- **Final Status**: canceled\n" +
		waitPipelineRows
	got := FormatWaitMarkdown(WaitOutput{
		Pipeline:    DetailOutput{ID: 10, Status: "canceled", WebURL: "https://gitlab.example.com/-/pipelines/10"},
		WaitedFor:   "15s",
		PollCount:   2,
		FinalStatus: "canceled",
	})
	if got != want {
		t.Errorf("FormatWaitMarkdown(canceled)\n got %q\nwant %q", got, want)
	}
}

// TestFormatPipelineNotFound verifies not-found result formatting for pipelines.
func TestFormatPipelineNotFound(t *testing.T) {
	result := formatPipelineNotFound(pipelineNotFoundOutput{Identifier: "ID 10 in project 42"})
	if result == nil || !result.IsError {
		t.Fatalf("formatPipelineNotFound() = %+v, want error result", result)
	}
}

// TestFormatWaitResult verifies wait result formatting marks timeouts as errors.
func TestFormatWaitResult(t *testing.T) {
	result := formatWaitResult(WaitOutput{
		Pipeline:    DetailOutput{ID: 10, Status: "running", WebURL: "https://gitlab.example.com/-/pipelines/10"},
		WaitedFor:   "60s",
		PollCount:   6,
		FinalStatus: "running",
		TimedOut:    true,
	})
	if result == nil || !result.IsError {
		t.Fatalf("formatWaitResult() = %+v, want timeout error result", result)
	}
}
