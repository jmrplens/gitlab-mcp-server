package subscriptions

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// scopeName names this package as the instrumentation scope on its spans.
const scopeName = "github.com/jmrplens/gitlab-mcp-server/v2/internal/subscriptions"

// tracer is fetched once. Without an installed SDK it is a working no-op, which
// is why there is no enabled flag here and must never be one.
var tracer = otel.Tracer(scopeName)

// detachSpan removes the span context while keeping every other value.
//
// # Why a watcher must not inherit the subscribe request's span
//
// A subscription outlives the request that created it, by design and by up to
// twenty-four hours. The span for that request ends in milliseconds, and ending
// a span "MUST NOT have any effects on child spans" and "MUST NOT inactivate
// the Span in any Context it is active in". So a context carried forward
// unchanged keeps parenting every poll, and every GitLab call inside it, to a
// span that finished long ago.
//
// What that produces is worse than untidy. The subscribe request's trace grows
// for a day, with children arriving hours after the root closed; a backend that
// finalizes a trace after a window drops them entirely; and the root's own
// duration stops meaning anything next to descendants that outlast it by four
// orders of magnitude.
//
// The relationship is kept as a link instead, which is what links are for: a
// causal connection between spans that are not in a parent-child lifetime.
func detachSpan(ctx context.Context) context.Context {
	return trace.ContextWithSpanContext(ctx, trace.SpanContext{})
}

// pollSpan starts the span for one poll, rooted rather than nested.
//
// origin is the subscribe request's span context, recorded as a link when it is
// valid. A subscription created before telemetry was enabled, or by a client
// sending no trace context, has none, and an invalid link is dropped rather
// than emitted as an empty one.
//
// The URI is never recorded as itself, and never part of the name. Our resource
// URIs embed project and group identifiers, and this server's documented
// position is that they are not exported, a position it holds even against the
// MCP convention's Conditionally Required mcp.resource.uri on a resources/read
// span. A poll span writing the raw URI would be that same disclosure by
// another route, repeated for the life of the watch rather than once.
//
// What goes on the span instead comes from the resource hook, which the manager
// receives rather than decides: a keyed digest by default, so two watchers of
// the same kind stay distinguishable and one watcher stays correlatable across
// its polls while nothing names a project, and the URI itself only under the
// identity policy that already exports a caller's real name.
//
// Either way it is an attribute and never the span name. A name built per
// watcher is one span name per project, which is the cardinality trap the
// conventions name for exactly this attribute.
func (m *Manager[S]) pollSpan(ctx context.Context, w *watcher[S]) (context.Context, trace.Span) {
	attrs := []attribute.KeyValue{
		attribute.String("gitlab_mcp.subscription.kind", w.kind.String()),
	}
	attrs = append(attrs, m.resourceAttributes(w.uri)...)

	opts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attrs...),
	}
	if w.origin.IsValid() {
		opts = append(opts, trace.WithLinks(trace.Link{SpanContext: w.origin}))
	}
	return tracer.Start(ctx, "subscription poll", opts...)
}

// resourceAttributes says what a poll span may record about the resource it
// polls, which is a decision this package receives rather than makes.
//
// The hook rather than an import: this package depends on the OpenTelemetry API
// and nothing that pulls the SDK in, and redaction lives in a package that does.
// A nil hook records nothing, which is right for a manager built without
// telemetry wiring: the kind alone still says what shape of thing was polled.
func (m *Manager[S]) resourceAttributes(uri string) []attribute.KeyValue {
	if m == nil || m.opts.ResourceAttributes == nil {
		return nil
	}
	return m.opts.ResourceAttributes(uri)
}
