// action_timeout.go bounds how long one action may run.
//
// Every action passes through one of the four WrapAction functions, which
// makes them the one place a deadline reaches all of them. The transports
// already end a call whose
// client went away: an HTTP POST's lifetime cancels the calls it carries, and
// a stdio client's own cancellation notification reaches the handler. What
// neither bounds is a handler whose client is still waiting: a poll that
// never sees the state it asked for, a retry loop against an instance that
// answers slowly enough to keep every attempt alive. The deadline is what
// ends those, so the process cannot be made to hold a goroutine, a
// connection and a pooled entry for as long as someone cares to keep asking.

package toolutil

import (
	"context"
	"sync/atomic"
	"time"
)

// actionTimeoutNanos is the per-action deadline in nanoseconds; 0 means none.
// Process-wide: it is a property of the deployment, set once at startup from
// the configuration, and read on every call.
var actionTimeoutNanos atomic.Int64

// SetActionTimeout sets the deadline every action runs under. Zero or a
// negative value disables it.
func SetActionTimeout(d time.Duration) {
	if d < 0 {
		d = 0
	}
	actionTimeoutNanos.Store(int64(d))
}

// ActionTimeout reports the deadline every action runs under, 0 for none.
func ActionTimeout() time.Duration {
	return time.Duration(actionTimeoutNanos.Load())
}

// WithActionDeadline derives the context an action runs under: the caller's,
// bounded by the configured deadline when there is one. The cancel function
// is always safe to call.
//
// Exported for the one model-facing handler that is not a catalog action and
// so passes through none of the WrapAction functions: the dynamic surface's
// gitlab_find_action, whose cost is the catalog rather than a GitLab call and
// which was therefore the only registered tool with no deadline at all.
func WithActionDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	d := ActionTimeout()
	if d <= 0 {
		return ctx, func() { /* no deadline was added, so there is nothing to cancel */ }
	}
	return context.WithTimeout(ctx, d)
}
