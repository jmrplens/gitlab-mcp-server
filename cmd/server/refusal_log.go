// refusal_log.go throttles the log lines a caller can trigger at will.
//
// Every refusal the gates write before a credential is checked is a line an
// unauthenticated caller chooses to cause: send a request with no token, or a
// bad instance header, and the server logs it. Written once per request, that
// is a way to fill an operator's log at whatever rate the caller likes,
// hiding the lines that matter between thousands that do not. The refusals
// themselves are already counted, by the failure budget and the metrics; the
// line is what needs holding back, one per message per window, with how many
// it stands for on the next one that does get written.
package main

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// refusalLogWindow is how often one refusal message is written at most.
const refusalLogWindow = time.Minute

// refusalLog throttles the pre-authentication refusal lines of both gates.
var refusalLog = newLineThrottle(refusalLogWindow)

// lineThrottle writes each distinct message at most once per window.
type lineThrottle struct {
	window time.Duration
	// now is the clock, replaced in tests.
	now func() time.Time

	mu    sync.Mutex
	lines map[string]*throttledLine
}

// throttledLine is what the throttle remembers about one message.
type throttledLine struct {
	windowStart time.Time
	suppressed  int
}

// newLineThrottle returns a throttle that writes each message once per window.
func newLineThrottle(window time.Duration) *lineThrottle {
	return &lineThrottle{window: window, now: time.Now, lines: map[string]*throttledLine{}}
}

// log writes msg with args at level, unless the same msg was written inside
// the current window, in which case it is counted and the count is reported
// on the next line written. The count is attached as
// also_since_last_report, the same key the rate limiter's own refusal line
// uses.
func (t *lineThrottle) log(ctx context.Context, level slog.Level, msg string, args ...any) {
	if !slog.Default().Enabled(ctx, level) {
		return
	}
	suppressed, write := t.admit(msg)
	if !write {
		return
	}
	if suppressed > 0 {
		args = append(args, "also_since_last_report", suppressed)
	}
	slog.Default().Log(ctx, level, msg, args...)
}

// admit decides whether msg is written now, and returns how many writes of it
// were held back since the last one that was.
func (t *lineThrottle) admit(msg string) (suppressed int, write bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	line, seen := t.lines[msg]
	if seen && now.Sub(line.windowStart) < t.window {
		line.suppressed++
		return 0, false
	}
	if !seen {
		line = &throttledLine{}
		t.lines[msg] = line
	}
	suppressed = line.suppressed
	line.windowStart = now
	line.suppressed = 0
	return suppressed, true
}
