//go:build e2e

// poll.go holds the two ways a test waits for GitLab.
//
// Poll waits for a condition to become true and says what it last saw when it
// gives up, because "timed out" without the last observation is a failure
// nobody can act on. Retry runs an operation again after a failure the caller
// classified as transient, which is what a GitLab that has just booted, or one
// under the load of a parallel suite, produces.

package harness

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// ErrPollTimeout identifies a Poll that exhausted its wait budget, so a caller
// can tell a timeout from a condition that failed outright.
var ErrPollTimeout = errors.New("poll timeout")

// defaultPollInterval is what Poll uses when the caller names no interval.
const defaultPollInterval = 100 * time.Millisecond

// Poll evaluates condition until it reports done, returns an error, runs out
// of time, or ctx is cancelled.
//
// The condition returns three things: whether it is done, what it observed,
// and an error. The error is for a failure that will not become success by
// waiting; anything worth waiting through is reported as state instead, and
// comes back in the timeout message.
func Poll(ctx context.Context, interval, timeout time.Duration, condition func() (bool, string, error)) error {
	if condition == nil {
		return errors.New("poll condition is nil")
	}
	if interval <= 0 {
		interval = defaultPollInterval
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

// pollContextError reports a cancelled context as a timeout when the context
// ended because its own deadline passed, and as a cancellation otherwise.
func pollContextError(err error, timeout time.Duration, lastState string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return pollTimeoutError(timeout, lastState)
	}
	return fmt.Errorf("poll canceled: %w", err)
}

// pollTimeoutError wraps ErrPollTimeout with the budget that ran out and the
// last thing the condition saw.
func pollTimeoutError(timeout time.Duration, lastState string) error {
	return fmt.Errorf("%w after %s (last state: %s)", ErrPollTimeout, timeout, lastState)
}

// stopTimer stops timer and drains it when the stop came too late, so a caller
// leaving a select does not leak the timer's send.
func stopTimer(timer *time.Timer) {
	if timer.Stop() {
		return
	}
	select {
	case <-timer.C:
	default:
	}
}

// Retry runs operation until it succeeds, reports a failure it calls
// permanent, runs out of attempts, or ctx is cancelled. The delay between
// attempts grows by baseDelay each time.
//
// The operation returns its result, whether the failure is worth another
// attempt, a short reason for the log, and the error. Naming the reason is
// what makes a retried run readable afterwards: "attempt 2/3 failed (502 from
// a booting GitLab)" is a story, and a bare repeated error is not.
func Retry[O any](
	ctx context.Context,
	tb testing.TB,
	label string,
	maxRetries int,
	baseDelay time.Duration,
	operation func(attempt int) (O, bool, string, error),
) (O, error) {
	tb.Helper()

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

		tb.Logf("%s: attempt %d/%d failed (%s), retrying: %v", label, attempt+1, maxRetries, reason, err)
		select {
		case <-ctx.Done():
			return output, fmt.Errorf("%s canceled before retry after attempt %d/%d: %w (last error: %s)",
				label, attempt+1, maxRetries, ctx.Err(), err.Error())
		case <-time.After(time.Duration(attempt+1) * baseDelay):
		}
	}

	return output, fmt.Errorf("%s failed without executing retry operation", label)
}
