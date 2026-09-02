//go:build httpe2e

package httpe2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// telemetryEnv is the environment a deployment sets to export, pointed at a
// port nothing listens on so every batch fails and every failure path runs.
//
// The durations are integers because the specification says so: "Any value that
// represents a timeout MUST be an integer representing a number of
// milliseconds". Writing "200ms" here would parse as nothing, silently keep the
// ten-second default, and make these tests slow for a reason nobody could see.
func telemetryEnv() map[string]string {
	return map[string]string{
		"GITLAB_MCP_TELEMETRY":        "true",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:1",
		"OTEL_EXPORTER_OTLP_TIMEOUT":  "200",
		"OTEL_BSP_SCHEDULE_DELAY":     "100",
		"OTEL_BSP_EXPORT_TIMEOUT":     "200",
		"OTEL_METRIC_EXPORT_INTERVAL": "100",
		"OTEL_METRIC_EXPORT_TIMEOUT":  "200",
	}
}

// TestTelemetry_ServerCardAnnouncesItWithoutNamingTheCollector drives the two
// requirements that pull against each other, over the wire rather than through
// the builder.
//
// The card must say telemetry is running, because the switch is off by default
// for privacy and a privacy default nobody can observe is worth less than one
// they can. It must not say where the telemetry goes, because the collector
// address names the operator's own infrastructure and this document is served
// to anyone who asks for it.
func TestTelemetry_ServerCardAnnouncesItWithoutNamingTheCollector(t *testing.T) {
	env := telemetryEnv()
	env["OTEL_EXPORTER_OTLP_ENDPOINT"] = "http://collector.internal.example:4318"
	srv := startServer(t, env)

	got := srv.do(t, request{method: http.MethodGet, path: "/server-card"})
	if got.status != http.StatusOK {
		t.Fatalf("GET /server-card = %d, want 200: %s", got.status, got.body)
	}

	if strings.Contains(got.body, "collector.internal.example") {
		t.Errorf("the card names the collector, and it is served to anyone who asks: %s", got.body)
	}

	var card map[string]any
	if err := json.Unmarshal([]byte(got.body), &card); err != nil {
		t.Fatalf("the card is not JSON: %v", err)
	}
	block, ok := card["telemetry"].(map[string]any)
	if !ok {
		t.Fatalf("the card carries no telemetry block while telemetry is running: %s", got.body)
	}
	if enabled, _ := block["enabled"].(bool); !enabled {
		t.Error("the telemetry block does not report itself enabled")
	}
	for _, key := range []string{"signals", "recorded", "not_recorded"} {
		t.Run(key, func(t *testing.T) {
			if _, present := block[key]; !present {
				t.Errorf("the telemetry block has no %q; announcing without saying what is captured is not announcing", key)
			}
		})
	}
}

// TestTelemetry_CollectorCredentialsNeverReachTheLogs is the privacy assertion
// behind the answer to "can an operator authenticate their telemetry".
//
// They can, and the mechanism is entirely the specification's:
// OTEL_EXPORTER_OTLP_HEADERS carries whatever the collector requires, and it
// works precisely because this server passes its exporters no options at all.
// Had we passed a WithHeaders of our own, the variable would be silently dead,
// since Go applies programmatic options after the environment.
//
// The obligation that creates is this one: a value we never read is a value we
// must never print. A bearer token in a log line is a credential leak whether
// or not it was ours to begin with, and an export failure is exactly when a
// server is most tempted to log the request it was making.
// The four shapes below are a table because the guarantee held for exactly one
// of them, and that one is the shape that reaches no logging branch at all.
//
// The SDK prints what this server does not. Its header parser logs the raw pair
// when one has no "=", logs the raw value when percent-decoding fails, and the
// log exporter hands the error handler the *entire* variable, so a typo in a
// non-credential pair prints every credential beside it. The third malformation
// is the sharp one: the credential itself is perfectly well formed and only a
// neighboring key is not.
func TestTelemetry_CollectorCredentialsNeverReachTheLogs(t *testing.T) {
	const secret = "s3cr3t-collector-token"

	tests := []struct {
		name  string
		value string
		// wellFormed says nothing about the variable should appear at all,
		// which is only true when the SDK had no complaint to make. Once it
		// has one, naming the variable is what an operator needs: the fix is
		// that the line no longer quotes its value.
		wellFormed bool
	}{
		{
			// W3C Baggage syntax, so the space is percent-encoded. Writing it
			// literally would make the pair unparseable, which is the next case
			// rather than this one.
			name:       "a well-formed credential",
			value:      "Authorization=Bearer%20" + secret,
			wellFormed: true,
		},
		{
			name:  "a pair with no equals sign",
			value: "Authorization: Bearer " + secret,
		},
		{
			name:  "a value that does not percent-decode",
			value: "authorization=Bearer%2" + secret,
		},
		{
			name:  "a valid credential beside an invalid key",
			value: "api-key=" + secret + ",x tenant=acme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := telemetryEnv()
			env["OTEL_EXPORTER_OTLP_HEADERS"] = tt.value
			srv := startServer(t, env)

			// Traffic, so the exporter has genuinely tried and failed.
			for range 3 {
				srv.do(t, request{body: `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`})
			}

			logs := srv.logs()
			if strings.Contains(logs, secret) {
				t.Errorf("the collector credential appears in the server's own logs: %s", logs)
			}
			if tt.wellFormed && strings.Contains(logs, "OTEL_EXPORTER_OTLP_HEADERS") {
				t.Errorf("the credential variable is named in the logs with nothing wrong with it, which invites printing its value next: %s", logs)
			}
		})
	}
}

// TestTelemetry_AnUnreachableCollectorChangesNoResponse is the decision this
// project makes where the specification permits either answer.
//
// "The API or SDK MAY fail fast and cause the application to fail on
// initialization... but MUST NOT cause the application to fail later at
// runtime" is a MAY, so the choice is ours, and the choice is that a server
// which can talk to GitLab keeps serving when it cannot talk to a collector.
//
// The assertion is a comparison rather than a fixed expectation, and that is
// the point. Two servers, identical but for telemetry, are asked the same
// questions and every answer must match. Asserting a particular status instead
// would be testing the harness's own credential setup, and would pass just as
// happily if telemetry turned every rejection into a different rejection.
func TestTelemetry_AnUnreachableCollectorChangesNoResponse(t *testing.T) {
	quiet := startServer(t, nil)
	instrumented := startServer(t, telemetryEnv())

	// bodyIsDeterministic is false for /health, whose payload carries the
	// process start time and uptime. Two servers started a second apart differ
	// there for reasons that have nothing to do with telemetry, and comparing
	// it anyway made this test fail whenever the suite ran long enough for the
	// clock to tick between them. The status is still compared, which is what
	// the assertion is actually about.
	cases := []struct {
		call                request
		bodyIsDeterministic bool
	}{
		{call: request{body: `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`}, bodyIsDeterministic: true},
		{call: request{body: `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"gitlab_execute_action","arguments":{"action":"issue.list"}}}`}, bodyIsDeterministic: true},
		{call: request{body: `{"jsonrpc":"2.0","id":3,"method":"nonexistent/method"}`}, bodyIsDeterministic: true},
		{call: request{method: http.MethodGet, path: "/health"}, bodyIsDeterministic: false},
	}

	for _, tc := range cases {
		t.Run(subtestName(tc.call), func(t *testing.T) {
			want := quiet.do(t, tc.call)
			got := instrumented.do(t, tc.call)

			if got.status != want.status {
				t.Errorf("status %d with telemetry and %d without", got.status, want.status)
			}
			if tc.bodyIsDeterministic && got.body != want.body {
				t.Errorf("the body differs with telemetry enabled: got %q, want %q", got.body, want.body)
			}
		})
	}

	// Nothing about the collector may reach a client, whatever the outcome was.
	for _, tc := range cases {
		t.Run(subtestName(tc.call)+" leaks nothing", func(t *testing.T) {
			body := strings.ToLower(instrumented.do(t, tc.call).body)
			for _, leak := range []string{"127.0.0.1:1", "otlp", "collector", "telemetry"} {
				t.Run(leak, func(t *testing.T) {
					if strings.Contains(body, leak) {
						t.Errorf("%q reached a client response: %s", leak, body)
					}
				})
			}
		})
	}
}

// TestTelemetry_OffByDefaultOverHTTP asserts the default against the real
// binary in the transport a hosted deployment uses.
//
// Off by default is a privacy decision, and it is the kind that erodes
// silently: a change making the switch default to true would break no test
// unless one asserts the default here. The endpoint is set and unreachable, so
// if telemetry were running despite nothing asking, the log would say so.
func TestTelemetry_OffByDefaultOverHTTP(t *testing.T) {
	srv := startServer(t, map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:1",
	})

	srv.do(t, request{body: `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`})

	if logs := srv.logs(); strings.Contains(logs, "telemetry enabled") {
		t.Errorf("telemetry started without being asked for: %s", logs)
	}

	got := srv.do(t, request{method: http.MethodGet, path: "/server-card"})
	var card map[string]any
	if err := json.Unmarshal([]byte(got.body), &card); err != nil {
		t.Fatalf("the card is not JSON: %v", err)
	}
	if _, present := card["telemetry"]; present {
		t.Errorf("the card carries a telemetry block with telemetry off: %s", got.body)
	}
}

// subtestName names a comparison case without slashes, which -run would read
// as subtest hierarchy, and without the whole JSON payload.
func subtestName(r request) string {
	name := r.path
	if name == "" {
		var msg struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal([]byte(r.body), &msg)
		name = msg.Method
	}
	if name == "" {
		name = "empty"
	}
	return strings.ReplaceAll(strings.TrimPrefix(name, "/"), "/", " ")
}
