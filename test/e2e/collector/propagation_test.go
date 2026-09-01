//go:build collectore2e

package collectore2e

import (
	"net/http"
	"strings"
	"testing"
)

// The trace a client claims to be part of. Fixed rather than generated, so a
// failure names the value that was supposed to arrive.
const (
	clientTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	clientSpanID  = "00f067aa0ba902b7"
	clientTrace   = "00-" + clientTraceID + "-" + clientSpanID + "-01"
)

// TestRealCollector_TraceContextArrivesInMetaAndTheHTTPSpanIsLinked covers the
// propagation rule, which has two halves and had one.
//
// The convention asks a server to parent its MCP span on the context from
// params._meta and to link the ambient one. Extract replaces the ambient
// context, so an implementation that reads it afterwards finds the extracted
// one and links nothing: the parenting looked right and the HTTP request that
// carried the call disappeared from the record.
//
// The keys are unprefixed, which is the exception MCP grants: "the keys
// traceparent, tracestate, and baggage are reserved for OpenTelemetry trace
// context propagation". A prefixed variant would be wrong rather than merely
// unusual, so this drives the real one.
func TestRealCollector_TraceContextArrivesInMetaAndTheHTTPSpanIsLinked(t *testing.T) {
	c := startCollector(t)
	srv := startServer(t, telemetryEnv(c), "--gitlab-url="+startFakeGitLab(t))

	for i := range 3 {
		srv.callWithTraceContext(t, i+1, clientTrace)
	}

	_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		return strings.HasPrefix(s.Name, "tools/call")
	})
	if !ok {
		t.Fatalf("no tools/call span.\nCollector:\n%s\nServer:\n%s", c.containerLogs(t), srv.logs())
	}

	t.Run("the MCP span joins the caller's trace", func(t *testing.T) {
		if span.TraceID != clientTraceID {
			t.Errorf("trace = %s, want the caller's %s: the context in params._meta was not adopted",
				span.TraceID, clientTraceID)
		}
		if span.ParentSpanID != clientSpanID {
			t.Errorf("parent = %s, want the caller's span %s", span.ParentSpanID, clientSpanID)
		}
	})

	t.Run("the HTTP span it arrived on is linked", func(t *testing.T) {
		if len(span.Links) != 1 {
			t.Fatalf("recorded %d links, want 1: the ambient HTTP span is dropped rather than linked, so nothing records which request carried this call",
				len(span.Links))
		}
		link := span.Links[0]
		if link.TraceID == clientTraceID {
			t.Error("the link points into the caller's trace; it should point at this server's own HTTP span")
		}
		if _, found := c.spanByID(t, link.TraceID, link.SpanID); !found {
			t.Errorf("the link names span %s in trace %s, which was never exported", link.SpanID, link.TraceID)
		}
	})
}

// TestRealCollector_AnInventedHTTPMethodIsBounded covers the substitution the
// HTTP convention requires and the cardinality hole it closes here.
//
// net/http accepts any token as a method, so that string is chosen by the
// caller, and it sat on the one instrument every request touches. A client
// could mint one time series per invented verb until the budget ran out, and
// the SDK answers an exhausted budget by collapsing everything past the limit
// into otel.metric.overflow rather than refusing it.
func TestRealCollector_AnInventedHTTPMethodIsBounded(t *testing.T) {
	c := startCollector(t)
	srv := startServer(t, telemetryEnv(c), "--gitlab-url="+startFakeGitLab(t))

	const invented = "FROBNICATE"
	srv.rawRequest(t, invented, "/mcp")

	_, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		value, _ := attr(s.Attributes, "http.request.method_original")
		return value == invented
	})
	if !ok {
		t.Fatalf("no span records http.request.method_original; nothing says what the client sent.\nCollector:\n%s\nServer:\n%s",
			c.containerLogs(t), srv.logs())
	}

	t.Run("the recorded method is the bucket", func(t *testing.T) {
		if got, _ := attr(span.Attributes, "http.request.method"); got != "_OTHER" {
			t.Errorf("http.request.method = %q, want _OTHER", got)
		}
		// Stated separately from the attribute by the convention, and the
		// distinction is real: a span name is a label a backend groups by, and
		// _OTHER there reads as a method rather than as the absence of one.
		if span.Name != "HTTP" {
			t.Errorf("span name = %q, want HTTP", span.Name)
		}
	})

	t.Run("the verb never reaches a metric", func(t *testing.T) {
		if _, _, found := c.awaitMetric(t, exportDeadline, "http.server.request.duration"); !found {
			t.Fatal("no http.server.request.duration metric, so this would pass vacuously")
		}
		if instrument, key := metricValueExists(t, c, invented); instrument != "" {
			t.Errorf("%s carries the caller's verb on %s; a client then chooses the label space", key, instrument)
		}
		if instrument, _ := metricValueExists(t, c, "_OTHER"); instrument == "" {
			t.Error("no metric carries _OTHER, so the request was not measured at all")
		}
	})
}

// rawRequest sends a bare request with an arbitrary method, which is the shape
// this server must survive rather than serve.
func (s *server) rawRequest(t *testing.T, method, path string) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, s.baseURL+path, nil)
	if err != nil {
		t.Fatalf("building the %s request: %v", method, err)
	}
	req.Header.Set("PRIVATE-TOKEN", "glpat-collector-e2e-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	// The status is not the subject: whatever the server answers, the question
	// is what it recorded about being asked.
}
