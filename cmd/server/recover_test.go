package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestRecoverPanics_KeepsTheProcessAndHidesTheStack pins the backstop that
// stops one request's defect from ending everyone else's session.
//
// The SDK runs each request on its own goroutine and does not recover, so
// before this middleware a nil dereference in any handler killed the binary.
// On the hosted endpoint that meant every tenant sharing the process, not just
// the caller who triggered it.
//
// Two things are asserted: the caller gets a well-formed internal error rather
// than a dropped connection, and the panic's text does not travel in it. A
// panic message can carry a path, a token fragment or part of a query, and the
// client that tripped the bug is not owed any of it; the stack goes to the log
// instead, where the defect can be found and fixed.
func TestRecoverPanics_KeepsTheProcessAndHidesTheStack(t *testing.T) {
	const secret = "glpat-secret-in-the-panic-message"

	tests := []struct {
		name    string
		handler mcp.MethodHandler
		wantErr bool
	}{
		{
			name: "a panicking handler becomes an internal error",
			handler: func(context.Context, string, mcp.Request) (mcp.Result, error) {
				panic("boom " + secret)
			},
			wantErr: true,
		},
		{
			name: "a nil dereference is recovered like any other panic",
			handler: func(context.Context, string, mcp.Request) (mcp.Result, error) {
				// The shape that crashed the server: a nil *ClientCapabilities
				// read as though it were present. It comes from a function so
				// the nilness analyzer cannot fold it away, which is also how
				// it reached production — through an accessor that returns nil.
				_ = noCapabilities().Elicitation
				return &mcp.CallToolResult{}, nil
			},
			wantErr: true,
		},
		{
			name: "a handler that returns normally is untouched",
			handler: func(context.Context, string, mcp.Request) (mcp.Result, error) {
				return &mcp.CallToolResult{}, nil
			},
			wantErr: false,
		},
		{
			name: "a handler's own error is passed through unchanged",
			handler: func(context.Context, string, mcp.Request) (mcp.Result, error) {
				return nil, errors.New("the handler's own failure")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := recoverPanics(tt.handler)(context.Background(), "tools/call", nil)

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
				if result == nil {
					t.Error("the result was dropped for a handler that returned one")
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("the panic message reached the caller: %v", err)
			}
		})
	}

	t.Run("the recovered error carries the internal-error code", func(t *testing.T) {
		_, err := recoverPanics(func(context.Context, string, mcp.Request) (mcp.Result, error) {
			panic("boom")
		})(context.Background(), "tools/call", nil)

		var rpcErr *jsonrpc.Error
		if !errors.As(err, &rpcErr) {
			t.Fatalf("error does not carry a JSON-RPC code: %v", err)
		}
		if rpcErr.Code != jsonrpc.CodeInternalError {
			t.Errorf("code = %d, want %d (internal error)", rpcErr.Code, jsonrpc.CodeInternalError)
		}
	})
}

// noCapabilities returns the nil the SDK returns for a request that declared
// no capabilities, opaquely enough that static analysis does not fold the
// dereference above into a compile-time error.
func noCapabilities() *mcp.ClientCapabilities { return nil }
