package mcpotel

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TestKnownMethod_SubstitutesAnythingUnrecognized covers the classification the
// HTTP convention requires, and the reason it matters here.
//
// net/http accepts any token as a request method, so r.Method is a string the
// caller chooses. Recording it verbatim on a metric is one time series per
// invented verb, on the one instrument every request touches, and the SDK
// answers an exhausted series budget by collapsing everything past the limit
// into otel.metric.overflow rather than by refusing it.
func TestKnownMethod_SubstitutesAnythingUnrecognized(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		wantRecorded string
		wantOriginal string
	}{
		{name: "GET is known", method: "GET", wantRecorded: "GET"},
		{name: "every convention method is known", method: "CONNECT", wantRecorded: "CONNECT"},
		{name: "an invented verb", method: "FROBNICATE", wantRecorded: "_OTHER", wantOriginal: "FROBNICATE"},
		{
			// HTTP methods are case sensitive, so a lowercase spelling is not
			// the method it resembles. Folding it would report a request the
			// server did not route that way.
			name: "lowercase is not the method it resembles", method: "get",
			wantRecorded: "_OTHER", wantOriginal: "get",
		},
		{name: "empty", method: "", wantRecorded: "_OTHER", wantOriginal: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorded, original := knownMethod(tc.method)
			if recorded != tc.wantRecorded {
				t.Errorf("recorded = %q, want %q", recorded, tc.wantRecorded)
			}
			if original != tc.wantOriginal {
				t.Errorf("original = %q, want %q", original, tc.wantOriginal)
			}
		})
	}
}

// TestServerMiddleware_AnInventedMethodIsNotAMetricDimension is the assertion
// that matters, because knownMethod being correct proves nothing about whether
// the middleware uses it on both signals or on only one.
//
// The span keeps the original, which is where an operator finds out what a
// misbehaving client is sending. The metric must not, or the substitution has
// simply moved the unbounded value to a neighboring key.
func TestServerMiddleware_AnInventedMethodIsNotAMetricDimension(t *testing.T) {
	reader, restore := newMetricRecorder(t)
	defer restore()

	recorder := newRecorder(t)

	handler := ServerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequestWithContext(t.Context(), "FROBNICATE", "/mcp", nil)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	var sawOriginal bool
	for _, span := range recorder.Ended() {
		if value, ok := attrOf(span, attrHTTPRequestMethod); ok && value.AsString() != "_OTHER" {
			t.Errorf("span records http.request.method = %q for an unknown verb", value.AsString())
		}
		if value, ok := attrOf(span, attrHTTPRequestMethodOriginal); ok {
			sawOriginal = true
			if value.AsString() != "FROBNICATE" {
				t.Errorf("http.request.method_original = %q, want the verb the caller sent", value.AsString())
			}
		}
	}
	if !sawOriginal {
		t.Error("the span does not carry http.request.method_original, so nothing records what the client actually sent")
	}

	if instrument, key := metricCarryingValue(t, reader, "FROBNICATE"); instrument != "" {
		t.Errorf("%s carries the caller's verb on metric %s; a client then chooses the label space", key, instrument)
	}

	// The absence above is not enough on its own: it also holds if the
	// middleware stopped recording the instrument at all, which would pass this
	// test while deleting the measurement it exists to bound. So the bucket has
	// to be present, not merely the verb absent.
	if instrument, _ := metricCarryingValue(t, reader, "_OTHER"); instrument == "" {
		t.Error("no metric carries http.request.method=_OTHER; the duration histogram was not recorded at all")
	}
}

// metricCarryingValue reports the first instrument and key recording a given
// attribute value, or empty strings.
//
// Split out of the test above because an assertion about what must never appear
// has to sweep every instrument and every data point, and four nested loops
// inside a test body say less about what is being asserted than one call does.
func metricCarryingValue(t *testing.T, reader interface {
	Collect(context.Context, *metricdata.ResourceMetrics) error
}, value string,
) (instrument, key string) {
	t.Helper()

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}
	eachNamedDataPoint(collected, func(name string, attrs []attribute.KeyValue) {
		for _, kv := range attrs {
			if kv.Value.AsString() == value && instrument == "" {
				instrument, key = name, string(kv.Key)
			}
		}
	})
	return instrument, key
}

// TestServerMiddleware_TheSpanNameIsHTTPForASubstitutedMethod pins the naming
// half of the same rule, which the convention states separately from the
// attribute.
//
// A span name is a low-cardinality label a backend groups by, and "_OTHER"
// there would read as a method rather than as the absence of one.
func TestServerMiddleware_TheSpanNameIsHTTPForASubstitutedMethod(t *testing.T) {
	recorder := newRecorder(t)

	handler := ServerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequestWithContext(t.Context(), "FROBNICATE", "/mcp", nil))

	spans := recorder.Ended()
	if len(spans) == 0 {
		// Without this, a middleware that emitted nothing would pass every
		// assertion in the loop below by never running it.
		t.Fatal("no span was recorded")
	}
	for _, span := range spans {
		if span.Name() != "HTTP" {
			t.Errorf("span name = %q, want %q for a method the convention does not name", span.Name(), "HTTP")
		}
	}
}

// TestKnownMethod_QUERYIsKnown covers the method the first version of this list
// omitted. QUERY is in the convention's set, and recording a valid request as
// _OTHER loses it in the bucket meant for invented verbs.
func TestKnownMethod_QUERYIsKnown(t *testing.T) {
	if recorded, original := knownMethod("QUERY"); recorded != "QUERY" || original != "" {
		t.Errorf("knownMethod(QUERY) = (%q, %q), want (QUERY, \"\")", recorded, original)
	}
}

// TestStatusRecorder_ForwardsFlush covers the method whose absence would not
// look like a bug anywhere.
//
// This server's default HTTP mode answers with text/event-stream, so a
// ResponseWriter wrapper that swallowed Flush would turn every streaming
// response into a hang: the handler would write, the recorder would hold, and
// the client would wait on bytes that were already produced. Nothing about that
// failure names the wrapper.
func TestStatusRecorder_ForwardsFlush(t *testing.T) {
	underlying := &flushCountingWriter{ResponseRecorder: httptest.NewRecorder()}
	recorder := &statusRecorder{ResponseWriter: underlying, status: http.StatusOK}

	recorder.Flush()

	if underlying.flushes != 1 {
		t.Errorf("the underlying writer was flushed %d times, want 1: a held SSE response is a hang the client cannot diagnose",
			underlying.flushes)
	}
}

// TestStatusRecorder_FlushIsSafeWithoutAFlusher keeps the forwarding from
// becoming a panic when the writer underneath cannot flush, which is every
// writer in a test and some in production behind a proxy wrapper.
func TestStatusRecorder_FlushIsSafeWithoutAFlusher(t *testing.T) {
	recorder := &statusRecorder{ResponseWriter: nonFlushingWriter{}, status: http.StatusOK}
	recorder.Flush()
}

// TestStatusRecorder_WriteMarksTheResponseWritten pins the flag the middleware
// uses to tell an answered request from an abandoned one.
func TestStatusRecorder_WriteMarksTheResponseWritten(t *testing.T) {
	recorder := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}

	if _, err := recorder.Write([]byte("body")); err != nil {
		t.Fatalf("writing: %v", err)
	}
	if !recorder.written {
		t.Error("a written response is not marked written")
	}
}

// flushCountingWriter records how many times it was flushed.
type flushCountingWriter struct {
	*httptest.ResponseRecorder
	flushes int
}

func (w *flushCountingWriter) Flush() { w.flushes++ }

// nonFlushingWriter is a ResponseWriter that cannot flush, which is what the
// type assertion in Flush exists for.
type nonFlushingWriter struct{}

func (nonFlushingWriter) Header() http.Header         { return http.Header{} }
func (nonFlushingWriter) Write(b []byte) (int, error) { return len(b), nil }
func (nonFlushingWriter) WriteHeader(int)             {}

// TestServerMiddleware_JoinsTheCallersTrace covers the front door's half of a
// promise the MCP layer already keeps.
//
// A gateway in front of this server sends traceparent like any instrumented
// proxy. The middleware started a new root regardless, so the operator's own
// trace broke exactly at this server while the layer underneath was carefully
// honoring _meta. The assertion is on the trace id, which is the only thing
// joining is.
func TestServerMiddleware_JoinsTheCallersTrace(t *testing.T) {
	recorder := newRecorder(t)
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	previousProp := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(previousProp) })

	handler := ServerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	const upstream = "4bf92f3577b34da6a3ce929d0e0e4736"
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
	req.Header.Set("traceparent", "00-"+upstream+"-00f067aa0ba902b7-01")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	spans := recorder.Ended()
	if len(spans) == 0 {
		t.Fatal("no span was recorded")
	}
	if got := spans[0].SpanContext().TraceID().String(); got != upstream {
		t.Errorf("the HTTP span has trace id %s; the caller sent %s, so their trace breaks at this server's front door",
			got, upstream)
	}
}

// TestRequestScheme_ReadsTheConnectionAndNotAHeader pins where the scheme comes
// from.
//
// X-Forwarded-Proto is deliberately not consulted: it is attacker-controlled on
// any deployment reachable without a proxy, and nothing here branches on the
// scheme, so honoring it would let a caller write the wrong value into an
// operator's dashboard for free. TLS on the connection is the one signal a
// client cannot forge.
func TestRequestScheme_ReadsTheConnectionAndNotAHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tls  bool
		want string
	}{
		{name: "a plain connection", want: "http"},
		{name: "a TLS connection", tls: true, want: "https"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", http.NoBody)
			r.Header.Set("X-Forwarded-Proto", "https")
			if tt.tls {
				r.TLS = &tls.ConnectionState{}
			} else {
				r.TLS = nil
			}

			if got := requestScheme(r); got != tt.want {
				t.Errorf("requestScheme = %q, want %q (a forwarded-proto header must not decide it)", got, tt.want)
			}
		})
	}
}

// TestServerMiddleware_AnUnrecognizedMethodIsBoundedOnTheSpan keeps an
// unauthenticated caller from sizing an exported record.
//
// This middleware runs before the credential check, and net/http accepts any
// token as a method, so http.request.method_original was a value the caller
// chose with no ceiling but net/http's 1 MiB header budget. Twenty requests
// carrying a 500 KB verb produced ten megabytes of exported traces. The file's
// own comment said "the span is where an unbounded value is affordable", which
// is true of cardinality — the point it was making — and false of bytes.
//
// The prefix is kept because that is what the attribute is for: an operator
// looking at a misbehaving client wants to see what it sent, and the first few
// dozen characters answer that as well as a megabyte does.
func TestServerMiddleware_AnUnrecognizedMethodIsBoundedOnTheSpan(t *testing.T) {
	const marker = "TAILMARKERZZ"

	tests := []struct {
		name   string
		method string
		want   string
	}{
		{
			name:   "a short invented verb is recorded whole",
			method: "FROBNICATE",
			want:   "FROBNICATE",
		},
		{
			name:   "an oversized verb keeps its prefix and loses its tail",
			method: strings.Repeat("Q", 64*1024) + marker,
			want:   strings.Repeat("Q", maxOriginalMethod),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := newRecorder(t)

			handler := ServerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			handler.ServeHTTP(httptest.NewRecorder(),
				httptest.NewRequestWithContext(t.Context(), tt.method, "/mcp", nil))

			spans := recorder.Ended()
			if len(spans) == 0 {
				t.Fatal("no span was recorded")
			}
			value, ok := attrOf(spans[0], attrHTTPRequestMethodOriginal)
			if !ok {
				t.Fatal("the span carries no http.request.method_original, so nothing records what the client sent")
			}
			if got := value.AsString(); got != tt.want {
				t.Errorf("http.request.method_original is %d bytes, want the %d-byte %q",
					len(got), len(tt.want), tt.want)
			}
		})
	}
}

// TestServerMiddleware_ACallerCannotSuppressTheServersOwnSpan covers the
// decision an anonymous caller does not get to make.
//
// Trace context is extracted from every request's headers before the credential
// check, and the SDK's default sampler is ParentBased(AlwaysOn), whose
// remoteParentNotSampled branch is NeverSample. So a scanner sending
// "traceparent: 00-<id>-<id>-00" produced no HTTP span at all, including for
// the 401 it was about to receive: the party being refused decided whether the
// refusal was recorded. W3C calls the flags "recommendations given by the
// caller rather than strict rules", and at an unauthenticated front door a
// recommendation is all this one is.
//
// The trace id is asserted alongside, because the fix must not cost the joining
// this middleware exists to provide.
func TestServerMiddleware_ACallerCannotSuppressTheServersOwnSpan(t *testing.T) {
	const upstream = "4bf92f3577b34da6a3ce929d0e0e4736"

	tests := []struct {
		name  string
		flags string
	}{
		{name: "the caller sampled the trace", flags: "01"},
		{name: "the caller cleared the sampled flag", flags: "00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := newRecorder(t)

			handler := ServerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			}))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
			req.Header.Set("traceparent", "00-"+upstream+"-00f067aa0ba902b7-"+tt.flags)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			spans := recorder.Ended()
			if len(spans) == 0 {
				t.Fatal("the caller's trace flags suppressed this server's own span, so an anonymous party decides whether a refusal is recorded")
			}
			if got := spans[0].SpanContext().TraceID().String(); got != upstream {
				t.Errorf("the HTTP span has trace id %s; the caller sent %s, so their trace breaks at this server's front door",
					got, upstream)
			}
		})
	}
}

// TestServerMiddleware_AnOperatorsSamplerKeepsTheCallersDecision is the guard
// on the fix above, and the reason it is guarded rather than unconditional.
//
// Ignoring a caller's sampled flag is right when nobody chose otherwise. It is
// wrong when the operator did: the SDK applies OTEL_TRACES_SAMPLER before any
// option, so an operator running a ratio to keep their bill down has already
// decided how much of a fronting gateway's traffic to record, and overriding
// that from inside a middleware would be exactly the silent override this
// project refuses everywhere else it reads the OTEL_ namespace.
func TestServerMiddleware_AnOperatorsSamplerKeepsTheCallersDecision(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "parentbased_always_on")

	recorder := newRecorder(t)
	handler := ServerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if spans := recorder.Ended(); len(spans) != 0 {
		t.Errorf("recorded %d spans while the operator's own sampler said not to; the middleware overrode a configured decision",
			len(spans))
	}
}

// TestServerMiddleware_BoundsACallersTracestate bounds the other value a caller
// writes straight onto an exported span.
//
// The W3C propagator accepts 32 tracestate members of up to 256 characters each
// and the SDK exports the value verbatim, before authentication, so an
// anonymous request carries about 16 KiB of chosen bytes onto every span
// including the one for its own 401. The bound drops whole entries from the
// end, which is what the specification requires of a truncation ("the vendor
// MUST truncate whole entries"), so a fronting gateway's own vendor state
// survives at the head.
func TestServerMiddleware_BoundsACallersTracestate(t *testing.T) {
	const upstream = "4bf92f3577b34da6a3ce929d0e0e4736"

	recorder := newRecorder(t)
	handler := ServerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", nil)
	req.Header.Set("traceparent", "00-"+upstream+"-00f067aa0ba902b7-01")
	req.Header.Set("tracestate", maximalTraceState())
	handler.ServeHTTP(httptest.NewRecorder(), req)

	spans := recorder.Ended()
	if len(spans) == 0 {
		t.Fatal("no span was recorded")
	}
	state := spans[0].SpanContext().TraceState().String()
	if len(state) > maxTraceStateBytes {
		t.Errorf("the span carries %d bytes of caller-chosen tracestate, over the %d bound",
			len(state), maxTraceStateBytes)
	}
	if state == "" {
		t.Error("the whole tracestate was dropped; truncation is supposed to keep the head, so a gateway's vendor state survives")
	}
	if got := spans[0].SpanContext().TraceID().String(); got != upstream {
		t.Errorf("bounding the tracestate broke the join: trace id %s, want %s", got, upstream)
	}
}

// maximalTraceState builds the largest tracestate the W3C parser accepts: 32
// members, each with a 256-character value.
func maximalTraceState() string {
	members := make([]string, 0, 32)
	for i := range 32 {
		members = append(members, fmt.Sprintf("vendor%02d=%s", i, strings.Repeat("v", 256)))
	}
	return strings.Join(members, ",")
}
