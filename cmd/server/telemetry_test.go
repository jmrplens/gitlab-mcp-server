package main

import (
	"bytes"
	"context"
	"flag"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/telemetry"
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

	provider, stop := startTelemetry(t.Context(), "2.7.6", config.ToolSurfaceDynamic)
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

	_, stop := startTelemetry(t.Context(), "2.7.6", config.ToolSurfaceDynamic)

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

	provider, stop := startTelemetry(t.Context(), "2.7.6", config.ToolSurfaceDynamic)
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

	provider, stop := startTelemetry(t.Context(), "2.7.6", config.ToolSurfaceDynamic)
	t.Cleanup(func() { stop(boundedShutdown(t)) })

	if provider.Enabled() {
		t.Error("OTEL_SDK_DISABLED=true did not veto an explicitly requested start")
	}
}

// TestInstallSlogBridge_DoesNotRecurse is the regression for a hang whose stack
// trace explains nothing until you know the trick.
//
// slog.SetDefault does two things, and the second is easy to miss: it also
// points the standard library's log package at the new default handler. So a
// bridge that wraps whatever slog.Default() currently holds is safe only while
// that is a handler somebody installed. When it is still slog's own built-in
// handler, which writes through log.Print, the chain closes on itself:
//
//	fanOutHandler.Handle -> slog.defaultHandler.Handle -> log.Logger.output
//	  -> slog.handlerWriter.Write -> fanOutHandler.Handle -> ...
//
// It presents as a process that never finishes and produces no output, which is
// how it went unnoticed until a package's tests went from a hundred seconds to
// a timeout. In this binary main installs a JSON handler first so the cycle
// never fires in production, and depending on that ordering is precisely the
// fragility this test exists to prevent.
//
// The assertion is the timeout rather than the content: infinite recursion has
// no wrong value to compare against, only a call that does not come back.
func TestInstallSlogBridge_DoesNotRecurse(t *testing.T) {
	var buf bytes.Buffer
	restoreBase := baseLogHandler
	baseLogHandler = slog.NewJSONHandler(&buf, nil)
	t.Cleanup(func() { baseLogHandler = restoreBase })

	restore := installSlogBridge()
	t.Cleanup(restore)

	done := make(chan struct{})
	go func() {
		slog.Info("a record that must return")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("logging through the bridge did not return; the handler is calling itself")
	}

	if buf.Len() == 0 {
		t.Error("the record never reached the base handler")
	}
}

// TestInstallSlogBridge_WithoutABaseHandlerIsANoOp covers the path a caller
// reaches by starting telemetry before the logger is configured.
//
// There is no known-safe handler to wrap at that point, and wrapping the
// built-in one is what recurses. Skipping costs the log export and keeps the
// process alive, which is the right trade for an ordering nothing should rely
// on. Returning a usable restore function matters too: the caller defers it
// unconditionally.
func TestInstallSlogBridge_WithoutABaseHandlerIsANoOp(t *testing.T) {
	restoreBase := baseLogHandler
	baseLogHandler = nil
	t.Cleanup(func() { baseLogHandler = restoreBase })

	before := slog.Default()
	restore := installSlogBridge()
	if restore == nil {
		t.Fatal("a nil restore function would panic in the deferred call that always runs")
	}
	if slog.Default() != before {
		t.Error("the default logger was replaced with no base handler to wrap")
	}
	restore()
}

// TestInstallSlogBridge_RestoresThePreviousLogger pins the symmetry.
//
// The default logger is a process global, so a bridge installed and never
// removed outlives the provider it writes into. In production that is a stopped
// exporter still receiving records during shutdown; in a test binary it is one
// test's telemetry configuration poisoning every later test's logging, which is
// how a package first started timing out.
func TestInstallSlogBridge_RestoresThePreviousLogger(t *testing.T) {
	var buf bytes.Buffer
	restoreBase := baseLogHandler
	baseLogHandler = slog.NewJSONHandler(&buf, nil)
	t.Cleanup(func() { baseLogHandler = restoreBase })

	before := slog.Default()
	restore := installSlogBridge()
	if slog.Default() == before {
		t.Fatal("the bridge did not replace the default logger, so this test proves nothing")
	}

	restore()
	if slog.Default() != before {
		t.Error("the previous logger was not restored")
	}
}

// boundedShutdown returns a context a test can safely pass to Shutdown.
//
// Never context.Background(). The provider-level Shutdown honors only the
// caller's context: the SDK's own 30s default applies per export, not to the
// whole drain. So an unbounded context against a collector that is not there
// waits forever, and the failure presents as a test binary that never finishes
// rather than as an assertion anybody can read.
//
// This was not hypothetical. Adding the logs pipeline gave Shutdown something
// real to drain, and this package went from a hundred seconds to a timeout.
// Nothing had changed in those tests; they had simply been passing an unbounded
// context to a call that previously had nothing to wait for.
func boundedShutdown(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestDropToolNameFromMetrics_ReadsTheFlagThenTheEnvironmentThenAuto covers the
// switch that decides whether gen_ai.tool.name is a metric dimension.
//
// Auto is the default because the answer differs per surface: the individual
// surface registers about a thousand tools, and one time series per tool
// exhausts the SDK's cardinality limit — which it answers by collapsing the
// long tail into a single otel.metric.overflow bucket, first-come-wins under
// cumulative temporality, so the loss is silent. A value this build cannot
// parse falls back to auto with a line rather than stopping a server that is
// already coming up.
func TestDropToolNameFromMetrics_ReadsTheFlagThenTheEnvironmentThenAuto(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		flag    string
		surface string
		want    bool
	}{
		{name: "auto keeps the name on the dynamic surface", surface: config.ToolSurfaceDynamic, want: false},
		{name: "auto drops it on the individual surface", surface: config.ToolSurfaceIndividual, want: true},
		{name: "the environment can force it off", env: "off", surface: config.ToolSurfaceDynamic, want: true},
		{name: "the environment can force it on", env: "on", surface: config.ToolSurfaceIndividual, want: false},
		{name: "the flag beats the environment", env: "off", flag: "on", surface: config.ToolSurfaceIndividual, want: false},
		{name: "an unusable value falls back to auto", env: "sometimes", surface: config.ToolSurfaceIndividual, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFreshFlagSet(t)
			t.Setenv(telemetry.EnvToolNameName, tt.env)

			previous := telemetryToolNameFlag
			t.Cleanup(func() { telemetryToolNameFlag = previous })
			telemetryToolNameFlag = flag.String("telemetry-tool-name", "", "")
			if tt.flag != "" {
				if err := flag.CommandLine.Parse([]string{"-telemetry-tool-name=" + tt.flag}); err != nil {
					t.Fatalf("parsing: %v", err)
				}
			}

			if got := dropToolNameFromMetrics(tt.surface); got != tt.want {
				t.Errorf("dropToolNameFromMetrics(%q) = %v, want %v", tt.surface, got, tt.want)
			}
		})
	}
}

// TestTelemetryEnabled_ThePassedFlagIsTheAnswer covers the switch's top
// precedence level.
//
// The environment variable is what a fleet sets and the flag is what one
// deployment types, so a flag that was actually passed has to win in both
// directions — including the one that turns telemetry off on a host whose
// environment turns it on, which is the case an operator uses to isolate a
// misbehaving collector.
func TestTelemetryEnabled_ThePassedFlagIsTheAnswer(t *testing.T) {
	tests := []struct {
		name string
		env  string
		flag string
		want bool
	}{
		{name: "the flag turns it on over an environment that says off", env: "false", flag: "true", want: true},
		{name: "the flag turns it off over an environment that says on", env: "true", flag: "false", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFreshFlagSet(t)
			t.Setenv(telemetry.EnvSwitchName, tt.env)

			previous := telemetryFlag
			t.Cleanup(func() { telemetryFlag = previous })
			telemetryFlag = flag.Bool("telemetry", false, "")
			if err := flag.CommandLine.Parse([]string{"-telemetry=" + tt.flag}); err != nil {
				t.Fatalf("parsing: %v", err)
			}

			if got := telemetryEnabled(); got != tt.want {
				t.Errorf("telemetryEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// levelCapturingHandler records the messages a logger emits, refusing anything
// below its threshold exactly as the stderr handler built from LOG_LEVEL does.
type levelCapturingHandler struct {
	threshold slog.Level
	mu        *sync.Mutex
	messages  *[]string
}

func (h levelCapturingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.threshold
}

func (h levelCapturingHandler) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.messages = append(*h.messages, record.Message)
	return nil
}

func (h levelCapturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h levelCapturingHandler) WithGroup(string) slog.Handler { return h }

// TestStartTelemetry_TheEnabledAnnouncement_SurvivesAWarnLogLevel verifies that
// the record naming the collector this process exports to is emitted above the
// level LOG_LEVEL can suppress.
//
// It is the only local evidence the channel exists. A deployment running at
// LOG_LEVEL=warn or error otherwise ships its whole operational log to a
// collector with nothing on its own stderr saying so, and the exported copy is
// no help: an operator looking for that is looking at the machine, not at the
// destination they are trying to discover.
func TestStartTelemetry_TheEnabledAnnouncement_SurvivesAWarnLogLevel(t *testing.T) {
	restore := telemetryFlag
	t.Cleanup(func() { telemetryFlag = restore })
	telemetryFlag = nil

	t.Setenv("GITLAB_MCP_TELEMETRY", "true")

	var mu sync.Mutex
	var messages []string
	capture := levelCapturingHandler{threshold: slog.LevelWarn, mu: &mu, messages: &messages}
	// Both, because both are what LOG_LEVEL reaches: main builds the stderr
	// handler at that level and keeps it as baseLogHandler, and the telemetry
	// bridge gates its stderr leg on that handler rather than on the default
	// logger it replaces.
	previous, previousBase := slog.Default(), baseLogHandler
	t.Cleanup(func() {
		slog.SetDefault(previous)
		baseLogHandler = previousBase
	})
	baseLogHandler = capture
	slog.SetDefault(slog.New(capture))

	provider, stop := startTelemetry(t.Context(), "2.7.6", config.ToolSurfaceDynamic)
	t.Cleanup(func() { stop(boundedShutdown(t)) })
	if !provider.Enabled() {
		t.Fatal("telemetry did not start, so there was no announcement to make")
	}

	mu.Lock()
	defer mu.Unlock()
	if !slices.Contains(messages, "telemetry enabled") {
		t.Errorf("the startup announcement was suppressed at LOG_LEVEL=warn; captured %v", messages)
	}
}

// TestStartTelemetry_WarnsWhenACredentialCrossesTheNetworkInTheClear covers
// the one configuration mistake this server refuses to keep quiet about: a
// collector header configured beside a plaintext endpoint on another host.
//
// The endpoint is a name under the reserved .invalid domain, so it is remote
// by any reading and resolves to nothing at once rather than hanging the
// shutdown flush on a connection attempt. The warning is read off the base
// handler because it is emitted after the bridge is installed, which is where
// it has to be for the collector to receive it too.
func TestStartTelemetry_WarnsWhenACredentialCrossesTheNetworkInTheClear(t *testing.T) {
	restore := telemetryFlag
	t.Cleanup(func() { telemetryFlag = restore })
	telemetryFlag = nil

	t.Setenv("GITLAB_MCP_TELEMETRY", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector.invalid:4318")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "authorization=Bearer%20secret")
	t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "200")
	t.Setenv("OTEL_BSP_EXPORT_TIMEOUT", "200")

	var mu sync.Mutex
	var messages []string
	capture := levelCapturingHandler{threshold: slog.LevelWarn, mu: &mu, messages: &messages}
	previous, previousBase := slog.Default(), baseLogHandler
	t.Cleanup(func() {
		slog.SetDefault(previous)
		baseLogHandler = previousBase
	})
	baseLogHandler = capture
	slog.SetDefault(slog.New(capture))

	provider, stop := startTelemetry(t.Context(), "2.7.6", config.ToolSurfaceDynamic)
	t.Cleanup(func() { stop(boundedShutdown(t)) })
	if !provider.Enabled() {
		t.Fatal("telemetry did not start, so there was no export to warn about")
	}

	mu.Lock()
	defer mu.Unlock()
	const warning = "a collector credential is configured against a plaintext endpoint on another host; it crosses the network in the clear on every export"
	if !slices.Contains(messages, warning) {
		t.Errorf("the plaintext-credential warning was not logged; captured %v", messages)
	}
}
