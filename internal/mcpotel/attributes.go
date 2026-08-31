package mcpotel

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// The attribute keys this package emits, written out rather than imported.
//
// The MCP semantic convention does not ship as a Go package. It lives in the
// open-telemetry/semantic-conventions-genai repository, not on
// opentelemetry.io, whose /docs/specs/semconv/mcp/ returns 404, and that
// repository has no tags and no releases. The frozen semconv/v1.41.0 module
// does contain some of these constants, but it is a snapshot of a convention
// that has since moved and been removed from Go semconv, so importing it would
// give a false sense of currency and would put two semconv versions in one
// build, which invites a schema URL conflict.
//
// The cost of writing them out is that a rename upstream produces no compile
// error. The mitigation is that they are all here, in one block, so a rename is
// a one-file change, and the convention is re-read before each release.
const (
	// TransportPipe and TransportTCP are the convention's own vocabulary for
	// network.transport, not names of our choosing: its "Recording MCP
	// transport" table gives "pipe" for stdio and "tcp" for streamable HTTP.
	TransportPipe = "pipe"
	TransportTCP  = "tcp"

	// attrHTTPRequestMethodOriginal carries the method a caller actually sent
	// when it was not one the convention names. Conditionally Required on the
	// span "if and only if" the recorded method is _OTHER, and deliberately
	// absent from the metric, where it would be the unbounded dimension the
	// substitution exists to prevent.
	attrHTTPRequestMethodOriginal = attribute.Key("http.request.method_original")

	// AttrResourceURI and AttrResourceRef name the resource a request asked
	// for. Which of the two is set is the identity policy's decision, made in
	// internal/telemetry; this package only has to know that neither may reach
	// a metric, because both are one value per resource a client touches.
	AttrResourceURI = attribute.Key("mcp.resource.uri")
	AttrResourceRef = attribute.Key("gitlab_mcp.resource.ref")

	// AttrMCPMethodName is Required: every span and every measurement carries it.
	AttrMCPMethodName = attribute.Key("mcp.method.name")

	// AttrMCPProtocolVersion is Recommended: the negotiated revision string.
	AttrMCPProtocolVersion = attribute.Key("mcp.protocol.version")

	// AttrGenAIToolName is Conditionally Required when the operation relates to
	// a specific tool. Note that the convention reuses the gen_ai namespace
	// here; there is no mcp.tool.name, which is a thing to check rather than
	// assume, because the obvious guess is wrong.
	AttrGenAIToolName = attribute.Key("gen_ai.tool.name")

	// AttrMCPSessionID is Recommended, and its note is a condition rather than
	// a preference: "When the MCP request or notification is part of a
	// session." The default HTTP mode is stateless and has no session id, so
	// the condition is simply not met there and the attribute is omitted rather
	// than filled with a per-request invention. It is deliberately absent from
	// the metric, which the convention's own instrument table omits it from.
	//
	// The serverpool key is never a substitute: it is derived from the token,
	// and putting it here would place a credential fingerprint on every span.
	AttrMCPSessionID = attribute.Key("mcp.session.id")

	// AttrGenAIPromptName is the same shape for prompts/get.
	AttrGenAIPromptName = attribute.Key("gen_ai.prompt.name")

	// AttrGenAIOperationName is Recommended, and its note is a SHOULD NOT as
	// well as a SHOULD: set to execute_tool when the operation describes a tool
	// call, and not set otherwise.
	AttrGenAIOperationName = attribute.Key("gen_ai.operation.name")

	// AttrErrorType is Stable, and the only Stable key in this list. It is
	// Conditionally Required "if and only if the operation fails", which is why
	// nothing here sets it on a success path.
	AttrErrorType = attribute.Key("error.type")

	// AttrRPCResponseStatusCode is Release Candidate. It records the JSON-RPC
	// error code whenever the response carries one, including for the five
	// codes that do not count as errors: the code is a fact about the response,
	// while error.type is a classification of a failure.
	AttrRPCResponseStatusCode = attribute.Key("rpc.response.status_code")

	// AttrNetworkTransport is Stable. The convention's note is explicit for
	// this protocol: tcp when the transport is HTTP, pipe when it is stdio.
	AttrNetworkTransport = attribute.Key("network.transport")
)

// Attribute keys this project invents, under one namespace.
//
// The guidance against extending an OpenTelemetry namespace is a bare
// recommendation rather than an RFC 2119 keyword, and it sanctions "the
// attribute name by your application name, provided that the application name
// is reasonably unique". gitlab_mcp is that name. It is deliberately not mcp.
// or gen_ai., which upstream owns and may extend into, and not gitlab., which
// sits next to the vcs.* namespace and to a product name somebody may yet
// conventionalize.
//
// Once shipped these cannot be renamed without breaking every dashboard built
// on them, so the set is kept small and each one earns its place.
const (
	// AttrActionID carries the canonical catalog action, such as issue.list.
	//
	// It exists because the dynamic surface, which is the default, exposes two
	// tools: gen_ai.tool.name is gitlab_execute_action for every call, so the
	// convention-owned attribute records nothing about what was actually done.
	// Without this the default deployment's traces cannot distinguish listing
	// issues from deleting a branch.
	AttrActionID = attribute.Key("gitlab_mcp.action")

	// AttrToolSurface records which catalog the deployment registered, because
	// the same request means different things across the three and a trace read
	// months later has no other way to tell.
	AttrToolSurface = attribute.Key("gitlab_mcp.tool_surface")

	// AttrRefusalReason carries a value from this server's closed set of
	// refusal reasons, for the failures that are ours rather than GitLab's.
	//
	// It sits alongside error.type rather than inside it, which is the shape
	// the error registry recommends for a domain-specific identifier, and it
	// keeps error.type predictable and low cardinality as that registry asks.
	AttrRefusalReason = attribute.Key("gitlab_mcp.refusal_reason")
)

// error.type values this server emits.
//
// "Instrumentations SHOULD document the list of errors they report", so this is
// a closed set rather than a pattern, and it is published in the documentation
// as well as declared here.
const (
	// ErrorTypeToolError is the convention's own instruction for the case where
	// a JSON-RPC call succeeds and the failure is inside the result:
	// "When CallToolResult is returned with isError set to true, this attribute
	// SHOULD be set to tool_error."
	ErrorTypeToolError = "tool_error"

	// ErrorTypeOther is the registry's fallback, for a failure this server
	// cannot classify. Emitting it is better than inventing a value, and better
	// than omitting the attribute on a span whose status is Error.
	ErrorTypeOther = "_OTHER"
)

// RecordRefusal marks the current span with why this server declined a call.
//
// Called from where the refusal is decided rather than from the middleware,
// because the middleware cannot know: a refusal travels as an error result,
// which is a successful JSON-RPC response carrying a failure meant for the
// model, and from outside the handler it is indistinguishable from a handler
// that ran and failed.
//
// A no-op when there is no recording span, which is the case with telemetry off
// and in every unit test that installs no provider.
func RecordRefusal(ctx context.Context, reason string) {
	if reason == "" {
		return
	}
	// The metric first, because it is the one that can be missed: the holder is
	// only in the context when a middleware put it there, and a handler called
	// from somewhere else still gets the span attribute.
	if holder, ok := ctx.Value(refusalHolderKey{}).(*refusalHolder); ok {
		holder.reason = reason
	}
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.SetAttributes(AttrRefusalReason.String(reason))
	}
}

// refusalHolderKey is the context key for the holder below. An empty struct
// type rather than a string, so nothing else can collide with it.
type refusalHolderKey struct{}

// refusalHolder carries a reason back from the handler to the middleware.
//
// The middleware cannot learn it any other way. A refusal travels as a
// successful JSON-RPC response carrying a failure meant for the model, so from
// outside the handler it is indistinguishable from a handler that ran and
// failed, which is the same reason RecordRefusal is called from where the
// refusal is decided.
//
// One holder per request, reachable only through that request's context, so
// there is nothing to synchronize: the handler writes before it returns and the
// middleware reads after.
type refusalHolder struct {
	reason string
}

// withRefusalHolder returns a context a handler can report a refusal through,
// and the holder to read afterwards.
func withRefusalHolder(ctx context.Context) (context.Context, *refusalHolder) {
	holder := &refusalHolder{}
	return context.WithValue(ctx, refusalHolderKey{}, holder), holder
}
