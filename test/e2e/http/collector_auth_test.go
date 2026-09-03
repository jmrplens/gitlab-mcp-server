//go:build httpe2e

package httpe2e

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestCollectorAuth_EveryOption drives each way an operator can authenticate to
// their collector, and asserts the credential arrives on the wire exactly as
// they wrote it.
//
// # Why the whole matrix
//
// The specification defines no username/password variable and no auth mode:
// every scheme is a header, carried by OTEL_EXPORTER_OTLP_HEADERS in W3C
// Baggage syntax. That makes the surface look trivial and hides two traps worth
// a test each.
//
// The value is percent-decoded, so a space must be written %20 and a literal
// percent must be written %25. A Bearer token written with a real space is
// dropped as a malformed pair, and nothing says so above debug level: the
// operator sees an unauthenticated export and a collector rejecting it.
//
// Basic auth is base64, which ends in "=" padding, and "=" is also the pair
// separator. Go cuts at the FIRST "=", so the padding survives, but that is a
// property worth pinning rather than assuming: an implementation splitting on
// every "=" would truncate every basic credential ever configured.
//
// # And why the unauthenticated case is here
//
// Because it is the default and it must stay working. A collector on the same
// host, or one behind a network boundary, needs no credential at all, and a
// server that grew a mandatory one would break every such deployment. The first
// case asserts the absence.
func TestCollectorAuth_EveryOption(t *testing.T) {
	basic := base64.StdEncoding.EncodeToString([]byte("otel-user:otel-pass"))

	tests := []struct {
		name    string
		headers string
		assert  func(t *testing.T, got http.Header)
	}{
		{
			name:    "no credential at all, which is the default",
			headers: "",
			assert: func(t *testing.T, got http.Header) {
				t.Helper()
				if value := got.Get("Authorization"); value != "" {
					t.Errorf("an Authorization header was sent with none configured: %q", value)
				}
			},
		},
		{
			name:    "bearer token, with the space percent-encoded",
			headers: "Authorization=Bearer%20collector-bearer-token",
			assert: func(t *testing.T, got http.Header) {
				t.Helper()
				if value := got.Get("Authorization"); value != "Bearer collector-bearer-token" {
					t.Errorf("Authorization = %q, want the decoded bearer credential", value)
				}
			},
		},
		{
			name:    "basic auth, whose base64 padding must survive the pair split",
			headers: "Authorization=Basic%20" + basic,
			assert: func(t *testing.T, got http.Header) {
				t.Helper()
				want := "Basic " + basic
				if value := got.Get("Authorization"); value != want {
					t.Errorf("Authorization = %q, want %q; the base64 padding was truncated", value, want)
				}
			},
		},
		{
			name:    "an api-key header, which is what several vendors want",
			headers: "api-key=abcdef123456",
			assert: func(t *testing.T, got http.Header) {
				t.Helper()
				if value := got.Get("Api-Key"); value != "abcdef123456" {
					t.Errorf("Api-Key = %q, want the configured key", value)
				}
			},
		},
		{
			name:    "several headers at once, comma separated",
			headers: "Authorization=Bearer%20tok,x-tenant=acme,x-env=prod",
			assert: func(t *testing.T, got http.Header) {
				t.Helper()
				for header, want := range map[string]string{
					"Authorization": "Bearer tok",
					"X-Tenant":      "acme",
					"X-Env":         "prod",
				} {
					t.Run(header, func(t *testing.T) {
						if value := got.Get(header); value != want {
							t.Errorf("%s = %q, want %q", header, value, want)
						}
					})
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := startCollector(t)
			env := collectorEnv(c)
			if tc.headers != "" {
				env["OTEL_EXPORTER_OTLP_HEADERS"] = tc.headers
			}
			srv := startServer(t, env)

			// Traffic, so there is something to export.
			for range 3 {
				srv.do(t, request{body: `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`})
			}

			exports := c.awaitExport(t, 15*time.Second)
			tc.assert(t, exports[0].headers)
		})
	}
}

// TestCollectorAuth_AMalformedHeaderDoesNotStopTheServer covers the operator
// who writes a real space instead of %20, which is the mistake this syntax
// invites.
//
// The pair is dropped and the export goes out unauthenticated, which the
// collector will reject. That is bad, and it is still better than the
// alternative: refusing to start, or failing requests, because a telemetry
// credential was written wrong. The server keeps serving GitLab.
func TestCollectorAuth_AMalformedHeaderDoesNotStopTheServer(t *testing.T) {
	c := startCollector(t)
	env := collectorEnv(c)
	env["OTEL_EXPORTER_OTLP_HEADERS"] = "Authorization=Bearer with a literal space"
	srv := startServer(t, env)

	got := srv.do(t, request{method: http.MethodGet, path: "/health"})
	if got.status != http.StatusOK {
		t.Fatalf("/health = %d after a malformed telemetry header: %s", got.status, got.body)
	}

	// And the credential must not appear anywhere in the logs, malformed or not.
	if logs := srv.logs(); strings.Contains(logs, "Bearer with a literal space") {
		t.Errorf("the malformed credential was logged verbatim: %s", logs)
	}
}
