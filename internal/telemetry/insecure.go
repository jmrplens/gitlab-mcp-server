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
		if isPlaintextRemoteForSignal(signal.name, endpointForSignal(signal.endpointKey)) {
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

// insecureKeys are the variables that decide transport security independently
// of the endpoint's scheme, most specific first.
var insecureKeys = map[string][]string{
	"traces":  {"OTEL_EXPORTER_OTLP_TRACES_INSECURE", "OTEL_EXPORTER_OTLP_INSECURE"},
	"metrics": {"OTEL_EXPORTER_OTLP_METRICS_INSECURE", "OTEL_EXPORTER_OTLP_INSECURE"},
	"logs":    {"OTEL_EXPORTER_OTLP_LOGS_INSECURE", "OTEL_EXPORTER_OTLP_INSECURE"},
}

// isPlaintextRemoteForSignal reports whether one signal's batches actually
// leave this host in the clear.
//
// # Why the scheme is not the answer
//
// The specification says OTEL_EXPORTER_OTLP_INSECURE "only applies to OTLP/gRPC
// when an endpoint is provided without the http or https scheme". The Go trace
// and metric exporters do not implement it that way: their environment
// configuration appends the scheme's decision and then the INSECURE options, so
// the later one wins and an https endpoint is downgraded to plaintext, with the
// collector credential going out on it. The newer log exporters resolve the
// scheme first and are correct. So the precedence has to be emulated per signal
// rather than read off the URL, in both directions: an INSECURE=false beside an
// http endpoint upgrades traces and metrics to TLS, and naming them in a
// plaintext warning would be the same defect pointing the other way.
//
// This deviation is upstream and this function only describes it. Nothing here
// refuses to start: see the type doc above for why that stays true.
func isPlaintextRemoteForSignal(signal, endpoint string) bool {
	if !isRemoteHost(endpoint) {
		return false
	}
	insecure, decided := insecureFromEnv(signal)
	switch {
	case !decided:
		return schemeOf(endpoint) == "http"
	case signal == "logs" && schemeOf(endpoint) != "":
		// The exporter this signal uses honors the specification, so a scheme
		// on the endpoint settles it and the variable never applies.
		return schemeOf(endpoint) == "http"
	default:
		return insecure
	}
}

// insecureFromEnv reads the insecure variables for one signal, most specific
// first, reporting whether any of them decided.
//
// Parsed the way the exporters parse it: case-insensitively, and only "true" or
// "false". An unset or unrecognized value leaves the decision to the scheme,
// which is what the exporters' own reader does when the conversion fails.
func insecureFromEnv(signal string) (insecure, decided bool) {
	for _, key := range insecureKeys[signal] {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		switch strings.ToLower(value) {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	}
	return false, false
}

// schemeOf returns an endpoint's scheme, or "" when it has none or does not
// parse.
func schemeOf(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Scheme)
}

// isRemoteHost reports whether an endpoint names a host other than this one.
//
// An unparseable or empty endpoint is not reported: the exporter will fail on
// its own and say so, and guessing at a malformed URL to raise a second warning
// about it would be noise on top of an error. A credential that never leaves
// the machine cannot be observed on a network, so loopback is not a disclosure.
func isRemoteHost(endpoint string) bool {
	if endpoint == "" {
		return false
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
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

// isPlaintextRemote reports whether an endpoint's own scheme says http to
// another host, which is the answer whenever no insecure variable overrides it.
func isPlaintextRemote(endpoint string) bool {
	return isRemoteHost(endpoint) && schemeOf(endpoint) == "http"
}
