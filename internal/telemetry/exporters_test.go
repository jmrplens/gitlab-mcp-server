// exporters_test.go verifies what the startup summary is allowed to say about
// the collector it exports to.
package telemetry

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TestRedactEndpointUserinfo_CredentialsNeverReachTheSummary covers the one
// transform applied to an endpoint before anything displays it.
//
// The value is display-only — the exporters read the variable themselves — but
// the startup line that carries it is itself exported through the log bridge,
// so a password written into the endpoint URL would travel to the very
// collector it authenticates to, and then sit in whatever stores that.
// Everything else about the URL is kept, because an operator reading the line
// needs to recognize their own deployment in it.
func TestRedactEndpointUserinfo_CredentialsNeverReachTheSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{
			name:     "user and password are replaced together",
			endpoint: "https://user:hunter2@collector.example.com:4318/v1/traces",
			want:     "https://redacted@collector.example.com:4318/v1/traces",
		},
		{
			name:     "a bare username is still userinfo",
			endpoint: "https://user@collector.example.com",
			want:     "https://redacted@collector.example.com",
		},
		{
			name:     "an endpoint without credentials is untouched",
			endpoint: "https://collector.example.com:4318",
			want:     "https://collector.example.com:4318",
		},
		{
			name:     "an unparseable endpoint is returned as it was written",
			endpoint: "://not-a-url",
			want:     "://not-a-url",
		},
		{
			name:     "nothing configured stays nothing",
			endpoint: "",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := redactEndpointUserinfo(tt.endpoint); got != tt.want {
				t.Errorf("redactEndpointUserinfo(%q) = %q, want %q", tt.endpoint, got, tt.want)
			}
		})
	}
}

// TestValidateTLSMaterial_RefusesWhatWouldSilentlyFallBack pins the direction
// exporter TLS misconfiguration fails in.
//
// The exporters read this material themselves and, on a failure, log and carry
// on **without** it: an unreadable CA leaves the client on the system roots,
// and a client certificate that will not load leaves mutual TLS unconfigured.
// Start never saw that, so it returned a working provider and the server
// announced "telemetry enabled" while an operator's private-CA pinning was not
// in effect. Reproduced against an impostor collector holding a certificate
// this process was made to trust: the batches and the collector credential went
// to the impostor.
//
// The half-pair case is the one with no signal at all. WithClientCert reads its
// certificate and key together and returns silently when only one is set, so a
// typo in one variable name disables mutual TLS without a log line anywhere.
//
// No network and no exporter is built: otlp*http.New does not dial, so this is
// about what the configuration says rather than what a collector answers.
func TestValidateTLSMaterial_RefusesWhatWouldSilentlyFallBack(t *testing.T) {
	certPath, keyPath := writeKeyPair(t)

	unreadable := filepath.Join(t.TempDir(), "absent.pem")
	notPEM := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(notPEM, []byte("this is not a certificate\n"), 0o600); err != nil {
		t.Fatalf("writing the non-PEM file: %v", err)
	}

	tests := []struct {
		name    string
		env     map[string]string
		signals Signals
		wantErr string
	}{
		{
			name:    "a readable CA is accepted",
			env:     map[string]string{"OTEL_EXPORTER_OTLP_CERTIFICATE": certPath},
			signals: AllSignals(),
		},
		{
			name:    "an unreadable CA is refused",
			env:     map[string]string{"OTEL_EXPORTER_OTLP_CERTIFICATE": unreadable},
			signals: AllSignals(),
			wantErr: "OTEL_EXPORTER_OTLP_CERTIFICATE",
		},
		{
			name:    "an unreadable per-signal CA is refused",
			env:     map[string]string{"OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE": unreadable},
			signals: AllSignals(),
			wantErr: "OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE",
		},
		{
			name:    "a file that is not a certificate is refused",
			env:     map[string]string{"OTEL_EXPORTER_OTLP_CERTIFICATE": notPEM},
			signals: AllSignals(),
			wantErr: "no certificate found",
		},
		{
			name:    "a client certificate without its key is refused",
			env:     map[string]string{"OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE": certPath},
			signals: AllSignals(),
			wantErr: "without its key",
		},
		{
			name:    "a client key without its certificate is refused",
			env:     map[string]string{"OTEL_EXPORTER_OTLP_CLIENT_KEY": keyPath},
			signals: AllSignals(),
			wantErr: "without its certificate",
		},
		{
			name: "a complete client pair is accepted",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE": certPath,
				"OTEL_EXPORTER_OTLP_CLIENT_KEY":         keyPath,
			},
			signals: AllSignals(),
		},
		{
			// The half-pair with both halves present. The exporters pair a
			// certificate with the key of its own prefix and nothing else, so
			// neither pair is complete to them: mutual TLS is not configured
			// and, as with a missing half, nothing is logged about it.
			name: "a certificate and key under different prefixes are refused",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE": certPath,
				"OTEL_EXPORTER_OTLP_CLIENT_KEY":                keyPath,
			},
			signals: Signals{Traces: true},
			wantErr: "different prefixes",
		},
		{
			// The other side of that rule, and the reason it is not "refuse
			// whenever one prefix is incomplete": the exporters fall back to
			// the shared pair, which is complete, so mutual TLS is configured
			// and a refusal here would be a start denied over nothing.
			name: "a stale per-signal certificate does not refuse a complete shared pair",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE": certPath,
				"OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE":        certPath,
				"OTEL_EXPORTER_OTLP_CLIENT_KEY":                keyPath,
			},
			signals: Signals{Traces: true},
		},
		{
			// The precedence rule, and the reason validation is per signal: a
			// stale shared variable no enabled signal would ever read must not
			// refuse a start the operator cannot fix by fixing what they use.
			name: "a signal naming its own file ignores a stale shared one",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_CERTIFICATE":        unreadable,
				"OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE": certPath,
			},
			signals: Signals{Traces: true},
		},
		{
			name:    "a disabled signal's material is not read",
			env:     map[string]string{"OTEL_EXPORTER_OTLP_LOGS_CERTIFICATE": unreadable},
			signals: Signals{Traces: true},
		},
		{
			// A complete pair that does not load. Both halves are named, so
			// nothing here is missing and only the files themselves are wrong;
			// without this the load error is a branch no row reaches and a pair
			// of unusable files would be announced as mutual TLS configured.
			name: "a client pair whose certificate is not a certificate is refused",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE": notPEM,
				"OTEL_EXPORTER_OTLP_CLIENT_KEY":         keyPath,
			},
			signals: AllSignals(),
			wantErr: "OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE",
		},
		{
			// Two half-pairs of the same kind. The message must name the
			// signal's own variable rather than the shared one: the signal
			// prefix is what the exporter reads first, so that is the file the
			// operator has to pair a key with.
			name: "a certificate under both prefixes is named by the most specific",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE": certPath,
				"OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE":        certPath,
			},
			signals: Signals{Traces: true},
			wantErr: "OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE is set without its key",
		},
		{
			name: "a key under both prefixes is named by the most specific",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY": keyPath,
				"OTEL_EXPORTER_OTLP_CLIENT_KEY":        keyPath,
			},
			signals: Signals{Traces: true},
			wantErr: "OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY is set without its certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{
				"OTEL_EXPORTER_OTLP_CERTIFICATE", "OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE", "OTEL_EXPORTER_OTLP_CLIENT_KEY",
				"OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", "OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE", "OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY",
				"OTEL_EXPORTER_OTLP_METRICS_CERTIFICATE", "OTEL_EXPORTER_OTLP_METRICS_CLIENT_CERTIFICATE", "OTEL_EXPORTER_OTLP_METRICS_CLIENT_KEY",
				"OTEL_EXPORTER_OTLP_LOGS_CERTIFICATE", "OTEL_EXPORTER_OTLP_LOGS_CLIENT_CERTIFICATE", "OTEL_EXPORTER_OTLP_LOGS_CLIENT_KEY",
			} {
				t.Setenv(key, "")
			}
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			err := validateTLSMaterial(tt.signals)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateTLSMaterial refused a configuration the exporters would honor: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("validateTLSMaterial accepted material the exporters would silently drop, and the server would still announce telemetry enabled")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to name %q so the operator knows which file to fix", err, tt.wantErr)
			}
		})
	}
}

// writeKeyPair writes a throwaway self-signed certificate and its key, and
// returns their paths.
//
// Generated rather than checked in: a fixture certificate has an expiry date,
// and a test that starts failing on a Tuesday in some future year for reasons
// nobody can reconstruct is worse than the twenty lines it saves.
func writeKeyPair(t *testing.T) (certPath, keyPath string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "telemetry-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating the certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshaling the key: %v", err)
	}

	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	write := func(path, blockType string, bytes []byte) {
		if writeErr := os.WriteFile(path,
			pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: bytes}), 0o600); writeErr != nil {
			t.Fatalf("writing %s: %v", path, writeErr)
		}
	}
	write(certPath, "CERTIFICATE", der)
	write(keyPath, "EC PRIVATE KEY", keyDER)
	return certPath, keyPath
}

// collectorProbe is an HTTP/1.1 collector on loopback that records which paths
// it was asked for.
//
// It is what makes the protocol choice observable. The two trace exporters
// return the same Go type, so nothing about the returned value says which one
// was built, and the only honest difference is what goes on the wire: the
// http/protobuf exporter posts to the signal's own path, while the gRPC one
// opens an HTTP/2 connection whose preface this server never routes to a
// handler. "The collector was asked for /v1/traces" therefore means
// http/protobuf and nothing else.
type collectorProbe struct {
	mu    sync.Mutex
	paths map[string]bool
}

// startCollectorProbe starts the probe and points every OTLP variable at it.
func startCollectorProbe(t *testing.T) *collectorProbe {
	t.Helper()

	probe := &collectorProbe{paths: map[string]bool{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probe.mu.Lock()
		probe.paths[r.URL.Path] = true
		probe.mu.Unlock()
		// An empty 200 decodes as an empty success response, which is what
		// keeps the http exporter from retrying and slowing the test down.
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", server.URL)
	// The trace and metric gRPC exporters do not read the endpoint's scheme, so
	// without this they would negotiate TLS against a plaintext listener and
	// fail for the wrong reason.
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
	// Milliseconds, and it is what the gRPC leg costs: that exporter retries an
	// Unavailable with a five-second initial backoff, so against a listener
	// that will never speak gRPC it spends exactly this budget before giving
	// up. The http leg needs a thousandth of it for a loopback round trip.
	t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "500")
	// A per-signal override in the developer's own environment would otherwise
	// send the export somewhere this probe cannot see.
	for _, signal := range []string{"TRACES", "METRICS", "LOGS"} {
		t.Setenv("OTEL_EXPORTER_OTLP_"+signal+"_ENDPOINT", "")
		t.Setenv("OTEL_EXPORTER_OTLP_"+signal+"_INSECURE", "")
		t.Setenv("OTEL_EXPORTER_OTLP_"+signal+"_TIMEOUT", "")
	}
	return probe
}

func (p *collectorProbe) asked(path string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.paths[path]
}

// protocolCases are the two answers the protocol argument can carry, and what
// each of them must put on the wire.
func protocolCases() []struct {
	name       string
	protocol   string
	wantPosted bool
} {
	return []struct {
		name       string
		protocol   string
		wantPosted bool
	}{
		{name: "http/protobuf posts to the signal's path", protocol: ProtocolHTTP, wantPosted: true},
		{name: "grpc speaks gRPC and posts nothing", protocol: ProtocolGRPC, wantPosted: false},
	}
}

// exportProbeContext bounds one export attempt, so a gRPC client talking to a
// listener that cannot answer it gives up rather than holding the test.
func exportProbeContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestNewTraceExporter_TheProtocolDecidesWhatTheCollectorIsAsked covers the
// dispatch every trace export goes through.
//
// Both branches return *otlptrace.Exporter, so the value proves nothing and an
// inverted condition here would be invisible to any test that only checked the
// call succeeded: the process would announce telemetry enabled, the operator
// would have configured http/protobuf, and the collector would be receiving
// gRPC on a port that does not speak it.
func TestNewTraceExporter_TheProtocolDecidesWhatTheCollectorIsAsked(t *testing.T) {
	for _, tt := range protocolCases() {
		t.Run(tt.name, func(t *testing.T) {
			probe := startCollectorProbe(t)
			ctx := exportProbeContext(t)

			exporter, err := newTraceExporter(ctx, tt.protocol)
			if err != nil {
				t.Fatalf("newTraceExporter(%q): %v", tt.protocol, err)
			}
			t.Cleanup(func() { _ = exporter.Shutdown(exportProbeContext(t)) })

			// The error is deliberately not asserted: the gRPC leg is expected
			// to fail against this listener, and what is being measured is what
			// the listener saw rather than whether the attempt succeeded.
			_ = exporter.ExportSpans(ctx, []sdktrace.ReadOnlySpan{endedSpan(t)})

			if got := probe.asked("/v1/traces"); got != tt.wantPosted {
				t.Errorf("the collector was asked for /v1/traces = %v under protocol %q, want %v",
					got, tt.protocol, tt.wantPosted)
			}
		})
	}
}

// TestNewMetricExporter_TheProtocolDecidesWhatTheCollectorIsAsked is the same
// dispatch for metrics, asserted the same way so the three cannot drift apart.
func TestNewMetricExporter_TheProtocolDecidesWhatTheCollectorIsAsked(t *testing.T) {
	for _, tt := range protocolCases() {
		t.Run(tt.name, func(t *testing.T) {
			probe := startCollectorProbe(t)
			ctx := exportProbeContext(t)

			exporter, err := newMetricExporter(ctx, tt.protocol)
			if err != nil {
				t.Fatalf("newMetricExporter(%q): %v", tt.protocol, err)
			}
			t.Cleanup(func() { _ = exporter.Shutdown(exportProbeContext(t)) })

			_ = exporter.Export(ctx, &metricdata.ResourceMetrics{
				Resource: resource.Empty(),
				ScopeMetrics: []metricdata.ScopeMetrics{{
					Scope: instrumentation.Scope{Name: "telemetry-test"},
					Metrics: []metricdata.Metrics{{
						Name: "probe",
						Data: metricdata.Sum[int64]{
							Temporality: metricdata.CumulativeTemporality,
							IsMonotonic: true,
							DataPoints:  []metricdata.DataPoint[int64]{{Value: 1}},
						},
					}},
				}},
			})

			if got := probe.asked("/v1/metrics"); got != tt.wantPosted {
				t.Errorf("the collector was asked for /v1/metrics = %v under protocol %q, want %v",
					got, tt.protocol, tt.wantPosted)
			}
		})
	}
}

// TestNewLogExporter_TheProtocolDecidesWhatTheCollectorIsAsked is the same
// dispatch for log records.
func TestNewLogExporter_TheProtocolDecidesWhatTheCollectorIsAsked(t *testing.T) {
	for _, tt := range protocolCases() {
		t.Run(tt.name, func(t *testing.T) {
			probe := startCollectorProbe(t)
			ctx := exportProbeContext(t)

			exporter, err := newLogExporter(ctx, tt.protocol)
			if err != nil {
				t.Fatalf("newLogExporter(%q): %v", tt.protocol, err)
			}
			t.Cleanup(func() { _ = exporter.Shutdown(exportProbeContext(t)) })

			_ = exporter.Export(ctx, []sdklog.Record{{}})

			if got := probe.asked("/v1/logs"); got != tt.wantPosted {
				t.Errorf("the collector was asked for /v1/logs = %v under protocol %q, want %v",
					got, tt.protocol, tt.wantPosted)
			}
		})
	}
}

// endedSpan produces one finished span for an exporter to carry, since
// ExportSpans short-circuits on an empty batch and would reach no client at
// all.
func endedSpan(t *testing.T) sdktrace.ReadOnlySpan {
	t.Helper()

	_, span := sdktrace.NewTracerProvider().Tracer("telemetry-test").Start(context.Background(), "probe")
	span.End()

	readOnly, ok := span.(sdktrace.ReadOnlySpan)
	if !ok {
		t.Fatalf("the SDK returned %T, which an exporter cannot carry", span)
	}
	return readOnly
}
