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

// ServerMiddleware instruments the HTTP layer, outside authentication.
//
// # What it adds that the MCP span does not
//
// The MCP span starts after the credential check, so it never exists for a
// request that was refused. That is deliberate, and it leaves an operator of a
// published endpoint unable to see the thing they most need to watch: how much
// traffic is being rejected, and how long the rejection takes. This span covers
// host validation, CORS, the credential check and the handler, so a refusal is
// visible as a status code without ever reaching the MCP instrumentation.
//
// # Why not otelhttp
//
// The contrib middleware records url.full and derives server.address from the
// client-controlled Host header, and the convention attaches a warning to that
// second one: "Since this attribute is based on HTTP headers, opting in to it
// may allow an attacker to trigger cardinality limits, degrading the usefulness
// of the metric." It also reads client.address from X-Forwarded-For with no
// allow-list, which ignores this server's own --trusted-proxy-header setting
// and would put an attacker-chosen string on every span.
//
// Each of those is fixable with an option or a SpanProcessor. Together they are
// more configuration than the thing being configured, and every one of them
// fails open: forget the option and the attribute ships.
//
// # No route, and why that is not the loss it looks like
//
// http.route would need the pattern the mux matched, and net/http sets that on
// the request it passes downward rather than on the one this middleware holds.
// Recording url.path instead would be worse than nothing on a published
// endpoint: the path is whatever a scanner sends, so /wp-admin.php and ten
// thousand friends would each mint a series.
//
// This server serves a handful of fixed paths, so method and status answer the
// questions an HTTP-level view is for: request rate, error rate, latency, and
// how much of it is being refused. What was called is on the MCP span.
func ServerMiddleware(next http.Handler) http.Handler {
	tracer := otel.Tracer(scopeName)
	duration := newHTTPServerDurationHistogram(otel.Meter(scopeName))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attrs := []attribute.KeyValue{
			attrHTTPRequestMethod.String(r.Method),
			attrURLScheme.String(requestScheme(r)),
			attrNetworkProtocolName.String("http"),
		}

		ctx, span := tracer.Start(r.Context(), r.Method,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attrs...),
		)
		defer span.End()

		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		started := time.Now()
		next.ServeHTTP(recorder, r.WithContext(ctx))
		elapsed := time.Since(started).Seconds()

		status := attrHTTPResponseStatus.Int(recorder.status)
		span.SetAttributes(status)

		// No span status is set for a 4xx, and that is the convention rather
		// than leniency: "For HTTP status codes in the 4xx range span status
		// MUST be left unset in case of SpanKind.SERVER". A refused credential
		// is the server working correctly. 5xx is ours, and takes the status.
		if recorder.status >= http.StatusInternalServerError {
			// No description: the status code is already recorded, and any
			// text here would come from a handler this middleware cannot see,
			// which is exactly where a GitLab error body could leak in.
			span.SetStatus(codes.Error, "")
		}

		duration.Record(ctx, elapsed, metric.WithAttributes(append(attrs, status)...))
	})
}

// requestScheme reports http or https without trusting a client-set header.
//
// X-Forwarded-Proto is deliberately not consulted. It is attacker-controlled on
// any deployment reachable without a proxy, and the value it would produce is
// cosmetic: nothing here branches on the scheme, so a wrong one misinforms a
// dashboard rather than changing behavior. TLS is read from the connection,
// which cannot be spoofed by a header.
func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// statusRecorder remembers the status code the handler chain wrote.
//
// Defaulting to 200 matches net/http: a handler that writes a body without
// calling WriteHeader has sent a 200. Without the default, every successful
// request would record a status of zero.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if !r.written {
		r.status = status
		r.written = true
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.written = true
	return r.ResponseWriter.Write(b)
}

// Flush forwards to the underlying writer when it supports flushing.
//
// Not optional here. This server's default HTTP mode answers with
// text/event-stream, and an SSE response that is never flushed is a response
// the client never sees: wrapping the ResponseWriter without forwarding Flush
// would turn every streaming response into a hang.
func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// newHTTPServerDurationHistogram builds the Stable server-side instrument.
func newHTTPServerDurationHistogram(meter metric.Meter) metric.Float64Histogram {
	histogram, err := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of an inbound HTTP request, including the credential check."),
		metric.WithExplicitBucketBoundaries(
			0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10,
		),
	)
	if err != nil {
		otel.Handle(err)
	}
	return histogram
}
