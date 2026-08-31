package mcpotel

import (
	"context"
	"testing"

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
