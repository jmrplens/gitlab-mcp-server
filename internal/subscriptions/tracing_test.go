package subscriptions

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// remoteSpanContext builds a valid span context standing in for the subscribe
// request's span.
func remoteSpanContext(t *testing.T) trace.SpanContext {
	t.Helper()

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("TraceIDFromHex: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("SpanIDFromHex: %v", err)
	}
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
}

// TestDetachSpan_RemovesTheSpanAndKeepsEverythingElse pins both halves, and the
// second half is why this is not simply context.Background().
//
// A watcher's context still has to carry whatever else the subscribe request
// put there. Detaching by starting from scratch would drop it all, which is the
// obvious implementation and the wrong one.
func TestDetachSpan_RemovesTheSpanAndKeepsEverythingElse(t *testing.T) {
	type marker struct{}

	ctx := context.WithValue(context.Background(), marker{}, "kept")
	ctx = trace.ContextWithSpanContext(ctx, remoteSpanContext(t))

	detached := detachSpan(ctx)

	if trace.SpanContextFromContext(detached).IsValid() {
		t.Error("the span context survived; every poll would nest under a span that has ended")
	}
	if got, _ := detached.Value(marker{}).(string); got != "kept" {
		t.Errorf("an unrelated context value was lost: %q", got)
	}
}

// TestPollSpan_IsRootedAndLinksBack is the assertion this whole file exists for.
//
// A subscription outlives its request by up to twenty-four hours while the
// request's span ends in milliseconds. Ending a span has no effect on children,
// so a poll that inherited it would nest under a finished span: the request's
// trace would grow for a day, a backend that finalizes traces after a window
// would drop the late arrivals, and the root's duration would mean nothing
// beside descendants outlasting it by four orders of magnitude.
//
// The causal connection is kept as a link, which is exactly what links are for.
func TestPollSpan_IsRootedAndLinksBack(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	previousTracer := tracer
	tracer = provider.Tracer(scopeName)
	t.Cleanup(func() {
		tracer = previousTracer
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})

	origin := remoteSpanContext(t)
	manager := &Manager[string]{}
	w := &watcher[string]{uri: "gitlab://projects/42/issues", kind: KindBranch, origin: origin}

	// Deliberately given a context that still carries the origin span, which is
	// the shape a caller would produce by forgetting detachSpan. The poll span
	// must be a root regardless.
	ctx := trace.ContextWithSpanContext(context.Background(), origin)
	_, span := manager.pollSpan(detachSpan(ctx), w)
	span.End()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	got := spans[0]

	if got.Parent().IsValid() {
		t.Errorf("the poll span has parent %s; it must be a root", got.Parent().SpanID())
	}
	if got.SpanContext().TraceID() == origin.TraceID() {
		t.Error("the poll span shares the subscribe request's trace; it was not detached")
	}

	links := got.Links()
	if len(links) != 1 {
		t.Fatalf("%d links, want exactly 1 back to the subscribe request", len(links))
	}
	if links[0].SpanContext.TraceID() != origin.TraceID() {
		t.Errorf("the link points at trace %s, want %s", links[0].SpanContext.TraceID(), origin.TraceID())
	}
}

// TestPollSpan_WithoutAnOriginEmitsNoLink covers the ordinary case, which is
// every subscription created by a client that sends no trace context and every
// one created before telemetry was turned on.
//
// An invalid link is dropped rather than emitted empty: a link to nowhere is
// worse than no link, because it looks like a connection somebody could follow.
func TestPollSpan_WithoutAnOriginEmitsNoLink(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousTracer := tracer
	tracer = provider.Tracer(scopeName)
	t.Cleanup(func() {
		tracer = previousTracer
		_ = provider.Shutdown(context.Background())
	})

	manager := &Manager[string]{}
	w := &watcher[string]{uri: "gitlab://projects/42/issues", kind: KindBranch}

	_, span := manager.pollSpan(context.Background(), w)
	span.End()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	if links := spans[0].Links(); len(links) != 0 {
		t.Errorf("%d links on a watcher with no origin; a link to nowhere looks like one somebody could follow", len(links))
	}
}

// TestPollSpan_DoesNotPutTheURIInTheName pins the cardinality rule for the one
// attribute the conventions call out by name.
//
// Our resource URIs embed project and group identifiers, so a span name built
// from one mints a distinct name per project. The convention says so directly
// about this attribute: include it as a target "SHOULD NOT ... by default to
// avoid high cardinality span names".
func TestPollSpan_DoesNotPutTheURIInTheName(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousTracer := tracer
	tracer = provider.Tracer(scopeName)
	t.Cleanup(func() {
		tracer = previousTracer
		_ = provider.Shutdown(context.Background())
	})

	const uri = "gitlab://projects/acme-holdings%2Fprivate-repo/issues"

	// The hook stands in for the redactor the server wires in, so this test
	// asserts the manager consults it rather than reimplementing the rule.
	manager := &Manager[string]{opts: Options{
		ResourceAttributes: func(u string) []attribute.KeyValue {
			return []attribute.KeyValue{attribute.String("gitlab_mcp.resource.ref", "digest-of-"+u[:14])}
		},
	}}
	_, span := manager.pollSpan(context.Background(), &watcher[string]{uri: uri, kind: KindBranch})
	span.End()

	recorded := recorder.Ended()[0]
	if recorded.Name() != "subscription poll" {
		t.Errorf("span name = %q; a URI in the name is one span name per project", recorded.Name())
	}

	var sawRef bool
	for _, kv := range recorded.Attributes() {
		if strings.Contains(kv.Value.AsString(), "acme-holdings") {
			t.Errorf("attribute %s carries the project path; a poll repeats for the life of the watch, so this writes it into a backend over and over", kv.Key)
		}
		if kv.Key == "gitlab_mcp.resource.ref" {
			sawRef = true
		}
	}
	if !sawRef {
		t.Error("nothing distinguishes this watcher from another of the same kind, so one failing subscription and all of them look alike")
	}
}

// TestPollSpan_WithoutAResourceHookRecordsNoResource pins the default for a
// manager built without telemetry wiring: the kind still says what shape of
// thing was polled, and nothing names it.
func TestPollSpan_WithoutAResourceHookRecordsNoResource(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousTracer := tracer
	tracer = provider.Tracer(scopeName)
	t.Cleanup(func() {
		tracer = previousTracer
		_ = provider.Shutdown(context.Background())
	})

	manager := &Manager[string]{}
	_, span := manager.pollSpan(context.Background(),
		&watcher[string]{uri: "gitlab://project/1", kind: KindBranch})
	span.End()

	for _, kv := range recorder.Ended()[0].Attributes() {
		if kv.Key != "gitlab_mcp.subscription.kind" {
			t.Errorf("unexpected attribute %s=%q with no resource hook configured", kv.Key, kv.Value.AsString())
		}
	}
}
