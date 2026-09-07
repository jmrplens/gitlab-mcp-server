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
	"fmt"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// legacyProtocolVersion is the newest MCP protocol version that still allows
// server-initiated elicitation requests during a tool call.
const legacyProtocolVersion = "2025-11-25"

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
		t.Fatalf("legacy client: marshal initialize params: %v", err)
		return false
	}
	initID, err := makeRequestID("legacy-init")
	if err != nil {
		t.Fatalf("legacy client: make id: %v", err)
		return false
	}
	if writeErr := conn.Write(ctx, &jsonrpc.Request{ID: initID, Method: "initialize", Params: initParams}); writeErr != nil {
		t.Fatalf("legacy client: write initialize: %v", writeErr)
		return false
	}
	if handshakeErr := awaitLegacyInitializeResponse(ctx, conn, handler); handshakeErr != nil {
		t.Fatalf("legacy client: %v", handshakeErr)
		return false
	}
	if notifyErr := conn.Write(ctx, &jsonrpc.Request{Method: "notifications/initialized", Params: json.RawMessage("{}")}); notifyErr != nil {
		t.Fatalf("legacy client: write initialized notification: %v", notifyErr)
		return false
	}
	return true
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
func awaitLegacyInitializeResponse(ctx context.Context, conn mcp.Connection, handler ElicitHandlerFunc) error {
	for {
		msg, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("read during handshake: %w", err)
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
