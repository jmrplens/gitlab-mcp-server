//go:build collectore2e

package collectore2e

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The OTLP log documents the file exporter writes, decoded to the fields under
// assertion. Hand-written for the same reason the trace and metric types are:
// importing the collector's own types to check the collector's own output would
// make both halves agree by construction.
type (
	otlpLogRecord struct {
		TimeUnixNano string `json:"timeUnixNano"`
		SeverityText string `json:"severityText"`
		Body         struct {
			StringValue string `json:"stringValue"`
		} `json:"body"`
		Attributes []otlpAttr `json:"attributes"`
		TraceID    string     `json:"traceId"`
		SpanID     string     `json:"spanId"`
	}

	otlpScopeLogs struct {
		Scope      otlpScope       `json:"scope"`
		LogRecords []otlpLogRecord `json:"logRecords"`
	}

	otlpResourceLogs struct {
		Resource  otlpResource    `json:"resource"`
		ScopeLogs []otlpScopeLogs `json:"scopeLogs"`
	}

	logDocument struct {
		ResourceLogs []otlpResourceLogs `json:"resourceLogs"`
	}
)

// awaitLog waits for a log record whose body matches, and returns it.
func (c *collector) awaitLog(t *testing.T, within time.Duration, match func(otlpLogRecord) bool) (otlpLogRecord, bool) {
	t.Helper()

	var seen []string
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		seen = nil
		for _, doc := range documents[logDocument](t, filepath.Join(c.outDir, logsFile)) {
			for _, resourceLogs := range doc.ResourceLogs {
				for _, scopeLogs := range resourceLogs.ScopeLogs {
					for _, record := range scopeLogs.LogRecords {
						seen = append(seen, record.Body.StringValue)
						if match(record) {
							return record, true
						}
					}
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("waited %s; the collector parsed these log bodies: %v", within, seen)
	return otlpLogRecord{}, false
}

// TestRealCollector_LogsArriveAndNameTheSpanTheyHappenedIn is the end-to-end
// form of the defect that motivated the log signal's existence.
//
// A collector receiving real traffic showed 235 exported records with not one
// trace id among them. The bridge was correct and every caller was not: the
// whole server logged through the context-free slog family, which passes
// context.Background(), so no record could ever name the span it happened
// inside. That is the only thing the OTLP log leg does that stderr cannot, so
// the signal was being paid for and delivering nothing.
//
// Nothing here could see it. The unit test that now covers it asserts on a
// record the bridge produced; this asserts on a record a real collector parsed,
// against a span the same collector parsed, which is the join an operator
// actually performs. The comment on this module's collector configuration says
// as much: "the suite passes today with the logs pipeline removed".
func TestRealCollector_LogsArriveAndNameTheSpanTheyHappenedIn(t *testing.T) {
	c := startCollector(t)
	srv := startServer(t, telemetryEnv(c), "--gitlab-url="+startFakeGitLab(t))

	const action = "issue.list"
	for i := range 5 {
		srv.callAction(t, i+1, action, "some-group/some-project")
	}

	record, ok := c.awaitLog(t, exportDeadline, func(r otlpLogRecord) bool {
		return strings.Contains(r.Body.StringValue, "tool call") && r.TraceID != ""
	})
	if !ok {
		t.Fatalf("no correlated tool-call log record reached the collector.\nCollector:\n%s\nServer:\n%s",
			c.containerLogs(t), srv.logs())
	}

	t.Run("the record carries a span, not only a trace", func(t *testing.T) {
		if record.SpanID == "" {
			t.Error("the record names a trace and no span, so it can be found in the trace and not against the operation")
		}
	})

	t.Run("the trace it names was really exported", func(t *testing.T) {
		// The join, which is the whole point. A record carrying a trace id that
		// belongs to no exported span would look correlated in every listing
		// and resolve to nothing in a backend.
		_, _, found := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
			return s.TraceID == record.TraceID && s.SpanID == record.SpanID
		})
		if !found {
			t.Errorf("no exported span has trace %s span %s; the record points at nothing",
				record.TraceID, record.SpanID)
		}
	})

	t.Run("the record is one this server wrote", func(t *testing.T) {
		if record.SeverityText == "" {
			t.Error("the record has no severity, so a backend cannot filter it")
		}
		if _, present := attr(record.Attributes, "tool"); !present {
			t.Errorf("the record carries no tool attribute; recorded %v", keys(record.Attributes))
		}
	})
}

// TestRealCollector_TheSpanTreeIsWhatTheConventionDescribes asserts the shape
// three separate instrumentations produce together, which no one of them can be
// tested for alone.
//
// One HTTP server span per request, the MCP operation inside it, and the GitLab
// call inside that. Each piece is covered by its own unit test and the tree is
// the thing that silently degrades: passing the wrong context onward compiles,
// runs, and yields a flat trace where every GitLab call is a root, which reads
// as a working system until somebody tries to find out what a slow request was
// waiting on.
func TestRealCollector_TheSpanTreeIsWhatTheConventionDescribes(t *testing.T) {
	c := startCollector(t)
	srv := startServer(t, telemetryEnv(c), "--gitlab-url="+startFakeGitLab(t))

	for i := range 5 {
		srv.callAction(t, i+1, "issue.list", "some-group/some-project")
	}

	_, mcpSpan, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		return strings.HasPrefix(s.Name, "tools/call")
	})
	if !ok {
		t.Fatalf("the collector parsed no tools/call span.\nCollector:\n%s\nServer:\n%s",
			c.containerLogs(t), srv.logs())
	}

	t.Run("the MCP span hangs off the HTTP span", func(t *testing.T) {
		if mcpSpan.ParentSpanID == "" {
			t.Fatal("the MCP span is a root; the HTTP middleware's context did not reach the MCP middleware")
		}
		parent, found := c.spanByID(t, mcpSpan.TraceID, mcpSpan.ParentSpanID)
		if !found {
			t.Fatalf("the MCP span's parent %s was never exported", mcpSpan.ParentSpanID)
		}
		if parent.Kind != spanKindServer {
			t.Errorf("the parent's kind is %d, want %d: an MCP request should sit under an HTTP server span",
				parent.Kind, spanKindServer)
		}
		if method, _ := attr(parent.Attributes, "http.request.method"); method != http.MethodPost {
			t.Errorf("the parent records http.request.method = %q, want POST", method)
		}
	})

	t.Run("the GitLab call hangs off the MCP span", func(t *testing.T) {
		client, found := c.childOf(t, mcpSpan.TraceID, mcpSpan.SpanID, spanKindClient)
		if !found {
			t.Fatal("no client span is a child of the MCP span; a GitLab call parented elsewhere is a flat trace")
		}
		if host, present := attr(client.Attributes, "server.address"); !present || host == "" {
			t.Errorf("the client span names no server.address; recorded %v", keys(client.Attributes))
		}
		// The privacy rule, asserted where it is actually enforced rather than
		// only where it is written down: a GitLab URL carries project paths.
		if value, present := attr(client.Attributes, "url.full"); present {
			t.Errorf("the client span carries url.full = %q; a GitLab URL names the project", value)
		}
	})
}

// spanByID finds one exported span by its identifiers.
func (c *collector) spanByID(t *testing.T, traceID, spanID string) (otlpSpan, bool) {
	t.Helper()
	_, span, ok := c.awaitSpan(t, exportDeadline/6, func(_ otlpResourceSpans, s otlpSpan) bool {
		return s.TraceID == traceID && s.SpanID == spanID
	})
	return span, ok
}

// childOf finds a span of the given kind whose parent is the one named.
func (c *collector) childOf(t *testing.T, traceID, parentID string, kind int) (otlpSpan, bool) {
	t.Helper()
	_, span, ok := c.awaitSpan(t, exportDeadline/6, func(_ otlpResourceSpans, s otlpSpan) bool {
		return s.TraceID == traceID && s.ParentSpanID == parentID && s.Kind == kind
	})
	return span, ok
}

// spanKindClient is SPAN_KIND_CLIENT in the OTLP enum.
const spanKindClient = 3
