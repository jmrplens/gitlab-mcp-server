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

	"github.com/google/jsonschema-go/jsonschema"
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
// and the JSON Schema describing the action's typed output (when known).
type ActionRoute struct {
	Handler     ActionFunc
	Destructive bool
	// OutputSchema describes the shape of the action's structured result.
	// Populated by the typed wrappers (RouteAction[T,R], DestructiveAction[T,R],
	// RouteActionWithRequest[T,R], DestructiveActionWithRequest[T,R]) using the
	// generic R type parameter. Nil for type-erased Route/DestructiveRoute and
	// for void variants. Used by MetaToolOutputSchema to build a discriminated
	// per-action output schema for the parent meta-tool.
	OutputSchema *jsonschema.Schema
}

// ActionMap maps action names to their route definitions (handler + metadata).
type ActionMap map[string]ActionRoute

// Route creates a non-destructive ActionRoute.
func Route(fn ActionFunc) ActionRoute {
	return ActionRoute{Handler: fn, Destructive: false}
}

// RouteTyped creates a non-destructive ActionRoute and captures the JSON
// Schema for the static result type R. Use this when the underlying handler
// must remain a type-erased ActionFunc (e.g. it returns a sentinel sum type
// like sampling unsupported), but you still want the meta-tool to advertise
// the typed shape in its OutputSchema for LLMs.
func RouteTyped[R any](fn ActionFunc) ActionRoute {
	return ActionRoute{Handler: fn, Destructive: false, OutputSchema: schemaFor[R]()}
}

// DestructiveRoute creates a destructive ActionRoute that will trigger
// user confirmation before execution.
func DestructiveRoute(fn ActionFunc) ActionRoute {
	return ActionRoute{Handler: fn, Destructive: true}
}

// DestructiveRouteTyped is the destructive equivalent of RouteTyped.
func DestructiveRouteTyped[R any](fn ActionFunc) ActionRoute {
	return ActionRoute{Handler: fn, Destructive: true, OutputSchema: schemaFor[R]()}
}

// schemaFor returns the JSON Schema for type R, post-processed for use as a
// branch in a meta-tool output schema. The schema is generated via
// jsonschema.For[R] which by default sets additionalProperties:false on
// structs; we relax that constraint at the *root* of the branch so that the
// meta-tool dispatcher can decorate the structured content with extra fields
// (e.g. "next_steps" injected by enrichWithHints) without failing SDK output
// validation. Nested object schemas keep their original additionalProperties
// settings so type safety inside child structures is preserved.
//
// Returns nil if schema generation fails (the route then contributes no
// branch to the union schema).
func schemaFor[R any]() *jsonschema.Schema {
	s, err := jsonschema.For[R](nil)
	if err != nil || s == nil {
		return nil
	}
	relaxRootAdditionalProperties(s)
	return s
}

// relaxRootAdditionalProperties clears AdditionalProperties on the schema's
// root only, and only when it explicitly forbids extras (i.e. is set to
// schema-false / `additionalProperties: false`). Schemas with
// `additionalProperties: true` or schema-valued constraints, and the
// AdditionalProperties of nested objects, are left untouched.
//
// This is sufficient because the dispatcher injects "next_steps" only at the
// top level of the marshaled result, so a recursive walk would needlessly
// weaken the contract of nested structures.
func relaxRootAdditionalProperties(s *jsonschema.Schema) {
	if s == nil || s.AdditionalProperties == nil {
		return
	}
	if schemaForbidsAdditional(s.AdditionalProperties) {
		s.AdditionalProperties = nil
	}
}

// schemaForbidsAdditional reports whether a schema is the trivially-false
// schema (`{"not":{}}` in JSON Schema 2020-12 vocabulary, or any schema
// whose Not is non-nil with no other constraints). This is what
// jsonschema-go produces for `additionalProperties: false`.
func schemaForbidsAdditional(s *jsonschema.Schema) bool {
	if s == nil {
		return false
	}
	if s.Not != nil {
		return true
	}
	return false
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

// RouteAction wraps a typed function as a non-destructive ActionRoute and
// captures the JSON Schema of the result type R for the meta-tool's union
// output schema.
func RouteAction[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) (R, error)) ActionRoute {
	r := Route(WrapAction(client, fn))
	r.OutputSchema = schemaFor[R]()
	return r
}

// RouteVoidAction wraps a typed void function as a non-destructive ActionRoute.
// Void actions contribute no branch to the meta-tool output schema.
func RouteVoidAction[T any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) error) ActionRoute {
	return Route(WrapVoidAction(client, fn))
}

// RouteActionWithRequest wraps a typed function that needs the MCP request as
// a non-destructive ActionRoute and captures R's schema.
func RouteActionWithRequest[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error)) ActionRoute {
	r := Route(WrapActionWithRequest(client, fn))
	r.OutputSchema = schemaFor[R]()
	return r
}

// DestructiveAction wraps a typed function as a destructive ActionRoute and
// captures R's schema.
func DestructiveAction[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) (R, error)) ActionRoute {
	r := DestructiveRoute(WrapAction(client, fn))
	r.OutputSchema = schemaFor[R]()
	return r
}

// DestructiveVoidAction wraps a typed void function as a destructive ActionRoute.
func DestructiveVoidAction[T any](client *gitlabclient.Client, fn func(ctx context.Context, client *gitlabclient.Client, input T) error) ActionRoute {
	return DestructiveRoute(WrapVoidAction(client, fn))
}

// DestructiveActionWithRequest wraps a typed function that needs the MCP request
// as a destructive ActionRoute and captures R's schema.
func DestructiveActionWithRequest[T any, R any](client *gitlabclient.Client, fn func(ctx context.Context, req *mcp.CallToolRequest, client *gitlabclient.Client, input T) (R, error)) ActionRoute {
	r := DestructiveRoute(WrapActionWithRequest(client, fn))
	r.OutputSchema = schemaFor[R]()
	return r
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
		// Ensure structured content is always a JSON object so it satisfies
		// MetaToolOutputSchema's `type: "object"` constraint. Void actions
		// (and any handler that returns nil) would otherwise marshal to
		// `null` and be rejected by the SDK output validator.
		structured := enrichWithHints(result, callResult)
		if structured == nil {
			structured = struct{}{}
		}
		return callResult, structured, err
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
				"description": "Action to perform. See the tool description for available actions and their parameters.",
			},
			"params": map[string]any{
				"type":                 "object",
				"description":          "Action-specific parameters as a JSON object. See the tool description for required/optional fields per action.",
				"additionalProperties": true,
			},
		},
		"required":             []any{"action"},
		"additionalProperties": false,
	}
}

// MetaToolOutputSchema builds a JSON Schema describing the union of possible
// outputs for a meta-tool. For each route in the map that has a typed result
// (captured via RouteAction[T,R] and friends, or RouteTyped[R] for type-erased
// handlers), a branch is added to the schema's "anyOf" array, titled with the
// action name. Type-erased routes (Route / DestructiveRoute without a
// captured R) and void variants contribute no typed branch.
//
// A permissive "any_action_result" branch is *always* appended so that
// SDK output validation tolerates results that do not match any typed
// branch. Such results occur when:
//   - a route is type-erased (Route / DestructiveRoute);
//   - a void action returns nil (the dispatcher serializes this as `{}`);
//   - a typed handler returns a sentinel sum-type value (e.g. the
//     samplingUnsupportedOutput empty struct used by gitlab_analyze when the
//     client does not advertise sampling capability).
//
// The typed branches still serve as machine-readable documentation for LLMs
// and scanners; the fallback only widens the validator, not the LLM hints.
//
// The returned schema is suitable for use as Tool.OutputSchema. Returns
// untyped nil if no routes carry an output schema, in which case the caller
// should leave OutputSchema unset.
//
// Validation note: each typed branch has root-level additionalProperties
// relaxed (see schemaFor / relaxRootAdditionalProperties) so SDK output
// validation tolerates the "next_steps" field that the dispatcher injects at
// the top level of structured results via enrichWithHints. The fallback
// branch is intentionally permissive (object with no constraints).
func MetaToolOutputSchema(routes ActionMap) any {
	if len(routes) == 0 {
		return nil
	}
	names := make([]string, 0, len(routes))
	for name, r := range routes {
		if r.OutputSchema == nil {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)

	branches := make([]*jsonschema.Schema, 0, len(names)+1)
	for _, name := range names {
		s := routes[name].OutputSchema
		// Copy to avoid mutating the cached schema; only Title needs setting.
		branch := *s
		branch.Title = name
		branches = append(branches, &branch)
	}
	// Always append a permissive fallback so the SDK does not reject results
	// that fall outside the typed branches (void actions, sentinel values,
	// type-erased routes). LLMs read the typed branches; the SDK uses the
	// fallback only as a last resort.
	branches = append(branches, &jsonschema.Schema{
		Title:       "any_action_result",
		Description: "Permissive fallback that accepts any object. Used for void actions returning {}, sentinel values, and type-erased routes.",
		Type:        "object",
	})

	return &jsonschema.Schema{
		Type:        "object",
		Description: "Action-dependent output. The concrete shape depends on the requested action; see the per-branch titles in anyOf for the typed schemas. The dispatcher may also inject a top-level \"next_steps\" array of LLM hints, which is tolerated by every branch.",
		AnyOf:       branches,
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
