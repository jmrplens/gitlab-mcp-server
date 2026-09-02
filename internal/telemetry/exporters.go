package telemetry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
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

// validateTLSMaterial refuses a start whose configured exporter TLS material
// cannot be loaded, for the signals that would actually read it.
//
// # Why this is not left to the exporters
//
// They read these variables themselves, and on a failure they log and carry on
// **without** the material: withTLSConfig applies a tls.Config only when a pool
// or a certificate was loaded, so an unreadable CA file leaves the client
// verifying against the system roots and an unloadable client certificate
// leaves mutual TLS silently unconfigured. Start does not observe that error,
// so it returns a working provider and the server logs "telemetry enabled".
// An operator who pinned a private CA is then verifying against public roots
// and has been told the opposite. Reproduced against an impostor collector: the
// batches, and the collector credential with them, went to the impostor.
//
// Worse and completely silent: WithClientCert reads its certificate and key
// together and returns with no log at all when only one of the pair is set, so
// a typo in one variable name disables mutual TLS without a word. That case is
// treated as a refusal here for the reason cmd/server/listen.go already states
// about this server's own listener: a cert without its key is a deployment that
// thinks it is encrypting and is not.
//
// # Why per signal, and why the precedence matters
//
// A signal reads its own variable when it has one and the shared variable
// otherwise. Checking every variable that is set would refuse a start over a
// stale shared value that no enabled signal would ever read, which is a
// failure the operator cannot act on. Checking only the shared one would miss
// the per-signal file entirely.
//
// The specification permits this: an SDK may "fail fast and cause the
// application to fail on initialization ... because of a bad user config". The
// caller turns it into a disabled provider with a named message rather than a
// dead server.
func validateTLSMaterial(signals Signals) error {
	for _, signal := range []struct {
		name   string
		prefix string
		on     bool
	}{
		{"traces", "OTEL_EXPORTER_OTLP_TRACES_", signals.Traces},
		{"metrics", "OTEL_EXPORTER_OTLP_METRICS_", signals.Metrics},
		{"logs", "OTEL_EXPORTER_OTLP_LOGS_", signals.Logs},
	} {
		if !signal.on {
			continue
		}
		if err := validateCertPool(signal.prefix); err != nil {
			return fmt.Errorf("%s: %w", signal.name, err)
		}
		if err := validateClientCert(signal.prefix); err != nil {
			return fmt.Errorf("%s: %w", signal.name, err)
		}
	}
	return nil
}

// validateCertPool checks the CA a signal would verify its collector against.
func validateCertPool(prefix string) error {
	key, path := firstSetEnv(prefix, "", "CERTIFICATE")
	if path == "" {
		return nil
	}
	pem, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s=%s: %w", key, path, err)
	}
	if !x509.NewCertPool().AppendCertsFromPEM(pem) {
		return fmt.Errorf("%s=%s: no certificate found; the collector would be verified against the system roots instead", key, path)
	}
	return nil
}

// validateClientCert checks the client certificate and key a signal would
// present, and that both halves are there.
func validateClientCert(prefix string) error {
	certKey, certPath := firstSetEnv(prefix, "", "CLIENT_CERTIFICATE")
	keyKey, keyPath := firstSetEnv(prefix, "", "CLIENT_KEY")
	switch {
	case certPath == "" && keyPath == "":
		return nil
	case keyPath == "":
		return fmt.Errorf("%s is set without its key; mutual TLS would be silently disabled", certKey)
	case certPath == "":
		return fmt.Errorf("%s is set without its certificate; mutual TLS would be silently disabled", keyKey)
	}
	if _, err := tls.LoadX509KeyPair(certPath, keyPath); err != nil {
		return fmt.Errorf("%s=%s with %s=%s: %w", certKey, certPath, keyKey, keyPath, err)
	}
	return nil
}

// firstSetEnv resolves one variable the way the exporters do: the signal's own
// name first, the shared name second.
//
// It returns the name it read as well as the value, because a message naming
// the variable an operator actually set is the difference between a fix and a
// search.
func firstSetEnv(signalPrefix, sharedPrefix, suffix string) (key, value string) {
	if sharedPrefix == "" {
		sharedPrefix = "OTEL_EXPORTER_OTLP_"
	}
	for _, name := range []string{signalPrefix + suffix, sharedPrefix + suffix} {
		if configured := strings.TrimSpace(os.Getenv(name)); configured != "" {
			return name, configured
		}
	}
	return "", ""
}

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
			return redactEndpointUserinfo(value)
		}
	}
	return ""
}

// redactEndpointUserinfo strips credentials from an endpoint before anything
// displays it.
//
// The value here is display-only: the exporters read the variable themselves,
// so nothing functional passes through this path. An operator can write
// user:password@host into a URL, and the startup summary that logs the endpoint
// is itself exported through the log bridge, so without this the credential
// would travel to the very collector it authenticates to.
func redactEndpointUserinfo(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.User == nil {
		return endpoint
	}
	parsed.User = url.User("redacted")
	return parsed.String()
}
