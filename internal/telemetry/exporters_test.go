// exporters_test.go verifies what the startup summary is allowed to say about
// the collector it exports to.
package telemetry

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
