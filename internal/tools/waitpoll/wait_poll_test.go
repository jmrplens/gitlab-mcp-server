package waitpoll

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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

// TestPoll_TerminalFailureWithoutCallbackReturnsDefaultError verifies failed
// terminal states do not panic when the optional failure callback is omitted.
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
func TestPoll_TimeoutReturnsLastItem(t *testing.T) {
	opts, _ := pollOptions("running")
	opts.IntervalSeconds = 60
	opts.TimeoutSeconds = 1

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
