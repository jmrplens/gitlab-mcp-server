// action_timeout_test.go covers the deadline every action runs under.
package toolutil

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// withActionTimeout sets the process-wide deadline for one test and restores
// what was there before, which is why none of these tests is parallel.
func withActionTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	previous := ActionTimeout()
	SetActionTimeout(d)
	t.Cleanup(func() { SetActionTimeout(previous) })
}

// blockingAction is an action that only returns when its context ends, the
// shape of a handler whose client is still waiting.
func blockingAction(ctx context.Context, _ *gitlabclient.Client, _ struct{}) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

// TestWrapAction_DeadlineEndsAHandlerThatNeverReturns verifies the bound is
// applied where every action passes, and that the error the caller sees is
// the deadline rather than a hang.
func TestWrapAction_DeadlineEndsAHandlerThatNeverReturns(t *testing.T) {
	withActionTimeout(t, 30*time.Millisecond)

	done := make(chan error, 1)
	go func() {
		_, err := WrapAction(nil, blockingAction)(context.Background(), nil)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("error = %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the action outlived a 30ms deadline by five seconds")
	}
}

// TestWrapActionWithRequest_DeadlineApplies verifies the request-aware wrapper
// runs under the same bound.
func TestWrapActionWithRequest_DeadlineApplies(t *testing.T) {
	withActionTimeout(t, 30*time.Millisecond)

	deadlineSeen := make(chan bool, 1)
	fn := WrapActionWithRequest(nil, func(ctx context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ struct{}) (string, error) {
		_, ok := ctx.Deadline()
		deadlineSeen <- ok
		return "", nil
	})
	if _, err := fn(context.Background(), nil); err != nil {
		t.Fatalf("WrapActionWithRequest: %v", err)
	}
	if !<-deadlineSeen {
		t.Error("the handler ran without a deadline while one was configured")
	}
}

// TestWrapVoidAction_DeadlineEndsAHandlerThatNeverReturns covers the wrapper
// the deadline used to skip.
//
// A void action is a delete, a cancel or a retry, so it is the shape an
// operator most wants bounded, and it was the one shape running unbounded
// while this setting's documentation said every action passed through the
// deadline. A handler that never returns held a goroutine, a connection and
// the caller's pooled entry for as long as the client cared to wait.
func TestWrapVoidAction_DeadlineEndsAHandlerThatNeverReturns(t *testing.T) {
	withActionTimeout(t, 30*time.Millisecond)

	done := make(chan error, 1)
	go func() {
		_, err := WrapVoidAction(nil, func(ctx context.Context, _ *gitlabclient.Client, _ struct{}) error {
			<-ctx.Done()
			return ctx.Err()
		})(context.Background(), nil)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("error = %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the void action outlived a 30ms deadline by five seconds")
	}
}

// TestWrapVoidActionWithRequest_DeadlineApplies covers the fourth wrapper, so
// that all four routes an action can be registered through carry the bound.
func TestWrapVoidActionWithRequest_DeadlineApplies(t *testing.T) {
	withActionTimeout(t, 30*time.Millisecond)

	deadlineSeen := make(chan bool, 1)
	fn := WrapVoidActionWithRequest(nil, func(ctx context.Context, _ *mcp.CallToolRequest, _ *gitlabclient.Client, _ struct{}) error {
		_, ok := ctx.Deadline()
		deadlineSeen <- ok
		return nil
	})
	if _, err := fn(context.Background(), nil); err != nil {
		t.Fatalf("WrapVoidActionWithRequest: %v", err)
	}
	if !<-deadlineSeen {
		t.Error("the handler ran without a deadline while one was configured")
	}
}

// TestWrapAction_ZeroDisablesTheDeadline verifies 0 means no bound at all,
// rather than a zero-length one.
func TestWrapAction_ZeroDisablesTheDeadline(t *testing.T) {
	withActionTimeout(t, 0)

	deadlineSeen := make(chan bool, 1)
	fn := WrapAction(nil, func(ctx context.Context, _ *gitlabclient.Client, _ struct{}) (string, error) {
		_, ok := ctx.Deadline()
		deadlineSeen <- ok
		return "", nil
	})
	if _, err := fn(context.Background(), nil); err != nil {
		t.Fatalf("WrapAction: %v", err)
	}
	if <-deadlineSeen {
		t.Error("the handler ran under a deadline while none was configured")
	}
}

// TestWrapAction_CallersEarlierDeadlineIsKept verifies a caller's tighter
// deadline is not extended to the configured one.
func TestWrapAction_CallersEarlierDeadlineIsKept(t *testing.T) {
	withActionTimeout(t, time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := WrapAction(nil, blockingAction)(ctx, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("the action ran %s under a 20ms caller deadline", elapsed)
	}
}

// TestSetActionTimeout_NegativeMeansNone verifies a negative value is read as
// disabled rather than as an already-expired deadline.
func TestSetActionTimeout_NegativeMeansNone(t *testing.T) {
	withActionTimeout(t, -time.Second)
	if got := ActionTimeout(); got != 0 {
		t.Errorf("ActionTimeout() = %s after a negative value, want 0", got)
	}
}

// TestDefaultActionTimeout_OutlastsTheLongestWait pins the default above the
// longest wait any action offers. A pipeline wait may run for PollMaxTimeout
// seconds and the deadline starts before the handler does, so a default equal
// to that wait would end it a moment before it returned on its own and report
// a deadline error in place of the wait's own answer. The inequality lives
// here because config cannot import the constant it has to exceed.
func TestDefaultActionTimeout_OutlastsTheLongestWait(t *testing.T) {
	longest := time.Duration(PollMaxTimeout) * time.Second
	if config.DefaultActionTimeout <= longest {
		t.Fatalf("DefaultActionTimeout = %s, want more than the longest wait an action offers (%s)",
			config.DefaultActionTimeout, longest)
	}
}
