// Package testutil: legacy_client.go provides a minimal hand-rolled MCP
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

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("legacy client: server connect: %v", err)
	}
	conn, err := clientTransport.Connect(ctx)
	if err != nil {
		_ = ss.Close()
		t.Fatalf("legacy client: transport connect: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		_ = ss.Close()
	})

	capabilities := map[string]any{
		"elicitation": elicitationCapability(opts),
		"roots":       map[string]any{"listChanged": true},
	}
	initParams, err := json.Marshal(map[string]any{
		"protocolVersion": legacyProtocolVersion,
		"capabilities":    capabilities,
		"clientInfo":      map[string]any{"name": "legacy-test-client", "version": "1.0.0"},
	})
	if err != nil {
		t.Fatalf("legacy client: marshal initialize params: %v", err)
	}
	initID, err := jsonrpc.MakeID("legacy-init")
	if err != nil {
		t.Fatalf("legacy client: make id: %v", err)
	}
	if writeErr := conn.Write(ctx, &jsonrpc.Request{ID: initID, Method: "initialize", Params: initParams}); writeErr != nil {
		t.Fatalf("legacy client: write initialize: %v", writeErr)
	}
	if handshakeErr := awaitLegacyInitializeResponse(ctx, conn, handler); handshakeErr != nil {
		t.Fatalf("legacy client: %v", handshakeErr)
	}
	if notifyErr := conn.Write(ctx, &jsonrpc.Request{Method: "notifications/initialized", Params: json.RawMessage("{}")}); notifyErr != nil {
		t.Fatalf("legacy client: write initialized notification: %v", notifyErr)
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
		select {
		case <-served:
		case <-time.After(5 * time.Second):
			t.Error("legacy client: serve goroutine did not exit after connection close")
		}
	})
	return ss
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
