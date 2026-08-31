package mcpotel

import (
	"errors"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// callerFaultCodes are the JSON-RPC error codes that a server does not count as
// its own failures.
//
// "The following error codes indicate that the caller sent a request the
// receiver could not serve and SHOULD NOT be considered errors: -32700, -32600,
// -32601, -32602, -32002. Any other error code SHOULD be considered an error."
//
// This matters more here than the sentence suggests. On the dynamic surface,
// which is the default, a model naming an action that does not exist is an
// ordinary event rather than a malfunction, and counting each one as a server
// error would turn this deployment's error rate into a measurement of model
// confusion.
//
// The exemption is server-side only. The same convention says the opposite for
// the client side, "All JSON-RPC error codes SHOULD be considered errors", and
// says it twice, in the client span note and the client metric note, so it is
// not an editing slip. It reaches us because the split is by initiator rather
// than by role: an elicitation request, or a resource-updated notification, is
// something this server initiates and therefore takes the strict rule. That is
// why this function names its side rather than being reusable for both.
var callerFaultCodes = map[int64]struct{}{
	-32700: {}, // parse error
	-32600: {}, // invalid request
	-32601: {}, // method not found
	-32602: {}, // invalid params
	-32002: {}, // resource not found
}

// outcome is the classification of one finished request, shared by the span and
// the duration histogram so the two can never disagree about whether something
// failed.
type outcome struct {
	// failed decides the span status and whether error.type is emitted at all.
	failed bool
	// errorType is the error.type value, empty when nothing failed.
	errorType string
	// statusCode is the JSON-RPC code as a string, recorded whenever the
	// response carried one, including for the codes that are not failures.
	statusCode string
	// description is the span status description, and never carries content
	// that could name a private project or repeat a GitLab error body.
	description string
}

// classify decides what a finished request was.
//
// The rules it implements, and the one place it exercises latitude:
//
//   - A Go error carrying a JSON-RPC code is a failure unless the code is one
//     of the five the caller is responsible for.
//   - A CallToolResult with IsError true is a failure with error.type
//     "tool_error", which the convention states outright.
//   - Nothing sets a success status. "Span Status Code MUST be left unset if
//     the instrumented operation has ended without any errors", and Ok is worse
//     than merely unnecessary: it is defined as a human's verdict rather than a
//     handler's, and it is final, since the Go SDK enforces the total order
//     Ok > Error > Unset with a literal early return, so an Ok set here would
//     make it impossible for any later layer to mark the span failed.
func classify(res mcp.Result, err error) outcome {
	if err != nil {
		return classifyError(err)
	}
	if isErrorResult(res) {
		return outcome{failed: true, errorType: ErrorTypeToolError}
	}
	return outcome{}
}

// classifyError turns a handler error into the three attributes that describe
// it.
//
// The JSON-RPC code is the classification when there is one: "If a response
// status code was returned and status indicates an error, error.type SHOULD be
// set to that status code". Falling back to _OTHER rather than to the Go error
// text is deliberate, and is what keeps error.type predictable and low
// cardinality as the registry requires; the text would be unbounded and would
// carry GitLab-side messages that can name private paths.
func classifyError(err error) outcome {
	var wire *jsonrpc.Error
	if !errors.As(err, &wire) {
		return outcome{failed: true, errorType: ErrorTypeOther}
	}

	code := codeString(wire.Code)
	if _, callerFault := callerFaultCodes[wire.Code]; callerFault {
		// Not a failure, but the code is still a fact about the response, and
		// recording it is what lets an operator see a client sending malformed
		// requests without that showing up as this server erroring.
		return outcome{statusCode: code}
	}
	return outcome{
		failed:     true,
		errorType:  code,
		statusCode: code,
		// The convention asks for the JSONRPCError message here, and it is
		// safe: these are protocol-level messages this server writes, not
		// GitLab response bodies.
		description: wire.Message,
	}
}

// classifyClient is the strict counterpart to [classifyError], for operations
// this server initiates.
//
// Every JSON-RPC error code counts. "All JSON-RPC error codes SHOULD be
// considered errors" is the client-side rule, and it contradicts the
// server-side one on purpose: a caller-fault code such as method-not-found is
// not the receiver's failure, but when WE are the caller a client that cannot
// serve our elicitation is exactly the thing to notice.
//
// It is a separate function rather than a parameter on the other one, because
// the two rules being one call apart with a boolean between them is how one of
// them eventually gets applied to the wrong side.
func classifyClient(err error) outcome {
	if err == nil {
		return outcome{}
	}

	var wire *jsonrpc.Error
	if !errors.As(err, &wire) {
		return outcome{failed: true, errorType: ErrorTypeOther}
	}
	code := codeString(wire.Code)
	return outcome{
		failed:      true,
		errorType:   code,
		statusCode:  code,
		description: wire.Message,
	}
}

// nameIsUnverified reports whether the response is one that a request naming
// something nonexistent produces.
//
// Method-not-found and invalid-params are the two the SDK answers with when the
// tool or prompt named does not exist. Both are already caller faults rather
// than failures of this server, so this asks a narrower question of the same
// classification rather than introducing a second one.
//
// A tool result carrying IsError is deliberately not included: the handler ran,
// which means the name resolved, and the failure is about what the handler
// found rather than about what it was called.
func (o outcome) nameIsUnverified() bool {
	return o.statusCode == codeString(-32601) || o.statusCode == codeString(-32602)
}

// isErrorResult reports whether a tool call answered with a failure inside a
// successful response.
//
// This shape is particular to MCP and is easy to miss: CallToolResult.IsError
// travels in a normal JSON-RPC success, because the failure is addressed to the
// model rather than to the transport. A middleware that only looked at the
// returned error would report every one of these as a success.
func isErrorResult(res mcp.Result) bool {
	callResult, ok := res.(*mcp.CallToolResult)
	return ok && callResult != nil && callResult.IsError
}

// record writes the outcome onto a span.
//
// It runs before the deferred End rather than inside it, and the ordering is
// load-bearing: after End, SetStatus and SetAttributes are silent no-ops, with
// no error and no log line, because the SDK guards each one with isRecording.
// A span recorded the wrong way round looks green forever and nothing says why.
func (o outcome) record(span trace.Span) {
	if !span.IsRecording() {
		return
	}
	attrs := make([]attribute.KeyValue, 0, 2)
	if o.errorType != "" {
		attrs = append(attrs, AttrErrorType.String(o.errorType))
	}
	if o.statusCode != "" {
		attrs = append(attrs, AttrRPCResponseStatusCode.String(o.statusCode))
	}
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	if o.failed {
		span.SetStatus(codes.Error, o.description)
	}
	// No SetStatus on the success path. See classify.
}

// metricAttributes returns the outcome's contribution to the duration
// histogram's label set.
//
// It is separate from record because the two carry different things on purpose:
// a span may hold attributes a metric must not, since every distinct label
// combination on a metric is a time series that has to be stored and paid for.
// Sharing one attribute builder between them is the way a high-cardinality
// value ends up on a metric by accident.
func (o outcome) metricAttributes() []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 2)
	if o.errorType != "" {
		attrs = append(attrs, AttrErrorType.String(o.errorType))
	}
	if o.statusCode != "" {
		attrs = append(attrs, AttrRPCResponseStatusCode.String(o.statusCode))
	}
	return attrs
}

// codeString renders a JSON-RPC code the way the convention asks for it.
//
// A string, not an int: the deprecated rpc.jsonrpc.error_code was an integer and
// its replacement rpc.response.status_code is a string, which is the kind of
// detail that produces a type mismatch nobody notices until a backend refuses
// the series.
func codeString(code int64) string {
	return strconv.FormatInt(code, 10)
}
