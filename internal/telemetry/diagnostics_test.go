package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

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

// TestSDKVerbosityHandler_ADerivedHandlerKeepsTheClamp covers the two methods
// slog calls when a caller attaches context to a logger.
//
// Both must return a handler that still clamps. The SDK's own logger is derived
// this way — it attaches a name and its component fields before logging
// anything — so a WithAttrs that returned the bare inner handler would put the
// clamp back exactly where it was before this type existed: warnings arriving
// at a level nothing prints, and a debug channel at -8 that even LOG_LEVEL=debug
// cannot see.
func TestSDKVerbosityHandler_ADerivedHandlerKeepsTheClamp(t *testing.T) {
	t.Parallel()

	// The SDK's warn channel: below Info, above Debug, and therefore invisible
	// to a handler at the default level unless it is clamped up to Warn.
	const sdkWarn = slog.LevelInfo - 1

	tests := []struct {
		name    string
		derive  func(slog.Handler) slog.Handler
		enabled bool
	}{
		{
			name:   "with attributes",
			derive: func(h slog.Handler) slog.Handler { return h.WithAttrs([]slog.Attr{slog.String("component", "sdk")}) },
		},
		{
			name:   "with a group",
			derive: func(h slog.Handler) slog.Handler { return h.WithGroup("sdk") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			base := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
			derived := tt.derive(&sdkVerbosityHandler{Handler: base})

			if !derived.Enabled(context.Background(), sdkWarn) {
				t.Fatal("the derived handler refuses the SDK's warn channel; every SDK warning would be dropped")
			}
			record := slog.NewRecord(time.Now(), sdkWarn, "the SDK said something", 0)
			if err := derived.Handle(context.Background(), record); err != nil {
				t.Fatalf("Handle: %v", err)
			}

			line := buf.String()
			if !strings.Contains(line, "level=WARN") {
				t.Errorf("record was written as %q, want it clamped up to WARN", strings.TrimSpace(line))
			}
			if !strings.Contains(line, "the SDK said something") {
				t.Errorf("record %q lost its message", strings.TrimSpace(line))
			}
		})
	}
}

// TestSetDiagnosticSinks_AMalformedHeaderVariableIsNotPrinted is the regression
// for a guarantee the guide states and a test could not fail on.
//
// docs/guides/telemetry.md says "This server never reads, logs or transforms
// that variable ... a test asserts the credential never appears in this
// server's own log output, including when an export fails". The server does not
// print it; the SDK does, through the sinks this file installs. Its header
// parser logs the raw pair when one has no "=", logs the raw value when
// percent-decoding fails, and the log exporter hands otel.Handle the entire
// variable, so a typo in a non-credential pair prints a perfectly well-formed
// credential sitting beside it.
//
// Three shapes, because each reaches a different branch, and the third is the
// one the original finding missed: the credential itself is valid and only a
// neighboring key is not. Both exporters are built for every case, because the
// worst path is the log exporter's and the trace exporter's is the one the SDK
// logs at V(1).
func TestSetDiagnosticSinks_AMalformedHeaderVariableIsNotPrinted(t *testing.T) {
	const secret = "SUPERSECRET-COLLECTOR-TOKEN-9f3a"

	tests := []struct {
		name  string
		value string
	}{
		{
			name:  "a pair with no equals sign",
			value: "Authorization: Bearer " + secret,
		},
		{
			name:  "a value that does not percent-decode",
			value: "authorization=Bearer%2" + secret,
		},
		{
			// The sharp case: this credential is well formed and would never
			// be logged on its own. The invalid key beside it is what makes
			// the exporter print the whole variable.
			name:  "a valid credential beside an invalid key",
			value: "api-key=" + secret + ",x tenant=acme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", tt.value)
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
			t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "200")

			var buf bytes.Buffer
			setDiagnosticSinks(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

			if _, err := newTraceExporter(context.Background(), ProtocolHTTP); err != nil {
				t.Fatalf("building the trace exporter: %v", err)
			}
			logExporter, err := newLogExporter(context.Background(), ProtocolHTTP)
			if err != nil {
				t.Fatalf("building the log exporter: %v", err)
			}
			t.Cleanup(func() { _ = logExporter.Shutdown(context.Background()) })

			printed := buf.String()
			if strings.Contains(printed, secret) {
				t.Errorf("the collector credential was printed by the SDK's own diagnostics: %s", printed)
			}
			// The absence above proves nothing on its own: an SDK that stopped
			// logging these shapes, a level mapping that dropped them, or a
			// sink that was never installed all leave it green. The variable
			// name is kept in the line on purpose, so the pair below is the
			// positive signal that the branch ran and the substitution with it.
			if !strings.Contains(printed, "OTEL_EXPORTER_OTLP_HEADERS") {
				t.Errorf("nothing named the malformed variable, so this row proves no redaction: %s", printed)
			}
			if !strings.Contains(printed, redactedPlaceholder) {
				t.Errorf("nothing was substituted, so the credential was never in the line to begin with: %s", printed)
			}
		})
	}
}

// TestNewCredentialRedactor_SubstitutesOnlyWhatIsWorthSubstituting covers the
// floor on what counts as a secret, on both sides of it.
//
// The floor exists because a short value is not a credential and substituting
// one would rewrite unrelated text wherever those characters happened to
// appear: an operator whose log lines are peppered with "[redacted]" learns
// nothing from the one that mattered. It is a floor rather than a ceiling, so a
// value of exactly that length is a secret and is substituted; the row below it
// is what proves the comparison is not simply being skipped.
func TestNewCredentialRedactor_SubstitutesOnlyWhatIsWorthSubstituting(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		want   bool
	}{
		{
			name:   "one character short of the floor is left alone",
			secret: strings.Repeat("s", minRedactedSecret-1),
			want:   false,
		},
		{
			name:   "exactly the floor is substituted",
			secret: strings.Repeat("k", minRedactedSecret),
			want:   true,
		},
		{
			name:   "a realistic credential is substituted",
			secret: "glpat-0123456789abcdefghij",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", tt.secret)
			// The other two are cleared so a developer's own environment
			// cannot contribute a pair and answer the question for us.
			t.Setenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", "")
			t.Setenv("OTEL_EXPORTER_OTLP_METRICS_HEADERS", "")
			t.Setenv("OTEL_EXPORTER_OTLP_LOGS_HEADERS", "")

			line := "the collector refused " + tt.secret + " and closed the stream"
			got := newCredentialRedactor()(line)

			if tt.want && strings.Contains(got, tt.secret) {
				t.Errorf("the value was printed verbatim: %s", got)
			}
			if !tt.want && got != line {
				t.Errorf("a value below the %d-character floor was rewritten: %q became %q",
					minRedactedSecret, line, got)
			}
		})
	}
}

// TestNewCredentialRedactor_ThePercentDecodedSpellingIsSubstitutedToo covers
// the second spelling of one secret.
//
// The variables use W3C Baggage syntax, so a credential containing a space is
// written percent-encoded, and the exporter logs whichever form it was holding
// when it gave up. Offering only the spelling the environment carries leaves
// the decoded one intact in the line, which is the whole secret in plain text
// under a different set of bytes.
func TestNewCredentialRedactor_ThePercentDecodedSpellingIsSubstitutedToo(t *testing.T) {
	const secret = "SUPERSECRET-COLLECTOR-TOKEN-9f3a"

	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "authorization=Bearer%20"+secret)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", "")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_HEADERS", "")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_HEADERS", "")

	redact := newCredentialRedactor()

	// The spelling the variable carries: the control, since a redactor that
	// offered nothing at all would pass the assertion below by doing nothing.
	escaped := "collector rejected header Bearer%20" + secret
	if got := redact(escaped); strings.Contains(got, secret) {
		t.Errorf("the escaped spelling was printed verbatim: %s", got)
	}

	decoded := "collector rejected header Bearer " + secret
	if got := redact(decoded); strings.Contains(got, secret) {
		t.Errorf("the percent-decoded spelling was printed verbatim: %s", got)
	}
}

// TestRedactingHandler_AnUnchangedValueKeepsItsType covers the branch that
// decides whether an attribute is handed back or rebuilt as a string.
//
// Rebuilding one that did not change costs its type on the wire: an integer
// becomes the text of an integer, and a backend that could have aggregated it
// can no longer. The exception is a value the handler had to render to inspect
// at all, which is how logr delivers an error, and there the rendered text is
// what must be written rather than the opaque value behind it.
func TestRedactingHandler_AnUnchangedValueKeepsItsType(t *testing.T) {
	var buf bytes.Buffer
	handler := &redactingHandler{
		Handler: slog.NewJSONHandler(&buf, nil),
		redact:  func(text string) string { return strings.ReplaceAll(text, "hunter2", redactedPlaceholder) },
	}

	slog.New(handler).Info("export failed",
		slog.Int("attempts", 7),
		slog.Any("err", errors.New("collector refused the batch")),
	)

	printed := buf.String()

	if !strings.Contains(printed, `"attempts":7`) {
		t.Errorf(`the integer attribute did not survive as a number: %s`, printed)
	}
	// An error is delivered as an opaque value, so the handler renders it to
	// inspect it and must write what it rendered. Left alone, encoding/json
	// marshals the error struct itself and the message is lost entirely.
	if !strings.Contains(printed, `"err":"collector refused the batch"`) {
		t.Errorf("the error attribute was not written as its rendered text: %s", printed)
	}
}

// TestRedactingHandler_DescendsIntoAGroup covers the recursion that keeps a
// grouped attribute from being read as one opaque value.
//
// logr delivers the exporter's own context as attributes, and the SDK is free
// to group them. A flat scan would see the group's rendered form, substitute
// nothing inside it, and hand the collector credential straight to the terminal
// under a key that looks structured.
func TestRedactingHandler_DescendsIntoAGroup(t *testing.T) {
	const secret = "SUPERSECRET-COLLECTOR-TOKEN-9f3a"

	var buf bytes.Buffer
	handler := &redactingHandler{
		Handler: slog.NewJSONHandler(&buf, nil),
		redact:  func(text string) string { return strings.ReplaceAll(text, secret, redactedPlaceholder) },
	}

	slog.New(handler).Info("export failed",
		slog.Group("request",
			slog.String("authorization", "Bearer "+secret),
			slog.Group("retry", slog.String("last", "Bearer "+secret)),
		),
	)

	printed := buf.String()
	if strings.Contains(printed, secret) {
		t.Errorf("a credential inside a group reached the terminal: %s", printed)
	}
	if !strings.Contains(printed, redactedPlaceholder) {
		t.Errorf("nothing was substituted, so the group was never descended: %s", printed)
	}
}

// TestClampSDKLevel_MapsTheSDKVerbosityScaleOntoLevels pins the mapping at
// every boundary it has.
//
// logr's verbosity counts downward from Info while slog's levels count upward,
// so the SDK's V(1) arrives as a level just below Info and its V(4) far below
// Debug. Anything at Info or above is not on that scale at all and passes
// through unchanged: clamping an Error to Warn would hide an export failure
// behind a level an operator filters out.
func TestClampSDKLevel_MapsTheSDKVerbosityScaleOntoLevels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   slog.Level
		want slog.Level
	}{
		{name: "error passes through", in: slog.LevelError, want: slog.LevelError},
		{name: "warn passes through", in: slog.LevelWarn, want: slog.LevelWarn},
		{name: "info is the lowest level that passes through", in: slog.LevelInfo, want: slog.LevelInfo},
		{name: "one below info is the SDK warn channel", in: slog.LevelInfo - 1, want: slog.LevelWarn},
		{name: "one above debug is still the SDK warn channel", in: slog.LevelDebug + 1, want: slog.LevelWarn},
		{name: "debug itself is the SDK info channel", in: slog.LevelDebug, want: slog.LevelDebug},
		{name: "below debug is internal detail", in: slog.LevelDebug - 4, want: slog.LevelDebug},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := clampSDKLevel(tt.in); got != tt.want {
				t.Errorf("clampSDKLevel(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
