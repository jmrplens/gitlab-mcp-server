package main

import (
	"context"
	"flag"
	"log/slog"

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
func startTelemetry(ctx context.Context, serverVersion string) (provider *telemetry.Provider, stop func(context.Context)) {
	provider, err := telemetry.Start(ctx, telemetry.Config{
		Enabled:        telemetryEnabled(),
		ServiceVersion: serverVersion,
		Signals:        telemetry.AllSignals(),
	})
	if err != nil {
		slog.Error("telemetry disabled: it could not be started",
			"component", "telemetry", "error", err)
		return &telemetry.Provider{}, func(context.Context) {}
	}
	if provider.Enabled() {
		snapshot := provider.Snapshot()
		slog.Info("telemetry enabled",
			"component", "telemetry",
			"protocol", snapshot.Protocol,
			"endpoint", snapshot.Endpoint,
			"signals", snapshot.Signals)
	}
	return provider, func(shutdownCtx context.Context) {
		if shutdownErr := provider.Shutdown(shutdownCtx); shutdownErr != nil {
			slog.Warn("telemetry did not shut down cleanly",
				"component", "telemetry", "error", shutdownErr)
		}
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
