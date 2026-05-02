package toolutil

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/elicitation"
)

// IsYOLOMode returns true when destructive action confirmation should be
// skipped entirely. Checks the YOLO_MODE and AUTOPILOT environment variables.
// Any truthy value (1, true, yes — case-insensitive) enables the mode.
func IsYOLOMode() bool {
	return isTruthy(os.Getenv("YOLO_MODE")) || isTruthy(os.Getenv("AUTOPILOT"))
}

// isTruthy returns true for common truthy string values.
func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// ConfirmDestructiveAction checks whether a destructive action should proceed.
// The confirmation flow is:
//
//  1. YOLO_MODE / AUTOPILOT env var → skip confirmation entirely
//  2. Explicit "confirm": true in params → skip confirmation
//  3. MCP elicitation supported → ask user interactively
//  4. Elicitation unsupported and no confirm param → return error prompting
//     the caller to re-send with confirm: true
//
// Returns nil if the action should proceed. Returns a non-nil *mcp.CallToolResult
// if the action was canceled or requires explicit confirmation.
func ConfirmDestructiveAction(ctx context.Context, req *mcp.CallToolRequest, params map[string]any, message string) *mcp.CallToolResult {
	tool := ""
	if req != nil {
		tool = req.Params.Name
	}

	if IsYOLOMode() {
		slog.Debug("destructive action auto-confirmed (YOLO mode)", "tool", tool)
		return nil
	}

	if hasExplicitConfirm(params) {
		slog.Debug("destructive action confirmed via explicit param", "tool", tool)
		return nil
	}

	return ConfirmAction(ctx, req, message)
}

// hasExplicitConfirm checks whether params contains "confirm": true.
func hasExplicitConfirm(params map[string]any) bool {
	if params == nil {
		return false
	}
	v, ok := params["confirm"]
	if !ok {
		return false
	}
	switch c := v.(type) {
	case bool:
		return c
	case string:
		return isTruthy(c)
	default:
		return false
	}
}

// ConfirmAction uses MCP elicitation to ask the user for confirmation before
// a destructive action. Returns nil if the user confirmed or elicitation is
// unsupported (fallback: action proceeds). Returns a non-error tool result
// if the user declined or canceled.
func ConfirmAction(ctx context.Context, req *mcp.CallToolRequest, message string) *mcp.CallToolResult {
	tool := ""
	if req != nil {
		tool = req.Params.Name
	}

	ec := elicitation.FromRequest(req)
	if !ec.IsSupported() {
		return nil
	}
	confirmed, err := ec.Confirm(ctx, message)
	if err != nil {
		if errors.Is(err, elicitation.ErrDeclined) || errors.Is(err, elicitation.ErrCancelled) {
			slog.Info("destructive action canceled by user", "tool", tool)
			return CancelledResult("Operation canceled by user.")
		}
		return nil
	}
	if !confirmed {
		slog.Info("destructive action denied by user", "tool", tool)
		return CancelledResult("Operation canceled by user.")
	}
	slog.Debug("destructive action confirmed by user", "tool", tool)
	return nil
}

// CancelledResult returns a non-error tool result indicating the user canceled.
func CancelledResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: message},
		},
	}
}
