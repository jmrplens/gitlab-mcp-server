package telemetry

import (
	"context"
	"os"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// The exporters are constructed with no options on purpose.
//
// Each one reads the standard OTEL_EXPORTER_OTLP_* environment itself:
// endpoint, headers, compression, timeout, and TLS material. Passing our own
// options would mean either duplicating that surface or, worse, overriding
// parts of it, so that an operator setting OTEL_EXPORTER_OTLP_ENDPOINT would
// find it silently ignored. The one decision this server makes is which
// transport to construct, because the Go exporters are separate packages and
// nothing in them reads OTEL_EXPORTER_OTLP_PROTOCOL to choose between them.

// newTraceExporter builds the span exporter for the selected protocol.
func newTraceExporter(ctx context.Context, protocol string) (sdktrace.SpanExporter, error) {
	if protocol == ProtocolGRPC {
		return otlptracegrpc.New(ctx)
	}
	return otlptracehttp.New(ctx)
}

// newMetricExporter builds the metric exporter for the selected protocol.
func newMetricExporter(ctx context.Context, protocol string) (sdkmetric.Exporter, error) {
	if protocol == ProtocolGRPC {
		return otlpmetricgrpc.New(ctx)
	}
	return otlpmetrichttp.New(ctx)
}

// newLogExporter builds the log record exporter for the selected protocol.
func newLogExporter(ctx context.Context, protocol string) (sdklog.Exporter, error) {
	if protocol == ProtocolGRPC {
		return otlploggrpc.New(ctx)
	}
	return otlploghttp.New(ctx)
}

// endpointFromEnv reports the collector address this deployment exports to, for
// the server card only.
//
// It reads the same variables the exporters do and applies the same precedence,
// the signal-specific one first. It is deliberately a report rather than a
// decision: nothing here is passed to an exporter, so a disagreement between
// this and what the SDK resolves can misinform the card but cannot misdirect a
// single span.
//
// Empty means the exporters fall back to their own default, localhost on the
// standard port, which is a collector on the same host and not worth
// publishing as an address.
// endpointForSignal resolves the endpoint one signal will actually use.
//
// The signal-specific variable wins over the shared one, which is the
// precedence the configuration specification defines, and the same order the
// exporters themselves apply. Reading only the traces variable, as this did,
// described a metrics-only deployment by a value it would never use.
func endpointForSignal(signalKey string) string {
	for _, key := range []string{signalKey, "OTEL_EXPORTER_OTLP_ENDPOINT"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
