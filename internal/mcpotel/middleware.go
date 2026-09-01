package mcpotel

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// scopeName is the instrumentation scope every span and measurement carries.
//
// It names the code doing the instrumenting, not the code being instrumented,
// which is what lets an operator tell our spans apart from those of a library
// that also instruments this process. The specification asks for "the
// instrumentation scope, such as the instrumentation library name", and a Go
// import path is the unambiguous form of that.
const scopeName = "github.com/jmrplens/gitlab-mcp-server/v2/internal/mcpotel"

// Options configure the middleware.
type Options struct {
	// Identifier resolves a tools/call to its catalog action. Nil is allowed
	// and means action attributes are omitted; see [CallIdentifier] for why
	// this is not something this package can work out for itself.
	Identifier CallIdentifier

	// Users turns an authenticated caller into attributes, subject to the
	// deployment's identity policy. Nil means nothing about who made a call is
	// ever recorded, which is also what the default policy does.
	Users UserAttributer

	// Resources turns a resource URI into attributes, subject to the
	// deployment's identity policy. Nil records nothing about which resource a
	// request named, which is also what the default policy does with the URI
	// itself: it records a keyed digest instead.
	Resources ResourceAttributer

	// Surface names the registered tool catalog (dynamic, meta, individual).
	// It goes on every span because the same request means different things
	// across the three, and a trace read months later has no other way to tell.
	Surface string

	// Transport is "pipe" for stdio and "tcp" for HTTP, which is what the
	// convention's note prescribes rather than a name of our choosing. Use
	// [TransportPipe] and [TransportTCP].
	Transport string

	// ProtocolVersions are the MCP revisions this server admits. Only a
	// version in this list is ever recorded, because the value arrives from the
	// caller and lands on a metric dimension; see protocolVersionFor. Empty
	// means the attribute is never recorded, which is the safe default for a
	// caller that has not thought about it.
	ProtocolVersions []string
}

// Middleware instruments every MCP request with a span and a duration
// measurement.
//
// # Shape
//
// One span per MCP request, SERVER kind, parented from the trace context in
// params._meta rather than from the transport. The convention gives the reason:
// one MCP request can be served by several HTTP requests when a client retries,
// and one streamable HTTP request can carry more than one MCP request, so
// parenting to the transport would attach an operation to whichever round trip
// happened to carry it.
//
// # No enabled flag
//
// There is none, deliberately. Without an installed SDK, otel.Tracer and
// otel.Meter return working no-ops that still propagate span context, so a
// flag would only add a branch that can disagree with whether telemetry is
// actually running. The cost of instrumenting unconditionally is the attribute
// construction below, which is a handful of constant-keyed strings.
func Middleware(opts Options) mcp.Middleware {
	identifier := opts.Identifier
	if identifier == nil {
		identifier = noIdentity{}
	}
	users := opts.Users
	if users == nil {
		users = noUserAttributes{}
	}
	resources := opts.Resources
	if resources == nil {
		resources = noResourceAttributes{}
	}
	tracer := otel.Tracer(scopeName)
	meter := otel.Meter(scopeName)
	duration := newDurationHistogram(meter)
	allowed := allowedVersions(opts.ProtocolVersions)

	// Built once: these are the same for every request this process serves, and
	// rebuilding them per call would allocate on the hot path for no reason.
	constant := make([]attribute.KeyValue, 0, 2)
	if opts.Surface != "" {
		constant = append(constant, AttrToolSurface.String(opts.Surface))
	}
	if opts.Transport != "" {
		constant = append(constant, AttrNetworkTransport.String(opts.Transport))
	}

	sessions := newSessionTracker(meter, constant, opts.Transport)

	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			call := describe(method, req, identifier, resources)

			// Extract before Start, so the incoming trace context is the
			// parent rather than a link. A malformed or absent value leaves
			// ctx untouched, which is the propagator's specified behavior and
			// the reason no error is checked here.
			//
			// The ambient context is read first because Extract replaces it.
			// On HTTP that ambient span is this server's own HTTP span, and
			// the convention asks for both relationships: parent on the MCP
			// context, "and SHOULD link current ambient context, if it's
			// present". Without the link, a trace that arrives through _meta
			// loses every trace of which HTTP request carried it.
			ambient := trace.SpanContextFromContext(ctx)
			ctx = otel.GetTextMapPropagator().Extract(ctx, carrierFor(req))
			parent := trace.SpanContextFromContext(ctx)

			// Identity is resolved before the span starts, like everything
			// else on it, because a sampler can only see what was present at
			// creation. A deployment sampling by user needs it there.
			identityAttrs := users.UserAttributes(ctx, req)

			// The version is bounded by the allow-list, so it is cheap enough
			// to carry on the metric too. The session id is not: it is one
			// value per connected client, and the convention's own instrument
			// table omits it for exactly that reason.
			version := protocolVersionFor(req, allowed)
			sessionID := sessionIDOf(req)

			attrs := make([]attribute.KeyValue, 0, len(constant)+len(call.attributes)+len(identityAttrs)+2)
			attrs = append(attrs, constant...)
			attrs = append(attrs, call.attributes...)
			if version != "" {
				attrs = append(attrs, AttrMCPProtocolVersion.String(version))
			}
			if sessionID != "" {
				attrs = append(attrs, AttrMCPSessionID.String(sessionID))
			}
			attrs = append(attrs, identityAttrs...)

			// The context returned by Start is the one passed onward. Passing
			// the original would compile, run, and silently produce a flat
			// trace with every GitLab call as a root.
			startOpts := []trace.SpanStartOption{
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(attrs...),
			}
			// Only when Extract actually changed the parent. With no incoming
			// context the ambient span is already the parent, and linking a
			// span to its own parent says nothing.
			if ambient.IsValid() && parent.IsValid() && !parent.Equal(ambient) {
				startOpts = append(startOpts, trace.WithLinks(trace.Link{SpanContext: ambient}))
			}

			ctx, span := tracer.Start(ctx, call.spanName, startOpts...)

			// The handler reports a refusal through here, since the middleware
			// cannot tell one from an ordinary failure by looking at the
			// result.
			ctx, refusal := withRefusalHolder(ctx)
			// Deferred so a panic still ends the span, which is what makes the
			// SDK record the panic as an exception event before re-panicking.
			defer span.End()

			// The tracker deliberately outlives this request: it parks a
			// goroutine on the session, which ends long after the call returns.
			sessions.observe(req, version) //nolint:contextcheck // the session outlives the request, so a request context would cancel the measurement

			started := time.Now()
			res, err := next(ctx, method, req)
			result := classify(res, err)

			// Before End, not inside it. After End, SetStatus and SetAttributes
			// are silent no-ops guarded by isRecording, so an outcome recorded
			// afterwards leaves the span green with nothing to say why.
			result.record(span)

			// The metric deliberately omits the identity attributes the span
			// carries. Every distinct label combination is a time series that
			// has to be stored and paid for, and a per-user dimension is
			// unbounded by construction: it grows with the number of people
			// using the deployment, which is exactly the number an operator
			// cannot predict. The Go SDK would drop the overflow into a bucket
			// marked otel.metric.overflow rather than refuse it, so the failure
			// would be silent data destruction rather than an error.
			metricAttrs := make([]attribute.KeyValue, 0, len(constant)+len(call.attributes)+3)
			metricAttrs = append(metricAttrs, constant...)
			metricAttrs = append(metricAttrs, metricAttributesFor(call, result)...)
			if version != "" {
				metricAttrs = append(metricAttrs, AttrMCPProtocolVersion.String(version))
			}
			metricAttrs = append(metricAttrs, result.metricAttributes()...)
			// Bounded by construction: the reasons are a closed set of five, so
			// this adds at most five label combinations per action rather than
			// growing with traffic. Counting them is the point of recording
			// them at all, and a deployment refusing every third call looks
			// identical to a healthy one without it.
			if refusal.reason != "" {
				metricAttrs = append(metricAttrs, AttrRefusalReason.String(refusal.reason))
			}
			duration.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(metricAttrs...))

			return res, err
		}
	}
}

// call is what one request turned out to be, resolved before the span starts.
type call struct {
	// callerNameKey names the attribute whose value the caller chose, or is
	// empty when the request carries no such attribute. It exists so the metric
	// can bound a value the span records verbatim; see boundedMetricName.
	callerNameKey attribute.Key

	spanName   string
	attributes []attribute.KeyValue
}

// describe works out the span name and creation-time attributes for a request.
//
// The naming rule is the convention's: "{mcp.method.name} {target}" where the
// target is the tool or prompt name when there is one, and the bare method name
// otherwise. The resource URI is deliberately not a target: "Instrumentation
// MAY allow users to opt into including {mcp.resource.uri} as target in the
// span name when it is available but SHOULD NOT include it by default to avoid
// high cardinality span names." Our resource URIs embed project ids, so that is
// a trap rather than a nicety.
func describe(method string, req mcp.Request, identifier CallIdentifier, resources ResourceAttributer) call {
	attrs := []attribute.KeyValue{AttrMCPMethodName.String(method)}

	switch params := paramsOf(req).(type) {
	case *mcp.CallToolParamsRaw:
		return describeToolCall(method, params.Name, params.Arguments, attrs, identifier)
	case *mcp.CallToolParams:
		return describeToolCall(method, params.Name, params.Arguments, attrs, identifier)

	case *mcp.GetPromptParams:
		// 37 names, compiled in, so this is unambiguously low cardinality and
		// needs none of the deliberation the tool surfaces do.
		if params.Name != "" {
			attrs = append(attrs, AttrGenAIPromptName.String(params.Name))
			return call{
				spanName:      method + " " + params.Name,
				attributes:    attrs,
				callerNameKey: AttrGenAIPromptName,
			}
		}

	case *mcp.ReadResourceParams:
		// The URI is an attribute and never part of the name: a name built from
		// one is a distinct span name per project, which the convention calls
		// out for this attribute specifically.
		//
		// What the attribute says is the redactor's decision, not this
		// function's. By default it is a keyed digest, which correlates reads
		// of one resource without naming it; under the identity policy that
		// already exports a caller's real name, it is the URI, which is the
		// convention's Conditionally Required mcp.resource.uri.
		attrs = append(attrs, resources.ResourceAttributes(params.URI)...)
		return call{spanName: method, attributes: attrs}

	case *mcp.SubscribeParams:
		attrs = append(attrs, resources.ResourceAttributes(params.URI)...)
		return call{spanName: method, attributes: attrs}

	case *mcp.UnsubscribeParams:
		attrs = append(attrs, resources.ResourceAttributes(params.URI)...)
		return call{spanName: method, attributes: attrs}
	}

	return call{spanName: method, attributes: attrs}
}

// describeToolCall handles the one case where the tool name is not the answer.
//
// gen_ai.tool.name is what the client asked for and is always recorded, because
// the convention makes it Conditionally Required whenever the operation relates
// to a specific tool. gitlab_mcp.action is what the server actually does, and
// on the default surface it is the only attribute that distinguishes listing
// issues from deleting a branch: gen_ai.tool.name is gitlab_execute_action for
// every one of them.
//
// The span name follows the convention and uses the tool, not the action. That
// means the default surface produces two span names, which is a real loss, and
// it is accepted rather than papered over: deviating would put a value in the
// name that the convention says the target SHOULD match to gen_ai.tool.name,
// and the action is on the span either way for anyone who groups by it.
func describeToolCall(method, toolName string, arguments any, attrs []attribute.KeyValue, identifier CallIdentifier) call {
	if toolName == "" {
		return call{spanName: method, attributes: attrs}
	}

	attrs = append(attrs,
		AttrGenAIToolName.String(toolName),
		// "SHOULD be set to execute_tool when the operation describes a tool
		// call and SHOULD NOT be set otherwise", which is why it appears here
		// and in no other branch.
		AttrGenAIOperationName.String("execute_tool"),
	)

	if identity, ok := identifier.Identify(toolName, arguments); ok {
		if identity.ActionID != "" {
			attrs = append(attrs, AttrActionID.String(identity.ActionID))
		}
		if identity.Domain != "" {
			attrs = append(attrs, AttrDomain.String(identity.Domain))
		}
	}

	return call{
		spanName:      method + " " + toolName,
		attributes:    attrs,
		callerNameKey: AttrGenAIToolName,
	}
}

// newDurationHistogram builds the convention's server duration instrument.
//
// The bucket boundaries are the convention's, passed explicitly because the Go
// SDK's default set is wrong for this metric: "This metric SHOULD be specified
// with ExplicitBucketBoundaries of [0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5,
// 10, 30, 60, 120, 300]". Seconds, not milliseconds, which the convention also
// fixes and which is the opposite of the duration_ms this server logs.
//
// The error is checked rather than discarded. Go's Meter ends every creation
// method with "return i, validateInstrumentName(name)", handing back a fully
// working instrument alongside a non-nil error, so the constructor-shaped call
// invites ignoring it and an invalid name would then record and export nothing
// while looking entirely healthy. A failure here is not worth refusing to
// serve, so it degrades to a no-op instrument and says so once.
func newDurationHistogram(meter metric.Meter) metric.Float64Histogram {
	histogram, err := meter.Float64Histogram(
		"mcp.server.operation.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of an MCP request, measured on the receiver from arrival to response."),
		metric.WithExplicitBucketBoundaries(
			0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 30, 60, 120, 300,
		),
	)
	if err != nil {
		otel.Handle(err)
	}
	return histogram
}

// metricAttributesFor returns the call's attributes as a metric may carry
// them: the caller-supplied name bounded, and the resource attributes gone.
//
// # Why the resource attributes cannot go on a metric
//
// They name one resource. Under the default policy that is a keyed digest and
// under full it is the URI itself, and either way it is one distinct value per
// project, merge request, pipeline or job a client touches, which is a series
// count no operator can predict and no deployment can bound. The span carries
// them precisely because a span has no series budget.
//
// This was introduced by the change that added the attributes, because describe
// appends them to call.attributes and that list feeds both signals. The
// inventory test in test/e2e/collector exists to catch exactly this and did not:
// its never-on-a-metric list named mcp.resource.uri and not the
// gitlab_mcp.resource.ref key invented alongside it. A closed list only closes
// over what somebody remembered to write in it.
//
// # The hole this closes
//
// gen_ai.tool.name and gen_ai.prompt.name are copied off the request, and the
// convention puts both on the duration metric. Nothing checked that the name
// referred to anything: a prompts/get for a prompt that does not exist recorded
// the invented name as a metric dimension value, which was found by driving a
// real deployment and then reading its collector, where a name that is plainly
// an argument label sat in the label set. Any client could then mint one time
// series per string it cared to type, and the SDK's answer to an exhausted
// series budget is not an error but an otel.metric.overflow bucket that
// swallows everything after the limit, first-come-wins under cumulative
// temporality. Silent destruction of the real data, caused by a caller.
//
// # Why the outcome and not a registry
//
// Membership would mean handing this package the set of registered tool and
// prompt names for the active surface, which is between two and about eleven
// hundred entries depending on configuration, rebuilt wherever registration
// happens and drifting the first time someone adds a name somewhere new.
//
// The outcome answers the same question without a second copy of the truth: a
// name that names nothing cannot succeed. The SDK answers it with invalid-params
// or method-not-found, both already classified here as caller faults, so the
// substitution keys off a fact this function already has.
//
// The trade is deliberate and small: a real name whose call failed validation is
// bucketed too, so the metric under-reports it while the span still carries it
// exactly. Losing one label on a failed call is worth not letting a caller
// choose how many time series this process stores.
func metricAttributesFor(c call, result outcome) []attribute.KeyValue {
	bound := c.callerNameKey != "" && result.nameIsUnverified()

	out := make([]attribute.KeyValue, 0, len(c.attributes))
	for _, kv := range c.attributes {
		if kv.Key == AttrResourceURI || kv.Key == AttrResourceRef {
			continue
		}
		if bound && kv.Key == c.callerNameKey {
			out = append(out, kv.Key.String(ErrorTypeOther))
			continue
		}
		out = append(out, kv)
	}
	return out
}
