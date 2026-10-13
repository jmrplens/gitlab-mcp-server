// wait_test.go contains unit tests for the pipeline Wait polling tool.
// Tests use httptest with staged responses to simulate pipeline state transitions.
package pipelines

import (
	"context"
	"net/http"
	"strconv"
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

// TestWait_FailedPipeline_NamesTheFailedJobs verifies that a wait answering a
// failed pipeline names the jobs that failed, with the stage and the
// failure_reason GitLab gives each, and the next step that reads a job's log
// (issue 1333).
//
// The status alone gave a caller nothing to act on. The jobs are asked for with
// scope failed, which the mock holds the request to, and a list GitLab cannot
// give leaves the status sentence on its own, since the wait already knows the
// outcome it was asked for, and so does a list that names no failed job. A
// canceled pipeline asks for nothing: no job of it failed. A page with a next
// one says there are more and how to list them, and a job GitLab gives no
// failure_reason is named without one.
func TestWait_FailedPipeline_NamesTheFailedJobs(t *testing.T) {
	const failedJobs = `[` +
		`{"id":101,"name":"unit","stage":"test","status":"failed","failure_reason":"script_failure"},` +
		`{"id":102,"name":"deploy","stage":"deploy","status":"failed","failure_reason":""}` +
		`]`
	tests := []struct {
		name       string
		status     string
		jobsStatus int
		nextPage   string
		jobs       string
		want       []string
		notWant    []string
		wantJobs   bool
	}{
		{
			name:       "the failed jobs and the next step",
			status:     "failed",
			jobsStatus: http.StatusOK,
			want: []string{
				`pipelineWait: pipeline #10 finished with status "failed". Failed jobs: ` +
					`unit (job 101, stage test, failure_reason script_failure); deploy (job 102, stage deploy). ` +
					`Read a failed job's log with job.trace, passing its job_id`,
			},
			notWant:  []string{"and more"},
			wantJobs: true,
		},
		{
			name:       "more failed jobs than one page",
			status:     "failed",
			jobsStatus: http.StatusOK,
			nextPage:   "2",
			want:       []string{"deploy (job 102, stage deploy), and more: list them with job.list and scope failed. "},
			wantJobs:   true,
		},
		{
			name:       "a list GitLab does not give",
			status:     "failed",
			jobsStatus: http.StatusForbidden,
			want:       []string{`pipelineWait: pipeline #10 finished with status "failed"`},
			notWant:    []string{"Failed jobs", "job.trace"},
			wantJobs:   true,
		},
		{
			name:       "a list that names no failed job",
			status:     "failed",
			jobsStatus: http.StatusOK,
			jobs:       `[]`,
			want:       []string{`pipelineWait: pipeline #10 finished with status "failed"`},
			notWant:    []string{"Failed jobs", "job.trace"},
			wantJobs:   true,
		},
		{
			name:     "a canceled pipeline",
			status:   "canceled",
			want:     []string{`pipelineWait: pipeline #10 finished with status "canceled"`},
			notWant:  []string{"Failed jobs", "job.trace"},
			wantJobs: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var jobsAsked atomic.Bool
			jobs := tt.jobs
			if jobs == "" {
				jobs = failedJobs
			}
			client := testutil.NewTestClient(t, failedJobsServer(t, failedJobsAnswer{
				status: tt.status, jobsStatus: tt.jobsStatus, nextPage: tt.nextPage, jobs: jobs,
			}, &jobsAsked))

			_, err := Wait(t.Context(), nil, client, WaitInput{ProjectID: "42", PipelineID: 10, IntervalSeconds: 5, TimeoutSeconds: 30})
			if err == nil {
				t.Fatal("Wait() error = nil, want the pipeline's failure")
			}
			assertErrorText(t, err, tt.want, tt.notWant)
			if jobsAsked.Load() != tt.wantJobs {
				t.Errorf("failed jobs asked = %t, want %t", jobsAsked.Load(), tt.wantJobs)
			}
		})
	}
}

// failedJobsAnswer is what failedJobsServer answers: the pipeline's status,
// and the status, next page and body of its failed jobs list.
type failedJobsAnswer struct {
	status     string
	jobsStatus int
	nextPage   string
	jobs       string
}

// failedJobsServer answers the pipeline read with answer's status and the
// failed jobs list as answer says, recording in asked that the list was read
// and holding its request to scope failed.
func failedJobsServer(t *testing.T, answer failedJobsAnswer, asked *atomic.Bool) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pathWaitPipeline:
			testutil.RespondJSON(w, http.StatusOK, pipelineJSON(answer.status))
		case pathWaitPipeline + "/jobs":
			asked.Store(true)
			if got := r.URL.Query()["scope[]"]; len(got) != 1 || got[0] != "failed" {
				t.Errorf("jobs scope[] = %v, want [failed]", got)
			}
			if answer.nextPage != "" {
				w.Header().Set("X-Next-Page", answer.nextPage)
			}
			if answer.jobsStatus != http.StatusOK {
				testutil.RespondJSON(w, answer.jobsStatus, `{"message":"403 Forbidden"}`)
				return
			}
			testutil.RespondJSON(w, http.StatusOK, answer.jobs)
		default:
			http.NotFound(w, r)
		}
	})
}

// assertErrorText holds err's text to containing every one of want and none
// of notWant.
func assertErrorText(t *testing.T, err error, want, notWant []string) {
	t.Helper()
	for _, fragment := range want {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("error = %q, want it to contain %q", err, fragment)
		}
	}
	for _, fragment := range notWant {
		if strings.Contains(err.Error(), fragment) {
			t.Errorf("error = %q, must not contain %q", err, fragment)
		}
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

// TestWait_EmptyProjectID verifies that Wait refuses a call with no project,
// and refuses it here rather than letting GitLab answer: the mock fails the
// test if any request arrives, so the refusal is the handler's own.
func TestWait_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

	_, err := Wait(context.Background(), nil, client, WaitInput{
		PipelineID: 10,
	})
	if err == nil {
		t.Fatal("Wait() expected error for empty project_id, got nil")
	}
	if !strings.Contains(err.Error(), "project_id") {
		t.Errorf("Wait() error = %q, want it to name project_id", err)
	}
}

// TestWait_InvalidPipelineID verifies that Wait refuses every pipeline_id at
// or below zero, itself. Zero is the one a missing argument decodes to and so
// the one a model reaches by accident, and it is the value the guard's own
// boundary turns on: against a mock that answers anything, a test asserting
// only that some error came back passes on GitLab's 404 while the guard lets
// zero through.
func TestWait_InvalidPipelineID(t *testing.T) {
	for _, id := range []int64{0, -1} {
		t.Run(strconv.FormatInt(id, 10), func(t *testing.T) {
			client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

			_, err := Wait(context.Background(), nil, client, WaitInput{
				ProjectID:  "42",
				PipelineID: id,
			})
			if err == nil {
				t.Fatalf("Wait(pipeline_id=%d) expected an error, got nil", id)
			}
			if !strings.Contains(err.Error(), "pipeline_id") {
				t.Errorf("Wait(pipeline_id=%d) error = %q, want it to name pipeline_id", id, err)
			}
		})
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

// TestFormatWaitResult verifies wait result formatting marks a timeout as an
// error and a wait that reached a terminal status as none. Both halves matter:
// a result always flagged IsError tells a client every wait failed, and one
// never flagged tells it a wait that ran out of time succeeded.
func TestFormatWaitResult(t *testing.T) {
	cases := []struct {
		name     string
		out      WaitOutput
		wantErrs bool
	}{
		{
			name: "timed out",
			out: WaitOutput{
				Pipeline:    DetailOutput{ID: 10, Status: "running", WebURL: "https://gitlab.example.com/-/pipelines/10"},
				WaitedFor:   "60s",
				PollCount:   6,
				FinalStatus: "running",
				TimedOut:    true,
			},
			wantErrs: true,
		},
		{
			name: "reached a terminal status",
			out: WaitOutput{
				Pipeline:    DetailOutput{ID: 10, Status: statusSuccess, WebURL: "https://gitlab.example.com/-/pipelines/10"},
				WaitedFor:   "30s",
				PollCount:   3,
				FinalStatus: statusSuccess,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := formatWaitResult(tc.out)
			if result == nil {
				t.Fatal("formatWaitResult() = nil")
			}
			if result.IsError != tc.wantErrs {
				t.Errorf("formatWaitResult().IsError = %v, want %v", result.IsError, tc.wantErrs)
			}
		})
	}
}
