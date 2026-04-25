// Package logging provides MCP protocol-level logging via ServerSession.
// It sends structured log messages to connected clients for server monitoring,
// complementing the stderr-based slog logging already in place.
package logging

import (
	"context"
	"log/slog"
	"maps"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const loggerName = "gitlab-mcp-server"

// SessionLogger wraps a *mcp.ServerSession and provides convenience
// methods for sending MCP protocol log messages to the connected client.
//
// SECURITY: All data passed to log methods is sent to the MCP client.
// Never pass secrets (tokens, passwords, credentials) or structs that contain them.
type SessionLogger struct {
	session *mcp.ServerSession
}

// NewSessionLogger creates a SessionLogger from an explicit ServerSession.
// Returns nil if session is nil.
func NewSessionLogger(session *mcp.ServerSession) *SessionLogger {
	if session == nil {
		return nil
	}
	return &SessionLogger{session: session}
}

// FromToolRequest extracts the ServerSession from a CallToolRequest
// and returns a SessionLogger. Returns nil if the request, session, or
// client initialization params are nil (session not yet initialized).
func FromToolRequest(req *mcp.CallToolRequest) *SessionLogger {
	if req == nil || req.Session == nil {
		return nil
	}
	if req.Session.InitializeParams() == nil {
		return nil
	}
	return &SessionLogger{session: req.Session}
}

// log sends a message at the given level. Errors are logged to stderr only.
func (l *SessionLogger) log(ctx context.Context, level mcp.LoggingLevel, message string, data any) {
	if l == nil {
		return
	}
	logData := buildLogData(message, data)
	err := l.session.Log(ctx, &mcp.LoggingMessageParams{
		Level:  level,
		Logger: loggerName,
		Data:   logData,
	})
	if err != nil {
		slog.Debug("mcp log send failed", "level", level, "error", err)
	}
}

// Debug sends a debug-level log message to the client.
// The data parameter is sent to the MCP client as-is; do not include secrets.
func (l *SessionLogger) Debug(ctx context.Context, message string, data any) {
	l.log(ctx, "debug", message, data)
}

// Info sends an info-level log message to the client.
// The data parameter is sent to the MCP client as-is; do not include secrets.
func (l *SessionLogger) Info(ctx context.Context, message string, data any) {
	l.log(ctx, "info", message, data)
}

// Notice sends a notice-level log message to the client (RFC 5424).
// The data parameter is sent to the MCP client as-is; do not include secrets.
func (l *SessionLogger) Notice(ctx context.Context, message string, data any) {
	l.log(ctx, "notice", message, data)
}

// Warning sends a warning-level log message to the client.
// The data parameter is sent to the MCP client as-is; do not include secrets.
func (l *SessionLogger) Warning(ctx context.Context, message string, data any) {
	l.log(ctx, "warning", message, data)
}

// Error sends an error-level log message to the client.
// The data parameter is sent to the MCP client as-is; do not include secrets.
func (l *SessionLogger) Error(ctx context.Context, message string, data any) {
	l.log(ctx, "error", message, data)
}

// Critical sends a critical-level log message to the client (RFC 5424).
// The data parameter is sent to the MCP client as-is; do not include secrets.
func (l *SessionLogger) Critical(ctx context.Context, message string, data any) {
	l.log(ctx, "critical", message, data)
}

// Alert sends an alert-level log message to the client (RFC 5424).
// The data parameter is sent to the MCP client as-is; do not include secrets.
func (l *SessionLogger) Alert(ctx context.Context, message string, data any) {
	l.log(ctx, "alert", message, data)
}

// Emergency sends an emergency-level log message to the client (RFC 5424).
// The data parameter is sent to the MCP client as-is; do not include secrets.
func (l *SessionLogger) Emergency(ctx context.Context, message string, data any) {
	l.log(ctx, "emergency", message, data)
}

// LogToolCall sends a structured tool execution log to the client.
// It includes tool name, duration, and success/failure status.
// This complements the stderr-based logToolCall in the tools package.
func (l *SessionLogger) LogToolCall(ctx context.Context, tool string, start time.Time, err error) {
	if l == nil {
		return
	}
	duration := time.Since(start)
	data := map[string]any{
		"tool":     tool,
		"duration": duration.String(),
	}
	if err != nil {
		data["status"] = "error"
		data["error"] = err.Error()
		l.log(ctx, "error", "tool call failed: "+tool, data)
		return
	}
	data["status"] = "ok"
	l.log(ctx, "info", "tool call completed: "+tool, data)
}

// buildLogData creates a structured log payload.
// If data is nil, the message string is used as the payload.
// If data is a map, the message is merged into a shallow copy under the
// "message" key (the caller's map is never mutated).
// Otherwise both are wrapped in a new map.
// SECURITY: The returned value is sent to the MCP client; callers must not include secrets.
func buildLogData(message string, data any) any {
	if data == nil {
		return message
	}
	if m, ok := data.(map[string]any); ok {
		cp := make(map[string]any, len(m)+1)
		maps.Copy(cp, m)
		cp["message"] = message
		return cp
	}
	return map[string]any{
		"message": message,
		"data":    data,
	}
}
