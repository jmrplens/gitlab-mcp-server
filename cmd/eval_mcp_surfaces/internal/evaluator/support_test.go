package evaluator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestSupportLiveUniqueSuffix_ReturnsHexLikeToken verifies live resource suffixes
// are non-empty and safe to embed in GitLab resource names.
func TestSupportLiveUniqueSuffix_ReturnsHexLikeToken(t *testing.T) {
	got := liveUniqueSuffix()
	if strings.TrimSpace(got) == "" || strings.ContainsAny(got, " /\\") {
		t.Fatalf("liveUniqueSuffix() = %q, want non-empty path-safe token", got)
	}
}

// TestWaitForContext_TimerAndCancellation verifies the helper returns on both
// timer expiration and context cancellation.
func TestWaitForContext_TimerAndCancellation(t *testing.T) {
	if err := waitForContext(t.Context(), time.Nanosecond); err != nil {
		t.Fatalf("waitForContext(timer) error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := waitForContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForContext(canceled) error = %v, want context.Canceled", err)
	}
}
