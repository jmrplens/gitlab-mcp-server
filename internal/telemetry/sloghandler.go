package telemetry

import (
	"context"
	"log/slog"
	"strings"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/log/global"
)

// scopeName names this bridge as the instrumentation scope on every exported
// log record, matching what the span and metric instrumentation declares.
const scopeName = "github.com/jmrplens/gitlab-mcp-server/v2/internal/telemetry"

// DefaultLogSeverity is the floor for records that reach a collector.
//
// Info rather than debug, and the reason is bounded resource use rather than
// taste: "Logging could consume much memory by default if the end user
// application emits too many logs... the end user should consider reducing logs
// that are passed to the exporters." A debug run of this server emits a record
// per GitLab round trip, and exporting all of them would duplicate on the wire
// what the spans already describe, on top of the spans.
const DefaultLogSeverity = slog.LevelInfo

// NewSlogHandler wraps an existing handler so records go to both stderr and the
// collector.
//
// # Why both, rather than one or the other
//
// The stderr JSON is what an operator reads over somebody's shoulder, what a
// container platform captures, and what works when telemetry is off, which is
// the default. It cannot be replaced. The OTLP leg is what correlates a log
// record with the span it happened inside, which stderr cannot do at all.
//
// So this is a fan-out rather than a redirect, and the stderr leg is
// deliberately first: if the bridge ever blocks or panics, the record has
// already been written where somebody can see it.
//
// # The severity floor
//
// The collector leg is filtered and the stderr leg is not. An operator running
// at debug wants everything on their terminal and almost certainly does not
// want a record per GitLab round trip on their collector, on top of a span
// describing the same call. LOG_LEVEL still governs stderr; this floor governs
// only what is exported.
//
// # What this does not do
//
// It adds no attributes of its own. The identity policy, the redaction rules
// and the decision about what a record may carry all live where the record is
// written, so a field that must not be exported must not be logged either. A
// bridge that filtered fields would be a second place to get that wrong, and
// the two would disagree the first time somebody added a log line.
func NewSlogHandler(base slog.Handler, floor slog.Level) slog.Handler {
	if base == nil {
		return nil
	}
	return &fanOutHandler{
		stderr:   base,
		otlp:     otelslog.NewHandler(scopeName, otelslog.WithLoggerProvider(global.GetLoggerProvider())),
		otlpMin:  floor,
		exported: true,
	}
}

// fanOutHandler writes each record to stderr and, above a floor, to OTLP.
type fanOutHandler struct {
	stderr   slog.Handler
	otlp     slog.Handler
	otlpMin  slog.Level
	exported bool
}

// Enabled asks the stderr handler alone.
//
// The collector leg is strictly narrower, so a record stderr rejects can never
// be one OTLP wants. Consulting both would let the OTLP leg widen what the
// operator's LOG_LEVEL asked for, which is the opposite of what an export floor
// is for.
func (h *fanOutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.stderr.Enabled(ctx, level)
}

// Handle writes to stderr first, then to the collector.
//
// The stderr error is the one returned. An OTLP failure is deliberately
// swallowed: a collector that is down must not turn every log call in this
// server into an error path, and the SDK reports its own export failures
// through the handler installed in diagnostics.go, which is where an operator
// should learn about it.
func (h *fanOutHandler) Handle(ctx context.Context, record slog.Record) error {
	err := h.stderr.Handle(ctx, record)

	if h.exported && record.Level >= h.otlpMin {
		_ = h.otlp.Handle(ctx, redactRecord(record))
	}
	return err
}

// redactRecord returns the record as it may leave the process.
//
// Only resource URIs, and only on the exported copy. The rule is the one the
// span attributes already follow: which resource a request named is governed by
// the identity policy, and a value that arrives by another route is a second
// carrier the policy does not reach.
//
// It has to happen here rather than at the call sites because the records are
// not all ours. The Go SDK logs "resource subscribed" with the URI through the
// logger this server installs, so a rule applied only where this repository
// calls slog would leave the dependency's records untouched, which is how this
// was found: the span carried a digest and the log beside it carried the path.
//
// The common case allocates nothing. A record with no URI in it is returned as
// it came, which is every record this server writes except a handful about
// subscriptions.
func redactRecord(record slog.Record) slog.Record {
	if !recordNeedsRedaction(record) {
		return record
	}

	redacted := slog.NewRecord(record.Time, record.Level,
		RedactResourceURIs(record.Message), record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		redacted.AddAttrs(redactAttr(attr))
		return true
	})
	return redacted
}

// recordNeedsRedaction reports whether anything in the record mentions a
// resource URI, so the copy above is made only when it changes something.
func recordNeedsRedaction(record slog.Record) bool {
	if strings.Contains(record.Message, resourceURIScheme) {
		return true
	}
	found := false
	record.Attrs(func(attr slog.Attr) bool {
		if strings.Contains(attr.Value.String(), resourceURIScheme) {
			found = true
			return false
		}
		return true
	})
	return found
}

// redactAttr rewrites one attribute, descending into groups.
func redactAttr(attr slog.Attr) slog.Attr {
	if attr.Value.Kind() == slog.KindGroup {
		group := attr.Value.Group()
		rewritten := make([]slog.Attr, 0, len(group))
		for _, inner := range group {
			rewritten = append(rewritten, redactAttr(inner))
		}
		return slog.Attr{Key: attr.Key, Value: slog.GroupValue(rewritten...)}
	}

	text := attr.Value.String()
	if !strings.Contains(text, resourceURIScheme) {
		return attr
	}
	return slog.String(attr.Key, RedactResourceURIs(text))
}

// WithAttrs applies to both legs, so a logger derived from this one keeps
// exporting. Forgetting either would produce a handler that works until
// somebody calls slog.With, which is the ordinary way to build a component
// logger.
func (h *fanOutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &fanOutHandler{
		stderr:   h.stderr.WithAttrs(attrs),
		otlp:     h.otlp.WithAttrs(attrs),
		otlpMin:  h.otlpMin,
		exported: h.exported,
	}
}

// WithGroup applies to both legs, for the same reason as WithAttrs.
func (h *fanOutHandler) WithGroup(name string) slog.Handler {
	return &fanOutHandler{
		stderr:   h.stderr.WithGroup(name),
		otlp:     h.otlp.WithGroup(name),
		otlpMin:  h.otlpMin,
		exported: h.exported,
	}
}
