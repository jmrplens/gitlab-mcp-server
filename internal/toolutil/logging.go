package toolutil

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// LogToolCall logs a structured message after a tool handler completes.
// It records the tool name, elapsed duration, and any error that occurred.
func LogToolCall(tool string, start time.Time, err error) {
	duration := time.Since(start)
	switch {
	case wasCancelled(err):
		slog.Info("tool call canceled", "tool", tool, "duration", duration, "cause", err)
	case err != nil:
		slog.Error("tool call failed", "tool", tool, "duration", duration, "error", err)
	default:
		slog.Info("tool call completed", "tool", tool, "duration", duration)
	}
}

// wasCancelled reports whether a tool call ended because its caller went away
// rather than because anything went wrong.
//
// A client canceling a request, or a deadline it set expiring, is the protocol
// working: "Implementations SHOULD log cancellation reasons for debugging", and
// logging them at ERROR says the opposite of what happened. It also fills an
// operator's error dashboard with entries nobody can act on — a long list on
// stdio, where canceling a slow call is ordinary.
//
// The reason a client sent with the cancellation cannot be logged: go-sdk reads
// CancelledParams.RequestID and discards Reason before any application code
// runs, so what is left to record is the outcome and how long it took.
func wasCancelled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// LogToolCallAll logs a tool call to stderr (slog). It is the standard logging
// function for all tool handlers. When the request contains authenticated user
// identity (any mode), it includes the user in the log output for audit trail
// purposes.
func LogToolCallAll(ctx context.Context, req *mcp.CallToolRequest, tool string, start time.Time, err error) {
	user := ResolveIdentity(ctx, req)
	if user.IsAuthenticated() {
		logToolCallWithUser(tool, start, err, user)
	} else {
		LogToolCall(tool, start, err)
	}
}

// logToolCallWithUser logs a tool call including the authenticated user identity.
func logToolCallWithUser(tool string, start time.Time, err error, user UserIdentity) {
	duration := time.Since(start)
	switch {
	case wasCancelled(err):
		slog.Info("tool call canceled", "tool", tool, "duration", duration,
			"user", user.Username, "user_id", user.UserID, "cause", err)
	case err != nil:
		slog.Error("tool call failed", "tool", tool, "duration", duration,
			"user", user.Username, "user_id", user.UserID, "error", err)
	default:
		slog.Info("tool call completed", "tool", tool, "duration", duration,
			"user", user.Username, "user_id", user.UserID)
	}
}
