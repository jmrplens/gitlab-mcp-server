// refusal_log_test.go covers the throttle on the refusal lines a caller can
// trigger before any credential is checked.
package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// TestLineThrottle_OnePerMessagePerWindow verifies the accounting: the first
// write of a message goes through, repeats inside the window are held back
// and counted, a different message has its own window, and the first write
// after the window carries the count.
func TestLineThrottle_OnePerMessagePerWindow(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	throttle := newLineThrottle(time.Minute)
	throttle.now = func() time.Time { return clock }

	cases := []struct {
		name           string
		advance        time.Duration
		msg            string
		wantWrite      bool
		wantSuppressed int
	}{
		{name: "the first write goes through", msg: "a", wantWrite: true},
		{name: "a repeat inside the window is held", msg: "a", wantWrite: false},
		{name: "another repeat is counted too", msg: "a", wantWrite: false},
		{name: "a different message has its own window", msg: "b", wantWrite: true},
		{name: "just inside the window is still held", advance: 59 * time.Second, msg: "a", wantWrite: false},
		{name: "past the window it is written with the count", advance: time.Second, msg: "a", wantWrite: true, wantSuppressed: 3},
		{name: "the count restarts", msg: "a", wantWrite: false},
		{name: "the other message's window is its own", advance: 30 * time.Second, msg: "b", wantWrite: true, wantSuppressed: 0},
	}
	// sequential: each step's verdict depends on the writes before it
	for _, tc := range cases {
		clock = clock.Add(tc.advance)
		suppressed, write := throttle.admit(tc.msg)
		if write != tc.wantWrite || suppressed != tc.wantSuppressed {
			t.Fatalf("%s: admit(%q) = (%d, %v), want (%d, %v)", tc.name, tc.msg, suppressed, write, tc.wantSuppressed, tc.wantWrite)
		}
	}
}

// TestLineThrottle_Log verifies what reaches the log: the line itself, the
// attributes, and the count of held-back writes on the next line.
//
// Not parallel: it replaces the process-wide default logger.
func TestLineThrottle_Log(t *testing.T) {
	var out bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelInfo})))

	clock := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	throttle := newLineThrottle(time.Minute)
	throttle.now = func() time.Time { return clock }
	ctx := context.Background()

	throttle.log(ctx, slog.LevelInfo, "request rejected: no token", "ip", "203.0.113.7")
	throttle.log(ctx, slog.LevelInfo, "request rejected: no token", "ip", "203.0.113.8")
	throttle.log(ctx, slog.LevelInfo, "request rejected: no token", "ip", "203.0.113.9")
	clock = clock.Add(2 * time.Minute)
	throttle.log(ctx, slog.LevelInfo, "request rejected: no token", "ip", "203.0.113.10")
	// Below the handler's level: neither written nor counted.
	throttle.log(ctx, slog.LevelDebug, "request rejected: debug only")

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), out.String())
	}
	if !strings.Contains(lines[0], "ip=203.0.113.7") || strings.Contains(lines[0], "also_since_last_report") {
		t.Errorf("first line = %q, want the first caller and no count", lines[0])
	}
	if !strings.Contains(lines[1], "ip=203.0.113.10") || !strings.Contains(lines[1], "also_since_last_report=2") {
		t.Errorf("second line = %q, want the caller after the window and the two held back", lines[1])
	}
	if strings.Contains(out.String(), "debug only") {
		t.Error("a line below the handler's level was written")
	}
}
