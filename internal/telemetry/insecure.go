package telemetry

import (
	"net"
	"net/url"
	"os"
	"strings"
)

// headerKeys are the variables that carry a collector credential, most specific
// first, matching the precedence the exporters apply.
var headerKeys = map[string][]string{
	"traces":  {"OTEL_EXPORTER_OTLP_TRACES_HEADERS", "OTEL_EXPORTER_OTLP_HEADERS"},
	"metrics": {"OTEL_EXPORTER_OTLP_METRICS_HEADERS", "OTEL_EXPORTER_OTLP_HEADERS"},
	"logs":    {"OTEL_EXPORTER_OTLP_LOGS_HEADERS", "OTEL_EXPORTER_OTLP_HEADERS"},
}

// InsecureCredentialSignals names the signals that would send a collector
// credential over plaintext to another host.
//
// # What this is not
//
// It is not a refusal, and it must not become one. A collector on a trusted
// private network reached over plaintext is a legitimate deployment, and it is
// the one this project's telemetry work was validated against. The endpoint and
// the headers are both the operator's own configuration, and the specification
// is deliberate that OTEL_EXPORTER_OTLP_* belongs to the exporters. Overriding
// an explicit choice about somebody's own network is not this server's call.
//
// What is this server's call is not letting the mistake be silent. The guide
// carries the warning in prose; this is the same sentence at the moment it
// applies, which is the one that reaches an operator who did not read that
// section.
//
// # Why loopback is exempt
//
// A credential that never leaves the machine cannot be observed on a network,
// so a sidecar collector on 127.0.0.1 is not a disclosure. Anything else is,
// including a private address: "the LAN is trusted" is a judgement the operator
// is entitled to make and this function is not, so it says what is happening
// rather than what to do about it.
func InsecureCredentialSignals(signals Signals) []string {
	var affected []string
	for _, signal := range []struct {
		name        string
		on          bool
		endpointKey string
	}{
		{"traces", signals.Traces, tracesEndpointKey},
		{"metrics", signals.Metrics, metricsEndpointKey},
		{"logs", signals.Logs, logsEndpointKey},
	} {
		if !signal.on {
			continue
		}
		if !hasCredential(signal.name) {
			continue
		}
		if isPlaintextRemote(endpointForSignal(signal.endpointKey)) {
			affected = append(affected, signal.name)
		}
	}
	return affected
}

// hasCredential reports whether a signal has any header configured.
//
// The value is never read beyond emptiness. A header set at all is the
// condition, because deciding which headers count as credentials would mean
// parsing them, and parsing a credential to decide whether to warn about it is
// the wrong shape for a function whose whole purpose is that it never touches
// the value.
func hasCredential(signal string) bool {
	for _, key := range headerKeys[signal] {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

// isPlaintextRemote reports whether an endpoint is http and not loopback.
//
// An unparseable or empty endpoint is not reported: the exporter will fail on
// its own and say so, and guessing at a malformed URL to raise a second warning
// about it would be noise on top of an error.
func isPlaintextRemote(endpoint string) bool {
	if endpoint == "" {
		return false
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "http" {
		return false
	}

	host := parsed.Hostname()
	if host == "" || strings.EqualFold(host, "localhost") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return false
	}
	return true
}
