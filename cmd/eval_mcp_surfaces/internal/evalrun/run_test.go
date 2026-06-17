// run_test.go covers the small helpers shared by the live evaluation command:
// deterministic path-safe unique suffixes and a context-aware wait helper.

package evalrun

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestUniqueSuffix_ReturnsPathSafeToken verifies that [UniqueSuffix] produces a
// non-empty identifier free of path separators or whitespace, so it can be used
// to namespace ephemeral GitLab resources created during evaluation runs
// without colliding on allowed-name validation.
//
// The test calls UniqueSuffix once and asserts the token is non-empty and
// contains no space, slash, or backslash characters. This protects live fixtures
// (group paths, project paths, branch names) from being rejected by GitLab
// because of an unsafe unique-suffix token.
func TestUniqueSuffix_ReturnsPathSafeToken(t *testing.T) {
	got := UniqueSuffix()
	if strings.TrimSpace(got) == "" || strings.ContainsAny(got, " /\\") {
		t.Fatalf("UniqueSuffix() = %q, want non-empty path-safe token", got)
	}
}

// TestWaitForContext_TimerAndCancellation verifies that [WaitForContext] blocks
// until the supplied interval elapses and returns [context.Canceled] when the
// context is canceled before the interval finishes.
//
// The first subcase uses a one-nanosecond timer to confirm the helper returns
// nil after the timer fires. The second subcase cancels a derived context
// immediately and asserts the helper returns a wrapped [context.Canceled]
// error detected with [errors.Is]. This protects the evaluator's delay loops
// from silently ignoring cancellations.
func TestWaitForContext_TimerAndCancellation(t *testing.T) {
	if err := WaitForContext(t.Context(), time.Nanosecond); err != nil {
		t.Fatalf("WaitForContext(timer) error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := WaitForContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitForContext(canceled) error = %v, want context.Canceled", err)
	}
}
