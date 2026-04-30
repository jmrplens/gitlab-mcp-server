package suite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ErrPollTimeout identifies polling operations that exhausted their wait budget.
var ErrPollTimeout = errors.New("poll timeout")

// Poll repeatedly evaluates condition until it succeeds, fails, times out, or
// the context is canceled. The condition should return an error only for
// non-retryable failures; retryable observations should be reported as state.
func Poll(ctx context.Context, interval time.Duration, max time.Duration, condition func() (bool, string, error)) error {
	if condition == nil {
		return errors.New("poll condition is nil")
	}
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	if max <= 0 {
		max = interval
	}

	deadline := time.NewTimer(max)
	defer deadline.Stop()

	lastState := "no state observed"
	for {
		select {
		case <-ctx.Done():
			return pollContextError(ctx.Err(), max, lastState)
		default:
		}

		done, state, err := condition()
		if state != "" {
			lastState = state
		}
		if err != nil {
			return err
		}
		if done {
			return nil
		}

		wait := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			stopTimer(wait)
			return pollContextError(ctx.Err(), max, lastState)
		case <-deadline.C:
			stopTimer(wait)
			return pollTimeoutError(max, lastState)
		case <-wait.C:
		}
	}
}

func pollContextError(err error, max time.Duration, lastState string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return pollTimeoutError(max, lastState)
	}
	return fmt.Errorf("poll canceled: %w", err)
}

func pollTimeoutError(max time.Duration, lastState string) error {
	return fmt.Errorf("%w after %s (last state: %s)", ErrPollTimeout, max, lastState)
}

func stopTimer(timer *time.Timer) {
	if timer.Stop() {
		return
	}
	select {
	case <-timer.C:
	default:
	}
}

func TestPoll_ImmediateSuccess(t *testing.T) {
	calls := 0
	err := Poll(context.Background(), time.Millisecond, time.Second, func() (bool, string, error) {
		calls++
		return true, "ready", nil
	})

	if err != nil {
		t.Fatalf("Poll() error = %v, want nil", err)
	}
	if calls != 1 {
		t.Fatalf("condition calls = %d, want 1", calls)
	}
}

func TestPoll_RetrySuccess(t *testing.T) {
	calls := 0
	err := Poll(context.Background(), time.Millisecond, 100*time.Millisecond, func() (bool, string, error) {
		calls++
		if calls < 3 {
			return false, fmt.Sprintf("attempt %d", calls), nil
		}
		return true, "ready", nil
	})

	if err != nil {
		t.Fatalf("Poll() error = %v, want nil", err)
	}
	if calls != 3 {
		t.Fatalf("condition calls = %d, want 3", calls)
	}
}

func TestPoll_ReturnsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	err := Poll(ctx, time.Millisecond, time.Second, func() (bool, string, error) {
		calls++
		return false, "waiting", nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Poll() error = %v, want context.Canceled", err)
	}
	if calls != 0 {
		t.Fatalf("condition calls = %d, want 0", calls)
	}
}

func TestPoll_ReturnsTimeoutWithLastState(t *testing.T) {
	err := Poll(context.Background(), time.Millisecond, 3*time.Millisecond, func() (bool, string, error) {
		return false, "still waiting", nil
	})

	if !errors.Is(err, ErrPollTimeout) {
		t.Fatalf("Poll() error = %v, want ErrPollTimeout", err)
	}
	if !strings.Contains(err.Error(), "still waiting") {
		t.Fatalf("Poll() error = %q, want last state", err.Error())
	}
}

func TestPoll_ReturnsConditionError(t *testing.T) {
	conditionErr := errors.New("condition failed")
	err := Poll(context.Background(), time.Millisecond, time.Second, func() (bool, string, error) {
		return false, "failed", conditionErr
	})

	if !errors.Is(err, conditionErr) {
		t.Fatalf("Poll() error = %v, want condition error", err)
	}
}
