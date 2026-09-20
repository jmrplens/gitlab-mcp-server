package waitpoll

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type pollItem struct {
	Status string
	Value  int
}

func fastDuration(int) time.Duration { return time.Millisecond }

func pollOptions(statuses ...string) (Options[pollItem], *int) {
	attempts := 0
	opts := Options[pollItem]{
		IntervalSeconds: 1,
		TimeoutSeconds:  1,
		PollDuration:    fastDuration,
		ProgressMessage: func(attempt int) string { return "attempt" },
		Poll: func(context.Context) (pollItem, error) {
			status := statuses[min(attempts, len(statuses)-1)]
			attempts++
			return pollItem{Status: status, Value: attempts}, nil
		},
		Status:       func(item pollItem) string { return item.Status },
		FailureError: func(item pollItem) error { return errors.New("terminal " + item.Status) },
	}
	return opts, &attempts
}

// TestPoll_TerminalSuccessDefaultDuration verifies immediate terminal success
// and the default second-based duration path.
func TestPoll_TerminalSuccessDefaultDuration(t *testing.T) {
	opts, _ := pollOptions("success")
	opts.PollDuration = nil

	result, err := Poll(context.Background(), opts)
	if err != nil {
		t.Fatalf("Poll() unexpected error: %v", err)
	}
	if result.FinalStatus != "success" || result.PollCount != 1 || result.TimedOut {
		t.Fatalf("result = %#v, want success on first poll without timeout", result)
	}
}

// TestPoll_TerminalFailureReturnsPartialResult verifies failed terminal states
// return both the partial result and the configured error.
func TestPoll_TerminalFailureReturnsPartialResult(t *testing.T) {
	opts, _ := pollOptions("failed")

	result, err := Poll(context.Background(), opts)
	if err == nil {
		t.Fatal("Poll() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "terminal failed") {
		t.Fatalf("error = %q, want terminal failed", err.Error())
	}
	if result.FinalStatus != "failed" || result.Item.Status != "failed" {
		t.Fatalf("result = %#v, want failed partial result", result)
	}
}

// TestPoll_TerminalFailureAllowed verifies fail_on_error=false returns normally
// for failed or canceled terminal states.
func TestPoll_TerminalFailureAllowed(t *testing.T) {
	failOnError := false
	opts, _ := pollOptions("canceled")
	opts.FailOnError = &failOnError

	result, err := Poll(context.Background(), opts)
	if err != nil {
		t.Fatalf("Poll() unexpected error: %v", err)
	}
	if result.FinalStatus != "canceled" {
		t.Fatalf("FinalStatus = %q, want canceled", result.FinalStatus)
	}
}

// TestPoll_TerminalCanceledFailsWhenFailOnErrorIsOn verifies that a canceled
// terminal status, and not only a failed one, is reported as an error under the
// default fail_on_error.
//
// The two statuses share one branch, and a suite that only ever drove "failed"
// through it leaves "canceled" asserted by nothing: the arm could be dropped and
// a canceled pipeline would come back as a wait that succeeded, which is the one
// answer a caller must not get about a job that never ran.
func TestPoll_TerminalCanceledFailsWhenFailOnErrorIsOn(t *testing.T) {
	opts, _ := pollOptions("canceled")

	result, err := Poll(context.Background(), opts)
	if err == nil {
		t.Fatal("Poll() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "terminal canceled") {
		t.Fatalf("error = %q, want terminal canceled", err.Error())
	}
	if result.FinalStatus != "canceled" || result.Item.Status != "canceled" {
		t.Fatalf("result = %#v, want canceled partial result", result)
	}
}

// TestPoll_TerminalFailureWithoutCallbackReturnsDefaultError verifies failed
// terminal states do not panic when the optional failure callback is omitted.
// It leaves ProgressMessage nil as well, which is where the default
// empty-message path is driven.
func TestPoll_TerminalFailureWithoutCallbackReturnsDefaultError(t *testing.T) {
	opts, _ := pollOptions("failed")
	opts.FailureError = nil
	opts.ProgressMessage = nil

	result, err := Poll(context.Background(), opts)
	if err == nil {
		t.Fatal("Poll() expected error, got nil")
	}
	if !strings.Contains(err.Error(), `terminal status "failed"`) {
		t.Fatalf("error = %q, want default terminal status error", err.Error())
	}
	if result.FinalStatus != "failed" {
		t.Fatalf("FinalStatus = %q, want failed", result.FinalStatus)
	}
}

// TestPoll_MissingRequiredCallbacks verifies required callbacks fail fast with
// clear errors instead of nil function pointer panics.
func TestPoll_MissingRequiredCallbacks(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Options[pollItem])
		wantErr string
	}{
		{
			name:    "poll",
			mutate:  func(opts *Options[pollItem]) { opts.Poll = nil },
			wantErr: "poll callback is required",
		},
		{
			name:    "status",
			mutate:  func(opts *Options[pollItem]) { opts.Status = nil },
			wantErr: "status callback is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts, _ := pollOptions("success")
			tc.mutate(&opts)

			result, err := Poll(context.Background(), opts)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Poll() error = %v, want %q", err, tc.wantErr)
			}
			if result != (Result[pollItem]{}) {
				t.Fatalf("result = %#v, want zero result", result)
			}
		})
	}
}

// TestPoll_PollError verifies poller errors stop the loop immediately.
func TestPoll_PollError(t *testing.T) {
	wantErr := errors.New("poll failed")
	opts, _ := pollOptions("running")
	opts.Poll = func(context.Context) (pollItem, error) { return pollItem{}, wantErr }

	result, err := Poll(context.Background(), opts)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Poll() error = %v, want %v", err, wantErr)
	}
	if result != (Result[pollItem]{}) {
		t.Fatalf("result = %#v, want zero result", result)
	}
}

// TestPoll_TickerContinuesUntilTerminal verifies non-terminal states wait for
// the ticker before polling again.
func TestPoll_TickerContinuesUntilTerminal(t *testing.T) {
	opts, attempts := pollOptions("running", "success")
	opts.PollDuration = func(seconds int) time.Duration {
		if seconds == opts.TimeoutSeconds {
			return 50 * time.Millisecond
		}
		return time.Millisecond
	}

	result, err := Poll(context.Background(), opts)
	if err != nil {
		t.Fatalf("Poll() unexpected error: %v", err)
	}
	if *attempts != 2 || result.PollCount != 2 || result.FinalStatus != "success" {
		t.Fatalf("attempts=%d result=%#v, want second-poll success", *attempts, result)
	}
}

// TestPoll_TimeoutReturnsLastItem verifies timeout returns the last observed
// non-terminal item and TimedOut=true.
//
// This is where the property jobs.TestJobWait_Timeout cannot pin deterministically
// is pinned instead: the poll callback here ignores its context and returns at
// once, so a status is always read before the deadline, with no I/O to lose the
// race to.
//
// The durations are keyed on the seconds value rather than left to
// fastDuration, which returns a millisecond for both and so left the ticker and
// the deadline racing: whenever the ticker won, a second poll ran and Value was
// 2. Making the interval an hour is what the assertion on Value = 1 means.
func TestPoll_TimeoutReturnsLastItem(t *testing.T) {
	opts, _ := pollOptions("running")
	opts.IntervalSeconds = 60
	opts.TimeoutSeconds = 1
	opts.PollDuration = func(seconds int) time.Duration {
		if seconds == opts.TimeoutSeconds {
			return time.Millisecond
		}
		return time.Hour
	}

	result, err := Poll(context.Background(), opts)
	if err != nil {
		t.Fatalf("Poll() unexpected error: %v", err)
	}
	if !result.TimedOut || result.FinalStatus != "running" || result.Item.Value != 1 {
		t.Fatalf("result = %#v, want timed out running item", result)
	}
}

// TestPoll_ImmediateTimeoutReturnsBeforePolling verifies an already expired
// timeout stops before invoking the poll callback.
func TestPoll_ImmediateTimeoutReturnsBeforePolling(t *testing.T) {
	opts, _ := pollOptions("running")
	opts.IntervalSeconds = 2
	opts.TimeoutSeconds = 1
	opts.PollDuration = func(seconds int) time.Duration {
		if seconds == opts.TimeoutSeconds {
			return 0
		}
		return time.Hour
	}
	opts.Poll = func(context.Context) (pollItem, error) {
		t.Fatal("Poll callback should not run after an immediate timeout")
		return pollItem{}, nil
	}

	result, err := Poll(context.Background(), opts)
	if err != nil {
		t.Fatalf("Poll() unexpected error: %v", err)
	}
	if !result.TimedOut || result.PollCount != 0 {
		t.Fatalf("result = %#v, want timeout before polling", result)
	}
}

// TestPoll_PollReceivesTimeoutContext verifies a slow poller receives a context
// bounded by timeout_seconds and Poll returns a timeout result when it expires.
func TestPoll_PollReceivesTimeoutContext(t *testing.T) {
	opts, _ := pollOptions("running")
	opts.Poll = func(ctx context.Context) (pollItem, error) {
		<-ctx.Done()
		return pollItem{}, ctx.Err()
	}

	result, err := Poll(context.Background(), opts)
	if err != nil {
		t.Fatalf("Poll() unexpected error: %v", err)
	}
	if !result.TimedOut || result.PollCount != 1 || result.FinalStatus != "" {
		t.Fatalf("result = %#v, want timeout during first poll", result)
	}
}

// TestPoll_CallbackDeadlineExceededBeforeTimeoutReturnsError verifies a poller
// owned deadline error is not converted into the global wait timeout.
func TestPoll_CallbackDeadlineExceededBeforeTimeoutReturnsError(t *testing.T) {
	opts, _ := pollOptions("running")
	opts.IntervalSeconds = 60
	opts.TimeoutSeconds = 1
	opts.PollDuration = func(seconds int) time.Duration {
		if seconds == opts.TimeoutSeconds {
			return 50 * time.Millisecond
		}
		return time.Hour
	}
	opts.Poll = func(context.Context) (pollItem, error) {
		return pollItem{}, context.DeadlineExceeded
	}

	result, err := Poll(context.Background(), opts)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Poll() error = %v, want context.DeadlineExceeded", err)
	}
	if result != (Result[pollItem]{}) {
		t.Fatalf("result = %#v, want zero result", result)
	}
}

// TestPoll_PollErrorAfterTheDeadlineIsReturnedNotReportedAsTimedOut verifies
// that an error the poller raises for its own reasons is returned even when the
// wait deadline passed while that call was in flight.
//
// Only this wait's own deadline may become a timed-out result. A slow call that
// comes back with a 502 after the deadline still has to surface: reported as a
// timeout it would tell the caller the resource is merely still running, when
// what happened is that GitLab refused. The three conditions in
// pollReachedDeadline are an AND for exactly this reason, and the clock being
// past the deadline is on its own no reason to swallow an error.
func TestPoll_PollErrorAfterTheDeadlineIsReturnedNotReportedAsTimedOut(t *testing.T) {
	wantErr := errors.New("502 bad gateway")
	opts, _ := pollOptions("running")
	opts.IntervalSeconds = 60
	opts.TimeoutSeconds = 1
	opts.PollDuration = func(seconds int) time.Duration {
		if seconds == opts.TimeoutSeconds {
			return 5 * time.Millisecond
		}
		return time.Hour
	}
	opts.Poll = func(ctx context.Context) (pollItem, error) {
		<-ctx.Done() // outlive the wait deadline, then fail for another reason
		return pollItem{}, wantErr
	}

	result, err := Poll(context.Background(), opts)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Poll() error = %v, want %v", err, wantErr)
	}
	if result != (Result[pollItem]{}) {
		t.Fatalf("result = %#v, want zero result", result)
	}
}

// TestPoll_DeadlineErrorAfterCallerCancellationIsNotAWaitTimeout verifies that a
// context.DeadlineExceeded raised once the caller has already given up is
// returned as the error it is, rather than converted into a timed-out result.
//
// A timed-out result is a positive claim: this wait ran its full course and the
// resource had not finished. With the caller gone, the deadline the poller
// reports may well be somebody else's, and the wall clock having passed our own
// deadline too is not enough to claim it. This is the leg of pollReachedDeadline
// the suite never drove the false way, so the cancellation check could be
// removed and nothing would notice.
func TestPoll_DeadlineErrorAfterCallerCancellationIsNotAWaitTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	opts, _ := pollOptions("running")
	opts.IntervalSeconds = 60
	opts.TimeoutSeconds = 1
	opts.PollDuration = func(seconds int) time.Duration {
		if seconds == opts.TimeoutSeconds {
			return 5 * time.Millisecond
		}
		return time.Hour
	}
	opts.Poll = func(pollCtx context.Context) (pollItem, error) {
		deadline, ok := pollCtx.Deadline()
		if !ok {
			t.Errorf("poll context carries no deadline, want the wait deadline")
			return pollItem{}, context.DeadlineExceeded
		}
		cancel()
		// Put the wall clock past the wait deadline as well, so the only
		// condition still deciding the answer is the caller's cancellation.
		time.Sleep(time.Until(deadline) + time.Millisecond)
		return pollItem{}, context.DeadlineExceeded
	}

	result, err := Poll(ctx, opts)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Poll() error = %v, want context.DeadlineExceeded", err)
	}
	if result != (Result[pollItem]{}) {
		t.Fatalf("result = %#v, want zero result", result)
	}
}

// TestPoll_ContextCanceled verifies context cancellation returns ctx.Err after
// the first non-terminal poll.
func TestPoll_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	opts, _ := pollOptions("running")
	opts.Poll = func(context.Context) (pollItem, error) {
		cancel()
		return pollItem{Status: "running"}, nil
	}

	result, err := Poll(ctx, opts)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Poll() error = %v, want context.Canceled", err)
	}
	if result != (Result[pollItem]{}) {
		t.Fatalf("result = %#v, want zero result", result)
	}
}

// TestPoll_DeadlineDuringALaterPollReturnsTheLastObservedItem verifies that when
// the wait deadline expires while a later poll is in flight, the timed-out
// result carries what the previous poll observed rather than a zero item.
//
// The only other test reaching that path blocks on the very first poll, where
// the last observation is the zero value anyway, so both arguments carrying it
// could be replaced by zeros and the suite would stay green. A caller whose job
// was running when the wait ran out has to be told it was running: an empty
// final_status reads as a job that never reported anything at all.
func TestPoll_DeadlineDuringALaterPollReturnsTheLastObservedItem(t *testing.T) {
	opts, _ := pollOptions("running")
	opts.IntervalSeconds = 5
	opts.TimeoutSeconds = 1
	opts.PollDuration = func(seconds int) time.Duration {
		if seconds == opts.TimeoutSeconds {
			return 300 * time.Millisecond
		}
		return 5 * time.Millisecond
	}
	polls := 0
	opts.Poll = func(pollCtx context.Context) (pollItem, error) {
		polls++
		if polls == 1 {
			return pollItem{Status: "running", Value: 1}, nil
		}
		<-pollCtx.Done() // outlive the wait deadline on the second attempt
		return pollItem{}, pollCtx.Err()
	}

	result, err := Poll(context.Background(), opts)
	if err != nil {
		t.Fatalf("Poll() unexpected error: %v", err)
	}
	if !result.TimedOut || result.PollCount != 2 {
		t.Fatalf("result = %#v, want a timeout on the second poll", result)
	}
	if result.FinalStatus != "running" || result.Item.Value != 1 {
		t.Fatalf("result = %#v, want the first poll's running item", result)
	}
}

// TestPoll_WaitedForReportsTheElapsedTime verifies both result constructors fill
// WaitedFor with how long the wait actually took.
//
// Nothing asserted that field: deleting it from both constructors left the whole
// suite green, while job.wait and pipeline.wait publish it as waited_for, which
// is what tells a caller whether a wait that timed out had been running for a
// second or for an hour. Each case below waits long enough that the field's
// rounding to whole seconds cannot render it as "0s", so what is asserted is
// elapsed time rather than a constant the code happens to produce.
func TestPoll_WaitedForReportsTheElapsedTime(t *testing.T) {
	const pastTheRounding = 700 * time.Millisecond

	tests := []struct {
		name         string
		configure    func(opts *Options[pollItem])
		wantTimedOut bool
	}{
		{
			name: "terminal",
			configure: func(opts *Options[pollItem]) {
				opts.PollDuration = func(int) time.Duration { return time.Hour }
				opts.Poll = func(context.Context) (pollItem, error) {
					time.Sleep(pastTheRounding)
					return pollItem{Status: "success"}, nil
				}
			},
		},
		{
			name:         "timed out",
			wantTimedOut: true,
			configure: func(opts *Options[pollItem]) {
				opts.PollDuration = func(seconds int) time.Duration {
					if seconds == opts.TimeoutSeconds {
						return pastTheRounding
					}
					return time.Hour
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts, _ := pollOptions("running")
			tc.configure(&opts)

			result, err := Poll(context.Background(), opts)
			if err != nil {
				t.Fatalf("Poll() unexpected error: %v", err)
			}
			if result.TimedOut != tc.wantTimedOut {
				t.Fatalf("TimedOut = %v, want %v", result.TimedOut, tc.wantTimedOut)
			}
			waited, err := time.ParseDuration(result.WaitedFor)
			if err != nil {
				t.Fatalf("WaitedFor = %q, want a duration: %v", result.WaitedFor, err)
			}
			if waited < time.Second {
				t.Fatalf("WaitedFor = %q, want at least the %s this wait took", result.WaitedFor, pastTheRounding)
			}
		})
	}
}

// progressToolInput and progressToolOutput carry the schemas of the stand-in
// tool TestPoll_ProgressNotificationsNumberAndNameEachAttempt registers, since
// Poll only reports progress through a real MCP session.
type progressToolInput struct{}

type progressToolOutput struct {
	FinalStatus string `json:"final_status"`
}

// TestPoll_ProgressNotificationsNumberAndNameEachAttempt verifies that each
// attempt sends one progress notification carrying that attempt's number and the
// message ProgressMessage built for it.
//
// Every other test leaves Request nil, so the tracker is inactive and the whole
// progress path is a no-op: it could be deleted, or handed attempt 0 for every
// poll, without an assertion moving. That series is the only thing a caller sees
// while a wait is running, and a wait that reports no progress looks to them
// exactly like one that hung.
func TestPoll_ProgressNotificationsNumberAndNameEachAttempt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	server := mcp.NewServer(&mcp.Implementation{Name: "waitpoll-test-server", Version: "0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "wait_with_progress"},
		func(callCtx context.Context, req *mcp.CallToolRequest, _ progressToolInput) (*mcp.CallToolResult, progressToolOutput, error) {
			opts, _ := pollOptions("running", "running", "success")
			opts.Request = req
			opts.IntervalSeconds = 5
			opts.TimeoutSeconds = 1
			opts.PollDuration = func(seconds int) time.Duration {
				if seconds == opts.TimeoutSeconds {
					return 10 * time.Second
				}
				return time.Millisecond
			}
			opts.ProgressMessage = func(attempt int) string { return fmt.Sprintf("attempt %d", attempt) }

			result, err := Poll(callCtx, opts)
			if err != nil {
				return nil, progressToolOutput{}, err
			}
			return nil, progressToolOutput{FinalStatus: result.FinalStatus}, nil
		})

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	notifications := make(chan *mcp.ProgressNotificationParams, 8)
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "waitpoll-test-client"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			select {
			case notifications <- req.Params:
			default:
			}
		},
	})
	clientSession, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		_ = serverSession.Wait()
	})

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "wait_with_progress",
		Arguments: map[string]any{},
		Meta:      mcp.Meta{"progressToken": "waitpoll-progress-token"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool result reports an error: %+v", res.Content)
	}

	for attempt := 1; attempt <= 3; attempt++ {
		select {
		case params := <-notifications:
			if params.Progress != float64(attempt) {
				t.Errorf("notification %d: progress = %v, want %d", attempt, params.Progress, attempt)
			}
			if want := fmt.Sprintf("attempt %d", attempt); params.Message != want {
				t.Errorf("notification %d: message = %q, want %q", attempt, params.Message, want)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for progress notification %d", attempt)
		}
	}
}

// TestPoll_ContextCanceledBeforeFirstPoll verifies a pre-canceled context fails
// before invoking the poll callback.
func TestPoll_ContextCanceledBeforeFirstPoll(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opts, _ := pollOptions("running")
	opts.Poll = func(context.Context) (pollItem, error) {
		t.Fatal("Poll callback should not run with a pre-canceled context")
		return pollItem{}, nil
	}

	result, err := Poll(ctx, opts)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Poll() error = %v, want context.Canceled", err)
	}
	if result != (Result[pollItem]{}) {
		t.Fatalf("result = %#v, want zero result", result)
	}
}
