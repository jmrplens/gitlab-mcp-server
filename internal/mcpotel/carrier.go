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
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/propagation"
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
// leaking outward. Injection for our own outbound GitLab calls happens in the
// HTTP transport, where the standard propagator handles it.
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
func (metaCarrier) Set(string, string) {}

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

// carrierFor builds a carrier over a request's _meta, or an empty one.
//
// Every request type in the SDK embeds Meta and exposes GetMeta, so this cannot
// fail; the nil check is for a request whose params were never populated, which
// a test can produce even though the wire cannot.
func carrierFor(req mcp.Request) propagation.TextMapCarrier {
	if req == nil {
		return metaCarrier{}
	}
	params := req.GetParams()
	if params == nil {
		return metaCarrier{}
	}
	return metaCarrier{meta: params.GetMeta()}
}
