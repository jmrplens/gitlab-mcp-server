package mcpotel

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// sessionTracker records the convention's second server-side instrument,
// mcp.server.session.duration.
//
// # Where a session begins and ends
//
// The SDK exposes no hook for either, so this observes both from the one place
// that sees every session: the receiving middleware. The first request a
// session makes starts the clock, and a goroutine parked on ServerSession.Wait
// stops it. That start is the first request rather than the connect, which is
// honest rather than exact: on stdio the two are milliseconds apart, and on a
// stateful HTTP session the first request is the initialize handshake.
//
// # Not every session is measured
//
// Under the default stateless HTTP transport each POST is its own session that
// ends with its response, so a session duration there would be a histogram of
// request durations, which mcp.server.operation.duration already is, measured
// twice and named differently. Those are skipped. What remains is the two cases
// where a session is a real span of time: stdio, where it is the process
// lifetime, and --stateless=false, where it is what an Mcp-Session-Id names.
//
// The skip is decided by transport and session id rather than by reading
// configuration, so this package stays unaware of the flags: a session with an
// id is stateful whatever set it, and stdio has one session by construction.
type sessionTracker struct {
	duration  metric.Float64Histogram
	constant  []attribute.KeyValue
	isStdio   bool
	mu        sync.Mutex
	observing map[*mcp.ServerSession]struct{}
}

// newSessionTracker builds a tracker, or nil when nothing would be measured.
//
// A nil tracker is a working tracker whose observe is a no-op, which keeps the
// stateless path free of both the map lookup and the goroutine.
func newSessionTracker(meter metric.Meter, constant []attribute.KeyValue, transport string) *sessionTracker {
	histogram, err := meter.Float64Histogram(
		"mcp.server.session.duration",
		metric.WithUnit("s"),
		metric.WithDescription("The duration of the MCP session as observed on the MCP server."),
		// The convention prescribes boundaries for all four of its instruments.
		// A session is longer-lived than a request, so this is the session
		// scale rather than the operation one.
		metric.WithExplicitBucketBoundaries(
			0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300, 900, 1800, 3600, 21600, 86400,
		),
	)
	if err != nil {
		return nil
	}
	return &sessionTracker{
		duration:  histogram,
		constant:  constant,
		isStdio:   transport == TransportPipe,
		observing: make(map[*mcp.ServerSession]struct{}),
	}
}

// observe starts measuring a session the first time one of its requests is
// seen, and is a no-op for every request after that.
func (t *sessionTracker) observe(req mcp.Request, protocolVersion string) {
	if t == nil || req == nil {
		return
	}
	session, ok := req.GetSession().(*mcp.ServerSession)
	if !ok || session == nil {
		return
	}
	// A session with no id under a network transport is one stateless POST.
	if !t.isStdio && session.ID() == "" {
		return
	}

	t.mu.Lock()
	if _, already := t.observing[session]; already {
		t.mu.Unlock()
		return
	}
	t.observing[session] = struct{}{}
	t.mu.Unlock()

	attrs := slices.Clone(t.constant)
	if protocolVersion != "" {
		attrs = append(attrs, AttrMCPProtocolVersion.String(protocolVersion))
	}

	started := time.Now()
	go t.awaitEnd(session, started, attrs)
}

// awaitEnd blocks until the session closes and then records how long it lived.
//
// The context is Background rather than a request's: every request that could
// have carried one has returned by the time Wait unblocks, and a canceled
// context would silently drop the measurement. Metrics need no span to attach
// to, so nothing is lost.
func (t *sessionTracker) awaitEnd(session *mcp.ServerSession, started time.Time, attrs []attribute.KeyValue) {
	err := session.Wait()

	t.mu.Lock()
	delete(t.observing, session)
	t.mu.Unlock()

	// error.type is Conditionally Required "If and only if session ends with an
	// error", so a clean close carries no error attribute at all rather than
	// one saying there was no error.
	if err != nil {
		attrs = append(attrs, AttrErrorType.String(classifyError(err).errorType))
	}
	t.duration.Record(context.Background(), time.Since(started).Seconds(), metric.WithAttributes(attrs...))
}

// sessionIDOf returns the MCP session id, or "" when the request is not part of
// a session.
//
// The empty string is the SDK's own answer for a transport that has no session
// id, which lines up exactly with the convention's condition ("When the MCP
// request or notification is part of a session"), so no interpretation is
// needed on top of it.
func sessionIDOf(req mcp.Request) string {
	if req == nil {
		return ""
	}
	// A request built without a session hands back a non-nil interface wrapping
	// a nil pointer, so the interface comparison passes and the method call
	// then dereferences nil. The typed assertion is what actually catches it.
	session, ok := req.GetSession().(*mcp.ServerSession)
	if !ok || session == nil {
		return ""
	}
	return session.ID()
}
