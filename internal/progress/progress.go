// Package progress provides a Tracker for sending MCP progress notifications
// to the client during long-running tool operations.
//
// SECURITY: Progress tokens are opaque values provided by the client.
// They are forwarded as-is — never logged at a level above Debug and
// never included in error messages returned to the caller.
package progress

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tracker sends progress notifications to the client for a single tool call.
// It is safe to call methods on a zero-value or inactive Tracker (all calls
// become no-ops), so callers never need nil-checks.
type Tracker struct {
	session *mcp.ServerSession
	token   any
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
func (t Tracker) Update(ctx context.Context, progress, total float64, message string) {
	if !t.IsActive() {
		return
	}
	if err := ctx.Err(); err != nil {
		return
	}
	params := &mcp.ProgressNotificationParams{
		ProgressToken: t.token,
		Progress:      progress,
		Total:         total,
		Message:       message,
	}
	if err := t.session.NotifyProgress(ctx, params); err != nil {
		slog.Debug("failed to send progress notification", "error", err)
	}
}

// Step is a convenience that reports progress as step/total (1-based step index).
// Example: Step(ctx, 1, 3, "Fetching MR details...") sends progress=0, total=3.
func (t Tracker) Step(ctx context.Context, step, total int, message string) {
	t.Update(ctx, float64(step-1), float64(total), message)
}
