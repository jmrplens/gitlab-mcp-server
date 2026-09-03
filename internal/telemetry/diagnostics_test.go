package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// TestSetDiagnosticSinks_ErrorHandlerReachesTheLogger covers the first of the
// SDK's two self-report channels: everything the SDK passes to otel.Handle,
// which is where an export failure, a partial-success response and a sampler
// configuration error arrive.
//
// Without this wiring those still reach stderr, through the SDK's own default
// handler, but as standard-library log lines interleaved with this server's
// JSON. The assertion is therefore about the shape as much as the delivery: the
// record must parse as JSON and carry the error.
func TestSetDiagnosticSinks_ErrorHandlerReachesTheLogger(t *testing.T) {
	var buf bytes.Buffer
	setDiagnosticSinks(slog.New(slog.NewJSONHandler(&buf, nil)))

	otel.Handle(errors.New("collector refused the batch"))

	line := buf.String()
	if line == "" {
		t.Fatal("otel.Handle produced no record; the error handler is not wired")
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &record); err != nil {
		t.Fatalf("record is not JSON (%v): %q", err, line)
	}
	if got, _ := record["error"].(string); got != "collector refused the batch" {
		t.Errorf("error field = %q, want the handled error", got)
	}
	if got, _ := record["level"].(string); got != "ERROR" {
		t.Errorf("level = %q, want ERROR", got)
	}
}

// TestSetDiagnosticSinks_LogrStreamReachesTheLogger covers the second channel,
// which is the one that is actually invisible by default.
//
// The SDK's internal logr stream carries a duration that failed to parse, a
// malformed OTEL_EXPORTER_OTLP_HEADERS pair, an endpoint that is not a URL, and
// the protocol and compression warnings. It is reachable only from inside the
// SDK (go.opentelemetry.io/otel/internal/global is not importable), so this
// provokes a real one rather than calling the sink directly.
//
// The provocation is itself worth pinning. OTEL_EXPORTER_OTLP_TIMEOUT is
// specified as an integer number of milliseconds, not a Go duration: "Any value
// that represents a timeout MUST be an integer representing a number of
// milliseconds." So "30s" does not mean thirty seconds, it means nothing at
// all, and the exporter silently keeps its 10s default. Every duration variable
// in the OTEL_ namespace behaves this way while every flag this server defines
// takes a Go duration, which is exactly the confusion an operator will bring.
// Without this wiring the failure is a stdlib log line rather than a record.
func TestSetDiagnosticSinks_LogrStreamReachesTheLogger(t *testing.T) {
	var buf bytes.Buffer
	// Info, deliberately, because it is the default LOG_LEVEL. The SDK emits
	// this warning at V(1), which logr maps below Info, so without the
	// verbosity clamp the wiring delivered it to a handler that dropped every
	// one and this test only passed by running at debug.
	setDiagnosticSinks(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "30s")
	if _, err := newTraceExporter(context.Background(), ProtocolHTTP); err != nil {
		t.Fatalf("building the exporter: %v", err)
	}

	if !strings.Contains(buf.String(), "parse duration") {
		t.Errorf("the SDK's logr stream did not reach the logger; got %q", buf.String())
	}
}

// TestSetDiagnosticSinks_NeverWritesToStdout is the assertion that matters most
// on the stdio transport, where stdout carries JSON-RPC and one stray line ends
// the session.
//
// The trap is concrete rather than hypothetical: the rendered example for
// otel.SetLogger on pkg.go.dev builds its logger over os.Stdout. Anyone copying
// it gets a server that works until the first exporter hiccup. This test does
// not try to capture the process's real stdout, which no unit test can do
// reliably; it asserts the weaker but sufficient property that both sinks write
// to the handler they were given and nowhere else, by giving them a handler and
// checking everything landed there.
func TestSetDiagnosticSinks_NeverWritesToStdout(t *testing.T) {
	var buf bytes.Buffer
	setDiagnosticSinks(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "also not a number of milliseconds")
	otel.Handle(errors.New("handled error"))
	if _, err := newTraceExporter(context.Background(), ProtocolHTTP); err != nil {
		t.Fatalf("building the exporter: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"handled error", "parse duration"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(out, want) {
				t.Errorf("%q did not reach the provided handler; a sink is writing somewhere else", want)
			}
		})
	}
}

// TestInstallDiagnostics_IsIdempotent pins the guard rather than the wiring.
//
// otel.GetErrorHandler returns a delegating handler, so the first
// SetErrorHandler call retroactively redirects every handler previously handed
// out, including the ones the providers already hold. A second call sets the
// global but does not delegate to it, leaving those earlier holders pointing at
// the first handler. Once is the only correct number, and a second Start in one
// process (a test, a reload) must therefore be harmless rather than confusing.
func TestInstallDiagnostics_IsIdempotent(t *testing.T) {
	var first, second bytes.Buffer
	installDiagnostics(slog.New(slog.NewJSONHandler(&first, nil)))
	installDiagnostics(slog.New(slog.NewJSONHandler(&second, nil)))

	otel.Handle(errors.New("only the first logger should see this"))

	if second.Len() != 0 {
		t.Errorf("the second call replaced the sinks; got %q", second.String())
	}
}

// TestSetDiagnosticSinks_TheSDKWarnChannelSurvivesTheDefaultLevel covers the
// channel the default configuration silently discarded.
//
// The SDK documents its own map: "To see Warn messages use a logger with
// l.V(1).Enabled() == true". Through logr that is slog level -1, which the
// default Info handler refuses, so every warning the SDK raised vanished at
// LOG_LEVEL=info while errors and nothing else got through. The provocation is
// real: an empty meter name makes the pinned SDK call global.Warn.
func TestSetDiagnosticSinks_TheSDKWarnChannelSurvivesTheDefaultLevel(t *testing.T) {
	var buf bytes.Buffer
	setDiagnosticSinks(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	provider := sdkmetric.NewMeterProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	_ = provider.Meter("")

	if !strings.Contains(buf.String(), "Invalid Meter name") {
		t.Errorf("the SDK's warn channel did not reach an Info-level handler; got %q", buf.String())
	}
	if !strings.Contains(buf.String(), `"level":"WARN"`) {
		t.Errorf("the SDK warning arrived at the wrong level: %q", buf.String())
	}
}
