package toolutil

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/elicitation"
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
//  4. Elicitation unsupported and no confirm param → fail closed: return an
//     error result prompting the caller to re-send with confirm: true
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

	// Fail closed when the client cannot prompt: without this, every client
	// that does not support elicitation (most non-interactive agents) would
	// execute destructive actions silently, while the dynamic surface's own
	// guard requires confirm=true. Mirrors that guard's contract.
	flow, err := elicitation.FlowFromRequest(req)
	if err != nil {
		slog.Warn("destructive confirmation state invalid", "tool", tool, "error", err)
		return ErrorResult(err.Error())
	}
	if !flow.IsSupported() {
		slog.Warn("blocked destructive action without explicit confirmation", "tool", tool)
		return ErrorResult(message +
			" The connected client cannot prompt for confirmation." +
			" Re-send with confirm=true only after the user explicitly approves this operation.")
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
// unsupported (fallback: action proceeds — destructive callers must go
// through [ConfirmDestructiveAction], which fails closed in that case).
// Returns an error tool result if the user declined or canceled. On sessions negotiated at protocol
// >= 2026-07-28 the confirmation travels as a multi round-trip input request
// (SEP-2322): the first pass returns an input-required result and the client
// retries the call with the user's answer attached.
func ConfirmAction(ctx context.Context, req *mcp.CallToolRequest, message string) *mcp.CallToolResult {
	tool := ""
	if req != nil {
		tool = req.Params.Name
	}

	flow, err := elicitation.FlowFromRequest(req)
	if err != nil {
		slog.Warn("destructive confirmation state invalid", "tool", tool, "error", err)
		return ErrorResult(err.Error())
	}
	if !flow.IsSupported() {
		return nil
	}
	confirmed, err := flow.Confirm(ctx, elicitation.ConfirmExchangeID, message)
	switch {
	case errors.Is(err, elicitation.ErrInputPending):
		slog.Debug("destructive confirmation pending client input", "tool", tool)
		return flow.InputRequiredResult()
	case errors.Is(err, elicitation.ErrDeclined), errors.Is(err, elicitation.ErrCancelled):
		slog.Info("destructive action canceled by user", "tool", tool)
		return CancelledResult("Operation canceled by user.")
	case err != nil:
		// Fail closed: a destructive action must never proceed on a
		// malformed or failed confirmation exchange.
		slog.Warn("destructive confirmation failed", "tool", tool, "error", err)
		return elicitation.ConfirmFailedResult(err)
	}
	if !confirmed {
		slog.Info("destructive action denied by user", "tool", tool)
		return CancelledResult("Operation canceled by user.")
	}
	slog.Debug("destructive action confirmed by user", "tool", tool)
	return nil
}

// CancelledResult returns an error tool result indicating the user canceled.
func CancelledResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: message},
		},
		// The operation was declined, so it produced none of the output its
		// schema describes. A tool that declares an outputSchema MUST return
		// structured results conforming to it, and a cancellation conforms to
		// nothing, so this is reported as an error result rather than as a
		// success carrying only prose.
		IsError: true,
	}
}

// InputRequiredResultFromError extracts the input-required tool result from a
// handler error produced by [elicitation.Flow.PendingError]. Surface
// dispatchers call this before logging so a pending multi round-trip exchange
// is returned to the client instead of being reported as a handler failure.
func InputRequiredResultFromError(err error) (*mcp.CallToolResult, bool) {
	if inputErr, ok := errors.AsType[*elicitation.InputRequiredError](err); ok {
		return inputErr.Result(), true
	}
	return nil, false
}

// ExplicitConfirmFromRequest reports whether the caller set the reserved
// confirm key on this tool call.
//
// The key never reaches a typed action input: [stripReservedKeys] removes it
// before strict unmarshalling so it cannot trip the unknown-field rejection. A
// handler that needs to know whether the caller confirmed — because its
// destructiveness depends on runtime state and cannot be declared on the route
// — must therefore read it from the raw arguments, which this does for both
// call shapes: flat on individual tools, and nested under params on the
// dispatcher surfaces.
func ExplicitConfirmFromRequest(req *mcp.CallToolRequest) bool {
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 {
		return false
	}
	var arguments map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &arguments); err != nil {
		return false
	}
	if hasExplicitConfirm(arguments) {
		return true
	}
	nested, ok := arguments["params"].(map[string]any)
	return ok && hasExplicitConfirm(nested)
}
