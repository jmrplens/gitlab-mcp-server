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

// TestLiveUniqueSuffix_ReturnsDistinctNonEmptyValues verifies LiveUniqueSuffix returns distinct non empty values.
func TestLiveUniqueSuffix_ReturnsDistinctNonEmptyValues(t *testing.T) {
	first := liveUniqueSuffix()
	second := liveUniqueSuffix()
	if first == "" || second == "" {
		t.Fatalf("liveUniqueSuffix() returned empty values: %q %q", first, second)
	}
	if first == second {
		t.Fatalf("liveUniqueSuffix() returned duplicate values: %q", first)
	}
}

// TestWaitForContext_CanceledContextReturnsError verifies WaitForContext when canceled context returns error.
func TestWaitForContext_CanceledContextReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := waitForContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForContext() error = %v, want context.Canceled", err)
	}
}
