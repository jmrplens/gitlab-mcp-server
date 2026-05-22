//go:build e2e && enterprise

// wait_helpers_test.go contains polling and retry helpers used by E2E tests to
// absorb GitLab Docker startup lag and eventual-consistency delays.
package suite

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// ErrPollTimeout identifies polling operations that exhausted their wait budget.
var ErrPollTimeout = errors.New("poll timeout")

// Poll repeatedly evaluates condition until it succeeds, fails, times out, or
// the context is canceled. The condition should return an error only for
// non-retryable failures; retryable observations should be reported as state.
func Poll(ctx context.Context, interval, timeout time.Duration, condition func() (bool, string, error)) error {
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

// pollContextError maps context cancellation into the timeout sentinel when the
// context ended because the polling wait budget expired.
func pollContextError(err error, timeout time.Duration, lastState string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return pollTimeoutError(timeout, lastState)
	}
	return fmt.Errorf("poll canceled: %w", err)
}

// pollTimeoutError wraps [ErrPollTimeout] with the configured wait budget and
// last observed state for actionable failure messages.
func pollTimeoutError(timeout time.Duration, lastState string) error {
	return fmt.Errorf("%w after %s (last state: %s)", ErrPollTimeout, timeout, lastState)
}

// stopTimer stops timer and drains its channel when needed so callers can leave
// select blocks without leaking timer state.
func stopTimer(timer *time.Timer) {
	if timer.Stop() {
		return
	}
	select {
	case <-timer.C:
	default:
	}
}

// retryWithBackoff runs operation with a one-second base delay between
// retryable failures.
func retryWithBackoff[O any](ctx context.Context, t *testing.T, label string, maxRetries int, operation func(attempt int) (O, bool, string, error)) (O, error) {
	t.Helper()
	return retryWithBackoffInterval(ctx, t, label, maxRetries, time.Second, operation)
}

// retryWithBackoffInterval runs operation until it succeeds, returns a
// non-retryable error, exhausts maxRetries, or ctx is canceled.
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
