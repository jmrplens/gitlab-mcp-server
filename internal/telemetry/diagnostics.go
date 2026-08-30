package telemetry

import (
	"log/slog"
	"sync"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
)

// installDiagnostics routes what the OpenTelemetry SDK says about itself into
// this server's own logger.
//
// # Two sinks, not one
//
// The SDK reports its own troubles through two independent channels, and wiring
// either one alone leaves the other unstructured:
//
//   - [otel.SetErrorHandler] receives everything passed to otel.Handle, which is
//     where export failures, partial-success responses and sampler
//     configuration errors arrive.
//   - [otel.SetLogger] receives the internal logr stream, which is where a
//     duration that failed to parse, a malformed OTEL_EXPORTER_OTLP_HEADERS
//     pair, an endpoint that is not a URL, and the protocol and compression
//     warnings arrive.
//
// # What this actually buys
//
// Not visibility from nothing: the SDK's defaults already write to stderr
// (stdr over os.Stderr at Error level, and the default error handler likewise),
// so a misconfiguration is not silent today. What changes is that the output
// becomes structured slog JSON on the same stream as every other record instead
// of interleaved standard-library log lines, and that the Warn tier starts
// arriving at all. global.Warn is V(1).Info, which stdr drops at its default
// verbosity, and it carries exactly the messages an operator most needs:
// "grpc is not a valid protocol for OTLP/HTTP, defaulting to http/protobuf"
// and both metric enum warnings.
//
// # Why stderr is stated rather than assumed, and why the error handler is not
// optional
//
// The rendered example for otel.SetLogger on pkg.go.dev builds its logger over
// os.Stdout. Copying that verbatim corrupts every stdio session, because stdout
// is the JSON-RPC channel and a single stray log line ends the conversation.
// The logger passed here inherits this server's handler, which writes to
// stderr; nothing in this package may construct a sink over stdout.
//
// The error handler carries a sharper version of the same risk, and it is the
// reason installing one is a requirement rather than an improvement. The SDK's
// default handler ends in log.Print, which is the standard library's shared
// logger. That logger's destination is process-global: any dependency, anywhere,
// that calls log.SetOutput(os.Stdout) silently retargets every OpenTelemetry
// internal error into the JSON-RPC stream, from code that has never heard of
// this server. Measured on a real binary: with the default handler, an internal
// error went to stderr; after one log.SetOutput(os.Stdout) call elsewhere in the
// same process, the identical error appeared on stdout. Installing our own
// handler severs that link, because a handler that writes to a slog.Logger
// cannot be redirected by a package it does not know about.
//
// The corollary is a rule for the rest of the tree: nothing in this process may
// call log.SetOutput(os.Stdout).
//
// The SDK's other diagnostic channel is safe by construction and needs no such
// argument: otel/internal/global builds it as stdr.New(log.New(os.Stderr, ...)),
// a fresh logger pinned to stderr rather than the shared one.
//
// # Called at most once
//
// otel.GetErrorHandler returns a delegating handler, so the first
// SetErrorHandler call retroactively redirects every handler previously handed
// out, including ones the providers already hold. Later calls set the global
// but do not delegate to it, which would leave earlier holders pointing at the
// first handler. Once is therefore the only correct number, and the sync.Once
// here makes a second Start in the same process (a test, a reload) harmless
// rather than confusing.
func installDiagnostics(logger *slog.Logger) {
	diagnosticsOnce.Do(func() { setDiagnosticSinks(logger) })
}

// setDiagnosticSinks does the work, without the guard.
//
// It is separate so a test can drive it more than once with a logger it can
// read. Production code calls [installDiagnostics] instead: calling this twice
// in a running server is the mistake the guard exists to prevent, not a
// supported operation.
func setDiagnosticSinks(logger *slog.Logger) {
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Error("opentelemetry sdk error", "component", "telemetry", "error", err)
	}))
	otel.SetLogger(logr.FromSlogHandler(logger.Handler()))
}

// diagnosticsOnce guards the single permitted call, for the delegation reason
// in [installDiagnostics].
var diagnosticsOnce sync.Once
