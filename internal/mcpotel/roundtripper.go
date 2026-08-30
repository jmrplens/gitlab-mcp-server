package mcpotel

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Attribute keys for an outbound HTTP call, from the Stable HTTP conventions.
const (
	attrHTTPRequestMethod     = attribute.Key("http.request.method")
	attrHTTPResponseStatus    = attribute.Key("http.response.status_code")
	attrServerAddress         = attribute.Key("server.address")
	attrNetworkProtocolName   = attribute.Key("network.protocol.name")
	attrURLScheme             = attribute.Key("url.scheme")
	attrErrorTypeForTransport = AttrErrorType
)

// NewTransport wraps a RoundTripper so every GitLab API call becomes a child
// span of whatever MCP operation caused it.
//
// # Why not otelhttp
//
// The contrib instrumentation is the obvious choice and it records url.full,
// which for this server means a span carrying the project path, the group path,
// and the contents of every search query. That is the same category of data this
// project already declined to record as tool arguments, and declining it there
// while shipping it here through a different door would be worse than not
// instrumenting at all: it would look like a considered privacy position while
// being none.
//
// Redacting url.full afterwards is possible, with a SpanProcessor rewriting it
// at OnStart, and it is more machinery than the value justifies. So this records
// what an operator actually needs and nothing else.
//
// # What a trace shows without the path
//
// It is a smaller loss than it sounds, because the parent span already names the
// operation: gitlab_mcp.action carries issue.list or branch.delete, which says
// which endpoint family was called far more legibly than a URL would. The child
// spans then answer the questions the parent cannot: how many round trips one
// tool call took, how long each took, which one failed, and whether a retry
// happened. Pagination showing up as eleven children is exactly the kind of
// thing that is invisible in a log and obvious in a trace.
//
// # Errors
//
// A transport error is a failure. A 4xx or 5xx is NOT marked as a span error
// here, deliberately: "For HTTP status codes in the 4xx range span status ...
// SHOULD be set to Error in case of SpanKind.CLIENT", which would make every
// expected 404 from a not-found probe a red span. This server treats a 404 as
// an answer rather than a failure in its own handlers, and the span should
// agree with the handler rather than with a rule written for a generic client.
// The status code is always recorded, so a dashboard can classify however it
// likes.
func NewTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &instrumentedTransport{
		base:     base,
		tracer:   otel.Tracer(scopeName),
		duration: newHTTPClientDurationHistogram(otel.Meter(scopeName)),
	}
}

type instrumentedTransport struct {
	base     http.RoundTripper
	tracer   trace.Tracer
	duration metric.Float64Histogram
}

func (t *instrumentedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// The convention's client span name is the method alone when there is no
	// low-cardinality route template, and there is none here: every GitLab path
	// carries a project or group identifier, so a name built from one would
	// mint a distinct span name per project.
	attrs := []attribute.KeyValue{
		attrHTTPRequestMethod.String(req.Method),
		attrServerAddress.String(req.URL.Hostname()),
		attrURLScheme.String(req.URL.Scheme),
		attrNetworkProtocolName.String("http"),
	}

	ctx, span := t.tracer.Start(req.Context(), req.Method,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
	defer span.End()

	started := time.Now()
	// Clone rather than mutate: a RoundTripper "should not modify the request",
	// and client-go reuses request objects across its own retry loop.
	resp, err := t.base.RoundTrip(req.WithContext(ctx))
	elapsed := time.Since(started).Seconds()

	metricAttrs := attrs
	switch {
	case err != nil:
		// A transport error, which is the only failure this layer treats as
		// one: no response arrived at all. The error text is not recorded,
		// because it carries addresses and would make error.type unbounded.
		span.SetStatus(codes.Error, "")
		span.SetAttributes(attrErrorTypeForTransport.String(ErrorTypeOther))
		metricAttrs = append(metricAttrs, attrErrorTypeForTransport.String(ErrorTypeOther))
	case resp != nil:
		status := attrHTTPResponseStatus.Int(resp.StatusCode)
		span.SetAttributes(status)
		metricAttrs = append(metricAttrs, status)
	}

	t.duration.Record(ctx, elapsed, metric.WithAttributes(metricAttrs...))
	return resp, err
}

// newHTTPClientDurationHistogram builds the Stable HTTP client instrument.
//
// The boundaries are the convention's own for this metric, which are tighter
// than the MCP operation ones because a single API call is expected to be
// faster than the tool call containing it.
func newHTTPClientDurationHistogram(meter metric.Meter) metric.Float64Histogram {
	histogram, err := meter.Float64Histogram(
		"http.client.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of an outbound GitLab API call."),
		metric.WithExplicitBucketBoundaries(
			0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10,
		),
	)
	if err != nil {
		otel.Handle(err)
	}
	return histogram
}
