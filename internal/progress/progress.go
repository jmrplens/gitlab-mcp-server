// Package progress provides a Tracker for sending MCP progress notifications
// to the client during long-running tool operations.
//
// SECURITY: Progress tokens are opaque values provided by the client.
// They are forwarded as-is — never logged at a level above Debug and
// never included in error messages returned to the caller.
//
// SPEC: MCP 2025-11-25 requires progress values to strictly increase between
// notifications for the same token. The Tracker enforces this invariant by
// dropping non-monotonic Update calls (logged at Debug level), so misbehaving
// callers cannot violate the protocol contract.
package progress

import (
	"context"
	"log/slog"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// progressState carries per-Tracker mutable state. Stored behind a pointer so
// that copies of a Tracker value share the same monotonic counter.
type progressState struct {
	mu      sync.Mutex
	last    float64
	started bool
}

// Tracker sends progress notifications to the client for a single tool call.
// It is safe to call methods on a zero-value or inactive Tracker (all calls
// become no-ops), so callers never need nil-checks.
//
// Tracker values are safe to copy and share across goroutines: copies share
// the same underlying monotonic-progress state via a pointer field.
type Tracker struct {
	session *mcp.ServerSession
	token   any
	state   *progressState

	// scaled reports that this Tracker rewrites what its callers pass into a
	// shared scale, set by [Tracker.OnScale]. base is added to every progress
	// value and outerTotal replaces every total.
	scaled     bool
	base       float64
	outerTotal float64
}

// OnScale returns a Tracker that reports on a caller-chosen scale while sharing
// this one's monotonic state.
//
// It exists because "the progress value MUST increase with each notification"
// is a property of one progress token, not of one Tracker, and a token belongs
// to the whole tool call. A sub-step that measures itself in its own units —
// bytes of one file, while the step above counts files — cannot be made to
// satisfy that by guarding each counter separately: the two interleave on the
// wire and the sequence goes backwards. The guard in Update only ever compared
// a Tracker against itself, so it could not see it.
//
// The fix is one scale rather than one guard. The sub-step keeps reporting its
// own numbers, and they are placed inside the outer measure here: base is where
// this sub-step starts, outerTotal is the whole job. The returned Tracker shares
// the same *progressState, so what the client receives is a single increasing
// series with a stable total.
func (t Tracker) OnScale(base, outerTotal float64) Tracker {
	t.scaled = true
	t.base = base
	t.outerTotal = outerTotal
	return t
}

// FromRequest extracts the progress token from a CallToolRequest and returns
// a Tracker bound to that request's session. If the request has no progress
// token, no valid session, or the session is not initialized, the returned
// Tracker is inactive (all methods are no-ops).
func FromRequest(req *mcp.CallToolRequest) Tracker {
	if req == nil || req.Params == nil || req.Session == nil {
		return Tracker{}
	}
	if req.Session.InitializeParams() == nil {
		return Tracker{}
	}
	token := req.Params.GetProgressToken()
	if token == nil {
		return Tracker{}
	}
	return Tracker{
		session: req.Session,
		token:   token,
		state:   &progressState{},
	}
}

// IsActive returns true if the Tracker has both a valid session and a progress
// token, meaning it can send notifications.
func (t Tracker) IsActive() bool {
	return t.session != nil && t.token != nil
}

// Update sends a progress notification with explicit progress and total values.
// If the Tracker is inactive or the context is canceled, it silently does nothing.
// Errors from the notification are logged but never propagated — a failed progress
// notification must not abort the tool operation.
//
// SPEC: MCP 2025-11-25 mandates that progress values strictly increase. Calls
// where progress is less than or equal to a previously sent value are dropped
// (logged at Debug level) to preserve the protocol invariant.
func (t Tracker) Update(ctx context.Context, progress, total float64, message string) {
	if !t.IsActive() {
		return
	}
	if err := ctx.Err(); err != nil {
		return
	}
	if t.scaled {
		// Translated before the monotonic check, so the guard compares what
		// the client will actually receive.
		progress += t.base
		total = t.outerTotal
	}
	if t.state != nil {
		// Hold the mutex across NotifyProgress so concurrent callers cannot
		// reorder notifications on the wire even when state updates were
		// serialized correctly. The MCP "strictly increasing" invariant is
		// observed by what the client receives, not by the local last value,
		// so the send must stay inside the critical section.
		t.state.mu.Lock()
		defer t.state.mu.Unlock()
		if t.state.started && progress <= t.state.last {
			slog.DebugContext(ctx, "progress: dropping non-monotonic update",
				"previous", t.state.last, "attempted", progress)
			return
		}
		t.state.last = progress
		t.state.started = true
	}
	params := &mcp.ProgressNotificationParams{
		ProgressToken: t.token,
		Progress:      progress,
		Total:         total,
		Message:       message,
	}
	if err := t.session.NotifyProgress(ctx, params); err != nil {
		slog.DebugContext(ctx, "failed to send progress notification", "error", err)
	}
}

// Step is a convenience that reports progress as step/total (1-based step index).
// Example: Step(ctx, 1, 3, "Fetching MR details...") sends progress=0, total=3.
func (t Tracker) Step(ctx context.Context, step, total int, message string) {
	t.Update(ctx, float64(step-1), float64(total), message)
}

// Done sends a final progress notification reporting completion (progress==total).
// It is a convenience for callers that report intermediate steps via [Tracker.Step]
// (which uses zero-based progress) and want a clean 100% completion notification.
// If the Tracker is inactive or total is non-positive, Done is a no-op.
func (t Tracker) Done(ctx context.Context, total float64, message string) {
	if total <= 0 {
		return
	}
	t.Update(ctx, total, total, message)
}
