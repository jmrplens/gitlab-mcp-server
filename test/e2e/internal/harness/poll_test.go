//go:build e2e

// poll_test.go covers the two waiting primitives, ported from the suite this
// replaces (wait_helpers_ce_test.go).

package harness

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestPoll_ConditionTrueAtOnce_ReturnsWithoutWaiting checks that a condition
// already satisfied costs nothing.
//
// The condition reports done on its first call, and the assertion is that it
// ran exactly once. A Poll that slept before its first evaluation would add
// its interval to every wait in the suite for no reason at all.
func TestPoll_ConditionTrueAtOnce_ReturnsWithoutWaiting(t *testing.T) {
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

// TestPoll_ConditionTrueLater_KeepsPolling checks the retry loop itself.
//
// The condition reports a state twice and then success, and the assertion is
// that all three calls happened inside the budget. This is the path every
// eventual-consistency wait in the suite takes.
func TestPoll_ConditionTrueLater_KeepsPolling(t *testing.T) {
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

// TestPoll_ContextAlreadyCancelled_ReturnsCancellation checks that a cancelled
// context is respected before the condition is evaluated at all.
//
// A test whose context has ended is over, and evaluating its condition once
// more would issue a GitLab request nobody is waiting for.
func TestPoll_ContextAlreadyCancelled_ReturnsCancellation(t *testing.T) {
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

// TestPoll_BudgetExhausted_ReportsTheLastState checks the message a timeout
// carries.
//
// The error wraps ErrPollTimeout so a caller can tell it from a condition
// failure, and it quotes the last observation. "Timed out after 30s" alone
// says nothing about what GitLab was doing; "last state: pipeline pending"
// is the whole finding.
func TestPoll_BudgetExhausted_ReportsTheLastState(t *testing.T) {
	err := Poll(context.Background(), time.Millisecond, 3*time.Millisecond, func() (bool, string, error) {
		return false, "still waiting", nil
	})

	if !errors.Is(err, ErrPollTimeout) {
		t.Fatalf("Poll() error = %v, want ErrPollTimeout", err)
	}
	if !strings.Contains(err.Error(), "still waiting") {
		t.Fatalf("Poll() error = %q, want the last observed state", err.Error())
	}
}

// TestPoll_ConditionError_ReturnsItUnwrapped checks that a condition failure
// stops the wait and reaches the caller intact.
//
// The condition returns an error only for a failure waiting cannot fix, so
// Poll returns it as it is and errors.Is still matches the sentinel behind it.
func TestPoll_ConditionError_ReturnsItUnwrapped(t *testing.T) {
	conditionErr := errors.New("condition failed")
	err := Poll(context.Background(), time.Millisecond, time.Second, func() (bool, string, error) {
		return false, "failed", conditionErr
	})

	if !errors.Is(err, conditionErr) {
		t.Fatalf("Poll() error = %v, want the condition's own error", err)
	}
}

// TestPoll_NilCondition_IsRefused checks that a Poll with nothing to evaluate
// reports the mistake rather than spinning until its budget runs out.
func TestPoll_NilCondition_IsRefused(t *testing.T) {
	if err := Poll(context.Background(), time.Millisecond, time.Millisecond, nil); err == nil {
		t.Fatal("Poll() with a nil condition returned nil, want an error")
	}
}

// TestRetry_RetryableFailure_ReturnsTheLaterSuccess checks the retry path.
//
// The operation calls its first failure retryable and succeeds on the second
// attempt; the assertion is that the later result comes back and that exactly
// two attempts were made. This is what absorbs a GitLab that has just booted.
func TestRetry_RetryableFailure_ReturnsTheLaterSuccess(t *testing.T) {
	attempts := 0
	result, err := Retry(context.Background(), t, "retry test", 3, time.Millisecond,
		func(int) (int, bool, string, error) {
			attempts++
			if attempts < 2 {
				return 0, true, "transient", errors.New("try again")
			}
			return 42, false, "", nil
		})
	if err != nil {
		t.Fatalf("Retry() error = %v, want nil", err)
	}
	if result != 42 {
		t.Fatalf("result = %d, want 42", result)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

// TestRetry_PermanentFailure_StopsAtOnce checks that a failure the operation
// calls permanent is not retried.
//
// Retrying a 403 three times only delays the report by the backoff, and the
// error still has to reach the caller for errors.Is to work on it.
func TestRetry_PermanentFailure_StopsAtOnce(t *testing.T) {
	failure := errors.New("permanent failure")
	attempts := 0
	_, err := Retry(context.Background(), t, "retry test", 3, time.Millisecond,
		func(int) (int, bool, string, error) {
			attempts++
			return 0, false, "", failure
		})

	if !errors.Is(err, failure) {
		t.Fatalf("Retry() error = %v, want the permanent failure", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

// TestRetry_ContextCancelled_StopsBetweenAttempts checks that a cancelled
// context ends the retry while it is waiting, rather than after the whole
// backoff has elapsed.
//
// The message keeps the last error beside the cancellation, because "context
// canceled" on its own does not say what was failing.
func TestRetry_ContextCancelled_StopsBetweenAttempts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	_, err := Retry(ctx, t, "retry test", 3, 10*time.Millisecond,
		func(int) (int, bool, string, error) {
			attempts++
			cancel()
			return 0, true, "transient", errors.New("try again")
		})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Retry() error = %v, want context.Canceled", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if !strings.Contains(err.Error(), "try again") {
		t.Fatalf("Retry() error = %q, want the last error beside the cancellation", err.Error())
	}
}

// TestRetry_NilOperation_IsRefused checks that a Retry with nothing to run
// reports the mistake instead of reporting success.
func TestRetry_NilOperation_IsRefused(t *testing.T) {
	if _, err := Retry[int](context.Background(), t, "retry test", 1, time.Millisecond, nil); err == nil {
		t.Fatal("Retry() with a nil operation returned nil, want an error")
	}
}
