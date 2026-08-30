// Package telemetry is the only place in this server that knows about
// OpenTelemetry.
//
// Everything else reaches observability through the small API here, or through
// the OTel global providers this package installs. That is deliberate: an
// instrumentation library spread across 176 packages is one nobody can remove,
// reconfigure or reason about, and the seams that matter here are few enough to
// be listed. Nothing outside this package imports go.opentelemetry.io.
//
// # Off by default, and why
//
// Telemetry stays off unless an operator turns it on, for privacy rather than
// for cost. Instrumenting a deployment is the operator's decision to make about
// their own users, and a server that traced by default would be making it for
// them. PRIVACY.md's promise is unaffected either way: it is scoped to what the
// maintainer receives, and an exporter an operator points at their own
// collector sends nothing to anyone else.
//
// When it is on, the server says so. [Snapshot] feeds the `observability` block
// of the server card, so somebody connecting to a published endpoint can see
// that their calls are instrumented rather than having to ask.
//
// # Configuration
//
// One switch is ours and the rest is the specification's. `--otel` (or
// OTEL_ENABLED) turns it on; endpoint, headers, timeouts, sampling and
// resource attributes come from the standard OTEL_* environment variables the
// exporters read themselves. Reinventing that surface would mean maintaining a
// second, worse copy of a configuration an operator already knows.
//
// # What an attribute may carry
//
// The same discipline the logs already keep, stated here because a span makes
// it easy to break: record what was called and how it ended, never what was
// passed. Tool names, action ids, outcome, duration and the identity already in
// the log line are in; parameters, queries, tokens and response bodies are out.
// The existing code is the precedent: the dynamic surface logs `query_len` and
// not the query, and the pool logs a token suffix and not the token.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

// Protocol selects the OTLP transport.
//
// The two values are the ones the specification defines for
// OTEL_EXPORTER_OTLP_PROTOCOL. `http/protobuf` is the default because it
// crosses proxies and ingress that gRPC does not, which is the common shape for
// a collector that is not on the same host.
const (
	ProtocolHTTP = "http/protobuf"
	ProtocolGRPC = "grpc"
)

// DefaultServiceName is what this server calls itself to a collector when
// OTEL_SERVICE_NAME says nothing.
const DefaultServiceName = "gitlab-mcp-server"

// shutdownTimeout bounds the final flush.
//
// A collector that has gone away must not hold up the process: the exporters
// block trying to deliver, and shutdown is exactly when a stuck export is least
// welcome. Losing the last batch is the right trade.
const shutdownTimeout = 5 * time.Second

// Config is what this server decides. Everything else is read from the standard
// OTEL_* environment by the exporters.
type Config struct {
	// Enabled turns telemetry on. False leaves the OTel globals as the noop
	// implementations the API ships with, so every instrumentation call in the
	// rest of the codebase costs a nil check.
	Enabled bool
	// Protocol is [ProtocolHTTP] or [ProtocolGRPC]. Empty means HTTP.
	Protocol string
	// ServiceName overrides OTEL_SERVICE_NAME. Empty means
	// [DefaultServiceName], or whatever the environment already says.
	ServiceName string
	// ServiceVersion is reported as service.version, so a collector can tell
	// which build produced a span.
	ServiceVersion string
	// Signals selects what is exported. A zero value means all three.
	Signals Signals
}

// Signals selects which OTel signals are exported.
//
// Separable because their costs differ: traces are per call, metrics are
// aggregated, and logs duplicate a stream the operator may already collect from
// stderr. An operator shipping stderr to a log pipeline already wants the first
// two and not the third.
type Signals struct {
	Traces  bool
	Metrics bool
	Logs    bool
}

// AllSignals is what an unset [Signals] means.
func AllSignals() Signals { return Signals{Traces: true, Metrics: true, Logs: true} }

// none reports whether nothing at all was selected.
func (s Signals) none() bool { return !s.Traces && !s.Metrics && !s.Logs }

// Provider owns the exporters and the global registrations they back.
//
// The zero value is a working disabled provider, so a caller that never started
// telemetry can still call [Provider.Shutdown] and [Provider.Snapshot] without
// a nil check.
type Provider struct {
	enabled  bool
	protocol string
	signals  Signals
	endpoint string

	shutdowns []func(context.Context) error
}

// Start installs the OTel providers and returns the handle that retires them.
//
// A disabled configuration is not an error: it returns a provider that reports
// itself disabled and does nothing, which is what every caller wants at the one
// place this is wired.
func Start(ctx context.Context, cfg Config) (*Provider, error) {
	if !cfg.Enabled {
		return &Provider{}, nil
	}

	signals := cfg.Signals
	if signals.none() {
		signals = AllSignals()
	}
	protocol, err := normalizeProtocol(cfg.Protocol)
	if err != nil {
		return nil, err
	}

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("telemetry resource: %w", err)
	}

	p := &Provider{
		enabled:  true,
		protocol: protocol,
		signals:  signals,
		endpoint: endpointFromEnv(),
	}

	if startErr := p.startTraces(ctx, res, protocol, signals); startErr != nil {
		return nil, p.abandon(ctx, startErr)
	}
	if startErr := p.startMetrics(ctx, res, protocol, signals); startErr != nil {
		return nil, p.abandon(ctx, startErr)
	}
	if startErr := p.startLogs(ctx, res, protocol, signals); startErr != nil {
		return nil, p.abandon(ctx, startErr)
	}

	// Context propagation is installed whenever anything is, so a trace begun
	// by a caller upstream of this server continues into it rather than
	// starting again. W3C first, with Baggage alongside, which is what a
	// collector and every other instrumented service expect.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return p, nil
}

// abandon retires whatever started before an error, so a partial failure does
// not leave exporters running with no owner.
func (p *Provider) abandon(ctx context.Context, cause error) error {
	if err := p.Shutdown(ctx); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

// Enabled reports whether telemetry is running.
func (p *Provider) Enabled() bool { return p != nil && p.enabled }

// Shutdown flushes and retires every exporter, bounded by [shutdownTimeout].
//
// Errors are joined rather than returned on the first failure: each exporter
// owns a connection of its own, and stopping the rest matters more than
// reporting the first one that could not.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil || len(p.shutdowns) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	var errs []error
	// Reverse order, so a signal that depends on another is retired first.
	for _, shutdown := range slices.Backward(p.shutdowns) {
		if err := shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	p.shutdowns = nil
	p.enabled = false
	return errors.Join(errs...)
}

// Snapshot describes the running configuration for the server card.
//
// It names what is on rather than what the binary can do, matching the
// subscriptions block: a consumer branches on this deployment's answer.
type Snapshot struct {
	Enabled  bool     `json:"enabled"`
	Protocol string   `json:"protocol,omitempty"`
	Signals  []string `json:"signals,omitempty"`
	// Endpoint is the collector this deployment exports to, when the standard
	// environment names one. It is the operator's own address and is published
	// so somebody connecting to a shared endpoint can see where the record of
	// their calls goes, which is the point of announcing this at all.
	Endpoint string `json:"endpoint,omitempty"`
}

// Snapshot returns what to publish about this deployment.
func (p *Provider) Snapshot() Snapshot {
	if !p.Enabled() {
		return Snapshot{Enabled: false}
	}
	var signals []string
	if p.signals.Traces {
		signals = append(signals, "traces")
	}
	if p.signals.Metrics {
		signals = append(signals, "metrics")
	}
	if p.signals.Logs {
		signals = append(signals, "logs")
	}
	return Snapshot{
		Enabled:  true,
		Protocol: p.protocol,
		Signals:  signals,
		Endpoint: p.endpoint,
	}
}

// normalizeProtocol accepts the two values OTEL_EXPORTER_OTLP_PROTOCOL defines
// and refuses anything else by name.
//
// `http` is accepted as a spelling of `http/protobuf` because operators write
// it, and refusing it would fail at startup over a shorthand. `http/json` is
// refused explicitly rather than silently treated as protobuf: the Go
// exporters do not implement it, and a collector expecting JSON would receive
// protobuf and reject every batch.
func normalizeProtocol(protocol string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(protocol)) {
	case "", "http", ProtocolHTTP:
		return ProtocolHTTP, nil
	case ProtocolGRPC:
		return ProtocolGRPC, nil
	case "http/json":
		return "", fmt.Errorf(
			"OTLP protocol %q is not implemented by the OpenTelemetry Go exporters; use %q or %q",
			"http/json", ProtocolHTTP, ProtocolGRPC,
		)
	default:
		return "", fmt.Errorf("unknown OTLP protocol %q: use %q or %q", protocol, ProtocolHTTP, ProtocolGRPC)
	}
}

// buildResource describes this process to a collector.
//
// resource.WithFromEnv reads OTEL_RESOURCE_ATTRIBUTES and OTEL_SERVICE_NAME, so
// an operator can add deployment.environment or anything else without this
// server knowing about it. The explicit attributes are merged under that, which
// means the environment wins: an operator naming the service differently across
// two deployments of the same binary is doing so on purpose.
func buildResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	name := cfg.ServiceName
	if name == "" {
		name = DefaultServiceName
	}
	attrs := []attribute.KeyValue{semconv.ServiceName(name)}
	if cfg.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.ServiceVersion))
	}
	return resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attrs...),
	)
}

// startTraces installs the tracer provider.
func (p *Provider) startTraces(ctx context.Context, res *resource.Resource, protocol string, signals Signals) error {
	if !signals.Traces {
		return nil
	}
	exporter, err := newTraceExporter(ctx, protocol)
	if err != nil {
		return fmt.Errorf("otlp trace exporter: %w", err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)
	p.shutdowns = append(p.shutdowns, provider.Shutdown)
	return nil
}

// startMetrics installs the meter provider.
func (p *Provider) startMetrics(ctx context.Context, res *resource.Resource, protocol string, signals Signals) error {
	if !signals.Metrics {
		return nil
	}
	exporter, err := newMetricExporter(ctx, protocol)
	if err != nil {
		return fmt.Errorf("otlp metric exporter: %w", err)
	}
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(provider)
	p.shutdowns = append(p.shutdowns, provider.Shutdown)
	return nil
}

// startLogs installs the logger provider that [NewSlogHandler] bridges into.
func (p *Provider) startLogs(ctx context.Context, res *resource.Resource, protocol string, signals Signals) error {
	if !signals.Logs {
		return nil
	}
	exporter, err := newLogExporter(ctx, protocol)
	if err != nil {
		return fmt.Errorf("otlp log exporter: %w", err)
	}
	provider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
		sdklog.WithResource(res),
	)
	global.SetLoggerProvider(provider)
	p.shutdowns = append(p.shutdowns, provider.Shutdown)
	return nil
}
