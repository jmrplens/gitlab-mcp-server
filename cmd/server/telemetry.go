package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/telemetry"
)

// telemetryFlag holds --telemetry. It is a pointer so that "not passed" is
// distinguishable from "passed as false", which is what lets the environment
// variable be consulted only when the operator said nothing on the command
// line. A plain bool would make an unset flag indistinguishable from an
// explicit --telemetry=false, and silently give the environment the last word
// over a person who typed the opposite.
var telemetryFlag *bool

// telemetryEnabled resolves the one switch this server owns.
//
// Precedence is the house rule, most specific first: an explicit --telemetry
// beats GITLAB_MCP_TELEMETRY, which beats off. Off is the default for privacy
// rather than for cost: instrumenting a deployment is a decision about the
// people using it, and a server that exported by default would be making that
// decision on the operator's behalf.
//
// OTEL_SDK_DISABLED is not consulted here. It is a veto applied inside
// telemetry.Start, because it must override an operator who asked for
// telemetry, and folding it in here would make it look like one input among
// several rather than the override it is.
func telemetryEnabled() bool {
	if telemetryFlag != nil && isFlagPassed("telemetry") {
		return *telemetryFlag
	}
	return telemetry.EnvSwitch()
}

// startTelemetry brings up the OpenTelemetry providers and returns the function
// that retires them.
//
// A failure to start is logged and swallowed. The specification permits either
// answer ("The API or SDK MAY fail fast and cause the application to fail on
// initialization... but MUST NOT cause the application to fail later at
// runtime"), so this is a decision rather than a rule, and the decision is that
// a server which can talk to GitLab must keep doing so when it cannot talk to a
// collector. The official Go examples model the opposite, with twelve calls to
// panic on that page alone, and copying them would mean a telemetry
// misconfiguration takes down a working server.
//
// Both return values are always usable. The provider is never nil, reports
// itself disabled when telemetry is off or failed to start, and answers
// Snapshot for the server card; the stop function is safe to call in every one
// of those states. That is what lets the caller defer it unconditionally rather
// than carrying a nil check into the exit path, which is the worst place to put
// one because it runs after the work succeeded.
func startTelemetry(ctx context.Context, serverVersion, toolSurface string) (provider *telemetry.Provider, stop func(context.Context)) {
	provider, err := telemetry.Start(ctx, telemetry.Config{
		Enabled:                 telemetryEnabled(),
		ServiceVersion:          serverVersion,
		Signals:                 telemetry.AllSignals(),
		DropToolNameFromMetrics: dropToolNameFromMetrics(toolSurface),
	})
	if err != nil {
		slog.ErrorContext(ctx, "telemetry disabled: it could not be started",
			"component", "telemetry", "error", err)
		return &telemetry.Provider{}, func(context.Context) {
			// Nothing to shut down: the provider never started, and the
			// caller still defers this unconditionally.
		}
	}
	restoreLogger := func() {
		// Replaced below when the bridge is installed. Until then it is what
		// the caller defers, so the nil case needs a body rather than a check.
	}
	if provider.Enabled() {
		// The logs signal is only real once something writes into it. Until
		// this line existed the provider was installed, "logs" was announced
		// at startup and on the server card, and no record was ever exported:
		// the card advertised a signal that produced nothing. Bridging here,
		// after Start has installed the global logger provider, is what makes
		// the announcement true.
		restoreLogger = installSlogBridge()

		// After the bridge, so the announcement reaches the collector. Before
		// it, these lines went to stderr alone, which is the one place an
		// operator running several replicas is not looking.
		announceIdentityChoice()

		snapshot := provider.Snapshot()
		// Per signal rather than one value for the process. The two scalars are
		// empty when the enabled signals disagree, which is the honest answer
		// for a summary and a useless one for an operator looking at their own
		// deployment: they want to know which collector each signal will
		// actually reach, and that is exactly the case where no summary exists.
		// Said once, at the moment it applies. The guide carries the same
		// warning in prose, which reaches an operator who read that section.
		if affected := telemetry.InsecureCredentialSignals(provider.Signals()); len(affected) > 0 {
			slog.WarnContext(ctx, "a collector credential is configured against a plaintext endpoint on another host; it crosses the network in the clear on every export",
				"component", "telemetry", "signals", affected)
		}

		slog.InfoContext(ctx, "telemetry enabled",
			"component", "telemetry",
			"protocol", snapshot.Protocol,
			"endpoint", snapshot.Endpoint,
			"protocols", snapshot.SignalProtocols,
			"endpoints", snapshot.SignalEndpoints,
			"signals", snapshot.Signals)
	}
	return provider, func(shutdownCtx context.Context) {
		if shutdownErr := provider.Shutdown(shutdownCtx); shutdownErr != nil {
			slog.WarnContext(ctx, "telemetry did not shut down cleanly",
				"component", "telemetry", "error", shutdownErr)
		}
		// After the provider, so the warning above still reaches a collector
		// that is listening. Restoring makes the bridge's lifetime match the
		// provider's, which matters beyond tidiness: the logger is a process
		// global, so without this a stopped telemetry stack keeps every later
		// log record routed at a logger provider that has been shut down.
		restoreLogger()
	}
}

// isFlagPassed reports whether a flag was named on the command line, as opposed
// to holding its default.
//
// flag.Visit walks only the flags that were actually set, which is the
// distinction the precedence rule in [telemetryEnabled] rests on.
func isFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// installSlogBridge sends this server's structured log records to the collector
// as well as to stderr.
//
// It replaces the default logger rather than wrapping at the call sites,
// because every package here logs through slog.Default and threading a second
// logger to all of them would be a change with no upside: the bridge needs to
// see the same records, with the same fields, that stderr already gets.
//
// Called only when telemetry started successfully. With telemetry off, the
// default handler stays exactly as it was, so the ordinary deployment pays
// nothing for a bridge it does not use.
// It returns the function that puts the previous logger back. The default
// logger is a process global, so a bridge installed and never removed outlives
// the provider it writes into: in production that is a stopped exporter
// receiving records during shutdown, and in a test binary it is one test's
// telemetry poisoning every later test's logging.
//
// # Why it wraps baseLogHandler rather than slog.Default().Handler()
//
// Because the obvious version recurses forever, and the stack trace is not
// obvious at all:
//
//	fanOutHandler.Handle -> slog.defaultHandler.Handle -> log.Logger.output
//	  -> slog.handlerWriter.Write -> fanOutHandler.Handle -> ...
//
// slog.SetDefault does two things, and the second is easy to miss: it also
// redirects the standard library's log package at the new default handler. So
// wrapping whatever slog.Default() currently holds is safe only while that is
// something we installed. When it is still slog's own built-in handler, which
// writes through log.Print, the wrapper's stderr leg calls log, log calls the
// default slog handler, and the default slog handler is now the wrapper.
//
// In this binary main installs a JSON handler before telemetry ever starts, so
// the cycle does not fire in production. Depending on that ordering is the
// fragile part: any caller reaching startTelemetry first, which is exactly what
// a test does, hangs the process with no error. Wrapping a handler we captured
// ourselves removes the dependency rather than documenting it.
func installSlogBridge() (restore func()) {
	if baseLogHandler == nil {
		// Nothing installed a base handler, so there is no known-safe thing to
		// wrap. Skipping costs the log export and keeps the process alive,
		// which is the right trade for a path only reached out of order.
		return func() {
			// No bridge was installed, so there is nothing to restore.
		}
	}

	previous := slog.Default()
	bridged := telemetry.NewSlogHandler(baseLogHandler, telemetry.DefaultLogSeverity, identityRedactor())
	if bridged == nil {
		return func() {
			// Same as above: no bridge, nothing to put back.
		}
	}
	slog.SetDefault(slog.New(bridged))
	return func() { slog.SetDefault(previous) }
}

// baseLogHandler is the stderr handler this server installs at startup.
//
// Captured at the one place that builds it, so the telemetry bridge wraps a
// handler whose behavior is known rather than whatever the global happens to
// hold. See installSlogBridge for what goes wrong otherwise.
var baseLogHandler slog.Handler

// telemetryToolNameFlag holds --telemetry-tool-name.
var telemetryToolNameFlag *string

// dropToolNameFromMetrics resolves whether the tool name is a metric
// dimension, for the surface the caller resolved from the inputs its mode
// really uses. Reading TOOL_SURFACE here regardless of mode is the defect this
// parameter replaced.
func dropToolNameFromMetrics(toolSurface string) bool {
	value := os.Getenv(telemetry.EnvToolNameName)
	if telemetryToolNameFlag != nil && isFlagPassed("telemetry-tool-name") {
		value = *telemetryToolNameFlag
	}

	policy, err := telemetry.ParseToolNamePolicy(value)
	if err != nil {
		slog.Error("telemetry tool-name policy is unusable; using auto",
			"component", "telemetry", "error", err)
		policy = telemetry.ToolNameAuto
	}

	return telemetry.DropToolName(policy, toolSurface)
}
