package main

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// The ceiling on the stateful sessions the process keeps, across every
// credential it serves (register row HLD-010, issue 951).
//
// On --stateless=false the SDK keeps every session a client opens, with the
// goroutines serving it and its owner record (IDN-010), until the client
// deletes it, the pool evicts its credential, or it has sat idle for
// --session-timeout: half an hour by default, and never with a timeout of 0.
// And initialize is metered to no bucket (RTC-004), so nothing stopped a
// caller opening sessions as fast as it could post. An idle session holds no
// connection. The one it can hold open is its standalone stream, a GET the SDK
// serves for as long as the session lives, and those streams are what took the
// base process's descriptors: under a hard limit of 1024 it kept every session
// it was offered until their streams held every descriptor, and from there it
// answered nothing.
//
// So the ceiling is a share of the held-call ceiling rather than a number of
// its own, and each session the process keeps holds one of the held slots
// (HLD-011) for its stream from the POST that opens it until it ends, whether
// the stream is open or not. With the sessions at half of the held slots, the
// descriptor budget the held-call ceiling is sized from holds as it was, a call
// on an open session is still served when every session slot is taken, and a
// session the process keeps is never refused its stream: a client whose
// stream was refused would not ask for it again, and would lose every message
// the server sends outside a response. It is 96 under a hard descriptor limit
// of 1024.
//
// What it bounds is descriptors, and memory only where the limit is small. An
// idle session costs four goroutines and 88 to 110 KiB of resident set,
// measured through cmd/bench_resources' sessions mode at 500 to 4000 sessions,
// and the descriptor limit raises none of that: under the 524288 a systemd
// service gets, the ceiling is 114560 sessions, about ten to twelve GiB of
// them idle, and there the memory the process is given is what bounds the
// sessions. Issue 951 decided that no fixed cap stands beside the derived one:
// the container's memory limit already bounds that memory, and a cap no flag
// moves would be sized for one host. Nor does a per-credential ceiling, and
// initialize stays unmetered: a credential is a key a caller can mint, so a
// per-credential number multiplies with every token it mints.
const sessionHeldDivisor = tenancy.SessionHeldDivisor // register row HLD-010

// statefulSessionsFor is the session ceiling of a process that may hold held
// calls open at once. A process that may hold one call still keeps one
// session: a ceiling of zero would refuse every client of the transport.
func statefulSessionsFor(held int64) int64 {
	return max(1, held/sessionHeldDivisor)
}

// processStatefulSessions is the one count every gate of this process shares:
// a listener serves every credential, and a session belongs to one of them.
//
// It is keyed on the process because only a ceiling keyed on the process
// bounds the sessions it keeps: a per-caller number would multiply by however
// many credentials a caller mints. And no flag moves it, for the held-call
// ceiling's reason (INV-004, INV-018): an operator who could raise it could
// undo the bound. It follows the descriptor limit the process runs under,
// which raises the descriptors it protects and not the memory a session costs.
var processStatefulSessions = &processSlots{limit: statefulSessionsFor(processHeldRequests.limit)}

// opensSession reports whether a request would open a stateful session that
// outlives it: a POST that carries no session id, on a deployment that keeps
// sessions, on a revision that has sessions.
//
// The SDK creates a session for every such POST, whatever it carries, and keeps
// it past the POST only once its initialize has completed. A POST on protocol
// 2026-07-28 or later never gets that far on this transport: the SDK answers
// it, server/discover included, with the revisions the transport serves, so
// the client learns to fall back, and closes its session with the POST.
// Counting one would only replace that answer with a refusal to retry, for a
// slot no session would ever keep.
func (g *mcpServerGate) opensSession(r *http.Request) bool {
	return !g.stateless && r.Method == http.MethodPost && r.Header.Get(mcpSessionIDHeader) == "" &&
		r.Header.Get("MCP-Protocol-Version") < protocolVersionStandardHeaders
}

// takeSessionSlot takes what a POST that would open a stateful session holds
// if its session is kept, or writes the refusal and returns nil: one of the
// process's sessions (HLD-010), and one of its held slots (HLD-011) for the
// standalone stream the session may hold open for its whole life.
//
// The stream's slot is taken here rather than when the stream's GET arrives,
// because a GET refused at a full held ceiling cannot be told to come back: the
// SDK's client gives up on a standalone stream it is refused and does not ask
// again, and the server then drops every message meant for that session
// outside a response, a resource update among them, while the session carries
// on as if nothing were missing. A refused initialize is a failure the client
// sees. The log line names whichever ceiling refused.
func (g *mcpServerGate) takeSessionSlot(w http.ResponseWriter, r *http.Request) *sessionSlot {
	if !g.statefulSessions.acquire() {
		refuseStatefulSession(w, r, g.statefulSessions.limit)
		return nil
	}
	if g.held != nil && !g.held.acquire() {
		g.statefulSessions.release()
		refuseHeldRequest(w, r, g.held.limit)
		return nil
	}
	return &sessionSlot{sessions: g.statefulSessions, stream: g.held}
}

// sessionSlot is what the gate took for the session a POST opens. The session
// keeps it from the first request dispatched on it until it ends; a POST whose
// session never took it gives it back when it ends.
type sessionSlot struct {
	sessions *processSlots
	// stream is the held-call count the slot of the session's standalone
	// stream was taken from, or nil where the gate counts no held call.
	stream *processSlots
	kept   atomic.Bool
}

// sessionSlotKey is the context key a POST's sessionSlot travels under.
type sessionSlotKey struct{}

// keep hands the slot to the session, and reports whether this was the first
// to take it.
func (s *sessionSlot) keep() bool {
	return s.kept.CompareAndSwap(false, true)
}

// release gives back everything the slot holds.
func (s *sessionSlot) release() {
	s.sessions.release()
	if s.stream != nil {
		s.stream.release()
	}
}

// releaseUnkept gives the slot back when no session kept it: the POST opened a
// session the SDK closed with it, or reached no request at all. Taking the flag
// here too is what makes the two ends exclusive, so a slot is released once.
func (s *sessionSlot) releaseUnkept() {
	if s.kept.CompareAndSwap(false, true) {
		s.release()
	}
}

// claimSessionSlot returns the slot the gate took for the POST the carrier
// token names, when no request of that POST has kept it yet. A POST that is
// gone has no carrier left to look up, and one that opened no session carries
// no slot.
func claimSessionSlot(token string) *sessionSlot {
	carrier := mcpCarriers.lookup(token)
	if carrier == nil {
		return nil
	}
	slot, _ := carrier.Value(sessionSlotKey{}).(*sessionSlot)
	if slot == nil || !slot.keep() {
		return nil
	}
	return slot
}

// statefulSessionsMiddleware hands the slot the gate took for a POST that
// opened a session to that session, as the first request on it is dispatched,
// and gives it back when the session ends.
//
// The first request is where the session can be reached: the SDK creates it
// before it reads the body, and hands it to every request it dispatches. The
// slot is kept whatever the request is and however it ends, because the SDK
// closes a session whose initialize did not complete when the POST that opened
// it ends, and that ending gives the slot back the same way any other does. A
// request on a POST the gate took no slot for, which is every request on a
// session that is already open, every stateless one and every stdio one,
// claims nothing and is passed on.
func statefulSessionsMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if session, ok := req.GetSession().(*mcp.ServerSession); ok && session != nil {
			if slot := claimSessionSlot(carrierTokenOf(req)); slot != nil {
				go func() {
					_ = session.Wait()
					slot.release()
				}()
			}
		}
		return next(ctx, method, req)
	}
}

// refuseStatefulSession writes the gate's refusal of a POST that would open a
// session past the ceiling, and the operator's line for it.
//
// The refusal is the held-call ceiling's, word for word: the next action is the
// same, and words that named this ceiling would tell a caller that others hold
// sessions, where the one bit INV-019 accepts is that the process is full. The
// line is throttled like every refusal a caller can cause at will, and names
// the scope and the figure, so an operator knows which bound refused.
func refuseStatefulSession(w http.ResponseWriter, r *http.Request, limit int64) {
	refusalLog.log(r.Context(), slog.LevelWarn, "request refused: too many stateful sessions across the process",
		"scope", "process", "limit_stateful_sessions", limit)
	processBusyFailure().write(w, r)
}
