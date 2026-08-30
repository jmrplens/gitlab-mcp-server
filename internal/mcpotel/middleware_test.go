package mcpotel

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// newRecorder installs a real tracer provider that keeps finished spans in
// memory, and restores the globals afterwards.
//
// The SDK is used rather than a hand-rolled fake on purpose. Several of the
// rules this package implements are enforced inside the SDK and are invisible
// to a fake: SetStatus after End is a no-op, Ok outranks Error so it cannot be
// downgraded, and an attribute with an empty key is dropped silently. A fake
// would happily accept all three and the tests would prove nothing.
func newRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	previousTracer := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previousTracer)
		otel.SetTextMapPropagator(previousPropagator)
	})
	return recorder
}

// callToolRequest builds a tools/call the way the SDK actually delivers one.
//
// CallToolParamsRaw, with Arguments as json.RawMessage, not CallToolParams with
// a map. That distinction is not incidental: the SDK leaves argument decoding
// to the tool handler, so instrumentation reading a map would find nothing on
// every real request. Building the wrong shape here would have made these tests
// pass against code that records no action in production.
func callToolRequest(tool string, args, meta map[string]any) *mcp.CallToolRequest {
	var raw json.RawMessage
	if args != nil {
		encoded, err := json.Marshal(args)
		if err != nil {
			panic(err)
		}
		raw = encoded
	}
	params := &mcp.CallToolParamsRaw{Name: tool, Arguments: raw}
	if meta != nil {
		params.SetMeta(meta)
	}
	return &mcp.CallToolRequest{Params: params}
}

// attrOf reads one attribute off a recorded span.
func attrOf(span sdktrace.ReadOnlySpan, key attribute.Key) (attribute.Value, bool) {
	for _, kv := range span.Attributes() {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

// runOnce drives the middleware for one request and returns the span it made.
func runOnce(t *testing.T, opts Options, method string, req mcp.Request, res mcp.Result, handlerErr error) sdktrace.ReadOnlySpan {
	t.Helper()

	recorder := newRecorder(t)
	handler := Middleware(opts)(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return res, handlerErr
	})
	_, _ = handler(context.Background(), method, req)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want exactly 1", len(spans))
	}
	return spans[0]
}

// TestMiddleware_ToolCallSpanShape pins the whole contract for the common case,
// because each of these is a separate way to be silently wrong.
//
// The kind especially: omitting trace.WithSpanKind yields INTERNAL, which is
// coerced rather than rejected, so the trace contains no server span at all and
// a service map shows no inbound traffic while everything looks fine.
func TestMiddleware_ToolCallSpanShape(t *testing.T) {
	identifier := IdentifierFunc(func(string, any) (Identity, bool) {
		return Identity{ActionID: "issue.list", Domain: "issue"}, true
	})
	span := runOnce(t,
		Options{Identifier: identifier, Surface: "dynamic", Transport: "pipe"},
		"tools/call",
		callToolRequest("gitlab_execute_action", map[string]any{"action": "issue.list"}, nil),
		&mcp.CallToolResult{}, nil)

	if got := span.Name(); got != "tools/call gitlab_execute_action" {
		t.Errorf("span name = %q, want the convention's {method} {target}", got)
	}
	if got := span.SpanKind(); got != trace.SpanKindServer {
		t.Errorf("span kind = %v, want SERVER; INTERNAL is coerced silently and hides inbound traffic", got)
	}
	for key, want := range map[attribute.Key]string{
		AttrMCPMethodName:      "tools/call",
		AttrGenAIToolName:      "gitlab_execute_action",
		AttrGenAIOperationName: "execute_tool",
		AttrActionID:           "issue.list",
		AttrToolSurface:        "dynamic",
		AttrNetworkTransport:   "pipe",
	} {
		value, ok := attrOf(span, key)
		if !ok {
			t.Errorf("%s is absent", key)
			continue
		}
		if value.AsString() != want {
			t.Errorf("%s = %q, want %q", key, value.AsString(), want)
		}
	}
}

// TestMiddleware_ActionIsRecordedWhereTheToolNameIsNot is the reason the
// identifier exists, stated as an assertion.
//
// On the default surface every tools/call carries gen_ai.tool.name
// "gitlab_execute_action", so two spans for entirely different operations are
// indistinguishable without gitlab_mcp.action. This drives two calls that a
// trace must be able to tell apart.
func TestMiddleware_ActionIsRecordedWhereTheToolNameIsNot(t *testing.T) {
	recorder := newRecorder(t)
	// Reads the raw JSON the SDK actually delivers, the same shape the real
	// identifier in internal/tools handles.
	identifier := IdentifierFunc(func(_ string, arguments any) (Identity, bool) {
		raw, ok := arguments.(json.RawMessage)
		if !ok {
			return Identity{}, false
		}
		var envelope struct {
			Action string `json:"action"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Action == "" {
			return Identity{}, false
		}
		return Identity{ActionID: envelope.Action}, true
	})
	handler := Middleware(Options{Identifier: identifier, Surface: "dynamic"})(
		func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return &mcp.CallToolResult{}, nil
		},
	)

	for _, action := range []string{"issue.list", "branch.delete"} {
		_, _ = handler(context.Background(), "tools/call",
			callToolRequest("gitlab_execute_action", map[string]any{"action": action}, nil))
	}

	spans := recorder.Ended()
	if len(spans) != 2 {
		t.Fatalf("recorded %d spans, want 2", len(spans))
	}
	if spans[0].Name() != spans[1].Name() {
		t.Fatalf("span names already differ (%q, %q); this test no longer proves what it exists for",
			spans[0].Name(), spans[1].Name())
	}
	first, _ := attrOf(spans[0], AttrActionID)
	second, _ := attrOf(spans[1], AttrActionID)
	if first.AsString() == second.AsString() {
		t.Errorf("both spans carry action %q; listing issues and deleting a branch are indistinguishable",
			first.AsString())
	}
}

// TestMiddleware_SuccessLeavesTheStatusUnset covers the MUST in this area.
//
// "Span Status Code MUST be left unset if the instrumented operation has ended
// without any errors." Setting Ok would be worse than redundant: it is defined
// as a human's verdict rather than a handler's, and the SDK enforces the total
// order Ok > Error > Unset with an early return, so an Ok here would make it
// impossible for any later layer to mark the span failed.
func TestMiddleware_SuccessLeavesTheStatusUnset(t *testing.T) {
	span := runOnce(t, Options{}, "tools/list", &mcp.ListToolsRequest{Params: &mcp.ListToolsParams{}},
		&mcp.ListToolsResult{}, nil)

	if got := span.Status().Code; got != codes.Unset {
		t.Errorf("status = %v on a successful call, want Unset", got)
	}
	if _, ok := attrOf(span, AttrErrorType); ok {
		t.Error("error.type is set on a successful call; the registry says instrumentations SHOULD NOT")
	}
}

// TestMiddleware_IsErrorResultIsAFailure covers the shape unique to MCP: a
// failure traveling inside a successful JSON-RPC response, because it is
// addressed to the model rather than to the transport. A middleware that only
// inspected the returned error would call this a success.
func TestMiddleware_IsErrorResultIsAFailure(t *testing.T) {
	span := runOnce(t, Options{}, "tools/call",
		callToolRequest("gitlab_issue_list", map[string]any{}, nil),
		&mcp.CallToolResult{IsError: true}, nil)

	if got := span.Status().Code; got != codes.Error {
		t.Errorf("status = %v for an IsError result, want Error", got)
	}
	value, ok := attrOf(span, AttrErrorType)
	if !ok {
		t.Fatal("error.type is absent on a failed call")
	}
	if value.AsString() != ErrorTypeToolError {
		t.Errorf("error.type = %q, want %q, which the convention states outright",
			value.AsString(), ErrorTypeToolError)
	}
}

// TestMiddleware_CallerFaultCodesAreNotServerFailures is the exemption that
// matters most in practice on the default surface.
//
// A model naming an action that does not exist produces -32601, and counting
// each one as a server error would turn this deployment's error rate into a
// measurement of model confusion. The code is still recorded, because it is a
// fact about the response even when it is not our failure.
func TestMiddleware_CallerFaultCodesAreNotServerFailures(t *testing.T) {
	for _, code := range []int64{-32700, -32600, -32601, -32602, -32002} {
		t.Run(codeString(code), func(t *testing.T) {
			span := runOnce(t, Options{}, "tools/call",
				callToolRequest("gitlab_execute_action", map[string]any{}, nil),
				nil, &jsonrpc.Error{Code: code, Message: "no"})

			if got := span.Status().Code; got != codes.Unset {
				t.Errorf("status = %v for caller-fault code %d, want Unset", got, code)
			}
			if _, ok := attrOf(span, AttrErrorType); ok {
				t.Errorf("error.type is set for caller-fault code %d", code)
			}
			value, ok := attrOf(span, AttrRPCResponseStatusCode)
			if !ok {
				t.Fatalf("rpc.response.status_code is absent for code %d; the code is a fact about the response", code)
			}
			if value.AsString() != codeString(code) {
				t.Errorf("rpc.response.status_code = %q, want %q", value.AsString(), codeString(code))
			}
		})
	}
}

// TestMiddleware_ServerErrorIsAFailure is the other half. Any code outside the
// five is ours, and it takes the status, the classification and the message.
func TestMiddleware_ServerErrorIsAFailure(t *testing.T) {
	span := runOnce(t, Options{}, "tools/call",
		callToolRequest("gitlab_issue_list", map[string]any{}, nil),
		nil, &jsonrpc.Error{Code: -32603, Message: "internal error"})

	if got := span.Status().Code; got != codes.Error {
		t.Errorf("status = %v for -32603, want Error", got)
	}
	if got := span.Status().Description; got != "internal error" {
		t.Errorf("status description = %q, want the JSONRPCError message", got)
	}
	value, _ := attrOf(span, AttrErrorType)
	if value.AsString() != "-32603" {
		t.Errorf("error.type = %q, want the status code as a string", value.AsString())
	}
}

// TestMiddleware_UnclassifiableErrorIsOther pins the fallback. An error with no
// JSON-RPC code must not put the Go error text into error.type: the registry
// requires that attribute to be predictable and low cardinality, and our
// wrapped errors carry GitLab-side messages that can name private paths.
func TestMiddleware_UnclassifiableErrorIsOther(t *testing.T) {
	span := runOnce(t, Options{}, "tools/call",
		callToolRequest("gitlab_issue_list", map[string]any{}, nil),
		nil, errors.New("project gitlab-org/secret-thing not found"))

	value, ok := attrOf(span, AttrErrorType)
	if !ok {
		t.Fatal("error.type is absent on a failed call")
	}
	if value.AsString() != ErrorTypeOther {
		t.Errorf("error.type = %q, want %q; the error text must not become the classification",
			value.AsString(), ErrorTypeOther)
	}
}

// TestMiddleware_AdoptsTraceContextFromMeta asserts the parenting rule, and it
// is the one thing here that cannot be checked by reading the code: the keys
// are unprefixed inside _meta by an explicit exception to MCP's own prefix
// requirement, so a reasonable-looking prefixed lookup would find nothing and
// every trace would silently start over at this server.
func TestMiddleware_AdoptsTraceContextFromMeta(t *testing.T) {
	const (
		wantTrace = "4bf92f3577b34da6a3ce929d0e0e4736"
		parentSpn = "00f067aa0ba902b7"
	)
	span := runOnce(t, Options{}, "tools/call",
		callToolRequest("gitlab_issue_list", map[string]any{}, map[string]any{
			"traceparent": "00-" + wantTrace + "-" + parentSpn + "-01",
		}),
		&mcp.CallToolResult{}, nil)

	if got := span.SpanContext().TraceID().String(); got != wantTrace {
		t.Errorf("trace id = %s, want %s from the client's traceparent", got, wantTrace)
	}
	if got := span.Parent().SpanID().String(); got != parentSpn {
		t.Errorf("parent span id = %s, want %s", got, parentSpn)
	}
	if !span.Parent().IsRemote() {
		t.Error("the parent is not marked remote; a sampler cannot distinguish an inbound trace from a local one")
	}
}

// TestMiddleware_MalformedTraceContextIsIgnored covers what a hostile or merely
// broken client sends. The propagator's contract is that an unparseable value
// leaves the context untouched, and it must not become an error the caller
// sees: refusing a tools/call because its traceparent was malformed would let
// anyone disable a client by corrupting one header.
func TestMiddleware_MalformedTraceContextIsIgnored(t *testing.T) {
	for _, value := range []any{"not-a-traceparent", "", 42, map[string]any{"nested": true}} {
		span := runOnce(t, Options{}, "tools/call",
			callToolRequest("gitlab_issue_list", map[string]any{}, map[string]any{"traceparent": value}),
			&mcp.CallToolResult{}, nil)

		if span.Parent().IsValid() {
			t.Errorf("traceparent %v produced a parent; a malformed value must leave the context untouched", value)
		}
		if got := span.Status().Code; got != codes.Unset {
			t.Errorf("traceparent %v turned a successful call into status %v", value, got)
		}
	}
}

// TestMiddleware_PromptAndResourceNaming pins the two non-tool methods, and one
// deliberate omission.
//
// A prompt name is 37 values compiled in, so it goes in the span name without
// argument. A resource URI does not: "Instrumentation MAY allow users to opt
// into including {mcp.resource.uri} as target in the span name... but SHOULD
// NOT include it by default to avoid high cardinality span names", and ours
// embed project identifiers, so it would be both a cardinality problem and a
// privacy one.
func TestMiddleware_PromptAndResourceNaming(t *testing.T) {
	promptSpan := runOnce(t, Options{}, "prompts/get",
		&mcp.GetPromptRequest{Params: &mcp.GetPromptParams{Name: "review_mr"}},
		&mcp.GetPromptResult{}, nil)
	if got := promptSpan.Name(); got != "prompts/get review_mr" {
		t.Errorf("prompt span name = %q, want the method and the prompt name", got)
	}

	resourceSpan := runOnce(t, Options{}, "resources/read",
		&mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: "gitlab://projects/42/issues"}},
		&mcp.ReadResourceResult{}, nil)
	if got := resourceSpan.Name(); got != "resources/read" {
		t.Errorf("resource span name = %q, want the bare method: a URI in the name is high cardinality", got)
	}
	for _, kv := range resourceSpan.Attributes() {
		if kv.Value.AsString() == "gitlab://projects/42/issues" {
			t.Errorf("the resource URI reached attribute %s; it is Opt-In and declined", kv.Key)
		}
	}
}

// TestMiddleware_WithoutAnIdentifier degrades to no action attribute rather
// than to a panic. This runs on every tool call, so forgetting to wire the
// identifier must cost one attribute, not the process.
func TestMiddleware_WithoutAnIdentifier(t *testing.T) {
	span := runOnce(t, Options{}, "tools/call",
		callToolRequest("gitlab_issue_list", map[string]any{}, nil),
		&mcp.CallToolResult{}, nil)

	if _, ok := attrOf(span, AttrActionID); ok {
		t.Error("an action was recorded with no identifier configured")
	}
	if got := span.Name(); got != "tools/call gitlab_issue_list" {
		t.Errorf("span name = %q; the rest of the span must still be correct", got)
	}
}
