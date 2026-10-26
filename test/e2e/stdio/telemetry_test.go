//go:build stdioe2e

package stdioe2e

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestTelemetry_EnabledWithAnUnreachableCollectorKeepsStdoutClean is the test
// the whole telemetry design rests on, and it cannot live in a unit test.
//
// On stdio, stdout carries JSON-RPC and nothing else: one stray byte ends the
// session, and the failure presents as a client that stops working rather than
// as an error anybody can read. Telemetry introduces several ways to put bytes
// there, all of them plausible and none visible from inside a package:
//
//   - the stdout log exporter defaults to os.Stdout, and OTEL_LOGS_EXPORTER
//     "console" selects it, which is a gesture the OpenTelemetry documentation
//     recommends;
//   - OpenTelemetry's default error handler ends in log.Print, whose
//     destination is the standard library's process-global logger, so any
//     dependency calling log.SetOutput(os.Stdout) would retarget every internal
//     export failure into the JSON-RPC stream;
//   - the rendered example for otel.SetLogger on pkg.go.dev builds its logger
//     over os.Stdout.
//
// A unit test would assert against a handler chain it assembled itself, which
// is testing the copy. This drives the real binary with telemetry on and an
// endpoint nothing listens on, so every export fails and every failure path
// runs.
func TestTelemetry_EnabledWithAnUnreachableCollectorKeepsStdoutClean(t *testing.T) {
	gitlab := startFakeGitLab(t)

	env := baseEnv(gitlab.URL)
	env["GITLAB_MCP_TELEMETRY"] = "true"
	// Port 1 is reserved and nothing listens there, so every batch fails.
	env["OTEL_EXPORTER_OTLP_ENDPOINT"] = "http://127.0.0.1:1"
	// Milliseconds, integers only: the OTEL_ namespace defines every duration
	// that way, and "200ms" would parse as nothing and silently keep the
	// default. Short so the exporter has actually tried and failed before the
	// session ends.
	env["OTEL_EXPORTER_OTLP_TIMEOUT"] = "200"
	env["OTEL_BSP_SCHEDULE_DELAY"] = "100"
	env["OTEL_BSP_EXPORT_TIMEOUT"] = "200"
	env["OTEL_BLRP_SCHEDULE_DELAY"] = "100"
	env["OTEL_METRIC_EXPORT_INTERVAL"] = "100"

	session := startSession(t, env)

	// Every readMessage asserts the line parses as JSON, so an export failure
	// leaking onto stdout fails here with the offending line quoted.
	session.call(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"stdio-e2e","version":"0"}}}`)
	session.send(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	// Enough traffic that the batch processor's schedule fires during the
	// session rather than only at shutdown, which is the window where an
	// export failure would be written.
	for id := 2; id <= 5; id++ {
		session.call(t, `{"jsonrpc":"2.0","id":`+strconv.Itoa(id)+`,"method":"tools/list"}`)
	}
	time.Sleep(500 * time.Millisecond)
	session.call(t, `{"jsonrpc":"2.0","id":6,"method":"tools/list"}`)

	if !session.alive() {
		t.Fatalf("the server exited while telemetry was failing to export\nstderr: %s", session.stderrText())
	}

	// The failures must be visible, on stderr. A telemetry stack that fails
	// silently is worse than one that fails loudly: an operator would see an
	// empty dashboard and no reason for it. Asserted before the shutdown so it
	// holds on every platform, including the one that cannot reach the shutdown
	// below.
	session.waitForStderr(t, "telemetry", 5*time.Second)

	// Shutdown flushes, which is the other moment an exporter writes. Closing
	// stdin is the stdio shutdown a client actually performs, and the portable
	// one. This once hung on Windows and was blamed on the flush; the cause
	// was the harness reading notifications as responses and leaving the real
	// ones unread in a pipe too small to hold them, so the server's last write
	// never returned. call now matches on the id, and the exit is asserted on
	// every platform.
	if _, exited := session.closeStdinAndWait(t, 20*time.Second); !exited {
		t.Errorf("the server did not exit; a telemetry flush against a dead collector held it open\nstderr: %s",
			session.stderrText())
	}
}

// TestTelemetry_DisabledByDefault asserts that a server nobody asked to
// instrument does not connect anywhere.
//
// Off by default is a privacy decision rather than a performance one, and it is
// the kind of default that erodes quietly: a refactor that made the switch
// default to true would break no test unless one asserts the default here,
// against the real binary, in the transport an ordinary client uses.
func TestTelemetry_DisabledByDefault(t *testing.T) {
	gitlab := startFakeGitLab(t)

	env := baseEnv(gitlab.URL)
	// Deliberately pointed somewhere unreachable. If telemetry were on despite
	// nothing asking for it, this is where it would try to go, and the log
	// would say so.
	env["OTEL_EXPORTER_OTLP_ENDPOINT"] = "http://127.0.0.1:1"

	session := startSession(t, env)
	session.call(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"stdio-e2e","version":"0"}}}`)
	session.send(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	session.call(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)

	// Anchored on a line the server writes after its telemetry decision, so
	// the absence asserted below is an absence in what was logged rather than
	// in what the harness had copied so far.
	stderr := session.waitForStderr(t, "starting MCP server", 5*time.Second)
	if strings.Contains(stderr, "telemetry enabled") {
		t.Errorf("telemetry started without being asked for\nstderr: %s", stderr)
	}
	session.terminate(t, 10*time.Second)
}

// TestTelemetry_SDKDisabledVetoesTheSwitch drives the composition an operator
// relies on to turn telemetry off across a fleet without editing every unit
// file.
//
// OTEL_SDK_DISABLED cannot be the on switch, because its specified default
// means "enabled" while this server's telemetry is off until asked for, so
// adopting it as the only switch would invert its meaning for everyone who
// already knows it. It is a veto layered on top, and the point of asserting it
// here rather than only in a unit test is that the veto has to survive the real
// startup path, where the switch is read in one place and the veto in another.
func TestTelemetry_SDKDisabledVetoesTheSwitch(t *testing.T) {
	gitlab := startFakeGitLab(t)

	env := baseEnv(gitlab.URL)
	env["GITLAB_MCP_TELEMETRY"] = "true"
	env["OTEL_SDK_DISABLED"] = "true"
	env["OTEL_EXPORTER_OTLP_ENDPOINT"] = "http://127.0.0.1:1"

	session := startSession(t, env)
	session.call(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"stdio-e2e","version":"0"}}}`)

	// The veto is logged before the server serves anything, so it is on its
	// way by the time initialize is answered; waited for rather than read,
	// because the harness copies stderr on a goroutine of its own and the
	// buffer can trail the pipe by the very line this asserts on.
	stderr := session.waitForStderr(t, "OTEL_SDK_DISABLED", 5*time.Second)
	if strings.Contains(stderr, "telemetry enabled") {
		t.Errorf("OTEL_SDK_DISABLED=true did not veto an explicitly requested start\nstderr: %s", stderr)
	}
	session.terminate(t, 10*time.Second)
}

// identityTelemetryEnv is the environment of a stdio server exporting
// telemetry, to a port nothing listens on, with one identity setting set to
// value. The pseudonymisation key is left out, since beside a configured key
// the rotation is ignored.
func identityTelemetryEnv(t *testing.T, variable, value string) map[string]string {
	t.Helper()
	env := baseEnv(startFakeGitLab(t).URL)
	env["GITLAB_MCP_TELEMETRY"] = "true"
	env["OTEL_EXPORTER_OTLP_ENDPOINT"] = "http://127.0.0.1:1"
	env["OTEL_EXPORTER_OTLP_TIMEOUT"] = "200"
	env[variable] = value
	return env
}

// TestTelemetry_AnUnusableIdentitySetting_RefusesToStart pins the refusal
// register row IDN-009 states (issue 957) against the real binary. A policy
// the server does not know, or a rotation that is not a duration, used to
// start a server that recorded nothing about callers, and the only sign of
// why was in ERROR lines of its log, easy to miss behind a dashboard that
// simply stayed empty. With telemetry on the start is refused now, at ERROR,
// naming the variable, before anything is written on stdout.
func TestTelemetry_AnUnusableIdentitySetting_RefusesToStart(t *testing.T) {
	for _, tc := range []struct{ name, variable, value, says string }{
		{
			name: "a policy it does not know", variable: "GITLAB_MCP_TELEMETRY_IDENTITY", value: "pseudonimous",
			says: `GITLAB_MCP_TELEMETRY_IDENTITY: unknown identity policy "pseudonimous"`,
		},
		{
			name: "a rotation that is not a duration", variable: "GITLAB_MCP_TELEMETRY_IDENTITY_ROTATION", value: "24",
			says: `GITLAB_MCP_TELEMETRY_IDENTITY_ROTATION: time: missing unit in duration "24"`,
		},
		{
			name: "a rotation past thirty days", variable: "GITLAB_MCP_TELEMETRY_IDENTITY_ROTATION", value: "721h",
			says: "GITLAB_MCP_TELEMETRY_IDENTITY_ROTATION: telemetry identity key rotation 721h0m0s is out of range",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startSession(t, identityTelemetryEnv(t, tc.variable, tc.value))
			code, exited := s.waitExit(t, 20*time.Second)
			if !exited {
				t.Fatalf("a stdio server with %s=%s and telemetry on started instead of refusing\nstderr: %s",
					tc.variable, tc.value, s.stderrText())
			}
			if code == 0 {
				t.Errorf("the refusal exited 0\nstderr: %s", s.stderrText())
			}
			refused := awaitLogRecord(t, s, "server exited with error", 5*time.Second)
			if level, _ := refused["level"].(string); level != "ERROR" {
				t.Errorf("the refusal is at %s, want ERROR", level)
			}
			if said, _ := refused["error"].(string); !strings.Contains(said, tc.says) {
				t.Errorf("the refusal says %q, want it to carry %q", said, tc.says)
			}
			if logs := s.stderrText(); strings.Contains(logs, "telemetry enabled") || strings.Contains(logs, stdioStartedLine) {
				t.Errorf("the refused run started telemetry or the server:\n%s", logs)
			}
			if b, err := s.stdout.ReadByte(); err == nil {
				t.Errorf("the refused run wrote %q on stdout, which only JSON-RPC may use", b)
			}
		})
	}
}

// TestTelemetry_AnIdentitySettingTelemetryDoesNotUse_RefusesNothing is the
// other half: the refusal is for a value telemetry is about to use, and a
// value nothing is exported under refuses nothing. With telemetry off, or
// vetoed by OTEL_SDK_DISABLED, a policy the server does not know stops
// nothing, since a deployment that never exports should not be taken down by
// a privacy setting of its exports. And beside a configured key the rotation
// is ignored, so a typo in it stops nothing either.
func TestTelemetry_AnIdentitySettingTelemetryDoesNotUse_RefusesNothing(t *testing.T) {
	for _, tc := range []struct {
		name            string
		variable, value string
		// beside is the variable that leaves nothing exported under the
		// value, or has it ignored, and besideValue what it is set to.
		beside, besideValue string
	}{
		{
			name: "telemetry off", variable: "GITLAB_MCP_TELEMETRY_IDENTITY", value: "pseudonimous",
			beside: "GITLAB_MCP_TELEMETRY", besideValue: "false",
		},
		{
			name: "telemetry vetoed by OTEL_SDK_DISABLED", variable: "GITLAB_MCP_TELEMETRY_IDENTITY", value: "pseudonimous",
			beside: "OTEL_SDK_DISABLED", besideValue: "true",
		},
		{
			name: "a rotation beside a configured key", variable: "GITLAB_MCP_TELEMETRY_IDENTITY_ROTATION", value: "24",
			beside: "GITLAB_MCP_TELEMETRY_IDENTITY_KEY", besideValue: "a deployment-wide secret",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := identityTelemetryEnv(t, tc.variable, tc.value)
			env[tc.beside] = tc.besideValue
			s := startSession(t, env)
			if got := s.call(t, currentUserCall(1)); !served(got) {
				t.Fatalf("the server did not serve: %v\nstderr: %s", got, s.stderrText())
			}
			if logs := s.stderrText(); strings.Contains(logs, "server exited with error") {
				t.Errorf("the run reported a refusal:\n%s", logs)
			}
			s.terminate(t, 10*time.Second)
		})
	}
}
