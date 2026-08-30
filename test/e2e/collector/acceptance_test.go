//go:build collectore2e

// acceptance_test.go asks the one question the in-process OTLP stub in
// test/e2e/http cannot: does a real receiver accept this, and does what it
// parses out mean anything.
package collectore2e

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// telemetryEnv points a server at the collector and makes it export promptly.
//
// The timeouts are integers because the specification defines every OTEL_ one
// as an integer number of milliseconds. Writing "200ms" parses as nothing and
// silently keeps the ten-second default, which here means a test that times out
// with no export to look at and no hint as to why.
func telemetryEnv(c *collector) map[string]string {
	return map[string]string{
		"GITLAB_MCP_TELEMETRY":        "true",
		"OTEL_EXPORTER_OTLP_ENDPOINT": c.endpoint,
		"OTEL_EXPORTER_OTLP_TIMEOUT":  "2000",
		"OTEL_BSP_SCHEDULE_DELAY":     "100",
		"OTEL_METRIC_EXPORT_INTERVAL": "100",
	}
}

// exportDeadline bounds every wait for parsed telemetry.
//
// It is long because three schedules have to line up before a document exists
// to read: the span batcher, the metric reader, and the collector's own file
// flush. It is bounded because the alternative to a deadline here is a suite
// that hangs when the server exports nothing at all, which is the single most
// likely thing to break.
const exportDeadline = 60 * time.Second

// durationMetric is the instrument the MCP semantic convention defines for a
// server's request duration.
const durationMetric = "mcp.server.operation.duration"

// TestRealCollector_AcceptsAndParsesWhatThisServerEmits drives real traffic
// into a real OpenTelemetry Collector and asserts on what came out the far side
// of its pipeline.
//
// # The failure this exists to prevent
//
// Every telemetry test in this repository until now has been graded by code we
// wrote. The in-process receiver in test/e2e/http answers 200 to whatever it is
// handed and stores the bytes, which is exactly right for the two questions it
// asks (was the credential sent, did anything private leak) and is no evidence
// at all that the export is well formed. A protobuf we encode wrongly, a
// resource missing an attribute a backend requires, a metric whose unit
// contradicts its name: all three pass a stub, and all three ship a server
// whose telemetry an operator cannot use, with a green suite behind them.
//
// A collector is not a stub. It decodes the protobuf, builds pdata out of it,
// routes it through a pipeline and re-encodes it, and every one of those steps
// can reject. So the assertions below are deliberately about acceptance and
// shape rather than about content: that the export was taken without complaint,
// that a span survived with the kind and the name the convention requires, that
// the attributes an operator groups by are on it, that the duration metric
// arrived in the unit its name promises, and that the resource says which
// service and which instance produced all of it.
//
// The subtests share one collector, one server and one burst of traffic. They
// are facts about a single export rather than independent scenarios, and
// restarting the stack six times to assert them separately would buy nothing
// but five more container starts.
func TestRealCollector_AcceptsAndParsesWhatThisServerEmits(t *testing.T) {
	c := startCollector(t)
	srv := startServer(t, telemetryEnv(c), "--gitlab-url="+startFakeGitLab(t))

	// A canonical catalog action, because on the default surface the tool name
	// is gitlab_execute_action for every call and gitlab_mcp.action is the only
	// thing on the span that says what was done. Several calls, so a batch is
	// certainly non-empty rather than probably.
	const action = "issue.list"
	for i := range 5 {
		srv.callAction(t, i+1, action, "some-group/some-project")
	}

	// Both logs go into either failure message. A collector that refused the
	// export and a server that never sent one present identically from here, as
	// an empty file, and only the two logs together tell them apart.
	spanResource, span, ok := c.awaitSpan(t, exportDeadline, func(_ otlpResourceSpans, s otlpSpan) bool {
		return strings.HasPrefix(s.Name, "tools/call")
	})
	if !ok {
		t.Fatalf("the collector parsed no tools/call span.\nCollector:\n%s\nServer:\n%s", c.containerLogs(t), srv.logs())
	}

	metricResource, duration, ok := c.awaitMetric(t, exportDeadline, durationMetric)
	if !ok {
		t.Fatalf("the collector parsed no %s metric.\nCollector:\n%s\nServer:\n%s", durationMetric, c.containerLogs(t), srv.logs())
	}

	t.Run("the collector accepted every export without complaint", func(t *testing.T) {
		assertNothingWasRefused(t, c, srv)
	})
	t.Run("a server span is named for the MCP method", func(t *testing.T) {
		assertServerSpan(t, span)
	})
	t.Run("the span carries the method and the canonical action", func(t *testing.T) {
		assertSpanNamesTheOperation(t, span, action)
	})
	t.Run("the duration metric arrives in seconds", func(t *testing.T) {
		assertDurationInSeconds(t, duration)
	})
	t.Run("the resource names the service and the instance", func(t *testing.T) {
		// Asserted on both signals rather than once. They are built from one
		// resource, so a divergence means the resource is being assembled per
		// provider, and a metric that cannot be joined to the trace it came
		// from is most of the value of having both.
		assertResourceIdentifiesTheProcess(t, "traces", spanResource.Resource.Attributes)
		assertResourceIdentifiesTheProcess(t, "metrics", metricResource.Resource.Attributes)
	})
	t.Run("the spans are attributed to this server's instrumentation scope", func(t *testing.T) {
		assertInstrumentationScope(t, spanResource)
	})
}

// assertNothingWasRefused reads the same fact from both ends of the export.
//
// Either reading alone can be wrong in a way the other catches. The collector
// says whether it refused anything it was sent; the server says whether
// anything it sent came back as an error, which covers a refusal the collector
// recorded at a level this scan does not look at, or did not log at all.
func assertNothingWasRefused(t *testing.T, c *collector, srv *server) {
	t.Helper()

	for line := range strings.SplitSeq(c.containerLogs(t), "\n") {
		// The collector's console encoding is tab-separated with the level
		// second, so matching on delimited fields cannot be tripped by the word
		// "error" appearing inside a message.
		if strings.Contains(line, "\terror\t") || strings.Contains(line, "\tfatal\t") {
			t.Errorf("the collector logged a failure while receiving our export:\n%s", line)
		}
	}

	// The server routes every OTel SDK error to its own logger, so a rejected
	// export is visible from this side too, and is the sharper signal: it means
	// a batch came back as something other than success.
	if logs := srv.logs(); strings.Contains(logs, "opentelemetry sdk error") {
		t.Errorf("the server reported an SDK error, so the collector did not accept what it sent:\n%s", logs)
	}
}

// assertServerSpan pins the name and the kind, which are what a backend files a
// span by before anybody looks at an attribute.
func assertServerSpan(t *testing.T, span otlpSpan) {
	t.Helper()

	// The convention's rule is "{mcp.method.name} {target}", with the tool as
	// the target for a call. A span the collector will not parse has no name at
	// all here, so this asserts the parse as much as the name.
	if want := "tools/call gitlab_execute_action"; span.Name != want {
		t.Errorf("span name is %q, want %q", span.Name, want)
	}
	// SERVER, because this process is the one being called. A client-kind or
	// internal span here would put every MCP operation in the wrong half of any
	// service map built from these traces.
	if span.Kind != spanKindServer {
		t.Errorf("span kind is %d, want %d (SPAN_KIND_SERVER)", span.Kind, spanKindServer)
	}
}

// assertSpanNamesTheOperation pins the two attributes an operator groups by.
func assertSpanNamesTheOperation(t *testing.T, span otlpSpan, action string) {
	t.Helper()

	if got, present := attr(span.Attributes, "mcp.method.name"); !present || got != "tools/call" {
		t.Errorf("mcp.method.name is %q (present=%t), want %q. The span carries %v",
			got, present, "tools/call", keys(span.Attributes))
	}
	// Without this attribute the default surface's traces cannot tell listing
	// issues from deleting a branch: gen_ai.tool.name is gitlab_execute_action
	// for both.
	if got, present := attr(span.Attributes, "gitlab_mcp.action"); !present || got != action {
		t.Errorf("gitlab_mcp.action is %q (present=%t), want %q. The span carries %v",
			got, present, action, keys(span.Attributes))
	}
}

// assertDurationInSeconds pins the unit, and that there is a measurement in it.
func assertDurationInSeconds(t *testing.T, duration otlpMetric) {
	t.Helper()

	// The unit is the assertion that matters. This server logs durations in
	// milliseconds everywhere else, the convention fixes this instrument at
	// seconds, and a mismatch is invisible in a dashboard until somebody reads
	// a p99 of 400 and cannot tell which it is.
	if duration.Unit != "s" {
		t.Errorf("%s has unit %q, want %q", durationMetric, duration.Unit, "s")
	}
	// A histogram with no points would satisfy the unit check while carrying no
	// measurement, which is the shape a broken instrument takes rather than an
	// absent one.
	if duration.Histogram == nil {
		t.Fatalf("%s arrived without histogram data; the convention defines it as a histogram", durationMetric)
	}
	if len(duration.Histogram.DataPoints) == 0 {
		t.Errorf("%s arrived with no data points, after five calls were served", durationMetric)
	}
}

// assertResourceIdentifiesTheProcess pins what every signal has to say about
// where it came from.
func assertResourceIdentifiesTheProcess(t *testing.T, signal string, attrs []otlpAttr) {
	t.Helper()

	// service.name is what every backend groups by, and a resource without one
	// is filed under "unknown_service" beside every other unlabeled process
	// reporting to the same collector.
	if got, present := attr(attrs, "service.name"); !present || got == "" {
		t.Errorf("the %s resource has no service.name; it carries %v", signal, keys(attrs))
	}
	// service.instance.id is what separates two replicas of this server from
	// one replica serving twice the traffic.
	if _, present := attr(attrs, "service.instance.id"); !present {
		t.Errorf("the %s resource has no service.instance.id; it carries %v", signal, keys(attrs))
	}
}

// assertInstrumentationScope pins who the spans say instrumented them.
//
// The scope names the code doing the instrumenting, which is how an operator
// tells our spans from those of a library that also instruments this process.
// An empty scope makes that impossible, and is what an incorrectly built
// provider produces.
func assertInstrumentationScope(t *testing.T, resourceSpans otlpResourceSpans) {
	t.Helper()

	const want = "github.com/jmrplens/gitlab-mcp-server/v2/internal/mcpotel"
	scopes := make([]string, 0, len(resourceSpans.ScopeSpans))
	for _, scopeSpans := range resourceSpans.ScopeSpans {
		scopes = append(scopes, scopeSpans.Scope.Name)
	}
	if !slices.Contains(scopes, want) {
		t.Errorf("no scope named %q among %v", want, scopes)
	}
}
