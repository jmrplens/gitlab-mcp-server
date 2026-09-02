// Package mcpotel instruments MCP request handling with OpenTelemetry.
//
// It imports only the OpenTelemetry API, never the SDK. Which providers exist,
// where they export and whether they exist at all is decided once in
// internal/telemetry; this package asks the global providers for a tracer and a
// meter and gets working no-ops when nothing is installed. That is why there is
// no "telemetry enabled" flag anywhere here, and why there must never be one:
// the API is specified to work without an SDK, so a flag would only add a
// branch that can disagree with reality.
package mcpotel

import (
	"context"
	"reflect"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// metaCarrier adapts an MCP request's _meta field to the propagator interface,
// so the standard W3C implementation does the parsing rather than this package.
//
// # Why _meta and not the HTTP headers
//
// The MCP convention is explicit that a server "SHOULD, by default, use context
// extracted from MCP params._meta as a parent for MCP server span", and its
// stated reason is that the two do not line up: one MCP request can be served
// by several HTTP requests when a client retries, and one streamable HTTP
// request can carry more than one MCP request. Reading the transport's headers
// instead would parent an MCP operation to whichever HTTP round trip happened
// to carry it.
//
// It is also the only option on stdio, which has no headers at all and is this
// server's primary transport.
//
// # The keys are reserved, unprefixed, by name
//
// MCP requires most _meta keys to carry a reverse-DNS prefix, and then names
// this exception: "As an exception to the prefix requirement above, the keys
// traceparent, tracestate, and baggage are reserved for OpenTelemetry trace
// context propagation. When present, their values MUST follow W3C Trace
// Context." So the lookup is a bare key at the top level of _meta, and a
// prefixed variant would be wrong rather than merely unusual.
//
// # Read-only on purpose
//
// Set is a no-op. This server extracts trace context and never injects it back
// into a client's message: a response's _meta belongs to the response, and
// writing a traceparent there would offer a client the identifiers of our
// internal spans, which is exactly what the W3C security section warns about
// leaking outward.
//
// Nothing else injects either, and this comment used to say the HTTP transport
// did. It does not: there is no Inject call anywhere in the non-test tree, and
// an outbound GitLab request carries no traceparent, tracestate or baggage.
// This server is a sink for trace context, which is worth stating plainly
// because it is the reason bounding what arrives (see [sanitizeRemoteContext])
// costs nothing downstream.
type metaCarrier struct {
	meta map[string]any
}

// Get returns a value only when it is a string.
//
// A client can put any JSON value under these keys, and a number or an object
// where a traceparent belongs is not an error to report: the propagator's
// contract is that a value it cannot parse leaves the context untouched. "If a
// value can not be parsed from the carrier, for a cross-cutting concern, the
// implementation MUST NOT throw an exception and MUST NOT store a new value in
// the Context, in order to preserve any previously existing valid value."
// Returning the empty string is how that reaches the propagator.
func (c metaCarrier) Get(key string) string {
	value, ok := c.meta[key].(string)
	if !ok {
		return ""
	}
	return value
}

// Set does nothing. See the type doc: this carrier reads.
func (metaCarrier) Set(string, string) {
	// Deliberately empty. Writing a traceparent into a response's _meta would
	// hand a client the identifiers of this server's internal spans, which is
	// what the W3C security section warns about leaking outward. The propagator
	// interface requires the method; nothing here may implement it.
}

// Keys returns the propagation keys present, which is what the interface asks
// for rather than every key in _meta.
func (c metaCarrier) Keys() []string {
	keys := make([]string, 0, 3)
	for _, key := range []string{"traceparent", "tracestate", "baggage"} {
		if _, ok := c.meta[key].(string); ok {
			keys = append(keys, key)
		}
	}
	return keys
}

// maxTraceStateBytes bounds the caller-chosen tracestate this server carries
// onto its own spans.
//
// W3C permits 32 members of 256 characters of key plus 256 of value, so a
// conforming caller can hand over about 16 KiB, and the SDK exports it verbatim
// on every span in the trace. On the HTTP edge that caller has not
// authenticated yet, so the value is bytes an anonymous party writes into the
// operator's traces bill. 512 is enough for the vendor state a real gateway
// carries and far below what a caller could otherwise choose.
const maxTraceStateBytes = 512

// sanitizeRemoteContext bounds what an upstream caller decides about this
// server's own spans, without discarding the correlation they are for.
//
// Two rules, applied only to a span context that arrived from outside this
// process — a locally created ambient parent is this server's own and is left
// exactly as it is:
//
//   - The tracestate is truncated to [maxTraceStateBytes] by dropping whole
//     entries from the end, which is what W3C section 3.3.1.5 requires of a
//     truncation: "the vendor MUST truncate whole entries". Keeping the head
//     preserves a fronting gateway's own vendor state, which is the reason to
//     truncate rather than clear.
//   - When keepCallerSampling is false, a cleared sampled flag is treated as
//     the recommendation W3C says it is rather than as a veto. The trace id,
//     span id and parent relationship are untouched, so the caller's trace
//     still joins; what they no longer decide is whether this server records
//     the work it did for them.
//
// The two live together because they answer the same question: how much
// authority a value carries by virtue of having been sent to us.
func sanitizeRemoteContext(ctx context.Context, keepCallerSampling bool) context.Context {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() || !spanContext.IsRemote() {
		return ctx
	}

	changed := false
	if bounded, truncated := boundTraceState(spanContext.TraceState()); truncated {
		spanContext = spanContext.WithTraceState(bounded)
		changed = true
	}
	if !keepCallerSampling && !spanContext.IsSampled() {
		spanContext = spanContext.WithTraceFlags(spanContext.TraceFlags() | trace.FlagsSampled)
		changed = true
	}
	if !changed {
		return ctx
	}
	return trace.ContextWithRemoteSpanContext(ctx, spanContext)
}

// boundTraceState truncates a tracestate to the byte bound by dropping whole
// entries from the end, reporting whether it changed anything.
//
// Entries rather than bytes, because a tracestate cut mid-entry is not a
// shorter tracestate, it is a malformed one that the next hop's parser
// discards in full. Delete is used rather than rebuilding with Insert, which
// moves a key to the front and would reorder what survives.
func boundTraceState(state trace.TraceState) (trace.TraceState, bool) {
	if len(state.String()) <= maxTraceStateBytes {
		return state, false
	}

	keys := make([]string, 0, state.Len())
	state.Walk(func(key, _ string) bool {
		keys = append(keys, key)
		return true
	})
	for i := len(keys) - 1; i >= 0 && len(state.String()) > maxTraceStateBytes; i-- {
		state = state.Delete(keys[i])
	}
	return state, true
}

// carrierFor builds a carrier over a request's _meta, or an empty one.
func carrierFor(req mcp.Request) propagation.TextMapCarrier {
	params := paramsOf(req)
	if params == nil {
		return metaCarrier{}
	}
	return metaCarrier{meta: params.GetMeta()}
}

// paramsOf returns a request's parameters, or nil when it carries none.
//
// req.GetParams() cannot be compared against nil, and a comment here once
// claimed the wire could not produce one. It can, and did: `tools/list` with no
// `params` member at all is a valid request, the SDK allows the member to be
// missing for every list method and for notifications/initialized, and what a
// receiving middleware is handed for one is a Params interface holding a typed
// nil pointer. That is not a nil interface, so `params != nil` is true and the
// first method called on it dereferences the pointer. A hundred requests a day
// were panicking on a hosted deployment before this was written, all of them on
// methods a client issues once at startup.
//
// The SDK knows the case exists: Params declares isNil for exactly this. It is
// unexported, so a middleware outside that package has to ask reflect instead.
// Registered in docs/development/upstream-bugs.md.
func paramsOf(req mcp.Request) mcp.Params {
	if req == nil {
		return nil
	}
	params := req.GetParams()
	if params == nil {
		return nil
	}
	if value := reflect.ValueOf(params); value.Kind() == reflect.Pointer && value.IsNil() {
		return nil
	}
	return params
}
