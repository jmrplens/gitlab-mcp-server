package mcpotel

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// roundTripperFunc adapts a function to http.RoundTripper, for the transport
// error case that no real server can produce on demand.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestNewTransport_TheGitLabCallIsAChildSpanThatNamesNoURL covers the layer that
// had no unit test at all.
//
// It is exercised end to end by the collector module, which is where the shape
// of the trace is asserted, and that leaves the decisions inside it unchecked:
// which attributes are recorded, which are deliberately not, and what a
// transport failure does. Each of those is a separate way to be quietly wrong,
// and the one that matters most is an omission, which an end-to-end test can
// only notice if somebody thought to look for it.
func TestNewTransport_TheGitLabCallIsAChildSpanThatNamesNoURL(t *testing.T) {
	recorder := newRecorder(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	client := &http.Client{Transport: NewTransport(nil)}

	ctx, parent := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).
		Tracer("test").Start(context.Background(), "tools/call")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		upstream.URL+"/api/v4/projects/acme%2Fprivate-repo/issues?search=secret", nil)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("the request failed: %v", err)
	}
	_ = resp.Body.Close()
	parent.End()

	span := findSpan(t, recorder, http.MethodGet)

	t.Run("it hangs off the caller's span", func(t *testing.T) {
		if !span.Parent().Equal(parent.SpanContext()) {
			t.Error("the client span is not a child of the span it was made from; the context did not reach the transport")
		}
		if span.SpanKind() != trace.SpanKindClient {
			t.Errorf("kind = %v, want CLIENT", span.SpanKind())
		}
	})

	t.Run("it names the host and never the URL", func(t *testing.T) {
		if value, ok := attrOf(span, attrServerAddress); !ok || value.AsString() == "" {
			t.Error("server.address is absent, so nothing says which instance was called")
		}
		// The rule enforced here rather than merely written down. A GitLab URL
		// carries the project path and often the query, which is why the whole
		// URL is left out and the host kept.
		for _, forbidden := range []string{"url.full", "url.path", "url.query"} {
			for _, kv := range span.Attributes() {
				if string(kv.Key) == forbidden {
					t.Errorf("the span carries %s = %q; a GitLab URL names the project and can carry a search term",
						forbidden, kv.Value.AsString())
				}
			}
		}
	})

	t.Run("a 200 records the status and no error", func(t *testing.T) {
		if value, ok := attrOf(span, attrHTTPResponseStatus); !ok || value.AsInt64() != http.StatusOK {
			t.Errorf("http.response.status_code = %v, want 200", value)
		}
		if span.Status().Code == codes.Error {
			t.Error("a successful call was recorded as an error")
		}
	})
}

// TestNewTransport_A404IsAnAnswerRatherThanAFailure pins a deliberate departure
// from the HTTP convention, which is worth a test precisely because it is one.
//
// The convention says a 4xx SHOULD set the span status to Error on a client
// span. This server probes for things that may not exist, so an expected 404
// would paint a red span on every one of them, and the handler treats it as an
// answer. The status code is recorded either way, so a dashboard can still
// classify however it likes.
func TestNewTransport_A404IsAnAnswerRatherThanAFailure(t *testing.T) {
	recorder := newRecorder(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	client := &http.Client{Transport: NewTransport(nil)}
	ctx, parent := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).
		Tracer("test").Start(context.Background(), "tools/call")

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("the request failed: %v", err)
	}
	_ = resp.Body.Close()
	parent.End()

	span := findSpan(t, recorder, http.MethodGet)
	if span.Status().Code == codes.Error {
		t.Error("a 404 was recorded as a span failure; this server probes for things that may not exist")
	}
	if value, ok := attrOf(span, attrHTTPResponseStatus); !ok || value.AsInt64() != http.StatusNotFound {
		t.Errorf("http.response.status_code = %v, want 404: the code must be recorded whatever the status says", value)
	}
	if _, present := attrOf(span, attrErrorTypeForTransport); present {
		t.Error("error.type was set for a 404, which is an answer rather than a transport failure")
	}
}

// TestNewTransport_ATransportFailureIsBoundedAndSaysNothingAboutTheAddress
// covers the only failure this layer treats as one, and the reason its
// error.type is a constant.
//
// A dial error's text carries the address it failed to reach, so recording it
// would put a host and port into an attribute that a metric groups by, and
// error.type would grow one value per unreachable endpoint.
func TestNewTransport_ATransportFailureIsBoundedAndSaysNothingAboutTheAddress(t *testing.T) {
	recorder := newRecorder(t)
	reader, restore := newMetricRecorder(t)
	defer restore()

	failure := errors.New("dial tcp 10.1.2.3:443: connect: connection refused")
	transport := NewTransport(roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, failure
	}))

	ctx, parent := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).
		Tracer("test").Start(context.Background(), "tools/call")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://gitlab.example/api/v4/user", nil)

	resp, err := transport.RoundTrip(req)
	if err == nil {
		t.Fatal("the transport swallowed the error")
	}
	if resp != nil {
		_ = resp.Body.Close()
	}
	parent.End()

	span := findSpan(t, recorder, http.MethodGet)

	if span.Status().Code != codes.Error {
		t.Error("a transport failure was not recorded as one; no response arrived at all")
	}
	value, ok := attrOf(span, attrErrorTypeForTransport)
	if !ok {
		t.Fatal("error.type is absent for a transport failure")
	}
	if value.AsString() != ErrorTypeOther {
		t.Errorf("error.type = %q, want the bounded %q: a dial error's text names the address it could not reach",
			value.AsString(), ErrorTypeOther)
	}
	if span.Status().Description != "" {
		t.Errorf("the status description is %q; it would carry the address", span.Status().Description)
	}

	t.Run("the failure is measured too", func(t *testing.T) {
		if instrument, _ := metricCarryingValue(t, reader, ErrorTypeOther); instrument == "" {
			t.Error("no metric records the failure, so an operator counting GitLab errors sees none")
		}
	})
}

// findSpan returns the one recorded span with the given name.
func findSpan(t *testing.T, recorder *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, span := range recorder.Ended() {
		if span.Name() == name {
			return span
		}
	}
	t.Fatalf("no span named %q was recorded", name)
	return nil
}
