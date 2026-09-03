package telemetry

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"strings"
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
	// Both sinks, because the credential travels through both. See
	// [newCredentialRedactor] for what the SDK prints and why matching on its
	// message text would miss the worst of it.
	redact := newCredentialRedactor()
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Error("opentelemetry sdk error", "component", "telemetry", "error", redact(err.Error()))
	}))
	otel.SetLogger(logr.FromSlogHandler(&sdkVerbosityHandler{
		Handler: &redactingHandler{Handler: logger.Handler(), redact: redact},
	}))
}

// redactedPlaceholder replaces a collector credential wherever it is found.
const redactedPlaceholder = "[redacted]"

// minRedactedSecret is the shortest string worth substituting.
//
// A replacement list built from an operator's configuration is a blunt
// instrument: every occurrence of the string goes, everywhere, in every record
// this sink carries. Two or three characters would corrupt unrelated
// diagnostics for no benefit, since a header value that short is not a secret
// anyone is protecting. The whole-variable entry is what covers the case that
// matters, and it is always long.
const minRedactedSecret = 8

// newCredentialRedactor builds the substitution applied to everything the SDK
// says about itself.
//
// # Why this exists
//
// The exporters read OTEL_EXPORTER_OTLP*_HEADERS themselves and this server
// never touches the value, which is the guarantee docs/guides/telemetry.md
// makes. The SDK does not keep it: its header parser logs the raw pair when one
// has no "=", logs the raw value when percent-decoding fails, and the log
// exporter goes further and hands otel.Handle the *entire* variable
// ("invalid %s value %s"). So a typo in one non-credential pair prints every
// credential beside it, with the credential itself perfectly well formed.
//
// # Why by value and not by message
//
// Matching on the SDK's own message strings is brittle across versions and,
// more to the point, misses the whole-variable line entirely: it travels
// through the error handler under this server's own message, "opentelemetry sdk
// error". What is stable is the secret itself, which this process can read from
// the same variables the exporters do.
//
// The variable *name* is deliberately left in place. An operator whose
// configuration is malformed needs to know which variable to fix, and after
// this the line names it without quoting it.
func newCredentialRedactor() func(string) string {
	var pairs []string
	seen := map[string]bool{}
	add := func(secret string) {
		secret = strings.TrimSpace(secret)
		if len(secret) < minRedactedSecret || seen[secret] {
			return
		}
		seen[secret] = true
		pairs = append(pairs, secret, redactedPlaceholder)
	}
	// Longest first: strings.Replacer takes the first pattern that matches at a
	// position, so the whole variable has to be offered before its parts or a
	// component would be replaced inside it and leave the rest exposed.
	for _, key := range credentialEnvKeys() {
		value := os.Getenv(key)
		if strings.TrimSpace(value) == "" {
			continue
		}
		add(value)
		addUnescaped(add, value)
		for component := range strings.SplitSeq(value, ",") {
			add(component)
			addUnescaped(add, component)
			if _, headerValue, found := strings.Cut(component, "="); found {
				add(headerValue)
				addUnescaped(add, headerValue)
			}
		}
	}

	if len(pairs) == 0 {
		return func(text string) string { return text }
	}
	return strings.NewReplacer(pairs...).Replace
}

// addUnescaped offers the percent-decoded spelling too.
//
// The variables use W3C Baggage syntax, so a credential containing a space is
// written percent-encoded and the exporter logs whichever form it was holding
// when it gave up. Both spellings are the same secret.
func addUnescaped(add func(string), value string) {
	decoded, err := url.QueryUnescape(value)
	if err != nil || decoded == value {
		return
	}
	add(decoded)
}

// credentialEnvKeys lists the variables carrying a collector credential, in a
// fixed order, derived from the same table the plaintext check uses so the two
// cannot drift apart.
func credentialEnvKeys() []string {
	seen := map[string]bool{}
	keys := make([]string, 0, len(headerKeys)+1)
	for _, signal := range []string{"traces", "metrics", "logs"} {
		for _, key := range headerKeys[signal] {
			if !seen[key] {
				seen[key] = true
				keys = append(keys, key)
			}
		}
	}
	return keys
}

// redactingHandler runs every record through a substitution before the handler
// underneath it writes anything.
//
// It wraps the handler rather than the logger because the SDK's logr stream is
// handed a handler, not a logger, and a rule applied at one call site is a rule
// the next SDK version routes around.
type redactingHandler struct {
	slog.Handler
	redact func(string) string
}

func (h *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, h.redact(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		clean.AddAttrs(h.redactAttr(attr))
		return true
	})
	return h.Handler.Handle(ctx, clean)
}

// redactAttr rewrites one attribute, descending into groups and rendering an
// error value, which is how logr delivers what the exporter failed on.
func (h *redactingHandler) redactAttr(attr slog.Attr) slog.Attr {
	if attr.Value.Kind() == slog.KindGroup {
		group := attr.Value.Group()
		rewritten := make([]slog.Attr, 0, len(group))
		for _, inner := range group {
			rewritten = append(rewritten, h.redactAttr(inner))
		}
		return slog.Attr{Key: attr.Key, Value: slog.GroupValue(rewritten...)}
	}

	text := attr.Value.String()
	if err, ok := attr.Value.Any().(error); ok {
		text = err.Error()
	}
	redacted := h.redact(text)
	if redacted == text && attr.Value.Kind() != slog.KindAny {
		return attr
	}
	return slog.String(attr.Key, redacted)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	rewritten := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		rewritten = append(rewritten, h.redactAttr(attr))
	}
	return &redactingHandler{Handler: h.Handler.WithAttrs(rewritten), redact: h.redact}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{Handler: h.Handler.WithGroup(name), redact: h.redact}
}

// diagnosticsOnce guards the single permitted call, for the delegation reason
// in [installDiagnostics].
var diagnosticsOnce sync.Once

// sdkVerbosityHandler maps the SDK's logr verbosities to the slog levels its
// own documentation names.
//
// otel/internal/global spells the map out: "To see Warn messages use a logger
// with l.V(1).Enabled() == true", Info is V(4) and Debug is V(8).
// logr.FromSlogHandler turns V(n) into slog level -n, so without this the
// SDK's warn channel landed at -1, below Info, and the default handler dropped
// every warning the SDK ever raised; its debug channel landed at -8, below
// Debug, and was invisible even to LOG_LEVEL=debug. Each channel now arrives
// at the level its name promises: Warn at Warn, Info and Debug at Debug, which
// is where a stream the SDK itself calls chatty belongs.
type sdkVerbosityHandler struct {
	slog.Handler
}

func (h *sdkVerbosityHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Handler.Enabled(ctx, clampSDKLevel(level))
}

func (h *sdkVerbosityHandler) Handle(ctx context.Context, record slog.Record) error {
	record.Level = clampSDKLevel(record.Level)
	return h.Handler.Handle(ctx, record)
}

func (h *sdkVerbosityHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &sdkVerbosityHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *sdkVerbosityHandler) WithGroup(name string) slog.Handler {
	return &sdkVerbosityHandler{Handler: h.Handler.WithGroup(name)}
}

// clampSDKLevel maps a logr-derived level to the slog level the SDK's channel
// naming promises. Levels at Info and above pass through: they are not the
// SDK's verbosity scheme.
func clampSDKLevel(level slog.Level) slog.Level {
	switch {
	case level >= slog.LevelInfo:
		return level
	case level > slog.LevelDebug:
		// V(1) through V(3): the SDK's warn channel.
		return slog.LevelWarn
	default:
		// V(4) and beyond: the SDK's info and debug channels, both of which
		// its docs describe as internal detail.
		return slog.LevelDebug
	}
}
