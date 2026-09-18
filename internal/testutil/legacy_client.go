// legacy_client.go provides a minimal hand-rolled MCP
// client that performs the legacy initialize handshake at protocol version
// 2025-11-25 and serves server-initiated elicitation/create requests.
//
// The official SDK client always negotiates the newest protocol version and
// offers no exported way to pin an older one, but the legacy synchronous
// elicitation path only exists on sessions negotiated below 2026-07-28
// (SEP-2322 forbids server-initiated requests from that version on). Tests
// that exercise the synchronous path deterministically connect this fake
// client instead of a real one.

package testutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// legacyProtocolVersion is the newest MCP protocol version that still allows
// server-initiated elicitation requests during a tool call.
const legacyProtocolVersion = "2025-11-25"

// handshakeTimeout bounds the wait for the initialize response.
//
// Every connection this drives is in the same process, so the exchange is a
// pair of channel sends and the bound is never approached. What it is for is
// the case where the other end was never connected at all: the read then
// blocks for ever, and a helper that blocks for ever inside a test does not
// fail that test, it stops the whole binary and takes every assertion after it
// unrun. Reporting a handshake that did not happen is worth ten seconds of
// waiting on the one run where it happens.
const handshakeTimeout = 10 * time.Second

// LegacyClientOptions configures the fake legacy client's advertised
// capabilities.
type LegacyClientOptions struct {
	// URLElicitation advertises support for URL-mode elicitation in
	// addition to form elicitation.
	URLElicitation bool
}

// ElicitHandlerFunc handles one server-initiated elicitation request.
type ElicitHandlerFunc func(context.Context, *mcp.ElicitParams) (*mcp.ElicitResult, error)

// legacyReporter is the part of [testing.TB] this client uses.
//
// It is an interface for the reason the GraphQL gate's reporter is one: every
// failure below belongs to a transport that is a [net.Pipe] in the same
// process, so none of them happens in a passing run and none of them could be
// exercised by a test that fails when they fire. A recorder reaches them
// instead.
type legacyReporter interface {
	Helper()
	Cleanup(func())
	Error(args ...any)
	Fatalf(format string, args ...any)
}

// The connection, the encoding and the wait below are variables for the same
// reason the reporter is an interface: an in-memory transport does not fail to
// connect, a map of strings does not fail to marshal, an id made from a string
// literal is always valid, and a serve goroutine reading a closed connection
// always returns. Each failure is still handled, so each is reachable only by
// replacing the call that cannot fail.
var (
	connectSession = func(ctx context.Context, server *mcp.Server, transport mcp.Transport) (*mcp.ServerSession, error) {
		return server.Connect(ctx, transport, nil)
	}
	connectClient = func(ctx context.Context, transport mcp.Transport) (mcp.Connection, error) {
		return transport.Connect(ctx)
	}
	marshalInitParams = json.Marshal
	makeRequestID     = jsonrpc.MakeID
	serveExitTimeout  = 5 * time.Second
)

// ConnectLegacyElicitationClient connects a minimal legacy MCP client
// (protocol 2025-11-25, elicitation capability advertised) to server and
// returns the resulting server session. Server-initiated elicitation/create
// requests are answered by handler; ping requests are acknowledged; all
// other server-initiated requests fail with MethodNotFound. The session and
// the fake client are torn down via t.Cleanup.
//
// After the handshake, handler runs on the fake client's serving goroutine,
// not the test goroutine: report failures inside handler with t.Errorf or by
// returning an error, never t.Fatal/t.FailNow (which only terminate the
// calling goroutine).
func ConnectLegacyElicitationClient(ctx context.Context, t *testing.T, server *mcp.Server, handler ElicitHandlerFunc, opts LegacyClientOptions) *mcp.ServerSession {
	t.Helper()
	return connectLegacyElicitationClient(ctx, t, server, handler, opts)
}

// connectLegacyElicitationClient is the body of
// [ConnectLegacyElicitationClient], reporting through [legacyReporter] rather
// than *testing.T. Each failure returns as well as reporting: a recorder's
// Fatalf does not abort the goroutine the way [testing.T.Fatalf] does, and
// without the return the next line would run on a connection that was never
// made.
func connectLegacyElicitationClient(ctx context.Context, t legacyReporter, server *mcp.Server, handler ElicitHandlerFunc, opts LegacyClientOptions) *mcp.ServerSession {
	t.Helper()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ss, err := connectSession(ctx, server, serverTransport)
	if err != nil {
		t.Fatalf("legacy client: server connect: %v", err)
		return nil
	}
	conn, err := connectClient(ctx, clientTransport)
	if err != nil {
		_ = ss.Close()
		t.Fatalf("legacy client: transport connect: %v", err)
		return nil
	}
	t.Cleanup(func() {
		_ = conn.Close()
		_ = ss.Close()
	})

	if !legacyHandshake(ctx, t, conn, handler, opts) {
		return nil
	}

	served := make(chan struct{})
	go func() {
		defer close(served)
		serveLegacyClient(ctx, conn, handler)
	}()
	// Registered after the connection-closing cleanup, so it runs before it
	// (LIFO): close the connection here and join the serve goroutine. The
	// join is what makes coverage deterministic across GOMAXPROCS — without
	// it, on a single P the test ends before the goroutine ever observes the
	// closed connection, and its exit path counts on some runs and not
	// others. It also stops the goroutine from leaking into later tests.
	t.Cleanup(func() {
		_ = conn.Close()
		awaitServeExit(t, served)
	})
	return ss
}

// legacyHandshake performs the initialize exchange, reporting whether it got
// far enough for the caller to start serving.
func legacyHandshake(ctx context.Context, t legacyReporter, conn mcp.Connection, handler ElicitHandlerFunc, opts LegacyClientOptions) bool {
	t.Helper()

	// The exchange runs on a goroutine of its own and the report stays here.
	//
	// Both halves of that are deliberate. The connection cannot be hurried by
	// a context: the in-memory transport is a pipe, and a pipe blocks until
	// the other end moves whatever deadline the caller carries, so an exchange
	// with an end that was never connected blocks for ever. Waiting on the
	// goroutine instead turns that into one line. And the reporting stays on
	// the caller's goroutine because [testing.T.Fatalf] off it aborts the
	// wrong goroutine and leaves the test hanging or passing, which is the
	// contract in .github/instructions/test-goroutines.instructions.md.
	//
	// A goroutine left behind by an exchange that never finishes is the price,
	// and it is the right one: it ends with the process, while the blocked
	// read it replaces ended the whole binary with every assertion after it
	// unrun.
	done := make(chan error, 1)
	go func() { done <- exchangeLegacyInitialize(ctx, conn, handler, opts) }()

	var err error
	select {
	case err = <-done:
	case <-time.After(handshakeTimeout):
		err = fmt.Errorf("the initialize exchange did not finish within %s", handshakeTimeout)
	}
	if err != nil {
		t.Fatalf("legacy client: %v", err)
		return false
	}
	return true
}

// exchangeLegacyInitialize performs the initialize exchange, naming the step
// that failed. It reports nothing itself: it runs off the test goroutine.
func exchangeLegacyInitialize(ctx context.Context, conn mcp.Connection, handler ElicitHandlerFunc, opts LegacyClientOptions) error {
	capabilities := map[string]any{
		"elicitation": elicitationCapability(opts),
		"roots":       map[string]any{"listChanged": true},
	}
	initParams, err := marshalInitParams(map[string]any{
		"protocolVersion": legacyProtocolVersion,
		"capabilities":    capabilities,
		"clientInfo":      map[string]any{"name": "legacy-test-client", "version": "1.0.0"},
	})
	if err != nil {
		return fmt.Errorf("marshal initialize params: %w", err)
	}
	initID, err := makeRequestID("legacy-init")
	if err != nil {
		return fmt.Errorf("make id: %w", err)
	}
	if writeErr := conn.Write(ctx, &jsonrpc.Request{ID: initID, Method: "initialize", Params: initParams}); writeErr != nil {
		return fmt.Errorf("write initialize: %w", writeErr)
	}
	if handshakeErr := awaitLegacyInitializeResponse(ctx, conn, handler); handshakeErr != nil {
		return handshakeErr
	}
	if notifyErr := conn.Write(ctx, &jsonrpc.Request{Method: "notifications/initialized", Params: json.RawMessage("{}")}); notifyErr != nil {
		return fmt.Errorf("write initialized notification: %w", notifyErr)
	}
	return nil
}

// awaitServeExit waits for the serve goroutine to return, reporting the wait
// that did not end. A goroutine still reading a closed connection would
// otherwise be carried silently into whatever test runs next.
func awaitServeExit(t legacyReporter, served <-chan struct{}) {
	t.Helper()
	select {
	case <-served:
	case <-time.After(serveExitTimeout):
		t.Error("legacy client: serve goroutine did not exit after connection close")
	}
}

// elicitationCapability builds the advertised elicitation capability object.
func elicitationCapability(opts LegacyClientOptions) map[string]any {
	capability := map[string]any{"form": map[string]any{}}
	if opts.URLElicitation {
		capability["url"] = map[string]any{}
	}
	return capability
}

// awaitLegacyInitializeResponse reads messages until the initialize response
// arrives, servicing any interleaved server-initiated requests.
//
// A read that yields neither a message nor an error ends the wait. Nothing a
// real connection does produces that, which is exactly why it is worth
// answering: this loop only ever leaves through a message it recognizes or an
// error, so a connection answering nothing spins it at full speed for the rest
// of the run, and a test binary that never finishes reports no assertion at
// all rather than the one that was wrong.
func awaitLegacyInitializeResponse(ctx context.Context, conn mcp.Connection, handler ElicitHandlerFunc) error {
	for {
		msg, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("read during handshake: %w", err)
		}
		if msg == nil {
			return errors.New("read during handshake: the connection answered no message and no error")
		}
		switch m := msg.(type) {
		case *jsonrpc.Response:
			if m.Error != nil {
				return fmt.Errorf("initialize failed: %w", m.Error)
			}
			return nil
		case *jsonrpc.Request:
			respondLegacyRequest(ctx, conn, m, handler)
		}
	}
}

// serveLegacyClient answers server-initiated requests until the connection
// closes.
func serveLegacyClient(ctx context.Context, conn mcp.Connection, handler ElicitHandlerFunc) {
	for {
		msg, err := conn.Read(ctx)
		if err != nil {
			return
		}
		req, ok := msg.(*jsonrpc.Request)
		if !ok {
			continue
		}
		respondLegacyRequest(ctx, conn, req, handler)
	}
}

// respondLegacyRequest handles one server-initiated request: elicitation
// goes to the handler, pings are acknowledged, and anything else fails
// with MethodNotFound. Notifications are ignored.
func respondLegacyRequest(ctx context.Context, conn mcp.Connection, req *jsonrpc.Request, handler ElicitHandlerFunc) {
	if !req.ID.IsValid() {
		return
	}
	switch req.Method {
	case "elicitation/create":
		var params mcp.ElicitParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeLegacyError(ctx, conn, req.ID, jsonrpc.CodeInvalidParams, err.Error())
			return
		}
		result, err := handler(ctx, &params)
		if err != nil {
			writeLegacyError(ctx, conn, req.ID, jsonrpc.CodeInternalError, err.Error())
			return
		}
		raw, err := json.Marshal(result)
		if err != nil {
			writeLegacyError(ctx, conn, req.ID, jsonrpc.CodeInternalError, err.Error())
			return
		}
		_ = conn.Write(ctx, &jsonrpc.Response{ID: req.ID, Result: raw})
	case "ping":
		_ = conn.Write(ctx, &jsonrpc.Response{ID: req.ID, Result: json.RawMessage("{}")})
	default:
		writeLegacyError(ctx, conn, req.ID, jsonrpc.CodeMethodNotFound, fmt.Sprintf("method %q not supported by legacy test client", req.Method))
	}
}

// writeLegacyError writes a JSON-RPC error response.
func writeLegacyError(ctx context.Context, conn mcp.Connection, id jsonrpc.ID, code int64, message string) {
	_ = conn.Write(ctx, &jsonrpc.Response{ID: id, Error: &jsonrpc.Error{Code: code, Message: message}})
}
