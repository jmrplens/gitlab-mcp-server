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
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/telemetry"
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
	return runOnceWithOptions(t, opts, method, req, res, handlerErr)
}

// runOnceWithOptions is the same, named for the cases that vary the options
// rather than the request.
func runOnceWithOptions(t *testing.T, opts Options, method string, req mcp.Request, res mcp.Result, handlerErr error) sdktrace.ReadOnlySpan {
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

// unknownName is the sentinel a bounded dimension falls back to.
const unknownName = ErrorTypeOther

// errInvalidParams is what the SDK answers when the tool or prompt named does not
// exist. It is also what it answers for a real name with wrong arguments, which
// is the whole reason the substitution is coarse.
var errInvalidParams = &jsonrpc.Error{Code: -32602, Message: `unknown prompt "not-a-prompt"`}

// TestMetricName_AnUnknownPromptDoesNotBecomeADimensionValue is the regression
// for a hole found by driving the hosted deployment and reading its collector.
//
// A prompts/get naming a prompt that does not exist recorded the invented name
// as a value of gen_ai.prompt.name on mcp.server.operation.duration. The name
// is copied off the request and the convention puts it on the metric, and
// nothing checked that it referred to anything, so a caller could mint one time
// series per string it chose to send. The SDK does not refuse an exhausted
// series budget: it collapses everything past the limit into a single
// otel.metric.overflow bucket, first-come-wins under cumulative temporality, so
// the result is the silent loss of the real series rather than an error.
func TestMetricName_AnUnknownPromptDoesNotBecomeADimensionValue(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	handler := Middleware(Options{})(
		func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return nil, errInvalidParams
		},
	)
	_, _ = handler(context.Background(), "prompts/get",
		&mcp.GetPromptRequest{Params: &mcp.GetPromptParams{Name: "not-a-prompt"}})

	// The values are collected before they are examined: dimensionValues
	// returns an empty slice when the metric is absent, and a loop over that
	// passes while checking nothing at all.
	values := dimensionValues(t, reader, "gen_ai.prompt.name")
	if len(values) == 0 {
		t.Fatal("no gen_ai.prompt.name was recorded, so this assertion would pass without testing the bound")
	}
	for _, value := range values {
		if value == "not-a-prompt" {
			t.Error("a prompt name that names nothing became a metric dimension value; a caller chooses how many time series this process stores")
		}
		if value != unknownName {
			t.Errorf("gen_ai.prompt.name = %q on a failed lookup, want the %q bucket", value, unknownName)
		}
	}
}

// TestSpanName_AnUnknownPromptIsRecordedVerbatim is the other half, and the
// reason the bound is on the metric alone.
//
// A span has no series budget, and the name a client actually sent is the whole
// content of a report that its calls are failing. Bucketing it there would throw
// away the only evidence of what went wrong.
func TestSpanName_AnUnknownPromptIsRecordedVerbatim(t *testing.T) {
	span := runOnce(t, Options{}, "prompts/get",
		&mcp.GetPromptRequest{Params: &mcp.GetPromptParams{Name: "not-a-prompt"}},
		nil, errInvalidParams)

	value, ok := attrOf(span, AttrGenAIPromptName)
	if !ok {
		t.Fatal("gen_ai.prompt.name is absent from the span")
	}
	if value.AsString() != "not-a-prompt" {
		t.Errorf("span records %q; the span is where the name a client sent must survive exactly", value.AsString())
	}
}

// TestMetricName_AResolvedNameSurvives keeps the bound from swallowing the
// signal it exists to protect.
//
// A call that succeeded named something real, so the metric must carry that
// name rather than the bucket. Without this, a substitution that fired
// unconditionally would pass the test above while making the dimension useless.
func TestMetricName_AResolvedNameSurvives(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	handler := Middleware(Options{})(
		func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return &mcp.GetPromptResult{}, nil
		},
	)
	_, _ = handler(context.Background(), "prompts/get",
		&mcp.GetPromptRequest{Params: &mcp.GetPromptParams{Name: "review-merge-request"}})

	values := dimensionValues(t, reader, "gen_ai.prompt.name")
	if len(values) == 0 {
		t.Fatal("gen_ai.prompt.name is absent from the metric on a successful prompts/get")
	}
	for _, value := range values {
		if value != "review-merge-request" {
			t.Errorf("gen_ai.prompt.name = %q on a successful call, want the real name", value)
		}
	}
}

// TestMetricName_AnUnknownToolDoesNotBecomeADimensionValue covers the same hole
// on the other attribute that is copied off a request.
//
// It matters most on the individual surface, where the legitimate value space is
// already about eleven hundred names and has the least headroom left before the
// SDK's per-instrument limit.
func TestMetricName_AnUnknownToolDoesNotBecomeADimensionValue(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	handler := Middleware(Options{})(
		func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return nil, &jsonrpc.Error{Code: -32602, Message: `unknown tool "gitlab_not_a_tool"`}
		},
	)
	_, _ = handler(context.Background(), "tools/call",
		callToolRequest("gitlab_not_a_tool", map[string]any{}, nil))

	values := dimensionValues(t, reader, "gen_ai.tool.name")
	if len(values) == 0 {
		t.Fatal("no gen_ai.tool.name was recorded, so this assertion would pass without testing the bound")
	}
	for _, value := range values {
		if value == "gitlab_not_a_tool" {
			t.Error("a tool name that names nothing became a metric dimension value")
		}
		if value != unknownName {
			t.Errorf("gen_ai.tool.name = %q on a failed lookup, want the %q bucket", value, unknownName)
		}
	}
}

// dimensionValues collects every value recorded for one dimension across every
// instrument, so an assertion about a label space looks at all of it.
func dimensionValues(t *testing.T, reader interface {
	Collect(context.Context, *metricdata.ResourceMetrics) error
}, key string,
) []string {
	t.Helper()

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}

	var out []string
	eachDataPointAttributes(collected, func(attrs []attribute.KeyValue) {
		for _, kv := range attrs {
			if kv.Key == attribute.Key(key) {
				out = append(out, kv.Value.AsString())
			}
		}
	})
	return out
}

// admitted is the list a deployment passes in, kept short so a test that adds
// a version has to say so.
var admitted = []string{"2026-07-28", "2025-11-25"}

// TestMiddleware_ProtocolVersionIsRecordedFromMeta covers the attribute the
// convention marks Recommended and this package declared and then never set.
//
// The constant existed in attributes.go from the first commit, which is the
// failure mode worth naming: a declared-but-unused attribute key reads exactly
// like an implemented one, and nothing in Go complains about an unused package
// level constant. It took a collector with real traffic to notice the value was
// absent from every span.
func TestMiddleware_ProtocolVersionIsRecordedFromMeta(t *testing.T) {
	req := callToolRequest("gitlab_execute_action", nil, map[string]any{
		metaProtocolVersionKey: "2026-07-28",
	})

	span := runOnce(t, Options{ProtocolVersions: admitted}, "tools/call", req, nil, nil)

	value, ok := attrOf(span, AttrMCPProtocolVersion)
	if !ok {
		t.Fatal("mcp.protocol.version is absent; the convention marks it Recommended")
	}
	if value.AsString() != "2026-07-28" {
		t.Errorf("mcp.protocol.version = %q, want %q", value.AsString(), "2026-07-28")
	}
}

// TestMiddleware_UnadmittedProtocolVersionIsDropped is the guard that makes the
// attribute safe to put on a metric.
//
// The value arrives from the caller. Recording whatever it says would let a
// client mint a time series per spelling, and the SDK answers an exhausted
// series budget by collapsing the overflow into one otel.metric.overflow
// bucket, first-come-wins under cumulative temporality. That is silent data
// destruction, so the allow-list is load-bearing rather than defensive.
func TestMiddleware_UnadmittedProtocolVersionIsDropped(t *testing.T) {
	req := callToolRequest("gitlab_execute_action", nil, map[string]any{
		metaProtocolVersionKey: "1999-01-01-not-a-revision",
	})

	span := runOnce(t, Options{ProtocolVersions: admitted}, "tools/call", req, nil, nil)

	if _, ok := attrOf(span, AttrMCPProtocolVersion); ok {
		t.Error("an unadmitted version was recorded; a caller can then mint one time series per spelling")
	}
}

// TestMiddleware_NoAllowListRecordsNothing pins the default for a caller that
// never configured the list, so the unsafe case is the one that needs an
// explicit opt-in rather than the safe one.
func TestMiddleware_NoAllowListRecordsNothing(t *testing.T) {
	req := callToolRequest("gitlab_execute_action", nil, map[string]any{
		metaProtocolVersionKey: "2026-07-28",
	})

	span := runOnce(t, Options{}, "tools/call", req, nil, nil)

	if _, ok := attrOf(span, AttrMCPProtocolVersion); ok {
		t.Error("a version was recorded with no allow-list configured")
	}
}

// TestMiddleware_NoSessionMeansNoSessionID covers the condition attached to
// mcp.session.id, which is a condition rather than a preference: "When the MCP
// request or notification is part of a session."
//
// Under the default stateless HTTP transport there is no session id, and the
// right answer is to omit the attribute rather than invent a per-POST value.
// A request built without a session is the same case.
func TestMiddleware_NoSessionMeansNoSessionID(t *testing.T) {
	req := callToolRequest("gitlab_execute_action", nil, nil)

	span := runOnce(t, Options{}, "tools/call", req, nil, nil)

	if _, ok := attrOf(span, AttrMCPSessionID); ok {
		t.Error("mcp.session.id was recorded for a request that is part of no session")
	}
}

// TestMiddleware_LinksTheAmbientContextWhenMetaSuppliesTheParent is the
// regression for the half of the propagation rule that was missing.
//
// The convention asks for two relationships, not one: parent on the context
// from params._meta, "and SHOULD link current ambient context, if it's
// present". Extract replaces the ambient context in ctx, so an implementation
// that reads it afterwards finds the extracted one and links nothing. On HTTP
// the ambient span is this server's own HTTP span, so without the link a trace
// arriving through _meta has no record of which HTTP request carried it.
func TestMiddleware_LinksTheAmbientContextWhenMetaSuppliesTheParent(t *testing.T) {
	recorder := newRecorder(t)

	// An ambient span, standing in for the HTTP server span.
	ambientCtx, ambient := otel.Tracer("test").Start(context.Background(), "POST")
	defer ambient.End()

	req := callToolRequest("gitlab_execute_action", nil, map[string]any{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	})

	handler := Middleware(Options{})(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})
	_, _ = handler(ambientCtx, "tools/call", req)

	var span trace.SpanContext
	links := 0
	for _, s := range recorder.Ended() {
		if s.Name() != "tools/call gitlab_execute_action" {
			continue
		}
		span = s.Parent()
		links = len(s.Links())
		for _, l := range s.Links() {
			if !l.SpanContext.Equal(ambient.SpanContext()) {
				t.Errorf("link points at %v, want the ambient span %v", l.SpanContext, ambient.SpanContext())
			}
		}
	}

	if got := span.TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("parent trace = %s, want the one from _meta", got)
	}
	if links != 1 {
		t.Errorf("recorded %d links, want 1: the ambient context is dropped rather than linked", links)
	}
}

// TestMiddleware_NoLinkWhenTheAmbientContextIsAlreadyTheParent keeps the link
// from becoming noise.
//
// With no trace context in _meta the ambient span is already the parent, and a
// span linked to its own parent states a relationship the tree already carries.
func TestMiddleware_NoLinkWhenTheAmbientContextIsAlreadyTheParent(t *testing.T) {
	recorder := newRecorder(t)

	ambientCtx, ambient := otel.Tracer("test").Start(context.Background(), "POST")
	defer ambient.End()

	handler := Middleware(Options{})(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})
	_, _ = handler(ambientCtx, "tools/call", callToolRequest("gitlab_execute_action", nil, nil))

	for _, s := range recorder.Ended() {
		if s.Name() != "tools/call gitlab_execute_action" {
			continue
		}
		if got := len(s.Links()); got != 0 {
			t.Errorf("recorded %d links, want 0: the ambient span is the parent already", got)
		}
		if !s.Parent().Equal(ambient.SpanContext()) {
			t.Errorf("parent = %v, want the ambient span", s.Parent())
		}
	}
}

// staticResources answers with whatever it was built with, so a test can drive
// both policies without building a redactor.
type staticResources []attribute.KeyValue

func (r staticResources) ResourceAttributes(uri string) []attribute.KeyValue {
	if uri == "" {
		return nil
	}
	return r
}

// TestResourceAttributes_ReachTheSpanAndNeverAMetric is the regression for a
// hole opened by the change that added the attributes.
//
// describe appends them to call.attributes, and that list feeds both signals,
// so the resource became a metric dimension the moment it became a span
// attribute. Either form is one distinct value per project, merge request,
// pipeline or job a client touches: a series count no operator can predict and
// no deployment can bound.
//
// It reached production before anything caught it. The collector module's
// inventory test exists for precisely this and missed it, because its
// never-on-a-metric list named mcp.resource.uri and not the
// gitlab_mcp.resource.ref key invented alongside it.
func TestResourceAttributes_ReachTheSpanAndNeverAMetric(t *testing.T) {
	tests := []struct {
		name string
		attr attribute.KeyValue
	}{
		{name: "the digest under the default policy", attr: AttrResourceRef.String("10a1a87eec6fea96")},
		{name: "the URI under full", attr: AttrResourceURI.String("gitlab://project/82077663")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader, restore := newMetricRecorder(t)
			defer restore()
			recorder := newRecorder(t)

			handler := Middleware(Options{Resources: staticResources{tc.attr}})(
				func(context.Context, string, mcp.Request) (mcp.Result, error) {
					return &mcp.ReadResourceResult{}, nil
				},
			)
			_, _ = handler(context.Background(), "resources/read",
				&mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: "gitlab://project/82077663"}})

			var onSpan bool
			for _, span := range recorder.Ended() {
				if _, ok := attrOf(span, tc.attr.Key); ok {
					onSpan = true
				}
			}
			if !onSpan {
				t.Errorf("%s is absent from the span, where it is the only thing naming what was read", tc.attr.Key)
			}

			if instrument, _ := metricCarryingValue(t, reader, tc.attr.Value.AsString()); instrument != "" {
				t.Errorf("%s reached metric %s; it is one series per resource a client touches", tc.attr.Key, instrument)
			}
		})
	}
}

// TestResourceAttributeKeys_MatchTheRedactor guards a drift this package cannot
// see on its own.
//
// internal/telemetry decides which key a resource is named under and this
// package decides which keys a metric may not carry, and the two write the
// strings out separately because mcpotel imports the OpenTelemetry API and
// never the SDK. A rename on one side would silently stop the filter working:
// the attribute would still be produced, the filter would still run, and it
// would match nothing.
//
// A test can import both, so the constraint is checkable even though the
// production code cannot express it.
func TestResourceAttributeKeys_MatchTheRedactor(t *testing.T) {
	if string(AttrResourceURI) != telemetry.AttrResourceURI {
		t.Errorf("mcpotel says %q and telemetry says %q; the metric filter would match nothing",
			AttrResourceURI, telemetry.AttrResourceURI)
	}
	if string(AttrResourceRef) != telemetry.AttrResourceRef {
		t.Errorf("mcpotel says %q and telemetry says %q; the metric filter would match nothing",
			AttrResourceRef, telemetry.AttrResourceRef)
	}
}

// TestMiddleware_DurationHistogramMatchesTheConvention pins the three
// properties a dashboard silently depends on, none of which produces an error
// when wrong.
//
// The name is what every query is written against. The unit is seconds, which
// the convention fixes and which is the opposite of the milliseconds this
// server logs, so it is exactly the kind of thing a reader assumes rather than
// checks. And the bucket boundaries are a SHOULD in the convention with a very
// different default in the Go SDK: passing them is what makes a p99 comparable
// between this server and any other MCP server an operator runs.
//
// Getting any of the three wrong produces a working metric that means something
// else, which is worse than a missing one.
func TestMiddleware_DurationHistogramMatchesTheConvention(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	handler := Middleware(Options{Surface: "dynamic"})(
		func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return &mcp.CallToolResult{}, nil
		},
	)
	_, _ = handler(context.Background(), "tools/call",
		callToolRequest("gitlab_issue_list", map[string]any{}, nil))

	recorded := collectedHistogram(t, reader, "mcp.server.operation.duration")

	if recorded.Unit != "s" {
		t.Errorf("unit = %q, want %q; the convention fixes seconds and this server logs milliseconds, so the two disagree by a factor of a thousand",
			recorded.Unit, "s")
	}

	histogram, ok := recorded.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("data is %T, want a float64 histogram", recorded.Data)
	}
	if len(histogram.DataPoints) == 0 {
		t.Fatal("no data point was recorded for a call that completed")
	}

	want := []float64{0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 30, 60, 120, 300}
	got := histogram.DataPoints[0].Bounds
	if len(got) != len(want) {
		t.Fatalf("%d bucket boundaries, want %d; the SDK default was used instead of the convention's",
			len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("boundary %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestMiddleware_DurationCarriesTheMethodAndTheAction asserts the metric can
// answer the question it exists for.
//
// A duration with no dimensions is a single number for the whole server, which
// tells an operator that something is slow and nothing about what. The method
// is Required by the convention; the action is ours and is the only thing that
// distinguishes two calls on the default surface, where every tools/call names
// the same tool.
func TestMiddleware_DurationCarriesTheMethodAndTheAction(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	identifier := IdentifierFunc(func(string, any) (Identity, bool) {
		return Identity{ActionID: "issue.list", Domain: "issue"}, true
	})
	handler := Middleware(Options{Identifier: identifier, Surface: "dynamic"})(
		func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return &mcp.CallToolResult{}, nil
		},
	)
	_, _ = handler(context.Background(), "tools/call",
		callToolRequest("gitlab_execute_action", map[string]any{"action": "issue.list"}, nil))

	found := map[string]string{}
	for _, kv := range collectedAttributes(t, reader) {
		found[string(kv.Key)] = kv.Value.AsString()
	}
	for key, want := range map[string]string{
		string(AttrMCPMethodName): "tools/call",
		string(AttrActionID):      "issue.list",
		string(AttrToolSurface):   "dynamic",
	} {
		if found[key] != want {
			t.Errorf("%s = %q on the metric, want %q", key, found[key], want)
		}
	}
}

// newMetricRecorder installs a real meter provider that collects into memory.
//
// A manual reader rather than a fake meter, for the same reason the span tests
// use a real tracer provider: the rules being asserted are enforced inside the
// SDK. Attribute-set deduplication, the cardinality limit, and the exact shape
// of a histogram's data points are all SDK behavior, and a fake would accept
// whatever it was handed and prove nothing about what a collector would receive.
func newMetricRecorder(t *testing.T) (*metric.ManualReader, func()) {
	t.Helper()

	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))

	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)

	return reader, func() {
		otel.SetMeterProvider(previous)
		_ = provider.Shutdown(context.Background())
	}
}

// collectedAttributes returns every attribute on every data point collected.
//
// Flattened deliberately. An assertion about what must never appear has to look
// everywhere rather than at the one instrument the test author had in mind: a
// value leaking onto a metric nobody thought to check is exactly the case worth
// catching.
func collectedAttributes(t *testing.T, reader *metric.ManualReader) []attribute.KeyValue {
	t.Helper()

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}

	var attrs []attribute.KeyValue
	eachDataPointAttributes(collected, func(points []attribute.KeyValue) {
		attrs = append(attrs, points...)
	})
	return attrs
}

// collectedHistogram returns one named histogram, or fails.
func collectedHistogram(t *testing.T, reader *metric.ManualReader, name string) metricdata.Metrics {
	t.Helper()

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == name {
				return m
			}
		}
	}
	t.Fatalf("no metric named %q was recorded", name)
	return metricdata.Metrics{}
}

// eachDataPointAttributes calls fn with the attribute set of every data point,
// whatever the instrument's type.
//
// The type switch rather than one assertion is the whole point. Every
// instrument this server has today is a Histogram[float64], so a helper that
// handled only that type was right by accident, and an assertion about what
// must never appear on a metric would have gone on passing the day somebody
// added a counter carrying it. The failure would be silent, in the direction of
// green.
func eachDataPointAttributes(collected metricdata.ResourceMetrics, fn func([]attribute.KeyValue)) {
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			for _, attrs := range dataPointAttributeSets(m.Data) {
				fn(attrs)
			}
		}
	}
}

// dataPointAttributeSets returns the attribute set of every data point in one
// instrument's aggregation, whatever its type.
//
// Split from the sweep above only because the type switch is six near-identical
// arms and a linter counts that as complexity. It is not: each arm is the same
// sentence about a different generic instantiation, which Go has no way to say
// once.
func dataPointAttributeSets(data metricdata.Aggregation) [][]attribute.KeyValue {
	var out [][]attribute.KeyValue
	switch d := data.(type) {
	case metricdata.Histogram[float64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes.ToSlice())
		}
	case metricdata.Histogram[int64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes.ToSlice())
		}
	case metricdata.Sum[float64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes.ToSlice())
		}
	case metricdata.Sum[int64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes.ToSlice())
		}
	case metricdata.Gauge[float64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes.ToSlice())
		}
	case metricdata.Gauge[int64]:
		for _, p := range d.DataPoints {
			out = append(out, p.Attributes.ToSlice())
		}
	}
	return out
}

// eachNamedDataPoint is the same sweep, with the instrument's name, for the
// assertions that report which metric carried something.
func eachNamedDataPoint(collected metricdata.ResourceMetrics, fn func(name string, attrs []attribute.KeyValue)) {
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			eachDataPointAttributes(
				metricdata.ResourceMetrics{ScopeMetrics: []metricdata.ScopeMetrics{{Metrics: []metricdata.Metrics{m}}}},
				func(attrs []attribute.KeyValue) { fn(m.Name, attrs) },
			)
		}
	}
}
