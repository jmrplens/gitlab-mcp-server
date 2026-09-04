package main

import (
	"context"
	"log/slog"
	"strings"
)

// sdkLogger wraps the process logger for the MCP SDK's own use.
//
// The SDK logs nothing unless a Logger is supplied (ensureLogger hands it
// slog.DiscardHandler otherwise), so everything below is a consequence of this
// server asking for its logs, and belongs here rather than upstream.
//
// Two things are wrong with them as they arrive. Measured on the shipped
// default, HTTP mode, after the startup lines have scrolled past: twenty-four
// tools/call produced exactly ninety-six records, and all ninety-six were the
// same four per-session messages. Not most of them. All. In stateless mode
// every POST is a session, so connect, log-level, and disconnect are emitted
// per request, while the calls themselves are logged by the dynamic surface at
// DEBUG and by nothing at INFO. An operator watching the default stream sees
// steady traffic and learns nothing from it, and LOG_LEVEL is no help: raising
// it to warn silenced the SDK and this server's own startup signal together.
//
// The second is worse than noise. "client log level set" carries an attribute
// named level, which is also what slog's JSON handler calls the record's
// severity, and neither side deduplicates: the line goes out with two level
// members, and a parser keeping the last wins reads the severity as the empty
// string. A quarter of the steady-state stream reached an aggregator with no
// severity at all.
func sdkLogger() *slog.Logger {
	return slog.New(&sdkLogHandler{base: slog.Default().Handler()})
}

// sdkLogHandler demotes the SDK's per-session chatter and keeps its attributes
// from colliding with the record's own fields.
type sdkLogHandler struct {
	base slog.Handler
	// grouped records that a group is open, which nests every later attribute
	// and so removes any possibility of a collision at the top level.
	grouped bool
}

// sdkSessionChatter is the set of messages the SDK emits once per session.
//
// Demoted rather than dropped: under LOG_LEVEL=debug they are exactly what is
// wanted when a session is misbehaving. What they cannot be is the whole of the
// default stream.
var sdkSessionChatter = map[string]bool{
	"server run start":            true,
	"server connecting":           true,
	"server session connected":    true,
	"session initialized":         true,
	"client log level set":        true,
	"server session disconnected": true,
	"server session ended":        true,
}

// reservedLogKeys are the attribute names the record itself already uses, which
// a handler writing flat JSON cannot distinguish from an attribute of the same
// name.
var reservedLogKeys = map[string]bool{
	slog.TimeKey:    true,
	slog.LevelKey:   true,
	slog.MessageKey: true,
	slog.SourceKey:  true,
}

// Enabled implements [slog.Handler].
//
// It answers for the level the record arrives with, not the level it may be
// demoted to, because slog consults this before building the record. Handle
// asks again with the final level, so a demoted record is still dropped when
// the base handler would not have it.
func (h *sdkLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

// Handle implements [slog.Handler].
func (h *sdkLogHandler) Handle(ctx context.Context, r slog.Record) error {
	level := h.levelFor(r)
	renamed := !h.grouped && h.hasReservedKey(r)

	if level == r.Level && !renamed {
		return h.base.Handle(ctx, r)
	}
	if !h.base.Enabled(ctx, level) {
		return nil
	}

	rebuilt := slog.NewRecord(r.Time, level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		if renamed && reservedLogKeys[a.Key] {
			a.Key = "sdk_" + a.Key
		}
		rebuilt.AddAttrs(a)
		return true
	})
	return h.base.Handle(ctx, rebuilt)
}

// levelFor returns the level a record should be emitted at.
func (h *sdkLogHandler) levelFor(r slog.Record) slog.Level {
	if sdkSessionChatter[r.Message] {
		return slog.LevelDebug
	}

	// The two lines a normal stop produces. Both are the SDK reporting a
	// shutdown this server asked for, and both used to make an ordinary exit
	// look like a failure: the same misreport as the nonzero exit status
	// serveStdio no longer returns, which is why they are demoted here rather
	// than left as the one half still saying "error".
	//
	// Matching on the text is deliberate and safe in the direction it fails:
	// the cancellation sentinel the SDK formats with %v and the closing error
	// it wraps are both unreachable through errors.Is, and if the wording ever
	// changes the line simply stays at ERROR, which is where it is today.
	// TestSDKLogHandler_DemotesTheShutdownPair pins the wording so the drift is
	// visible rather than silent.
	switch r.Message {
	case "server run cancelled":
		// Only ctx.Done() reaches this, and the only context this process
		// cancels is the one signal.NotifyContext builds. A signal is how a
		// server is meant to be stopped.
		return slog.LevelDebug
	case "server session ended with error":
		if strings.Contains(shutdownCause(r), "server is closing") {
			return slog.LevelDebug
		}
	}
	return r.Level
}

// shutdownCause returns the record's error attribute rendered as text.
func shutdownCause(r slog.Record) string {
	cause := ""
	r.Attrs(func(a slog.Attr) bool {
		if a.Key != "error" {
			return true
		}
		cause = a.Value.String()
		return false
	})
	return cause
}

// hasReservedKey reports whether any attribute would collide with a field the
// record already carries.
func (h *sdkLogHandler) hasReservedKey(r slog.Record) bool {
	collides := false
	r.Attrs(func(a slog.Attr) bool {
		if reservedLogKeys[a.Key] {
			collides = true
			return false
		}
		return true
	})
	return collides
}

// WithAttrs implements [slog.Handler].
func (h *sdkLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &sdkLogHandler{base: h.base.WithAttrs(attrs), grouped: h.grouped}
}

// WithGroup implements [slog.Handler].
func (h *sdkLogHandler) WithGroup(name string) slog.Handler {
	return &sdkLogHandler{base: h.base.WithGroup(name), grouped: h.grouped || name != ""}
}
