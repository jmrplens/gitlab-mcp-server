// metatool.go implements the meta-tool dispatch pattern that routes
// a single MCP tool call to one of several action handlers based on
// the "action" parameter. It provides generic wrappers for typed and
// void handlers, JSON param deserialization, and action validation.

package toolutil

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
)

// maxInt is the maximum int value; used for overflow-safe capacity calculations.
const maxInt = int(math.MaxInt)

// MetaToolInput is the common input for all meta-tools.
// The LLM sends an action name and a params object; the dispatcher
// routes to the underlying handler function and deserializes params
// into the action-specific input struct.
type MetaToolInput struct {
	Action string         `json:"action" jsonschema:"Action to perform. See the tool description for available actions and their parameters."`
	Params map[string]any `json:"params,omitempty" jsonschema:"Action-specific parameters as a JSON object. See the tool description for required/optional fields per action."`
}

// ActionFunc is a handler that receives raw params and returns a result or error.
type ActionFunc func(ctx context.Context, params map[string]any) (any, error)

// ActionRoute pairs an action handler with metadata about its behavior.
// Used by meta-tools to carry per-route destructive classification
// without string parsing.
type ActionRoute struct {
	Handler     ActionFunc
	Destructive bool
}

// ActionMap maps action names to their route definitions (handler + metadata).
type ActionMap map[string]ActionRoute

// Route creates a non-destructive ActionRoute.
func Route(fn ActionFunc) ActionRoute {
	return ActionRoute{Handler: fn, Destructive: false}
}

// DestructiveRoute creates a destructive ActionRoute that will trigger
// user confirmation before execution.
func DestructiveRoute(fn ActionFunc) ActionRoute {
	return ActionRoute{Handler: fn, Destructive: true}
}

// requestContextKey is the context key for storing the MCP request in
// action handler contexts. Used by WrapActionWithRequest to pass the
// CallToolRequest to handlers that need it (e.g., for progress tracking).
type requestContextKey struct{}

// ContextWithRequest returns a derived context carrying the MCP request.
func ContextWithRequest(ctx context.Context, req *mcp.CallToolRequest) context.Context {
	return context.WithValue(ctx, requestContextKey{}, req)
}

// RequestFromContext extracts the MCP request from a context, or nil if absent.
func RequestFromContext(ctx context.Context) *mcp.CallToolRequest {
	req, _ := ctx.Value(requestContextKey{}).(*mcp.CallToolRequest)
	return req
}

// UnmarshalParams re-serializes params map to JSON and deserializes into T.
// LLMs frequently send numeric values as JSON strings (e.g. "17" instead of 17).
// When standard unmarshalling fails, this function retries after coercing
// string values that look like integers or floats into actual numbers.
func UnmarshalParams[T any](params map[string]any) (T, error) {
	var input T
	data, err := json.Marshal(params)
	if err != nil {
		return input, fmt.Errorf("invalid params: %w", err)
	}
	if err = json.Unmarshal(data, &input); err != nil {
		// Retry with numeric string coercion.
		coerced := coerceNumericStrings(params)
		data2, marshalErr := json.Marshal(coerced)
		if marshalErr != nil {
			return input, fmt.Errorf("invalid params for this action: %w", err)
		}
		if json.Unmarshal(data2, &input) != nil {
			// Return the original error for a clearer message.
			return input, fmt.Errorf("invalid params for this action: %w", err)
		}
		return input, nil
	}
	return input, nil
}

// coerceNumericStrings returns a shallow copy of params where string values
// that parse as int64 or float64 are replaced with their numeric equivalents.
// This handles the common LLM behavior of sending numbers as JSON strings.
func coerceNumericStrings(params map[string]any) map[string]any {
	result := make(map[string]any, len(params))
	for k, v := range params {
		s, ok := v.(string)
		if !ok {
			result[k] = v
			continue
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			result[k] = n
			continue
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			result[k] = f
			continue
		}
		result[k] = v
	}
	return result
}

// WrapAction wraps a typed handler (input T -> output R) into a generic ActionFunc.
func WrapAction[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) (R, error)) ActionFunc {
	return func(ctx context.Context, params map[string]any) (any, error) {
		input, err := UnmarshalParams[T](params)
		if err != nil {
			return nil, err
		}
		return fn(ctx, client, input)
	}
}

// WrapVoidAction wraps a typed handler that returns only error.
func WrapVoidAction[T any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) error) ActionFunc {
	return func(ctx context.Context, params map[string]any) (any, error) {
		input, err := UnmarshalParams[T](params)
		if err != nil {
			return nil, err
		}
		return nil, fn(ctx, client, input)
	}
}

// WrapActionWithRequest wraps a handler that also requires the MCP request
// (e.g., for progress tracking). The request is extracted from context via
// RequestFromContext; if absent, nil is passed.
func WrapActionWithRequest[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error)) ActionFunc {
	return func(ctx context.Context, params map[string]any) (any, error) {
		input, err := UnmarshalParams[T](params)
		if err != nil {
			return nil, err
		}
		return fn(ctx, RequestFromContext(ctx), client, input)
	}
}

// RouteAction wraps a typed function as a non-destructive ActionRoute.
func RouteAction[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) (R, error)) ActionRoute {
	return Route(WrapAction(client, fn))
}

// RouteVoidAction wraps a typed void function as a non-destructive ActionRoute.
func RouteVoidAction[T any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) error) ActionRoute {
	return Route(WrapVoidAction(client, fn))
}

// RouteActionWithRequest wraps a typed function that needs the MCP request as a non-destructive ActionRoute.
func RouteActionWithRequest[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error)) ActionRoute {
	return Route(WrapActionWithRequest(client, fn))
}

// DestructiveAction wraps a typed function as a destructive ActionRoute.
func DestructiveAction[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) (R, error)) ActionRoute {
	return DestructiveRoute(WrapAction(client, fn))
}

// DestructiveVoidAction wraps a typed void function as a destructive ActionRoute.
func DestructiveVoidAction[T any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) error) ActionRoute {
	return DestructiveRoute(WrapVoidAction(client, fn))
}

// DestructiveActionWithRequest wraps a typed function that needs the MCP request as a destructive ActionRoute.
func DestructiveActionWithRequest[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error)) ActionRoute {
	return DestructiveRoute(WrapActionWithRequest(client, fn))
}

// FormatResultFunc converts an action result into an MCP call tool result.
type FormatResultFunc func(any) *mcp.CallToolResult

// MakeMetaHandler creates a generic MCP tool handler that dispatches to action routes.
// The formatResult function converts the action result into an MCP response.
// If formatResult is nil, a default JSON formatter is used.
//
// Destructive actions (delete, remove, revoke, unprotect, etc.) are automatically
// intercepted with a user confirmation prompt via MCP elicitation before execution.
// Confirmation can be bypassed with YOLO_MODE/AUTOPILOT env vars or by passing
// "confirm": true in the action params.
func MakeMetaHandler(toolName string, routes ActionMap, formatResult FormatResultFunc) func(ctx context.Context, req *mcp.CallToolRequest, input MetaToolInput) (*mcp.CallToolResult, any, error) {
	if formatResult == nil {
		formatResult = defaultFormatResult
	}
	return func(ctx context.Context, req *mcp.CallToolRequest, input MetaToolInput) (*mcp.CallToolResult, any, error) {
		if input.Action == "" {
			return nil, nil, fmt.Errorf("%s: 'action' is required. Valid actions: %s", toolName, ValidActionsString(routes))
		}

		route, ok := routes[input.Action]
		if !ok {
			return nil, nil, fmt.Errorf("%s: unknown action %q. Valid actions: %s", toolName, input.Action, ValidActionsString(routes))
		}

		// Confirm destructive actions before execution using route metadata.
		if route.Destructive {
			msg := fmt.Sprintf("Confirm %s/%s? This action may be irreversible.", toolName, input.Action)
			if result := ConfirmDestructiveAction(ctx, req, input.Params, msg); result != nil {
				return result, nil, nil
			}
		}

		// Store the request in context so WrapActionWithRequest handlers can access it.
		actionCtx := ContextWithRequest(ctx, req)

		start := time.Now()
		result, err := route.Handler(actionCtx, input.Params)
		LogToolCallAll(ctx, req, fmt.Sprintf("%s/%s", toolName, input.Action), start, err)

		callResult := formatResult(result)
		return callResult, enrichWithHints(result, callResult), err
	}
}

// enrichWithHints extracts next-step hints from the Markdown content in
// callResult and merges them into the JSON result as a "next_steps" field.
// The returned json.RawMessage places next_steps as the first JSON field
// so that LLMs see actionable guidance before reading the full payload.
// If no hints exist, result is returned unchanged.
func enrichWithHints(result any, callResult *mcp.CallToolResult) any {
	if result == nil || callResult == nil {
		return result
	}
	var hints []string
	for _, c := range callResult.Content {
		tc, ok := c.(*mcp.TextContent)
		if !ok {
			continue
		}
		if h := ExtractHints(tc.Text); len(h) > 0 {
			hints = h
			break
		}
	}
	if len(hints) == 0 {
		return result
	}
	data, err := json.Marshal(result)
	if err != nil {
		return result
	}
	if len(data) == 0 || data[0] != '{' {
		return result
	}
	hintsData, err := json.Marshal(hints)
	if err != nil {
		return result
	}
	// Build JSON with next_steps as the first field so LLMs see guidance early.
	overhead := len(`"next_steps":,`)
	if len(data) > maxInt-overhead {
		return result
	}
	capacity := overhead + len(data)
	if len(hintsData) > maxInt-capacity {
		return result
	}
	capacity += len(hintsData)
	buf := make([]byte, 0, capacity)
	buf = append(buf, '{')
	buf = append(buf, `"next_steps":`...)
	buf = append(buf, hintsData...)
	if len(data) > 2 {
		buf = append(buf, ',')
		buf = append(buf, data[1:]...)
	} else {
		buf = append(buf, '}')
	}
	return json.RawMessage(buf)
}

// defaultFormatResult serializes the action result as JSON text content.
func defaultFormatResult(result any) *mcp.CallToolResult {
	if result == nil {
		return SuccessResult("ok")
	}
	data, err := json.Marshal(result)
	if err != nil {
		return SuccessResult(fmt.Sprintf("%v", result))
	}
	return SuccessResult(string(data))
}

// ValidActionsString returns a sorted, comma-separated list of action names.
func ValidActionsString(routes ActionMap) string {
	actions := make([]string, 0, len(routes))
	for k := range routes {
		actions = append(actions, k)
	}
	sort.Strings(actions)
	return strings.Join(actions, ", ")
}

// MetaToolSchema builds a JSON Schema for a meta-tool with the action field
// constrained to an enum of valid action names extracted from the routes map.
// Setting this as Tool.InputSchema ensures the LLM sees the exact list of
// valid actions in the schema, enabling first-try action selection.
func MetaToolSchema(routes ActionMap) map[string]any {
	actions := make([]string, 0, len(routes))
	for name := range routes {
		actions = append(actions, name)
	}
	sort.Strings(actions)

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        actions,
				"description": "Action to perform. Pick exactly one of the values in `enum`. Each action expects its own `params` object — see the tool description for the per-action parameter list.",
			},
			"params": map[string]any{
				"type":                 "object",
				"description":          "Action-specific parameters as a JSON object. Required and optional fields differ per action; consult this tool's description for the chosen action. Send only the fields documented for that action — unrelated keys are ignored by the underlying handler.",
				"additionalProperties": true,
			},
		},
		"required":             []any{"action"},
		"additionalProperties": false,
	}
}

// DeriveAnnotations computes tool-level MCP annotations from the route map.
// If any route is destructive, returns a copy of MetaAnnotations (DestructiveHint: true).
// If all routes are non-destructive, returns a copy of NonDestructiveMetaAnnotations.
// Each call returns a fresh copy to avoid aliasing the shared singletons.
func DeriveAnnotations(routes ActionMap) *mcp.ToolAnnotations {
	for _, r := range routes {
		if r.Destructive {
			cp := *MetaAnnotations
			return &cp
		}
	}
	cp := *NonDestructiveMetaAnnotations
	return &cp
}

// DeriveAnnotationsWithTitle returns route-derived annotations with Title set from the tool name.
func DeriveAnnotationsWithTitle(name string, routes ActionMap) *mcp.ToolAnnotations {
	a := DeriveAnnotations(routes)
	a.Title = TitleFromName(name)
	return a
}

// ReadOnlyMetaAnnotationsWithTitle returns a copy of ReadOnlyMetaAnnotations with Title set.
func ReadOnlyMetaAnnotationsWithTitle(name string) *mcp.ToolAnnotations {
	a := *ReadOnlyMetaAnnotations
	a.Title = TitleFromName(name)
	return &a
}
