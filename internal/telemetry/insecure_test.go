package telemetry

import (
	"slices"
	"testing"
)

// TestInsecureCredentialSignals covers the one condition worth a warning and,
// more importantly, the many that are not.
//
// The failure mode this function must avoid is not missing a case. It is
// warning about a deployment that is fine, because an operator who is told
// their correct configuration is dangerous learns to ignore the line, and then
// the one that matters is ignored too.
func TestInsecureCredentialSignals(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		signals Signals
		want    []string
	}{
		{
			name: "a credential to a plaintext host elsewhere",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_HEADERS":  "authorization=Bearer x",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector.example:4318",
			},
			signals: Signals{Traces: true},
			want:    []string{"traces"},
		},
		{
			// The deployment this project was validated against, and the reason
			// this is a warning and never a refusal.
			name: "no credential, plaintext, private network",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://192.168.0.99:4318",
			},
			signals: AllSignals(),
			want:    nil,
		},
		{
			name: "a credential over TLS",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_HEADERS":  "authorization=Bearer x",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "https://collector.example:4318",
			},
			signals: AllSignals(),
			want:    nil,
		},
		{
			// A credential that never leaves the machine cannot be observed on
			// a network, so a sidecar collector is not a disclosure.
			name: "a credential to loopback",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_HEADERS":  "authorization=Bearer x",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:4318",
			},
			signals: AllSignals(),
			want:    nil,
		},
		{
			name: "localhost by name is loopback too",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_HEADERS":  "authorization=Bearer x",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4318",
			},
			signals: AllSignals(),
			want:    nil,
		},
		{
			// The signal-specific variables win, so one signal can be exposed
			// while the others are not, and naming the wrong one would send an
			// operator to the wrong place.
			name: "only the signal whose own endpoint is plaintext",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_HEADERS":          "authorization=Bearer x",
				"OTEL_EXPORTER_OTLP_ENDPOINT":         "https://collector.example:4318",
				"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT": "http://metrics.example:4318",
			},
			signals: AllSignals(),
			want:    []string{"metrics"},
		},
		{
			name: "a disabled signal is not reported",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_HEADERS":  "authorization=Bearer x",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector.example:4318",
			},
			signals: Signals{Metrics: true},
			want:    []string{"metrics"},
		},
		{
			name:    "nothing configured at all",
			env:     map[string]string{},
			signals: AllSignals(),
			want:    nil,
		},
		{
			// The exporter reports a malformed endpoint on its own. A second
			// warning guessing at what it meant is noise on top of an error.
			name: "an unparseable endpoint is left to the exporter",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_HEADERS":  "authorization=Bearer x",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "://not a url",
			},
			signals: AllSignals(),
			want:    nil,
		},
		{
			// The trap this detector missed entirely. The Go OTLP/HTTP trace
			// and metric exporters apply INSECURE after the endpoint scheme, so
			// an https endpoint is downgraded to plaintext and the credential
			// leaves in the clear while the startup summary prints https. The
			// logs exporter reads the same variable the way the specification
			// describes — "only applies to OTLP/gRPC when an endpoint is
			// provided without the http or https scheme" — so it does not.
			name: "the insecure variable downgrades an https endpoint",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_HEADERS":  "authorization=Bearer x",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "https://collector.example:4318",
				"OTEL_EXPORTER_OTLP_INSECURE": "true",
			},
			signals: AllSignals(),
			want:    []string{"traces", "metrics"},
		},
		{
			name: "a per-signal insecure variable names only its signal",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_HEADERS":         "authorization=Bearer x",
				"OTEL_EXPORTER_OTLP_ENDPOINT":        "https://collector.example:4318",
				"OTEL_EXPORTER_OTLP_TRACES_INSECURE": "true",
			},
			signals: AllSignals(),
			want:    []string{"traces"},
		},
		{
			// The inverse, which matters because a detector that is wrong in
			// this direction names signals that are on TLS and teaches an
			// operator to ignore the line.
			name: "insecure false upgrades a plaintext endpoint for traces and metrics",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_HEADERS":  "authorization=Bearer x",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector.example:4318",
				"OTEL_EXPORTER_OTLP_INSECURE": "false",
			},
			signals: AllSignals(),
			want:    []string{"logs"},
		},
		{
			// The same inverse, spelled the way an operator reaches for it.
			// envconfig.WithBool lets any non-empty value decide and treats
			// everything but "true" as false, so "1" upgrades traces and
			// metrics to TLS exactly as "false" does. The log exporter
			// converts strictly and rejects the spelling, so its endpoint's
			// scheme still decides.
			name: "an unrecognized insecure value still decides for traces and metrics",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_HEADERS":  "authorization=Bearer x",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector.example:4318",
				"OTEL_EXPORTER_OTLP_INSECURE": "1",
			},
			signals: AllSignals(),
			want:    []string{"logs"},
		},
		{
			name: "a downgrade to loopback is still not a disclosure",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_HEADERS":  "authorization=Bearer x",
				"OTEL_EXPORTER_OTLP_ENDPOINT": "https://127.0.0.1:4318",
				"OTEL_EXPORTER_OTLP_INSECURE": "true",
			},
			signals: AllSignals(),
			want:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range []string{
				"OTEL_EXPORTER_OTLP_HEADERS", "OTEL_EXPORTER_OTLP_ENDPOINT",
				"OTEL_EXPORTER_OTLP_TRACES_HEADERS", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
				"OTEL_EXPORTER_OTLP_METRICS_HEADERS", "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
				"OTEL_EXPORTER_OTLP_LOGS_HEADERS", "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
				// Without these four an ambient value on the machine running
				// the tests decides every case above, and the table would be
				// grading the developer's shell.
				"OTEL_EXPORTER_OTLP_INSECURE", "OTEL_EXPORTER_OTLP_TRACES_INSECURE",
				"OTEL_EXPORTER_OTLP_METRICS_INSECURE", "OTEL_EXPORTER_OTLP_LOGS_INSECURE",
			} {
				t.Setenv(key, "")
			}
			for key, value := range tc.env {
				t.Setenv(key, value)
			}

			got := InsecureCredentialSignals(tc.signals)
			if !slices.Equal(got, tc.want) {
				t.Errorf("InsecureCredentialSignals = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestIsPlaintextRemote_SaysNothingAboutWhatItCannotRead covers the two inputs
// that are deliberately not warned about.
//
// An endpoint nobody configured is the default deployment, and an endpoint that
// does not parse is already going to fail inside the exporter with a message
// naming it. Guessing at either to raise a second warning would put noise on
// top of an error, and the warning this function feeds is one an operator is
// meant to act on.
func TestIsPlaintextRemote_SaysNothingAboutWhatItCannotRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		want     bool
	}{
		{name: "nothing configured", endpoint: "", want: false},
		{name: "unparseable", endpoint: "://not-a-url", want: false},
		{name: "https is not plaintext", endpoint: "https://collector.example.com:4318", want: false},
		{name: "plaintext loopback stays local", endpoint: "http://127.0.0.1:4318", want: false},
		{name: "plaintext to another host", endpoint: "http://collector.example.com:4318", want: true},
		// A literal address that is not loopback. The loopback exemption is the
		// only one this function grants, so a routable address written as
		// digits must be reported exactly as a name would be: a credential
		// leaving the machine is a disclosure whichever way the host is
		// spelled, and reading "it parsed as an IP" as "it is local" would
		// silence every deployment that names its collector by address.
		{name: "plaintext to a literal routable address", endpoint: "http://203.0.113.10:4318", want: true},
		// A private address is still another host: whether the LAN is trusted
		// is the operator's judgement and not this function's.
		{name: "plaintext to a literal private address", endpoint: "http://192.168.0.99:4318", want: true},
		{name: "IPv6 loopback stays local", endpoint: "http://[::1]:4318", want: false},
		// Parses, and names no host at all. There is nothing to be remote from,
		// and the exporter will fail on its own with a message naming it.
		{name: "a scheme with no host", endpoint: "http://", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isPlaintextRemote(tt.endpoint); got != tt.want {
				t.Errorf("isPlaintextRemote(%q) = %v, want %v", tt.endpoint, got, tt.want)
			}
		})
	}
}

// TestIsPlaintextRemoteForSignal_TheLogsExporterFallsBackToItsVariable covers
// the branch the per-signal rule exists for, in the one shape that reaches it.
//
// The log exporter resolves the scheme first, so a scheme on the endpoint
// settles the question and the variable never applies. When the endpoint
// carries no scheme there is nothing to resolve, and the variable is what
// decides — which is the same answer the other two signals give and the reason
// this branch is easy to write as though it were unconditional.
//
// The endpoint is spelled "//host:port" because that is the only scheme-less
// form whose host survives url.Parse: "host:port" is read as a scheme with an
// opaque body, which names no host and is not remote to begin with.
func TestIsPlaintextRemoteForSignal_TheLogsExporterFallsBackToItsVariable(t *testing.T) {
	const schemeless = "//collector.example.com:4317"

	tests := []struct {
		name     string
		insecure string
		want     bool
	}{
		{
			name:     "the variable says plaintext and nothing overrides it",
			insecure: "true",
			want:     true,
		},
		{
			name:     "the variable says TLS, so nothing is disclosed",
			insecure: "false",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_LOGS_INSECURE", tt.insecure)

			if got := isPlaintextRemoteForSignal("logs", schemeless); got != tt.want {
				t.Errorf("isPlaintextRemoteForSignal(logs, %q) with INSECURE=%q = %v, want %v",
					schemeless, tt.insecure, got, tt.want)
			}
		})
	}
}

// TestSchemeOf_AnEndpointThatDoesNotParse_HasNoScheme covers the error branch
// nothing else reaches.
//
// Every caller in this file checks isRemoteHost first, which refuses an
// unparseable endpoint before schemeOf is ever asked, so the guard is only
// observable from here. It must answer "no scheme" rather than panic or invent
// one: this function feeds a warning, and a warning derived from a URL nobody
// could read is noise on top of the error the exporter is about to report.
func TestSchemeOf_AnEndpointThatDoesNotParse_HasNoScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{name: "unparseable", endpoint: "://not-a-url", want: ""},
		{name: "a control character the parser refuses", endpoint: "http://collector\x7f.example", want: ""},
		{name: "no scheme at all", endpoint: "//collector.example:4317", want: ""},
		{name: "an uppercase scheme is lowered", endpoint: "HTTP://collector.example:4318", want: "http"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := schemeOf(tt.endpoint); got != tt.want {
				t.Errorf("schemeOf(%q) = %q, want %q", tt.endpoint, got, tt.want)
			}
		})
	}
}
