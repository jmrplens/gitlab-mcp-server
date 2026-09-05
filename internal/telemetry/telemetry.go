package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log/global"
	nooplog "go.opentelemetry.io/otel/log/noop"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
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
	// DropToolNameFromMetrics removes gen_ai.tool.name from metric attributes.
	//
	// Set by the caller, which knows the registered tool surface; see
	// [DropToolName] for the decision and for why the individual surface needs
	// it. It affects metrics only: the span keeps the attribute on every
	// surface, because one span carrying a tool name costs one span while one
	// metric dimension carrying it costs a time series per tool, forever.
	DropToolNameFromMetrics bool
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
	enabled bool
	// dropToolName removes gen_ai.tool.name from metric attributes, which the
	// individual surface needs and the other two do not.
	dropToolName bool
	// One protocol and one endpoint per signal, because they can differ: the
	// configuration specification gives each signal its own variables, and
	// reporting the traces values for the whole process described a
	// metrics-only deployment by a value it never used.
	protocol       string
	metricProtocol string
	logProtocol    string

	signals Signals

	endpoint       string
	metricEndpoint string
	logEndpoint    string

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
	if SDKDisabledByEnv() {
		slog.InfoContext(ctx, "telemetry suppressed by OTEL_SDK_DISABLED", "component", "telemetry")
		return &Provider{}, nil
	}

	// Before anything can fail, so that whatever the SDK says about the
	// failure arrives structured on stderr rather than as a stdlib log line on
	// whatever stream the default sink chose.
	installDiagnostics(slog.Default())

	signals := cfg.Signals
	if signals.none() {
		signals = AllSignals()
	}
	// Resolved per signal, before anything is built, so a protocol that cannot
	// be honored fails at startup with a name rather than at the first export
	// with a rejected batch.
	//
	// And only for the signals that are on. Validating a disabled signal's
	// variable refuses a start over configuration that would never be read: a
	// metrics-only deployment does not care that OTEL_EXPORTER_OTLP_TRACES_PROTOCOL
	// is misspelled, and telling it otherwise is a failure it cannot act on.
	// The traces resolution used to run unconditionally, two lines above the
	// comment saying it must not.
	var traceProtocol, metricProtocol, logProtocol string
	var err error
	if signals.Traces {
		if traceProtocol, err = resolveProtocol(cfg.Protocol, tracesProtocolKey); err != nil {
			return nil, fmt.Errorf("traces OTLP protocol: %w", err)
		}
	}
	if signals.Metrics {
		if metricProtocol, err = resolveProtocol(cfg.Protocol, metricsProtocolKey); err != nil {
			return nil, fmt.Errorf("metrics OTLP protocol: %w", err)
		}
	}
	if signals.Logs {
		if logProtocol, err = resolveProtocol(cfg.Protocol, logsProtocolKey); err != nil {
			return nil, fmt.Errorf("logs OTLP protocol: %w", err)
		}
	}

	// Before any exporter exists, for the same reason as the protocol above: a
	// CA file that cannot be read is a configuration failure the operator can
	// act on, and the exporters answer it by falling back to the system roots
	// and saying nothing. See [validateTLSMaterial].
	if err = validateTLSMaterial(signals); err != nil {
		return nil, fmt.Errorf("otlp exporter TLS material: %w", err)
	}

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("telemetry resource: %w", err)
	}

	p := &Provider{
		enabled:        true,
		dropToolName:   cfg.DropToolNameFromMetrics,
		protocol:       traceProtocol,
		metricProtocol: metricProtocol,
		logProtocol:    logProtocol,
		signals:        signals,
		endpoint:       endpointForSignal(tracesEndpointKey),
		metricEndpoint: endpointForSignal(metricsEndpointKey),
		logEndpoint:    endpointForSignal(logsEndpointKey),
	}

	if startErr := p.startTraces(ctx, res, traceProtocol, signals); startErr != nil {
		return nil, p.abandon(ctx, startErr)
	}
	if startErr := p.startMetrics(ctx, res, metricProtocol, signals); startErr != nil {
		return nil, p.abandon(ctx, startErr)
	}
	if startErr := p.startLogs(ctx, res, logProtocol, signals); startErr != nil {
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

	setCurrent(p.Snapshot())
	return p, nil
}

// EnvSwitchName is the environment variable that turns telemetry on.
//
// It carries this project's prefix rather than living in the OTEL_ namespace,
// for the reasons in the package doc: that namespace belongs to the
// specification and the language SDKs, and a bare name would collide with
// whatever else a host has exported.
const EnvSwitchName = "GITLAB_MCP_TELEMETRY"

// EnvSwitch reports whether the environment asks for telemetry.
//
// Parsed with the same grammar as every other boolean here, which is stricter
// than Go's: only the case-insensitive string "true" enables, an empty value
// counts as unset, and anything unrecognized is a warning and a false rather
// than an error. Using one parser for our switch and for OTEL_SDK_DISABLED is
// deliberate: two booleans in one configuration surface that disagree about
// whether "1" means true would be worse than either rule alone.
func EnvSwitch() bool {
	return envBool(EnvSwitchName)
}

// SDKDisabledByEnv reports whether the specification's own kill switch is set.
//
// OTEL_SDK_DISABLED cannot be this server's on switch, and reading it as one
// would be worse than not reading it at all. Its specified default is false,
// meaning "the SDK is enabled", while telemetry here is off until an operator
// asks for it. Treating the variable as the single switch would therefore
// invert its meaning for exactly the operators who already know what it means.
// It composes instead: our own switch turns telemetry on, and this vetoes.
//
// Nothing beneath us implements it. The string does not appear anywhere in the
// OpenTelemetry Go modules, and the specification's compliance matrix records
// no Go support, so a deployment that sets it and expects to be obeyed is
// relying on this function existing.
//
// The carve-out in the specification is honored by where this is called rather
// than by anything here: "This setting has no effect on propagators configured
// through the OTEL_PROPAGATORS variable." Returning early from Start leaves the
// global propagator untouched, which is the no-op composite the SDK installs by
// default.
func SDKDisabledByEnv() bool {
	return envBool("OTEL_SDK_DISABLED")
}

// envBool parses an OTEL_* boolean exactly as the configuration specification
// requires, which is strictly narrower than Go's own parser.
//
// The rule: "Any value that represents a Boolean MUST be set to true only by
// the case-insensitive string \"true\"... An implementation MUST NOT extend
// this definition and define additional values that are interpreted as true.
// Any value not explicitly defined here as a true value, including unset and
// empty values, MUST be interpreted as false."
//
// strconv.ParseBool violates that in three separate ways, which is why it is
// not used here: it accepts "1", "t" and "T" as true, which is precisely the
// extension the MUST NOT forbids; it returns an error for anything
// unrecognized rather than false; and it would need case folding bolted on
// anyway. An unrecognized value is warned about, per the specification's
// SHOULD, and then treated as false.
func envBool(key string) bool {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return false
	}
	value := strings.TrimSpace(raw)
	switch {
	case value == "":
		// "The SDK MUST interpret an empty value of an environment variable the
		// same way as when the variable is unset." Container orchestrators
		// routinely inject an empty variable for a secret that was never
		// provided, so this is the common case, not the pathological one.
		return false
	case strings.EqualFold(value, "true"):
		return true
	case strings.EqualFold(value, "false"):
		return false
	default:
		slog.Warn("ignoring unrecognized boolean environment variable",
			"component", "telemetry", "variable", key, "value", value,
			"expected", "true or false, case-insensitive")
		return false
	}
}

// abandon retires whatever started before an error, so a partial failure does
// not leave exporters running with no owner.
func (p *Provider) abandon(ctx context.Context, cause error) error {
	// The globals first: a signal that started before a later one failed has
	// already installed its provider, and shutting the exporter down while
	// leaving that provider registered would hand every instrument in the
	// process a dead pipeline. The no-ops are what an untelemetered process
	// has anyway.
	otel.SetTracerProvider(nooptrace.NewTracerProvider())
	otel.SetMeterProvider(noopmetric.NewMeterProvider())
	global.SetLoggerProvider(nooplog.NewLoggerProvider())

	if err := p.Shutdown(ctx); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

// Enabled reports whether telemetry is running.
func (p *Provider) Enabled() bool { return p != nil && p.enabled }

// Signals reports which signals this provider exports, so a caller can ask a
// question about them without reaching into the struct.
func (p *Provider) Signals() Signals {
	if p == nil {
		return Signals{}
	}
	return p.signals
}

// Shutdown flushes and retires every exporter, bounded by [shutdownTimeout]
// or by the caller's deadline, whichever comes first. The caller's
// cancellation is deliberately detached (a dying request must not abort the
// final flush), but a deadline the caller chose is a bound to honor: before
// this, the internal five seconds silently overrode any tighter one, which
// made cmd/server's own shutdown bound dead code and held every test that
// pointed an exporter at an unreachable collector for the full five seconds.
//
// Errors are joined rather than returned on the first failure: each exporter
// owns a connection of its own, and stopping the rest matters more than
// reporting the first one that could not.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil || len(p.shutdowns) == 0 {
		return nil
	}
	bound := shutdownTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < bound {
			bound = remaining
		}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bound)
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
	setCurrent(Snapshot{})
	return errors.Join(errs...)
}

// Snapshot describes the running configuration for the server card.
//
// It names what is on rather than what the binary can do, matching the
// subscriptions block: a consumer branches on this deployment's answer.
type Snapshot struct {
	Enabled bool `json:"enabled"`

	// Protocol is the transport every enabled signal uses, and is empty when
	// they do not all use the same one.
	//
	// Empty rather than one of them. This field used to hold the traces
	// protocol unconditionally, so a metrics-only deployment exporting over
	// gRPC published "http/protobuf": a value that was not merely imprecise but
	// false, in a document a client reads. A consumer now sees either an answer
	// that holds for the whole process or no answer, and SignalProtocols has
	// the detail when there is a disagreement to describe.
	Protocol string   `json:"protocol,omitempty"`
	Signals  []string `json:"signals,omitempty"`

	// Endpoint is the collector this deployment exports to, when the standard
	// environment names one and every enabled signal names the same one. It is
	// the operator's own address and is reported so somebody connecting to a
	// shared endpoint can see where the record of their calls goes, which is
	// the point of announcing this at all.
	Endpoint string `json:"endpoint,omitempty"`

	// SignalProtocols and SignalEndpoints carry the per-signal answers, keyed
	// by signal name.
	//
	// Never serialized: the server card is a public document and its shape is
	// worth changing deliberately rather than as a side effect of fixing a
	// wrong value. These exist for the startup log, where an operator is
	// looking at their own deployment and detail costs nothing.
	SignalProtocols map[string]string `json:"-"`
	SignalEndpoints map[string]string `json:"-"`
}

// Snapshot returns what to publish about this deployment.
func (p *Provider) Snapshot() Snapshot {
	if !p.Enabled() {
		return Snapshot{Enabled: false}
	}

	var signals []string
	protocols := map[string]string{}
	endpoints := map[string]string{}
	for _, signal := range []struct {
		name     string
		on       bool
		protocol string
		endpoint string
	}{
		{"traces", p.signals.Traces, p.protocol, p.endpoint},
		{"metrics", p.signals.Metrics, p.metricProtocol, p.metricEndpoint},
		{"logs", p.signals.Logs, p.logProtocol, p.logEndpoint},
	} {
		if !signal.on {
			continue
		}
		signals = append(signals, signal.name)
		if signal.protocol != "" {
			protocols[signal.name] = signal.protocol
		}
		if signal.endpoint != "" {
			endpoints[signal.name] = signal.endpoint
		}
	}

	return Snapshot{
		Enabled:         true,
		Protocol:        agreedValue(protocols, len(signals)),
		Signals:         signals,
		Endpoint:        agreedValue(endpoints, len(signals)),
		SignalProtocols: protocols,
		SignalEndpoints: endpoints,
	}
}

// agreedValue returns the value every enabled signal shares, or "".
//
// A value is only shared if every enabled signal reported one and they are all
// the same. A partial agreement is not an agreement: two signals naming one
// collector while a third names another has no single answer, and giving one
// anyway is how the field came to be wrong in the first place.
func agreedValue(values map[string]string, enabled int) string {
	if len(values) != enabled || enabled == 0 {
		return ""
	}
	var agreed string
	for _, value := range values {
		if agreed == "" {
			agreed = value
			continue
		}
		if value != agreed {
			return ""
		}
	}
	return agreed
}

// current holds what the running providers were started with, for surfaces that
// need to report telemetry without holding the provider.
//
// A package-level value rather than a threaded parameter, and the reason is
// that it mirrors what is already true: Start installs global OTel providers,
// so there is exactly one answer per process and pretending otherwise by
// passing a handle through the server-card builder and its test seam would add
// plumbing without adding truth.
var current struct {
	mu       sync.RWMutex
	snapshot Snapshot
}

// CurrentSnapshot reports what telemetry this process is running.
//
// The zero value means off, which is what a caller gets before Start, after
// Shutdown, and when telemetry was never enabled. No caller needs a nil check
// or a second branch.
func CurrentSnapshot() Snapshot {
	current.mu.RLock()
	defer current.mu.RUnlock()
	return current.snapshot
}

// setCurrent publishes the snapshot, or clears it on shutdown.
func setCurrent(snapshot Snapshot) {
	current.mu.Lock()
	defer current.mu.Unlock()
	current.snapshot = snapshot
}

// normalizeProtocol accepts the two transports this server implements and// normalizeProtocol accepts the two transports this server implements and
// refuses anything else by name.
//
// `http` is accepted as a spelling of `http/protobuf` because operators write
// it, and refusing it would fail at startup over a shorthand.
//
// `http/json` is refused, and the reason is narrower than it looks. It is not
// that nothing implements it: since v1.46.0 otlptracehttp does, selecting the
// payload encoding from these very variables and offering WithEncoding to
// override. otlpmetrichttp and otlploghttp do not. So honoring http/json would
// give a deployment two encodings at once, JSON spans beside protobuf metrics
// and logs, from one setting that reads like it selects one thing. Refusing at
// startup, by name, is the only outcome an operator can act on: silently
// downgrading to protobuf would ignore an explicit choice, and silently
// honoring it for traces alone would produce the split.
func normalizeProtocol(protocol string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(protocol)) {
	case "", "http", ProtocolHTTP:
		return ProtocolHTTP, nil
	case ProtocolGRPC:
		return ProtocolGRPC, nil
	case "http/json":
		return "", fmt.Errorf(
			"OTLP protocol %q is implemented by the Go trace exporter but not by the metric or log exporters, "+
				"so a deployment using it would emit JSON spans alongside protobuf metrics and logs; use %q or %q",
			"http/json", ProtocolHTTP, ProtocolGRPC,
		)
	default:
		return "", fmt.Errorf("unknown OTLP protocol %q: use %q or %q", protocol, ProtocolHTTP, ProtocolGRPC)
	}
}

// resolveProtocol decides the transport for one signal.
//
// Transport selection is this server's job and nothing beneath it can do the
// work. In Go the transport is chosen by which package is imported, so
// OTEL_EXPORTER_OTLP_PROTOCOL cannot reach across that boundary: on the HTTP
// path WithProtocol logs a warning about grpc and carries on over HTTP anyway.
// For metrics and logs this function is the only thing that gives those
// variables any effect at all, since otlpmetrichttp, otlpmetricgrpc,
// otlploghttp and otlploggrpc contain no protocol handling whatsoever.
//
// Reading the signal-specific variable is not a refinement, it is the point:
// "Each configuration option MUST be overridable by a signal specific option."
// Consulting only the general variable leaves a real hole, because
// otlptracehttp reads the signal-specific one itself to pick its payload
// encoding. An operator setting OTEL_EXPORTER_OTLP_TRACES_PROTOCOL=http/json
// with the general variable unset would slip past a check that looked only at
// the general one, and get JSON spans with protobuf everything else.
//
// Precedence follows this server's house rule, most specific first: an
// explicitly configured protocol beats the signal-specific variable, which
// beats the general one, which falls back to the default. An empty value counts
// as unset at every level, per the configuration specification.
func resolveProtocol(configured, signalKey string) (string, error) {
	for _, candidate := range []string{
		configured,
		os.Getenv(signalKey),
		os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"),
	} {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		return normalizeProtocol(candidate)
	}
	return ProtocolHTTP, nil
}

// The signal-specific protocol variables, named once so a typo cannot make one
// of them quietly unread.
const (
	tracesEndpointKey  = "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"
	metricsEndpointKey = "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"
	logsEndpointKey    = "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"

	tracesProtocolKey  = "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL"
	metricsProtocolKey = "OTEL_EXPORTER_OTLP_METRICS_PROTOCOL"
	logsProtocolKey    = "OTEL_EXPORTER_OTLP_LOGS_PROTOCOL"
)

// buildResource describes this process to a collector.
//
// The precedence chain is: a service name configured here beats OTEL_SERVICE_NAME,
// which beats a service.name key inside OTEL_RESOURCE_ATTRIBUTES, which beats the
// SDK's own unknown_service fallback. Everything else an operator puts in
// OTEL_RESOURCE_ATTRIBUTES passes through untouched, because this server has no
// opinion about deployment.environment.name or service.namespace and no business
// overwriting them.
//
// The conditional is the whole point and it is not decoration. resource.New
// merges each option as the *updating* resource in the order given, so an
// unconditional semconv.ServiceName here overwrites whatever the environment
// supplied, and the provider's own resource.Merge(resource.Environment(), r)
// then re-merges the environment underneath, where it loses a second time. That
// is what this function used to do: OTEL_SERVICE_NAME was discarded in silence,
// with no error and no log line, while the doc comment claimed the environment
// won. Setting the key only when nothing beneath us set it is what actually
// gives an operator the override.
func buildResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	var attrs []attribute.KeyValue
	if name := serviceName(cfg); name != "" {
		attrs = append(attrs, semconv.ServiceName(name))
	}
	if cfg.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.ServiceVersion))
	}
	return resource.New(ctx,
		// First, because WithService is two detectors and only one of them is
		// wanted here. It contributes service.instance.id, which resource.Default
		// omits unless the experimental OTEL_GO_X_RESOURCE flag is set and which
		// is what separates two concurrent copies of an HTTP deployment, or a
		// stdio process from an HTTP one sharing a service name. It also
		// contributes a service.name of "unknown_service:<binary>", and since
		// resource.New applies each option as the updating resource in order,
		// placing it after WithFromEnv would overwrite the operator's name with
		// that placeholder: the same silent defect this function was fixed for,
		// reintroduced from the other end.
		resource.WithService(),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attrs...),
	)
}

// serviceName resolves what this process calls itself, or returns the empty
// string to mean "leave it to whatever is beneath us".
//
// Empty is a real answer rather than a failure: the caller omits the attribute
// entirely, which is what lets OTEL_SERVICE_NAME and a service.name inside
// OTEL_RESOURCE_ATTRIBUTES take effect. Writing an empty string instead would
// be worse than useless, because resource.Merge lets an empty value from the
// updating resource erase a real one from the base.
func serviceName(cfg Config) string {
	if cfg.ServiceName != "" {
		return cfg.ServiceName
	}
	if envHasServiceName() {
		return ""
	}
	return DefaultServiceName
}

// envHasServiceName reports whether anything in the environment already names
// the service, by either of the two variables that can.
//
// An empty value counts as unset, per the configuration specification: "The SDK
// MUST interpret an empty value of an environment variable the same way as when
// the variable is unset." Container orchestrators routinely inject empty
// variables for secrets that were never provided, so reading os.Getenv(k) != ""
// as "the operator set this" is wrong in exactly the deployments most likely to
// hit it.
func envHasServiceName() bool {
	if strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME")) != "" {
		return true
	}
	for pair := range strings.SplitSeq(os.Getenv("OTEL_RESOURCE_ATTRIBUTES"), ",") {
		key, _, found := strings.Cut(pair, "=")
		if found && strings.TrimSpace(key) == string(semconv.ServiceNameKey) {
			return true
		}
	}
	return false
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
		sdktrace.WithRawSpanLimits(spanLimits()),
	)
	otel.SetTracerProvider(provider)
	p.shutdowns = append(p.shutdowns, provider.Shutdown)
	return nil
}

// defaultAttributeValueLength bounds one span attribute value when the operator
// has not chosen a bound of their own.
//
// The SDK's default is unlimited, which means a single attribute is as large as
// whatever produced it. The instrumentation in this tree is careful about that
// at each call site, and a per-site rule is exactly the kind that holds until
// somebody adds the next attribute: the HTTP method one was written with a
// comment saying an unbounded value was affordable on a span, and twenty
// anonymous requests then relayed ten megabytes to a collector. This is the
// floor under every attribute, including the ones nobody has written yet.
//
// 4096 rather than something tight: it is a backstop, not the bound any
// particular attribute should be relying on.
const defaultAttributeValueLength = 4096

// spanLimits returns the span limits to install, honoring the operator first.
//
// NewSpanLimits already reads every OTEL_SPAN_* variable, so this only fills in
// the one the specification leaves unlimited, and only when nothing set it. The
// guard is the same shape as envHasServiceName and exists for the same reason:
// a value passed as an option beats the environment in this SDK, so an
// unconditional limit would silently override an operator who chose one.
func spanLimits() sdktrace.SpanLimits {
	limits := sdktrace.NewSpanLimits()
	if _, set := os.LookupEnv("OTEL_SPAN_ATTRIBUTE_VALUE_LENGTH_LIMIT"); !set {
		limits.AttributeValueLengthLimit = defaultAttributeValueLength
	}
	return limits
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
	opts := []sdkmetric.Option{
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(res),
	}
	if p.dropToolName {
		// Registered as a View rather than by raising the cardinality limit,
		// because filtering happens before the limit is counted. See
		// cardinality.go for the number that makes this necessary and for the
		// deviation it represents.
		opts = append(opts, sdkmetric.WithView(toolNameView()))
	}
	provider := sdkmetric.NewMeterProvider(opts...)
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
