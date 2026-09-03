package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

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
func NewSlogHandler(base slog.Handler, floor slog.Level, identity *Redactor) slog.Handler {
	if base == nil {
		return nil
	}
	return &fanOutHandler{
		stderr:   base,
		otlp:     otelslog.NewHandler(scopeName, otelslog.WithLoggerProvider(global.GetLoggerProvider())),
		otlpMin:  floor,
		exported: true,
		identity: identity,
	}
}

// fanOutHandler writes each record to stderr and, above a floor, to OTLP.
type fanOutHandler struct {
	stderr   slog.Handler
	otlp     slog.Handler
	otlpMin  slog.Level
	exported bool
	// identity decides what the exported copy of a record may say about who
	// made the call. Nil redacts everything, which is the zero value's promise
	// and the right default for a caller that forgot.
	identity *Redactor
}

// Enabled asks the stderr handler alone.
//
// The collector leg is strictly narrower, so a record stderr rejects can never
// be one OTLP wants. Consulting both would let the OTLP leg widen what the
// operator's LOG_LEVEL asked for, which is the opposite of what an export floor
// is for.
func (h *fanOutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	// Either leg is reason enough. Delegating to stderr alone made LOG_LEVEL
	// govern the export too: at warn, no INFO record ever reached Handle, and
	// the guide's claim that the export floor is separate was false in that
	// direction. Handle gates each leg on its own.
	if h.stderr.Enabled(ctx, level) {
		return true
	}
	return h.exported && level >= h.otlpMin
}

// Handle writes to stderr first, then to the collector.
//
// The stderr error is the one returned. An OTLP failure is deliberately
// swallowed: a collector that is down must not turn every log call in this
// server into an error path, and the SDK reports its own export failures
// through the handler installed in diagnostics.go, which is where an operator
// should learn about it.
func (h *fanOutHandler) Handle(ctx context.Context, record slog.Record) error {
	var err error
	if h.stderr.Enabled(ctx, record.Level) {
		err = h.stderr.Handle(ctx, record)
	}
	if h.exported && record.Level >= h.otlpMin {
		_ = h.otlp.Handle(ctx, redactRecord(record, h.identity))
	}
	return err
}

// redactRecord returns the record as it may leave the process.
//
// Resource URIs, identity fields, the named strip list, error text and
// oversized values, and only on the exported copy. The rule is the one the span
// attributes already follow: what a request named is governed by the identity
// policy, and a value that arrives by another route is a second carrier the
// policy does not reach.
//
// It has to happen here rather than at the call sites because the records are
// not all ours. The Go SDK logs "resource subscribed" with the URI through the
// logger this server installs, so a rule applied only where this repository
// calls slog would leave the dependency's records untouched, which is how this
// was found: the span carried a digest and the log beside it carried the path.
//
// # Why it is unconditional
//
// It used to return the record untouched unless something in it mentioned a
// gitlab:// URI or an identity field, which is exactly the shape of the
// anonymous "tool call failed" record — the one carrying the GitLab URL and
// GitLab's response body inside its error value. An early return that has to
// enumerate what needs redacting is a second copy of the redaction rules that
// silently disagrees with the first the next time one grows. The exported leg
// starts at Info, so the copy is made on a record that is already leaving the
// process over a network.
func redactRecord(record slog.Record, identity *Redactor) slog.Record {
	redacted := slog.NewRecord(record.Time, record.Level,
		RedactResourceURIs(record.Message), record.PC)

	attrs := make([]slog.Attr, 0, record.NumAttrs())
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})
	redacted.AddAttrs(exportAttrs(attrs, identity)...)
	return redacted
}

// exportStrippedFields are the log fields removed from the exported copy at
// any depth, by name.
//
// A named list rather than a memory. Both this and the identity fields are the
// same shape of defect: a field added to a log line that the export-side
// redactor had no reason to know about, found in production twice. A field
// belongs here when it identifies a caller without being an identity the policy
// governs — token_suffix is the last four characters of the client's
// credential, which authenticates nothing and correlates everything, and it
// survived both the none and the pseudonymous policies untouched because
// neither had heard of it.
//
// The call sites keep writing it: stderr is the operator's own terminal, and
// the suffix is what they correlate a refusal by.
var exportStrippedFields = map[string]bool{
	LogFieldTokenSuffix: true,
}

// exportAttrs returns what the exported copy of an attribute set may carry.
//
// It is the one place that decides, and both suppliers of exported attributes
// go through it: a record's own attributes in redactRecord, and the attributes
// a derived logger attaches in WithAttrs. Having two paths was the hole: the
// attached ones went to the OTLP handler untransformed, so any component logger
// built with slog.With bypassed the policy for whatever it attached.
//
// Identity fields are collected at any depth, groups included, because
// slog.Group is an ordinary value a caller can pass and a group was enough to
// smuggle user and user_id past the previous field check. What the policy
// allows is re-added at the top level under the registry's user.* names, which
// is what a backend already knows how to join on.
func exportAttrs(attrs []slog.Attr, identity *Redactor) []slog.Attr {
	var userID, username string
	var strip func(attrs []slog.Attr) []slog.Attr
	strip = func(attrs []slog.Attr) []slog.Attr {
		out := make([]slog.Attr, 0, len(attrs))
		for _, attr := range attrs {
			// Resolved first: a LogValuer is opaque until asked, so inspecting
			// the unresolved value would wave through whatever it later
			// resolves to, URIs and identity included, on the OTLP handler's
			// side of the policy.
			attr.Value = attr.Value.Resolve()
			switch {
			case attr.Key == LogFieldUserID:
				userID = attr.Value.String()
			case attr.Key == LogFieldUser:
				username = attr.Value.String()
			case exportStrippedFields[attr.Key]:
				// Dropped outright: see exportStrippedFields.
			case attr.Value.Kind() == slog.KindGroup:
				out = append(out, slog.Attr{
					Key:   attr.Key,
					Value: slog.GroupValue(strip(attr.Value.Group())...),
				})
			default:
				out = append(out, redactAttr(attr))
			}
		}
		return out
	}

	out := strip(attrs)
	for _, attr := range identity.Attributes(userID, username) {
		out = append(out, slog.String(string(attr.Key), attr.Value.AsString()))
	}
	return out
}

// redactAttr rewrites one attribute for the exported leg.
//
// Three rules, in the order they can each defeat the next:
//
//   - An error value never exports its text. See [errorTypeName].
//   - A gitlab:// URI is rewritten wherever it appears.
//   - Every value is bounded, so no attribute can be sized by whoever supplied
//     its content.
//
// Groups are descended, because slog.Group is an ordinary value a caller can
// pass and a flat scan reads one as a single opaque value.
func redactAttr(attr slog.Attr) slog.Attr {
	if attr.Value.Kind() == slog.KindGroup {
		group := attr.Value.Group()
		rewritten := make([]slog.Attr, 0, len(group))
		for _, inner := range group {
			rewritten = append(rewritten, redactAttr(inner))
		}
		return slog.Attr{Key: attr.Key, Value: slog.GroupValue(rewritten...)}
	}

	if attr.Value.Kind() == slog.KindAny {
		if err, ok := attr.Value.Any().(error); ok {
			return slog.String(attr.Key, errorTypeName(err))
		}
	}

	text := attr.Value.String()
	if strings.Contains(text, resourceURIScheme) {
		text = RedactResourceURIs(text)
	} else if len(text) <= maxExportedAttrValue && utf8.ValidString(text) {
		// Nothing to change: hand back the original so a typed value keeps its
		// type on the wire rather than becoming a string.
		return attr
	}
	return slog.String(attr.Key, truncateForExport(text))
}

// maxExportedAttrValue bounds one exported attribute value.
//
// Nothing in the logs SDK applies a length limit, so without this an attribute
// built from a caller-controlled string is relayed to the collector byte for
// byte: an unauthenticated request can carry roughly a megabyte in a header,
// and the refusal paths log what they refused. The number is generous enough
// that no legitimate field this server writes is near it, and small enough that
// a flood of them is a rounding error on the operator's bill.
const maxExportedAttrValue = 1024

// truncateForExport bounds a value, saying so where it was cut, and hands back
// something a proto3 string field will accept.
//
// The marker matters more than the bound: a value that was silently shortened
// reads as the whole value, and somebody eventually debugs the difference
// between a truncated host name and a wrong one.
//
// The bound is a byte count, so the cut can land inside a multi-byte character,
// and that costs more than the character. The OTLP log exporters serialize with
// proto.Marshal, which validates every string field, so one partial rune fails
// the upload for the entire batch: every record in it, from every caller, is
// dropped and an SDK error line is all that is left. The value is caller-chosen
// on reachable paths, a refused tools/call being logged with the name the
// caller sent, so the repair covers what arrives as well as what is cut here.
func truncateForExport(text string) string {
	if len(text) <= maxExportedAttrValue {
		return strings.ToValidUTF8(text, string(utf8.RuneError))
	}
	return strings.ToValidUTF8(text[:maxExportedAttrValue], string(utf8.RuneError)) + "[truncated]"
}

// errorTypeName reports the type an error should be classified as, and is the
// whole of what the exported copy says about it.
//
// The otelslog bridge promotes any error-valued attribute into
// exception.message equal to err.Error(), and for this server that string is
// client-go's "METHOD scheme://host/path: CODE body": the URL-encoded project
// path, the query string, and GitLab's own response text, on every failed tool
// call and under every identity policy. Replacing the value with a string both
// removes the text and defeats the promotion at its source, since the bridge
// only promotes a value that still is an error.
//
// A type name is a compile-time constant, so it can carry no request data, and
// it answers the question an operator actually has at this level: whether the
// call failed at the transport or was refused by the API. The status code and
// the timing are on the GitLab client span, which records neither URL nor body.
//
// The chain is walked past the generic wrappers, because fmt.Errorf produces
// *fmt.wrapError for every wrapped error in this tree and classifying every
// failure as that would be the same as classifying none.
func errorTypeName(err error) string {
	for range maxUnwrapDepth {
		name := fmt.Sprintf("%T", err)
		if !genericErrorTypes[name] {
			return name
		}
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			return name
		}
		err = unwrapped
	}
	return "error"
}

// maxUnwrapDepth bounds the walk, because an error chain is built by whatever
// wrapped it and a cyclic Unwrap is a hang rather than a panic.
const maxUnwrapDepth = 16

// genericErrorTypes are the wrapper types that say nothing about what failed.
var genericErrorTypes = map[string]bool{
	"*fmt.wrapError":      true,
	"*fmt.wrapErrors":     true,
	"*errors.errorString": true,
	"*errors.joinError":   true,
}

// WithAttrs applies to both legs, so a logger derived from this one keeps
// exporting. Forgetting either would produce a handler that works until
// somebody calls slog.With, which is the ordinary way to build a component
// logger.
func (h *fanOutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &fanOutHandler{
		stderr: h.stderr.WithAttrs(attrs),
		// The exported copy of attached attributes goes through the same
		// transform as a record's own. Handing them over raw was a bypass: a
		// component logger built with slog.With exported whatever it attached,
		// and redactRecord never saw it because attachment happens here.
		otlp:     h.otlp.WithAttrs(exportAttrs(attrs, h.identity)),
		otlpMin:  h.otlpMin,
		exported: h.exported,
		identity: h.identity,
	}
}

// WithGroup applies to both legs, for the same reason as WithAttrs.
func (h *fanOutHandler) WithGroup(name string) slog.Handler {
	return &fanOutHandler{
		stderr:   h.stderr.WithGroup(name),
		otlp:     h.otlp.WithGroup(name),
		otlpMin:  h.otlpMin,
		exported: h.exported,
		identity: h.identity,
	}
}
