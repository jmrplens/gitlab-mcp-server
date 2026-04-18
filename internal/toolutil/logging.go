// logging.go provides structured logging helpers for tool handlers.
// It logs to both stderr (via slog) and to the MCP client (via protocol-level
// logging notifications).

package toolutil

import (
	"context"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/logging"
)

// LogToolCall logs a structured message after a tool handler completes.
// It records the tool name, elapsed duration, and any error that occurred.
func LogToolCall(tool string, start time.Time, err error) {
	duration := time.Since(start)
	if err != nil {
		slog.Error("tool call failed", "tool", tool, "duration", duration, "error", err)
		return
	}
	slog.Info("tool call completed", "tool", tool, "duration", duration)
}

// LogToolCallAll logs to both stderr (slog) and the MCP client (protocol logging).
// It is the standard logging function for all tool handlers. When the request
// contains authenticated user identity (any mode), it includes the user in the
// log output for audit trail purposes.
func LogToolCallAll(ctx context.Context, req *mcp.CallToolRequest, tool string, start time.Time, err error) {
	user := ResolveIdentity(ctx, req)
	if user.IsAuthenticated() {
		logToolCallWithUser(tool, start, err, user)
	} else {
		LogToolCall(tool, start, err)
	}
	logging.FromToolRequest(req).LogToolCall(ctx, tool, start, err)
}

// logToolCallWithUser logs a tool call including the authenticated user identity.
func logToolCallWithUser(tool string, start time.Time, err error, user UserIdentity) {
	duration := time.Since(start)
	if err != nil {
		slog.Error("tool call failed", "tool", tool, "duration", duration, "user", user.Username, "user_id", user.UserID, "error", err)
		return
	}
	slog.Info("tool call completed", "tool", tool, "duration", duration, "user", user.Username, "user_id", user.UserID)
}
