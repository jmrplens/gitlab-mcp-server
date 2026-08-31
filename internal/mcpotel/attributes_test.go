package mcpotel

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TestRecordRefusal_MarksTheSpanWithTheReason is the regression for an
// attribute that was declared and set by nothing.
//
// AttrRefusalReason arrived with a doc comment explaining its shape and its
// place beside error.type, and no code ever wrote it. That is the second key in
// this package to be declared and then forgotten, after mcp.protocol.version,
// and the reason both survived is the same: a package-level constant that
// nobody reads compiles without complaint and reads exactly like a feature.
//
// The cost is specific. A safe-mode preview, a rate-limit refusal and an
// unknown action all reach a span as an ordinary failed call, so a deployment
// refusing every third request looks the same to an operator as one whose
// GitLab is erroring.
func TestRecordRefusal_MarksTheSpanWithTheReason(t *testing.T) {
	recorder := newRecorder(t)

	ctx, span := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).
		Tracer("test").Start(context.Background(), "tools/call")
	RecordRefusal(ctx, "safe_mode")
	span.End()

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(ended))
	}
	value, ok := attrOf(ended[0], AttrRefusalReason)
	if !ok {
		t.Fatal("gitlab_mcp.refusal_reason is absent; the span says a call failed and not that this server declined it")
	}
	if value.AsString() != "safe_mode" {
		t.Errorf("gitlab_mcp.refusal_reason = %q, want %q", value.AsString(), "safe_mode")
	}
}

// TestRecordRefusal_WithoutASpanIsHarmless covers the two ordinary cases where
// there is nothing to mark: telemetry off, and a unit test that installed no
// provider. Both are the common path, so a panic there would be worse than the
// missing attribute this fixes.
func TestRecordRefusal_WithoutASpanIsHarmless(t *testing.T) {
	RecordRefusal(context.Background(), "safe_mode")
}

// TestRecordRefusal_AnEmptyReasonRecordsNothing keeps an absent reason from
// becoming an attribute whose value is the empty string, which a backend
// renders as a refusal that happened for no reason.
func TestRecordRefusal_AnEmptyReasonRecordsNothing(t *testing.T) {
	recorder := newRecorder(t)

	ctx, span := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).
		Tracer("test").Start(context.Background(), "tools/call")
	RecordRefusal(ctx, "")
	span.End()

	for _, ended := range recorder.Ended() {
		if _, present := attrOf(ended, AttrRefusalReason); present {
			t.Error("an empty reason was recorded as an attribute")
		}
	}
}

// TestRecordRefusal_ReachesTheDurationMetric covers the half that was missing
// while the attribute was on the span alone.
//
// The reason exists to be counted: a deployment refusing every third call
// because its clients have not learned the parameter shape looks identical to a
// healthy one otherwise. Counting is a metric, and a span attribute is not a
// counter, so recording it only there answered the wrong question.
//
// It is affordable because the reasons are a closed set of five, which bounds
// the label space by construction rather than by hoping.
func TestRecordRefusal_ReachesTheDurationMetric(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	handler := Middleware(Options{})(
		func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
			// What a refusing handler does: report the reason and answer with
			// an error result, which is a successful response carrying a
			// failure meant for the model.
			RecordRefusal(ctx, "safe_mode")
			return &mcp.CallToolResult{IsError: true}, nil
		},
	)
	_, _ = handler(context.Background(), "tools/call",
		callToolRequest("gitlab_issue", map[string]any{}, nil))

	values := dimensionValues(t, reader, "gitlab_mcp.refusal_reason")
	if len(values) == 0 {
		t.Fatal("no metric carries the refusal reason, so a refusal rate cannot be computed")
	}
	for _, value := range values {
		if value != "safe_mode" {
			t.Errorf("gitlab_mcp.refusal_reason = %q on the metric, want safe_mode", value)
		}
	}
}

// TestRecordRefusal_AnOrdinaryCallCarriesNoReason keeps the dimension from
// appearing on every measurement.
//
// A label present on some data points and absent on others is two series rather
// than one, which is the intended shape here: the refusals are separable and
// the ordinary calls are not tagged with an empty reason.
func TestRecordRefusal_AnOrdinaryCallCarriesNoReason(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	handler := Middleware(Options{})(
		func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return &mcp.CallToolResult{}, nil
		},
	)
	_, _ = handler(context.Background(), "tools/call",
		callToolRequest("gitlab_issue", map[string]any{}, nil))

	if values := dimensionValues(t, reader, "gitlab_mcp.refusal_reason"); len(values) != 0 {
		t.Errorf("a call that was not refused carries a reason: %v", values)
	}
}

// TestRecordRefusal_WithoutTheHolderStillMarksTheSpan pins the fallback, which
// is what a handler reached from outside this middleware gets.
//
// The holder is only in the context when a middleware put it there, so the span
// attribute has to work on its own rather than depending on it.
func TestRecordRefusal_WithoutTheHolderStillMarksTheSpan(t *testing.T) {
	recorder := newRecorder(t)

	ctx, span := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).
		Tracer("test").Start(context.Background(), "tools/call")
	RecordRefusal(ctx, "rate_limited")
	span.End()

	value, ok := attrOf(recorder.Ended()[0], AttrRefusalReason)
	if !ok || value.AsString() != "rate_limited" {
		t.Errorf("the span does not carry the reason without a holder in the context; got %v", value)
	}
}
