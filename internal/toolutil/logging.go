package toolutil

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

// Reasons a call was declined before its handler ran, recorded as the reason
// attribute of [LogToolRefusal].
//
// They are a closed set on purpose: an operator computing a refusal rate needs
// to group by something, and free text does not group.
const (
	RefusalSafeMode          = "safe_mode"
	RefusalNeedsConfirmation = "needs_confirmation"
	RefusalInvalidParams     = "invalid_params"
	RefusalUnknownAction     = "unknown_action"
	RefusalRateLimited       = "rate_limited"
)

// LogToolCallAll logs a tool call to stderr (slog). It is the standard logging
// function for all tool handlers. When the request contains authenticated user
// identity (any mode), it includes the user in the log output for audit trail
// purposes.
//
// result is what the handler returned, taken so the record can say whether the
// call actually succeeded. A handler that reports failure in its result rather
// than as a Go error, which [NotFoundResult] does at eighteen call sites, used
// to be logged as "tool call completed" with nothing distinguishing it from a
// call that worked. An operator could not compute an error rate from this stream,
// because the stream did not contain one.
func LogToolCallAll(ctx context.Context, req *mcp.CallToolRequest, tool string, start time.Time, result any, err error) {
	user := ResolveIdentity(ctx, req)
	if user.IsAuthenticated() {
		logToolCallWithUser(tool, start, resultIsError(result), err, user)
		return
	}
	logToolCall(tool, start, resultIsError(result), err)
}

// LogToolRefusal records a call the server declined to run.
//
// These paths returned an error result to the model and wrote nothing at all:
// safe-mode previews, unknown actions, missing parameters and unconfirmed
// destructive actions each returned before reaching [LogToolCallAll]. They are
// refusals rather than failures, so they are INFO, and they are exactly the
// events an operator most wants to count: a deployment refusing every third
// call because its clients have not learned the parameter shape looks identical
// to a healthy one from the logs.
//
// It is the second half of the observability item ADR-0011 already accepted:
// "add observability for find query, selected action, validation failure,
// policy block, and destructive confirmation events". Only the destructive one
// was delivered.
func LogToolRefusal(ctx context.Context, req *mcp.CallToolRequest, tool, reason string) {
	user := ResolveIdentity(ctx, req)
	if user.IsAuthenticated() {
		slog.Info("tool call refused", "tool", tool, "reason", reason,
			"user", user.Username, "user_id", user.UserID)
		return
	}
	slog.Info("tool call refused", "tool", tool, "reason", reason)
}

// resultIsError reports whether a handler's return value is an error result.
//
// The dispatchers log before formatting, so what they hold is the raw value.
// When a handler has already built a [mcp.CallToolResult], which is how
// [NotFoundResult] reports a 404 without raising a Go error, the flag is right
// there on it.
func resultIsError(result any) bool {
	callResult, ok := result.(*mcp.CallToolResult)
	return ok && callResult != nil && callResult.IsError
}

// logToolCall writes the anonymous form of the record: the tool name, the
// elapsed duration, and how the call ended.
//
// It replaced an exported LogToolCall of the same shape without the isError
// argument. Nothing outside this package called it, and leaving both would have
// left one that could not say whether the call actually succeeded.
func logToolCall(tool string, start time.Time, isError bool, err error) {
	duration := time.Since(start)
	switch {
	case wasCancelled(err):
		slog.Info("tool call canceled", "tool", tool, "duration", duration, "cause", err)
	case err != nil:
		slog.Error("tool call failed", "tool", tool, "duration", duration, "error", err)
	default:
		slog.Info("tool call completed", "tool", tool, "duration", duration, "is_error", isError)
	}
}

// logToolCallWithUser logs a tool call including the authenticated user identity.
func logToolCallWithUser(tool string, start time.Time, isError bool, err error, user UserIdentity) {
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
			"user", user.Username, "user_id", user.UserID, "is_error", isError)
	}
}
