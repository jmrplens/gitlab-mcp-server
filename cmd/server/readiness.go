// readiness.go answers the handshake before the tool catalog exists.
//
// A stdio client writes initialize the instant it has spawned the process.
// There is no socket to connect to and no other event it could wait for, so
// "the process exists" is the only readiness signal the binding gives it. This
// server used to build its whole catalog before it began reading stdin, which
// took 1.8 seconds where the defect was reported and 2.2 to 5.4 seconds where
// it was reproduced, depending on the tool surface. A client that gives up at
// 1.7 seconds and writes initialize again puts two messages in the pipe; when
// the server finally reads, it answers the first and refuses the second with
// `duplicate "initialize" received`, which is correct and fatal. The retry meant
// to recover the connection is what kills it.
//
// The defect is answering the handshake late, not the seconds themselves.
// Making startup faster only moves the threshold: a slower machine, a colder
// page cache or a larger catalog puts it back. So the handshake is answered
// from a connected transport within milliseconds, and everything that needs the
// catalog waits here until registration has finished.
//
// Waiting, not answering early. A client that receives an empty tools/list and
// does not act on notifications/tools/list_changed concludes the server has no
// tools, which is a worse failure than a short wait and far harder to diagnose.
package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// readinessGate holds back every method that needs the tool catalog until
// registration reports it is there.
type readinessGate struct {
	// ready is closed once, by markReady or markFailed, when registration has
	// finished one way or the other.
	ready chan struct{}
	// failure holds the error when registration ended without a catalog. It is
	// written before ready is closed, so a reader that has observed the close
	// observes the error too.
	failure atomic.Pointer[error]
	// lifetime is the server's own context. A waiter released by it is
	// released because the server is going away, not because it is ready.
	lifetime <-chan struct{}
	openOnce sync.Once
}

// newReadinessGate builds a closed gate bound to the server's lifetime.
//
// The lifetime is the same context [createServer] takes, which bounds the
// server rather than any one request. It has to be in the wait because on stdio
// it is the ONLY thing that ends a parked request other than the client: the
// SDK wraps a stdio connection's context in jsonrpc2.notDone, so a request
// context there is never cancelled by shutdown, and mcp.Server.Run waits for
// in-flight requests after it closes the session. A gate watching only the
// request context would hold the process open through its own shutdown.
func newReadinessGate(lifetime context.Context) *readinessGate {
	var done <-chan struct{}
	if lifetime != nil {
		done = lifetime.Done()
	}
	return &readinessGate{ready: make(chan struct{}), lifetime: done}
}

// markReady opens the gate and releases every waiter. Idempotent, because both
// the stdio startup path and [createServer] end by calling it and neither
// should have to know whether the other already did.
func (g *readinessGate) markReady() {
	g.openOnce.Do(func() { close(g.ready) })
}

// markFailed releases every waiter with an error instead of a catalog.
//
// Under stdio the process is leaving when registration fails, so nobody waits
// long enough to care. Under HTTP the server is one pooled entry among many and
// the process stays up, so a failure has to reach the requests parked behind
// it: holding them until their own deadline would report a timeout for
// something that already has a cause, and opening the gate as if all were well
// would answer an empty catalog, which is the one outcome this gate exists to
// prevent.
func (g *readinessGate) markFailed(cause error) {
	g.openOnce.Do(func() {
		g.failure.Store(&cause)
		close(g.ready)
	})
}

// failed reports the registration error, if registration failed.
func (g *readinessGate) failed() error {
	if p := g.failure.Load(); p != nil {
		return *p
	}
	return nil
}

// isReady reports whether the catalog is in place. A gate that failed is not
// ready, so the middleware routes the request into await, which reports why.
func (g *readinessGate) isReady() bool {
	if g.failed() != nil {
		return false
	}
	select {
	case <-g.ready:
		return true
	default:
		return false
	}
}

// gatedConnectionKey marks a connection whose requests are subject to the gate.
type gatedConnectionKey struct{}

// withReadinessGate marks the context a client's transport is served on, so
// requests arriving over it wait for the catalog.
//
// The gate is opt-in per connection, and that direction is deliberate. This
// server talks MCP to itself while it is being built: the exclusion pass, the
// read-only and safe-mode filters, the tool count, the meta-route filter and
// the gitlab://tools manifest each connect an in-memory client and call
// tools/list, some of them on a context.Background() with no deadline. Under an
// opt-OUT gate every one of those had to remember to exempt itself, and the
// first that did not deadlocked registration against itself, waiting for a
// step that could not finish until the call returned. Under this one, the
// default is to pass, and the only thing that waits is a connection somebody
// deliberately handed to a client.
//
// A context value reaches the handler: the SDK derives every request context
// from the one passed to Server.Connect, and jsonrpc2.notDone forwards Value
// even where it suppresses cancellation.
func withReadinessGate(ctx context.Context) context.Context {
	return context.WithValue(ctx, gatedConnectionKey{}, gatedConnectionKey{})
}

// readinessEnforced reports whether this request arrived over a connection the
// gate applies to.
func readinessEnforced(ctx context.Context) bool {
	return ctx.Value(gatedConnectionKey{}) != nil
}

// readinessExempt reports whether a method may be served before the catalog
// exists.
//
// Three calls say nothing about the catalog. initialize reports the server's
// capabilities, which come from configuration and are final before any
// registration happens. server/discover is the same answer for a client
// speaking 2026-07-28, where initialize no longer exists, plus the protocol
// versions the transport supports. ping is liveness. Everything else (tools,
// resources, prompts, completion) would answer about a catalog that is not
// there yet.
//
// That initialize and server/discover are genuinely final is what
// newServerShell declares resources.subscribe for rather than leaving to the
// SDK, which would otherwise add the bit only once a resource had been
// registered and make the handshake depend on the half that is still running.
//
// Notifications pass as a class, and that is a correctness requirement rather
// than a courtesy. The SDK's connection runs its handler queue sequentially and
// moves to the next entry only when a handler calls jsonrpc2.Async, which it
// does for calls and not for notifications: one blocked here would stall every
// message queued behind it. notifications/cancelled is the sharpest case. The
// SDK's preempter has already cancelled the target request by the time the
// notification reaches a handler, so blocking it does not delay the
// cancellation itself, but it does stall whatever the client sends next. A
// notification carries no response either way, so letting one through costs
// nothing.
func readinessExempt(method string) bool {
	switch method {
	case "initialize", "server/discover", "ping":
		return true
	}
	return strings.HasPrefix(method, "notifications/")
}

// middleware returns the receiving middleware that enforces the gate.
//
// Installed as the FIRST middleware, which makes it the innermost: the SDK
// wraps the current handler on each AddReceivingMiddleware call, so the last
// one added runs first. Everything that can refuse or answer a request without
// the catalog therefore does so before any waiting starts: capguard's -32601
// for an undeclared capability, the rate limiter, the argument-shape limit.
// And everything that observes the outcome, telemetry included, sees the real
// one with the wait inside its span.
func (g *readinessGate) middleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if g.isReady() || !readinessEnforced(ctx) || readinessExempt(method) {
				return next(ctx, method, req)
			}
			if err := g.await(ctx, method); err != nil {
				return nil, err
			}
			return next(ctx, method, req)
		}
	}
}

// await blocks until the catalog is ready, the request ends, or the server
// does.
func (g *readinessGate) await(ctx context.Context, method string) error {
	started := time.Now()
	select {
	case <-g.ready:
		if cause := g.failed(); cause != nil {
			return g.abandoned(ctx, method, started, cause)
		}
		// At DEBUG so an operator debugging a slow start can see which methods
		// waited and for how long, without the everyday startup paying for it.
		slog.DebugContext(ctx, "method waited for the tool catalog",
			"method", method, "waited_ms", time.Since(started).Milliseconds())
		return nil
	case <-ctx.Done():
		return g.abandoned(ctx, method, started, ctx.Err())
	case <-g.lifetime:
		return g.abandoned(ctx, method, started, errStoppedBeforeReady)
	}
}

// errStoppedBeforeReady is the cause recorded when the server's own lifetime,
// rather than the request, is what ended the wait.
var errStoppedBeforeReady = errors.New("the server stopped before its tool catalog was ready")

// abandoned answers a request that stopped waiting.
//
// It answers rather than returning nothing, because a call must produce a
// response and the SDK writes one even for a request the client has cancelled
// (jsonrpc2 hands the write a notDone context on purpose). The only choice left
// is what that response says.
//
// codeServerBusy is what this server already uses for refusals that are about
// server state rather than the request, and it is honest here: the request was
// well formed and the same request a moment later succeeds. Returning ctx.Err()
// instead would be marshaled as -32001, "unknown error", which tells a client
// nothing and is indistinguishable from a bug; the cause goes to the log, where
// whoever is debugging a slow start can read it.
func (g *readinessGate) abandoned(ctx context.Context, method string, started time.Time, cause error) error {
	slog.DebugContext(ctx, "method gave up waiting for the tool catalog",
		"method", method, "waited_ms", time.Since(started).Milliseconds(), "cause", cause)
	return &jsonrpc.Error{
		Code: codeServerBusy,
		Message: "the server is still building its tool catalog and this request ended before it was ready; " +
			"retry it",
	}
}

// deferredCallIdentifier lets telemetry be installed before the catalog it
// resolves calls against exists.
//
// The receiving middleware chain has to be complete before the first request
// arrives, because the SDK snapshots it per request: a tools/list that arrives
// during startup and parks in the gate would otherwise be released into
// whatever chain existed when it arrived, skipping every middleware added since
// (the input-schema lockdown and the pagination bounds among them, both of
// which shape what that very response says). So mcpotel.Middleware is installed
// with the rest, and the one thing it needs from registration arrives later
// through here.
//
// A call cannot reach Identify before the catalog is set: tools/call is not
// exempt from the gate, and nothing else is identified.
type deferredCallIdentifier struct {
	resolved atomic.Pointer[mcpotel.CallIdentifier]
}

// set publishes the identifier registration built.
func (d *deferredCallIdentifier) set(identifier mcpotel.CallIdentifier) {
	d.resolved.Store(&identifier)
}

// Identify implements [mcpotel.CallIdentifier].
func (d *deferredCallIdentifier) Identify(toolName string, arguments any) (mcpotel.Identity, bool) {
	held := d.resolved.Load()
	if held == nil || *held == nil {
		return mcpotel.Identity{}, false
	}
	return (*held).Identify(toolName, arguments)
}

// deferredIdentity carries the stdio caller's identity into request contexts
// once startup has resolved it.
//
// stdio resolves one identity for the whole process by asking GitLab who the
// token belongs to, and used to do it before serving so the answer could simply
// live in the context handed to Server.Run. That call is a network round trip,
// which is exactly what must no longer sit between spawn and the handshake, so
// the identity now arrives while the server is already answering. Tool-call
// logging and the telemetry identity policy both read it from the request
// context, so it is put back there per request instead.
//
// Only stdio installs this. HTTP resolves identity per request in authgate.go,
// from the credential that request carried.
type deferredIdentity struct {
	resolved atomic.Pointer[toolutil.UserIdentity]
}

// set publishes the identity startup resolved.
func (d *deferredIdentity) set(identity toolutil.UserIdentity) {
	d.resolved.Store(&identity)
}

// middleware overlays the identity onto each request context.
//
// Installed OUTSIDE mcpotel.Middleware, which resolves the caller through
// toolutil.ResolveIdentity from the context it is given: inside it, a span
// would record nobody while the log line for the same call named someone.
func (d *deferredIdentity) middleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if identity := d.resolved.Load(); identity != nil {
				ctx = toolutil.IdentityToContext(ctx, *identity)
			}
			return next(ctx, method, req)
		}
	}
}
