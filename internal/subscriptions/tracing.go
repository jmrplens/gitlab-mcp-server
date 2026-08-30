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
// The URI is an attribute and never part of the name: our resource URIs embed
// project and group identifiers, so a name built from one would mint a distinct
// span name per project, which is the cardinality trap the conventions name
// explicitly for exactly this attribute.
func (m *Manager[S]) pollSpan(ctx context.Context, w *watcher[S]) (context.Context, trace.Span) {
	opts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("gitlab_mcp.subscription.kind", w.kind.String()),
			attribute.String("gitlab_mcp.subscription.uri", w.uri),
		),
	}
	if w.origin.IsValid() {
		opts = append(opts, trace.WithLinks(trace.Link{SpanContext: w.origin}))
	}
	return tracer.Start(ctx, "subscription poll", opts...)
}
