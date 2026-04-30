//go:build e2e

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
func Poll(ctx context.Context, interval time.Duration, timeout time.Duration, condition func() (bool, string, error)) error {
	if condition == nil {
		return errors.New("poll condition is nil")
	}
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	if timeout <= 0 {
		timeout = interval
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	lastState := "no state observed"
	for {
		select {
		case <-ctx.Done():
			return pollContextError(ctx.Err(), timeout, lastState)
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
			return pollContextError(ctx.Err(), timeout, lastState)
		case <-deadline.C:
			stopTimer(wait)
			return pollTimeoutError(timeout, lastState)
		case <-wait.C:
		}
	}
}

func pollContextError(err error, timeout time.Duration, lastState string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return pollTimeoutError(timeout, lastState)
	}
	return fmt.Errorf("poll canceled: %w", err)
}

func pollTimeoutError(timeout time.Duration, lastState string) error {
	return fmt.Errorf("%w after %s (last state: %s)", ErrPollTimeout, timeout, lastState)
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

func retryWithBackoff[O any](ctx context.Context, t *testing.T, label string, maxRetries int, operation func(attempt int) (O, bool, string, error)) (O, error) {
	return retryWithBackoffInterval(ctx, t, label, maxRetries, time.Second, operation)
}

func retryWithBackoffInterval[O any](ctx context.Context, t *testing.T, label string, maxRetries int, baseDelay time.Duration, operation func(attempt int) (O, bool, string, error)) (O, error) {
	t.Helper()
	var output O
	if operation == nil {
		return output, errors.New("retry operation is nil")
	}
	if maxRetries <= 0 {
		maxRetries = 1
	}
	if baseDelay <= 0 {
		baseDelay = time.Millisecond
	}

	for attempt := range maxRetries {
		result, retryable, reason, err := operation(attempt)
		output = result
		if err == nil {
			return output, nil
		}
		if attempt >= maxRetries-1 || !retryable {
			return output, fmt.Errorf("%s failed after %d attempt(s): %w", label, attempt+1, err)
		}
		if reason == "" {
			reason = "retryable error"
		}

		t.Logf("%s: attempt %d/%d failed (%s), retrying: %v", label, attempt+1, maxRetries, reason, err)
		delay := time.Duration(attempt+1) * baseDelay
		select {
		case <-ctx.Done():
			return output, fmt.Errorf("%s canceled before retry after attempt %d/%d: %w (last error: %s)", label, attempt+1, maxRetries, ctx.Err(), err.Error())
		case <-time.After(delay):
		}
	}

	return output, fmt.Errorf("%s failed without executing retry operation", label)
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

func TestRetryWithBackoffInterval_RetrySuccess(t *testing.T) {
	attempts := 0
	result, err := retryWithBackoffInterval(context.Background(), t, "retry test", 3, time.Millisecond, func(int) (int, bool, string, error) {
		attempts++
		if attempts < 2 {
			return 0, true, "transient", errors.New("try again")
		}
		return 42, false, "", nil
	})

	if err != nil {
		t.Fatalf("retryWithBackoffInterval() error = %v, want nil", err)
	}
	if result != 42 {
		t.Fatalf("result = %d, want 42", result)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestRetryWithBackoffInterval_ReturnsNonRetryableError(t *testing.T) {
	failure := errors.New("permanent failure")
	_, err := retryWithBackoffInterval(context.Background(), t, "retry test", 3, time.Millisecond, func(int) (int, bool, string, error) {
		return 0, false, "", failure
	})

	if !errors.Is(err, failure) {
		t.Fatalf("retryWithBackoffInterval() error = %v, want permanent failure", err)
	}
}

func TestRetryWithBackoffInterval_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	_, err := retryWithBackoffInterval(ctx, t, "retry test", 3, 10*time.Millisecond, func(int) (int, bool, string, error) {
		attempts++
		cancel()
		return 0, true, "transient", errors.New("try again")
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("retryWithBackoffInterval() error = %v, want context.Canceled", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}
