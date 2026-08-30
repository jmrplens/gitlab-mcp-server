package main

import (
	"context"
	"testing"
	"time"
)

// TestTelemetryEnabled_DefaultsToOff pins the default, which is a privacy
// decision rather than a performance one. Instrumenting a deployment says
// something about the people using it, and a server that exported without being
// asked would be making that call on the operator's behalf.
func TestTelemetryEnabled_DefaultsToOff(t *testing.T) {
	restore := telemetryFlag
	t.Cleanup(func() { telemetryFlag = restore })
	telemetryFlag = nil

	if telemetryEnabled() {
		t.Error("telemetry is on with nothing configured")
	}
}

// TestTelemetryEnabled_FromTheEnvironment covers the switch an operator sets in
// a client's JSON configuration, where there is no command line to pass a flag
// on. That is the ordinary case for stdio, so it is not a fallback.
func TestTelemetryEnabled_FromTheEnvironment(t *testing.T) {
	restore := telemetryFlag
	t.Cleanup(func() { telemetryFlag = restore })
	telemetryFlag = nil

	t.Setenv("GITLAB_MCP_TELEMETRY", "true")
	if !telemetryEnabled() {
		t.Error("GITLAB_MCP_TELEMETRY=true did not enable telemetry")
	}
}

// TestTelemetryEnabled_RejectsTheValuesStrconvWouldAccept pins the grammar at
// the boundary a person actually touches.
//
// The specification's rule is that only the case-insensitive string "true"
// enables, and that an implementation MUST NOT extend that set. strconv.ParseBool
// accepts "1", "t" and "T", so using it here would quietly make this server
// non-conformant in the one place an operator types the value by hand.
func TestTelemetryEnabled_RejectsTheValuesStrconvWouldAccept(t *testing.T) {
	restore := telemetryFlag
	t.Cleanup(func() { telemetryFlag = restore })
	telemetryFlag = nil

	for _, value := range []string{"1", "t", "T", "yes", "on", ""} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("GITLAB_MCP_TELEMETRY", value)
			if telemetryEnabled() {
				t.Errorf("%q enabled telemetry; only case-insensitive \"true\" may", value)
			}
		})
	}
	for _, value := range []string{"true", "TRUE", "tRuE", "  true  "} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("GITLAB_MCP_TELEMETRY", value)
			if !telemetryEnabled() {
				t.Errorf("%q did not enable telemetry; the rule is case-insensitive", value)
			}
		})
	}
}

// TestStartTelemetry_DisabledReturnsAUsableStop asserts that the caller needs no
// nil check and no second branch.
//
// runWithContext defers the returned function unconditionally, so a stop that
// could be nil, or that panicked when telemetry was never started, would turn
// the ordinary no-telemetry path into a crash on exit: the worst place to put
// one, because it happens after the work succeeded.
func TestStartTelemetry_DisabledReturnsAUsableStop(t *testing.T) {
	restore := telemetryFlag
	t.Cleanup(func() { telemetryFlag = restore })
	telemetryFlag = nil
	t.Setenv("GITLAB_MCP_TELEMETRY", "false")

	provider, stop := startTelemetry(t.Context(), "2.7.6")
	if stop == nil {
		t.Fatal("startTelemetry returned a nil stop function; the deferred call in runWithContext would panic")
	}
	if provider == nil {
		t.Fatal("startTelemetry returned a nil provider; the server card would panic reading it")
	}
	if provider.Enabled() {
		t.Error("provider reports itself enabled with the switch off")
	}
	stop(boundedShutdown(t))
}

// TestStartTelemetry_SurvivesAnUnreachableCollector is the decision this project
// makes where the specification permits either answer.
//
// "The API or SDK MAY fail fast and cause the application to fail on
// initialization... but MUST NOT cause the application to fail later at
// runtime." MAY, so this is ours to choose, and the choice is that a server
// which can talk to GitLab keeps doing so when it cannot talk to a collector.
// The official Go examples model the opposite, with twelve calls to panic on
// one page, and copying them would let a telemetry misconfiguration take down a
// working server.
//
// The endpoint is a port nothing listens on. The OTLP exporters connect lazily,
// so this asserts what matters at startup: the process comes up, and stopping
// it does not hang or panic.
func TestStartTelemetry_SurvivesAnUnreachableCollector(t *testing.T) {
	restore := telemetryFlag
	t.Cleanup(func() { telemetryFlag = restore })
	telemetryFlag = nil

	t.Setenv("GITLAB_MCP_TELEMETRY", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
	t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "200")
	t.Setenv("OTEL_BSP_EXPORT_TIMEOUT", "200")

	_, stop := startTelemetry(t.Context(), "2.7.6")

	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), telemetryShutdownTimeout)
		defer cancel()
		stop(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(telemetryShutdownTimeout + 5*time.Second):
		t.Fatal("shutting down against an unreachable collector did not return")
	}
}

// TestStartTelemetry_RefusesAnUnusableProtocolWithoutKillingTheProcess pins the
// other half of the same decision. A configuration this server cannot honor is
// worth an error, and that error belongs in the log rather than in an exit
// code: the operator asked for telemetry, not for the server to stop existing.
func TestStartTelemetry_RefusesAnUnusableProtocolWithoutKillingTheProcess(t *testing.T) {
	restore := telemetryFlag
	t.Cleanup(func() { telemetryFlag = restore })
	telemetryFlag = nil

	t.Setenv("GITLAB_MCP_TELEMETRY", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "carrier-pigeon")

	provider, stop := startTelemetry(t.Context(), "2.7.6")
	if stop == nil {
		t.Fatal("a refused configuration returned a nil stop function")
	}
	if provider.Enabled() {
		t.Error("a protocol this server cannot honor was accepted; nothing would reach the collector and nothing would say so")
	}
	stop(boundedShutdown(t))
}

// TestSDKDisabledVetoesTheFlag asserts the composition an operator relies on
// when they need telemetry off across a fleet without editing every unit file.
//
// OTEL_SDK_DISABLED cannot be this server's on switch, because its specified
// default means "enabled" while telemetry here defaults to off, so adopting it
// as the only switch would invert its meaning. It is a veto layered on top, and
// it has to beat an explicit request.
func TestSDKDisabledVetoesTheFlag(t *testing.T) {
	restore := telemetryFlag
	t.Cleanup(func() { telemetryFlag = restore })
	telemetryFlag = nil

	t.Setenv("GITLAB_MCP_TELEMETRY", "true")
	t.Setenv("OTEL_SDK_DISABLED", "true")

	provider, stop := startTelemetry(t.Context(), "2.7.6")
	t.Cleanup(func() { stop(boundedShutdown(t)) })

	if provider.Enabled() {
		t.Error("OTEL_SDK_DISABLED=true did not veto an explicitly requested start")
	}
}
