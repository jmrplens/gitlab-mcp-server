package evalrun

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestUniqueSuffix_ReturnsPathSafeToken(t *testing.T) {
	got := UniqueSuffix()
	if strings.TrimSpace(got) == "" || strings.ContainsAny(got, " /\\") {
		t.Fatalf("UniqueSuffix() = %q, want non-empty path-safe token", got)
	}
}

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
