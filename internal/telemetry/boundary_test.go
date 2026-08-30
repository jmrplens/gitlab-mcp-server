package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// TestOutboundContext_ClearsBaggageThatWouldOtherwiseBeInjected is the
// regression for a leak that would have shipped, and it asserts the leak rather
// than the function.
//
// The whole point is that not forwarding baggage is not the default. The test
// therefore builds the real thing that leaks it: the composite propagator this
// package installs, injecting into an outbound request's headers. It first
// proves the header appears without the boundary, so that a future change
// making OutboundContext a no-op fails here rather than passing quietly.
func TestOutboundContext_ClearsBaggageThatWouldOtherwiseBeInjected(t *testing.T) {
	propagator := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})

	member, err := baggage.NewMember("tenant", "acme")
	if err != nil {
		t.Fatalf("NewMember: %v", err)
	}
	bag, err := baggage.New(member)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inbound := baggage.ContextWithBaggage(context.Background(), bag)

	leaked := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://gitlab.example.com/api/v4/projects", nil)
	propagator.Inject(inbound, propagation.HeaderCarrier(leaked.Header))
	if leaked.Header.Get("baggage") == "" {
		t.Fatal("the propagator did not inject baggage at all; this test can no longer detect the leak it exists for")
	}

	guarded := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://gitlab.example.com/api/v4/projects", nil)
	propagator.Inject(OutboundContext(inbound), propagation.HeaderCarrier(guarded.Header))
	if got := guarded.Header.Get("baggage"); got != "" {
		t.Errorf("baggage header = %q; a caller's baggage reached the outbound request", got)
	}
}

// TestOutboundContext_LeavesTraceContextAlone pins the other half. Clearing
// baggage must not sever the distributed trace: a span this server creates for
// an outbound call still belongs to the caller's trace, which is the entire
// reason for propagating context in the first place.
func TestOutboundContext_LeavesTraceContextAlone(t *testing.T) {
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("TraceIDFromHex: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("SpanIDFromHex: %v", err)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	inbound := trace.ContextWithSpanContext(context.Background(), sc)

	got := trace.SpanContextFromContext(OutboundContext(inbound))
	if got.TraceID() != traceID {
		t.Errorf("trace id = %s, want %s; clearing baggage severed the trace", got.TraceID(), traceID)
	}
	if got.SpanID() != spanID {
		t.Errorf("span id = %s, want %s", got.SpanID(), spanID)
	}
}

// TestOutboundContext_OnAContextWithNoBaggage is the ordinary case, which is
// most calls. It must not allocate a problem where there is none, and it must
// not panic on a context that never carried baggage.
func TestOutboundContext_OnAContextWithNoBaggage(t *testing.T) {
	ctx := OutboundContext(context.Background())

	if members := baggage.FromContext(ctx).Len(); members != 0 {
		t.Errorf("baggage has %d members on a context that never carried any", members)
	}
}
