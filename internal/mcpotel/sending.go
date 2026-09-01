package mcpotel

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// SendingMiddleware instruments the requests and notifications this server
// initiates: elicitation/create, sampling/createMessage, roots/list, and the
// notifications/* family including resources/updated and the list_changed set.
//
// # Why a second middleware rather than one
//
// The MCP convention splits client and server spans by INITIATOR, not by role.
// This server is the receiver for a tools/call and the initiator for an
// elicitation, so the same process produces both kinds, and they do not share
// rules:
//
//   - Kind is CLIENT here and SERVER there.
//   - Error classification is stricter here. "All JSON-RPC error codes SHOULD be
//     considered errors" on the client side, while the server side exempts five
//     caller-fault codes. The convention says this twice, in the client span
//     note and the client metric note, so it is a decision rather than an
//     editing slip: a code the caller is responsible for is not the receiver's
//     failure, but when WE are the caller every refusal is ours to notice.
//
// Folding the two into one function with a boolean would put those differences
// behind a flag, which is how one of them eventually gets applied to the wrong
// side.
//
// # Trace context is not injected outward
//
// A span is recorded here and nothing is written into the outgoing message's
// _meta. Injecting would let the client join our trace, which is the textbook
// reason to propagate, and it would also hand every caller the identifiers of
// this server's internal spans. On stdio that is harmless, since the client and
// the server share a principal. On a shared HTTP endpoint it is the outward
// leak the W3C security section warns about, and it is the same judgement
// already made for baggage in [OutboundContext]. One rule, both directions.
func SendingMiddleware(opts Options) mcp.Middleware {
	tracer := otel.Tracer(scopeName)
	duration := newClientDurationHistogram(otel.Meter(scopeName))

	constant := make([]attribute.KeyValue, 0, 2)
	if opts.Surface != "" {
		constant = append(constant, AttrToolSurface.String(opts.Surface))
	}
	if opts.Transport != "" {
		constant = append(constant, AttrNetworkTransport.String(opts.Transport))
	}

	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			attrs := make([]attribute.KeyValue, 0, len(constant)+1)
			attrs = append(attrs, constant...)
			attrs = append(attrs, AttrMCPMethodName.String(method))

			ctx, span := tracer.Start(ctx, method,
				trace.WithSpanKind(trace.SpanKindClient),
				trace.WithAttributes(attrs...),
			)
			defer span.End()

			started := time.Now()
			recorded := false
			record := func(result outcome) {
				recorded = true
				result.record(span)
				duration.Record(ctx, time.Since(started).Seconds(),
					metric.WithAttributes(append(attrs, result.metricAttributes()...)...))
			}
			// A panic unwinding through here would end the span with no
			// outcome and skip the metric, so the client instrument would
			// undercount exactly the calls most worth counting. The panic
			// itself is not recovered: the span's own End records it, and
			// whoever is below this middleware decides what a panic means.
			defer func() {
				if !recorded {
					record(classifyClient(errPanicked))
				}
			}()

			res, err := next(ctx, method, req)
			record(classifyClient(err))

			return res, err
		}
	}
}

// IsNotification reports whether a method name is a notification rather than a
// request.
//
// Notifications have no response and therefore no error code, so the only
// failure they can record is a transport one. The prefix is the protocol's own
// convention and there is no other way to tell from a method name.
func IsNotification(method string) bool {
	return strings.HasPrefix(method, "notifications/")
}

// newClientDurationHistogram builds the convention's client-side duration
// instrument.
//
// Same boundaries and unit as the server one, and a separate instrument rather
// than a shared one with a direction attribute: the convention defines two
// names, and merging them would make every dashboard built on either of them
// wrong.
func newClientDurationHistogram(meter metric.Meter) metric.Float64Histogram {
	histogram, err := meter.Float64Histogram(
		"mcp.client.operation.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of an MCP request this server initiated, measured until the response or acknowledgement."),
		metric.WithExplicitBucketBoundaries(
			0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 30, 60, 120, 300,
		),
	)
	if err != nil {
		otel.Handle(err)
	}
	return histogram
}

// errPanicked stands in for an outcome when a panic unwinds through the
// middleware: there is no error value to classify, and inventing one keeps the
// classification bounded.
var errPanicked = errors.New("panic")
