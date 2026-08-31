package mcpotel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
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
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			histogram, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				continue
			}
			for _, point := range histogram.DataPoints {
				for _, kv := range point.Attributes.ToSlice() {
					if kv.Value.AsString() == value {
						return m.Name, string(kv.Key)
					}
				}
			}
		}
	}
	return "", ""
}
